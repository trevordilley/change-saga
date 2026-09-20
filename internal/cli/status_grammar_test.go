package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/requirements"
)

// grammarHelp runs one grammar command with -h through the same entry
// points main uses, returning the real flag set's help text. A query's help
// is its usage line, since query writes JSON.
func grammarHelp(t *testing.T, name string) string {
	t.Helper()
	fields := strings.Fields(name)
	if fields[0] == "query" {
		return strings.ReplaceAll(queryUsage[""], "--", "  -") + " "
	}
	run := skillCommands[fields[0]]
	if run == nil {
		t.Fatalf("no dispatcher for implemented grammar command %q", name)
	}
	var output bytes.Buffer
	_ = run(context.Background(), append(append([]string{}, fields[1:]...), "-h"), &output)
	return output.String()
}

// usageKey names a grammar command's entry in commandUsage, where the design
// family's operations are spelled design-<operation>.
func usageKey(name string) string {
	if operation, ok := strings.CutPrefix(name, "design "); ok {
		return "design-" + operation
	}
	return name
}

// runnableCommands returns every command the CLI runs: each commandUsage
// entry that is not a family of further commands, by its grammar name.
func runnableCommands() []string {
	result := []string{}
	for key := range commandUsage {
		name := key
		if operation, ok := strings.CutPrefix(key, "design-"); ok {
			name = "design " + operation
		}
		family := false
		for other := range commandUsage {
			family = family || strings.HasPrefix(other, key+" ") || key == "design" && strings.HasPrefix(other, "design-")
		}
		if !family {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

// The grammar is the single source spec --json and next actions read. Every
// implemented shape must match the CLI's own usage line byte for byte, and
// every flag it declares must be accepted by the real flag set.
func TestGrammarMatchesTheImplementedCLI(t *testing.T) {
	for _, command := range grammar.Commands() {
		if command.Status != grammar.StatusImplemented {
			continue
		}
		if usage := commandUsage[usageKey(command.Name)]; usage != command.Usage {
			t.Errorf("%s usage drifted:\n grammar %q\n cli     %q", command.Name, command.Usage, usage)
		}
		help := grammarHelp(t, command.Name)
		for _, flag := range command.Flags {
			if !strings.Contains(help, "  -"+flag.Name+" ") && !strings.Contains(help, "  -"+flag.Name+"\n") {
				t.Errorf("%s declares --%s but the CLI flag set does not accept it:\n%s", command.Name, flag.Name, help)
			}
		}
	}
}

func TestSpecJSONDescribesTheLivingGrammar(t *testing.T) {
	var output bytes.Buffer
	if err := Spec([]string{"--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Version       int             `json:"version"`
		ChapterSuffix string          `json:"chapter_suffix"`
		Deck          json.RawMessage `json:"implementation_deck"`
		Reserved      []string        `json:"reserved_directories"`
		Living        struct {
			Resources      []grammar.Resource `json:"resources"`
			RelationMatrix []grammar.Relation `json:"relation_matrix"`
			Commands       []grammar.Command  `json:"commands"`
			StatusSchema   string             `json:"status_schema"`
		} `json:"living"`
	}
	if err := json.Unmarshal(output.Bytes(), &contract); err != nil {
		t.Fatal(err)
	}
	if contract.Version != 5 || contract.ChapterSuffix != ".chapter" || len(contract.Deck) == 0 || len(contract.Reserved) == 0 {
		t.Fatal("spec must describe the one Saga format")
	}
	kinds := map[string]bool{}
	for _, resource := range contract.Living.Resources {
		kinds[resource.Kind] = true
	}
	for _, kind := range []string{"story", "criterion", "citation", "relation", "prototype", "prototype-annotation", "coverage-exception", "test-case", "quality-policy", "quality-evidence", "test-run"} {
		if !kinds[kind] {
			t.Errorf("spec omits living resource %s", kind)
		}
	}
	addresses := false
	for _, relation := range contract.Living.RelationMatrix {
		if relation.Type == "addresses" && strings.Contains(strings.Join(relation.Sources, ","), "item") && strings.Contains(strings.Join(relation.Scopes, ","), "descendants") {
			addresses = true
		}
	}
	if !addresses {
		t.Error("spec omits the v5 addresses endpoint matrix")
	}
	if len(contract.Living.Commands) != len(grammar.Commands()) || contract.Living.StatusSchema != StatusSchema {
		t.Error("spec commands are the grammar table")
	}
	published := map[string]bool{}
	for _, command := range contract.Living.Commands {
		published[command.Name] = true
	}
	for _, name := range runnableCommands() {
		if !published[name] {
			t.Errorf("spec --json omits the CLI command %q", name)
		}
	}
}

func TestStatusJSONKeepsV1KeysAndAddsTheAuthoringGrammar(t *testing.T) {
	root, repo := coveredSaga(t)
	var output bytes.Buffer
	_ = Status(context.Background(), []string{"--against", "main", "--json", "--repo", repo, root}, &output)
	var document map[string]any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, output.String())
	}
	for _, key := range []string{"complete", "summary", "uncovered", "overlaps", "stale_references", "saga_changes"} {
		if _, ok := document[key]; !ok {
			t.Errorf("status dropped version 1 key %q", key)
		}
	}
	for _, key := range []string{"status_schema", "coverage", "axes", "stale", "changed_source", "quality", "next_actions", "authoring_loop"} {
		if _, ok := document[key]; !ok {
			t.Errorf("status omits %q", key)
		}
	}
	assertNoReducingStatusKey(t, document, "")
}

func assertNoReducingStatusKey(t *testing.T, value any, path string) {
	t.Helper()
	forbidden := map[string]bool{"percent": true, "percentage": true, "score": true, "ratio": true, "fraction": true, "progress": true, "readiness_score": true, "coverage_percent": true, "overall": true}
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if forbidden[key] {
				t.Errorf("%s/%s reduces status to one opaque number", path, key)
			}
			assertNoReducingStatusKey(t, child, path+"/"+key)
		}
	case []any:
		for _, child := range value {
			assertNoReducingStatusKey(t, child, path+"/[]")
		}
	}
}

