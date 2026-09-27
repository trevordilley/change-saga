package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// refreshMove is a reference refresh-coverage re-pinned without judgment:
// its lines only moved, or it is a whole-file reference whose file event
// (add, rename, mode change, delete) is still part of the review's range.
type refreshMove struct {
	Item         string           `json:"item"`
	EvidenceFile string           `json:"evidence_file"`
	Reference    int              `json:"reference"`
	From         coderef.Location `json:"from"`
	To           coderef.Location `json:"to"`
	Why          string           `json:"why"`
}

// refreshAddition is newly changed code given to the one Item that already
// covers its file, as re-running cover --changed-lines for it would.
type refreshAddition struct {
	Item         string           `json:"item"`
	EvidenceFile string           `json:"evidence_file"`
	Location     coderef.Location `json:"location"`
}

// refreshGap is newly changed code refresh-coverage left for an author:
// several Items cover its file, or none does.
type refreshGap struct {
	Path       string   `json:"path"`
	Locations  []string `json:"locations"`
	Candidates []string `json:"candidates"`
	Reason     string   `json:"reason"`
}

type refreshOutput struct {
	OK      bool   `json:"ok"`
	DryRun  bool   `json:"dry_run"`
	Review  string `json:"review"`
	BaseOID string `json:"base_oid"`
	HeadOID string `json:"head_oid"`
	// Moved are references re-pinned because their lines only moved or their
	// file event persists.
	Moved []refreshMove `json:"moved"`
	// Added are newly changed lines and file events given to the one Item
	// covering their file.
	Added []refreshAddition `json:"added"`
	// NeedsJudgment are stale references: an edit landed inside them. Each
	// carries its proposal and, when there is one, the accept command. They
	// are never accepted here.
	NeedsJudgment []staleRow `json:"needs_judgment"`
	// Uncovered is newly changed code no single Item's files decide.
	Uncovered []refreshGap           `json:"uncovered"`
	Slides    []SlideTransactionResult `json:"slide_transactions"`
}

// reviewRefreshCoverage re-runs a review's coverage over its current range.
func reviewRefreshCoverage(ctx context.Context, args []string, out io.Writer) error {
	name := "review refresh-coverage"
	flags := commandFlags(name, commandUsage[name], out)
	reviewID := flags.String("review", "", "review id")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	dryRun := flags.Bool("dry-run", false, "report what would change without writing")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *reviewID); err != nil {
		return err
	}
	root := flags.Arg(0)
	document, _, err := saga.Load(root)
	if err != nil {
		return err
	}
	review := document.FindReview(*reviewID)
	if review == nil {
		return fmt.Errorf("review %q does not exist%s", *reviewID, knownReviews(document))
	}
	if review.Merged != nil {
		return fmt.Errorf("review %q is history: its evidence cannot be refreshed after landing", review.ID)
	}
	checkout := firstNonEmpty(*repo, document.Root)
	rng, err := reviewstate.ResolveRange(ctx, checkout, review)
	if err != nil {
		return err
	}
	changes, err := gitdiff.ReadWithOptions(ctx, checkout, document.Manifest.Source.Repository, rng.BaseOID, rng.HeadOID, gitdiff.ReadOptions{AllowRepositoryMismatch: true})
	if err != nil {
		return err
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return err
	}
	defer resolver.Close()
	result, edits := planRefresh(ctx, resolver, review, changes, root, *repo)
	result.DryRun = *dryRun
	if len(edits) > 0 {
		if result.Slides, err = writeEvidenceEdits(ctx, root, *repo, edits, "refresh-coverage", *dryRun); err != nil {
			return err
		}
	}
	if *jsonOutput {
		return writeJSON(out, result)
	}
	printRefresh(out, result, root)
	return nil
}

