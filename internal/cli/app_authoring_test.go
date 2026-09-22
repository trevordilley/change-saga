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

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/nextaction"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// appStatusFacts are the app-level status facts.
type appStatusFacts struct {
	Features []struct {
		ID      string   `json:"id"`
		Stories []string `json:"stories"`
		GatedBy []string `json:"gated_by"`
	} `json:"features"`
	Personas []struct {
		Persona  string   `json:"persona"`
		State    string   `json:"state"`
		ServedBy []string `json:"served_by"`
		Stories  []string `json:"stories"`
		Gap      bool     `json:"gap"`
	} `json:"personas"`
	PersonaOrphans []struct {
		Personas []string `json:"personas"`
		Stories  []string `json:"stories"`
	} `json:"persona_orphans"`
	Stories []struct {
		Story        string   `json:"story"`
		Feature      string   `json:"feature"`
		State        string   `json:"state"`
		Personas     []string `json:"personas"`
		GatedBy      []string `json:"gated_by"`
		Availability string   `json:"availability"`
	} `json:"stories"`
}

// appStatus is the slice of status --json the app-level tests read; the app
// facts are top-level keys beside the coverage report.
type appStatus struct {
	appStatusFacts
	NextActions []struct {
		ID       string `json:"id"`
		Feature  string `json:"feature"`
		Resource string `json:"resource"`
		Command  *struct {
			Argv []string `json:"argv"`
		} `json:"command"`
		Question *struct {
			Options []struct {
				Commands []struct {
					Argv []string `json:"argv"`
				} `json:"commands"`
			} `json:"options"`
		} `json:"question"`
	} `json:"next_actions"`
}

// runAppStatus runs status --json. Status exits non-zero while readiness is
// blocked, which these fixtures always are, so only the document matters.
func runAppStatus(t *testing.T, root string, repo ...string) (appStatus, string) {
	t.Helper()
	args := []string{"--json"}
	if len(repo) > 0 {
		args = append(args, "--repo", repo[0])
	}
	var output bytes.Buffer
	_ = Status(context.Background(), append(againstMain(args), root), &output)
	var status appStatus
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatalf("status --json: %v\n%s", err, output.String())
	}
	if raw := map[string]json.RawMessage{}; json.Unmarshal(output.Bytes(), &raw) != nil || raw["features"] == nil || raw["personas"] == nil || raw["persona_orphans"] == nil {
		t.Fatalf("status --json has no app facts:\n%s", output.String())
	}
	return status, output.String()
}

func mustLiving(t *testing.T, name string, run func(context.Context, []string, *bytes.Buffer) error, args ...string) livingMutationOutput {
	t.Helper()
	var output bytes.Buffer
	if err := run(context.Background(), append(args, "--json"), &output); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, output.String())
	}
	return decodeLivingOutput(t, &output)
}

func storyCommand(ctx context.Context, args []string, out *bytes.Buffer) error {
	return Story(ctx, args, out)
}
func featureCommand(ctx context.Context, args []string, out *bytes.Buffer) error {
	return Feature(ctx, args, out)
}
func personaCommand(ctx context.Context, args []string, out *bytes.Buffer) error {
	return Persona(ctx, args, out)
}
func flagCommand(ctx context.Context, args []string, out *bytes.Buffer) error {
	return FeatureFlag(ctx, args, out)
}
func relationCommand(ctx context.Context, args []string, out *bytes.Buffer) error {
	return Relation(ctx, args, out)
}

// addStory adds a proposed story with one criterion, "fast", to feature.
func addStory(t *testing.T, root, feature, id string, personas ...string) livingMutationOutput {
	t.Helper()
	args := []string{"add", root, "--feature", feature, "--id", id, "--revision", "r1", "--event", "proposed",
		"--title", id, "--statement", "As a user I can " + id, "--priority", "must", "--criterion", "fast=It finishes promptly"}
	for _, persona := range personas {
		args = append(args, "--persona", persona)
	}
	return mustLiving(t, "story add", storyCommand, args...)
}

func acceptStory(t *testing.T, root, story string) {
	t.Helper()
	mustLiving(t, "story set-state", storyCommand, "set-state", root, "--story", story, "--event", "accepted",
		"--parent", story+":event:proposed", "--state", "accepted")
}

