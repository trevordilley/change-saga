package server

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/changeview"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/testfixture"
)

func TestNamedWhiteboardDocumentsAndIndependentGitContent(t *testing.T) {
	root, repo := termSaga(t)
	doc := diagram.Document{Version: 1, Width: 800, Height: 500, Elements: []diagram.Element{{ID: "area", Kind: "group", Shape: "section", Label: "Planning", Width: 700, Height: 400, Style: "normal"}, {ID: "step", Parent: "area", Kind: "node", Shape: "rect", Label: "Native plan", X: 30, Y: 60, Width: 200, Height: 100, Style: "normal"}}}
	publish := func(id, title string) string {
		t.Helper()
		value, err := requirements.PublishWhiteboard(context.Background(), root, "test", id, "", "", title, doc)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := requirements.WhiteboardContentDigest(*value)
		if err != nil {
			t.Fatal(err)
		}
		commitServerDocumentation(t, root)
		return digest
	}
	first := publish("architecture", "First architecture")
	journey := publish("journey", "Customer journey")
	current := publish("architecture", "Revised architecture")
	data := publish("data-model", "Shared data")
	nextData := publish("data-model", "Revised data")
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	for _, test := range []struct{ path, id, pin, title string }{
		{whiteboardHref("architecture", ""), "architecture", current, "Revised architecture"},
		{whiteboardFocusHref(whiteboardHref("architecture", first), "area"), "architecture", first, "First architecture"},
		{whiteboardHref("journey", ""), "journey", journey, "Customer journey"},
		{whiteboardHref("data-model", ""), "data-model", nextData, "Revised data"},
		{whiteboardHref("data-model", data), "data-model", data, "Shared data"},
	} {
		status, body := technicalGet(t, mux, test.path)
		for _, want := range []string{`data-whiteboard-id="` + test.id + `"`, `data-whiteboard-content-digest="` + test.pin + `"`, "<h1>" + test.title + "</h1>", `data-whiteboard-section="area"`} {
			if status != 200 || !strings.Contains(body, want) {
				t.Fatalf("%s lost %s: %d", test.path, want, status)
			}
		}
		if strings.Contains(test.path, "content_digest=") && !strings.Contains(body, "Saved whiteboard content from Git") {
			t.Fatalf("saved Git content was not identified: %s", test.path)
		}
		sidebar := technicalSidebar(t, body)
		if !navChildren(t, sidebar, "nav-technical") {
			t.Fatal("technical nav collapsed")
		}
	}
	for _, path := range []string{"/technical/whiteboards/architecture?revision=r1", "/technical/erd?revision=r1"} {
		if status, _ := technicalGet(t, mux, path); status != 410 {
			t.Fatalf("removed selector %s: %d", path, status)
		}
	}
	for _, path := range []string{"/technical/whiteboards/missing", whiteboardHref("architecture", "sha256:"+strings.Repeat("f", 64))} {
		if status, _ := technicalGet(t, mux, path); status != 404 {
			t.Fatalf("missing exact content %s: %d", path, status)
		}
	}
	_, body := technicalGet(t, mux, "/technical")
	for _, href := range []string{whiteboardHref("architecture", ""), whiteboardHref("journey", ""), whiteboardHref("data-model", "")} {
		if !strings.Contains(body, `href="`+href+`"`) {
			t.Fatal("missing board discovery", href)
		}
	}
	request := httptest.NewRequest("GET", "/technical/whiteboards/data-model?focus=area", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 302 || response.Header().Get("Location") != "/technical/erd?focus=area" {
		t.Fatal("canonical board route lost focus", response)
	}
	publish("journey", "Updated journey")
	if status, body := technicalGet(t, mux, "/technical"); status != 200 || !strings.Contains(body, "Updated journey") {
		t.Fatal("discovery ignored current edit")
	}
	if err := os.WriteFile(serverBoardCurrentPath(root, "journey"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if status, body := technicalGet(t, mux, whiteboardHref("architecture", "")); status != 200 || !strings.Contains(body, "Revised architecture") {
		t.Fatal("broken sibling hid valid board")
	}
}

func TestNamedWhiteboardSourceFocusPreservesDocumentAndRevision(t *testing.T) {
	root, repo := termSaga(t)
	if _, err := testfixture.WriteERDWhiteboardFixture(context.Background(), root, repo); err != nil {
		t.Fatal(err)
	}
	source, err := requirements.LoadWhiteboard(root, "test", "data-model", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := requirements.PublishWhiteboard(context.Background(), root, "test", "architecture", "r1", "", "Imported model in architecture", source.Document); err != nil {
		t.Fatal(err)
	}
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	board := (&app{root: root}).makeNamedWhiteboardView(inventory, "architecture", "")
	if got := whiteboardSourceDestination(board, "postgres"); got != "/technical/whiteboards/architecture?focus=cloud%2Fpostgres" {
		t.Fatal(got)
	}
	// The legacy ERD route still opens a source revision, not a board revision.
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	if status, body := technicalGet(t, mux, technicalHref("erd", "postgres", currentServerPin(t, root, "urn:change-saga:test:erd:postgres"))); status != 200 || !strings.Contains(body, `data-erd-view="urn:change-saga:test:erd:postgres"`) {
		t.Fatal("legacy source history lost")
	}
}

func TestNamedWhiteboardJavaScriptNavigationContract(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	prelude := `
const assert=require('node:assert/strict');
let location=new URL('https://local.test/technical/whiteboards/architecture?content_digest=r1&focus=area');
const pageScope=new AbortController(), listeners={};
const document={addEventListener:(type,callback)=>listeners[type]=callback};
const windowListeners={};
const addEventListener=(type,callback)=>windowListeners[type]=callback;
const pushes=[];
const history={pushState:(state,unused,url)=>{pushes.push(String(url));location=new URL(url);}};
class Link {
 constructor(href){this.attrs={href};this.dataset={};}
 getAttribute(key){return this.attrs[key] || null;}
 setAttribute(key,value){this.attrs[key]=value;}
 hasAttribute(key){return key in this.attrs;}
 closest(selector){return selector==='a[href],[data-documentation-target]' ? this : null;}
 matches(selector){return selector==='a[href]';}
}
const source=new Link('/technical/erd/sqlite?content_digest=r1');
const current=new Link('/technical/whiteboards/architecture');
const historical=new Link('/technical/whiteboards/architecture?content_digest=r2');
const other=new Link('/technical/whiteboards/journey?content_digest=r1');
const board={dataset:{whiteboardUrl:'/technical/whiteboards/architecture?content_digest=r1'},isConnected:true,listeners:{},addEventListener(type,callback){this.listeners[type]=callback;}};
const qa=(selector,root)=>selector==='[data-whiteboard-frame]' ? [{dataset:{whiteboardFrame:'local',whiteboardSourceHref:'/technical/erd/sqlite?content_digest=r1'}}] : selector==='[data-whiteboard-section]' ? [{dataset:{whiteboardSection:'area'}}] : selector==='a[href]' ? [source,current,historical,other] : [];
const q=()=>null;
const focused=[];
const focus=(path,all)=>{focused.push([path,Boolean(all)]);board.listeners['whiteboard:focus']?.({detail:{path,fitted:all}});};
const click=link=>{const event={button:0,target:link,preventDefault(){this.prevented=true;},stopImmediatePropagation(){}};listeners.click(event);return event.prevented;};
`
	checks := `
prepareWhiteboardNavigation(board,focus);
assert.deepEqual(focused.at(-1),['area',false]);
assert.equal(pushes.length,0);
assert.equal(source.getAttribute('href'),'/technical/whiteboards/architecture?content_digest=r1&focus=local');
assert.equal(source.dataset.whiteboardSourceHref,'/technical/erd/sqlite?content_digest=r1');
assert.equal(click(source),true);
assert.equal(location.pathname,'/technical/whiteboards/architecture');
assert.equal(location.searchParams.get('content_digest'),'r1');
assert.deepEqual(focused.at(-1),['local',false]);
for(const link of [current,historical,other])assert.equal(click(link),undefined,'another document/content_digest must be a page navigation');
location=new URL('https://local.test/technical/whiteboards/architecture?content_digest=r1&focus=area');
windowListeners.popstate();
assert.deepEqual(focused.at(-1),['area',false]);
assert.equal(pushes.length,1,'Back must not push another entry');
location=new URL('https://local.test/technical/whiteboards/journey?content_digest=r1&focus=local');
windowListeners.popstate();
assert.deepEqual(focused.at(-1),['area',false],'another board owns its own history');
pageScope.abort();
assert.equal(source.getAttribute('href'),'/technical/erd/sqlite?content_digest=r1');
`
	path := filepath.Join(t.TempDir(), "named-whiteboard.js")
	if err := os.WriteFile(path, []byte(prelude+whiteboardNavigationJavaScript+checks), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(node, path).CombinedOutput(); err != nil {
		t.Fatalf("navigation contract: %v\n%s", err, output)
	}
}

func TestWhiteboardHistoryComesFromGitCommits(t *testing.T) {
	root, _ := termSaga(t)
	doc := diagram.Document{Version: 1, Width: 600, Height: 400, Elements: []diagram.Element{{ID: "step", Kind: "node", Shape: "rect", Label: "Plan", X: 20, Y: 20, Width: 200, Height: 100, Style: "normal"}}}
	commits := map[string]bool{}
	for _, title := range []string{"Original planning", "Updated planning"} {
		if _, err := requirements.PublishWhiteboard(context.Background(), root, "test", "planning", "", "", title, doc); err != nil {
			t.Fatal(err)
		}
		commits[commitServerDocumentation(t, root)] = true
	}
	if _, err := requirements.PublishWhiteboard(context.Background(), root, "test", "other", "", "", "Another board", doc); err != nil {
		t.Fatal(err)
	}
	commitServerDocumentation(t, root)
	history, err := changeview.NodeHistory(context.Background(), root, "urn:change-saga:test:whiteboard:planning")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Events) != 2 {
		t.Fatalf("Git history lost commits or included another board: %#v", history.Events)
	}
	for _, event := range history.Events {
		if !commits[event.Head] {
			t.Fatalf("history fabricated a version: %#v", event)
		}
	}
}