// planRefresh decides every mechanical edit and reports the rest.
func planRefresh(ctx context.Context, resolver *coderesolve.Resolver, review *saga.Review, changes gitdiff.ChangeSet, root, repo string) (refreshOutput, []evidenceEdit) {
	result := refreshOutput{OK: true, Review: review.ID, BaseOID: changes.BaseOID, HeadOID: changes.HeadOID,
		Moved: []refreshMove{}, Added: []refreshAddition{}, NeedsJudgment: []staleRow{}, Uncovered: []refreshGap{}, Slides: []SlideTransactionResult{}}
	var edits []evidenceEdit
	// Where each Item's evidence now accounts for code, and which evidence
	// file of each Item holds references into each path.
	var effective []coderef.Location
	holders := map[string]map[string]string{}
	hold := func(path, item, file string) {
		if holders[path] == nil {
			holders[path] = map[string]string{}
		}
		if _, ok := holders[path][item]; !ok {
			holders[path][item] = file
		}
	}
	notes := map[string]string{}
	events := map[string]coderef.Location{}
	for _, atom := range changes.Atoms {
		if atom.Kind == "event" {
			location := changes.Location(atom)
			for _, path := range []string{atom.Path, atom.OldPath, atom.NewPath} {
				if path != "" {
					events[path] = location
				}
			}
		}
	}
	for _, ref := range reviewReferences(review) {
		file := ref.EvidenceFile
		atHead := resolver.Resolve(ctx, ref.Code, changes.HeadOID)
		atBase := resolver.Resolve(ctx, ref.Code, changes.BaseOID)
		if _, ok := notes[file+"\x00"+ref.Code.Path]; !ok {
			notes[file+"\x00"+ref.Code.Path] = ref.Code.Note
		}
		move := func(location coderef.Location, why string) {
			replacement, err := resolver.Author(ctx, location, ref.Code.Note)
			if err != nil {
				return
			}
			edits = append(edits, evidenceEdit{EvidenceFile: file, Reference: ref.Index, previous: ref.Code, replacement: replacement})
			result.Moved = append(result.Moved, refreshMove{Item: ref.Owner, EvidenceFile: file, Reference: ref.Index, From: ref.Code.Location(), To: location, Why: why})
		}
		switch {
		case atHead.Current() || atBase.Current():
			for _, resolution := range []coderesolve.Resolution{atHead, atBase} {
				if resolution.Current() {
					effective = append(effective, resolution.Location)
					hold(resolution.Location.Path, ref.Owner, file)
				}
			}
			if atHead.Current() && atHead.Moved {
				move(atHead.Location, "its lines moved")
			} else if !atHead.Current() && atBase.Moved && ref.Code.Commit != changes.BaseOID {
				move(atBase.Location, "its deleted-side lines moved with the merge-base")
			}
		case ref.Code.WholeFile():
			hold(ref.Code.Path, ref.Owner, file)
			if location, ok := events[ref.Code.Path]; ok {
				effective = append(effective, location)
				hold(location.Path, ref.Owner, file)
				move(location, "its file event is still in the range")
				continue
			}
			result.NeedsJudgment = append(result.NeedsJudgment, staleRow{ownedReference: ref, Pinned: ref.Code.Location(), Reason: atHead.Reason,
				Proposal: coderesolve.Proposal{Reason: "the file no longer has an add, rename, mode, or delete event in the range; remove or re-cover it"}})
		default:
			hold(ref.Code.Path, ref.Owner, file)
			view := changes.HeadOID
			if ref.Code.Commit == changes.BaseOID || isAncestor(ctx, resolver.Repository(), ref.Code.Commit, changes.BaseOID) {
				view = changes.BaseOID
			}
			row := staleRow{ownedReference: ref, Pinned: ref.Code.Location(), Reason: atHead.Reason, Proposal: resolver.Propose(ctx, ref.Code, view)}
			if row.Proposal.Proposed() {
				// A proposal reserves its lines: they wait for the author's
				// judgment rather than being handed out again as new.
				effective = append(effective, *row.Proposal.Location)
				hold(row.Proposal.Location.Path, ref.Owner, file)
				row.Accept = acceptInvocation(ref, "", root, repo)
			}
			result.NeedsJudgment = append(result.NeedsJudgment, row)
		}
	}
	covered := func(location coderef.Location) bool {
		for _, candidate := range effective {
			// A whole-file reference accounts for its file's event, never its
			// lines.
			if candidate.WholeFile() == location.WholeFile() && candidate.Contains(location) {
				return true
			}
		}
		return false
	}
	// Newly changed code: coalesce uncovered lines per side and file, then
	// give each run to the one Item holding that file.
	type run struct{ commit, path string }
	lines := map[run][]int{}
	var order []run
	var wholeFiles []coderef.Location
	for _, atom := range changes.Atoms {
		location := changes.Location(atom)
		if covered(location) {
			continue
		}
		if location.WholeFile() {
			wholeFiles = append(wholeFiles, location)
			continue
		}
		key := run{location.Commit, location.Path}
		if _, ok := lines[key]; !ok {
			order = append(order, key)
		}
		lines[key] = append(lines[key], location.Start)
	}
	gaps := map[string]*refreshGap{}
	var gapOrder []string
	assign := func(location coderef.Location) {
		items := holders[location.Path]
		if len(items) == 1 {
			for item, file := range items {
				_, _, _, managed := managedEvidence(file)
				note := notes[file+"\x00"+location.Path]
				if !managed || !location.WholeFile() && note != "" {
					if reference, err := resolver.Author(ctx, location, note); err == nil {
						edits = append(edits, evidenceEdit{EvidenceFile: file, replacement: reference})
						result.Added = append(result.Added, refreshAddition{Item: item, EvidenceFile: file, Location: location})
						return
					}
				}
			}
		}
		gap := gaps[location.Path]
		if gap == nil {
			candidates := []string{}
			for item := range items {
				candidates = append(candidates, item)
			}
			sort.Strings(candidates)
			reason := "no Item covers this file yet"
			switch {
			case len(candidates) > 1:
				reason = "several Items cover this file; choose the one that explains it"
			case len(candidates) == 1:
				reason = "the Item that covers this file cannot take it mechanically (a managed slide needs a note and exact lines)"
			}
			gap = &refreshGap{Path: location.Path, Locations: []string{}, Candidates: candidates, Reason: reason}
			gaps[location.Path] = gap
			gapOrder = append(gapOrder, location.Path)
		}
		gap.Locations = append(gap.Locations, location.String())
	}
	for _, location := range wholeFiles {
		assign(location)
	}
	for _, key := range order {
		values := lines[key]
		sort.Ints(values)
		for index := 0; index < len(values); {
			end := index
			for end+1 < len(values) && values[end+1] <= values[end]+1 {
				end++
			}
			assign(coderef.Location{Commit: key.commit, Path: key.path, Start: values[index], End: values[end]})
			index = end + 1
		}
	}
	for _, path := range gapOrder {
		result.Uncovered = append(result.Uncovered, *gaps[path])
	}
	return result, edits
}

