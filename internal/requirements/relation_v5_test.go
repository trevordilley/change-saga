package requirements

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/twentyideas/changesaga/internal/store"
)

const (
	v5Story       = "urn:change-saga:test:story:checkout"
	v5Criterion   = "urn:change-saga:test:story:checkout:criterion:fast"
	v5StoryR1     = "urn:change-saga:test:story:checkout:revision:r1"
	v5TestCase    = "urn:change-saga:test:test-case:fast-path"
	v5TestCaseR1  = "urn:change-saga:test:test-case:fast-path:revision:r1"
	v5TestCaseR2  = "urn:change-saga:test:test-case:fast-path:revision:r2"
	v5RelationURN = "urn:change-saga:test:relation:fast-path-verifies-fast"
)

func newV5Saga(t *testing.T) string {
	t.Helper()
	root := newSaga(t)
	manifest := map[string]any{
		"$schema": "https://changesaga.dev/schema/v5/saga.schema.json",
		"version": 5, "id": "test", "title": "Test",
		"source": map[string]any{"repository": "https://example.com/repo.git", "base": "main", "head": "feature"},
	}
	if err := store.WriteJSON(filepath.Join(root, "saga.json"), manifest, false); err != nil {
		t.Fatal(err)
	}
	if _, err := AddStory(root, "test", storyInput("checkout", "r1", "proposed", []Criterion{{ID: "fast", Statement: "Checkout finishes promptly"}})); err != nil {
		t.Fatal(err)
	}
	return root
}

func verifiesInput() AddRelationInput {
	return AddRelationInput{
		ID: "fast-path-verifies-fast", Type: RelationVerifies, From: v5TestCase, To: v5Criterion,
		Rationale: "The case exercises the fast path.", FromRevision: v5TestCaseR1, ToRevision: v5StoryR1, CreatedAt: testTime,
	}
}

