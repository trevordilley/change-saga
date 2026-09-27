package cli

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// staleRow is one stale reference with where its diff proposes it now is.
// Accept is the one-line command that takes the proposal, present only for
// coverage evidence with a proposal: claims and quality evidence are
// append-only, and a term is revised through term revise.
type staleRow struct {
	ownedReference
	Pinned   coderef.Location     `json:"pinned"`
	Reason   string               `json:"reason"`
	Proposal coderesolve.Proposal `json:"proposal"`
	Accept   *grammar.Invocation  `json:"accept,omitempty"`
}

// changeStaleness is what a change did to the Saga's references: those it
// made stale, each with a proposal, and a count of those already stale
// before it. A reference is made stale by the change when it is stale at the
// head and either current at the merge-base or pinned at a commit the
// merge-base does not contain (evidence written during the change).
type changeStaleness struct {
	BaseOID string `json:"base_oid"`
	HeadOID string `json:"head_oid"`
	// Count is how many references the change made stale.
	Count int `json:"count"`
	// Proposed is how many of them have a proposed range to accept.
	Proposed int `json:"proposed"`
	// PreExisting is how many other references were already stale.
	PreExisting int        `json:"pre_existing"`
	References  []staleRow `json:"references"`
}

// historicalOwners names quality evidence a later record superseded: its
// staleness is history, not debt.
func historicalOwners(document *saga.Saga) map[string][]string {
	historical := map[string][]string{}
	qualityDoc, err := quality.Load(document.Root)
	if err != nil {
		return historical
	}
	for _, test := range qualityDoc.TestCases {
		heads := map[string]bool{}
		for _, head := range test.EvidenceHeads {
			heads[head] = true
		}
		for _, evidence := range test.Evidence {
			owner, _ := qualityid.Evidence(document.Manifest.ID, test.Identity.ID, evidence.ID)
			if !heads[owner] {
				historical[owner] = []string{"evidence was superseded"}
			}
		}
	}
	return historical
}

// measureChangeStaleness classifies every stale reference at head against
// base. root and repo shape the accept commands.
func measureChangeStaleness(ctx context.Context, resolver *coderesolve.Resolver, owned []ownedReference, historical map[string][]string, base, head, root, repo string) changeStaleness {
	result := changeStaleness{BaseOID: base, HeadOID: head, References: []staleRow{}}
	inBase := map[string]bool{}
	pinnedBefore := func(commit string) bool {
		known, ok := inBase[commit]
		if !ok {
			exists, _ := resolver.CommitExists(ctx, commit)
			known = !exists || commit == base || isAncestor(ctx, resolver.Repository(), commit, base)
			inBase[commit] = known
		}
		return known
	}
	for _, ref := range owned {
		if len(historical[ref.Owner]) > 0 {
			continue
		}
		resolution := resolver.Resolve(ctx, ref.Code, head)
		if resolution.Current() {
			continue
		}
		byChange := base != "" && base != head && (!pinnedBefore(ref.Code.Commit) || resolver.Resolve(ctx, ref.Code, base).Current())
		if !byChange {
			result.PreExisting++
			continue
		}
		row := staleRow{ownedReference: ref, Pinned: ref.Code.Location(), Reason: resolution.Reason, Proposal: resolver.Propose(ctx, ref.Code, head)}
		if row.Proposal.Proposed() {
			result.Proposed++
			row.Accept = acceptInvocation(ref, head, root, repo)
		}
		result.Count++
		result.References = append(result.References, row)
	}
	return result
}

// acceptInvocation is the one-line accept for one evidence reference.
func acceptInvocation(ref ownedReference, head, root, repo string) *grammar.Invocation {
	if ref.Kind != "evidence" || ref.EvidenceFile == "" {
		return nil
	}
	values := []grammar.Value{grammar.V("accept-proposed", "true"), grammar.V("record", ref.EvidenceFile), grammar.V("reference", strconv.Itoa(ref.Index))}
	if head != "" {
		values = append(values, grammar.V("head", head))
	}
	if repo != "" {
		values = append(values, grammar.V("repo", repo))
	}
	invocation := grammar.MustInvoke("repin", root, values...)
	return &invocation
}

