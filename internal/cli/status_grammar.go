package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/reviewstate"

	"github.com/twentyideas/changesaga/internal/changeview"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/nextaction"
	"github.com/twentyideas/changesaga/internal/readiness"
	"github.com/twentyideas/changesaga/internal/saga"
)

// StatusSchema names the machine-readable status contract. Version 2 adds the
// living authoring grammar to the version 1 changed-source report; every
// version 1 key keeps its meaning and position.
const StatusSchema = "change-saga.status/v2"

// statusDocument is the `status --json` contract. The embedded report is the
// unchanged version 1 changed-source accounting; the embedded living status
// adds the gate table, axis cells, stale pins, and quality domain; the next
// actions are derived from both and built from the published grammar.
type statusDocument struct {
	coverage.Report
	Schema string `json:"status_schema"`
	// Opening is how the Saga was opened. It is never read from the Saga.
	Opening opening `json:"opening"`
	// Comparison is the Changed, Affected, and Code layers; only a Saga
	// opened with --against has them.
	Comparison *changeview.Layers `json:"comparison,omitempty"`
	livingapp.Status
	// Reviews reports every open pull request review slide by slide: each
	// reviewer's decision and whether it is out of date. It is a report,
	// never part of the exit status.
	Reviews       []reviewstate.Report `json:"reviews"`
	NextActions   []nextaction.Action  `json:"next_actions"`
	AuthoringLoop nextaction.Loop      `json:"authoring_loop"`
}

