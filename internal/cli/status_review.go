package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/areas"
	"github.com/twentyideas/changesaga/internal/gitdiff"
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
// reviews: what init creates, the reviews, and the review-side records.
var reviewOnlyEntries = map[string]bool{
	saga.ManifestName: true, "README.md": true, saga.ReviewsDir: true, saga.MergesDir: true,
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

// reviewsOfChange are the open review reports a status is about. Comparing,
// they are the reviews whose head is the comparison's head: the pull request
// under review. Observing, every open review is.
func reviewsOfChange(reports []reviewstate.Report, changes gitdiff.ChangeSet) []reviewstate.Report {
	if changes.Mode != gitdiff.ModeCompare {
		return reports
	}
	result := []reviewstate.Report{}
	for _, report := range reports {
		if report.Range != nil && report.Range.HeadOID == changes.HeadOID {
			result = append(result, report)
		}
	}
	return result
}

// unreviewedReason is why a changed line is uncovered when no review is about
// the change.
const unreviewedReason = "no review explains this change yet"

// reviewArea computes the review coverage area: each review of the change
// over its own range. A comparison with no review of its head leaves every
// changed line unreviewed.
func reviewArea(reports []reviewstate.Report, changes gitdiff.ChangeSet) areas.Area {
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
	if changes.Mode == gitdiff.ModeCompare {
		if len(reports) == 0 {
			return areas.ReviewArea(nil, changes.Atoms, unreviewedReason, "no review follows "+changes.Head+"; create one with change-saga review create")
		}
		return areas.ReviewArea(inputs, nil, "", "each review over its own range")
	}
	if len(reports) == 0 {
		return areas.ReviewArea(nil, nil, "", "no open reviews")
	}
	return areas.ReviewArea(inputs, nil, "", "each open review over its own range")
}

// createReviewAction is the next action for a change no review explains yet.
func createReviewAction(changes gitdiff.ChangeSet, root string) nextaction.Action {
	return nextaction.Action{
		ID: "review:create", Kind: nextaction.KindCommand, Category: nextaction.CategoryReview,
		Reason:  fmt.Sprintf("no review explains the %d changed lines of %s..%s yet; create one, then explain the change's architecture on its slides", len(changes.Atoms), changes.Base, changes.Head),
		Command: ptrInvocation(grammar.MustInvoke("review create", root, grammar.V("base", changes.Base))),
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
	case comparing && len(status.ChangeReviews) == 0:
		fmt.Fprintf(out, "\nReview: none yet. No review explains the %d changed lines of this change.\n", area.Total)
		fmt.Fprintf(out, "  Create one: change-saga review create --base %s %s\n", status.Opening.Against, status.sagaPath)
		return
	case len(status.ChangeReviews) == 0:
		return
	}
	label := "Review " + status.ChangeReviews[0] + ": its deck explains"
	if len(status.ChangeReviews) > 1 {
		label = "Reviews " + strings.Join(status.ChangeReviews, ", ") + ": their decks explain"
	}
	fmt.Fprintf(out, "\n%s %d of %d changed lines and file events of the pull request", label, area.Covered, area.Total)
	if area.Complete {
		fmt.Fprintln(out, " — every changed line is explained.")
	} else {
		fmt.Fprintf(out, " (%d not yet explained).\n", area.Uncovered)
		printGaps(out, area, maxItems)
	}
}
