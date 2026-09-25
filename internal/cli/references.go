package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/reviewstore"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// ownedReference is one code reference the Saga records, with its owner.
// Kind is evidence (a coverage record), claim, quality_evidence, or term. A
// term's references name the code that defines a word of the project's
// vocabulary; when one goes stale, its owner is the term to update.
type ownedReference struct {
	Kind         string            `json:"kind"`
	Owner        string            `json:"owner"`
	EvidenceFile string            `json:"evidence_file,omitempty"`
	Index        int               `json:"reference"`
	Code         coderef.Reference `json:"code"`
}

// sagaReferences lists every code reference in evidence records, claims,
// quality evidence, and current term revisions, in a stable order.
func sagaReferences(document *saga.Saga) ([]ownedReference, error) {
	var result []ownedReference
	coverage.WalkDocumentCode(document, func(target string, files []saga.CodeFile) {
		for _, file := range files {
			for index, reference := range file.References {
				result = append(result, ownedReference{Kind: "evidence", Owner: target, EvidenceFile: filepath.ToSlash(file.Path), Index: index + 1, Code: reference})
			}
		}
	})
	for _, claim := range document.Claims {
		for index, reference := range claim.Evidence {
			result = append(result, ownedReference{Kind: "claim", Owner: "urn:change-saga:" + document.Manifest.ID + ":claim:" + claim.ID, Index: index + 1, Code: reference})
		}
	}
	qualityDocument, err := quality.Load(document.Root)
	if err != nil {
		return nil, fmt.Errorf("load quality: %w", err)
	}
	for _, testCase := range qualityDocument.TestCases {
		for _, evidence := range testCase.Evidence {
			urn, _ := qualityid.Evidence(document.Manifest.ID, testCase.Identity.ID, evidence.ID)
			for index, reference := range evidence.Code {
				result = append(result, ownedReference{Kind: "quality_evidence", Owner: urn, Index: index + 1, Code: reference})
			}
		}
	}
	vocabulary, err := requirements.Load(document.Root, document.Manifest.ID)
	if err != nil {
		return nil, fmt.Errorf("load terms: %w", err)
	}
	for _, term := range vocabulary.Terms {
		if term.CurrentRevision == nil {
			continue
		}
		urn, _ := requirements.TermURN(document.Manifest.ID, term.Identity.ID)
		for index, reference := range term.CurrentRevision.Code {
			result = append(result, ownedReference{Kind: "term", Owner: urn, Index: index + 1, Code: reference})
		}
	}
	return result, nil
}

type referenceHealth struct {
	ownedReference
	State  coderesolve.State `json:"state"`
	Moved  bool              `json:"moved"`
	Head   *coderef.Location `json:"head,omitempty"`
	Base   *coderef.Location `json:"base,omitempty"`
	Reason string            `json:"reason,omitempty"`
	Diff   string            `json:"diff_since_pin,omitempty"`
	Pinned coderef.Location  `json:"pinned"`
}

type referencesOutput struct {
	OK         bool              `json:"ok"`
	Base       string            `json:"base_oid"`
	Head       string            `json:"head_oid"`
	Total      int               `json:"total"`
	Current    int               `json:"current"`
	Remapped   int               `json:"remapped"`
	Stale      int               `json:"stale"`
	References []referenceHealth `json:"references"`
}

