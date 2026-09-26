package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Related reviews are derived, never authored. A feature, a story, and an
// acceptance criterion each list the reviews that touched them, computed by
// intersecting the code their records reference with each review's changed
// lines, through the same chain that already reaches code: a criterion
// through the design that addresses it and the slide Item that explains it, a
// story through its criteria, a feature through its stories and its own
// records.
//
// There is no relation to add, no record to write, and nothing for an author
// or an agent to keep up to date. The links come from the code diffs
// themselves, so they cannot drift, cannot be forgotten, and cannot be
// inflated by an author who writes more of them. They are a footnote on the
// record, never a headline: no table column, no count in a header.
//
// The intersection is the coverage engine, run over the documentation tree
// against a review's own range. That is exactly what review coverage already
// does to the review's deck, read the other way round.

// relatedReviewView is one review that changed code a record explains.
type relatedReviewView struct {
	ID     string
	Title  string
	Href   string
	Number int
	Merged bool
}

// relatedReviewIndex is the derived index, by record URN. It is keyed by URN
// rather than by view so a feature, a story, and a criterion all read from
// one build.
type relatedReviewIndex struct {
	byRecord map[string][]relatedReviewView
	// Reviews is how many reviews were intersected, and Elapsed how long the
	// whole build took. Both are reported by the performance test rather than
	// shown to a reader.
	Reviews int
	Elapsed time.Duration
}

// For names the reviews that touched one record, or nothing.
func (index *relatedReviewIndex) For(urn string) []relatedReviewView {
	if index == nil || urn == "" {
		return nil
	}
	return index.byRecord[urn]
}

// relatedReviewCache keeps one build alive while nothing it reads has
// changed: the Saga's own files, including the code references and the
// reviews the outline cache deliberately skips, and the source repository's
// head. A documentation page therefore pays for the cross product once, not
// once per request.
type relatedReviewCache struct {
	mutex       sync.Mutex
	fingerprint string
	index       *relatedReviewIndex
	builds      int
	// touched is the documentation targets each review's range touched, by
	// what decides them: the range's commits, the documentation's code, and
	// the checkout's attribute files. None changes under its key, so an edit to the Saga's prose or
	// records rebuilds the index without reading any review's diff again.
	touched map[string][]string
}

// relatedReviews is the derived index for this request. It is empty, and
// costs nothing, while the Saga has no reviews.
//
// Everything it is built from is read under the state it is kept under: the
// records, with the complete slides' links projected in as every page reads
// them, come from that state's files, and the Saga with its code is read
// after that state was taken. An index is therefore never kept under a state
// newer than what it was built from.
func (a *app) relatedReviews(ctx context.Context) *relatedReviewIndex {
	empty := &relatedReviewIndex{byRecord: map[string][]relatedReviewView{}}
	a.related.mutex.Lock()
	defer a.related.mutex.Unlock()
	state := a.sagaState(ctx, true)
	fingerprint, err := state.relatedKey()
	// Git reads the checkout's attribute files when it diffs a review's
	// range, so the index is kept under them too.
	if attributes, attributesErr := checkoutAttributes(ctx, a.sourceDir); attributesErr != nil {
		err = attributesErr
	} else {
		fingerprint += "\x00" + attributes
	}
	if err == nil && a.related.index != nil && fingerprint == a.related.fingerprint {
		return a.related.index
	}
	files := a.sagaFilesAt(state)
	narrative := files.narrative()
	if narrative == nil || len(narrative.Reviews) == 0 {
		return empty
	}
	records, recordsErr := files.projectedRecords(narrative.Manifest.ID)
	if recordsErr != nil {
		return empty
	}
	// The narrative carries no code, so the intersection reads the Saga once
	// with its references. Reviews live in the same document.
	full, validation, loadErr := saga.Load(a.root)
	if loadErr != nil || !validation.Valid {
		return empty
	}
	// A resolver serves one build. What it reads is kept only for the build,
	// so a read that failed, or a pinned commit a later fetch brings in, is
	// asked again by the next one.
	var resolver coverage.Resolver
	if opened, openErr := coderesolve.New(ctx, a.sourceDir); openErr == nil {
		defer opened.Close()
		resolver = opened
	}
	if a.related.touched == nil {
		a.related.touched = map[string][]string{}
	}
	index := buildRelatedReviews(ctx, a.sourceDir, full, records, resolver, a.related.touched)
	// A build the request abandoned may have read nothing for some reviews;
	// it answers that request and is kept for no other.
	if err == nil && ctx.Err() == nil {
		a.related.fingerprint, a.related.index = fingerprint, index
		a.related.builds++
	}
	return index
}

