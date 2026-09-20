package grammar

import (
	"sort"
	"strings"
	"testing"
)

func TestCommandsAreUniqueSortedAndSelfConsistent(t *testing.T) {
	values := Commands()
	if !sort.SliceIsSorted(values, func(i, j int) bool { return values[i].Name < values[j].Name }) {
		t.Fatal("commands are published in stable name order")
	}
	seen := map[string]bool{}
	for _, command := range values {
		if seen[command.Name] {
			t.Fatalf("command %q is declared twice", command.Name)
		}
		seen[command.Name] = true
		if !strings.HasPrefix(command.Usage, "change-saga "+command.Name) {
			t.Errorf("%s usage %q does not start with its own name", command.Name, command.Usage)
		}
		if command.Status != StatusImplemented && command.Status != StatusPlanned {
			t.Errorf("%s has unknown status %q", command.Name, command.Status)
		}
		flags := map[string]bool{}
		for _, flag := range command.Flags {
			if flags[flag.Name] {
				t.Errorf("%s declares --%s twice", command.Name, flag.Name)
			}
			flags[flag.Name] = true
			if flag.Description == "" {
				t.Errorf("%s --%s has no description", command.Name, flag.Name)
			}
		}
	}
}

func TestEveryResourceWriterIsADeclaredCommand(t *testing.T) {
	for _, resource := range Resources() {
		if len(resource.Writers) == 0 {
			t.Errorf("resource %s names no writer", resource.Kind)
		}
		for _, writer := range resource.Writers {
			if _, ok := Lookup(writer); !ok {
				t.Errorf("resource %s names undeclared writer %q", resource.Kind, writer)
			}
		}
	}
}

func TestRelationMatrixCoversEveryPersistedRelationType(t *testing.T) {
	want := []string{"addresses", "conflicts_with", "explains", "implements", "refines", "supersedes", "verifies"}
	got := []string{}
	for _, relation := range Relations() {
		got = append(got, relation.Type)
		if len(relation.Sources) == 0 || len(relation.Targets) == 0 || len(relation.Scopes) == 0 || relation.Meaning == "" {
			t.Errorf("relation %s is under-specified: %#v", relation.Type, relation)
		}
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("relation matrix types = %v", got)
	}
}

func TestInvokeRefusesShapesTheSpecDoesNotDeclare(t *testing.T) {
	if _, err := Invoke("relation delete", "s.saga"); err == nil {
		t.Fatal("an undeclared command must be refused")
	}
	if _, err := Invoke("relation add", "s.saga", V("weight", "1")); err == nil {
		t.Fatal("an undeclared flag must be refused")
	}
	if _, err := Invoke("relation add", "s.saga", V("to", "a"), V("to", "b")); err == nil {
		t.Fatal("a repeated non-repeatable flag must be refused")
	}
	if _, err := Invoke("story revise", "s.saga", V("parent", "a"), V("parent", "b")); err != nil {
		t.Fatalf("a repeatable flag may repeat: %v", err)
	}
}

func TestInvokeSeparatesKnownValuesFromAuthorInputs(t *testing.T) {
	invocation := MustInvoke("relation add", "s.saga", V("type", "addresses"), V("to", "urn:c"), V("from", ""), V("json", "true"))
	if invocation.Status != StatusImplemented || invocation.Usage == "" {
		t.Fatalf("invocation carries its grammar entry: %#v", invocation)
	}
	known := map[string]string{}
	for _, argument := range invocation.Arguments {
		known[argument.Flag] = argument.Value
	}
	if known["type"] != "addresses" || known["to"] != "urn:c" || known["json"] != "true" {
		t.Fatalf("arguments = %#v", invocation.Arguments)
	}
	inputs := []string{}
	for _, input := range invocation.Inputs {
		inputs = append(inputs, input.Flag)
	}
	if strings.Join(inputs, ",") != "feature,id,from,rationale" {
		t.Fatalf("required flags without values and empty supplied values become inputs in declaration order: %v", inputs)
	}
	argv := strings.Join(invocation.Argv, " ")
	if argv != "change-saga relation add --feature ID --id ID --type addresses --from URN --to urn:c --rationale TEXT --json s.saga" {
		t.Fatalf("argv = %q", argv)
	}
}

func TestAxisRulesNameEveryAxisOnce(t *testing.T) {
	want := "prototype,ux,ui,technical,quality,implementation"
	got := []string{}
	for _, rule := range AxisRules() {
		got = append(got, rule.Axis)
	}
	if strings.Join(got, ",") != want {
		t.Fatalf("axis rules = %v", got)
	}
}
