package cli

import (
	"context"

	"fmt"
	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/areas"
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
	"github.com/twentyideas/changesaga/internal/saga"
)

// StatusSchema names the machine-readable status contract. Version 3
// replaces the readiness gate table with the coverage report: status has no
// verdict, and "coverage" is the part teams write their own rules over.
const StatusSchema = "change-saga.status/v3"

// statusDocument is the `status --json` contract. Coverage is the report
// teams script against: every area's counts and lists, in scope. The embedded
// report is the changed-line accounting behind it; the embedded living status
// adds axis cells, stale pins, and the quality domain; the next actions are
// derived from all of them and built from the published grammar.
type statusDocument struct {
	coverage.Report
	Schema string `json:"status_schema"`
	// Opening is how the Saga was opened. It is never read from the Saga.
	Opening opening `json:"opening"`
	// Coverage is the coverage report: for each area, how many things in
	// scope are covered, with the lists of what is and is not. It has no
	// verdict and no blended score.
	Coverage areas.Report `json:"coverage"`
	// Comparison is the Changed, Affected, and Code layers; only a Saga
	// opened with --against has them.
	Comparison *changeview.Layers `json:"comparison,omitempty"`
	livingapp.Status
	// Reviews reports every open pull request review slide by slide: each
	// reviewer's decision and whether it is out of date, and how completely
	// its deck covers its range. It is a report, never part of the exit
	// status.
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
func buildStatus(ctx context.Context, root, repoDir string, rng gitdiff.Range, allowMismatch bool, epic string) (statusDocument, error) {
	value, err := readComparison(ctx, root, repoDir, rng, allowMismatch)
	if err != nil {
		return statusDocument{}, err
	}
	if epic != "" {
		resolved, err := applayout.Require(value.document.Root, value.document.Manifest.ID, epic)
		if err != nil {
			return statusDocument{}, err
		}
		epic = resolved.ID
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
		AuthoringLoop: nextaction.AuthoringLoop(root),
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
	document.Coverage = areas.Evaluate(coverageInputs(value.document, value.changes, value.report, living, document.Comparison, epic))
	document.NextActions = nextaction.Derive(living, root, nextaction.Context{Coverage: document.Coverage, Places: storyPlaces(value.document)})
	document.NextActions = append(document.NextActions, nextaction.Reviews(document.Reviews, root)...)
	return document, nil
}

// trustworthy reports why the status report cannot be trusted, if it
// cannot: the Saga is malformed (a schema error, such as a duplicate ID), or
// its living records could not be composed. Every gap is a finding and never
// makes the report untrustworthy.
func (status statusDocument) trustworthy(root string) error {
	errors := 0
	for _, issue := range status.SchemaIssues {
		if issue.Severity == "error" {
			errors++
		}
	}
	if errors > 0 || !status.SchemaValid {
		return fmt.Errorf("the Saga is malformed (%d schema errors), so this report cannot be trusted; fix them and re-run: change-saga validate %s", errors, root)
	}
	for _, diagnostic := range status.Diagnostics {
		return fmt.Errorf("the Saga's records could not be read (%s), so this report cannot be trusted: %s", diagnostic.Code, diagnostic.Message)
	}
	return nil
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
	printCoverage(out, status.Coverage)
	printAppStatus(out, status.Status)
	if len(status.Reviews) > 0 {
		fmt.Fprintln(out, "\nReviews (decisions per slide and each deck's coverage of its range; the team decides what it requires):")
		printReviewReports(out, status.Reviews)
	}
	if len(status.Stale) > 0 {
		fmt.Fprintf(out, "\nStale pins: %d records must be revisited\n", len(status.Stale))
	}
	work, growth := []nextaction.Action{}, []nextaction.Action{}
	for _, action := range status.NextActions {
		if action.Category == nextaction.CategoryGrowth {
			growth = append(growth, action)
		} else {
			work = append(work, action)
		}
	}
	if len(work) == 0 {
		fmt.Fprintln(out, "\nNext actions: none. Every changed line is covered and nothing existing is stale or broken; that is not a claim of correctness.")
	} else {
		fmt.Fprintf(out, "\nNext actions (%d, in order):\n", len(work))
		printActions(out, work, maxItems, false)
	}
	if len(growth) > 0 {
		fmt.Fprintf(out, "\nGrowth (%d optional suggestions, most valuable to this change first; take them a step at a time or ignore them):\n", len(growth))
		printActions(out, growth, maxItems, true)
	}
}

func printActions(out io.Writer, actions []nextaction.Action, maxItems int, practice bool) {
	limit := len(actions)
	if maxItems > 0 && maxItems < limit {
		limit = maxItems
	}
	for index, action := range actions[:limit] {
		scope := string(action.Category)
		if action.Area != "" && action.Category == nextaction.CategoryGrowth {
			scope = action.Area
		}
		if action.Epic != "" {
			scope += " · epic " + action.Epic
		}
		fmt.Fprintf(out, "  %d. [%s] %s\n", index+1, scope, firstLine(action.Reason))
		if practice && action.Practice != "" {
			fmt.Fprintf(out, "     why: %s\n", action.Practice)
		}
		switch {
		case action.Command != nil:
			fmt.Fprintf(out, "     $ %s\n", strings.Join(action.Command.Argv, " "))
		case action.Question != nil && practice && len(action.Question.Options) > 0 && len(action.Question.Options[0].Commands) > 0:
			fmt.Fprintf(out, "     $ %s\n", strings.Join(action.Question.Options[0].Commands[0].Argv, " "))
		case action.Question != nil:
			fmt.Fprintf(out, "     ? %s\n", action.Question.Text)
		}
	}
	if limit < len(actions) {
		fmt.Fprintf(out, "  … and %d more (use --max 0 or --json)\n", len(actions)-limit)
	}
}

// printAppStatus prints the app-level facts: the overview and its terms,
// epics, persona coverage, the questions persona retirements raise, and
// stories gated off by flags.
func printAppStatus(out io.Writer, status livingapp.Status) {
	printOverviewStatus(out, status)
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

// printOverviewStatus prints the overview's parts, each absent one as a gap,
// every term whose code went stale, and a comparison's new terminology. None
// of it is required.
func printOverviewStatus(out io.Writer, status livingapp.Status) {
	overview := status.Overview
	if overview.Name == "" {
		return
	}
	part := func(value *livingapp.OverviewPart, command string) string {
		if value == nil {
			return "gap (optional; change-saga overview " + command + ")"
		}
		return value.Path
	}
	fmt.Fprintln(out, "\nOverview:")
	fmt.Fprintf(out, "  %-28s %s\n", "name", overview.Name)
	fmt.Fprintf(out, "  %-28s %s\n", "elevator pitch", part(overview.Pitch, "set-pitch"))
	fmt.Fprintf(out, "  %-28s %s\n", "description", part(overview.Description, "set-description"))
	terms := fmt.Sprintf("%d active", overview.Terms)
	if overview.Terms == 0 {
		terms = "gap (optional; change-saga term add)"
	}
	fmt.Fprintf(out, "  %-28s %s\n", "terms and vocabulary", terms)
	for _, term := range status.Terms {
		for _, code := range term.Code {
			if code.State == coderesolve.Stale {
				fmt.Fprintf(out, "  stale term %s (%s): %s — %s\n", term.ID, term.Name, code.Pinned, code.Reason)
			}
		}
	}
	if len(status.NewTerminology) > 0 {
		fmt.Fprintf(out, "\nNew terminology (%d suggestions; optional, never blocking):\n", len(status.NewTerminology))
		for _, suggestion := range status.NewTerminology {
			name := suggestion.Name
			if suggestion.Container != "" {
				name = suggestion.Container + "." + name
			}
			fmt.Fprintf(out, "  %-28s %s#L%d\n", name, suggestion.Location.Path, suggestion.Location.Start)
		}
	}
}

func areaNames() []string {
	result := []string{}
	for _, name := range areas.Names() {
		result = append(result, string(name))
	}
	return result
}

func categoryNames() []string {
	result := []string{}
	for _, category := range nextaction.Categories() {
		result = append(result, string(category))
	}
	return result
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
			"exceptions":          "a cited, revision-pinned decision that one axis does not apply to one criterion; never excuses changed-source accounting",
			"changed_source":      "every changed atom must be owned by some target; transitivity proves criteria reach code but cannot prove nothing else changed",
			"review_coverage":     "every changed line of a review's own range must be covered by its deck's Items; reported per review with next actions, never part of the exit status or the documentation's coverage",
			"staleness":           "derived only from pins (story/test/prototype revisions, content digests, diff selectors, run source identity), never from Git history",
			"no_reducing_numbers": true,
		},
		"coverage_report": map[string]any{
			"areas": areaNames(),
			"area_rules": map[string]string{
				"implementation": "every changed line is referenced by the implementation deck",
				"stories":        "every changed line reaches a story through the chain",
				"personas":       "every changed line reaches a persona",
				"design":         "every story in scope has design",
				"quality":        "every acceptance criterion in scope has a test",
				"health":         "nothing that already existed went stale or broke",
			},
			"units":             []string{string(areas.UnitChangedLine), string(areas.UnitCodeTarget), string(areas.UnitStory), string(areas.UnitCriterion), string(areas.UnitRecord)},
			"scope":             "with --against, the change: what it changed and what it affected; without, the whole app, where the line areas count documented code targets instead of changed lines; --epic narrows either, keeping changed lines no record owns",
			"shape":             "status --json .coverage.areas.<area> has total, covered, uncovered, complete, covered_entries, and uncovered_entries; counts are the sums of the entries' counts; never a blended score",
			"verdict":           "none: status reports every gap as a finding; a team that wants a gap to fail its build asks check --covers or writes its own rule over the JSON",
			"status_exit_codes": map[string]string{"0": "the report was produced; every gap is a finding", "1": "the report cannot be trusted: a malformed Saga (such as a duplicate ID), unreadable records, or a checkout that does not match the declared repository"},
			"check_exit_codes":  map[string]string{"0": "every named area is fully covered in scope", "3": "a named area has a gap; only the named areas' gaps are printed", "1": "the report cannot be trusted, as for status"},
			"check_schema":      CheckSchema,
		},
		"commands": grammar.Commands(),
		"next_actions": map[string]any{
			"kinds":      []string{string(nextaction.KindCommand), string(nextaction.KindQuestion)},
			"needs":      []string{string(nextaction.NeedProductJudgment), string(nextaction.NeedExternalAccess), string(nextaction.NeedExplicitExclusion)},
			"categories": categoryNames(),
			"contract":   "a command action carries a grammar invocation whose inputs the author supplies; a question action carries one focused question and the invocation each answer leads to; area names the coverage area it advances; a growth action is optional and carries the practice it teaches",
			"loop":       "inspect status --json, ask or mutate, validate, re-evaluate; a list of growth suggestions only is the fixed point and never a claim of correctness",
		},
		"status_schema": StatusSchema,
	}
}
