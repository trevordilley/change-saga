package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	var preview removalOutput
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
	var removed removalOutput
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

// Every record kind with a remove follows the same rule: it and the links that
// exist only for it are deleted, the Saga stays valid, and what referenced it
// is reported to adjust.
func TestRemovingEachRecordKindCleansItsLinksAndReportsTheRest(t *testing.T) {
	t.Parallel()
	root := newAuthoredSaga(t)
	ctx := context.Background()
	persona := mustLiving(t, "persona add", personaCommand, "add", root, "--id", "buyer", "--name", "Buyer", "--description", "Buys things").Resource
	citation := mustLiving(t, "citation add", func(ctx context.Context, args []string, out *bytes.Buffer) error { return Citation(ctx, args, out) },
		"add", root, "--feature", testFeature, "--id", "interview", "--kind", "url", "--title", "Interview", "--reference", "https://example.test/interview").Resource
	var output bytes.Buffer
	if err := Story(ctx, []string{"add", root, "--feature", testFeature, "--id", "pay", "--revision", "r1", "--event", "proposed", "--title", "Pay",
		"--statement", "As a buyer I can pay", "--priority", "must", "--criterion", "fast=It finishes promptly", "--persona", persona, "--citation", citation, "--json"}, &output); err != nil {
		t.Fatalf("story add: %v\n%s", err, output.String())
	}
	story := "urn:change-saga:atomic:story:pay"
	testCase := runQuality(t, "", "test-case", "add", root, "--feature", testFeature, "--id", "pay-check", "--title", "Pay check", "--kind", "positive", "--automation", "manual",
		"--step", `{"id":"pay","action":"Pay","expected_result":"Paid"}`, "--expected-result", "Paid").Resource
	mustLiving(t, "relation add", relationCommand, "add", root, "--feature", testFeature, "--id", "pay-check-verifies",
		"--type", "verifies", "--from", testCase, "--to", story+":criterion:fast", "--rationale", "It checks paying.")
	output.Reset()
	if err := Term(ctx, []string{"add", "--id", "wallet", "--name", "Wallet", "--definition", "Holds money.", "--json", root}, &output); err != nil {
		t.Fatalf("term add: %v\n%s", err, output.String())
	}
	output.Reset()
	if err := Term(ctx, []string{"add", "--id", "checkout", "--name", "Checkout", "--definition", "Paying.", "--record", "urn:change-saga:atomic:term:wallet", "--json", root}, &output); err != nil {
		t.Fatalf("term add: %v\n%s", err, output.String())
	}
	runPrototype(t, "add-html", root, "--feature", testFeature, "--id", "pay-page", "--revision", "r1", "--title", "Pay", "--source", newPrototypeSource(t, `<button id="pay">Pay</button>`))
	runPrototype(t, "annotate", root, "--prototype", "urn:change-saga:atomic:prototype:pay-page", "--id", "pay", "--target", story,
		"--rationale", "Paying.", "--prototype-revision", "urn:change-saga:atomic:prototype:pay-page:revision:r1", "--story-revision", story+":revision:r1", "--element-id", "pay")
	assertValid(t, root)

	remove := func(run func(context.Context, []string, io.Writer) error, args ...string) removalOutput {
		t.Helper()
		var output bytes.Buffer
		if err := run(ctx, append(args, "--json"), &output); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output.String())
		}
		var result removalOutput
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		assertValid(t, root)
		return result
	}
	adjusts := func(result removalOutput, record string) bool {
		for _, value := range result.Adjust {
			if value.Record == record {
				return true
			}
		}
		return false
	}

	if result := remove(Prototype, "remove", root, "--prototype", "pay-page"); !strings.Contains(strings.Join(result.Removed, "\n"), "annotations/pay-page.prototype") {
		t.Fatalf("a prototype's annotations go with it: %v", result.Removed)
	}
	if result := remove(Quality, "test-case", "remove", root, "--test-case", testCase); !strings.Contains(strings.Join(result.Removed, "\n"), "relations/pay-check-verifies.json") {
		t.Fatalf("a test case's verifies relation goes with it: %v", result.Removed)
	}
	if result := remove(Persona, "remove", root, "--persona", persona); !adjusts(result, story) {
		t.Fatalf("a story serving a removed persona must be listed to adjust: %#v", result.Adjust)
	}
	if result := remove(Citation, "remove", root, "--citation", citation); !adjusts(result, story) {
		t.Fatalf("a story citing a removed citation must be listed to adjust: %#v", result.Adjust)
	}
	if result := remove(Term, "remove", root, "--term", "wallet"); !adjusts(result, "urn:change-saga:atomic:term:checkout") {
		t.Fatalf("a term naming a removed term must be listed to adjust: %#v", result.Adjust)
	}
}

// feature remove refuses a feature with content unless --with-content, which
// deletes it with everything in it, drops it from themes, and lists what other
// records still referenced its stories.
func TestFeatureRemoveWithContentDeletesTheFeatureAndReportsWhatOthersLost(t *testing.T) {
	t.Parallel()
	root := newAuthoredSaga(t)
	ctx := context.Background()
	mustLiving(t, "feature add", featureCommand, "add", root, "--id", "billing", "--title", "Billing")
	pay := addStory(t, root, "billing", "pay").Resource
	if err := FeatureTheme(ctx, []string{"add", root, "--id", "money", "--title", "Money", "--text", "Getting paid.", "--feature", "billing", "--feature", testFeature}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Term(ctx, []string{"add", "--id", "invoice", "--name", "Invoice", "--definition", "A bill.", "--story", "pay", "--json", root}, &output); err != nil {
		t.Fatalf("term add: %v\n%s", err, output.String())
	}
	assertValid(t, root)

	output.Reset()
	if err := Feature(ctx, []string{"remove", root, "--feature", "billing"}, &output); err == nil || !strings.Contains(err.Error(), "--with-content") {
		t.Fatalf("removing a feature with content must point at --with-content: %v", err)
	}
	output.Reset()
	if err := Feature(ctx, []string{"remove", root, "--feature", "billing", "--with-content", "--json"}, &output); err != nil {
		t.Fatalf("feature remove --with-content: %v\n%s", err, output.String())
	}
	var result removalOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Changed) != 1 || !strings.Contains(result.Changed[0], "money.theme/theme.json") {
		t.Fatalf("the theme must stop listing the removed feature: %#v", result.Changed)
	}
	found := false
	for _, value := range result.Adjust {
		if value.Record == "urn:change-saga:atomic:term:invoice" && len(value.Missing) == 1 && value.Missing[0] == pay {
			found = true
		}
	}
	if !found {
		t.Fatalf("the term naming the feature's story must be listed to adjust: %#v", result.Adjust)
	}
	if _, err := os.Stat(filepath.Join(root, "___features", "billing.feature")); !os.IsNotExist(err) {
		t.Fatal("the feature directory must be gone")
	}
	assertValid(t, root)
}
