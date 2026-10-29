package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/requirements"
)

// Removing a story deletes it and every record that only linked to it, so
// nothing is left pointing at dead data, and lists the records with content of
// their own that lost what they covered. The Saga stays valid; status reports
// those records until their author adjusts them.
func TestStoryRemoveDeletesTheStoryAndItsLinksAndReportsWhatToAdjust(t *testing.T) {
	t.Parallel()
	root := newAuthoredSaga(t)
	checkout := addStory(t, root, testFeature, "checkout").Resource
	keep := addStory(t, root, testFeature, "browse").Resource
	mustLiving(t, "relation add", relationCommand, "add", root, "--feature", testFeature, "--id", "checkout-refines-browse",
		"--type", "refines", "--from", checkout+":criterion:fast", "--to", keep+":criterion:fast", "--rationale", "Checkout narrows browsing.")
	mustLiving(t, "relation add", relationCommand, "add", root, "--feature", testFeature, "--id", "browse-refines-browse",
		"--type", "refines", "--from", keep, "--to", keep+":criterion:fast", "--rationale", "Unrelated to checkout.")
	var output bytes.Buffer
	if err := Term(context.Background(), []string{"add", "--id", "cart", "--name", "Cart", "--definition", "What a buyer collects.", "--story", "checkout", "--json", root}, &output); err != nil {
		t.Fatalf("term add: %v\n%s", err, output.String())
	}
	mustLiving(t, "flag add", flagCommand, "add", root, "--id", "new-checkout", "--description", "Gates checkout", "--target", checkout, "--target", keep)
	runPrototype(t, "add-html", root, "--feature", testFeature, "--id", "cart-page", "--revision", "r1", "--title", "Cart", "--source", newPrototypeSource(t, `<button id="buy">Buy</button>`))
	runPrototype(t, "annotate", root, "--prototype", "urn:change-saga:atomic:prototype:cart-page", "--id", "buy",
		"--target", checkout+":criterion:fast", "--rationale", "Buying is prompt.", "--prototype-revision", "urn:change-saga:atomic:prototype:cart-page:revision:r1",
		"--story-revision", checkout+":revision:r1", "--element-id", "buy")
	assertValid(t, root)

	// The dry run names everything and writes nothing.
	output.Reset()
	if err := Story(context.Background(), []string{"remove", root, "--story", checkout, "--dry-run", "--json"}, &output); err != nil {
		t.Fatalf("story remove --dry-run: %v\n%s", err, output.String())
	}
	var preview storyRemoveOutput
	if err := json.Unmarshal(output.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	storyDir := filepath.Join(root, filepath.FromSlash(requirements.StoryPackagePath(testFeature, "checkout")))
	if _, err := os.Stat(storyDir); err != nil {
		t.Fatal("a dry run must not remove the story")
	}

	output.Reset()
	if err := Story(context.Background(), []string{"remove", root, "--story", "checkout", "--json"}, &output); err != nil {
		t.Fatalf("story remove: %v\n%s", err, output.String())
	}
	var removed storyRemoveOutput
	if err := json.Unmarshal(output.Bytes(), &removed); err != nil {
		t.Fatal(err)
	}
	if strings.Join(removed.Removed, "\n") != strings.Join(preview.Removed, "\n") || len(removed.Adjust) != len(preview.Adjust) {
		t.Fatalf("the dry run must preview the removal exactly:\npreview %#v\nremoved %#v", preview, removed)
	}
	joined := strings.Join(removed.Removed, "\n")
	for _, want := range []string{"stories/checkout.story", "relations/checkout-refines-browse.json", "annotations/cart-page.prototype/buy.json"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("removal missed %s:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "browse-refines-browse") || strings.Contains(joined, "browse.story") {
		t.Fatalf("removal reached records that never named the story:\n%s", joined)
	}
	adjust := map[string]bool{}
	for _, value := range removed.Adjust {
		adjust[value.Kind+" "+value.Record] = true
	}
	if !adjust["term urn:change-saga:atomic:term:cart"] || !adjust["flag urn:change-saga:atomic:flag:new-checkout"] {
		t.Fatalf("the term and flag that named the story must be listed to adjust: %#v", removed.Adjust)
	}
	if _, err := os.Stat(storyDir); !os.IsNotExist(err) {
		t.Fatal("the story directory must be gone")
	}
	assertValid(t, root)

	// Status keeps telling the author what to adjust until they do.
	status, err := livingapp.LoadStatus(context.Background(), livingapp.StatusOptions{SagaRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	reported := map[string]bool{}
	for _, value := range status.Dangling {
		reported[value.Record] = true
	}
	if !reported["urn:change-saga:atomic:term:cart"] || !reported["urn:change-saga:atomic:flag:new-checkout"] {
		t.Fatalf("status must report the records that still point at the removed story: %#v", status.Dangling)
	}
	if err := Story(context.Background(), []string{"remove", root, "--story", "checkout"}, &output); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("removing a removed story = %v", err)
	}
}
