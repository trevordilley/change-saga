package requirements

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/currentrecord"
	"github.com/twentyideas/changesaga/internal/store"
)

func legacyRepinMigrationFixture(t *testing.T, stale bool) (string, string, string) {
	t.Helper()
	root := newSaga(t)
	if _, err := AddStory(root, "test", storyInput("checkout", "", "", nil)); err != nil {
		t.Fatal(err)
	}
	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	story := document.FindStory("checkout")
	dir := filepath.Join(root, filepath.FromSlash(storyPackagePath("core", "checkout")))
	first := *story.CurrentRevision
	first.ID, first.CreatedAt, first.Parents = "r1", testTime, []string{}
	second := first
	second.ID = "r2"
	second.CreatedAt = testTime.Add(time.Minute)
	second.Title = "Current checkout"
	second.Parents = []string{v5StoryR1}
	event := *story.CurrentLifecycle
	event.ID, event.CreatedAt, event.Parents = "proposed", testTime, []string{}
	type legacyRevision Revision
	type legacyEvent LifecycleEvent
	for _, member := range []struct {
		file  string
		value any
	}{{"revisions/r1.json", legacyRevision(first)}, {"revisions/r2.json", legacyRevision(second)}, {"events/proposed.json", legacyEvent(event)}} {
		path := filepath.Join(dir, filepath.FromSlash(member.file))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := store.WriteJSON(path, member.value, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(dir, currentrecord.Name)); err != nil {
		t.Fatal(err)
	}
	source := "urn:change-saga:test:fragment:design"
	digest := func(c string) string { return "sha256:" + strings.Repeat(c, 64) }
	relation := Relation{Schema: V5RelationSchemaURL, Version: 5, ID: "addresses", Type: RelationAddresses, From: source, To: v5Story, Scope: ScopeSelf, Rationale: "Original explanation remains current authored prose", FromContentDigest: digest("a"), ToRevision: v5StoryR1, State: RelationActive, CreatedAt: testTime}
	relationPath := filepath.Join(root, filepath.FromSlash(relationPath("core", "addresses")))
	if err := os.MkdirAll(filepath.Dir(relationPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(relationPath, relation, false); err != nil {
		t.Fatal(err)
	}
	targetWinner := v5Story + ":revision:r2"
	sourceWinner := digest("d")
	if stale {
		targetWinner = v5StoryR1
		sourceWinner = digest("c")
	}
	repins := []RelationRepin{
		{ID: "z-earlier", CreatedAt: testTime.Add(time.Minute), FromContentDigest: digest("b")},
		{ID: "a-tie", CreatedAt: testTime.Add(2 * time.Minute), ToRevision: v5Story + ":revision:r2"},
		{ID: "b-tie", CreatedAt: testTime.Add(2 * time.Minute), ToRevision: targetWinner},
		{ID: "a-partial-later", CreatedAt: testTime.Add(3 * time.Minute), FromContentDigest: sourceWinner},
	}
	for _, repin := range repins {
		repin.Schema, repin.Version, repin.Relation, repin.Rationale = RelationRepinSchemaURL, 5, "urn:change-saga:test:relation:addresses", "Re-read existing evidence"
		path := filepath.Join(root, filepath.FromSlash(repinPath("core", "addresses", repin.ID)))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := store.WriteJSON(path, repin, false); err != nil {
			t.Fatal(err)
		}
	}
	commitCurrencyFixture(t, root)
	return root, source, digest("d")
}

func TestLegacyRepinMigrationPreservesEffectivePinsAndCurrency(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "stale"}[stale], func(t *testing.T) {
			root, source, currentDigest := legacyRepinMigrationFixture(t, stale)
			before, err := Load(root, "test")
			if err != nil {
				t.Fatal(err)
			}
			relation := before.relation("addresses").Confirmed()
			inputs := StaleInputs{CurrentContentDigests: map[string]string{source: currentDigest}}
			currency := NewRelationEvaluator(before).Evaluate(*before.relation("addresses"), inputs)
			if (currency.Status == CurrencyCurrent) == stale {
				t.Fatalf("invalid legacy test premise: %+v", currency)
			}
			pins, err := currentrecord.AuditPins(root)
			if err != nil {
				t.Fatal(err)
			}
			expectedTo, err := pins.Resolve(relation.ToRevision)
			if err != nil {
				t.Fatal(err)
			}
			if err := currentrecord.Normalize(root); err != nil {
				t.Fatal(err)
			}
			after, err := Load(root, "test")
			if err != nil {
				t.Fatal(err)
			}
			got := after.relation("addresses")
			if got.FromContentDigest != relation.FromContentDigest || got.ToRevision != expectedTo || got.Rationale != relation.Rationale || len(got.Repins) != 0 {
				t.Fatalf("effective pins changed: before %+v, after %+v", relation, got)
			}
			afterCurrency := NewRelationEvaluator(after).Evaluate(*got, inputs)
			if currency.Status != afterCurrency.Status {
				t.Fatalf("migration refreshed or staled evidence: before %+v after %+v", currency, afterCurrency)
			}
			var beforeReasons, afterReasons []string
			for _, r := range currency.Reasons {
				beforeReasons = append(beforeReasons, r.Endpoint+":"+r.Code)
			}
			for _, r := range afterCurrency.Reasons {
				afterReasons = append(afterReasons, r.Endpoint+":"+r.Code)
			}
			if !reflect.DeepEqual(beforeReasons, afterReasons) {
				t.Fatalf("currency reasons changed: %v -> %v", beforeReasons, afterReasons)
			}
			path := filepath.Join(root, filepath.FromSlash(relationPath("core", "addresses")))
			data, _ := os.ReadFile(path)
			if err := currentrecord.Normalize(root); err != nil {
				t.Fatal(err)
			}
			again, _ := os.ReadFile(path)
			if string(data) != string(again) {
				t.Fatal("normalization was not idempotent")
			}
		})
	}
}

