package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/areas"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/nextaction"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Report shapes. A Saga that holds only reviews is reported review first:
// the review's coverage and what it asks, and nothing about documentation it
// never adopted. A Saga with living documentation, or anyone who asks with
// status --full, gets the full report. Nothing is counted or offered: a team
// may never grow its Saga beyond reviews, and that is fine.
const (
	reportReviewFirst = "review_first"
	reportFull        = "full"
)

// documentationState says whether the Saga documents the application beyond
// its reviews, and which report status gave. It never blocks anything.
type documentationState struct {
	// LivingDocumentation is true once the Saga holds any record beyond its
	// pull request reviews: a feature, a deck, the overview, a persona, a
	// story, a term, and so on.
	LivingDocumentation bool `json:"living_documentation"`
	// Report is review_first or full.
	Report string `json:"report"`
}

// reviewOnlyEntries are the top-level entries of a Saga that holds nothing but
// reviews: what init creates, a companion Saga's sync cursor, the reviews,
// and the review-side records.
var reviewOnlyEntries = map[string]bool{
	saga.ManifestName: true, "README.md": true, saga.CursorName: true, saga.ReviewsDir: true, saga.MergesDir: true,
	"___claims": true, "___verifications": true,
}

// holdsLivingDocumentation reports whether the Saga holds any record beyond
// its reviews. It reads the Saga's top level, so any documentation kind,
// including one added later, counts without being listed: anything but the
// review-only entries, or an app-level code reference. Hidden files never
// count.
func holdsLivingDocumentation(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return true
	}
	for _, entry := range entries {
		name := entry.Name()
		switch {
		case strings.HasPrefix(name, "."), reviewOnlyEntries[name]:
		case name == saga.CodeDirName:
			if code, err := os.ReadDir(filepath.Join(root, name)); err != nil || len(code) > 0 {
				return true
			}
		default:
			return true
		}
	}
	return false
}

// documentationOf decides the report: full for a Saga with living
// documentation, which opted into it, or when asked; review first otherwise.
func documentationOf(root string, asked bool) documentationState {
	state := documentationState{LivingDocumentation: holdsLivingDocumentation(root), Report: reportReviewFirst}
	if asked || state.LivingDocumentation {
		state.Report = reportFull
	}
	return state
}

// reviewOfChange is an open review a comparison is about, with its report
// and, when its own range differs from the comparison, why.
type reviewOfChange struct {
	review *saga.Review
	report reviewstate.Report
}

// matchReviews finds the open reviews a comparison is about, and the
// reviews that follow HEAD whose evidence was rewritten (an amend, rebase,
// or squash), which likely are this change's review and only need pinning.
//
// A review that names a branch matches when that branch (or origin's) is
// the comparison's head, or the second parent of GitHub's pull request merge
// commit. When the branch does not resolve here, as a fork's branch never
// does in the base repository's CI, it matches when its evidence is part of
// this change. A review that follows HEAD matches unless its evidence shows
// another change. Merged reviews are history and never match.
func matchReviews(ctx context.Context, checkout string, reviews []*saga.Review, reports []reviewstate.Report, changes gitdiff.ChangeSet) ([]reviewOfChange, []*saga.Review) {
	byID := map[string]reviewstate.Report{}
	for _, report := range reports {
		byID[report.ID] = report
	}
	heads := map[string]bool{changes.HeadOID: true}
	if parents := strings.Fields(gitText(ctx, checkout, "rev-list", "--parents", "-n", "1", changes.HeadOID)); len(parents) == 3 {
		heads[parents[2]] = true
	}
	result, rewritten := []reviewOfChange{}, []*saga.Review{}
	for _, review := range reviews {
		if review.Merged != nil {
			continue
		}
		matched := false
		followsHEAD := review.Head == "" || review.Head == "HEAD"
		head, _, err := reviewstate.ResolveHead(ctx, checkout, review)
		switch evidence := evidenceOf(ctx, checkout, review, changes); {
		case !followsHEAD && err == nil:
			matched = heads[head]
		case !followsHEAD:
			matched = evidence == evidenceInChange
		case evidence == evidenceNone || evidence == evidenceInChange:
			matched = true
		case evidence == evidenceRewritten:
			rewritten = append(rewritten, review)
		}
		if matched {
			result = append(result, reviewOfChange{review: review, report: byID[review.ID]})
		}
	}
	return result, rewritten
}