// relatedFingerprint commits to everything the intersection reads: every file
// of the Saga, code references and reviews included, and the source head the
// references resolve against.
func (a *app) relatedFingerprint(ctx context.Context) (string, error) {
	return a.sagaState(ctx, true).relatedKey()
}

// buildRelatedReviews intersects every review's changed lines with the code
// the documentation references, and attributes what it finds to the records
// that reach that code.
//
// touched holds what earlier builds found for each review, keyed by the
// review's commits, the documentation's code, and the checkout's attribute
// files; it is read and refilled, and
// keeps only what this build used.
func buildRelatedReviews(ctx context.Context, sourceDir string, document *saga.Saga, records requirements.Document, resolver coverage.Resolver, touched map[string][]string) *relatedReviewIndex {
	started := time.Now()
	index := &relatedReviewIndex{byRecord: map[string][]relatedReviewView{}}
	chain := newRecordChain(document, records)
	documented := documentedPaths(document)
	if len(documented) == 0 {
		// Documentation that references no code intersects no review.
		index.Elapsed = time.Since(started)
		return index
	}
	code := documentedCodeDigest(document)
	// Git diffs each range under the checkout's attribute files, so what a
	// range touched is kept under them as well. A checkout that cannot be
	// found reads no range, so nothing is kept under the empty identity.
	attributes, _ := checkoutAttributes(ctx, sourceDir)
	var touchedMutex sync.Mutex
	used := map[string]bool{}
	// Each review is an independent intersection, and most of the cost is Git
	// reads, so they run together. The resolver is safe for concurrent use and
	// its blob cache is shared across them.
	type touchedReview struct {
		view    relatedReviewView
		records map[string]bool
	}
	results := make([]*touchedReview, len(document.Reviews))
	jobs := make(chan int)
	workers := min(len(document.Reviews), 8)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for position := range jobs {
				review := document.Reviews[position]
				rng, err := reviewstate.ResolveRange(ctx, sourceDir, review)
				if err != nil {
					continue
				}
				key := rng.BaseOID + "\x00" + rng.HeadOID + "\x00" + code + "\x00" + attributes
				touchedMutex.Lock()
				targets, known := touched[key]
				used[key] = true
				touchedMutex.Unlock()
				if !known {
					settled := &settledResolver{resolver: resolver}
					targets, err = touchedTargets(ctx, sourceDir, document, documented, rng, settled)
					if err != nil {
						continue
					}
					// Only an answer decided by the range and the code alone
					// is kept under them: not one a failed or abandoned read,
					// or a pinned commit this repository lacks, had a part in.
					if !settled.provisional.Load() && ctx.Err() == nil {
						touchedMutex.Lock()
						touched[key] = targets
						touchedMutex.Unlock()
					}
				}
				view := relatedReviewView{ID: review.ID, Title: review.Title, Href: reviewHref(review.ID), Merged: review.Merged != nil}
				if view.Title == "" {
					view.Title = review.ID
				}
				if review.PullRequest != nil {
					view.Number = review.PullRequest.Number
				}
				result := &touchedReview{view: view, records: map[string]bool{}}
				results[position] = result
				for _, target := range targets {
					for _, record := range chain.recordsFor(target) {
						result.records[record] = true
					}
				}
			}
		}()
	}
	for position := range document.Reviews {
		jobs <- position
	}
	close(jobs)
	wait.Wait()
	for key := range touched {
		if !used[key] {
			delete(touched, key)
		}
	}
	for _, result := range results {
		if result == nil {
			continue
		}
		index.Reviews++
		for record := range result.records {
			index.byRecord[record] = append(index.byRecord[record], result.view)
		}
	}
	for record := range index.byRecord {
		rows := index.byRecord[record]
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	}
	index.Elapsed = time.Since(started)
	return index
}

// settledResolver notes whether any answer it passed on was provisional:
// one that may change without the range or the code changing.
type settledResolver struct {
	resolver    coverage.Resolver
	provisional atomic.Bool
}

func (settled *settledResolver) Resolve(ctx context.Context, reference coderef.Reference, view string) coderesolve.Resolution {
	if settled.resolver == nil {
		settled.provisional.Store(true)
		return coderesolve.Resolution{State: coderesolve.Stale, Location: reference.Location(), Reason: "the source repository could not be read", Provisional: true}
	}
	resolution := settled.resolver.Resolve(ctx, reference, view)
	if resolution.Provisional {
		settled.provisional.Store(true)
	}
	return resolution
}

