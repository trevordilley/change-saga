package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/grammar"
)

// grammarHelp runs one grammar command with -h through the same entry
// points main uses, returning the real flag set's help text.
func grammarHelp(t *testing.T, name string) string {
	t.Helper()
	fields := strings.Fields(name)
	var output bytes.Buffer
	args := append(append([]string{}, fields[1:]...), "-h")
	ctx := context.Background()
	run := map[string]func() error{
		"story":           func() error { return story(ctx, args, &output, strings.NewReader("")) },
		"criterion":       func() error { return criterion(ctx, args, &output, strings.NewReader("")) },
		"citation":        func() error { return Citation(ctx, args, &output) },
		"relation":        func() error { return Relation(ctx, args, &output) },
		"prototype":       func() error { return Prototype(ctx, args, &output) },
		"add-deck":        func() error { return AddDeck(ctx, args, &output) },
		"cover":           func() error { return Cover(ctx, args, &output) },
		"rebase-evidence": func() error { return RebaseEvidence(ctx, args, &output) },
		"upgrade":         func() error { return Upgrade(ctx, args, &output) },
		"validate":        func() error { return Validate(ctx, args, &output) },
		"status":          func() error { return Status(ctx, args, &output) },
		"spec":            func() error { return Spec(args, &output) },
	}[fields[0]]
	if run == nil {
		t.Fatalf("no dispatcher for implemented grammar command %q", name)
	}
	_ = run()
	return output.String()
}

// The grammar is the single source spec --json and next actions read. Every
// implemented shape must match the CLI's own usage line byte for byte, and
// every flag it declares must be accepted by the real flag set.
func TestGrammarMatchesTheImplementedCLI(t *testing.T) {
	for _, command := range grammar.Commands() {
		if command.Status != grammar.StatusImplemented {
			continue
		}
		if usage := commandUsage[command.Name]; usage != command.Usage {
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

func TestSpecJSONDescribesTheLivingGrammarWithoutBreakingV2Keys(t *testing.T) {
	var output bytes.Buffer
	if err := Spec([]string{"--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Version        int             `json:"version"`
		ChapterSuffix  string          `json:"chapter_suffix"`
		SlideNativeV4  json.RawMessage `json:"slide_native_v4"`
		ReservedLegacy []string        `json:"reserved_directories"`
		Living         struct {
			Resources      []grammar.Resource `json:"resources"`
			RelationMatrix []grammar.Relation `json:"relation_matrix"`
			Commands       []grammar.Command  `json:"commands"`
			StatusSchema   string             `json:"status_schema"`
		} `json:"living"`
	}
	if err := json.Unmarshal(output.Bytes(), &contract); err != nil {
		t.Fatal(err)
	}
	if contract.Version != 2 || contract.ChapterSuffix != ".chapter" || len(contract.SlideNativeV4) == 0 || len(contract.ReservedLegacy) == 0 {
		t.Fatal("existing spec keys must keep their values")
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
}

func TestStatusJSONKeepsV1KeysAndAddsTheAuthoringGrammar(t *testing.T) {
	root, repo := coveredSaga(t)
	var output bytes.Buffer
	_ = Status(context.Background(), []string{"--json", "--repo", repo, root}, &output)
	var document map[string]any
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, output.String())
	}
	for _, key := range []string{"complete", "summary", "uncovered", "overlaps", "orphans", "saga_changes"} {
		if _, ok := document[key]; !ok {
			t.Errorf("status dropped version 1 key %q", key)
		}
	}
	for _, key := range []string{"status_schema", "readiness", "axes", "stale", "changed_source", "quality", "capabilities", "next_actions", "authoring_loop"} {
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
