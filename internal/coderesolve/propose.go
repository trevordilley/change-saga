package coderesolve

import (
	"bytes"
	"context"
	"fmt"
	"unicode"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/gitdiff"
)

// maxProposalDiff bounds the diff a proposal shows, so a range swallowed by a
// large rewrite still reads as a hint rather than a patch.
const maxProposalDiff = 40

// maxAdjacentDiff bounds the lines shown of each hunk just outside the range,
// so context never crowds out the hunks inside it.
const maxAdjacentDiff = 8

// Proposal is where a stale reference's lines are at a viewed commit by diff
// arithmetic alone: the pinned start and end mapped through the hunks, widened
// to take in every line a hunk inside the range inserted. It is a suggestion
// for an author who has read the diff, never a judgment that the explanation
// still holds. Location is nil when nothing can be proposed, and Reason says
// why; otherwise Reason says what the arithmetic did.
type Proposal struct {
	Location *coderef.Location `json:"location,omitempty"`
	// Widened is set when the proposal takes in lines the original range did
	// not have: lines a hunk inserted inside it, a replacement longer than
	// what it replaced, or code a hunk crossing its edge rewrote. A pure move,
	// a removal, or a replacement no longer than the original leaves it unset.
	Widened bool   `json:"widened,omitempty"`
	Reason  string `json:"reason"`
	// Diff is the zero-context diff of the hunks inside the range: -old and
	// +new lines under each hunk header, capped at maxProposalDiff lines. The
	// nearest hunk on each side outside the range comes first and last, its
	// header marked "(just before the range)" or "(just after the range)",
	// so code extracted or moved next to the range shows beside it.
	Diff []string `json:"diff,omitempty"`
}

// Proposed reports whether the proposal names a location.
func (proposal Proposal) Proposed() bool { return proposal.Location != nil }

// Propose computes the proposal for reference at view, which must be a full
// object name. A reference already current at view is proposed where it
// resolves.
func (resolver *Resolver) Propose(ctx context.Context, reference coderef.Reference, view string) Proposal {
	none := func(format string, args ...any) Proposal { return Proposal{Reason: fmt.Sprintf(format, args...)} }
	if resolution := resolver.Resolve(ctx, reference, view); resolution.Current() {
		location := resolution.Location
		return Proposal{Location: &location, Reason: "current at " + short(view)}
	}
	exists, err := resolver.CommitExists(ctx, reference.Commit)
	if err != nil || !exists {
		return none("pinned commit %s is not in this repository, so its diff cannot be read", short(reference.Commit))
	}
	if checked := resolver.verify(ctx, reference); checked.reason != "" {
		return none("%s; re-cover it by hand", checked.reason)
	}
	changes, err := resolver.treeChanges(ctx, reference.Commit, view)
	if err != nil {
		return none("%v", err)
	}
	change, changed := changes[reference.Path]
	switch {
	case !changed:
		return none("%s did not change between %s and %s", reference.Path, short(reference.Commit), short(view))
	case change.Deleted:
		return none("%s was deleted; if its content moved to another file, re-cover it there", reference.Path)
	case change.Binary:
		return none("%s is binary; its lines cannot be mapped", reference.Path)
	}
	if reference.WholeFile() {
		added, removed := 0, 0
		for _, hunk := range change.Hunks {
			added, removed = added+hunk.NewCount, removed+hunk.OldCount
		}
		location := coderef.Location{Commit: view, Path: change.NewPath}
		return Proposal{Location: &location, Reason: fmt.Sprintf("the whole file at %s (+%d -%d lines since the pin)", short(view), added, removed)}
	}
	mapped := ProposeRange(change.Hunks, reference.Start, reference.End)
	if !mapped.OK {
		return none("%s", mapped.Reason)
	}
	if len(mapped.Touched) > 0 {
		// A lone brace that survived an edit anchors nothing: it may close
		// whatever block now sits where the range was.
		pinned, err := resolver.blob(ctx, reference.Commit, reference.Path)
		if err != nil {
			return none("%v", err)
		}
		if onlyTrivialSurvive(pinned.lines, mapped.Touched, reference.Start, reference.End) {
			return none("only trivial lines (braces, blank, punctuation) of L%d-L%d survive, so nothing anchors a proposal; re-cover by hand", reference.Start, reference.End)
		}
	}
	proposal := Proposal{
		Location: &coderef.Location{Commit: view, Path: change.NewPath, Start: mapped.Start, End: mapped.End},
		Widened:  mapped.Widened, Reason: mapped.Reason,
	}
	proposal.Diff = resolver.rangeDiff(ctx, reference, view, change, mapped)
	return proposal
}

