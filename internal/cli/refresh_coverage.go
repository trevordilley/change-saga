package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
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

// refreshAddition is newly changed code the author accepted for its proposed
// owner, the one Item that already covers its file.
type refreshAddition struct {
	Item         string           `json:"item"`
	EvidenceFile string           `json:"evidence_file"`
	Location     coderef.Location `json:"location"`
	Note         string           `json:"note"`
}

// refreshGap is newly changed code no reference covers. Coverage says someone
// linked every changed line to its explanation, so refresh never extends an
// Item on its own: when exactly one Item covers the file it is the proposed
// owner, and Accept gives it the lines once the author has checked that its
// explanation covers them.
type refreshGap struct {
	Path          string              `json:"path"`
	Locations     []string            `json:"locations"`
	Candidates    []string            `json:"candidates"`
	ProposedOwner string              `json:"proposed_owner,omitempty"`
	Accept        *grammar.Invocation `json:"accept,omitempty"`
	Reason        string              `json:"reason"`
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
	// Added are newly changed lines and file events accepted for their
	// proposed owner with --accept-proposed; empty without it.
	Added []refreshAddition `json:"added"`
	// NeedsJudgment are stale references: an edit landed inside them. Each
	// carries its proposal and, when there is one, the accept command. They
	// are never accepted here.
	NeedsJudgment []staleRow `json:"needs_judgment"`
	// Uncovered is newly changed code no reference covers, per file, with its
	// proposed owner and accept command when one Item covers the file.
	Uncovered []refreshGap             `json:"uncovered"`
	Slides    []SlideTransactionResult `json:"slide_transactions"`
}

