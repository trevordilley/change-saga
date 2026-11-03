package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/areas"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/nextaction"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// These budgets keep one ordinary call from costing a session its context: an
// agent's every later turn pays again for whatever a call printed. Raise one
// only after asking whether the new bytes are what an agent decides on, or
// belong behind an explicit expansion such as --detail.
const (
	compactStatusBudget       = 48 << 10
	compactSpecBudget         = 8 << 10
	compactRequirementsBudget = 40 << 10
	statusTextWarningRows     = 20
)

// oversizedStatus is a status document far larger than any default should show:
// every list long, every row carrying the detail a compact row leaves out.
func oversizedStatus() statusDocument {
	const rows = 2000
	long := strings.Repeat("a long explanation of one record ", 12)
	var document statusDocument
	entries := make([]areas.Entry, rows)
	for index := range entries {
		entries[index] = areas.Entry{Resource: fmt.Sprintf("internal/pkg/file%d.go", index), Lines: "4-9,12", Count: 7, Via: []string{long}, Targets: []string{long}, Reason: "no Item references these lines"}
	}
	area := areas.Area{Area: areas.Stories, Unit: areas.UnitChangedLine, Total: rows * 14, Uncovered: rows * 7, CoveredEntries: entries, UncoveredEntries: entries}
	document.Coverage.Areas = areas.Areas{Review: area, Implementation: area, Stories: area, Personas: area, Design: area, Quality: area, Health: area}
	document.StaleReferences = make([]coverage.StaleReference, rows)
	document.SchemaIssues = make([]saga.Issue, rows)
	for index := range document.SchemaIssues {
		document.SchemaIssues[index] = saga.Issue{Severity: "warning", Path: "___reviews/pr-1.review/deck/40-e.json", Message: long}
	}
	document.Features = make([]livingapp.FeatureStatus, rows)
	document.Stories = make([]livingapp.StoryStatus, rows)
	document.Status.Stale = make([]livingapp.StaleRecord, rows)
	for index := range document.Status.Stale {
		document.Status.Stale[index] = livingapp.StaleRecord{Record: "urn:change-saga:app:relation:one", Kind: "relation", Reasons: []string{"pin moved"}, Affects: []string{long}}
	}
	document.Dangling = make([]livingapp.DanglingReference, rows)
	document.OverviewRevisions = make([]livingapp.OverviewRevisionStatus, rows)
	document.Reviews = make([]reviewstate.Report, rows)
	for index := range document.Reviews {
		document.Reviews[index] = reviewstate.Report{
			ID: fmt.Sprintf("pr-%d", index), Title: "A reviewed change", Slides: make([]reviewstate.SlideReport, rows/10),
			Coverage: &reviewstate.Coverage{UncoveredFiles: make([]reviewstate.UncoveredFile, rows/10)},
		}
	}
	document.NextActions = make([]nextaction.Action, rows)
	for index := range document.NextActions {
		document.NextActions[index] = nextaction.Action{
			ID: fmt.Sprintf("growth:story:%d", index), Kind: nextaction.KindQuestion, Reason: "documents code no story explains", Practice: long,
			Question: &nextaction.Question{Text: "Which story does it deliver?", Options: []nextaction.Option{{Answer: "capture it", Effect: long, Commands: []grammar.Invocation{{Usage: long}}}}},
		}
	}
	return document
}

