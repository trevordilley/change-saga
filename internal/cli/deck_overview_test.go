package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

func TestDeckOverviewAuthorCheckAndQuery(t *testing.T) {
	t.Parallel()
	root, repo := coveredSaga(t)
	ctx := context.Background()
	var out bytes.Buffer
	if err := AddDeck(ctx, []string{"--feature", testFeature, "--objective", "Explain flow", root, "overview-test"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "second"} {
		if err := AddSlide(ctx, []string{"--deck", "overview-test", "--intent", "explain", "--layout", "diagram", "--front", "Authored summary", "--front", "Details on the back", root, id}, &out); err != nil {
			t.Fatal(err)
		}
	}
	out.Reset()
	err := Deck(ctx, []string{"overview", "--deck", "overview-test", "--check", "--json", root}, &out)
	var status *StatusError
	if !errors.As(err, &status) || status.Code != 3 {
		t.Fatalf("legacy check: %v %s", err, out.String())
	}
	var check saga.DeckOverviewReport
	if err := json.Unmarshal(out.Bytes(), &check); err != nil || !check.Generated || check.Complete || len(check.UncoveredSlides) != 2 {
		t.Fatalf("legacy output: %v %s", err, out.String())
	}
	doc, _, _ := saga.Load(root)
	deck := findDeck(doc, "overview-test")
	path := filepath.Join(root, filepath.FromSlash(deck.Path))
	before, _ := os.ReadFile(path)
	file := filepath.Join(t.TempDir(), "overview.json")
	overview := saga.DeckOverview{Body: "[First](annotation:a) and [Second](annotation:b).\n\n![Flow](slide:first)", Annotations: []saga.OverviewAnnotation{{ID: "a", Label: "First", Slide: "first"}, {ID: "b", Label: "Second", Slide: deck.Slides[1].Target}}}
	if err := store.WriteJSON(file, overview, true); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	args := []string{"overview", "--deck", deck.Target, "--file", file, "--json", root}
	if err := Deck(ctx, append([]string{"overview", "--dry-run"}, args[1:]...), &out); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("dry-run changed deck")
	}
	out.Reset()
	if err := Deck(ctx, args, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Deck(ctx, []string{"overview", "--deck", "overview-test", "--check", "--json", root}, &out); err != nil {
		t.Fatal(err, out.String())
	}
	if err := json.Unmarshal(out.Bytes(), &check); err != nil || !check.Complete || check.Generated {
		t.Fatalf("check: %v %+v", err, check)
	}
	doc, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %+v", err, validation)
	}
	if !reflect.DeepEqual(findDeck(doc, "overview-test").Overview, &overview) {
		t.Fatal("report did not round-trip")
	}
	out.Reset()
	if err := Query(ctx, []string{"overview", "--saga", root, "--repo", repo}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "annotation:a") || strings.Contains(out.String(), `"covered_slides"`) || !strings.Contains(out.String(), deck.Target) {
		t.Fatalf("directory must retain deck identity without report content: %s", out.String())
	}
	out.Reset()
	if err := Query(ctx, []string{"overview", "--deck", deck.Target, "--saga", root, "--repo", repo}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"covered_slides"`) || !strings.Contains(out.String(), `"annotation:a"`) && !strings.Contains(out.String(), "annotation:a") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := Query(ctx, []string{"slide", "--target", deck.Slides[0].Target, "--saga", root, "--repo", repo}, &out); err != nil {
		t.Fatal(err)
	}
	var slide struct {
		Data struct {
			Front []string `json:"front"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &slide); err != nil || !reflect.DeepEqual(slide.Data.Front, []string{"Authored summary", "Details on the back"}) {
		t.Fatalf("front query: %v %s", err, out.String())
	}
	before, _ = os.ReadFile(path)
	overview.Annotations[0].Slide = "urn:change-saga:foreign:slide:first"
	if err := store.WriteJSON(file, overview, false); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Deck(ctx, args, &out); err == nil {
		t.Fatal("invalid overview accepted")
	}
	after, _ = os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("invalid report changed deck")
	}
	// The strict input boundary never permits independent evidence on an annotation.
	writeFile(t, file, `{"body":"Report","annotations":[{"id":"a","label":"A","slide":"first","code":[]}]}`)
	if err := Deck(ctx, args, &out); err == nil {
		t.Fatal("independent overview evidence accepted")
	}
}

func TestFrontTransactionRoundTrip(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	request := slideTransactionRequest(t, repo, base, commit, sagaID, "front-create", "create", "absent", "node")
	request.Slide.Front = []string{"Run safely", "Returns without error"}
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false)
	if err != nil {
		t.Fatal(err)
	}
	doc, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	slide := findSlide(doc, "flow")
	current, err := currentSlideRequest(doc, slide)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current.Slide.Front, request.Slide.Front) {
		t.Fatal("print-current/repair dropped front")
	}
	request.Operation = "update"
	request.RequestID = "front-update"
	request.ExpectedSnapshot = created.Snapshot
	request.Slide.Front = []string{"Revised summary"}
	updated, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Snapshot == created.Snapshot || updated.Diff.AssetChanged || len(updated.ChangedIDs) != 1 {
		t.Fatalf("front update: %+v", updated)
	}
	request.RequestID = "front-invalid"
	request.ExpectedSnapshot = updated.Snapshot
	request.Slide.Front = []string{" "}
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, request, true); err == nil {
		t.Fatal("blank front accepted")
	}
}
