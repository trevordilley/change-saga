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

// Growth offers are how prominently status suggests growing the Saga beyond
// its reviews. A review-only Saga that holds no more than growthAfterReviews
// reviews gets one quiet line; past that, a short section; and whoever asks
// (status --growth), or a Saga that already holds living documentation, gets
// the full report.
const (
	growthOfferQuiet     = "quiet"
	growthOfferProminent = "prominent"
	growthOfferFull      = "full"
	// growthAfterReviews is "a couple": creating the universe happens after
	// a couple of reviews, so the offer grows louder only past it.
	growthAfterReviews = 2
	// prominentGrowthSuggestions is how many growth suggestions the short
	// section shows.
	prominentGrowthSuggestions = 3
)

// growthState says whether the Saga has grown beyond its reviews and how
// prominently status offers to grow it. It never blocks anything.
type growthState struct {
	// LivingDocumentation is true once the Saga holds any record beyond its
	// pull request reviews: a feature, a deck, the overview, a persona, a
	// story, a term, and so on.
	LivingDocumentation bool `json:"living_documentation"`
	// Reviews counts every review the Saga holds, open or merged.
	Reviews int `json:"reviews"`
	// Offer is quiet, prominent, or full.
	Offer string `json:"offer"`
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

// growthOf decides the growth offer. A Saga with living documentation has
// opted into it, so its report is the full one; so is every report someone
// asks to grow.
func growthOf(root string, document *saga.Saga, asked bool) growthState {
	state := growthState{LivingDocumentation: holdsLivingDocumentation(root), Reviews: len(document.Reviews), Offer: growthOfferQuiet}
	switch {
	case asked || state.LivingDocumentation:
		state.Offer = growthOfferFull
	case state.Reviews > growthAfterReviews:
		state.Offer = growthOfferProminent
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
// deck's changed-source actions are left out, because the review deck is
// what explains the change; growth follows the offer.
func reviewFirstActions(actions []nextaction.Action, growth growthState) []nextaction.Action {
	if growth.Offer == growthOfferFull {
		return actions
	}
	result := []nextaction.Action{}
	suggested := 0
	for _, action := range actions {
		switch action.Category {
		case nextaction.CategorySource:
			continue
		case nextaction.CategoryGrowth:
			if growth.Offer != growthOfferProminent || suggested >= prominentGrowthSuggestions {
				continue
			}
			suggested++
		}
		result = append(result, action)
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

// printGrowthOffer prints the growth offer of a review-only Saga: one quiet
// line, or past a couple of reviews a short section with the guided setup and
// the first few suggestions (printed with the other actions).
func printGrowthOffer(out io.Writer, status statusDocument) {
	switch status.Growth.Offer {
	case growthOfferQuiet:
		fmt.Fprintf(out, "\nGrowing the Saga is optional: personas, stories, features, and living documentation can come after a few reviews (change-saga status --growth %s).\n", status.sagaPath)
	case growthOfferProminent:
		fmt.Fprintf(out, "\nGrow the Saga (optional, never required): it holds %d reviews and no living documentation yet.\n", status.Growth.Reviews)
		fmt.Fprintln(out, "  When the team wants the app itself documented, with personas, stories, features, and design that stay current as reviews land,")
		fmt.Fprintln(out, "  run change-saga setup-initial-saga for a guided interview, or take one small step below.")
		fmt.Fprintf(out, "  Every suggestion: change-saga status --growth %s\n", status.sagaPath)
	}
}
