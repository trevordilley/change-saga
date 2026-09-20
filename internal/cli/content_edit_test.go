package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

// contentSaga returns a Saga with an implementation deck whose slide holds a
// callout about a node, the node's code evidence, and a chapter with a
// fragment.
func contentSaga(t *testing.T) (root string) {
	t.Helper()
	root, repo := coveredSaga(t)
	var output bytes.Buffer
	for _, step := range []struct {
		run  func(context.Context, []string, io.Writer) error
		args []string
	}{
		{AddDeck, []string{"--feature", testFeature, "--objective", "Explain the change.", root, "implementation"}},
		{AddSlide, []string{"--deck", "implementation", "--intent", "explain", "--layout", "diagram", root, "flow"}},
		{AddItem, []string{"--slide", "flow", "--kind", "node", "--id", "handler", "--element-id", "slide-title", "--label", "Wrong label", "--description", "The handler.", root}},
		{AddItem, []string{"--slide", "flow", "--kind", "callout", "--id", "why", "--element-id", "slide-desc", "--about", "handler", "--description", "Why.", "--body", "Because.", root}},
		{AddChapter, []string{"--feature", testFeature, root, "service"}},
		{AddFragment, []string{"--feature", testFeature, "--section", "service", "--title", "Notes", root}},
	} {
		if err := step.run(context.Background(), step.args, &output); err != nil {
			t.Fatalf("%v: %v\n%s", step.args, err, output.String())
		}
	}
	if out, err := runCover(t, "", "--repo", repo, "--target", "urn:change-saga:batch:slide:flow:item:handler", "--ref", "HEAD:internal/service/handler.go#L3", "--name", "handler", root); err != nil {
		t.Fatalf("cover: %v\n%s", err, out)
	}
	return root
}

func runContentJSON(t *testing.T, run func(context.Context, []string, io.Writer) error, args ...string) (contentEditOutput, error) {
	t.Helper()
	var output bytes.Buffer
	err := run(context.Background(), append([]string{"--json"}, args...), &output)
	var result contentEditOutput
	if err != nil {
		// A --json failure is itself one JSON value carrying the message.
		var failure contentEditOutput
		if decodeErr := json.Unmarshal(output.Bytes(), &failure); decodeErr != nil || failure.Error == nil {
			t.Fatalf("failure is not one JSON value: %s", output.String())
		}
		return result, errors.New(failure.Error.Message)
	}
	if decodeErr := json.Unmarshal(output.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode %s: %v", output.String(), decodeErr)
	}
	return result, nil
}

func loadSlide(t *testing.T, root string) *saga.Slide {
	t.Helper()
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: err=%v issues=%#v", err, validation.Issues)
	}
	return document.Decks[0].Slides[0]
}

