package cli

import (
	"context"
	"fmt"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"io"
	"strings"

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
	livingapp.Status
	NextActions   []nextaction.Action `json:"next_actions"`
	AuthoringLoop nextaction.Loop     `json:"authoring_loop"`
}

// comparison is one Saga snapshot with the source comparison it describes.
type comparison struct {
	document   *saga.Saga
	validation saga.Validation
	changes    gitdiff.ChangeSet
	report     coverage.Report
	checkout   string
}

func readComparison(ctx context.Context, root, repoDir string, allowMismatch bool) (comparison, error) {
	document, validation, err := saga.Load(root)
	if err != nil {
		return comparison{}, err
	}
	checkout := firstNonEmpty(repoDir, document.Root)
	changes, err := gitdiff.ReadWithOptions(ctx, checkout, document.Manifest.Source.Repository, document.Manifest.Source.Base, document.Manifest.Source.Head, gitdiff.ReadOptions{AllowRepositoryMismatch: allowMismatch})
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
func buildStatus(ctx context.Context, root, repoDir string, allowMismatch bool) (statusDocument, error) {
	value, err := readComparison(ctx, root, repoDir, allowMismatch)
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
	return statusDocument{
		Report: value.report, Schema: StatusSchema, Status: living,
		NextActions: nextaction.Derive(living, root), AuthoringLoop: nextaction.AuthoringLoop(root),
	}, nil
}

// readyForReview is status's pass/fail: the ready_for_review gate, which
// requires every earlier gate (changed-source accounting included) plus no
// conflicts, orphaned evidence, or failed required runs. review_complete is
// reported but does not decide the exit code: it waits on reviewer decisions,
// which authoring cannot supply.
func (status statusDocument) readyForReview() bool {
	gate, ok := status.Readiness.Gate(readiness.GateReadyForReview)
	return ok && gate.Status == readiness.StatusReady
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
		fmt.Fprintf(out, "  %d. [%s] %s\n", index+1, action.Category, firstLine(action.Reason))
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
			"readiness_gate_order": []string{"requirements_ready", "product_ready", "design_ready", "implementation_trace_ready", "quality_ready", "ready_for_review", "review_complete"},
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