// touchedTargets is every documentation target that accounts for a line the
// review's range changed: the documentation's own coverage of the range. The
// rest of the coverage report describes only the files read, so only its
// targets are kept.
func touchedTargets(ctx context.Context, sourceDir string, document *saga.Saga, documented []string, rng reviewstate.Range, resolver coverage.Resolver) ([]string, error) {
	changes, err := documentedChanges(ctx, sourceDir, document, documented, rng)
	if err != nil {
		return nil, err
	}
	targets := []string{}
	if len(changes.Atoms) == 0 {
		return targets, nil
	}
	report := coverage.EvaluateTargets(ctx, changedFileTargets(document, changes), saga.Validation{Valid: true}, changes, resolver)
	for _, target := range report.Targets {
		if target.Covered > 0 {
			targets = append(targets, target.Target)
		}
	}
	return targets, nil
}

// documentedCodeDigest commits to everything of the documentation a review's
// intersection reads: the repository it names and the code every target
// references. Prose, records, and reviews are not part of it.
func documentedCodeDigest(document *saga.Saga) string {
	digest := sha256.New()
	fmt.Fprintf(digest, "%s\x00", document.Manifest.Source.Repository)
	coverage.WalkDocumentCode(document, func(target string, code []saga.CodeFile) {
		encoded, _ := json.Marshal(code)
		fmt.Fprintf(digest, "%s\x00%s\x00", target, encoded)
	})
	return hex.EncodeToString(digest.Sum(nil))
}

// recordChain is the one chain that already reaches code, read backwards: a
// documentation target that references code names the records that reach it.
// It is built from the same relations the record pages trace along, so a
// related review and a traced design are the same link followed in opposite
// directions.
type recordChain struct {
	sagaID string
	// itemSlide names the slide an Item belongs to, so an Item's code counts
	// for the slide that shows it, exactly as linked code rolls up today.
	itemSlide map[string]string
	// targetFeature names the feature whose report, design, or deck holds a
	// target.
	targetFeature map[string]string
	// outbound are the active relations leaving a documentation target: the
	// criteria a design addresses, and the records a slide explains.
	outbound map[string][]requirements.Relation
	// criterionStory and storyFeature climb from a criterion to its story and
	// from a story to its feature.
	criterionStory map[string]string
	storyFeature   map[string]string
}

func newRecordChain(document *saga.Saga, records requirements.Document) *recordChain {
	chain := &recordChain{
		sagaID: document.Manifest.ID, itemSlide: map[string]string{},
		targetFeature: featureTargets(document), outbound: map[string][]requirements.Relation{},
		criterionStory: map[string]string{}, storyFeature: map[string]string{},
	}
	// A feature's decks are its records too: its slides and their Items.
	for _, feature := range document.Features {
		for _, deck := range feature.Decks {
			chain.targetFeature[deck.Target] = feature.ID
			for _, slide := range deck.Slides {
				chain.targetFeature[slide.Target] = feature.ID
				for _, item := range slide.Items {
					chain.targetFeature[item.Target] = feature.ID
					chain.itemSlide[item.Target] = slide.Target
				}
			}
		}
	}
	for _, relation := range records.Relations {
		if relation.State != requirements.RelationActive {
			continue
		}
		chain.outbound[relation.From] = append(chain.outbound[relation.From], relation)
	}
	for _, story := range records.Stories {
		storyURN, err := livingid.Story(records.SagaID, story.Identity.ID)
		if err != nil {
			continue
		}
		if story.Feature != "" {
			chain.storyFeature[storyURN] = applayout.FeatureURN(records.SagaID, story.Feature)
		}
		if story.CurrentRevision == nil {
			continue
		}
		for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
			if urn, err := livingid.Criterion(records.SagaID, story.Identity.ID, criterion.ID); err == nil {
				chain.criterionStory[urn] = storyURN
			}
		}
	}
	return chain
}

