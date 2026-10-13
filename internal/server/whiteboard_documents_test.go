package server

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/testfixture"
)

func TestNamedWhiteboardDocumentsAndIndependentHistory(t *testing.T) {
	root, repo := termSaga(t)
	document := diagram.Document{Version: 1, Width: 800, Height: 500, Elements: []diagram.Element{
		{ID: "area", Kind: "group", Shape: "section", Label: "Planning", Width: 700, Height: 400, Style: "normal"},
		{ID: "step", Parent: "area", Kind: "node", Shape: "rect", Label: "Native plan", X: 30, Y: 60, Width: 200, Height: 100, Style: "normal"},
	}}
	publish := func(id, revision, parent, name string) {
		t.Helper()
		if _, err := requirements.PublishWhiteboard(context.Background(), root, "test", id, revision, parent, name, document); err != nil {
			t.Fatal(err)
		}
	}
	publish("architecture", "r1", "", "First architecture")
	publish("journey", "r1", "", "Customer journey")
	publish("architecture", "r2", "r1", "Revised architecture")
	publish("data-model", "r1", "", "Shared data")
	publish("data-model", "r2", "r1", "Revised data")
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	for _, test := range []struct{ path, id, revision, title string }{
		{"/technical/whiteboards/architecture", "architecture", "r2", "Revised architecture"},
		{"/technical/whiteboards/architecture?revision=r1&focus=area", "architecture", "r1", "First architecture"},
		{"/technical/whiteboards/journey", "journey", "r1", "Customer journey"},
		{"/technical/erd", "data-model", "r2", "Revised data"},
		{"/technical/erd?revision=r1", "data-model", "r1", "Shared data"},
	} {
		status, body := technicalGet(t, mux, test.path)
		if status != 200 {
			t.Fatalf("%s: %d %s", test.path, status, body)
		}
		for _, want := range []string{`data-whiteboard-id="` + test.id + `"`, `data-whiteboard-revision="` + test.revision + `"`, "<h1>" + test.title + "</h1>", `data-whiteboard-section="area"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing %s", test.path, want)
			}
		}
		if strings.Contains(test.path, "revision=r1") && !strings.Contains(technicalSidebar(t, body), `focus=area&amp;revision=r1`) {
			t.Errorf("historical sidebar focus lost revision: %s", test.path)
		}
		if test.id != "data-model" && strings.Contains(body, `data-technical-data-model`) {
			t.Fatal("generic document presented as a data model")
		}
	}
	status, body := technicalGet(t, mux, "/technical/whiteboards/architecture?revision=r1")
	if status != 200 || !strings.Contains(body, `data-whiteboard-url="/technical/whiteboards/architecture?revision=r1"`) || !strings.Contains(body, `href="/technical/whiteboards/architecture?revision=r2"`) {
		t.Fatal("missing independent historical URLs")
	}
	_, body = technicalGet(t, mux, "/technical")
	for _, href := range []string{"/technical/whiteboards/architecture", "/technical/whiteboards/journey", "/technical/erd"} {
		if !strings.Contains(technicalSidebar(t, body), `href="`+href+`"`) {
			t.Errorf("board missing from navigation: %s", href)
		}
	}
	if !strings.Contains(body, `data-technical-whiteboards`) {
		t.Fatal("missing board discovery list")
	}
	for _, path := range []string{"/technical/whiteboards/missing", "/technical/whiteboards/architecture?revision=r99", "/technical/erd?revision=r99"} {
		if status, _ := technicalGet(t, mux, path); status != 404 {
			t.Errorf("missing document/revision %s: %d", path, status)
		}
	}
	request := httptest.NewRequest("GET", "/technical/whiteboards/data-model?revision=r1&focus=area", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 302 || response.Header().Get("Location") != "/technical/erd?revision=r1&focus=area" {
		t.Fatal("data model canonical route lost revision/focus", response)
	}
	publish("journey", "r2", "r1", "Updated journey")
	if status, body := technicalGet(t, mux, "/technical"); status != 200 || !strings.Contains(technicalSidebar(t, body), "Updated journey") {
		t.Fatal("discovery cache ignored board revision update")
	}
	// One broken document does not hide other board documents or their history.
	if err := os.WriteFile(filepath.Join(root, "___whiteboards", "journey", "revisions", "r1.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if status, body := technicalGet(t, mux, "/technical/whiteboards/architecture"); status != 200 || !strings.Contains(body, "Revised architecture") {
		t.Fatal("independent board hidden by broken sibling")
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
	board := (&app{root: root}).makeNamedWhiteboardView(inventory, "architecture", "r1")
	if got := whiteboardSourceDestination(board, "postgres"); got != "/technical/whiteboards/architecture?focus=cloud%2Fpostgres&revision=r1" {
		t.Fatal(got)
	}
	// The legacy ERD route still opens a source revision, not a board revision.
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	if status, body := technicalGet(t, mux, "/technical/erd/postgres?revision=r1"); status != 200 || !strings.Contains(body, `data-erd-revision="r1"`) {
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
let location=new URL('https://local.test/technical/whiteboards/architecture?revision=r1&focus=area');
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
const source=new Link('/technical/erd/sqlite?revision=r1');
const current=new Link('/technical/whiteboards/architecture');
const historical=new Link('/technical/whiteboards/architecture?revision=r2');
const other=new Link('/technical/whiteboards/journey?revision=r1');
const board={dataset:{whiteboardUrl:'/technical/whiteboards/architecture?revision=r1'},isConnected:true,listeners:{},addEventListener(type,callback){this.listeners[type]=callback;}};
const qa=(selector,root)=>selector==='[data-whiteboard-frame]' ? [{dataset:{whiteboardFrame:'local',whiteboardSourceHref:'/technical/erd/sqlite?revision=r1'}}] : selector==='[data-whiteboard-section]' ? [{dataset:{whiteboardSection:'area'}}] : selector==='a[href]' ? [source,current,historical,other] : [];
const q=()=>null;
const focused=[];
const focus=(path,all)=>{focused.push([path,Boolean(all)]);board.listeners['whiteboard:focus']?.({detail:{path,fitted:all}});};
const click=link=>{const event={button:0,target:link,preventDefault(){this.prevented=true;},stopImmediatePropagation(){}};listeners.click(event);return event.prevented;};
`
	checks := `
prepareWhiteboardNavigation(board,focus);
assert.deepEqual(focused.at(-1),['area',false]);
assert.equal(pushes.length,0);
assert.equal(source.getAttribute('href'),'/technical/whiteboards/architecture?revision=r1&focus=local');
assert.equal(source.dataset.whiteboardSourceHref,'/technical/erd/sqlite?revision=r1');
assert.equal(click(source),true);
assert.equal(location.pathname,'/technical/whiteboards/architecture');
assert.equal(location.searchParams.get('revision'),'r1');
assert.deepEqual(focused.at(-1),['local',false]);
for(const link of [current,historical,other])assert.equal(click(link),undefined,'another document/revision must be a page navigation');
location=new URL('https://local.test/technical/whiteboards/architecture?revision=r1&focus=area');
windowListeners.popstate();
assert.deepEqual(focused.at(-1),['area',false]);
assert.equal(pushes.length,1,'Back must not push another entry');
location=new URL('https://local.test/technical/whiteboards/journey?revision=r1&focus=local');
windowListeners.popstate();
assert.deepEqual(focused.at(-1),['area',false],'another board owns its own history');
pageScope.abort();
assert.equal(source.getAttribute('href'),'/technical/erd/sqlite?revision=r1');
`
	path := filepath.Join(t.TempDir(), "named-whiteboard.js")
	if err := os.WriteFile(path, []byte(prelude+whiteboardNavigationJavaScript+checks), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(node, path).CombinedOutput(); err != nil {
		t.Fatalf("navigation contract: %v\n%s", err, output)
	}
}
