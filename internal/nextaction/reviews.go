package nextaction

import (
	"sort"

	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/reviewstate"
)

// Reviews returns one action per file of an open review whose deck leaves
// changes of the review's range uncovered: cover them from the review Item
// that explains them. cover on a review Item compares the review's own range,
// so the shape needs no --against. Review coverage is reported, never gating:
// these actions name no gate. The actions follow every Derive action.
func Reviews(reports []reviewstate.Report, sagaPath string) []Action {
	actions := []Action{}
	for _, report := range reports {
		if report.Merged != nil || report.Coverage == nil {
			continue
		}
		for _, file := range report.Coverage.UncoveredFiles {
			actions = append(actions, Action{
				ID: "review:uncovered:" + report.ID + ":" + file.Path, Kind: KindCommand, Category: CategoryReview,
				Resource: report.Target,
				Reason: itoa(file.Atoms) + " changed lines or file events of " + file.Path + " in review " + report.ID +
					"'s range are explained by no review Item; cover them from the review Item that explains them (" + report.Target + ":slide:<slide>:item:<item>)",
				Command: ptr(grammar.MustInvoke("cover", sagaPath, grammar.V("target", ""), grammar.V("path", file.Path), grammar.V("changed-lines", "true"))),
			})
		}
	}
	sort.SliceStable(actions, func(i, j int) bool { return actions[i].ID < actions[j].ID })
	return actions
}
