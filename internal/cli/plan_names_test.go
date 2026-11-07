package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPlanNamesBuildAHierarchyThatTasksCheckOff(t *testing.T) {
	t.Parallel()
	root := newPlanFixture(t).root
	run(t, Plan, "add-name", "--plan", "first", "--id", "model", "--title", "Model", root)
	run(t, Plan, "add-name", "--plan", "first", "--id", "record", "--parent", "model", "--title", "Record", root)
	run(t, Plan, "add-name", "--plan", "first", "--id", "view", "--title", "View", "--link", "model=shows", root)
	run(t, Plan, "add-name", "--plan", "first", "--id", "spare", "--title", "Spare", root)
	run(t, Task, "add", "--plan", "first", "--id", "build", "--title", "Build", "--story", planStoryURN, "--name", "record", root)
	run(t, Task, "add", "--plan", "first", "--id", "show", "--title", "Show", "--story", planStoryURN, "--name", "view", root)
	run(t, Task, "add", "--plan", "first", "--id", "polish", "--title", "Polish", "--story", planStoryURN, root)
	run(t, Task, "done", "--plan", "first", "--task", "build", root)

	plan := loadedPlan(t, root, "first")
	if name := plan.FindName("view"); name == nil || len(name.Links) != 1 || name.Links[0].To != "model" || name.Links[0].Label != "shows" {
		t.Fatalf("view = %#v", name)
	}
	var result planNamesResult
	if err := json.Unmarshal([]byte(run(t, Plan, "names", "--plan", "first", "--json", root)), &result); err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, row := range result.Names {
		states[row.ID] = row.State
	}
	if want := map[string]string{"model": "done", "record": "done", "view": "planned", "spare": "unreached"}; !reflect.DeepEqual(states, want) {
		t.Fatalf("states = %v, want %v", states, want)
	}
	if !reflect.DeepEqual(result.Unnamed, []string{"polish"}) || !reflect.DeepEqual(result.Unreached, []string{"spare"}) {
		t.Fatalf("unnamed %v unreached %v", result.Unnamed, result.Unreached)
	}
	var queried struct {
		Data struct {
			Plan struct {
				Names planNamesResult `json:"names"`
			} `json:"plan"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(run(t, Query, "plan", "--saga", root, "--plan", "first")), &queried); err != nil {
		t.Fatal(err)
	}
	if names := queried.Data.Plan.Names; !reflect.DeepEqual(names.Roots, []string{"model", "view", "spare"}) || len(names.Names) != 4 {
		t.Fatalf("query plan names = %#v", names)
	}
	for _, row := range queried.Data.Plan.Names.Names {
		if row.ID == "record" && row.Parent != "model" || row.ID == "model" && !reflect.DeepEqual(row.Children, []string{"record"}) {
			t.Fatalf("query plan row = %#v", row)
		}
	}
	text := run(t, Plan, "names", "--plan", "first", root)
	for _, want := range []string{"✓ Model (model)  1/1 done", "  ✓ Record (record)", "Unexpected work, on no name: polish", "Possibly unneeded, no task reaches it: Spare (spare)"} {
		if !strings.Contains(text, want) {
			t.Errorf("plan names lacks %q:\n%s", want, text)
		}
	}
	guide := run(t, Plan, "guide", "--plan", "first", root)
	if !strings.Contains(guide, "Unexpected work, on no name: polish") || !strings.Contains(guide, "Possibly unneeded, no task reaches it: Spare") {
		t.Fatalf("plan guide does not report the gaps:\n%s", guide)
	}
	assertValid(t, root)
}

func TestPlanNamesRefuseWhatWouldLeaveThemBroken(t *testing.T) {
	t.Parallel()
	root := newPlanFixture(t).root
	run(t, Plan, "add-name", "--plan", "first", "--id", "model", "--title", "Model", root)
	run(t, Plan, "add-name", "--plan", "first", "--id", "record", "--parent", "model", "--title", "Record", root)
	run(t, Plan, "add-name", "--plan", "first", "--id", "view", "--title", "View", "--link", "record", root)
	run(t, Task, "add", "--plan", "first", "--id", "build", "--title", "Build", "--story", planStoryURN, "--name", "model", root)
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"add-name", "--plan", "first", "--id", "x", "--title", "X", "--parent", "nope", root}, `its parent "nope" is not a name of this Plan`},
		{[]string{"add-name", "--plan", "first", "--id", "x", "--title", "X", "--link", "nope", root}, `it links to "nope"`},
		{[]string{"set-name", "--plan", "first", "--id", "model", "--parent", "record", root}, "parent cycle"},
		{[]string{"remove-name", "--plan", "first", "--id", "model", root}, "names beneath it: record"},
		{[]string{"remove-name", "--plan", "first", "--id", "record", root}, "names linking to it: view"},
		{[]string{"add-name", "--plan", "first", "--id", "x", "--title", "X", "--record", "urn:change-saga:app:term:nope", root}, "does not exist"},
	} {
		if message := refused(t, planCommand, test.args...); !strings.Contains(message, test.want) {
			t.Errorf("%v: %q, want %q", test.args, message, test.want)
		}
	}
	if message := refused(t, taskCommand, "set", "--plan", "first", "--task", "build", "--name", "nope", root); !strings.Contains(message, `it names "nope"`) {
		t.Errorf("task set --name nope: %q", message)
	}
	run(t, Plan, "set-name", "--plan", "first", "--id", "view", "--link", "", root)
	run(t, Plan, "set-name", "--plan", "first", "--id", "record", "--parent", "", root)
	run(t, Task, "set", "--plan", "first", "--task", "build", "--name", "", root)
	run(t, Plan, "remove-name", "--plan", "first", "--id", "model", root)
	if plan := loadedPlan(t, root, "first"); plan.FindName("model") != nil || len(plan.FindTask("build").Names) != 0 {
		t.Fatalf("names = %#v", plan.Names)
	}
}

func TestPlanNamePointingAtARemovedTermIsReportedDangling(t *testing.T) {
	t.Parallel()
	root := newPlanFixture(t).root
	var output bytes.Buffer
	if err := Term(context.Background(), []string{"add", "--id", "cart", "--name", "Cart", "--definition", "What a buyer collects.", root}, &output); err != nil {
		t.Fatalf("term add: %v\n%s", err, output.String())
	}
	run(t, Plan, "add-name", "--plan", "first", "--id", "cart", "--title", "Cart", "--record", "urn:change-saga:app:term:cart", root)
	output.Reset()
	if err := Term(context.Background(), []string{"remove", "--term", "urn:change-saga:app:term:cart", "--json", root}, &output); err != nil {
		t.Fatalf("term remove: %v\n%s", err, output.String())
	}
	var removed removalOutput
	if err := json.Unmarshal(output.Bytes(), &removed); err != nil {
		t.Fatal(err)
	}
	for _, adjust := range removed.Adjust {
		if adjust.Record == "urn:change-saga:app:plan:first:name:cart" && adjust.Kind == "plan_name" {
			return
		}
	}
	t.Fatalf("the name is not listed to adjust: %#v", removed.Adjust)
}