func TestCompactStatusStaysWithinItsBudgetHoweverLargeTheSaga(t *testing.T) {
	t.Parallel()
	document := oversizedStatus()
	var compact, complete bytes.Buffer
	if err := writeJSON(&compact, compactStatusDocument(document, compactListLimit)); err != nil {
		t.Fatal(err)
	}
	if compact.Len() > compactStatusBudget {
		t.Fatalf("status --json printed %d bytes of an oversized Saga; its budget is %d", compact.Len(), compactStatusBudget)
	}
	if err := writeJSON(&complete, document); err != nil {
		t.Fatal(err)
	}
	if complete.Len() < compactStatusBudget*10 {
		t.Fatalf("the fixture is only %d bytes in full, so the budget proves nothing", complete.Len())
	}
	var unbounded compactStatus
	var all bytes.Buffer
	if err := writeJSON(&all, compactStatusDocument(document, 0)); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(all.Bytes(), &unbounded); err != nil {
		t.Fatal(err)
	}
	if len(unbounded.NextActions) != len(document.NextActions) || unbounded.NextActionsTotal != len(document.NextActions) || len(unbounded.Coverage.Areas.Stories.UncoveredEntries) != len(document.Coverage.Areas.Stories.UncoveredEntries) {
		t.Fatal("--max 0 does not return every row")
	}
}

// A bound on a list must say it is one: every bounded list has a total, and a
// schema error, which makes the report untrustworthy, is never bounded away.
func TestCompactStatusCountsWhatItLeavesOut(t *testing.T) {
	t.Parallel()
	document := oversizedStatus()
	document.SchemaIssues = append(document.SchemaIssues, saga.Issue{Severity: "error", Path: "x", Message: "malformed"})
	compact := compactStatusDocument(document, compactListLimit)
	if len(compact.NextActions) != compactListLimit || compact.NextActionsTotal != len(document.NextActions) {
		t.Fatalf("next actions: %d of %d", len(compact.NextActions), compact.NextActionsTotal)
	}
	area := compact.Coverage.Areas.Stories
	if len(area.UncoveredEntries) != compactListLimit || area.UncoveredEntriesTotal != 2000 || area.Uncovered != 14000 || area.Total != 28000 {
		t.Fatalf("the area's counts changed with its list: %+v", area)
	}
	if len(compact.SchemaIssues) != compactListLimit || compact.SchemaIssues[0].Severity != "error" || compact.SchemaIssuesTotal != 2001 {
		t.Fatalf("schema issues: %d of %d, first %+v", len(compact.SchemaIssues), compact.SchemaIssuesTotal, compact.SchemaIssues[0])
	}
	if compact.Reviews[0].SlidesTotal != 200 || compact.Reviews[0].Coverage.UncoveredFilesTotal != 200 {
		t.Fatalf("review counts: %+v", compact.Reviews[0])
	}
	errorsOnly := make([]saga.Issue, compactListLimit+5)
	for index := range errorsOnly {
		errorsOnly[index].Severity = "error"
	}
	if got := boundedSchemaIssues(errorsOnly, compactListLimit); len(got) != len(errorsOnly) {
		t.Fatalf("a bound hid schema errors: %d of %d", len(got), len(errorsOnly))
	}
}

// The default keeps the paths a team's rule most likely reads, and a list it
// drops is absent: a rule that walks it fails instead of passing on nothing.
func TestStatusJSONDefaultIsCompactAndDetailIsComplete(t *testing.T) {
	t.Parallel()
	root, repo := coveredSaga(t)
	run := func(args ...string) map[string]any {
		var output bytes.Buffer
		if err := Status(context.Background(), append(append([]string{"--against", "main", "--repo", repo}, args...), root), &output); err != nil {
			t.Fatalf("status %v: %v", args, err)
		}
		var document map[string]any
		if err := json.Unmarshal(output.Bytes(), &document); err != nil {
			t.Fatal(err)
		}
		return document
	}
	compact := run("--json")
	if compact["status_schema"] != CompactStatusSchema {
		t.Fatalf("default schema = %v", compact["status_schema"])
	}
	for _, key := range []string{"complete", "summary", "opening", "documentation", "change_reviews", "stale_references", "next_actions", "next_actions_total", "counts", "expand"} {
		if _, ok := compact[key]; !ok {
			t.Errorf("compact status omits %q", key)
		}
	}
	for _, key := range []string{"uncovered", "overlaps", "targets", "saga_changes", "stories", "terms", "personas", "axes", "quality"} {
		if _, ok := compact[key]; ok {
			t.Errorf("compact status still prints %q", key)
		}
	}
	area := compact["coverage"].(map[string]any)["areas"].(map[string]any)["implementation"].(map[string]any)
	for _, key := range []string{"total", "covered", "uncovered", "complete", "uncovered_entries", "uncovered_entries_total"} {
		if _, ok := area[key]; !ok {
			t.Errorf("a compact area omits %q", key)
		}
	}
	if _, ok := area["covered_entries"]; ok {
		t.Error("a compact area still lists covered entries")
	}
	complete := run("--json", "--detail")
	if complete["status_schema"] != StatusSchema {
		t.Fatalf("--detail schema = %v", complete["status_schema"])
	}
	for _, key := range []string{"uncovered", "stories", "terms", "axes", "quality"} {
		if _, ok := complete[key]; !ok {
			t.Errorf("--detail omits %q", key)
		}
	}
}