// References reports every code reference viewed in the Saga's comparison:
// current where its code is unchanged (remapped when its lines only moved),
// stale with a reason where the code changed, and, with --diff, the patch of
// the referenced file since the pin.
func References(ctx context.Context, args []string, out io.Writer) error {
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
	flags := commandFlags("references", commandUsage["references"], out)
	repoDir := flags.String("repo", "", "source repository checkout; required when separate")
	opening := registerOpenFlags(flags)
	staleOnly := flags.Bool("stale", false, "list only stale references")
	withDiff := flags.Bool("diff", false, "include each stale reference's diff since its pin")
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	allowMismatch := flags.Bool("allow-repository-mismatch", false, "use a checkout whose origin differs from the declared repository")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage["references"])
	}
	document, _, err := saga.Load(flags.Arg(0))
	if err != nil {
		return err
	}
	checkout := firstNonEmpty(*repoDir, document.Root)
	changes, err := gitdiff.ReadRange(ctx, checkout, document.Manifest.Source.Repository, opening.rng(), gitdiff.ReadOptions{AllowRepositoryMismatch: *allowMismatch})
	if err != nil {
		return fmt.Errorf("read source comparison (use --repo for a separate saga repository): %w", err)
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return err
	}
	defer resolver.Close()
	owned, err := sagaReferences(document)
	if err != nil {
		return err
	}
	result := referencesOutput{OK: true, Base: changes.BaseOID, Head: changes.HeadOID, References: []referenceHealth{}}
	for _, value := range owned {
		health := referenceHealth{ownedReference: value, Pinned: value.Code.Location(), State: coderesolve.Stale}
		head := resolver.Resolve(ctx, value.Code, changes.HeadOID)
		base := resolver.Resolve(ctx, value.Code, changes.BaseOID)
		if head.Current() {
			location := head.Location
			health.Head, health.State, health.Moved = &location, coderesolve.Current, head.Moved
		}
		// A term names the code as it is now, so only the head decides whether
		// it is current: a rename in the change makes it stale.
		if base.Current() && value.Kind != "term" {
			location := base.Location
			health.Base, health.State = &location, coderesolve.Current
			health.Moved = health.Moved || base.Moved && health.Head == nil
		}
		if health.State == coderesolve.Stale {
			health.Reason = head.Reason
			if *withDiff {
				health.Diff, _ = resolver.DiffSince(ctx, value.Code, changes.HeadOID)
			}
		}
		result.Total++
		switch {
		case health.State == coderesolve.Stale:
			result.Stale++
		case health.Moved:
			result.Remapped++
			result.Current++
		default:
			result.Current++
		}
		if *staleOnly && health.State != coderesolve.Stale {
			continue
		}
		result.References = append(result.References, health)
	}
	if *jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "%d references: %d current (%d remapped), %d stale\n", result.Total, result.Current, result.Remapped, result.Stale)
	for _, health := range result.References {
		owner := health.Owner
		if health.EvidenceFile != "" {
			owner = health.EvidenceFile
		}
		fmt.Fprintf(out, "  %-7s %s #%d %s", health.State, owner, health.Index, health.Pinned)
		if health.Head != nil && health.Moved {
			fmt.Fprintf(out, " -> %s", health.Head)
		}
		if health.Reason != "" {
			fmt.Fprintf(out, "\n          %s", health.Reason)
		}
		fmt.Fprintln(out)
		if health.Diff != "" {
			for _, line := range strings.Split(strings.TrimRight(health.Diff, "\n"), "\n") {
				fmt.Fprintf(out, "          %s\n", line)
			}
		}
	}
	return nil
}

type repinChange struct {
	EvidenceFile string           `json:"evidence_file"`
	Reference    int              `json:"reference"`
	From         coderef.Location `json:"from"`
	To           coderef.Location `json:"to"`
	ByDigest     bool             `json:"by_digest,omitempty"`
}

type repinSkip struct {
	Kind         string           `json:"kind"`
	Owner        string           `json:"owner"`
	EvidenceFile string           `json:"evidence_file,omitempty"`
	Reference    int              `json:"reference"`
	Pinned       coderef.Location `json:"pinned"`
	Reason       string           `json:"reason"`
}

type repinOutput struct {
	OK          bool                `json:"ok"`
	DryRun      bool                `json:"dry_run"`
	Onto        string              `json:"onto"`
	Repinned    []repinChange       `json:"repinned"`
	Unchanged   int                 `json:"unchanged"`
	Left        []repinSkip         `json:"left_pinned"`
	Base        string              `json:"base,omitempty"`
	Commits     []saga.MergedCommit `json:"commits"`
	MergeRecord string              `json:"merge_record,omitempty"`
	// Cursor is the code commit a companion Saga's sync cursor moved to.
	Cursor string `json:"cursor,omitempty"`
	// Review is the pull request review frozen at the landed change's exact
	// base and head, so it stays viewable after the branch is gone.
	Review *repinReview `json:"review,omitempty"`
	// OpenReviews are reviews still following a head, left unfrozen because
	// the landed change's review could not be decided; pass --review.
	OpenReviews []string `json:"open_reviews,omitempty"`
}

type repinReview struct {
	ID string `json:"id"`
	saga.ReviewMerge
}

