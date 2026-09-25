package server

import (
	"net/http"
	"os"
	"path/filepath"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// technicalFixture writes a Component with two revisions and a System pinning
// its first revision, through the real inventory writer.
func technicalFixture(t *testing.T) (root, repo string) {
	t.Helper()
	root, repo = termSaga(t)
	commit := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	digest, err := coderef.DigestRange([]byte(serverKinds), 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	code := []coderef.Reference{{Commit: commit, Path: "kinds.go", Start: 6, End: 6, Digest: digest, Note: "Exact declaration provenance."}}
	store := "urn:change-saga:test:component:store"
	reader := "urn:change-saga:test:component:reader"
	for _, id := range []string{"store", "reader"} {
		if _, err := requirements.WriteTechnical(root, "test", "component", id, "r1", nil, requirements.TechnicalDefinition{Name: strings.ToUpper(id[:1]) + id[1:], Explanation: "The " + id + " of flag values.", Code: code}, true); err != nil {
			t.Fatal(err)
		}
	}
	system := requirements.TechnicalDefinition{Name: "Flags", Explanation: "Reads flags from the store.", Code: code,
		Components:   []saga.DocumentationLink{{Target: reader, Revision: reader + ":revision:r1"}, {Target: store, Revision: store + ":revision:r1"}},
		Interactions: []requirements.Interaction{{ID: "read", From: reader, To: store, Description: "Reads a flag value.", Code: code}}}
	if _, err := requirements.WriteTechnical(root, "test", "system", "flags", "r1", nil, system, true); err != nil {
		t.Fatal(err)
	}
	if _, err := requirements.WriteTechnical(root, "test", "component", "store", "r2", []string{store + ":revision:r1"}, requirements.TechnicalDefinition{Name: "FlagStore", Explanation: "The revised store.", Code: code}, false); err != nil {
		t.Fatal(err)
	}
	return root, repo
}

func technicalGet(t *testing.T, mux http.Handler, path string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	return rr.Code, rr.Body.String()
}

func TestTechnicalDesignPageListsDefinitionsWithoutInferringIntent(t *testing.T) {
	root, repo := technicalFixture(t)
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body := technicalGet(t, mux, "/technical")
	if status != 200 {
		t.Fatalf("technical page: %d %s", status, body)
	}
	for _, want := range []string{
		`data-technical-page`, `data-technical-kind="system"`, `data-technical-kind="component"`,
		`href="/technical/system/flags"`, `href="/technical/component/store"`, "FlagStore",
		`<span class="directory-gap">unspecified</span>`, `data-technical-data-model`,
		`data-directory-target="urn:change-saga:test:system:flags"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("technical page lost %s: %s", want, body)
		}
	}
	// The sidebar reaches every definition from the overview.
	if !strings.Contains(body, `href="/technical"`) || !strings.Contains(body, `nav-technical`) {
		t.Fatal("sidebar lost Technical design")
	}
	status, body = technicalGet(t, mux, "/technical?q=reader")
	if status != 200 || !strings.Contains(body, `data-directory-row="store" data-directory-text=`) || !strings.Contains(body, `hidden data-directory-row="store"`) {
		t.Fatalf("server filter did not hide non-matching rows: %s", body)
	}
	status, body = technicalGet(t, mux, "/")
	if status != 200 || !strings.Contains(body, `data-overview-part="Technical design"`) || !strings.Contains(body, "1 System · 2 Components") {
		t.Fatalf("overview directory lost Technical design: %s", body)
	}
}

func TestTechnicalEntityPageRendersExactPin(t *testing.T) {
	root, repo := technicalFixture(t)
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body := technicalGet(t, mux, "/technical/component/store")
	if status != 200 || !strings.Contains(body, `data-technical-revision="r2"`) || !strings.Contains(body, "<h1>FlagStore</h1>") || strings.Contains(body, `data-technical-pin-status`) {
		t.Fatalf("current definition: %d %s", status, body)
	}
	status, body = technicalGet(t, mux, "/technical/component/store?revision=r1")
	for _, want := range []string{`data-technical-revision="r1"`, "<h1>Store</h1>", `data-technical-pin-status="stale"`, "nothing has been repinned", `href="/technical/component/store">Read the current definition`, "Exact declaration provenance."} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("saved revision lost %s: %d %s", want, status, body)
		}
	}
	// A System's members open their own pinned pages, not the latest revision.
	status, body = technicalGet(t, mux, "/technical/system/flags")
	if status != 200 || !strings.Contains(body, `href="/technical/component/store?revision=r1"`) || strings.Contains(body, `data-documentation-target="urn:change-saga:test:component:store"`) {
		t.Fatalf("system members: %d %s", status, body)
	}
	for _, path := range []string{"/technical/component/store?revision=missing", "/technical/component/absent", "/technical/entity/store"} {
		if status, _ := technicalGet(t, mux, path); status != 404 {
			t.Fatalf("%s: %d", path, status)
		}
	}
	if _, err := requirements.SetTechnicalState(root, "test", "component", "store", "retired", "retired", "Replaced by the remote store", []string{"urn:change-saga:test:component:store:event:active"}); err != nil {
		t.Fatal(err)
	}
	status, body = technicalGet(t, mux, "/technical/component/store")
	if status != 200 || !strings.Contains(body, `data-technical-pin-status="retired"`) || !strings.Contains(body, "Replaced by the remote store") {
		t.Fatalf("retirement hidden: %d %s", status, body)
	}
}

func TestTechnicalEntityPageNamesCompetingHeads(t *testing.T) {
	root, repo := technicalFixture(t)
	// Simulate two branches that each revised r1 and then merged: a copy of
	// r2 renamed r3 is a second head with the same parent. No writer is asked
	// to choose a winner.
	revisions := filepath.Join(root, "___inventory", "components", "store.component", "revisions")
	data, err := os.ReadFile(filepath.Join(revisions, "r2.json"))
	if err != nil {
		t.Fatal(err)
	}
	copied := strings.Replace(strings.Replace(string(data), `"id": "r2"`, `"id": "r3"`, 1), `"name": "FlagStore"`, `"name": "BranchStore"`, 1)
	if copied == string(data) {
		t.Fatalf("fixture revision layout changed: %s", data)
	}
	writeServerFile(t, filepath.Join(revisions, "r3.json"), copied)
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body := technicalGet(t, mux, "/technical/component/store")
	if status != 200 || !strings.Contains(body, `data-technical-conflict`) || !strings.Contains(body, "None is chosen as current") || strings.Contains(body, "data-documentation-view") {
		t.Fatalf("conflict: %d %s", status, body)
	}
	for _, head := range []string{"r2", "r3"} {
		if !strings.Contains(body, `href="/technical/component/store?revision=`+head+`"`) {
			t.Fatalf("conflict hid head %s: %s", head, body)
		}
	}
	status, body = technicalGet(t, mux, "/technical/component/store?revision=r3")
	if status != 200 || !strings.Contains(body, `data-technical-pin-status="conflicted"`) || !strings.Contains(body, "<h1>BranchStore</h1>") {
		t.Fatalf("conflicted pin: %d %s", status, body)
	}
	status, body = technicalGet(t, mux, "/technical")
	if status != 200 || !strings.Contains(body, "2 competing revisions") {
		t.Fatalf("directory hid the conflict: %s", body)
	}
}

func TestTechnicalUsagesFollowItemPins(t *testing.T) {
	target := "urn:change-saga:test:component:store"
	pin := func(revision string) *saga.DocumentationLink {
		return &saga.DocumentationLink{Target: target, Revision: target + ":revision:" + revision}
	}
	item := func(slide, id string, link *saga.DocumentationLink) *saga.Item {
		return &saga.Item{ItemManifest: saga.ItemManifest{ID: id, Label: id, Documentation: link}, Target: "urn:change-saga:test:slide:" + slide + ":item:" + id}
	}
	deck := &saga.Deck{Target: "urn:change-saga:test:deck:flags", DeckManifest: saga.DeckManifest{Title: "Flags deck"}, Slides: []*saga.Slide{
		{Target: "urn:change-saga:test:slide:read", SlideManifest: saga.SlideManifest{Title: "Reading flags"}, Items: []*saga.Item{item("read", "store", pin("r1")), item("read", "other", nil)}},
		{Target: "urn:change-saga:test:slide:write", SlideManifest: saga.SlideManifest{Title: "Writing flags"}, Items: []*saga.Item{item("write", "store", pin("r2"))}},
	}}
	review := &saga.Review{ReviewManifest: saga.ReviewManifest{ID: "pr-7", Title: "Flag storage"}, Deck: &saga.Deck{DeckManifest: saga.DeckManifest{Title: "Review"}, Slides: []*saga.Slide{
		{Target: "urn:change-saga:test:review:pr-7:slide:store", SlideManifest: saga.SlideManifest{Title: "Store"}, Items: []*saga.Item{{ItemManifest: saga.ItemManifest{ID: "store", Label: "Store", Documentation: pin("r2")}, Target: "urn:change-saga:test:review:pr-7:slide:store:item:store"}}},
	}}}
	document := &saga.Saga{Section: &saga.Section{Target: "urn:change-saga:test"}, Decks: []*saga.Deck{deck}, Reviews: []*saga.Review{review},
		Features: []*saga.Feature{{ID: "core", Title: "Core", Decks: []*saga.Deck{deck}}}}
	inventory := requirements.Inventory{SagaID: "test", Records: []requirements.TechnicalRecord{{Kind: "component", Target: target,
		Revisions:       []requirements.TechnicalRevision{{ID: "r1"}, {ID: "r2"}},
		CurrentRevision: &requirements.TechnicalRevision{ID: "r2"}, CurrentLifecycle: &requirements.TechnicalEvent{State: "active"}}}}
	view := indexTechnicalUsages(document, inventory).view(target)
	if view.Total != 3 || view.Current != 2 || len(view.Usages) != 3 {
		t.Fatalf("usages: %#v", view)
	}
	first := view.Usages[0]
	if first.Status != "stale" || first.Feature != "Core" || first.Slide != "Reading flags" || first.PinHref != "/technical/component/store?revision=r1" {
		t.Fatalf("stale implementation usage: %#v", first)
	}
	last := view.Usages[2]
	if last.Context != "Review" || last.Href != "/reviews/pr-7#"+domID("urn:change-saga:test:review:pr-7:slide:store:item:store") || last.Status != "current" {
		t.Fatalf("review usage: %#v", last)
	}
}
