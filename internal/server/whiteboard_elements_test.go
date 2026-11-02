package server

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestWhiteboardElementActionsStayPinnedAndCodeBecomesStale(t *testing.T) {
	ctx := context.Background()
	root, repo := termSaga(t)
	resolver, err := coderesolve.New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	commit := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	ref, err := resolver.Author(ctx, coderef.Location{Commit: commit, Path: "kinds.go", Start: 6, End: 6}, "Defines testtaker.")
	if err != nil {
		t.Fatal(err)
	}
	doc := diagram.Document{Version: 1, Width: 800, Height: 500, Elements: []diagram.Element{{ID: "owner", Kind: "node", Shape: "rect", X: 100, Y: 100, Width: 240, Height: 120, Style: "normal", Label: "Testtaker", Record: "urn:change-saga:test:feature:core", Evidence: []coderef.Reference{ref}}}}
	if _, err := requirements.PublishWhiteboard(ctx, root, "test", "architecture", "r1", "", "Architecture", doc); err != nil {
		t.Fatal(err)
	}
	request := url.Values{"board": {"architecture"}, "revision": {"r1"}, "element": {"owner"}}
	href := "/api/whiteboard-element?" + request.Encode()
	// Hover actions do not need the source checkout, nor eagerly render code.
	lazyMux := newMux(&app{root: root, sourceDir: filepath.Join(t.TempDir(), "absent"), template: serverTemplate(t)})
	status, body := technicalGet(t, lazyMux, href)
	if status != 200 || !strings.Contains(body, "data-target-code-href") || !strings.Contains(body, `href="/features/core"`) || strings.Contains(body, "data-file-path") {
		t.Fatalf("lazy actions %d: %s", status, body)
	}
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body = technicalGet(t, mux, "/technical/whiteboards/architecture")
	if status != 200 || !strings.Contains(body, `data-whiteboard-element="owner"`) || strings.Contains(body, `data-evidence-state="stale"`) {
		t.Fatalf("current reader %d: %s", status, body)
	}
	writeServerFile(t, filepath.Join(repo, "kinds.go"), strings.ReplaceAll(serverKinds, "testtaker", "candidate"))
	serverGit(t, repo, "add", "kinds.go")
	serverGit(t, repo, "commit", "-m", "Rename testtaker")
	mux = newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body = technicalGet(t, mux, "/technical/whiteboards/architecture")
	if status != 200 || !strings.Contains(body, `data-evidence-state="stale"`) || !strings.Contains(body, "1 stale element") {
		t.Fatalf("stale reader %d: %s", status, body)
	}
	status, body = technicalGet(t, mux, href+"&panel=code")
	if status != 200 || !strings.Contains(body, "source view: ") && !strings.Contains(body, `data-evidence-resolution="stale"`) || !strings.Contains(body, ref.Location().String()) || !strings.Contains(body, "testtaker") {
		t.Fatalf("saved stale code %d: %s", status, body)
	}
	// Live documented-code coverage includes the real board owner.
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := requirements.Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, owned := range collectDocumentedCode(document, records, quality.Document{}) {
		if owned.target == requirements.WhiteboardElementTarget("test", "architecture", "owner") {
			found = true
		}
	}
	if !found {
		t.Fatal("whiteboard missing from documented code")
	}
	request.Set("revision", "absent")
	if status, _ := technicalGet(t, mux, "/api/whiteboard-element?"+request.Encode()); status != 404 {
		t.Fatal("missing revision fell back to current")
	}
	if status, _ := technicalGet(t, mux, "/api/whiteboard-element?board=architecture&element=owner"); status != 400 {
		t.Fatal("unversioned actions accepted")
	}
}
