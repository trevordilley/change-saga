package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A Roadmap feature may augment only another existing feature, and only while
// it is planned; the link is canonicalized to the feature URN, can be cleared,
// and a link left dangling by the target's removal is reported by status.
func TestFeatureAugmentsNamesAnotherExistingFeature(t *testing.T) {
	t.Parallel()
	root := newAuthoredSaga(t)
	ctx := context.Background()
	refuse := func(want string, args ...string) {
		t.Helper()
		if err := Feature(ctx, append(args, root), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%v = %v, want an error containing %q", args, err, want)
		}
	}
	refuse("does not exist", "add", "--id", "search", "--title", "Search", "--status", "planned", "--augments", "ghost")
	refuse("only to a planned", "add", "--id", "search", "--title", "Search", "--augments", testFeature)
	mustLiving(t, "feature add", featureCommand, "add", root, "--id", "search", "--title", "Search", "--status", "planned", "--augments", testFeature)
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := document.FindFeature("search").Plan.Augments; got != applayout.FeatureURN("atomic", testFeature) {
		t.Fatalf("augments = %q, want the canonical feature URN", got)
	}
	refuse("cannot augment itself", "set-plan", "--feature", "search", "--augments", "search")
	refuse("feature deliver", "set-plan", "--feature", "search", "--status", "built")
	mustLiving(t, "feature set-plan", featureCommand, "set-plan", root, "--feature", "search", "--augments", "")
	document, _, _ = saga.Load(root)
	if got := document.FindFeature("search").Plan.Augments; got != "" {
		t.Fatalf("an empty --augments must clear it, got %q", got)
	}

	// A link to a feature that no longer exists is a dangling reference.
	mustLiving(t, "feature add", featureCommand, "add", root, "--id", "billing", "--title", "Billing")
	mustLiving(t, "feature set-plan", featureCommand, "set-plan", root, "--feature", "search", "--augments", "billing")
	if err := os.RemoveAll(applayout.FeatureDir(root, "billing")); err != nil {
		t.Fatal(err)
	}
	assertValid(t, root)
	status, err := livingapp.LoadStatus(ctx, livingapp.StatusOptions{SagaRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Dangling) != 1 || status.Dangling[0].Record != applayout.FeatureURN("atomic", "search") || status.Dangling[0].Missing[0] != applayout.FeatureURN("atomic", "billing") || !strings.Contains(status.Dangling[0].Fix, "--augments") {
		t.Fatalf("status must report the dangling augments link: %#v", status.Dangling)
	}
}

// Delivering a Roadmap feature moves everything it holds into the feature it
// augments with every URN unchanged, deletes it, and leaves the recipient's
// overview unhealthy until feature set-overview revises it. The dry run
// reports exactly what the delivery then does.
func TestFeatureDeliverMovesTheRoadmapIntoItsRecipient(t *testing.T) {
	t.Parallel()
	root, repo := coveredSaga(t)
	ctx := context.Background()
	mustLiving(t, "feature add", featureCommand, "add", root, "--id", "search", "--title", "Search", "--status", "planned")
	var output bytes.Buffer
	if err := Feature(ctx, []string{"set-overview", "--feature", "search", "--text", "# Search\n\nFind anything in the catalog.\n", root}, &output); err != nil {
		t.Fatal(err)
	}
	if err := FeatureTheme(ctx, []string{"add", root, "--id", "discovery", "--title", "Discovery", "--text", "Finding things.", "--feature", "search", "--feature", testFeature}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	story := addStory(t, root, "search", "find").Resource
	criterion := story + ":criterion:fast"
	citation := mustLiving(t, "citation add", func(ctx context.Context, args []string, out *bytes.Buffer) error { return Citation(ctx, args, out) },
		"add", root, "--feature", "search", "--id", "interview", "--kind", "url", "--title", "Interview", "--reference", "https://example.test/interview").Resource
	testCase := runQuality(t, "", "test-case", "add", root, "--feature", "search", "--id", "find-check", "--title", "Find check", "--kind", "positive", "--automation", "manual",
		"--step", `{"id":"find","action":"Search","expected_result":"Found"}`, "--expected-result", "Found").Resource
	mustLiving(t, "relation add", relationCommand, "add", root, "--feature", "search", "--id", "find-check-verifies",
		"--type", "verifies", "--from", testCase, "--to", criterion, "--rationale", "It checks finding.")
	runPrototype(t, "add-html", root, "--feature", "search", "--id", "search-page", "--revision", "r1", "--title", "Search", "--source", newPrototypeSource(t, `<input id="query">`))
	runPrototype(t, "annotate", root, "--prototype", "urn:change-saga:batch:prototype:search-page", "--id", "query",
		"--target", criterion, "--rationale", "Searching is prompt.", "--prototype-revision", "urn:change-saga:batch:prototype:search-page:revision:r1",
		"--story-revision", story+":revision:r1", "--element-id", "query")
	if err := AddDeck(ctx, []string{"--feature", "search", "--id", "search-impl", "--objective", "Explain search.", root, "search-impl"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddSlide(ctx, []string{"--deck", "search-impl", "--intent", "orient", "--layout", "hero", "--title", "Search", "--takeaway", "It finds.", root, "search-slide"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(ctx, []string{"--slide", "search-slide", "--kind", "callout", "--id", "query-box", "--element-id", "slide-title", "--description", "D", "--body", "B", root}, &output); err != nil {
		t.Fatal(err)
	}
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	item := document.FindFeature("search").Decks[0].Slides[0].Items[0].Target
	mustLiving(t, "relation add", relationCommand, "add", root, "--feature", "search", "--id", "query-box-addresses",
		"--type", "addresses", "--from", item, "--to", criterion, "--rationale", "The slide explains finding.")
	assertValid(t, root)

	deliver := func(args ...string) (deliveryOutput, error) {
		t.Helper()
		var output bytes.Buffer
		err := Feature(ctx, append(append([]string{"deliver", "--feature", "search"}, args...), "--json", root), &output)
		var result deliveryOutput
		if err != nil {
			t.Logf("deliver %v: %s", args, output.String())
		}
		if err == nil {
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("%v\n%s", err, output.String())
			}
		}
		return result, err
	}
	if err := Feature(ctx, []string{"deliver", "--feature", "search", root}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "--into") {
		t.Fatalf("delivering a feature that augments nothing without --into = %v", err)
	}
	mustLiving(t, "feature set-plan", featureCommand, "set-plan", root, "--feature", "search", "--augments", testFeature)

	preview, err := deliver("--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(applayout.FeatureDir(root, "search")); err != nil {
		t.Fatal("a dry run must not remove the Roadmap feature")
	}
	var text bytes.Buffer
	if err := Feature(ctx, []string{"deliver", "--feature", "search", "--dry-run", root}, &text); err != nil {
		t.Fatal(err)
	}
	if first := strings.SplitN(text.String(), "\n", 2)[0]; !strings.HasPrefix(first, "Dry run: would move ") || !strings.HasSuffix(first, "nothing was written.") {
		t.Fatalf("the dry run must lead with its size: %q", first)
	}
	for _, want := range []string{"Find anything in the catalog.", story, "It finishes promptly", "change-saga feature set-overview --feature " + testFeature} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("the delivery must show %q so the essay can be revised:\n%s", want, text.String())
		}
	}

	delivered, err := deliver()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(preview.Records, delivered.Records) || !slices.Equal(preview.Removed, delivered.Removed) || !slices.Equal(preview.Changed, delivered.Changed) ||
		!slices.Equal(preview.Moved, delivered.Moved) || len(preview.Adjust) != len(delivered.Adjust) {
		t.Fatalf("the dry run must preview the delivery exactly:\npreview %#v\ndelivered %#v", preview, delivered)
	}
	for _, want := range []string{story, citation, testCase, "urn:change-saga:batch:prototype:search-page", "urn:change-saga:batch:relation:find-check-verifies"} {
		if !slices.Contains(delivered.Records, want) {
			t.Fatalf("delivery missed %s: %v", want, delivered.Records)
		}
	}
	if delivered.Into != applayout.FeatureURN("batch", testFeature) || len(delivered.Stories) != 1 || delivered.Stories[0].Story != story || !strings.Contains(delivered.Overview, "Find anything") {
		t.Fatalf("delivery = %#v", delivered)
	}
	if _, err := os.Stat(applayout.FeatureDir(root, "search")); !os.IsNotExist(err) {
		t.Fatal("the Roadmap feature directory must be gone")
	}
	assertValid(t, root)

	// Every record is now in the recipient under its unchanged URN, and the
	// links between them still resolve.
	requirementsDocument, err := requirements.Load(root, "batch")
	if err != nil {
		t.Fatal(err)
	}
	if found := requirementsDocument.FindStory("find"); found == nil || found.Feature != testFeature {
		t.Fatalf("story = %#v", found)
	}
	for _, relation := range requirementsDocument.Relations {
		if relation.Feature != testFeature {
			t.Fatalf("relation %s stayed in %s", relation.ID, relation.Feature)
		}
	}
	qualityDocument, _ := quality.Load(root)
	prototypeDocument, _ := prototypes.Load(root, "batch")
	if qualityDocument.TestCases[0].Feature != testFeature || prototypeDocument.Prototypes[0].Feature != testFeature || len(prototypeDocument.Annotations) != 1 {
		t.Fatalf("test case or prototype did not move: %#v %#v", qualityDocument.TestCases, prototypeDocument.Prototypes)
	}
	document, _, _ = saga.Load(root)
	recipient := document.FindFeature(testFeature)
	if len(recipient.Decks) != 1 || recipient.Decks[0].Slides[0].Items[0].Target != item {
		t.Fatalf("the deck must move with its Item target unchanged: %#v", recipient.Decks)
	}
	for _, theme := range document.FeatureThemes {
		if slices.Contains(theme.Features, applayout.FeatureURN("batch", "search")) {
			t.Fatal("themes must stop listing the delivered feature")
		}
	}
	if pending := recipient.Plan.OverviewRevisionPending; len(pending) != 1 || pending[0].Feature != applayout.FeatureURN("batch", "search") || !slices.Equal(pending[0].Stories, []string{story}) {
		t.Fatalf("recipient overview revision = %#v", pending)
	}

	// Health fails, naming the stories and the command, until set-overview.
	status, err := livingapp.LoadStatus(ctx, livingapp.StatusOptions{SagaRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Dangling) != 0 || len(status.OverviewRevisions) != 1 || !slices.Equal(status.OverviewRevisions[0].Stories, []string{story}) {
		t.Fatalf("status = dangling %#v overview %#v", status.Dangling, status.OverviewRevisions)
	}
	var health bytes.Buffer
	err = Check(ctx, []string{"--covers", "health", "--repo", repo, root}, &health)
	var exit *StatusError
	if !errors.As(err, &exit) || !strings.Contains(health.String(), story) || !strings.Contains(health.String(), "feature set-overview --feature "+testFeature) {
		t.Fatalf("check --covers health must fail on the unrevised overview: %v\n%s", err, health.String())
	}
	if err := Feature(ctx, []string{"set-overview", "--feature", testFeature, "--text", "# Core\n\nCore now also finds things.\n", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	document, _, _ = saga.Load(root)
	if pending := document.FindFeature(testFeature).Plan.OverviewRevisionPending; len(pending) != 0 {
		t.Fatalf("set-overview must clear the pending revision: %#v", pending)
	}
	health.Reset()
	if err := Check(ctx, []string{"--covers", "health", "--repo", repo, root}, &health); err != nil {
		t.Fatalf("health after revising the overview: %v\n%s", err, health.String())
	}
}

// Content a delivery cannot place, such as a design chapter, is listed by the
// dry run, and the real delivery refuses rather than delete it.
func TestFeatureDeliverRefusesToLoseContentItCannotPlace(t *testing.T) {
	t.Parallel()
	root := newAuthoredSaga(t)
	ctx := context.Background()
	mustLiving(t, "feature add", featureCommand, "add", root, "--id", "search", "--title", "Search", "--status", "planned", "--augments", testFeature)
	addStory(t, root, "search", "find")
	var output bytes.Buffer
	if err := AddChapter(ctx, []string{"--feature", "search", "--title", "Index model", root, "index-model"}, &output); err != nil {
		t.Fatalf("add-chapter: %v\n%s", err, output.String())
	}
	output.Reset()
	if err := Feature(ctx, []string{"deliver", "--feature", "search", "--dry-run", "--json", root}, &output); err != nil {
		t.Fatal(err)
	}
	var preview deliveryOutput
	if err := json.Unmarshal(output.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Unplaced) != 1 || !strings.Contains(preview.Unplaced[0], "index-model") {
		t.Fatalf("the dry run must list the chapter it cannot place: %#v", preview.Unplaced)
	}
	err := Feature(ctx, []string{"deliver", "--feature", "search", root}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "index-model") {
		t.Fatalf("delivering with unplaceable content = %v", err)
	}
	if _, err := os.Stat(filepath.Join(applayout.FeatureDir(root, "search"), applayout.RequirementsDir, "stories", "find.story")); err != nil {
		t.Fatal("a refused delivery must not move anything")
	}
	if err := Feature(ctx, []string{"deliver", "--feature", testFeature, "--into", "search", root}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "planned") {
		t.Fatalf("delivering a built feature = %v", err)
	}
}