func TestStatusActionReturnsOneActionWhole(t *testing.T) {
	t.Parallel()
	actions := []nextaction.Action{{ID: "one"}, {ID: "two", Practice: "why it pays off"}}
	found, err := findNextAction(actions, "two")
	if err != nil || found.Practice == "" {
		t.Fatalf("found %+v, %v", found, err)
	}
	if _, err := findNextAction(actions, "three"); err == nil {
		t.Fatal("an unknown action was found")
	}
	for _, args := range [][]string{{"--detail", "saga"}, {"--action", "x", "saga"}, {"--json", "--detail", "--action", "x", "saga"}} {
		if err := Status(context.Background(), args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "--detail") {
			t.Errorf("status %v: %v", args, err)
		}
	}
}

func TestSpecJSONIsAnIndexThatReadsOnePartOrAll(t *testing.T) {
	t.Parallel()
	read := func(args ...string) (string, error) {
		var output bytes.Buffer
		err := Spec(append([]string{"--json"}, args...), &output)
		return output.String(), err
	}
	index, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if len(index) > compactSpecBudget {
		t.Fatalf("spec --json printed %d bytes; its budget is %d", len(index), compactSpecBudget)
	}
	var parsed specIndex
	if err := json.Unmarshal([]byte(index), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Commands) != len(grammar.Commands()) || len(parsed.Resources) != len(grammar.Resources()) {
		t.Fatal("the index does not name every command and resource")
	}
	whole, err := read("--all")
	if err != nil {
		t.Fatal(err)
	}
	var contract map[string]any
	if err := json.Unmarshal([]byte(whole), &contract); err != nil {
		t.Fatal(err)
	}
	for _, section := range parsed.Sections {
		output, err := read("--section", section)
		if err != nil {
			t.Fatalf("the index names section %q, which does not read: %v", section, err)
		}
		if len(output) >= len(whole) {
			t.Fatalf("section %q is the whole contract", section)
		}
	}
	for _, want := range []string{"implementation_deck", "theme", "whiteboards", "query", "coverage_report", "relation_matrix"} {
		if !containsString(parsed.Sections, want) {
			t.Errorf("the index omits section %q", want)
		}
	}
	command, err := read("--command", "review create")
	if err != nil || !strings.Contains(command, `"usage": "change-saga review create`) {
		t.Fatalf("--command: %v\n%s", err, command)
	}
	resource, err := read("--resource", "story")
	if err != nil || !strings.Contains(resource, `"kind": "story"`) {
		t.Fatalf("--resource: %v\n%s", err, resource)
	}
	deck, err := read("--section", "implementation_deck")
	if err != nil {
		t.Fatal(err)
	}
	var section struct {
		Deck struct {
			Diagram map[string]any `json:"diagram_source"`
		} `json:"implementation_deck"`
	}
	if err := json.Unmarshal([]byte(deck), &section); err != nil || len(section.Deck.Diagram) == 0 {
		t.Fatalf("a section does not keep its path in the whole contract: %v", err)
	}
	for _, args := range [][]string{{"--command", "nope"}, {"--resource", "nope"}, {"--section", "nope"}, {"--section", "living"}, {"--section", "commands"}, {"--all", "--command", "spec"}} {
		if _, err := read(args...); err == nil {
			t.Errorf("spec --json %v was accepted", args)
		}
	}
	if err := Spec([]string{"--all"}, &bytes.Buffer{}); err == nil {
		t.Error("spec --all without --json was accepted")
	}
}