// shortLocation spells a location without its commit: path#Lstart-Lend.
func shortLocation(location coderef.Location) string {
	spelled := location.String()
	if _, rest, ok := strings.Cut(spelled, ":"); ok {
		return rest
	}
	return spelled
}

// describeProposal is "old -> proposed" or why nothing is proposed.
func describeProposal(pinned coderef.Location, proposal coderesolve.Proposal) string {
	if !proposal.Proposed() {
		return shortLocation(pinned) + " -> nothing proposed: " + proposal.Reason
	}
	text := shortLocation(pinned) + " -> " + shortLocation(*proposal.Location)
	if proposal.Widened {
		text += " (widened)"
	}
	return text
}

// printChangeStaleness leads a report with what the change made stale.
func printChangeStaleness(out io.Writer, staleness changeStaleness, preExistingHint string) {
	switch staleness.Count {
	case 0:
		fmt.Fprintln(out, "Your change made no references stale.")
	case 1:
		fmt.Fprintln(out, "Your change made 1 reference stale:")
	default:
		fmt.Fprintf(out, "Your change made %d references stale:\n", staleness.Count)
	}
	for _, row := range staleness.References {
		fmt.Fprintf(out, "  %s  %s\n", row.Owner, describeProposal(row.Pinned, row.Proposal))
	}
	if staleness.Proposed > 0 {
		fmt.Fprintf(out, "  %d with a proposed range: read its diff, then accept with change-saga repin --accept-proposed --record FILE [--reference N]\n", staleness.Proposed)
	}
	if staleness.PreExisting > 0 {
		fmt.Fprintf(out, "Pre-existing stale references: %d (stale before this change%s)\n", staleness.PreExisting, preExistingHint)
	}
}

// reviewReferences lists an open review's Item evidence like sagaReferences.
func reviewReferences(review *saga.Review) []ownedReference {
	var result []ownedReference
	if review.Deck == nil {
		return result
	}
	for _, slide := range review.Deck.Slides {
		for _, item := range slide.Items {
			for _, file := range item.Code {
				for index, reference := range file.References {
					result = append(result, ownedReference{Kind: "evidence", Owner: item.Target, EvidenceFile: filepath.ToSlash(file.Path), Index: index + 1, Code: reference})
				}
			}
		}
	}
	return result
}

// acceptScope is what repin --accept-proposed repairs.
type acceptScope struct {
	record, target, review string
	reference              int
	all                    bool
}

func (scope acceptScope) includes(ref ownedReference, review string) bool {
	switch {
	case scope.record != "":
		return ref.EvidenceFile == filepath.ToSlash(filepath.Clean(scope.record)) && (scope.reference == 0 || ref.Index == scope.reference)
	case scope.target != "":
		return ref.Owner == scope.target || strings.HasPrefix(ref.Owner, scope.target+":")
	case scope.review != "":
		return review == scope.review
	}
	return scope.all
}

type acceptChange struct {
	Kind         string           `json:"kind"`
	Owner        string           `json:"owner"`
	EvidenceFile string           `json:"evidence_file"`
	Reference    int              `json:"reference"`
	From         coderef.Location `json:"from"`
	To           coderef.Location `json:"to"`
	Widened      bool             `json:"widened,omitempty"`
	Diff         []string         `json:"diff,omitempty"`
	replacement  coderef.Reference
	previous     coderef.Reference
}

type acceptRefusal struct {
	Kind         string           `json:"kind"`
	Owner        string           `json:"owner"`
	EvidenceFile string           `json:"evidence_file,omitempty"`
	Reference    int              `json:"reference"`
	Pinned       coderef.Location `json:"pinned"`
	Reason       string           `json:"reason"`
}