// Regression: on a v5 Saga, a test case linked by a verifies relation whose
// story was then revised is linked-but-stale. It is not an orphan, the stale
// reason is exactly the moved story pin, and the author gets exactly one next
// action for it: re-pin. Recommending both re-pin and link would lead an agent
// to create a duplicate relation.
func TestStatusReportsAStaleTestCaseLinkOnceAndNeverAsAnOrphan(t *testing.T) {
	ctx := context.Background()
	root, repo := coveredSaga(t)
	mustRun := func(name string, err error, output *bytes.Buffer) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, output.String())
		}
	}
	var output bytes.Buffer
	if err := addCheckoutStory(t, root, "checkout"); err != nil {
		t.Fatal(err)
	}
	document, err := requirements.Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	prefix := "urn:change-saga:" + document.SagaID
	story, testCase, relation := prefix+":story:checkout", prefix+":test-case:fast", prefix+":relation:fast-verifies"
	mustRun("accept", Story(ctx, []string{"set-state", root, "--story", story, "--event", "accepted", "--parent", story + ":event:proposed", "--state", "accepted", "--json"}, &output), &output)
	runQuality(t, "", "test-case", "add", root, "--feature", testFeature, "--id", "fast", "--title", "Fast checkout", "--kind", "positive",
		"--automation", "automated", "--step", `{"id":"s1","action":"Check out","expected_result":"Done"}`, "--expected-result", "Done")
	mustRun("relation add", Relation(ctx, []string{"add", root, "--feature", testFeature, "--id", "fast-verifies", "--type", "verifies", "--from", testCase,
		"--to", story + ":criterion:fast", "--rationale", "Exercises the fast path.", "--json"}, &output), &output)
	mustRun("story revise", Story(ctx, []string{"revise", root, "--story", story, "--revision", "r2", "--parent", story + ":revision:r1",
		"--persona", personaURNFor(document.SagaID), "--title", "Checkout", "--statement", "As a buyer I can check out quickly", "--priority", "must",
		"--criterion", "fast=Checkout finishes in two seconds", "--json"}, &output), &output)

	output.Reset()
	_ = Status(ctx, []string{"--against", "main", "--json", "--repo", repo, root}, &output)
	var status struct {
		Quality struct {
			TestCases []struct {
				TestCase string `json:"test_case"`
				Orphaned bool   `json:"orphaned"`
				Verifies []struct {
					Relation string `json:"relation"`
					Currency string `json:"currency"`
				} `json:"verifies"`
			} `json:"test_cases"`
		} `json:"quality"`
		Stale []struct {
			Record  string   `json:"record"`
			Reasons []string `json:"reasons"`
		} `json:"stale"`
		NextActions []struct {
			ID       string `json:"id"`
			Resource string `json:"resource"`
			Question *struct {
				Options []struct {
					Commands []struct {
						Command string   `json:"command"`
						Argv    []string `json:"argv"`
					} `json:"commands"`
				} `json:"options"`
			} `json:"question"`
		} `json:"next_actions"`
	}
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatalf("status: %v\n%s", err, output.String())
	}
	if len(status.Quality.TestCases) != 1 || status.Quality.TestCases[0].Orphaned || len(status.Quality.TestCases[0].Verifies) != 1 ||
		status.Quality.TestCases[0].Verifies[0].Relation != relation || status.Quality.TestCases[0].Verifies[0].Currency != "stale" {
		t.Fatalf("the linked test case is stale, not orphaned: %#v", status.Quality.TestCases)
	}
	found := false
	for _, record := range status.Stale {
		if record.Record == relation {
			found = true
			if len(record.Reasons) != 1 || record.Reasons[0] != "to criterion statement changed" {
				t.Fatalf("the only stale reason is the reworded criterion: %#v", record.Reasons)
			}
		}
	}
	if !found {
		t.Fatalf("the stale relation is in the stale set: %#v", status.Stale)
	}
	matching := []string{}
	for _, action := range status.NextActions {
		if action.Resource == relation || action.Resource == testCase {
			matching = append(matching, action.ID)
		}
	}
	if len(matching) != 1 || matching[0] != "stale:"+relation {
		t.Fatalf("exactly one next action concerns the test case, and it re-pins: %v", matching)
	}
	for _, action := range status.NextActions {
		if action.ID != "stale:"+relation {
			continue
		}
		argv := strings.Join(action.Question.Options[0].Commands[0].Argv, " ")
		if !strings.Contains(argv, "relation repin") || !strings.Contains(argv, "--relation "+relation) ||
			!strings.Contains(argv, "--to-revision "+story+":revision:r2") {
			t.Fatalf("the re-pin advances the relation it already has: %s", argv)
		}
	}
}