func TestRequirementRowsNameConflictsAndStayWithinTheirBudget(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a long requirement statement ", 40)
	page := livingapp.RequirementPage{Requirements: make([]livingapp.Requirement, livingapp.DefaultPageLimit)}
	for index := range page.Requirements {
		page.Requirements[index] = livingapp.Requirement{
			Requirement: fmt.Sprintf("urn:change-saga:app:story:story-%d", index), ID: fmt.Sprintf("story-%d", index), Feature: "checkout", CreatedAt: time.Now(),
			CurrentRevision: &requirements.Revision{
				Title: "Let a shopper check out with a saved card", Statement: text, Overview: text,
				AcceptanceCriteria: []requirements.Criterion{{ID: "one", Statement: text}, {ID: "two", Statement: text}},
			},
			CurrentLifecycle: &requirements.LifecycleEvent{State: requirements.StateAccepted},
		}
	}
	page.Requirements[1] = livingapp.Requirement{Requirement: "urn:s", ID: "s", Feature: "checkout", RevisionConflict: true, RevisionHeads: []string{"urn:r1", "urn:r2"}}
	rows := livingapp.CompactRequirements(page)
	first, conflicted := rows.Requirements[0], rows.Requirements[1]
	if first.ID != "story-0" || first.Title == "" || first.State != "accepted" || first.Feature != "checkout" || first.Criteria != 2 {
		t.Fatalf("row: %+v", first)
	}
	if conflicted.State != "conflicted" || !conflicted.RevisionConflict || len(conflicted.RevisionHeads) != 2 || conflicted.Title != "" {
		t.Fatalf("a conflict lost its heads: %+v", conflicted)
	}
	var output bytes.Buffer
	if err := writeJSON(&output, rows); err != nil {
		t.Fatal(err)
	}
	if output.Len() > compactRequirementsBudget {
		t.Fatalf("one default page of query requirements printed %d bytes; its budget is %d", output.Len(), compactRequirementsBudget)
	}
}

func TestQueryRequirementsDefaultsToRowsAndSchemaSaysHowToExpand(t *testing.T) {
	t.Parallel()
	parse := func(args ...string) livingQuery {
		parsed, _, _, err := parseQuery("requirements", append([]string{"--saga", "x"}, args...))
		if err != nil {
			t.Fatal(err)
		}
		return parsed.(livingQuery)
	}
	if !parse().CompactRequirements || !parse("--feature", "core").CompactRequirements {
		t.Error("query requirements does not default to rows")
	}
	if parse("--detail").CompactRequirements || parse("--requirement", "story").CompactRequirements {
		t.Error("--detail and --requirement must return the full record")
	}
	description := querySchemaFor("requirements")
	if !strings.Contains(description.Detail, "--detail") || !strings.Contains(strings.Join(description.DataPaths, " "), "data.requirements[].criteria") {
		t.Fatalf("query schema does not describe the rows: %+v", description)
	}
}

func TestStatusTextBoundsSchemaWarningsAndSaysSo(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	var document statusDocument
	document.Opening.Mode = "observe"
	document.SchemaIssues = oversizedStatus().SchemaIssues
	printReportHeader(&output, document, statusTextWarningRows)
	if strings.Count(output.String(), "warning: ") != statusTextWarningRows || !strings.Contains(output.String(), "… and 1980 more warnings (use --max 0)") {
		t.Fatalf("text status:\n%s", output.String())
	}
}