func TestInitCreatesOnlyTheAppWithEveryOverviewPartAGap(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	writeFile(t, filepath.Join(repo, "README.md"), "base\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	root := filepath.Join(t.TempDir(), "app.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	// Both first runs are named: covering a change, and documenting existing
	// code by observing HEAD.
	for _, want := range []string{"To cover a change", "add-deck", "cover --against main", "status --against main", "To document existing code",
		"change-saga status " + root, "overview set-pitch", "--lines RANGES | --file", "optional"} {
		if !strings.Contains(text, want) {
			t.Fatalf("init output does not lead to %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "cover --against main") > strings.Index(text, "optional") {
		t.Fatalf("init leads with covering the change before anything optional:\n%s", text)
	}
	for _, absent := range []string{"overview.fragment", applayout.OverviewDir, applayout.FeaturesDir, applayout.PersonasDir, "___requirements"} {
		if _, err := os.Stat(filepath.Join(root, absent)); !os.IsNotExist(err) {
			t.Fatalf("init created %s: %v", absent, err)
		}
	}
	assertValid(t, root)
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	// The overview's name is the manifest title; its pitch, description, and
	// terms are gaps until they are written.
	if len(document.Features) != 0 || len(document.Section.Fragments) != 0 || document.OverviewPart(applayout.OverviewPitch) != nil {
		t.Fatalf("init app = features %d, fragments %#v", len(document.Features), document.Section.Fragments)
	}

	// Observing the fresh app, the path that documents existing code, has no
	// change to cover, and its loop has the overview to take as growth.
	var status bytes.Buffer
	if err := Status(context.Background(), []string{"--repo", repo, "--allow-repository-mismatch", root}, &status); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Next actions: none. Observing, there is no change to cover", "for the app as it is, the overview and vocabulary first",
		"[overview] the overview has no elevator pitch", "$ " + shellJoin([]string{"change-saga", "overview", "set-pitch", "--text", "TEXT", root}), "[terms] no term is defined yet"} {
		if !strings.Contains(status.String(), want) {
			t.Fatalf("observed status lacks %q:\n%s", want, status.String())
		}
	}
	if strings.Contains(status.String(), "Every changed line is covered") || strings.Contains(status.String(), "most valuable to this change") {
		t.Fatalf("observing speaks of a change it does not have:\n%s", status.String())
	}
}

func TestFeatureAddCreatesReplaysAndRejectsDuplicates(t *testing.T) {
	root := newAuthoredSaga(t)
	args := []string{"add", root, "--id", "billing", "--title", "Billing", "--description", "Invoices and payments", "--request-id", "billing-request"}
	created := mustLiving(t, "feature add", featureCommand, args...)
	if !created.OK || created.Replayed || created.Resource != "urn:change-saga:atomic:feature:billing" || created.Path != "___features/billing.feature" {
		t.Fatalf("feature add = %#v", created)
	}
	var manifest applayout.FeatureManifest
	if err := applayout.ReadStrictJSON(filepath.Join(root, "___features", "billing.feature", "feature.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "billing" || manifest.Title != "Billing" || manifest.Description != "Invoices and payments" || manifest.RequestID != "billing-request" {
		t.Fatalf("feature.json = %#v", manifest)
	}
	if replay := mustLiving(t, "feature add", featureCommand, args...); !replay.OK || !replay.Replayed || replay.Resource != created.Resource {
		t.Fatalf("identical feature add did not replay: %#v", replay)
	}

	var output bytes.Buffer
	changed := append(append([]string{}, args...), "--title", "Payments")
	if err := Feature(context.Background(), changed, &output); err == nil || !strings.Contains(err.Error(), `feature "billing" already exists`) {
		t.Fatalf("a changed replay must be refused: %v", err)
	}
	if err := Feature(context.Background(), []string{"add", root, "--id", testFeature, "--title", "Core again"}, &output); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("a duplicate feature must be refused: %v", err)
	}
	if err := Feature(context.Background(), []string{"add", root, "--id", "../escape", "--title", "Escape"}, &output); err == nil {
		t.Fatal("an unstable feature id was accepted")
	}
	assertValid(t, root)
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Features) != 2 || document.FindFeature("billing") == nil || document.FindFeature(testFeature) == nil {
		t.Fatalf("features = %#v", document.Features)
	}
}

func TestPersonaAddReviseAndSetState(t *testing.T) {
	root := newAuthoredSaga(t)
	added := mustLiving(t, "persona add", personaCommand, "add", root, "--id", "admin", "--name", "Admin",
		"--description", "Runs the store", "--request-id", "admin-request")
	persona := "urn:change-saga:atomic:persona:admin"
	if !added.OK || added.Resource != persona || !strings.HasPrefix(added.Path, applayout.PersonasDir+"/") {
		t.Fatalf("persona add = %#v", added)
	}
	if replay := mustLiving(t, "persona add", personaCommand, "add", root, "--id", "admin", "--name", "Admin",
		"--description", "Runs the store", "--request-id", "admin-request"); !replay.Replayed {
		t.Fatalf("identical persona add did not replay: %#v", replay)
	}
	var output bytes.Buffer
	if err := Persona(context.Background(), []string{"add", root, "--id", "admin", "--name", "Other", "--description", "Other"}, &output); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("a duplicate persona must be refused: %v", err)
	}

	revised := mustLiving(t, "persona revise", personaCommand, "revise", root, "--persona", persona, "--revision", "r2",
		"--parent", persona+":revision:r1", "--name", "Store admin", "--description", "Runs one store")
	if !reflect.DeepEqual(revised.CurrentHeads, []string{persona + ":revision:r2"}) {
		t.Fatalf("persona revise = %#v", revised)
	}
	output.Reset()
	if err := Persona(context.Background(), []string{"revise", root, "--persona", persona, "--revision", "r3",
		"--parent", persona + ":revision:r1", "--name", "Stale", "--description", "Stale"}, &output); err == nil || !strings.Contains(err.Error(), "every current head") {
		t.Fatalf("a stale persona parent must be refused: %v", err)
	}

	retired := mustLiving(t, "persona set-state", personaCommand, "set-state", root, "--persona", persona, "--event", "retired",
		"--parent", persona+":event:active", "--state", "retired", "--reason", "No longer served")
	if !reflect.DeepEqual(retired.EventIDs, []string{"retired"}) {
		t.Fatalf("persona set-state = %#v", retired)
	}
	document, err := requirements.Load(root, "atomic")
	if err != nil {
		t.Fatal(err)
	}
	found := document.FindPersona("admin")
	if found == nil || found.CurrentRevision == nil || found.CurrentRevision.Name != "Store admin" || found.CurrentLifecycle == nil || found.CurrentLifecycle.State != requirements.PersonaRetired {
		t.Fatalf("persona = %#v", found)
	}
	assertValid(t, root)
}