// opening names how a Saga was opened: observe one commit, or compare head
// with its merge-base with against.
type opening struct {
	Mode    string `json:"mode"`
	Against string `json:"against,omitempty"`
	Head    string `json:"head"`
	BaseOID string `json:"base_oid,omitempty"`
	HeadOID string `json:"head_oid"`
	// Companion is set when the Saga lives in a different repository from
	// its code; Cursor is then the code commit its sync cursor names.
	Companion bool   `json:"companion,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
}

func openingOf(changes gitdiff.ChangeSet) opening {
	value := opening{Mode: changes.Mode, Against: changes.Base, Head: changes.Head, HeadOID: changes.HeadOID}
	if changes.Mode == gitdiff.ModeCompare {
		value.BaseOID = changes.BaseOID
	}
	return value
}

// comparison is one Saga snapshot with the source comparison it describes.
type comparison struct {
	document   *saga.Saga
	validation saga.Validation
	changes    gitdiff.ChangeSet
	report     coverage.Report
	checkout   string
}

func readComparison(ctx context.Context, root, repoDir string, rng gitdiff.Range, allowMismatch bool) (comparison, error) {
	document, validation, err := saga.Load(root)
	if err != nil {
		return comparison{}, err
	}
	checkout := firstNonEmpty(repoDir, document.Root)
	changes, err := gitdiff.ReadRange(ctx, checkout, document.Manifest.Source.Repository, rng, gitdiff.ReadOptions{AllowRepositoryMismatch: allowMismatch})
	if err != nil {
		return comparison{}, fmt.Errorf("read source diff (use --repo for a separate saga repository): %w", err)
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return comparison{}, fmt.Errorf("open source repository: %w", err)
	}
	defer resolver.Close()
	report := coverage.Evaluate(ctx, document, validation, changes, resolver)
	return comparison{document: document, validation: validation, changes: changes, report: report, checkout: checkout}, nil
}

// buildStatus composes the complete status document. A living-record load
// failure never hides changed-source accounting: it becomes a diagnostic and
// the first next action.
func buildStatus(ctx context.Context, root, repoDir string, rng gitdiff.Range, allowMismatch bool) (statusDocument, error) {
	value, err := readComparison(ctx, root, repoDir, rng, allowMismatch)
	if err != nil {
		return statusDocument{}, err
	}
	resolver, err := coderesolve.New(ctx, value.checkout)
	if err != nil {
		return statusDocument{}, fmt.Errorf("open source repository: %w", err)
	}
	defer resolver.Close()
	options := livingapp.StatusOptions{SagaRoot: root, Document: value.document, Report: value.report, Changes: value.changes, Resolver: resolver}
	living, err := livingapp.LoadStatus(ctx, options)
	if err != nil {
		living = livingapp.Assemble(livingapp.StatusInputs{
			SagaID: value.document.Manifest.ID, SagaVersion: value.document.Manifest.Version,
			Report: value.report, Changes: value.changes,
			Diagnostics: []livingapp.Diagnostic{{Code: "living_records_unavailable", Message: err.Error()}},
		})
	}
	view := openingOf(value.changes)
	if companionCheckout(ctx, root, value.checkout) {
		view.Companion = true
		if cursor, ok, err := saga.ReadCursor(root); err == nil && ok {
			view.Cursor = cursor.Commit
		}
	}
	document := statusDocument{
		Report: value.report, Schema: StatusSchema, Opening: view, Status: living,
		NextActions: nextaction.Derive(living, root), AuthoringLoop: nextaction.AuthoringLoop(root),
	}
	var open []*saga.Review
	for _, review := range value.document.Reviews {
		if review.Merged == nil {
			open = append(open, review)
		}
	}
	if document.Reviews, err = buildReviewReports(ctx, value.document, value.checkout, open); err != nil {
		return statusDocument{}, err
	}
	if value.changes.Mode == gitdiff.ModeCompare {
		layers, _, err := changeview.Open(ctx, changeview.OpenOptions{
			SagaRoot: root, Document: value.document, Checkout: value.checkout,
			Changes: value.changes, Report: value.report, Resolver: resolver,
		})
		if err != nil {
			return statusDocument{}, fmt.Errorf("open the comparison: %w", err)
		}
		document.Comparison = &layers
	}
	return document, nil
}

// readyForReview is status's pass/fail: the ready_for_review gate, which
// requires every earlier gate (changed-source accounting included) plus no
// conflicts, orphaned evidence, or failed required runs. Reviews are reported
// slide by slide and never decide the exit code: the team decides.
func (status statusDocument) readyForReview() bool {
	gate, ok := status.Readiness.Gate(readiness.GateReadyForReview)
	return ok && gate.Status == readiness.StatusReady
}

// printReasons prints the commits beside a record, each squash merge with
// the branch commits it collapsed.
func printReasons(out io.Writer, reasons []changeview.Reason) {
	for _, reason := range reasons {
		fmt.Fprintf(out, "      why: %s %s\n", shortOID(reason.Commit), reason.Subject)
		for _, collapsed := range reason.Collapsed {
			fmt.Fprintf(out, "           %s %s\n", shortOID(collapsed.Commit), collapsed.Subject)
		}
	}
}

// printComparison prints the three layers of a compared Saga.
func printComparison(out io.Writer, layers *changeview.Layers, maxItems int) {
	if layers == nil {
		return
	}
	limit := func(n int) int {
		if maxItems > 0 && maxItems < n {
			return maxItems
		}
		return n
	}
	fmt.Fprintf(out, "\nChanged (%d records the change added, revised, or retired):\n", len(layers.Changed))
	for _, change := range layers.Changed[:limit(len(layers.Changed))] {
		fmt.Fprintf(out, "  %-8s %-10s %s  %s\n", change.Change, change.Kind, change.Title, change.URN)
		if pairing := change.Pair; pairing != nil {
			switch {
			case pairing.Basis == changeview.PairAmbiguous:
				fmt.Fprintf(out, "      may replace or be replaced by one of %s; link it: %s\n", strings.Join(pairing.Candidates, ", "), pairing.Link)
			case pairing.Role == changeview.PairReplaces:
				fmt.Fprintf(out, "      replaces %s (%s)\n", pairing.With, pairing.Basis)
			default:
				fmt.Fprintf(out, "      replaced by %s (%s)\n", pairing.With, pairing.Basis)
			}
		}
		printReasons(out, change.Reasons)
	}
	fmt.Fprintf(out, "\nAffected (%d records the change did not edit but invalidated):\n", len(layers.Affected))
	for _, affected := range layers.Affected[:limit(len(layers.Affected))] {
		fmt.Fprintf(out, "  %-10s %s  %s\n", affected.Kind, affected.Title, affected.URN)
		for _, cause := range affected.Because {
			fmt.Fprintf(out, "      %s: %s\n", cause.Kind, cause.Detail)
		}
		printReasons(out, affected.Reasons)
	}
	fmt.Fprintf(out, "\nCode: %d changed lines under %d records; %d lines no record references\n", layers.Summary.ChangedLines, layers.Summary.CodeGroups, layers.Summary.Unreferenced)
	for _, diagnostic := range layers.Diagnostics {
		fmt.Fprintf(out, "  note %s: %s\n", diagnostic.Code, diagnostic.Message)
	}
}

func printLivingStatus(out io.Writer, status statusDocument, maxItems int) {
	fmt.Fprintln(out, "\nReadiness:")
	for _, gate := range status.Readiness.Gates {
		fmt.Fprintf(out, "  %-28s %s", gate.Name, gate.Status)
		if len(gate.Blockers) > 0 {
			fmt.Fprintf(out, " — %d blocking facts", len(gate.Blockers))
		}
		fmt.Fprintln(out)
	}
	printAppStatus(out, status.Status)
	if len(status.Reviews) > 0 {
		fmt.Fprintln(out, "\nReviews (decisions per slide; the team decides what it requires):")
		printReviewReports(out, status.Reviews)
	}
	if len(status.Stale) > 0 {
		fmt.Fprintf(out, "\nStale pins: %d records must be revisited\n", len(status.Stale))
	}
	if len(status.NextActions) == 0 {
		fmt.Fprintln(out, "\nNo next actions: no required current gap remains. That is not a claim of correctness.")
		return
	}
	fmt.Fprintf(out, "\nNext actions (%d, in order):\n", len(status.NextActions))
	limit := len(status.NextActions)
	if maxItems > 0 && maxItems < limit {
		limit = maxItems
	}
	for index, action := range status.NextActions[:limit] {
		scope := string(action.Category)
		if action.Epic != "" {
			scope += " · epic " + action.Epic
		}
		fmt.Fprintf(out, "  %d. [%s] %s\n", index+1, scope, firstLine(action.Reason))
		switch {
		case action.Command != nil:
			fmt.Fprintf(out, "     $ %s\n", strings.Join(action.Command.Argv, " "))
		case action.Question != nil:
			fmt.Fprintf(out, "     ? %s\n", action.Question.Text)
		}
	}
	if limit < len(status.NextActions) {
		fmt.Fprintf(out, "  … and %d more (use --max 0 or --json)\n", len(status.NextActions)-limit)
	}
}

// printAppStatus prints the app-level facts: epics, persona coverage, the
// questions persona retirements raise, and stories gated off by flags.
func printAppStatus(out io.Writer, status livingapp.Status) {
	if len(status.Epics) > 0 {
		fmt.Fprintln(out, "\nEpics:")
		for _, epic := range status.Epics {
			stories := "stories"
			if len(epic.Stories) == 1 {
				stories = "story"
			}
			fmt.Fprintf(out, "  %-28s %d %s", epic.ID, len(epic.Stories), stories)
			if len(epic.GatedBy) > 0 {
				fmt.Fprintf(out, ", gated by %s", strings.Join(epic.GatedBy, ", "))
			}
			fmt.Fprintln(out)
		}
	}
	if len(status.Personas) > 0 {
		fmt.Fprintln(out, "\nPersonas:")
		for _, persona := range status.Personas {
			served := fmt.Sprintf("served by %d accepted stories", len(persona.ServedBy))
			if len(persona.ServedBy) == 1 {
				served = "served by 1 accepted story"
			}
			if persona.Gap {
				served = "gap: no accepted story serves it"
			}
			fmt.Fprintf(out, "  %-28s %-8s %s\n", persona.ID, persona.State, served)
		}
	}
	for _, group := range status.PersonaOrphans {
		fmt.Fprintf(out, "\nStories serving only retired %s: %s — retire them or reassign them\n", strings.Join(group.Personas, ", "), strings.Join(group.Stories, ", "))
	}
	for _, story := range status.Stories {
		if story.Availability == livingapp.AvailabilityImplementedNotEnabled {
			fmt.Fprintf(out, "\nImplemented, not enabled: %s (gated by %s)\n", story.Story, strings.Join(story.GatedBy, ", "))
		}
	}
}

func firstLine(value string) string {
	if index := strings.IndexByte(value, '\n'); index >= 0 {
		return value[:index]
	}
	return value
}

// livingSpec is the `spec --json` description of the living authoring
// grammar. It is generated from the same grammar table next actions use.
func livingSpec() map[string]any {
	axes := []string{}
	for _, axis := range coverage.Axes() {
		axes = append(axes, string(axis))
	}
	return map[string]any{
		"saga_version":    5,
		"resources":       grammar.Resources(),
		"relation_matrix": grammar.Relations(),
		"derived_edges":   grammar.DerivedEdges(),
		"relation_scopes": map[string]string{
			"self":        "only the source target is asserted to address the requirement",
			"descendants": "a deck or slide source reaches contained Items and their code references",
		},
		"coverage": map[string]any{
			"axes":        axes,
			"axis_rules":  grammar.AxisRules(),
			"cell_states": []string{"covered_direct", "covered_broad", "excluded", "stale", "invalid", "conflicted", "gap"},
			"resolutions": []string{"linked", "excluded", "gap"},
			"quality_kind_states": []string{
				"covered", "missing_kind", "not_run", "failed", "blocked", "skipped", "stale", "inactive", "invalid", "conflicted", "excluded", "automation_not_allowed",
			},
			"exceptions":           "a cited, revision-pinned decision that one axis does not apply to one criterion; never excuses changed-source accounting",
			"changed_source":       "every changed atom must be owned by some target; transitivity proves criteria reach code but cannot prove nothing else changed",
			"staleness":            "derived only from pins (story/test/prototype revisions, content digests, diff selectors, run source identity), never from Git history",
			"no_reducing_numbers":  true,
			"readiness_gate_order": []string{"requirements_ready", "product_ready", "design_ready", "implementation_trace_ready", "quality_ready", "ready_for_review"},
			"readiness_rule":       "every gate always applies and every axis is required; an axis is excused only by an explicit, pinned, cited coverage exception, and no exception excuses changed-source accounting",
			"status_exit_codes":    map[string]string{"0": "ready_for_review is ready", "3": "ready_for_review is blocked"},
		},
		"commands": grammar.Commands(),
		"next_actions": map[string]any{
			"kinds":      []string{string(nextaction.KindCommand), string(nextaction.KindQuestion)},
			"needs":      []string{string(nextaction.NeedProductJudgment), string(nextaction.NeedExternalAccess), string(nextaction.NeedExplicitExclusion)},
			"categories": []string{"invalid_saga", "conflict", "invalid", "stale", "changed_source", "requirements", "coverage", "orphan"},
			"contract":   "a command action carries a grammar invocation whose inputs the author supplies; a question action carries one focused question and the invocation each answer leads to",
			"loop":       "inspect status --json, ask or mutate, validate, re-evaluate; an empty list is the fixed point and never a claim of correctness",
		},
		"status_schema": StatusSchema,
	}
}