// recordsFor names every record a documentation target belongs to: the
// feature that holds it, the criteria and stories its relations reach, and
// the stories and features above those. An Item also counts for the slide
// that shows it, so an Item's code reaches whatever the slide explains.
func (chain *recordChain) recordsFor(target string) []string {
	found := map[string]bool{}
	seeds := []string{target}
	if slide := chain.itemSlide[target]; slide != "" {
		seeds = append(seeds, slide)
	}
	for _, seed := range seeds {
		if feature := chain.targetFeature[seed]; feature != "" {
			found[applayout.FeatureURN(chain.sagaID, feature)] = true
		}
		for _, relation := range chain.outbound[seed] {
			chain.climb(relation.To, found)
		}
	}
	result := make([]string, 0, len(found))
	for record := range found {
		result = append(result, record)
	}
	return result
}

// climb marks a record and everything above it: a criterion marks its story,
// and a story marks its feature.
func (chain *recordChain) climb(urn string, found map[string]bool) {
	switch recordKind(urn) {
	case "criterion":
		found[urn] = true
		if story := chain.criterionStory[urn]; story != "" {
			chain.climb(story, found)
		}
	case "story":
		found[urn] = true
		if feature := chain.storyFeature[urn]; feature != "" {
			found[feature] = true
		}
	case "feature":
		found[urn] = true
	}
}

// recordKind says which kind of record a URN names, or "" for anything the
// footnote does not carry.
func recordKind(urn string) string {
	parts := strings.Split(urn, ":")
	if len(parts) < 5 {
		return ""
	}
	switch parts[3] {
	case "feature":
		return "feature"
	case "story":
		if len(parts) == 7 && parts[5] == "criterion" {
			return "criterion"
		}
		if len(parts) == 5 {
			return "story"
		}
	}
	return ""
}

// attachRelatedReviews hangs the derived footnote on the records a page
// shows: the feature, the story, and each of its criteria. Nothing else
// carries it, and no table or header counts it.
func attachRelatedReviews(index *relatedReviewIndex, data *pageData) {
	if feature := data.Feature; feature != nil {
		feature.RelatedReviews = index.For(feature.Target)
	}
	page := data.Requirements
	if page == nil || page.Story == nil {
		return
	}
	page.Story.RelatedReviews = index.For(page.Story.Target)
	for _, criterion := range page.Story.Criteria {
		criterion.RelatedReviews = index.For(criterion.Target)
	}
}

// relatedReviewsView is the footnote as one record's page renders it: the
// reviews, and the word for what kind of record they touched.
type relatedReviewsView struct {
	Kind    string
	Reviews []relatedReviewView
}

// documentedPaths is every file the documentation tree references. It bounds
// the intersection: a reference can only account for changes in a file it
// names, so no other file of a review's range has to be read at all.
func documentedPaths(document *saga.Saga) []string {
	seen := map[string]bool{}
	var paths []string
	coverage.WalkDocumentCode(document, func(_ string, code []saga.CodeFile) {
		for _, file := range code {
			for _, reference := range file.References {
				if !seen[reference.Path] {
					seen[reference.Path] = true
					paths = append(paths, reference.Path)
				}
			}
		}
	})
	sort.Strings(paths)
	return paths
}

// documentedChanges reads only the part of a review's range the documentation
// could possibly explain: the patch of the files its code references name, and
// of no others. A whole range parsed per review is a pull request's worth of
// lines, and a documentation page would pay that once per review on the page.
func documentedChanges(ctx context.Context, sourceDir string, document *saga.Saga, paths []string, rng reviewstate.Range) (gitdiff.ChangeSet, error) {
	return gitdiff.ReadPaths(ctx, sourceDir, document.Manifest.Source.Repository, rng.BaseOID, rng.HeadOID, gitdiff.ReadOptions{AllowRepositoryMismatch: true}, paths...)
}

// changedFileTargets walks only the documentation targets whose references
// name a file this review changed. Resolving a reference is a Git read, and a
// reference in a file the review never touched can account for nothing, so
// resolving it would cost the page a round trip for a certainty.
func changedFileTargets(document *saga.Saga, changes gitdiff.ChangeSet) func(func(string, []saga.CodeFile)) {
	changed := map[string]bool{}
	for _, atom := range changes.Atoms {
		for _, path := range []string{atom.Path, atom.OldPath, atom.NewPath} {
			if path != "" {
				changed[path] = true
			}
		}
	}
	return func(visit func(string, []saga.CodeFile)) {
		coverage.WalkDocumentCode(document, func(target string, code []saga.CodeFile) {
			for _, file := range code {
				for _, reference := range file.References {
					if changed[reference.Path] {
						visit(target, code)
						return
					}
				}
			}
		})
	}
}