// When the app has several features, every command that creates a top-level
// feature record must name its feature: the choice is the author's.
func TestCreateCommandsRequireAnFeatureAmongSeveral(t *testing.T) {
	root := newAuthoredSaga(t)
	story := addStory(t, root, testFeature, "checkout", testPersonaURN).Resource
	mustLiving(t, "feature add", featureCommand, "add", root, "--id", "billing", "--title", "Billing")
	source := newPrototypeSource(t, "<p>prototype</p>")
	ctx := context.Background()
	commands := []struct {
		name string
		run  func(*bytes.Buffer) error
	}{
		{"story add", func(out *bytes.Buffer) error {
			return Story(ctx, []string{"add", root, "--persona", testPersonaURN, "--id", "s", "--revision", "r1", "--event", "proposed", "--title", "S", "--statement", "S", "--priority", "must"}, out)
		}},
		{"citation add", func(out *bytes.Buffer) error {
			return Citation(ctx, []string{"add", root, "--id", "c", "--kind", "url", "--title", "C", "--reference", "https://example.test/c"}, out)
		}},
		{"relation add", func(out *bytes.Buffer) error {
			return Relation(ctx, []string{"add", root, "--id", "r", "--type", "refines", "--from", story, "--to", story + ":criterion:fast", "--rationale", "R"}, out)
		}},
		{"prototype add-html", func(out *bytes.Buffer) error {
			return Prototype(ctx, []string{"add-html", root, "--id", "p", "--revision", "r1", "--title", "P", "--source", source}, out)
		}},
		{"prototype add-external", func(out *bytes.Buffer) error {
			return Prototype(ctx, []string{"add-external", root, "--id", "p", "--revision", "r1", "--title", "P", "--url", "https://example.test/p"}, out)
		}},
		{"plan add-wave", func(out *bytes.Buffer) error {
			return Plan(ctx, []string{"add-wave", root, "--id", "w", "--revision", "r1", "--title", "W", "--objective", "O", "--request-id", "w"}, out)
		}},
		{"plan add-item", func(out *bytes.Buffer) error {
			return Plan(ctx, []string{"add-item", root, "--id", "i", "--revision", "r1", "--title", "I", "--objective", "O", "--deliverable", "D", "--request-id", "i"}, out)
		}},
		{"plan add-dependency", func(out *bytes.Buffer) error {
			return Plan(ctx, []string{"add-dependency", root, "--id", "d", "--prerequisite", "urn:change-saga:atomic:work-item:a", "--dependent", "urn:change-saga:atomic:work-item:b", "--condition", "progress_done", "--reason", "R", "--request-id", "d"}, out)
		}},
		{"plan add-contract", func(out *bytes.Buffer) error {
			return Plan(ctx, []string{"add-contract", root, "--id", "k", "--revision", "r1", "--kind", "handoff", "--provider", "urn:change-saga:atomic:work-item:a", "--consumer", "urn:change-saga:atomic:work-item:b", "--statement", "S", "--acceptance", "A", "--request-id", "k"}, out)
		}},
		{"quality test-case add", func(out *bytes.Buffer) error {
			return Quality(ctx, []string{"test-case", "add", root, "--id", "t", "--title", "T", "--kind", "positive", "--automation", "manual", "--step", `{"id":"s1","action":"A","expected_result":"E"}`, "--expected-result", "E"}, out)
		}},
		{"quality policy set", func(out *bytes.Buffer) error {
			return Quality(ctx, []string{"policy", "set", root, "--criterion", story + ":criterion:fast", "--story-revision", story + ":revision:r1", "--require", "positive", "--rationale", "R"}, out)
		}},
		{"add-deck", func(out *bytes.Buffer) error {
			return AddDeck(ctx, []string{"--objective", "Explain.", root, "implementation"}, out)
		}},
		{"add-chapter", func(out *bytes.Buffer) error { return AddChapter(ctx, []string{"--title", "C", root, "chapter"}, out) }},
		{"add-fragment", func(out *bytes.Buffer) error {
			return AddFragment(ctx, []string{"--title", "F", "--name", "f", root}, out)
		}},
		{"design add-chapter", func(out *bytes.Buffer) error {
			return Design(ctx, []string{"add-chapter", "--title", "C", root, "chapter"}, out)
		}},
		{"design add-fragment", func(out *bytes.Buffer) error {
			return Design(ctx, []string{"add-fragment", "--title", "F", "--name", "f", root}, out)
		}},
	}
	before := treeListing(t, root)
	for _, command := range commands {
		var output bytes.Buffer
		err := command.run(&output)
		if err == nil || !strings.Contains(err.Error(), "--feature is required") || !strings.Contains(err.Error(), "known features: billing, "+testFeature) {
			t.Errorf("%s without --feature = %v", command.name, err)
		}
	}
	if after := treeListing(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("a refused create wrote files:\nbefore %v\nafter  %v", before, after)
	}

	// A story needs no persona: personas are optional, so a first story can
	// be written before anyone is named.
	var output bytes.Buffer
	if err := Story(ctx, []string{"add", root, "--feature", testFeature, "--id", "s", "--revision", "r1", "--event", "proposed", "--title", "S", "--statement", "S", "--priority", "must"}, &output); err != nil {
		t.Fatalf("story add without --persona = %v", err)
	}
	// Nor a priority: it is optional free text, so incremental adoption never
	// makes an author invent one.
	if err := Story(ctx, []string{"add", root, "--feature", testFeature, "--id", "unprioritized", "--revision", "r1", "--event", "proposed", "--title", "U", "--statement", "U"}, &output); err != nil {
		t.Fatalf("story add without --priority = %v", err)
	}
	revision, err := os.ReadFile(filepath.Join(root, "___features", testFeature+".feature", "___requirements", "stories", "unprioritized.story", "revisions", "r1.json"))
	if err != nil || strings.Contains(string(revision), "priority") {
		t.Fatalf("a story with no priority records none: %v\n%s", err, revision)
	}
	// An unknown feature lists the known ones.
	err = AddChapter(ctx, []string{"--feature", "missing", "--title", "C", root, "chapter"}, &output)
	if err == nil || !strings.Contains(err.Error(), `feature "missing" does not exist`) || !strings.Contains(err.Error(), "known features: billing, "+testFeature) {
		t.Fatalf("unknown feature = %v", err)
	}
}