type acceptOutput struct {
	OK       bool            `json:"ok"`
	DryRun   bool            `json:"dry_run"`
	Accepted []acceptChange  `json:"accepted"`
	Refused  []acceptRefusal `json:"refused"`
	// Current counts references in scope that needed nothing.
	Current int `json:"current"`
	// Slides are the complete-slide transactions that carried accepted
	// evidence of slides apply-slide manages.
	Slides []SlideTransactionResult `json:"slide_transactions"`
}

// acceptProposed is repin --accept-proposed.
func acceptProposed(ctx context.Context, root, repoDir, headRev string, scope acceptScope, dryRun, jsonOutput bool, out io.Writer) error {
	document, _, err := saga.Load(root)
	if err != nil {
		return err
	}
	checkout := firstNonEmpty(repoDir, document.Root)
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return err
	}
	defer resolver.Close()
	head, err := resolveCommit(ctx, checkout, firstNonEmpty(headRev, "HEAD"))
	if err != nil {
		return err
	}
	type candidate struct {
		ref        ownedReference
		review     string
		base, head string
	}
	var candidates []candidate
	if scope.review == "" {
		owned, err := sagaReferences(document)
		if err != nil {
			return err
		}
		for _, ref := range owned {
			candidates = append(candidates, candidate{ref: ref, head: head})
		}
	}
	if scope.review != "" && document.FindReview(scope.review) == nil {
		return fmt.Errorf("review %q does not exist%s", scope.review, knownReviews(document))
	}
	for _, review := range document.Reviews {
		if scope.review != "" && review.ID != scope.review {
			continue
		}
		refs := reviewReferences(review)
		if len(refs) == 0 || (scope.review == "" && !scope.all && !anyIncluded(scope, refs, review.ID)) {
			continue
		}
		if review.Merged != nil {
			if scope.review != "" || scope.record != "" {
				return fmt.Errorf("review %q is history: its evidence cannot be repaired after landing", review.ID)
			}
			continue
		}
		rng, err := reviewstate.ResolveRange(ctx, checkout, review)
		if err != nil {
			return fmt.Errorf("read review %s's range: %w", review.ID, err)
		}
		for _, ref := range refs {
			candidates = append(candidates, candidate{ref: ref, review: review.ID, base: rng.BaseOID, head: rng.HeadOID})
		}
	}
	result := acceptOutput{OK: true, DryRun: dryRun, Accepted: []acceptChange{}, Refused: []acceptRefusal{}, Slides: []SlideTransactionResult{}}
	matched := 0
	for _, value := range candidates {
		if !scope.includes(value.ref, value.review) {
			continue
		}
		matched++
		view := value.head
		if value.review != "" {
			// A review Item's deleted-side evidence lives at the merge-base; it
			// keeps its side when the base moves.
			if value.ref.Code.Commit == value.base || isAncestor(ctx, checkout, value.ref.Code.Commit, value.base) {
				view = value.base
			}
			if resolver.Resolve(ctx, value.ref.Code, value.base).Current() {
				result.Current++
				continue
			}
		}
		if resolver.Resolve(ctx, value.ref.Code, view).Current() {
			result.Current++
			continue
		}
		refuse := func(reason string) {
			result.Refused = append(result.Refused, acceptRefusal{Kind: value.ref.Kind, Owner: value.ref.Owner, EvidenceFile: value.ref.EvidenceFile, Reference: value.ref.Index, Pinned: value.ref.Code.Location(), Reason: reason})
		}
		switch value.ref.Kind {
		case "evidence":
		case "term":
			refuse("a term's code is revised with change-saga term revise; its proposal is " + describeProposal(value.ref.Code.Location(), resolver.Propose(ctx, value.ref.Code, view)))
			continue
		default:
			refuse(value.ref.Kind + " records are append-only; record a new one rather than repinning")
			continue
		}
		proposal := resolver.Propose(ctx, value.ref.Code, view)
		if !proposal.Proposed() {
			refuse("nothing proposed: " + proposal.Reason)
			continue
		}
		replacement, err := resolver.Author(ctx, *proposal.Location, value.ref.Code.Note)
		if err != nil {
			refuse(err.Error())
			continue
		}
		result.Accepted = append(result.Accepted, acceptChange{
			Kind: value.ref.Kind, Owner: value.ref.Owner, EvidenceFile: value.ref.EvidenceFile, Reference: value.ref.Index,
			From: value.ref.Code.Location(), To: replacement.Location(), Widened: proposal.Widened, Diff: proposal.Diff,
			replacement: replacement, previous: value.ref.Code,
		})
	}
	if matched == 0 {
		return fmt.Errorf("no evidence reference matches the scope; list them with change-saga references --stale")
	}
	if scope.record != "" && scope.reference != 0 && len(result.Refused) > 0 {
		refused := result.Refused[0]
		return fmt.Errorf("%s #%d has no proposal to accept: %s", refused.EvidenceFile, refused.Reference, refused.Reason)
	}
	if len(result.Accepted) > 0 {
		slides, err := writeAcceptedEvidence(ctx, root, repoDir, result.Accepted, dryRun)
		if err != nil {
			return err
		}
		result.Slides = slides
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	verb := "Accepted"
	if dryRun {
		verb = "Would accept"
	}
	fmt.Fprintf(out, "%s %d proposed %s (%d in scope already current, %d refused)\n", verb, len(result.Accepted), plural(len(result.Accepted), "range", "ranges"), result.Current, len(result.Refused))
	for _, change := range result.Accepted {
		widened := ""
		if change.Widened {
			widened = " (widened)"
		}
		fmt.Fprintf(out, "  %s #%d %s -> %s%s\n", change.EvidenceFile, change.Reference, shortLocation(change.From), shortLocation(change.To), widened)
		for _, line := range change.Diff {
			fmt.Fprintf(out, "      %s\n", line)
		}
	}
	for _, refused := range result.Refused {
		fmt.Fprintf(out, "  refused %s #%d %s: %s\n", firstNonEmpty(refused.EvidenceFile, refused.Owner), refused.Reference, shortLocation(refused.Pinned), refused.Reason)
	}
	for _, slide := range result.Slides {
		fmt.Fprintf(out, "  slide %s: snapshot %s\n", slide.Target, slide.Snapshot)
	}
	return nil
}