// Repin moves every evidence reference to the commit a change landed as, so
// squash merges and deleted branches lose nothing: each reference is remapped
// from its pin to that commit, falling back to its content digest when its
// pinned commit is gone. It also records the branch's commit messages with the
// change. Claims, quality evidence, and review records are immutable and are
// not rewritten; they keep resolving through the same remap and digest
// fallback.
func Repin(ctx context.Context, args []string, out io.Writer) error {
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
	flags := commandFlags("repin", commandUsage["repin"], out)
	onto := flags.String("onto", "", "the commit the change landed as on its target branch")
	branch := flags.String("branch", "", "the branch's last commit, when it is still available; widens the recorded commit messages")
	reviewID := flags.String("review", "", "the pull request review of the landed change to freeze; defaults to the Saga's only open review")
	repoDir := flags.String("repo", "", "source repository checkout; required when separate")
	dryRun := flags.Bool("dry-run", false, "report what would be re-pinned without writing")
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || strings.TrimSpace(*onto) == "" {
		return fmt.Errorf("usage: %s", commandUsage["repin"])
	}
	root := flags.Arg(0)
	document, _, err := saga.Load(root)
	if err != nil {
		return err
	}
	checkout := firstNonEmpty(*repoDir, document.Root)
	ontoCommit, err := resolveCommit(ctx, checkout, *onto)
	if err != nil {
		return err
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return err
	}
	defer resolver.Close()

	result := repinOutput{OK: true, DryRun: *dryRun, Onto: ontoCommit, Repinned: []repinChange{}, Left: []repinSkip{}, Commits: []saga.MergedCommit{}}
	owned, err := sagaReferences(document)
	if err != nil {
		return err
	}
	pins := map[string]bool{}
	updates := map[string]map[int]coderef.Reference{}
	for _, value := range owned {
		exists, _ := resolver.CommitExists(ctx, value.Code.Commit)
		if exists {
			pins[value.Code.Commit] = true
		}
		if value.Code.Commit == ontoCommit {
			result.Unchanged++
			continue
		}
		// A pin that is gone resolves through its content digest.
		resolution := resolver.Resolve(ctx, value.Code, ontoCommit)
		byDigest := !exists && resolution.Current()
		if !resolution.Current() {
			if found, ok := resolver.Find(ctx, value.Code, ontoCommit); ok {
				resolution, byDigest = found, true
			}
		}
		if !resolution.Current() {
			reason := resolution.Reason
			if value.Kind == "evidence" {
				reason += "; left pinned where it was written"
			}
			result.Left = append(result.Left, repinSkip{Kind: value.Kind, Owner: value.Owner, EvidenceFile: value.EvidenceFile, Reference: value.Index, Pinned: value.Code.Location(), Reason: reason})
			continue
		}
		if value.Kind != "evidence" {
			// Immutable records keep their pins; the remap and digest fallback
			// already resolve them at the landed commit.
			result.Unchanged++
			continue
		}
		repinned := value.Code
		repinned.Commit, repinned.Path, repinned.Start, repinned.End = ontoCommit, resolution.Location.Path, resolution.Location.Start, resolution.Location.End
		if updates[value.EvidenceFile] == nil {
			updates[value.EvidenceFile] = map[int]coderef.Reference{}
		}
		updates[value.EvidenceFile][value.Index] = repinned
		result.Repinned = append(result.Repinned, repinChange{EvidenceFile: value.EvidenceFile, Reference: value.Index, From: value.Code.Location(), To: repinned.Location(), ByDigest: byDigest})
	}
	if *branch != "" {
		branchCommit, err := resolveCommit(ctx, checkout, *branch)
		if err != nil {
			return err
		}
		pins[branchCommit] = true
	}
	// The landed change's review is frozen at its exact head, and that head
	// is the branch whose commit messages the merge record keeps.
	frozen, err := reviewToFreeze(ctx, document, checkout, ontoCommit, *branch, *reviewID, &result)
	if err != nil {
		return err
	}
	if frozen != nil {
		pins[frozen.Head] = true
	}
	delete(pins, ontoCommit)
	result.Commits, err = branchCommits(ctx, checkout, ontoCommit, pins)
	if err != nil {
		return err
	}
	forkedFrom := *branch
	if forkedFrom == "" && frozen != nil {
		forkedFrom = frozen.Head
	}
	result.Base = landedBase(ctx, checkout, ontoCommit, forkedFrom)
	companion := companionCheckout(ctx, root, checkout)

	if companion {
		result.Cursor = ontoCommit
	}
	if !*dryRun && (len(updates) > 0 || len(result.Commits) > 0 || companion || frozen != nil) {
		err = authorMutation(root, func(locked *saga.Saga) error {
			for relative, changed := range updates {
				path := filepath.Join(locked.Root, filepath.FromSlash(relative))
				var file saga.CodeFile
				if err := readStrictJSONFile(path, &file); err != nil {
					return fmt.Errorf("read %s: %w", relative, err)
				}
				for index, reference := range changed {
					if index < 1 || index > len(file.References) || file.References[index-1].Location().String() != referenceKeyBefore(result.Repinned, relative, index) {
						return fmt.Errorf("%s changed while re-pinning; run repin again", relative)
					}
					file.References[index-1] = reference
				}
				if err := store.WriteJSON(path, file, false); err != nil {
					return err
				}
			}
			if companion {
				// A companion Saga now documents the landed commit.
				if err := store.WriteJSON(filepath.Join(locked.Root, saga.CursorName), saga.Cursor{Schema: saga.CursorSchemaURL, Version: saga.CursorVersion, Commit: ontoCommit}, false); err != nil {
					return err
				}
			}
			if frozen != nil {
				review := locked.FindReview(frozen.ID)
				if review == nil || review.Merged != nil {
					return fmt.Errorf("review %s changed while re-pinning; run repin again", frozen.ID)
				}
				if err := reviewstore.WriteFrozen(review, frozen.ReviewMerge); err != nil {
					return err
				}
			}
			if len(result.Commits) == 0 {
				return nil
			}
			dir, err := store.EnsureDirWithin(locked.Root, filepath.Join(locked.Root, saga.MergesDir))
			if err != nil {
				return err
			}
			path := filepath.Join(dir, saga.MergeFilename(ontoCommit))
			record := saga.Merge{Version: saga.CurrentVersion, Commit: ontoCommit, Base: result.Base, Commits: result.Commits, PinnedAt: time.Now().UTC()}
			if frozen != nil {
				record.Review = frozen.ID
			}
			if err := store.WriteJSON(path, record, false); err != nil {
				return err
			}
			relative, _ := filepath.Rel(locked.Root, path)
			result.MergeRecord = filepath.ToSlash(relative)
			return nil
		})
		if err != nil {
			return err
		}
	}
	if *jsonOutput {
		return writeJSON(out, result)
	}
	verb := "Re-pinned"
	if *dryRun {
		verb = "Would re-pin"
	}
	fmt.Fprintf(out, "%s %d references to %s (%d already there)\n", verb, len(result.Repinned), ontoCommit, result.Unchanged)
	for _, skipped := range result.Left {
		fmt.Fprintf(out, "  left %s %s #%d at %s: %s\n", skipped.Kind, firstNonEmpty(skipped.EvidenceFile, skipped.Owner), skipped.Reference, skipped.Pinned, skipped.Reason)
	}
	if result.Cursor != "" {
		fmt.Fprintf(out, "Moved the sync cursor to %s\n", result.Cursor)
	}
	if result.Review != nil {
		verb := "Froze"
		if *dryRun {
			verb = "Would freeze"
		}
		fmt.Fprintf(out, "%s review %s at %s..%s (landed as %s)\n", verb, result.Review.ID, shortOID(result.Review.Base), shortOID(result.Review.Head), shortOID(result.Review.Landed))
	}
	if len(result.OpenReviews) > 0 {
		fmt.Fprintf(out, "Left reviews %s following their heads; pass --review to freeze the landed change's review\n", strings.Join(result.OpenReviews, ", "))
	}
	if len(result.Commits) > 0 {
		fmt.Fprintf(out, "Recorded %d branch commit messages", len(result.Commits))
		if result.MergeRecord != "" {
			fmt.Fprintf(out, " in %s", result.MergeRecord)
		}
		fmt.Fprintln(out)
	}
	return nil
}