func treeListing(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if !strings.HasPrefix(filepath.Base(rel), ".change-saga") {
			paths = append(paths, rel)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return paths
}

// A command on an existing record accepts --feature only as an assertion of the
// feature that already holds the record.
func TestFeatureMismatchOnAnExistingRecordIsRejected(t *testing.T) {
	root := newAuthoredSaga(t)
	mustLiving(t, "feature add", featureCommand, "add", root, "--id", "billing", "--title", "Billing")
	story := addStory(t, root, testFeature, "checkout", testPersonaURN).Resource
	var output bytes.Buffer
	err := Story(context.Background(), []string{"set-state", root, "--feature", "billing", "--story", story, "--event", "accepted",
		"--parent", story + ":event:proposed", "--state", "accepted"}, &output)
	if err == nil || !strings.Contains(err.Error(), `is in feature "core", not "billing"`) {
		t.Fatalf("story set-state with the wrong feature = %v", err)
	}
	err = Story(context.Background(), []string{"revise", root, "--feature", "billing", "--story", story, "--revision", "r2",
		"--parent", story + ":revision:r1", "--persona", testPersonaURN, "--title", "T", "--statement", "S", "--priority", "must"}, &output)
	if err == nil || !strings.Contains(err.Error(), `is in feature "core", not "billing"`) {
		t.Fatalf("story revise with the wrong feature = %v", err)
	}
	// The matching feature, by ID or URN, is accepted.
	mustLiving(t, "story set-state", storyCommand, "set-state", root, "--feature", "urn:change-saga:atomic:feature:core", "--story", story,
		"--event", "accepted", "--parent", story+":event:proposed", "--state", "accepted")

	if err := AddChapter(context.Background(), []string{"--feature", testFeature, "--id", "backend", "--title", "Backend", root, "backend"}, &output); err != nil {
		t.Fatal(err)
	}
	err = AddSection(context.Background(), []string{"--feature", "billing", "--title", "Flow", root, "backend/flow"}, &output)
	if err == nil || !strings.Contains(err.Error(), `is in feature "core", not "billing"`) {
		t.Fatalf("add-section into another feature's chapter = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(testFeatureDir(root), "backend.chapter", "flow")); !os.IsNotExist(statErr) {
		t.Fatalf("a refused section was written: %v", statErr)
	}
	assertValid(t, root)
}

// Moving a story between features changes where it is stored, never its
// identity, so a relation pinned to it stays current.
func TestStoryMoveKeepsRelationsCurrent(t *testing.T) {
	root, repo := coveredSaga(t)
	mustLiving(t, "feature add", featureCommand, "add", root, "--id", "billing", "--title", "Billing")
	story := addStory(t, root, testFeature, "checkout", personaURNFor("batch")).Resource
	runQuality(t, "", "test-case", "add", root, "--feature", testFeature, "--id", "fast", "--title", "Fast", "--kind", "positive",
		"--automation", "automated", "--step", `{"id":"s1","action":"Check out","expected_result":"Done"}`, "--expected-result", "Done")
	mustLiving(t, "relation add", relationCommand, "add", root, "--feature", testFeature, "--id", "fast-verifies", "--type", "verifies",
		"--from", "urn:change-saga:batch:test-case:fast", "--to", story+":criterion:fast", "--rationale", "Exercises the fast path.")

	moved := mustLiving(t, "story move", storyCommand, "move", root, "--story", story, "--feature", "billing")
	if !moved.OK || moved.Resource != story || !strings.HasPrefix(moved.Path, "___features/billing.feature/") {
		t.Fatalf("story move = %#v", moved)
	}
	if _, err := os.Stat(filepath.Join(testFeatureDir(root), "___requirements", "stories", "checkout.story")); !os.IsNotExist(err) {
		t.Fatalf("the story stayed in its old feature: %v", err)
	}
	document, err := requirements.Load(root, "batch")
	if err != nil {
		t.Fatal(err)
	}
	if found := document.FindStory("checkout"); found == nil || found.Feature != "billing" {
		t.Fatalf("moved story = %#v", found)
	}

	var output bytes.Buffer
	if err := Relation(context.Background(), []string{"status", root, "--json"}, &output); err != nil {
		t.Fatalf("relation status: %v", err)
	}
	var result relationStatusOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || len(result.Relations) != 1 || !result.Relations[0].Current() {
		t.Fatalf("the relation did not stay current across the move: %s (%v)", output.String(), err)
	}
	status, raw := runAppStatus(t, root, repo)
	for _, row := range status.Stories {
		if row.Story == story && row.Feature != "billing" {
			t.Fatalf("status reports the moved story in feature %q:\n%s", row.Feature, raw)
		}
	}

	output.Reset()
	if err := Story(context.Background(), []string{"move", root, "--story", story, "--feature", "missing"}, &output); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("moving to an unknown feature = %v", err)
	}
	assertValid(t, root)
}

// The onboarding deck belongs to the app, and each of its Items explains a
// persona, feature, or story record rather than code.
func TestOnboardingDeckItemsExplainRecords(t *testing.T) {
	root := newAuthoredSaga(t)
	ctx := context.Background()
	var output bytes.Buffer
	if err := AddDeck(ctx, []string{"--role", "onboarding", "--feature", testFeature, "--objective", "Orient.", root, "welcome"}, &output); err == nil || !strings.Contains(err.Error(), "omit --feature") {
		t.Fatalf("an onboarding deck in a feature = %v", err)
	}
	if err := AddDeck(ctx, []string{"--role", "onboarding", "--id", "welcome", "--objective", "Orient new contributors.", root, "welcome"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddDeck(ctx, []string{"--role", "onboarding", "--id", "second", "--objective", "Again.", root, "second"}, &output); err == nil || !strings.Contains(err.Error(), "already has onboarding deck") {
		t.Fatalf("a second onboarding deck = %v", err)
	}
	if err := AddSlide(ctx, []string{"--deck", "welcome", "--intent", "orient", "--layout", "hero", "--title", "Who we serve", "--takeaway", "One persona.", root, "who"}, &output); err != nil {
		t.Fatal(err)
	}
	item := []string{"--slide", "who", "--kind", "callout", "--element-id", "slide-title", "--description", "The persona the app serves.", "--body", "Users."}
	if err := AddItem(ctx, append(append([]string{}, item...), "--id", "missing", root), &output); err == nil || !strings.Contains(err.Error(), "--record is required") {
		t.Fatalf("an onboarding item without --record = %v", err)
	}
	if err := AddItem(ctx, append(append([]string{}, item...), "--id", "ghost", "--record", "urn:change-saga:atomic:persona:ghost", root), &output); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("an onboarding item for a missing record = %v", err)
	}
	if err := AddItem(ctx, append(append([]string{}, item...), "--id", "user", "--feature", testFeature, "--record", testPersonaURN, root), &output); err == nil || !strings.Contains(err.Error(), "omit --feature") {
		t.Fatalf("an onboarding item with --feature = %v", err)
	}
	output.Reset()
	if err := AddItem(ctx, append(append([]string{}, item...), "--id", "user", "--record", testPersonaURN, root), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Record: "+testPersonaURN) || strings.Contains(output.String(), "change-saga cover") {
		t.Fatalf("onboarding item output = %q", output.String())
	}
	if err := AddItem(ctx, append(append([]string{}, item...), "--id", "core", "--record", "urn:change-saga:atomic:feature:core", root), &output); err != nil {
		t.Fatalf("an onboarding item for a feature: %v", err)
	}

	assertValid(t, root)
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Onboarding) != 1 || len(document.Decks) != 0 || !strings.HasPrefix(document.Onboarding[0].Path, applayout.OnboardingDir+"/") {
		t.Fatalf("onboarding = %#v decks = %#v", document.Onboarding, document.Decks)
	}
	items := document.Onboarding[0].Slides[0].Items
	if len(items) != 2 || items[0].Record == "" || items[1].Record == "" {
		t.Fatalf("onboarding items = %#v", items)
	}

	// An implementation Item explains code, not a record.
	if err := AddDeck(ctx, []string{"--feature", testFeature, "--id", "impl", "--objective", "Explain.", root, "impl"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddSlide(ctx, []string{"--deck", "impl", "--intent", "orient", "--layout", "hero", "--title", "Change", "--takeaway", "It changed.", root, "change"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(ctx, []string{"--slide", "change", "--kind", "callout", "--id", "code", "--element-id", "slide-title", "--description", "D", "--body", "B", "--record", testPersonaURN, root}, &output); err == nil || !strings.Contains(err.Error(), "--record is for onboarding items") {
		t.Fatalf("an implementation item with --record = %v", err)
	}
	if err := AddItem(ctx, []string{"--slide", "change", "--kind", "callout", "--id", "code", "--element-id", "slide-title", "--description", "D", "--body", "B", root}, &output); err != nil {
		t.Fatal(err)
	}
	assertValid(t, root)
}

// Status reports each active persona no accepted story serves as a gap, and a
// persona retirement as one question for all the stories it leaves without an
// active persona.
func TestStatusReportsPersonaGapsAndOneQuestionPerRetirement(t *testing.T) {
	root, repo := coveredSaga(t)
	user := personaURNFor("batch")
	admin := mustLiving(t, "persona add", personaCommand, "add", root, "--id", "admin", "--name", "Admin", "--description", "Runs the store").Resource
	first := addStory(t, root, testFeature, "browse", user).Resource
	second := addStory(t, root, testFeature, "search", user).Resource
	shared := addStory(t, root, testFeature, "manage", user, admin).Resource

	status, raw := runAppStatus(t, root, repo)
	gaps := map[string]bool{}
	for _, persona := range status.Personas {
		gaps[persona.Persona] = persona.Gap
	}
	if !gaps[user] || !gaps[admin] {
		t.Fatalf("personas with no accepted story are gaps: %#v\n%s", status.Personas, raw)
	}
	gapAction := false
	for _, action := range status.NextActions {
		gapAction = gapAction || (action.ID == "growth:persona:"+user && action.Resource == user)
	}
	if !gapAction {
		t.Fatalf("no next action asks which story serves the persona:\n%s", raw)
	}

	acceptStory(t, root, first)
	status, raw = runAppStatus(t, root, repo)
	for _, persona := range status.Personas {
		if persona.Persona == user && (persona.Gap || !reflect.DeepEqual(persona.ServedBy, []string{first})) {
			t.Fatalf("an accepted story closes the persona gap: %#v\n%s", persona, raw)
		}
		if persona.Persona == admin && !persona.Gap {
			t.Fatalf("admin is still a gap: %#v", persona)
		}
	}

	mustLiving(t, "persona set-state", personaCommand, "set-state", root, "--persona", user, "--event", "retired",
		"--parent", user+":event:active", "--state", "retired", "--reason", "The app no longer serves them")
	status, raw = runAppStatus(t, root, repo)
	if len(status.PersonaOrphans) != 1 {
		t.Fatalf("one retirement is one persona_orphans group: %#v\n%s", status.PersonaOrphans, raw)
	}
	group := status.PersonaOrphans[0]
	if !reflect.DeepEqual(group.Personas, []string{user}) || !reflect.DeepEqual(group.Stories, []string{first, second}) {
		t.Fatalf("orphan group = %#v; %s still serves an active persona", group, shared)
	}
	questions := 0
	for _, action := range status.NextActions {
		if strings.HasPrefix(action.ID, "growth:retired-personas:") {
			questions++
			if action.Resource != user || action.Question == nil || len(action.Question.Options) != 2 {
				t.Fatalf("retirement question = %#v", action)
			}
		}
		for _, story := range []string{first, second} {
			if action.Resource == story && !strings.HasPrefix(action.ID, "growth:retired-personas:") && strings.Contains(action.ID, "persona") {
				t.Fatalf("an orphaned story got its own persona action %q", action.ID)
			}
		}
	}
	if questions != 1 {
		t.Fatalf("expected one next action for the retirement, got %d:\n%s", questions, raw)
	}
	for _, persona := range status.Personas {
		if persona.Persona == user && (persona.State != "retired" || persona.Gap) {
			t.Fatalf("a retired persona is not a gap: %#v", persona)
		}
	}
}

// Every next action about feature content names its feature, and its commands are
// aimed at that feature.
func TestStatusNextActionsNameTheirFeature(t *testing.T) {
	root, repo := coveredSaga(t)
	mustLiving(t, "feature add", featureCommand, "add", root, "--id", "billing", "--title", "Billing")
	core := addStory(t, root, testFeature, "checkout", personaURNFor("batch")).Resource
	billing := addStory(t, root, "billing", "invoice", personaURNFor("batch")).Resource
	status, raw := runAppStatus(t, root, repo)

	features := map[string][]string{}
	for _, feature := range status.Features {
		features[feature.ID] = feature.Stories
	}
	if !reflect.DeepEqual(features[testFeature], []string{core}) || !reflect.DeepEqual(features["billing"], []string{billing}) {
		t.Fatalf("status features = %#v", status.Features)
	}
	named, aimed := map[string]bool{}, map[string]bool{}
	for _, action := range status.NextActions {
		if action.Resource != core && action.Resource != billing {
			continue
		}
		want := map[string]string{core: testFeature, billing: "billing"}[action.Resource]
		if action.Feature != want {
			t.Fatalf("next action %q about %s names feature %q, want %q", action.ID, action.Resource, action.Feature, want)
		}
		named[want] = true
		for _, argv := range actionArgv(action.Command, action.Question) {
			joined := strings.Join(argv, " ")
			if strings.Contains(joined, "--feature") && !strings.Contains(joined, "--feature "+want) {
				t.Fatalf("next action %q aims a command at another feature: %s", action.ID, joined)
			}
			aimed[want] = aimed[want] || strings.Contains(joined, "--feature "+want)
		}
	}
	if !named[testFeature] || !named["billing"] || !aimed[testFeature] || !aimed["billing"] {
		t.Fatalf("expected feature-scoped next actions with --feature commands for both features (named %v, aimed %v):\n%s", named, aimed, raw)
	}
}

func actionArgv(command *struct {
	Argv []string `json:"argv"`
}, question *struct {
	Options []struct {
		Commands []struct {
			Argv []string `json:"argv"`
		} `json:"commands"`
	} `json:"options"`
}) [][]string {
	var all [][]string
	if command != nil {
		all = append(all, command.Argv)
	}
	if question != nil {
		for _, option := range question.Options {
			for _, command := range option.Commands {
				all = append(all, command.Argv)
			}
		}
	}
	return all
}

// A story gated by an off flag is implemented but not enabled once its code is
// mapped; turning the flag on enables it.
func TestFlagGatedStoryIsImplementedButNotEnabled(t *testing.T) {
	root, repo := coveredSaga(t)
	persona := personaURNFor("batch")
	ctx := context.Background()
	story := addStory(t, root, testFeature, "checkout", persona).Resource
	acceptStory(t, root, story)

	var output bytes.Buffer
	if err := FeatureFlag(ctx, []string{"add", root, "--id", "new-checkout", "--description", "Gates the new checkout"}, &output); err == nil {
		t.Fatal("a flag must gate at least one target")
	}
	flag := mustLiving(t, "flag add", flagCommand, "add", root, "--id", "new-checkout", "--description", "Gates the new checkout", "--target", story)
	if !flag.OK || flag.Resource != "urn:change-saga:batch:flag:new-checkout" || !strings.HasPrefix(flag.Path, applayout.FeatureFlagsDir+"/") {
		t.Fatalf("flag add = %#v", flag)
	}
	storyRow := func(status appStatus) (string, []string) {
		for _, row := range status.Stories {
			if row.Story == story {
				return row.Availability, row.GatedBy
			}
		}
		t.Fatalf("status omits %s", story)
		return "", nil
	}
	status, raw := runAppStatus(t, root, repo)
	if availability, gatedBy := storyRow(status); availability != "not_implemented" || !reflect.DeepEqual(gatedBy, []string{flag.Resource}) {
		t.Fatalf("unmapped gated story = %s gated by %v\n%s", availability, gatedBy, raw)
	}

	// Map the whole change to an implementation Item that addresses the criterion.
	if err := AddDeck(ctx, []string{"--feature", testFeature, "--id", "impl", "--objective", "Explain the change.", root, "impl"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddSlide(ctx, []string{"--deck", "impl", "--intent", "explain", "--layout", "diagram", "--title", "Handler", "--takeaway", "The handler is new.", root, "handler"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(ctx, []string{"--slide", "handler", "--kind", "callout", "--id", "handler-item", "--element-id", "slide-title", "--description", "The new handler.", "--body", "It is new.", root}, &output); err != nil {
		t.Fatal(err)
	}
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	item := document.Decks[0].Slides[0].Items[0].Target
	if out, err := runCover(t, "", "--repo", repo, "--target", item, "--path", "internal/service/handler.go", "--changed-lines", "--name", "handler", root); err != nil {
		t.Fatalf("cover: %v\n%s", err, out)
	}
	mustLiving(t, "relation add", relationCommand, "add", root, "--feature", testFeature, "--id", "handler-explains-fast", "--type", "explains",
		"--from", item, "--to", story+":criterion:fast", "--rationale", "The handler implements the fast path.")

	status, raw = runAppStatus(t, root, repo)
	if availability, gatedBy := storyRow(status); availability != "implemented_not_enabled" || !reflect.DeepEqual(gatedBy, []string{flag.Resource}) {
		t.Fatalf("mapped gated story = %s gated by %v\n%s", availability, gatedBy, raw)
	}
	var text bytes.Buffer
	_ = Status(ctx, []string{"--against", "main", "--repo", repo, root}, &text)
	if !strings.Contains(text.String(), "Implemented, not enabled: "+story) {
		t.Fatalf("text status omits the gated story:\n%s", text.String())
	}

	mustLiving(t, "flag set-state", flagCommand, "set-state", root, "--flag", flag.Resource, "--event", "on",
		"--parent", flag.Resource+":event:off", "--state", "on", "--reason", "Released")
	status, raw = runAppStatus(t, root, repo)
	if availability, gatedBy := storyRow(status); availability != "implemented_enabled" || len(gatedBy) != 0 {
		t.Fatalf("enabled story = %s gated by %v\n%s", availability, gatedBy, raw)
	}
	assertValid(t, root)
}

// The incremental case: a first change names a feature, writes one story, and
// explains itself with a deck, without defining a single persona. It is
// valid, status reports it and exits zero, and personas are only ever growth.
func TestFirstChangeNeedsNoPersonas(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeFile(t, filepath.Join(repo, "README.md"), "base\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	root := filepath.Join(t.TempDir(), "first.saga")
	ctx := context.Background()
	var output bytes.Buffer
	if err := Init(ctx, []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", "--id", "first", root}, &output); err != nil {
		t.Fatal(err)
	}
	mustLiving(t, "feature add", featureCommand, "add", root, "--id", "checkout", "--title", "Checkout")
	story := addStory(t, root, "checkout", "pay").Resource
	output.Reset()
	if err := AddDeck(ctx, []string{"--feature", "checkout", "--objective", "Explain how payment works.", root, "payments"}, &output); err != nil {
		t.Fatalf("add-deck: %v\n%s", err, output.String())
	}
	output.Reset()
	if err := Validate(ctx, []string{"--json", root}, &output); err != nil {
		t.Fatalf("a first change without personas is invalid: %v\n%s", err, output.String())
	}

	var status bytes.Buffer
	if err := Status(ctx, []string{"--against", "main", "--json", "--repo", repo, root}, &status); err != nil {
		t.Fatalf("status reports and exits zero for a first change with no personas: %v\n%s", err, status.String())
	}
	var document struct {
		Coverage struct {
			Areas map[string]struct {
				Total    int  `json:"total"`
				Complete bool `json:"complete"`
			} `json:"areas"`
		} `json:"coverage"`
		PersonaCoverage struct {
			Blocking bool              `json:"blocking"`
			Facts    []json.RawMessage `json:"facts"`
		} `json:"persona_coverage"`
		NextActions []nextaction.Action `json:"next_actions"`
	}
	if err := json.Unmarshal(status.Bytes(), &document); err != nil {
		t.Fatalf("status --json: %v\n%s", err, status.String())
	}
	if document.PersonaCoverage.Blocking || len(document.PersonaCoverage.Facts) != 0 {
		t.Fatalf("persona coverage with no personas: %+v", document.PersonaCoverage)
	}
	if design := document.Coverage.Areas["design"]; design.Total != 1 || design.Complete {
		t.Fatalf("the story the change added is in scope for design: %+v", design)
	}
	for _, action := range document.NextActions {
		if action.Area == "personas" && action.Category != nextaction.CategoryGrowth {
			t.Fatalf("personas are never demanded: %#v", action)
		}
	}
	_ = story
}

// An omitted --feature uses the app's only feature, and when the app has none, the
// first command that needs one creates it, named after the branch. Either
// way the choice is reported.
func TestOmittedFeatureDefaultsAndSaysWhatItChose(t *testing.T) {
	repo := shortTempDir(t)
	git(t, repo, "init", "-b", "main")
	git(t, repo, "checkout", "-b", "feature/checkout-flow")
	root := filepath.Join(repo, "app.saga")
	ctx := context.Background()
	if err := Init(ctx, []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", "--allow-repository-mismatch", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var notices bytes.Buffer
	previous := featureNotices
	featureNotices = &notices
	defer func() { featureNotices = previous }()

	if err := AddDeck(ctx, []string{"--objective", "Explain.", root, "implementation"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("add-deck with no feature yet: %v", err)
	}
	features, err := applayout.Features(root)
	if err != nil || len(features) != 1 || features[0].ID != "checkout-flow" || features[0].Title != "Checkout flow" {
		t.Fatalf("the first feature is named after the branch: %+v %v", features, err)
	}
	if !strings.Contains(notices.String(), `Created feature "checkout-flow"`) || !strings.Contains(notices.String(), "branch feature/checkout-flow") {
		t.Fatalf("the created feature is reported: %q", notices.String())
	}
	notices.Reset()
	if err := Story(ctx, []string{"add", root, "--id", "pay", "--revision", "r1", "--event", "proposed", "--title", "Pay", "--statement", "S", "--priority", "must"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("story add with one feature: %v", err)
	}
	if !strings.Contains(notices.String(), `Using feature "checkout-flow", the app's only feature`) {
		t.Fatalf("the implied feature is reported: %q", notices.String())
	}
	assertValid(t, root)
}

// The reviewer's sidebar and status list features in the order the author
// created them, not alphabetically.
func TestFeaturesArePresentedInCreationOrder(t *testing.T) {
	root := newAuthoredSaga(t)
	for _, id := range []string{"zebra", "accounts"} {
		mustLiving(t, "feature add", featureCommand, "add", root, "--id", id, "--title", id)
	}
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, feature := range document.Features {
		got = append(got, feature.ID)
	}
	if strings.Join(got, ",") != testFeature+",zebra,accounts" {
		t.Fatalf("document features = %v", got)
	}
	status, err := livingapp.LoadStatus(context.Background(), livingapp.StatusOptions{SagaRoot: root, Document: document})
	if err != nil {
		t.Fatal(err)
	}
	got = got[:0]
	for _, feature := range status.Features {
		got = append(got, feature.ID)
	}
	if strings.Join(got, ",") != testFeature+",zebra,accounts" {
		t.Fatalf("status features = %v", got)
	}
}