func anyIncluded(scope acceptScope, refs []ownedReference, review string) bool {
	for _, ref := range refs {
		if scope.includes(ref, review) {
			return true
		}
	}
	return false
}

// managedEvidence parses a complete-slide evidence_file: the slide record,
// the Item, and the 0-based evidence index.
func managedEvidence(file string) (string, string, int, bool) {
	record, rest, ok := strings.Cut(file, "#items/")
	if !ok {
		return "", "", 0, false
	}
	item, index, ok := strings.Cut(rest, "/evidence/")
	if !ok {
		return "", "", 0, false
	}
	number, err := strconv.Atoi(index)
	if err != nil || number < 0 {
		return "", "", 0, false
	}
	return record, item, number, true
}

// writeAcceptedEvidence writes accepted references. A plain evidence file is
// rewritten in place, keeping its path (so its owner) and every other
// reference; a slide apply-slide manages gets one complete-slide update built
// from its current revision with only the accepted evidence changed.
func writeAcceptedEvidence(ctx context.Context, root, repo string, accepted []acceptChange, dryRun bool) ([]SlideTransactionResult, error) {
	plain := map[string][]acceptChange{}
	managed := map[string][]acceptChange{}
	for _, change := range accepted {
		if record, _, _, ok := managedEvidence(change.EvidenceFile); ok {
			managed[record] = append(managed[record], change)
		} else {
			plain[change.EvidenceFile] = append(plain[change.EvidenceFile], change)
		}
	}
	if !dryRun && len(plain) > 0 {
		err := authorMutation(root, func(locked *saga.Saga) error {
			for _, relative := range sortedKeys(plain) {
				path := filepath.Join(locked.Root, filepath.FromSlash(relative))
				var file saga.CodeFile
				if err := readStrictJSONFile(path, &file); err != nil {
					return fmt.Errorf("read %s: %w", relative, err)
				}
				for _, change := range plain[relative] {
					if change.Reference < 1 || change.Reference > len(file.References) || file.References[change.Reference-1].Key() != change.previous.Key() {
						return fmt.Errorf("%s changed while accepting proposals; run the command again", relative)
					}
					file.References[change.Reference-1] = change.replacement
				}
				if err := store.WriteJSON(path, file, false); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	results := []SlideTransactionResult{}
	for _, record := range sortedKeys(managed) {
		document, _, err := saga.Load(root)
		if err != nil {
			return nil, err
		}
		slide := findSlide(document, record)
		if slide == nil {
			return nil, fmt.Errorf("no slide is recorded at %s", record)
		}
		request, err := currentSlideRequest(document, slide)
		if err != nil {
			return nil, err
		}
		fingerprint := sha256.New()
		fmt.Fprint(fingerprint, request.ExpectedSnapshot)
		for _, change := range managed[record] {
			_, itemID, evidenceIndex, _ := managedEvidence(change.EvidenceFile)
			replaced := false
			for itemIndex := range request.Items {
				item := &request.Items[itemIndex]
				if item.ID != itemID || evidenceIndex >= len(item.Evidence) {
					continue
				}
				references := item.Evidence[evidenceIndex].References
				if change.Reference >= 1 && change.Reference <= len(references) && references[change.Reference-1].Key() == change.previous.Key() {
					references[change.Reference-1] = change.replacement
					replaced = true
				}
			}
			if !replaced {
				return nil, fmt.Errorf("%s changed while accepting proposals; run the command again", change.EvidenceFile)
			}
			fmt.Fprint(fingerprint, change.EvidenceFile, change.Reference, change.replacement.Key())
		}
		request.RequestID = "accept-proposed-" + hex.EncodeToString(fingerprint.Sum(nil))[:16]
		base, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		result, err := ApplySlideTransaction(ctx, root, base, repo, request, dryRun)
		if err != nil {
			return nil, fmt.Errorf("update slide %s: %w", slide.Target, err)
		}
		results = append(results, result)
	}
	return results, nil
}

// currentSlideRequest rebuilds the complete apply-slide request that
// republishes a managed slide's current revision unchanged: its diagram
// source, or its asset inline as base64, every Item with its evidence,
// criterion links, and documentation links. It is an update expecting the
// current snapshot; the caller supplies a fresh request_id.
func currentSlideRequest(document *saga.Saga, slide *saga.Slide) (SlideTransactionRequest, error) {
	if !transactionManagedSlide(slide) {
		return SlideTransactionRequest{}, fmt.Errorf("slide %s is not managed by apply-slide", slide.Target)
	}
	if slide.AuthoringConflict || len(slide.AuthoringHeads) > 1 {
		return SlideTransactionRequest{}, fmt.Errorf("slide %s has divergent heads %s; reconcile them with apply-slide first", slide.Target, strings.Join(slide.AuthoringHeads, ", "))
	}
	var deck *saga.Deck
	for _, candidate := range allDecks(document) {
		for _, owned := range candidate.Slides {
			if owned == slide {
				deck = candidate
			}
		}
	}
	for _, review := range document.Reviews {
		if review.Deck != nil {
			for _, owned := range review.Deck.Slides {
				if owned == slide {
					deck = review.Deck
				}
			}
		}
	}
	if deck == nil {
		return SlideTransactionRequest{}, fmt.Errorf("slide %s belongs to no deck", slide.Target)
	}
	var record saga.SlideTransactionRecord
	if err := readStrictJSONPath(filepath.Join(document.Root, filepath.FromSlash(slide.Path)), &record); err != nil {
		return SlideTransactionRequest{}, err
	}
	revision, err := record.CurrentRevision()
	if err != nil {
		return SlideTransactionRequest{}, err
	}
	value := revision.Slide
	request := SlideTransactionRequest{
		Version: saga.SlideTransactionVersion, Operation: "update", Deck: deck.ID, ExpectedSnapshot: revision.Snapshot,
		Slide: SlideTransactionSlide{
			ID: value.ID, Title: value.Title, Rank: value.Rank, Section: value.Section, Intent: value.Intent, Layout: value.Layout,
			MediaType: value.MediaType, Takeaway: value.Takeaway, ReadingOrder: append([]string{}, value.ReadingOrder...), ExceptionRationale: value.ExceptionRationale,
		},
		Items: []SlideTransactionItemRequest{},
	}
	if revision.Diagram != nil {
		data, err := os.ReadFile(filepath.Join(deck.Directory, revision.Diagram.Source))
		if err != nil {
			return SlideTransactionRequest{}, err
		}
		source, err := diagram.Decode(data)
		if err != nil {
			return SlideTransactionRequest{}, fmt.Errorf("diagram source %s: %w", revision.Diagram.Source, err)
		}
		request.Diagram = &source
	} else {
		data, err := os.ReadFile(filepath.Join(deck.Directory, revision.Asset))
		if err != nil {
			return SlideTransactionRequest{}, err
		}
		request.Asset = SlideTransactionAsset{ContentBase64: base64.StdEncoding.EncodeToString(data)}
	}
	for _, item := range revision.Items {
		manifest := item.Item
		evidence := make([]saga.CodeFile, 0, len(item.Evidence))
		for _, file := range item.Evidence {
			evidence = append(evidence, saga.CodeFile{Version: file.Version, References: append([]coderef.Reference{}, file.References...)})
		}
		request.Items = append(request.Items, SlideTransactionItemRequest{
			Documentation: manifest.Documentation, DocumentationView: manifest.DocumentationView, Selections: append([]saga.ItemSelection(nil), manifest.Selections...),
			ID: manifest.ID, Rank: manifest.Rank, Kind: manifest.Kind, Label: manifest.Label, Description: manifest.Description,
			Selector: manifest.Selector, Hotspot: manifest.Hotspot, About: manifest.About, Body: manifest.Body, Placement: manifest.Placement, Leader: manifest.Leader,
			Evidence: evidence, CriterionLinks: append([]saga.CriterionLink{}, item.CriterionLinks...),
		})
	}
	return request, nil
}

// livingStaleness measures what base..head did to the living documentation's
// references, or nil when the Saga has none.
func livingStaleness(ctx context.Context, document *saga.Saga, resolver *coderesolve.Resolver, base, head, root, repo string) *changeStaleness {
	owned, err := sagaReferences(document)
	if err != nil || len(owned) == 0 || resolver == nil {
		return nil
	}
	staleness := measureChangeStaleness(ctx, resolver, owned, historicalOwners(document), base, head, root, repo)
	return &staleness
}

// reviewItemRows lists a review's stale Item references with proposals at
// the side each lives on: deleted-side evidence at the merge-base, the rest
// at the head.
func reviewItemRows(ctx context.Context, resolver *coderesolve.Resolver, review *saga.Review, base, head, root, repo string) []staleRow {
	rows := []staleRow{}
	for _, ref := range reviewReferences(review) {
		atHead := resolver.Resolve(ctx, ref.Code, head)
		if atHead.Current() || resolver.Resolve(ctx, ref.Code, base).Current() {
			continue
		}
		view := head
		if ref.Code.Commit == base || isAncestor(ctx, resolver.Repository(), ref.Code.Commit, base) {
			view = base
		}
		row := staleRow{ownedReference: ref, Pinned: ref.Code.Location(), Reason: atHead.Reason, Proposal: resolver.Propose(ctx, ref.Code, view)}
		if row.Proposal.Proposed() {
			row.Accept = acceptInvocation(ref, "", root, repo)
		}
		rows = append(rows, row)
	}
	return rows
}