func printRefresh(out io.Writer, result refreshOutput, root string) {
	verb := map[bool]string{true: "Would refresh", false: "Refreshed"}[result.DryRun]
	fmt.Fprintf(out, "%s review %s's coverage over %s..%s\n", verb, result.Review, shortOID(result.BaseOID), shortOID(result.HeadOID))
	fmt.Fprintf(out, "Moved %d %s (no judgment needed):\n", len(result.Moved), plural(len(result.Moved), "reference", "references"))
	for _, move := range result.Moved {
		fmt.Fprintf(out, "  %s #%d %s -> %s: %s\n", move.EvidenceFile, move.Reference, shortLocation(move.From), shortLocation(move.To), move.Why)
	}
	fmt.Fprintf(out, "Added %d newly changed %s to the Item already covering the file:\n", len(result.Added), plural(len(result.Added), "range", "ranges"))
	for _, added := range result.Added {
		fmt.Fprintf(out, "  %s  %s\n", added.Item, shortLocation(added.Location))
	}
	fmt.Fprintf(out, "Needs judgment: %d stale %s (an edit landed inside; read the diff):\n", len(result.NeedsJudgment), plural(len(result.NeedsJudgment), "reference", "references"))
	for _, row := range result.NeedsJudgment {
		fmt.Fprintf(out, "  %s #%d %s\n", row.EvidenceFile, row.Index, describeProposal(row.Pinned, row.Proposal))
		for _, line := range row.Proposal.Diff {
			fmt.Fprintf(out, "      %s\n", line)
		}
		if row.Accept != nil {
			fmt.Fprintf(out, "    accept if the explanation still holds: %s\n", shellJoin(row.Accept.Argv))
		}
	}
	fmt.Fprintf(out, "Uncovered: %d %s left for you:\n", len(result.Uncovered), plural(len(result.Uncovered), "file", "files"))
	for _, gap := range result.Uncovered {
		fmt.Fprintf(out, "  %s: %s (%s)\n", gap.Path, strings.Join(gap.Locations, " "), gap.Reason)
		for _, candidate := range gap.Candidates {
			fmt.Fprintf(out, "    candidate %s\n", candidate)
		}
		cover := grammar.MustInvoke("cover", root, grammar.V("target", ""), grammar.V("path", gap.Path), grammar.V("changed-lines", "true"))
		fmt.Fprintf(out, "    cover: %s\n", shellJoin(cover.Argv))
	}
	for _, slide := range result.Slides {
		fmt.Fprintf(out, "  slide %s: snapshot %s\n", slide.Target, slide.Snapshot)
	}
}