// reviewToFreeze decides which review a landing freezes and at which
// commits: --review, or the Saga's only open review. Its head is --branch, or
// else the ref it follows; its base is where that head forked from the
// target branch as it was before the change landed.
func reviewToFreeze(ctx context.Context, document *saga.Saga, checkout, onto, branch, reviewID string, result *repinOutput) (*repinReview, error) {
	var review *saga.Review
	if reviewID != "" {
		if review = document.FindReview(reviewID); review == nil {
			return nil, fmt.Errorf("review %q does not exist%s", reviewID, knownReviews(document))
		}
		if review.Merged != nil {
			return nil, fmt.Errorf("review %q is already frozen: it landed as %s", reviewID, shortOID(review.Merged.Landed))
		}
	} else {
		var open []*saga.Review
		for _, candidate := range document.Reviews {
			if candidate.Merged == nil {
				open = append(open, candidate)
			}
		}
		if len(open) != 1 {
			for _, candidate := range open {
				result.OpenReviews = append(result.OpenReviews, candidate.ID)
			}
			return nil, nil
		}
		review = open[0]
	}
	head := ""
	if branch != "" {
		value, err := resolveCommit(ctx, checkout, branch)
		if err != nil {
			return nil, err
		}
		head = value
	} else {
		rng, err := reviewstate.ResolveRange(ctx, checkout, review)
		if err != nil {
			return nil, fmt.Errorf("freeze review %s: %w; pass --branch with the pull request's last commit", review.ID, err)
		}
		head = rng.HeadOID
	}
	base := landedBase(ctx, checkout, onto, head)
	if base == "" {
		return nil, fmt.Errorf("freeze review %s: %s has no parent to compare from", review.ID, shortOID(onto))
	}
	frozen := &repinReview{ID: review.ID, ReviewMerge: saga.ReviewMerge{Base: base, Head: head, Landed: onto, MergedAt: time.Now().UTC()}}
	result.Review = frozen
	return frozen, nil
}