func compileV5RelationSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(source), "..", "..", "schema", "v5", "relation.schema.json")
	schema, err := jsonschema.NewCompiler().Compile(path)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestV5SagaWritesSchemaValidTestCaseVerifiesRelation(t *testing.T) {
	root := newV5Saga(t)
	result, err := AddRelation(root, "test", verifiesInput())
	if err != nil {
		t.Fatal(err)
	}
	if result.URN != v5RelationURN {
		t.Fatalf("relation URN = %q", result.URN)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.Path)))
	if err != nil {
		t.Fatal(err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := compileV5RelationSchema(t).Validate(instance); err != nil {
		t.Fatalf("written relation violates the frozen v5 schema: %v\n%s", err, data)
	}
	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	relation := document.Relations[0]
	if relation.Version != V5RelationVersion || relation.Schema != V5RelationSchemaURL || relation.Scope != ScopeSelf {
		t.Fatalf("relation = %#v", relation)
	}
	if _, err := AddRelation(root, "test", AddRelationInput{ID: "fast-path-verifies-fast", Type: RelationVerifies, From: v5TestCase, To: v5Criterion,
		Rationale: "The case exercises the fast path.", FromRevision: v5TestCaseR1, ToRevision: v5StoryR1, RequestID: "no-replay"}); err == nil {
		t.Fatal("duplicate relation id was accepted")
	}
}

func TestV5GoldenRelationExampleLoads(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "..", "schema", "v5", "examples", "relation.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "relation.json")
	if err := os.WriteFile(root, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var value Relation
	if err := readStrictJSON(root, &value); err != nil {
		t.Fatal(err)
	}
	if err := validateRelation(value, "checkout-refund", value.ID); err != nil {
		t.Fatalf("frozen example is invalid: %v", err)
	}
}

func TestV3RelationsKeepTheirContractBesideV5(t *testing.T) {
	// A v3 Saga still writes v3 bytes and refuses v5-only endpoints and scope.
	v3 := newSaga(t)
	if _, err := AddStory(v3, "test", storyInput("checkout", "r1", "proposed", []Criterion{{ID: "fast", Statement: "Checkout finishes promptly"}})); err != nil {
		t.Fatal(err)
	}
	if _, err := AddRelation(v3, "test", verifiesInput()); err == nil || !strings.Contains(err.Error(), `unsupported relation endpoint kind "test-case"`) {
		t.Fatalf("v3 test-case endpoint error = %v", err)
	}
	scoped := AddRelationInput{ID: "refines", Type: RelationRefines, From: v5Story, To: v5Criterion, Rationale: "narrower", Scope: ScopeSelf}
	if _, err := AddRelation(v3, "test", scoped); err == nil || !strings.Contains(err.Error(), "requires a format v5 saga") {
		t.Fatalf("v3 scope error = %v", err)
	}
	scoped.Scope = ""
	result, err := AddRelation(v3, "test", scoped)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(v3, filepath.FromSlash(result.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(RelationSchemaURL)) || bytes.Contains(data, []byte(`"scope"`)) {
		t.Fatalf("v3 relation bytes = %s", data)
	}

	// A v5 record smuggled into a v3 Saga is refused rather than reinterpreted.
	v5 := newV5Saga(t)
	if _, err := AddRelation(v5, "test", verifiesInput()); err != nil {
		t.Fatal(err)
	}
	record, err := os.ReadFile(filepath.Join(v5, "___requirements", "relations", "fast-path-verifies-fast.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v3, "___requirements", "relations", "fast-path-verifies-fast.json"), record, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(v3, "test"); err == nil || !strings.Contains(err.Error(), "requires a format v5 saga") {
		t.Fatalf("v5 relation in v3 saga error = %v", err)
	}

	// Mixed history: a v3 record inside a v5 container loads unchanged.
	v3Record, err := os.ReadFile(filepath.Join(v3, filepath.FromSlash(result.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v5, filepath.FromSlash(result.Path)), v3Record, 0o644); err != nil {
		t.Fatal(err)
	}
	document, err := Load(v5, "test")
	if err != nil {
		t.Fatal(err)
	}
	versions := map[string]int{}
	for _, relation := range document.Relations {
		versions[relation.ID] = relation.Version
	}
	if !reflect.DeepEqual(versions, map[string]int{"fast-path-verifies-fast": 5, "refines": 3}) {
		t.Fatalf("mixed relation versions = %v", versions)
	}
}

func TestV5RelationMatrixAndScope(t *testing.T) {
	root := newV5Saga(t)
	cases := []struct {
		name   string
		mutate func(*AddRelationInput)
		want   string
	}{
		{"test case requires its revision pin", func(in *AddRelationInput) { in.FromRevision = "" }, "source test-case revision pin"},
		{"criterion revision pin required", func(in *AddRelationInput) { in.ToRevision = "" }, "target criterion revision pin"},
		{"test case revision must be its own", func(in *AddRelationInput) {
			in.FromRevision = "urn:change-saga:test:test-case:other:revision:r1"
		}, "from_revision does not pin the test-case endpoint"},
		{"test case cannot carry a digest", func(in *AddRelationInput) {
			in.FromContentDigest = "sha256:" + strings.Repeat("a", 64)
		}, "cannot use a content digest"},
		{"verifies targets only criteria", func(in *AddRelationInput) { in.To = v5Story }, "criterion target"},
		{"descendants needs a deck or slide addresses/explains", func(in *AddRelationInput) { in.Scope = ScopeDescendants }, "descendants scope"},
		{"unknown scope", func(in *AddRelationInput) { in.Scope = "everything" }, "scope must be self or descendants"},
		{"cross saga", func(in *AddRelationInput) { in.From = "urn:change-saga:other:test-case:fast-path" }, "different saga"},
		{"refines pins both sides", func(in *AddRelationInput) {
			in.Type, in.From, in.FromRevision = RelationRefines, v5Story, ""
		}, "source and target story revision pins"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := verifiesInput()
			tc.mutate(&input)
			if _, err := AddRelation(root, "test", input); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
	explains := AddRelationInput{ID: "deck-explains", Type: RelationExplains, From: "urn:change-saga:test:deck:flow", To: v5Criterion,
		Rationale: "The deck walks the flow.", ToRevision: v5StoryR1, Scope: ScopeDescendants}
	if _, err := AddRelation(root, "test", explains); err != nil {
		t.Fatalf("deck descendants explains: %v", err)
	}
	supersedes := AddRelationInput{ID: "tc-supersedes", Type: RelationSupersedes, From: v5TestCase, To: "urn:change-saga:test:test-case:old-path", Rationale: "Replaces the old case."}
	if _, err := AddRelation(root, "test", supersedes); err != nil {
		t.Fatalf("test-case supersedes: %v", err)
	}
}

func TestRelationCurrencyIsDerivedFromPins(t *testing.T) {
	root := newV5Saga(t)
	if _, err := AddRelation(root, "test", verifiesInput()); err != nil {
		t.Fatal(err)
	}
	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	evaluate := func(inputs StaleInputs) RelationCurrency {
		t.Helper()
		currencies := EvaluateRelations(document, inputs)
		if len(currencies) != 1 || currencies[0].Relation != v5RelationURN {
			t.Fatalf("currencies = %#v", currencies)
		}
		return currencies[0]
	}
	codes := func(currency RelationCurrency) []string {
		values := []string{}
		for _, reason := range currency.Reasons {
			values = append(values, reason.Endpoint+":"+reason.Code)
		}
		return values
	}

	// Without the quality domain's heads a test-case link is never assumed current.
	if got := evaluate(StaleInputs{}); got.Status != CurrencyInvalid || got.Current() || !reflect.DeepEqual(codes(got), []string{"from:head_unresolved"}) {
		t.Fatalf("unresolved = %#v", got)
	}
	if relation := document.Relations[0]; !relation.Stale || !reflect.DeepEqual(relation.StaleReasons, []string{"from test-case revision head was not supplied"}) {
		t.Fatalf("load projection = %v %v", relation.Stale, relation.StaleReasons)
	}

	current := StaleInputs{}
	current.SetTestCaseHeads("test", map[string][]string{"fast-path": {v5TestCaseR1}})
	if got := evaluate(current); !got.Current() || len(got.Reasons) != 0 || got.Scope != ScopeSelf || got.Version != 5 {
		t.Fatalf("current = %#v", got)
	}

	revisedTest := StaleInputs{}
	revisedTest.SetTestCaseHeads("test", map[string][]string{"fast-path": {v5TestCaseR2}})
	got := evaluate(revisedTest)
	if got.Status != CurrencyStale || !reflect.DeepEqual(got.Reasons, []CurrencyReason{{Endpoint: "from", Code: ReasonRevisionChanged, Pinned: v5TestCaseR1, Current: []string{v5TestCaseR2}, Message: "from revision changed"}}) {
		t.Fatalf("revised test case = %#v", got)
	}

	conflicted := StaleInputs{}
	conflicted.SetTestCaseHeads("test", map[string][]string{"fast-path": {v5TestCaseR2, v5TestCaseR1}})
	if got := evaluate(conflicted); got.Status != CurrencyConflicted || !reflect.DeepEqual(codes(got), []string{"from:multiple_revision_heads"}) {
		t.Fatalf("conflicted = %#v", got)
	}

	missing := StaleInputs{}
	missing.SetTestCaseHeads("test", map[string][]string{})
	if got := evaluate(missing); got.Status != CurrencyInvalid || !reflect.DeepEqual(codes(got), []string{"from:endpoint_missing"}) {
		t.Fatalf("missing = %#v", got)
	}

	// A new story revision makes every relation pinned to the old one stale.
	if _, err := ReviseStory(root, "test", ReviseStoryInput{
		Story: v5Story, ID: "r2", Parents: []string{v5StoryR1}, Title: "Checkout", Statement: "As a buyer, I check out quickly", Priority: "high",
		AcceptanceCriteria: []Criterion{{ID: "fast", Statement: "Checkout finishes in two seconds"}}, CreatedAt: testTime.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if document, err = Load(root, "test"); err != nil {
		t.Fatal(err)
	}
	got = evaluate(current)
	if got.Status != CurrencyStale || got.Current() || !reflect.DeepEqual(got.Reasons, []CurrencyReason{{Endpoint: "to", Code: ReasonRevisionChanged, Pinned: v5StoryR1, Current: []string{"urn:change-saga:test:story:checkout:revision:r2"}, Message: "to revision changed"}}) {
		t.Fatalf("revised story = %#v", got)
	}

	if _, err := SupersedeRelation(root, "test", v5RelationURN, testTime.Add(2*time.Minute), ""); err != nil {
		t.Fatal(err)
	}
	if document, err = Load(root, "test"); err != nil {
		t.Fatal(err)
	}
	if got := evaluate(current); got.Status != CurrencySuperseded || got.Current() || len(got.Reasons) != 0 {
		t.Fatalf("superseded = %#v", got)
	}
}