// reviewRefreshCoverage re-runs a review's coverage over its current range.
func reviewRefreshCoverage(ctx context.Context, args []string, out io.Writer) error {
	name := "review refresh-coverage"
	flags := commandFlags(name, commandUsage[name], out)
	reviewID := flags.String("review", "", "review id")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	accept := flags.Bool("accept-proposed", false, "give newly changed lines to their proposed owner, the one Item covering their file, after checking its explanation covers them")
	path := flags.String("path", "", "with --accept-proposed: accept only this file's lines")
	note := flags.String("note", "", "with --accept-proposed: the note of the added references (default: added in <head>)")
	dryRun := flags.Bool("dry-run", false, "report what would change without writing")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *reviewID); err != nil {
		return err
	}
	if !*accept && (*path != "" || *note != "") {
		return fmt.Errorf("--path and --note go with --accept-proposed")
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
	options := refreshOptions{accept: *accept, note: *note}
	if *path != "" {
		options.path = filepath.ToSlash(filepath.Clean(*path))
	}
	result, edits := planRefresh(ctx, resolver, review, changes, root, *repo, options)
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

// refreshOptions is what the author accepted: with accept, newly changed
// lines of path (every path when empty) go to their proposed owner under
// note.
type refreshOptions struct {
	accept     bool
	path, note string
}

// planRefresh decides every mechanical edit and reports the rest.
func planRefresh(ctx context.Context, resolver *coderesolve.Resolver, review *saga.Review, changes gitdiff.ChangeSet, root, repo string, options refreshOptions) (refreshOutput, []evidenceEdit) {
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
		// The reference keeps its side: deleted-side evidence is read at the
		// merge-base, the rest at the head (reviewView).
		view := reviewView(ctx, resolver, ref.Code, changes.BaseOID, changes.HeadOID)
		atHead := resolver.Resolve(ctx, ref.Code, changes.HeadOID)
		atBase := resolver.Resolve(ctx, ref.Code, changes.BaseOID)
		atView := atHead
		if view == changes.BaseOID {
			atView = atBase
		}
		move := func(location coderef.Location, why string) {
			replacement, err := resolver.Author(ctx, location, ref.Code.Note)
			if err != nil {
				return
			}
			edits = append(edits, evidenceEdit{EvidenceFile: file, Reference: ref.Index, previous: ref.Code, replacement: replacement})
			result.Moved = append(result.Moved, refreshMove{Item: ref.Owner, EvidenceFile: file, Reference: ref.Index, From: ref.Code.Location(), To: location, Why: why})
		}
		// Coverage counts a reference wherever it is current, as review list
		// does, so what refresh reports uncovered matches it.
		for _, resolution := range []coderesolve.Resolution{atHead, atBase} {
			if resolution.Current() {
				effective = append(effective, resolution.Location)
				hold(resolution.Location.Path, ref.Owner, file)
			}
		}
		switch {
		case atView.Current():
			if atView.Moved && view == changes.BaseOID {
				move(atView.Location, "its deleted-side lines moved with the merge-base")
			} else if atView.Moved {
				move(atView.Location, "its lines moved")
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
			row := staleRow{ownedReference: ref, Pinned: ref.Code.Location(), Reason: atView.Reason, Proposal: resolver.Propose(ctx, ref.Code, view)}
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
	note := options.note
	if note == "" {
		// Another reference's note explains other code; a neutral note says
		// only where the lines came from until the author writes one.
		note = "added in " + shortOID(changes.HeadOID)
	}
	assign := func(location coderef.Location) {
		items := holders[location.Path]
		owner, ownerFile := "", ""
		if len(items) == 1 {
			for item, file := range items {
				// A managed slide's evidence takes exact lines only.
				if _, _, _, managed := managedEvidence(file); !managed || !location.WholeFile() {
					owner, ownerFile = item, file
				}
			}
		}
		if owner != "" && options.accept && (options.path == "" || options.path == location.Path) {
			if reference, err := resolver.Author(ctx, location, note); err == nil {
				edits = append(edits, evidenceEdit{EvidenceFile: ownerFile, replacement: reference})
				result.Added = append(result.Added, refreshAddition{Item: owner, EvidenceFile: ownerFile, Location: location, Note: note})
				return
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
			case owner != "":
				reason = "one Item covers this file, so it is the proposed owner; accept only if its explanation covers these lines, else cover them with the Item that does"
			case len(candidates) == 1:
				reason = "the Item that covers this file cannot take it mechanically (a managed slide takes exact lines only)"
			}
			gap = &refreshGap{Path: location.Path, Locations: []string{}, Candidates: candidates, Reason: reason}
			if owner != "" {
				values := []grammar.Value{grammar.V("review", review.ID), grammar.V("accept-proposed", "true"), grammar.V("path", location.Path)}
				if repo != "" {
					values = append(values, grammar.V("repo", repo))
				}
				accept := grammar.MustInvoke("review refresh-coverage", root, values...)
				gap.ProposedOwner, gap.Accept = owner, &accept
			}
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
	fmt.Fprintf(out, "%s review %s's coverage over %s..%s: %d moved, %d added, %d %s judgment, %d %s uncovered\n", verb, result.Review, shortOID(result.BaseOID), shortOID(result.HeadOID),
		len(result.Moved), len(result.Added), len(result.NeedsJudgment), plural(len(result.NeedsJudgment), "needs", "need"), len(result.Uncovered), plural(len(result.Uncovered), "file", "files"))
	// Only what happened is listed; an empty section says nothing.
	if len(result.Moved) > 0 {
		fmt.Fprintln(out, "Moved (no judgment needed):")
	}
	for _, move := range result.Moved {
		fmt.Fprintf(out, "  %s #%d %s -> %s: %s\n", move.EvidenceFile, move.Reference, shortLocation(move.From), shortLocation(move.To), move.Why)
	}
	if len(result.Added) > 0 {
		fmt.Fprintln(out, "Accepted for their proposed owner:")
	}
	for _, added := range result.Added {
		fmt.Fprintf(out, "  %s  %s  %q\n", added.Item, shortLocation(added.Location), added.Note)
	}
	if len(result.NeedsJudgment) > 0 {
		fmt.Fprintln(out, "Needs judgment (an edit landed inside; read the diff):")
	}
	for _, row := range result.NeedsJudgment {
		fmt.Fprintf(out, "  %s #%d %s\n", row.EvidenceFile, row.Index, describeProposal(row.Pinned, row.Proposal))
		for _, line := range row.Proposal.Diff {
			fmt.Fprintf(out, "      %s\n", line)
		}
		if row.Accept != nil {
			fmt.Fprintf(out, "    accept if the explanation still holds: %s\n", shellJoin(row.Accept.Argv))
		}
	}
	if len(result.Uncovered) > 0 {
		fmt.Fprintln(out, "Uncovered, left for you (read the new lines before giving them to an Item):")
	}
	for _, gap := range result.Uncovered {
		fmt.Fprintf(out, "  %s: %s (%s)\n", gap.Path, strings.Join(gap.Locations, " "), gap.Reason)
		for _, candidate := range gap.Candidates {
			if candidate == gap.ProposedOwner {
				fmt.Fprintf(out, "    proposed owner %s\n", candidate)
			} else {
				fmt.Fprintf(out, "    candidate %s\n", candidate)
			}
		}
		if gap.Accept != nil {
			fmt.Fprintf(out, "    accept if its explanation covers them: %s [--note TEXT]\n", shellJoin(gap.Accept.Argv))
		}
		cover := grammar.MustInvoke("cover", root, grammar.V("target", ""), grammar.V("path", gap.Path), grammar.V("changed-lines", "true"))
		fmt.Fprintf(out, "    cover: %s\n", shellJoin(cover.Argv))
	}
	for _, slide := range result.Slides {
		fmt.Fprintf(out, "  slide %s: snapshot %s\n", slide.Target, slide.Snapshot)
	}
}