// landedBase is the commit the landed change is compared from: the target
// branch's previous tip (the landed commit's first parent), or, when the
// branch is still available, the merge-base of that tip and the branch. It is
// empty for a root commit.
func landedBase(ctx context.Context, checkout, onto, branch string) string {
	parent, err := firstParent(ctx, checkout, onto)
	if err != nil {
		return ""
	}
	base := strings.TrimSpace(string(parent))
	if branch != "" {
		if forked, err := gitexec.Output(ctx, "-C", checkout, "merge-base", base, branch); err == nil {
			base = strings.TrimSpace(string(forked))
		}
	}
	return base
}

// firstParent is onto's first parent, which a commit ID fixes forever.
func firstParent(ctx context.Context, checkout, onto string) ([]byte, error) {
	query := func() ([]byte, error) {
		return gitexec.Output(ctx, "-C", checkout, "rev-parse", "--verify", "--quiet", onto+"^1")
	}
	if gitexec.NamesObjects(onto) {
		return gitexec.Stable(ctx, checkout, []string{onto}, []string{"first-parent", onto}, query)
	}
	return query()
}

// referenceKeyBefore returns the key the planned change expects to replace,
// so a concurrent edit to the record is detected rather than overwritten.
func referenceKeyBefore(changes []repinChange, file string, index int) string {
	for _, change := range changes {
		if change.EvidenceFile == file && change.Reference == index {
			return change.From.String()
		}
	}
	return ""
}

// branchCommits returns the commits the change brought, ancestors first: every
// commit reachable from a pinned commit (or --branch) that the target branch
// did not already have before the change landed. The target's previous tip is
// the landed commit's first parent, which holds for squash, merge, and
// rebase landings alike.
func branchCommits(ctx context.Context, checkout, onto string, tips map[string]bool) ([]saga.MergedCommit, error) {
	if len(tips) == 0 {
		return []saga.MergedCommit{}, nil
	}
	args := []string{"-C", checkout, "log", "--topo-order", "--reverse", "--format=%H%x00%an <%ae>%x00%aI%x00%s%x00%b%x1e"}
	sorted := make([]string, 0, len(tips))
	for tip := range tips {
		sorted = append(sorted, tip)
	}
	sort.Strings(sorted)
	args = append(args, sorted...)
	if parent, err := firstParent(ctx, checkout, onto); err == nil {
		args = append(args, "^"+strings.TrimSpace(string(parent)))
	}
	args = append(args, "--")
	output, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("read branch commits: %w", err)
	}
	commits := []saga.MergedCommit{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if index := bytes.IndexByte(data, 0x1e); index >= 0 {
			return index + 1, data[:index], nil
		}
		if atEOF && len(bytes.TrimSpace(data)) > 0 {
			return len(data), data, nil
		}
		if atEOF {
			return len(data), nil, nil
		}
		return 0, nil, nil
	})
	for scanner.Scan() {
		fields := strings.SplitN(strings.TrimLeft(scanner.Text(), "\n"), "\x00", 5)
		if len(fields) != 5 {
			continue
		}
		date, err := time.Parse(time.RFC3339, fields[2])
		if err != nil {
			return nil, fmt.Errorf("read branch commit date %q: %w", fields[2], err)
		}
		subject := strings.TrimSpace(fields[3])
		if subject == "" {
			subject = "(no subject)"
		}
		commits = append(commits, saga.MergedCommit{Commit: fields[0], Author: fields[1], Date: date.UTC(), Subject: subject, Body: strings.TrimSpace(fields[4])})
	}
	return commits, scanner.Err()
}

func readStrictJSONFile(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}
