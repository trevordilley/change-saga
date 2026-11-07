package saga

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func testName(id, parent string, at int, links ...string) NameManifest {
	name := NameManifest{Schema: NameSchemaURL, Version: PlanVersion, ID: id, Title: strings.ToUpper(id), Parent: parent,
		CreatedAt: time.Date(2026, 11, 7, 0, 0, at, 0, time.UTC)}
	for _, link := range links {
		name.Links = append(name.Links, NameLink{To: link})
	}
	return name
}

func TestPlanNameProblemsRefuseMissingParentsLinksCyclesAndTaskNames(t *testing.T) {
	t.Parallel()
	names := []NameManifest{testName("a", "c", 0), testName("b", "a", 1), testName("c", "b", 2), testName("d", "gone", 3, "nowhere")}
	tasks := []TaskManifest{{ID: "t1", Names: []string{"a", "a"}}, {ID: "t2", Names: []string{"missing"}}}
	var messages []string
	for _, problem := range PlanNameProblems(names, tasks) {
		messages = append(messages, problem.String())
	}
	joined := strings.Join(messages, "\n")
	for _, want := range []string{
		`name d: its parent "gone" is not a name of this Plan`,
		`name d: it links to "nowhere", which is not a name of this Plan`,
		"parent cycle: a -> c -> b -> a",
		`task t1: it names "a" more than once`,
		`task t2: it names "missing", which is not a name of this Plan`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems lack %q:\n%s", want, joined)
		}
	}
	if count := strings.Count(joined, "parent cycle"); count != 1 {
		t.Errorf("a cycle is reported once, got %d:\n%s", count, joined)
	}
}

func TestValidateNameManifestRefusesSelfReferencesAndForeignRecords(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		change func(*NameManifest)
		want   string
	}{
		{func(n *NameManifest) { n.Parent = n.ID }, "its own parent"},
		{func(n *NameManifest) { n.Links = []NameLink{{To: n.ID}} }, "link to itself"},
		{func(n *NameManifest) { n.Links = []NameLink{{To: "b"}, {To: "b"}} }, "more than once"},
		{func(n *NameManifest) { n.Record = "urn:change-saga:app:story:s" }, "term, component, or system"},
		{func(n *NameManifest) { n.Title = " " }, "title is required"},
	} {
		name := testName("a", "", 0)
		test.change(&name)
		if err := ValidateNameManifest("app", name); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("error = %v, want %q", err, test.want)
		}
	}
	name := testName("a", "", 0)
	name.Record = "urn:change-saga:app:component:checkout"
	if err := ValidateNameManifest("app", name); err != nil {
		t.Fatalf("a component pointer is valid: %v", err)
	}
}

func TestPlanNamesRollsTaskStatesUpAndReportsGaps(t *testing.T) {
	t.Parallel()
	names := []NameManifest{
		testName("model", "", 0), testName("record", "model", 1), testName("links", "model", 2),
		testName("view", "", 3), testName("tree", "view", 4),
		testName("extra", "", 5), testName("extra-child", "extra", 6),
	}
	tasks := []TaskManifest{
		{ID: "t1", State: TaskDone, Names: []string{"record", "model"}},
		{ID: "t2", State: TaskInProgress, Names: []string{"links"}},
		{ID: "t3", State: TaskPlanned, Names: []string{"tree"}},
		{ID: "t4", State: TaskPlanned},
	}
	view := PlanNames(names, tasks)
	if !reflect.DeepEqual(view.Roots, []string{"model", "view", "extra"}) {
		t.Fatalf("roots = %v, want the order they were added", view.Roots)
	}
	model := view.Progress["model"]
	if model.All != 2 || model.Done != 1 || model.State != NameUnderWay {
		t.Fatalf("model = %+v: a Task naming a name and its parent counts once, and a started Task makes it under way", model)
	}
	if !reflect.DeepEqual(model.Tasks, []string{"t1"}) || !reflect.DeepEqual(model.Children, []string{"record", "links"}) {
		t.Fatalf("model tasks %v children %v", model.Tasks, model.Children)
	}
	if got := view.Progress["record"]; got.State != NameDone || got.Depth != 1 {
		t.Fatalf("record = %+v", got)
	}
	if got := view.Progress["view"]; got.State != NamePlanned {
		t.Fatalf("view = %+v", got)
	}
	if !reflect.DeepEqual(view.Unnamed, []string{"t4"}) {
		t.Fatalf("unnamed = %v", view.Unnamed)
	}
	if !reflect.DeepEqual(view.Unreached, []string{"extra"}) {
		t.Fatalf("unreached = %v: a branch no Task reaches is reported once, at its top", view.Unreached)
	}
	if view.Progress["extra-child"].Depth != 1 {
		t.Fatalf("depth inside an unreached branch is still set")
	}
	if got := view.Ancestry(names, "tree"); !reflect.DeepEqual(got, []string{"view", "tree"}) {
		t.Fatalf("ancestry = %v", got)
	}
	if PlanNames(nil, tasks) != nil {
		t.Fatal("a Plan without names reports nothing")
	}
}