// A wrong label is fixed with a command, not a hand edit; repeating the same
// revise writes nothing and says so.
func TestReviseItemCorrectsFieldsInPlaceAndReplays(t *testing.T) {
	root := contentSaga(t)
	item := "urn:change-saga:batch:slide:flow:item:handler"
	result, err := runContentJSON(t, ReviseItem, "--item", item, "--label", "Handler", "--rank", "40", root)
	if err != nil || result.Replayed || !reflect.DeepEqual(result.Changed, []string{"label", "rank"}) {
		t.Fatalf("revise = %+v, %v", result, err)
	}
	slide := loadSlide(t, root)
	var found *saga.Item
	for _, candidate := range slide.Items {
		if candidate.Target == item {
			found = candidate
		}
	}
	if found == nil || found.Label != "Handler" || found.Rank != 40 || len(found.Code) != 1 {
		t.Fatalf("revised Item = %+v", found)
	}
	replay, err := runContentJSON(t, ReviseItem, "--item", "handler", "--slide", "flow", "--label", "Handler", root)
	if err != nil || !replay.Replayed || len(replay.Paths) != 0 {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
}

// An edit that would make the Saga invalid is refused and leaves every file
// as it was.
func TestContentEditThatInvalidatesTheSagaChangesNothing(t *testing.T) {
	root := contentSaga(t)
	before := snapshotFiles(t, root)
	if _, err := runContentJSON(t, ReviseItem, "--item", "why", "--slide", "flow", "--kind", "node", root); err == nil || !strings.Contains(err.Error(), "callout-only") {
		t.Fatalf("invalidating revise error = %v", err)
	}
	if after := snapshotFiles(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("a refused edit changed the Saga")
	}
	if entries, _ := filepath.Glob(filepath.Join(filepath.Dir(root), ".change-saga-remove-*")); len(entries) != 0 {
		t.Fatalf("a refused edit left removed files behind: %v", entries)
	}
}

func snapshotFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.Contains(path, ".change-saga-lock") {
			return err
		}
		data, err := os.ReadFile(path)
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// Removing an Item deletes its record and evidence and drops it from the
// reading order; a callout about it must go first.
func TestRemoveItemDeletesEvidenceAndReadingOrder(t *testing.T) {
	root := contentSaga(t)
	if _, err := runContentJSON(t, RemoveItem, "--item", "handler", "--slide", "flow", root); err == nil || !strings.Contains(err.Error(), "is about handler") {
		t.Fatalf("remove of a callout's subject = %v", err)
	}
	dry, err := runContentJSON(t, RemoveItem, "--item", "why", "--slide", "flow", "--dry-run", root)
	if err != nil || !dry.DryRun || len(dry.Paths) == 0 || len(loadSlide(t, root).Items) != 2 {
		t.Fatalf("dry run = %+v, %v", dry, err)
	}
	if _, err := runContentJSON(t, RemoveItem, "--item", "why", "--slide", "flow", root); err != nil {
		t.Fatal(err)
	}
	result, err := runContentJSON(t, RemoveItem, "--item", "urn:change-saga:batch:slide:flow:item:handler", root)
	if err != nil || !reflect.DeepEqual(result.Removed, []string{"urn:change-saga:batch:slide:flow:item:handler"}) || len(result.Paths) != 3 {
		t.Fatalf("remove = %+v, %v", result, err)
	}
	slide := loadSlide(t, root)
	if len(slide.Items) != 0 || len(slide.ReadingOrder) != 0 {
		t.Fatalf("slide after removing its Items = %+v", slide)
	}
}

// Removing a slide, deck, or chapter removes what it contains and names the
// relations left pointing at it.
func TestRemoveContainers(t *testing.T) {
	root := contentSaga(t)
	var output bytes.Buffer
	if err := Story(context.Background(), []string{"add", "--feature", testFeature, "--id", "handle", "--revision", "r1", "--event", "proposed", "--title", "Handle", "--statement", "As a user I want requests handled so that I am served", "--priority", "must", root}, &output); err != nil {
		t.Fatal(err)
	}
	if err := Relation(context.Background(), []string{"add", "--feature", testFeature, "--id", "flow-explains-handle", "--type", "explains", "--from", "urn:change-saga:batch:slide:flow", "--to", "urn:change-saga:batch:story:handle", "--rationale", "The slide shows it.", root}, &output); err != nil {
		t.Fatalf("relation: %v\n%s", err, output.String())
	}
	result, err := runContentJSON(t, RemoveSlide, "--slide", "flow", root)
	if err != nil || len(result.Removed) != 3 || !reflect.DeepEqual(result.DanglingRelations, []string{"urn:change-saga:batch:relation:flow-explains-handle"}) {
		t.Fatalf("remove-slide = %+v, %v", result, err)
	}
	if result, err = runContentJSON(t, RemoveDeck, "--deck", "implementation", root); err != nil || !reflect.DeepEqual(result.Removed, []string{"urn:change-saga:batch:deck:implementation"}) {
		t.Fatalf("remove-deck = %+v, %v", result, err)
	}
	if result, err = runContentJSON(t, ReviseChapter, "--target", "service", "--title", "Service", "--order", "5", root); err != nil || !reflect.DeepEqual(result.Changed, []string{"title", "order"}) {
		t.Fatalf("revise-chapter = %+v, %v", result, err)
	}
	if result, err = runContentJSON(t, RemoveChapter, "--target", "urn:change-saga:batch:chapter:service", root); err != nil || len(result.Removed) != 2 {
		t.Fatalf("remove-chapter = %+v, %v", result, err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid || len(document.Decks) != 0 {
		t.Fatalf("after removals: err=%v issues=%#v decks=%d", err, validation.Issues, len(document.Decks))
	}
	if _, err := os.Stat(filepath.Join(testFeatureDir(root), "service.chapter")); !os.IsNotExist(err) {
		t.Fatalf("chapter directory survived: %v", err)
	}
}

// A kind mismatch is refused rather than editing some other record.
func TestNarrativeEditsRefuseOtherKinds(t *testing.T) {
	root := contentSaga(t)
	if _, err := runContentJSON(t, ReviseFragment, "--target", "urn:change-saga:batch:chapter:service", "--title", "X", root); err == nil || !strings.Contains(err.Error(), "not a fragment") {
		t.Fatalf("revise-fragment of a chapter = %v", err)
	}
	var output bytes.Buffer
	err := RemoveSection(context.Background(), []string{"--json", "--target", "urn:change-saga:batch:chapter:service", root}, &output)
	if err == nil || !strings.Contains(output.String(), `"ok": false`) || !strings.Contains(output.String(), "not a section") {
		t.Fatalf("--json failure is not one JSON value: %v\n%s", err, output.String())
	}
}