// Evidence states: what a review's pinned code says about which change it
// explains.
const (
	evidenceNone      = "none"
	evidenceInChange  = "in_change"
	evidenceLanded    = "landed"
	evidenceRewritten = "rewritten"
)

// evidenceOf reads which change a review's evidence belongs to. Evidence is
// part of this change when a pinned commit is the comparison's base (the
// deleted side) or an ancestor of its head but not of its base. Otherwise it
// landed when every pinned commit is in the base, and was rewritten (amended,
// rebased, squashed, or another branch's) when some commit is in neither.
func evidenceOf(ctx context.Context, checkout string, review *saga.Review, changes gitdiff.ChangeSet) string {
	commits := map[string]bool{}
	if review.Deck != nil {
		for _, slide := range review.Deck.Slides {
			for _, item := range slide.Items {
				for _, file := range item.Code {
					for _, reference := range file.References {
						commits[reference.Commit] = true
					}
				}
			}
		}
	}
	if len(commits) == 0 {
		return evidenceNone
	}
	landed := true
	for commit := range commits {
		if commit == changes.BaseOID {
			return evidenceInChange
		}
		inBase := isAncestor(ctx, checkout, commit, changes.BaseOID)
		if !inBase && isAncestor(ctx, checkout, commit, changes.HeadOID) {
			return evidenceInChange
		}
		landed = landed && inBase
	}
	if landed {
		return evidenceLanded
	}
	return evidenceRewritten
}

func isAncestor(ctx context.Context, checkout, ancestor, descendant string) bool {
	_, err := gitexec.Output(ctx, "-C", checkout, "merge-base", "--is-ancestor", ancestor, descendant)
	return err == nil
}