func TestLegacyRepinFoldDoesNotResolveCompetingContentHeads(t *testing.T) {
	root, _, _ := legacyRepinMigrationFixture(t, true)
	dir := filepath.Join(root, filepath.FromSlash(storyPackagePath("core", "checkout")))
	data, err := os.ReadFile(filepath.Join(dir, "revisions", "r2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fork map[string]any
	if err := json.Unmarshal(data, &fork); err != nil {
		t.Fatal(err)
	}
	fork["id"], fork["title"] = "other", "Independently authored checkout"
	if err := store.WriteJSON(filepath.Join(dir, "revisions", "other.json"), fork, false); err != nil {
		t.Fatal(err)
	}
	relation := filepath.Join(root, filepath.FromSlash(relationPath("core", "addresses")))
	before, _ := os.ReadFile(relation)
	if err := currentrecord.Normalize(root); err == nil || !strings.Contains(err.Error(), "competing") {
		t.Fatalf("competing content heads: %v", err)
	}
	after, _ := os.ReadFile(relation)
	if string(before) != string(after) {
		t.Fatal("failed head resolution changed relation pins")
	}
	if _, err := os.Stat(filepath.Join(dir, currentrecord.Name)); !os.IsNotExist(err) {
		t.Fatal("a content head was silently chosen")
	}
	if _, err := os.Stat(filepath.Join(dir, "revisions", "other.json")); err != nil {
		t.Fatal("independent content was discarded")
	}
}

func commitCurrencyFixture(t *testing.T, root string) string {
	t.Helper()
	run := func(dir string, args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Saga test", "-c", "user.email=saga@example.test"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run(filepath.Dir(root), "init", "--quiet")
	run(root, "add", ".")
	run(root, "commit", "--quiet", "--allow-empty", "-m", "Preserve authored fixture content in Git")
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestCurrentCurrencyUsesGitWithoutRefreshingEvidencePins(t *testing.T) {
	root := newSaga(t)
	input := storyInput("checkout", "", "", []Criterion{{ID: "fast", Statement: "Checkout completes promptly"}})
	if _, err := AddStory(root, "test", input); err != nil {
		t.Fatal(err)
	}
	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	original := document.FindStory("checkout").CurrentRevision
	pin := ContentPin(v5Story, original.ID)
	source := "urn:change-saga:test:fragment:design"
	digest := "sha256:" + strings.Repeat("d", 64)
	for id, target := range map[string]string{"story": v5Story, "criterion": v5Criterion} {
		if _, err := AddRelation(root, "test", AddRelationInput{Feature: "core", ID: id, Type: RelationAddresses, From: source, To: target, FromContentDigest: digest, ToRevision: pin, Rationale: "Confirmed exact authored obligation"}); err != nil {
			t.Fatal(err)
		}
	}
	commitCurrencyFixture(t, root)
	revise := ReviseStoryInput{Story: v5Story, Title: original.Title, Statement: original.Statement, Overview: original.Overview, Priority: "low", Personas: original.Personas, Citations: original.Citations, AcceptanceCriteria: original.AcceptanceCriteria}
	if _, err := ReviseStory(root, "test", revise); err != nil {
		t.Fatal(err)
	}
	document, err = Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	inputs := StaleInputs{CurrentContentDigests: map[string]string{source: digest}}
	for _, id := range []string{"story", "criterion"} {
		relation := document.relation(id)
		currency := EvaluateRelation(document, *relation, inputs)
		if relation.ToRevision != pin || currency.Status != CurrencyCurrent || len(currency.CarriedForward) != 1 {
			t.Fatalf("metadata-only change aged or refreshed existing evidence: %+v %+v", relation, currency)
		}
	}
	// A real obligation edit must still stale the story pin. Removing the
	// criterion must stay visible after its old record leaves current storage.
	revise.Statement = "As a buyer I can choose a different checkout path"
	revise.AcceptanceCriteria = nil
	if _, err := ReviseStory(root, "test", revise); err != nil {
		t.Fatal(err)
	}
	document, err = Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"story", "criterion"} {
		relation := document.relation(id)
		currency := EvaluateRelation(document, *relation, inputs)
		expected := CurrencyStale
		if id == "criterion" {
			expected = CurrencyInvalid
		}
		if relation.ToRevision != pin || currency.Status != expected {
			t.Fatalf("changed or removed obligation inherited evidence: %+v %+v", relation, currency)
		}
	}
	if currency := EvaluateRelation(document, *document.relation("criterion"), inputs); len(currency.Reasons) != 1 || currency.Reasons[0].Code != ReasonCriterionRemoved {
		t.Fatalf("removed stable criterion looked current: %+v", currency)
	}
}

func TestSavedCurrencyCannotReadFutureStoryContent(t *testing.T) {
	root, source, digest := legacyRepinMigrationFixture(t, false)
	if err := currentrecord.Normalize(root); err != nil {
		t.Fatal(err)
	}
	savedCommit := commitCurrencyFixture(t, root)
	saved, err := LoadWithOptions(root, "test", LoadOptions{GitSource: &currentrecord.GitSource{Repository: filepath.Dir(root), SagaPath: filepath.Base(root), Commit: savedCommit}})
	if err != nil {
		t.Fatal(err)
	}
	current := saved.FindStory("checkout").CurrentRevision
	if _, err := ReviseStory(root, "test", ReviseStoryInput{Story: v5Story, Title: current.Title, Statement: current.Statement, Overview: current.Overview, Priority: "changed metadata", Personas: current.Personas, Citations: current.Citations, AcceptanceCriteria: current.AcceptanceCriteria}); err != nil {
		t.Fatal(err)
	}
	commitCurrencyFixture(t, root)
	future, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	futurePin := ContentPin(v5Story, future.FindStory("checkout").CurrentRevision.ID)
	relation := *saved.relation("addresses")
	relation.ToRevision = futurePin
	inputs := StaleInputs{CurrentContentDigests: map[string]string{source: digest}}
	if got := EvaluateRelation(saved, relation, inputs); got.Status != CurrencyStale {
		t.Fatalf("saved view consulted future Git content: %+v", got)
	}
	unbounded := saved
	unbounded.gitSource = nil
	if got := EvaluateRelation(unbounded, relation, inputs); got.Status != CurrencyCurrent {
		t.Fatalf("fixture does not distinguish the future equal obligation: %+v", got)
	}
}