// onlyTrivialSurvive reports whether some line of start..end no touched hunk
// removed survives, and every such line is trivial.
func onlyTrivialSurvive(lines [][]byte, touched []gitdiff.Hunk, start, end int) bool {
	survived := false
	line := start
	// survive checks the lines from line through last, which no hunk removed.
	survive := func(last int) bool {
		for ; line <= last; line++ {
			if line < 1 || line > len(lines) {
				continue
			}
			if !trivial(lines[line-1]) {
				return false
			}
			survived = true
		}
		return true
	}
	for _, hunk := range touched {
		if hunk.OldCount == 0 {
			continue
		}
		if !survive(min(hunk.OldStart-1, end)) {
			return false
		}
		line = max(line, hunk.OldStart+hunk.OldCount)
	}
	return survive(end) && survived
}

// trivial reports whether line is blank or only braces, brackets,
// parentheses, and other punctuation or symbols.
func trivial(line []byte) bool {
	for _, r := range string(bytes.TrimSpace(line)) {
		if !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
			return false
		}
	}
	return true
}

// RangeProposal is ProposeRange's answer. Touched lists the hunks inside or
// across the range, in order. Before and After are the nearest hunks wholly
// outside the range on each side, nil when there is none.
type RangeProposal struct {
	Start, End    int
	Widened       bool
	OK            bool
	Reason        string
	Touched       []gitdiff.Hunk
	Before, After *gitdiff.Hunk
}

// ProposeRange maps the inclusive range start..end across zero-context
// hunks. An unchanged boundary line moves by the hunks before it. A boundary
// line a hunk changed maps to that hunk's edge on the new side, so the
// proposal takes in the hunk's replacement lines. It refuses when every line
// of the range was removed, and when no line of the range survives and a hunk
// crosses its edge, because nothing then anchors the range to lines on the new
// side. The proposal is widened when it takes in lines the range did not
// have.
func ProposeRange(hunks []gitdiff.Hunk, start, end int) RangeProposal {
	result := RangeProposal{}
	startShift, endShift := 0, 0
	startHunk, endHunk := -1, -1
	survivors := end - start + 1
	crosses, grows := false, false
	for index, hunk := range hunks {
		if hunk.OldCount == 0 {
			// A pure insertion after line OldStart.
			if hunk.OldStart < start {
				startShift += hunk.NewCount
			}
			if hunk.OldStart < end {
				endShift += hunk.NewCount
			}
			switch {
			case hunk.OldStart < start:
				result.Before = &hunks[index]
			case hunk.OldStart >= end:
				if result.After == nil {
					result.After = &hunks[index]
				}
			default:
				result.Touched = append(result.Touched, hunk)
				grows = grows || hunk.NewCount > 0
			}
			continue
		}
		last := hunk.OldStart + hunk.OldCount - 1
		delta := hunk.NewCount - hunk.OldCount
		if last < start {
			startShift += delta
		}
		if last < end {
			endShift += delta
		}
		if last < start {
			result.Before = &hunks[index]
			continue
		}
		if hunk.OldStart > end {
			if result.After == nil {
				result.After = &hunks[index]
			}
			continue
		}
		result.Touched = append(result.Touched, hunk)
		grows = grows || hunk.NewCount > hunk.OldCount
		survivors -= min(last, end) - max(hunk.OldStart, start) + 1
		if hunk.OldStart <= start && start <= last {
			startHunk = index
		}
		if hunk.OldStart <= end && end <= last {
			endHunk = index
		}
		if hunk.OldStart < start || last > end {
			crosses = true
		}
	}
	result.Start, result.End = start+startShift, end+endShift
	if startHunk >= 0 {
		hunk := hunks[startHunk]
		result.Start = hunk.NewStart
		if hunk.NewCount == 0 {
			// A deletion's NewStart is the line before it.
			result.Start = hunk.NewStart + 1
		}
	}
	if endHunk >= 0 {
		hunk := hunks[endHunk]
		result.End = hunk.NewStart + hunk.NewCount - 1
		if hunk.NewCount == 0 {
			result.End = hunk.NewStart
		}
	}
	switch {
	case result.End < result.Start || result.Start < 1:
		result.Reason = fmt.Sprintf("every line of %d-%d was removed", start, end)
		return RangeProposal{Reason: result.Reason}
	case survivors == 0 && crosses:
		result.Reason = fmt.Sprintf("lines %d-%d were rewritten together with code outside them, so no line anchors a proposal; re-cover by hand", start, end)
		return RangeProposal{Reason: result.Reason}
	}
	result.OK = true
	result.Widened = crosses || grows
	switch {
	case len(result.Touched) == 0:
		result.Reason = "the lines only moved"
	case survivors == 0:
		result.Reason = "every line of the range changed; proposed the lines that replaced it"
	case crosses:
		result.Reason = "an edit crosses the range's edge; widened to take in all of its lines"
	case grows:
		result.Reason = "edits inside the range; widened to take in the lines they inserted"
	default:
		result.Reason = "edits inside the range only removed or replaced lines"
	}
	return result
}