func gitText(ctx context.Context, checkout string, args ...string) string {
	output, err := gitexec.Output(ctx, append([]string{"-C", checkout}, args...)...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// unreviewedReason is why a changed line is uncovered when no review is about
// the change.
const unreviewedReason = "no review explains this change yet"

// reviewArea computes the review coverage area. Comparing, it is measured
// over the comparison's own changed lines: each review of the change's Items
// are evaluated against them, so a review whose range is narrower than the
// comparison (a stacked pull request) or cannot be read is never reported
// complete for lines it does not explain. Observing, it is each open review
// over its own range.
func reviewArea(ctx context.Context, matched []reviewOfChange, rewritten []*saga.Review, branch, root string, reports []reviewstate.Report, changes gitdiff.ChangeSet, resolver coverage.Resolver) areas.Area {
	if changes.Mode == gitdiff.ModeCompare {
		if len(matched) == 0 && len(rewritten) > 0 {
			// Like status's headline: the review is most likely this change's,
			// so it is pinned rather than duplicated.
			return areas.ReviewOverChange(changes.Atoms, nil, func(gitdiff.Atom) string { return unreviewedReason }, rewrittenReviewNote(rewritten, branch, root))
		}
		if len(matched) == 0 {
			return areas.ReviewOverChange(changes.Atoms, nil, func(gitdiff.Atom) string { return unreviewedReason }, "no review follows "+changes.Head+"; create one with change-saga review create")
		}
		via := map[string][]string{}
		notes := []string{}
		for _, match := range matched {
			covered := reviewstate.Evaluate(ctx, match.review, changes, resolver)
			uncovered := map[string]bool{}
			for _, atom := range covered.Uncovered {
				uncovered[atom.Key] = true
			}
			for _, atom := range changes.Atoms {
				if !uncovered[atom.Key] {
					via[atom.Key] = append(via[atom.Key], match.review.Target)
				}
			}
			if note := rangeNote(match, changes); note != "" {
				notes = append(notes, note)
			}
		}
		reason := "no Item of " + strings.Join(matchedTargets(matched), " or ") + " explains it"
		if len(notes) > 0 {
			reason += "; " + strings.Join(notes, "; ")
		}
		return areas.ReviewOverChange(changes.Atoms, via, func(gitdiff.Atom) string { return reason }, "measured over this change's changed lines")
	}
	inputs := []areas.ReviewInput{}
	for _, report := range reports {
		input := areas.ReviewInput{Target: report.Target, Title: report.Title}
		if report.Coverage == nil {
			input.Unreadable = firstNonEmpty(strings.Join(report.Diagnostics, "; "), "its range could not be read")
		} else {
			input.Covered, input.Uncovered = report.Coverage.Summary.Covered, report.Coverage.Uncovered
		}
		inputs = append(inputs, input)
	}
	if len(reports) == 0 {
		return areas.ReviewArea(nil, nil, "", "no open reviews")
	}
	return areas.ReviewArea(inputs, nil, "", "each open review over its own range")
}

// rangeNote says how a review's own range differs from the comparison: a
// different base (a stacked pull request), or a range that cannot be read.
func rangeNote(match reviewOfChange, changes gitdiff.ChangeSet) string {
	switch rng := match.report.Range; {
	case rng == nil:
		return "review " + match.review.ID + "'s own range could not be read (" + firstNonEmpty(strings.Join(match.report.Diagnostics, "; "), "unknown") + ")"
	case rng.BaseOID != changes.BaseOID:
		return "review " + match.review.ID + " reviews its own range from " + match.review.Base + ", which differs from this comparison against " + changes.Base
	}
	return ""
}

func matchedTargets(matched []reviewOfChange) []string {
	result := []string{}
	for _, match := range matched {
		result = append(result, match.review.ID)
	}
	return result
}

// reviewAreaActions are the next actions for a comparison's review gaps: a
// cover command for each file whose lines lie in the review's own range, or,
// when the review's range differs from the comparison, one action that says
// so and compares against the review's own base.
func reviewAreaActions(area areas.Area, matched []reviewOfChange, changes gitdiff.ChangeSet, root, repo string) []nextaction.Action {
	if len(matched) == 0 || area.Complete {
		return nil
	}
	for _, match := range matched {
		if note := rangeNote(match, changes); note != "" {
			values := []grammar.Value{grammar.V("against", match.review.Base)}
			if repo != "" {
				values = append(values, grammar.V("repo", repo))
			}
			return []nextaction.Action{{
				ID: "review:range:" + match.review.ID, Kind: nextaction.KindCommand, Category: nextaction.CategoryReview, Resource: match.review.Target,
				Reason:  fmt.Sprintf("%d changed lines of %s..%s are explained by no review Item; %s. Explain the rest in the review of that base, or compare against the review's own base", area.Uncovered, changes.Base, changes.Head, note),
				Command: ptrInvocation(grammar.MustInvoke("status", root, values...)),
			}}
		}
	}
	files := map[string]int{}
	order := []string{}
	for _, entry := range area.UncoveredEntries {
		if _, seen := files[entry.Resource]; !seen {
			order = append(order, entry.Resource)
		}
		files[entry.Resource] += entry.Count
	}
	actions := []nextaction.Action{}
	target := matched[0].review.Target
	for _, path := range order {
		values := []grammar.Value{grammar.V("target", ""), grammar.V("path", path), grammar.V("changed-lines", "true")}
		if repo != "" {
			values = append(values, grammar.V("repo", repo))
		}
		actions = append(actions, nextaction.Action{
			ID: "review:uncovered:" + matched[0].review.ID + ":" + path, Kind: nextaction.KindCommand, Category: nextaction.CategoryReview, Resource: target,
			Reason:  fmt.Sprintf("%d changed lines or file events of %s are explained by no review Item; cover them from the review Item that explains them (%s:slide:<slide>:item:<item>)", files[path], path, target),
			Command: ptrInvocation(grammar.MustInvoke("cover", root, values...)),
		})
	}
	return actions
}

// rewrittenReviewNote says that reviews follow HEAD but their evidence was
// rewritten, and how to pin the first to the branch.
func rewrittenReviewNote(rewritten []*saga.Review, branch, root string) string {
	ids := []string{}
	for _, review := range rewritten {
		ids = append(ids, review.ID)
	}
	return fmt.Sprintf("%s follows HEAD, but none of its evidence is in this change (amended, rebased, or squashed?); if it is this change's review, pin it: change-saga review follow --review %s --head %s %s", strings.Join(ids, ", "), ids[0], firstNonEmpty(branch, "BRANCH"), shellJoin([]string{root}))
}

// followReviewAction is the next action for a review that follows HEAD but
// whose evidence was rewritten: it is most likely this change's review, so it
// is pinned to the branch rather than duplicated.
func followReviewAction(review *saga.Review, branch, root string) nextaction.Action {
	return nextaction.Action{
		ID: "review:follow:" + review.ID, Kind: nextaction.KindCommand, Category: nextaction.CategoryReview, Resource: review.Target,
		Reason:  "review " + review.ID + " follows HEAD, but none of its evidence is in this change (an amend, rebase, or squash rewrites it); if it is this change's review, pin it to the branch, then re-cover what moved",
		Command: ptrInvocation(grammar.MustInvoke("review follow", root, grammar.V("review", review.ID), grammar.V("head", branch))),
	}
}

// createReviewAction is the next action for a change no review explains yet.
func createReviewAction(changes gitdiff.ChangeSet, root, repo string) nextaction.Action {
	values := []grammar.Value{grammar.V("base", changes.Base)}
	if repo != "" {
		values = append(values, grammar.V("repo", repo))
	}
	return nextaction.Action{
		ID: "review:create", Kind: nextaction.KindCommand, Category: nextaction.CategoryReview,
		Reason:  fmt.Sprintf("no review explains the %d changed lines of %s..%s yet; create one, then explain the change's architecture on its slides", len(changes.Atoms), changes.Base, changes.Head),
		Command: ptrInvocation(grammar.MustInvoke("review create", root, values...)),
	}
}

func ptrInvocation(value grammar.Invocation) *grammar.Invocation { return &value }

// reviewFirstActions keeps what a review-only Saga asks of its author: the
// review's own gaps and anything existing that broke. The implementation
// deck's changed-source actions and documentation growth suggestions are left
// out, because the review deck is what explains the change.
func reviewFirstActions(actions []nextaction.Action, documentation documentationState) []nextaction.Action {
	if documentation.Report == reportFull {
		return actions
	}
	result := []nextaction.Action{}
	for _, action := range actions {
		if action.Category != nextaction.CategorySource && action.Category != nextaction.CategoryGrowth {
			result = append(result, action)
		}
	}
	return result
}

// printReviewHeadline prints the first-class answer for a change: how
// completely the review deck explains it.
func printReviewHeadline(out io.Writer, status statusDocument, maxItems int) {
	area := status.Coverage.Areas.Review
	comparing := status.Opening.Mode == gitdiff.ModeCompare
	switch {
	case comparing && len(status.ChangeReviews) == 0 && area.Total == 0:
		return
	case comparing && len(status.ChangeReviews) == 0 && len(status.RewrittenReviews) > 0:
		fmt.Fprintf(out, "\nReview: none matched. %s follows HEAD, but none of its evidence is in this change (amended, rebased, or squashed?).\n", strings.Join(status.RewrittenReviews, ", "))
		fmt.Fprintf(out, "  If it is this change's review, pin it: change-saga review follow --review %s --head %s %s\n", status.RewrittenReviews[0], firstNonEmpty(status.branch, "BRANCH"), shellJoin([]string{status.sagaPath}))
		return
	case comparing && len(status.ChangeReviews) == 0:
		fmt.Fprintf(out, "\nReview: none yet. No review explains the %d changed lines of this change.\n", area.Total)
		repo := ""
		if status.repoFlag != "" {
			repo = " --repo " + shellJoin([]string{status.repoFlag})
		}
		fmt.Fprintf(out, "  Create one: change-saga review create --base %s%s %s\n", shellJoin([]string{status.Opening.Against}), repo, shellJoin([]string{status.sagaPath}))
		return
	case len(status.ChangeReviews) == 0:
		return
	}
	label := "Review " + status.ChangeReviews[0] + ": its deck explains"
	if len(status.ChangeReviews) > 1 {
		label = "Reviews " + strings.Join(status.ChangeReviews, ", ") + ": their decks explain"
	}
	subject := "this change"
	if !comparing {
		subject = "their own ranges"
		if len(status.ChangeReviews) == 1 {
			subject = "its own range"
		}
	}
	fmt.Fprintf(out, "\n%s %d of %d changed lines and file events of %s", label, area.Covered, area.Total, subject)
	if area.Complete {
		fmt.Fprintln(out, " — every changed line is explained.")
	} else {
		fmt.Fprintf(out, " (%d not yet explained).\n", area.Uncovered)
		printGaps(out, area, maxItems)
	}
}
