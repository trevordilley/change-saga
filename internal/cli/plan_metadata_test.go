package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

func TestPlanSetEditsMetadataTogetherAndPreservesContent(t *testing.T) {
	root := newPlanFixture(t).root
	run(t, Plan, "add-phase", "--plan", "first", "--id", "gate", "--title", "Design review", "--checkpoint", root)
	run(t, Plan, "expect", "--plan", "first", "--checkpoint", "gate", "--id", "access", "--statement", "Access is approved", root)
	beforePlan := loadedPlan(t, root, "first")
	before := entityFiles(t, root)
	args := []string{"set", "--plan", beforePlan.Target, "--title", "Updated scope", "--objective", "Explain the approved access levels.", "--checkpoint-passer", "person", "--json"}
	var out bytes.Buffer
	if err := Plan(context.Background(), append(append([]string{}, args...), "--dry-run", root), &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, entityFiles(t, root)) {
		t.Fatal("dry-run changed files")
	}
	out.Reset()
	if err := Plan(context.Background(), append(args, root), &out); err != nil {
		t.Fatal(err)
	}
	var result contentEditOutput
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || !result.OK || len(result.Paths) != 2 {
		t.Fatalf("result: %s (%v)", &out, err)
	}
	after := loadedPlan(t, root, "first")
	want := beforePlan.PlanManifest
	want.Title, want.CheckpointPasser = "Updated scope", "person"
	if !reflect.DeepEqual(after.PlanManifest, want) || after.Deck.Title != want.Title || after.Deck.Objective != "Explain the approved access levels." {
		t.Fatalf("metadata not updated coherently: %+v / %+v", after.PlanManifest, after.Deck.DeckManifest)
	}
	delete(before, filepath.Join(saga.PlansDir, "first.plan", "plan.json"))
	delete(before, filepath.FromSlash(beforePlan.Deck.Path))
	remaining := entityFiles(t, root)
	delete(remaining, filepath.Join(saga.PlansDir, "first.plan", "plan.json"))
	delete(remaining, filepath.FromSlash(beforePlan.Deck.Path))
	if !reflect.DeepEqual(before, remaining) {
		t.Fatal("metadata edit changed unrelated authored data")
	}
	unchanged := entityFiles(t, root)
	out.Reset()
	if err := Plan(context.Background(), append(args, root), &out); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || !result.Replayed || !reflect.DeepEqual(unchanged, entityFiles(t, root)) {
		t.Fatalf("same-value edit should not write: %s (%v)", &out, err)
	}
}

func TestPlanSetRejectsInvalidMetadataWithoutWrites(t *testing.T) {
	root := newPlanFixture(t).root
	for _, flags := range [][]string{
		{}, {"--title", " "}, {"--objective", ""}, {"--objective", strings.Repeat("界", 241)},
		{"--title", "New", "--checkpoint-passer", "anyone"},
		{"--plan", "urn:change-saga:other:plan:first", "--title", "Cross Saga"},
	} {
		before := entityFiles(t, root)
		args := append([]string{"set", "--plan", "first"}, flags...)
		var out bytes.Buffer
		if err := Plan(context.Background(), append(args, "--json", root), &out); err == nil {
			t.Fatalf("accepted %v", flags)
		}
		if !json.Valid(out.Bytes()) || !reflect.DeepEqual(before, entityFiles(t, root)) {
			t.Fatalf("invalid edit changed data or emitted invalid JSON: %v / %s", flags, &out)
		}
	}
}

func TestPlanSetPreservesOmittedFieldsAndRefusesDelivered(t *testing.T) {
	root := newPlanFixture(t).root
	before := loadedPlan(t, root, "first")
	run(t, Plan, "set", "--plan", "first", "--objective", "A revised objective", root)
	after := loadedPlan(t, root, "first")
	if !reflect.DeepEqual(before.PlanManifest, after.PlanManifest) || before.Deck.Title != after.Deck.Title {
		t.Fatal("objective-only edit changed Plan fields")
	}
	run(t, Plan, "set", "--plan", "first", "--title", "A revised title", root)
	if loadedPlan(t, root, "first").Deck.Objective != "A revised objective" {
		t.Fatal("title-only edit changed objective")
	}
	// A delivered Plan stays immutable even for display metadata.
	path := filepath.Join(root, saga.PlansDir, "first.plan", "plan.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["delivered_at"] = "2026-11-01T00:00:00Z"
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	files := entityFiles(t, root)
	if msg := refused(t, planCommand, "set", "--plan", "first", "--title", "Too late", root); !strings.Contains(msg, "delivered") {
		t.Fatal(msg)
	}
	if !reflect.DeepEqual(files, entityFiles(t, root)) {
		t.Fatal("delivered edit wrote data")
	}
}