// rangeDiff renders the touched hunks as -old and +new lines, between the
// nearest hunks outside the range.
func (resolver *Resolver) rangeDiff(ctx context.Context, reference coderef.Reference, view string, change gitdiff.FileChange, mapped RangeProposal) []string {
	if len(mapped.Touched) == 0 {
		return nil
	}
	oldBlob, oldErr := resolver.blob(ctx, reference.Commit, reference.Path)
	newBlob, newErr := resolver.blob(ctx, view, change.NewPath)
	if oldErr != nil || newErr != nil {
		return nil
	}
	var lines []string
	total := 0
	add := func(line string) {
		total++
		if len(lines) < maxProposalDiff {
			lines = append(lines, line)
		}
	}
	// render adds one hunk, showing at most limit of its lines when limit is
	// positive.
	render := func(hunk gitdiff.Hunk, label string, limit int) {
		header := fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount)
		if label != "" {
			header += " (" + label + ")"
		}
		add(header)
		var body []string
		for line := hunk.OldStart; line < hunk.OldStart+hunk.OldCount; line++ {
			if line >= 1 && line <= len(oldBlob.lines) {
				body = append(body, "-"+string(bytes.TrimRight(oldBlob.lines[line-1], "\r\n")))
			}
		}
		for line := hunk.NewStart; line < hunk.NewStart+hunk.NewCount; line++ {
			if line >= 1 && line <= len(newBlob.lines) {
				body = append(body, "+"+string(bytes.TrimRight(newBlob.lines[line-1], "\r\n")))
			}
		}
		if limit > 0 && len(body) > limit {
			body = append(body[:limit], fmt.Sprintf("… %d more lines of this hunk", len(body)-limit))
		}
		for _, line := range body {
			add(line)
		}
	}
	if mapped.Before != nil {
		render(*mapped.Before, "just before the range", maxAdjacentDiff)
	}
	for _, hunk := range mapped.Touched {
		render(hunk, "", 0)
	}
	if mapped.After != nil {
		render(*mapped.After, "just after the range", maxAdjacentDiff)
	}
	if total > len(lines) {
		lines = append(lines, fmt.Sprintf("… %d more diff lines", total-len(lines)))
	}
	return lines
}

// ChangedPaths names every product path a change between from and to
// touches, on either side of a rename.
func (resolver *Resolver) ChangedPaths(ctx context.Context, from, to string) (map[string]bool, error) {
	changes, err := resolver.treeChanges(ctx, from, to)
	if err != nil {
		return nil, err
	}
	paths := map[string]bool{}
	for old, change := range changes {
		paths[old] = true
		if change.NewPath != "" {
			paths[change.NewPath] = true
		}
	}
	return paths, nil
}

// Removed reports whether every line of start..end of path at from is a line
// the change from..to removes or replaces: deleted-side evidence, which
// documents what a change took out rather than code it left behind.
func (resolver *Resolver) Removed(ctx context.Context, from, to, path string, start, end int) bool {
	if start == 0 && end == 0 {
		return false
	}
	changes, err := resolver.treeChanges(ctx, from, to)
	if err != nil {
		return false
	}
	change, ok := changes[path]
	if !ok {
		return false
	}
	if change.Deleted {
		return true
	}
	line := start
	for _, hunk := range change.Hunks {
		if hunk.OldCount == 0 || hunk.OldStart+hunk.OldCount-1 < line {
			continue
		}
		if hunk.OldStart > line {
			return false
		}
		line = hunk.OldStart + hunk.OldCount
		if line > end {
			return true
		}
	}
	return false
}
