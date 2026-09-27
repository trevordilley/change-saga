package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
	reviewserver "github.com/twentyideas/changesaga/internal/server"
)

// notedDiagram annotates an Item's element, an element no Item selects, and
// an edge, with Markdown and text HTML must escape.
func notedDiagram(d diagram.Document, notes map[string]string) diagram.Document {
	for index := range d.Elements {
		d.Elements[index].Note = notes[d.Elements[index].ID]
	}
	return d
}

func TestDiagramNotesRoundTripThroughImplementationSlides(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	d := notedDiagram(testDiagram(), map[string]string{
		"worker": "Runs **one job** at a time.",
		"store":  "Keeps `jobs` & <b>history</b>",
		"write":  "Retries *twice*, then:\n\n- logs\n- [alerts](https://example.com/alerts)",
	})
	invalid := diagramSlideRequest(t, repo, base, commit, sagaID, "noted-create", "create", "absent", d)
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, invalid, false); err == nil || !strings.Contains(err.Error(), "store: note uses raw HTML") {
		t.Fatalf("apply-slide must refuse raw HTML in a note: %v", err)
	}
	d.Elements[2].Note = "Keeps `jobs` & `<b>history</b>`"
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, diagramSlideRequest(t, repo, base, commit, sagaID, "noted-create", "create", "absent", d), false)
	if err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %#v", err, validation.Issues)
	}
	slide := document.Decks[0].Slides[0]
	source, _ := os.ReadFile(filepath.Join(slide.Directory, slide.Diagram.Source))
	if stored, err := diagram.Decode(source); err != nil || stored.Elements[1].Note != "Runs **one job** at a time." {
		t.Fatalf("stored source lost the note: %v", err)
	}
	asset, _ := os.ReadFile(filepath.Join(slide.Directory, slide.Entrypoint))
	if !strings.Contains(string(asset), "<desc>Keeps jobs &amp; &lt;b&gt;history&lt;/b&gt;</desc>") {
		t.Fatalf("SVG must carry the note as plain text:\n%s", asset)
	}

	text, err := runDiagram(t, "", "describe", "--slide", "flow", root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"  worker \"Worker\" shape=service icon=lucide:server\n    note: Runs **one job** at a time.\n",
		"  store \"Store\" shape=datastore\n    note: Keeps `jobs` & `<b>history</b>`\n",
		"    note: \"Retries *twice*, then:\\n\\n- logs\\n- [alerts](https://example.com/alerts)\"\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("describe lacks %q:\n%s", want, text)
		}
	}
	got, err := runDiagram(t, "", "get", "--slide", "flow", "--id", "worker", root)
	if err != nil || !strings.Contains(got, `"note": "Runs **one job** at a time."`) {
		t.Fatalf("get must return the note: %s %v", got, err)
	}

	edit := `[{"op":"update","id":"store","set":{"note":"Keeps every job for *30 days*."}},{"op":"update","id":"write","set":{"note":null}}]`
	output, err := runDiagram(t, edit, "edit", "--slide", "flow", "--expected", created.Snapshot, "--request-id", "renote", "--from", "OPS", "--repo", repo, "--json", root)
	if err != nil {
		t.Fatalf("diagram edit: %v\n%s", err, output)
	}
	var result DiagramEditResult
	if err := json.Unmarshal([]byte(output), &result); err != nil || strings.Join(result.ChangedElements, ",") != "store,write" || !result.Diff.DiagramChanged {
		t.Fatalf("edit result = %s err=%v", output, err)
	}
	if got, err := runDiagram(t, "", "get", "--slide", "flow", "--id", "store", root); err != nil || !strings.Contains(got, `"note": "Keeps every job for *30 days*."`) {
		t.Fatalf("edit did not set the note: %s %v", got, err)
	}
	if got, err := runDiagram(t, "", "get", "--slide", "flow", "--id", "write", root); err != nil || strings.Contains(got, `"note"`) {
		t.Fatalf("edit did not clear the note: %s %v", got, err)
	}
	if _, err := runDiagram(t, `[{"op":"update","id":"store","set":{"note":"# Heading"}}]`, "edit", "--slide", "flow", "--expected", result.Snapshot, "--request-id", "bad-note", "--from", "OPS", "--repo", repo, root); err == nil || !strings.Contains(err.Error(), "store: note uses headings") {
		t.Fatalf("edit must refuse an invalid note: %v", err)
	}

	// The Saga's deck viewer renders every note with the sanitizing Markdown
	// helper into inert templates beside the slide's landmarks.
	serverURL := serveSagaForTest(t, root, repo)
	fragment := getPage(t, serverURL+"/api/fragment?target="+url.QueryEscape(slide.Target))
	for _, want := range []string{
		`data-element-note-target data-element-id="store" data-element-name="Store" hidden><template data-landmark-note-template><div class="element-note"><div class="element-note-markdown"><p>Keeps every job for <em>30 days</em>.</p>`,
		`<p class="element-note-label">Worker</p><p class="element-note-description">The worker performs the operation.</p><div class="element-note-markdown"><p>Runs <strong>one job</strong> at a time.</p>`,
	} {
		if !strings.Contains(fragment, want) {
			t.Errorf("deck fragment lacks %s:\n%s", want, fragment)
		}
	}
	if strings.Contains(fragment, `data-element-id="write" data-element-name`) {
		t.Error("a cleared note must leave no note target")
	}
}

func TestApplySlidePublishesNotedReviewSlide(t *testing.T) {
	t.Parallel()
	fixture, _ := newEmptyReviewFixture(t)
	root, repo := fixture.root, fixture.repo
	d := notedDiagram(reviewDiagram(), map[string]string{
		"queue":  "Enqueue now opens a **transaction**.",
		"insert": "One `INSERT` per job; see [the design](https://example.com/design).",
	})
	created, err := ApplySlideTransaction(context.Background(), root, t.TempDir(), repo, reviewSlideRequest("noted-flow", "create", "absent", d), false)
	if err != nil {
		t.Fatal(err)
	}
	slide := loadReviewSlide(t, root, "flow")
	source, _ := os.ReadFile(filepath.Join(slide.Directory, slide.Diagram.Source))
	if stored, err := diagram.Decode(source); err != nil || stored.Elements[3].Note == "" {
		t.Fatalf("review source lost the note: %v", err)
	}
	text, err := runDiagram(t, "", "describe", "--review", "pr-7", "--slide", "flow", root)
	if err != nil || !strings.Contains(text, "  queue \"Enqueue\" shape=service icon=lucide:server\n    note: Enqueue now opens a **transaction**.\n") {
		t.Fatalf("review describe = %s err=%v", text, err)
	}
	if _, err := runDiagram(t, `[{"op":"update","id":"table","set":{"note":"[x](javascript:alert(1))"}}]`, "edit", "--review", "pr-7", "--slide", "flow", "--expected", created.Snapshot, "--request-id", "bad-link", "--from", "OPS", root); err == nil || !strings.Contains(err.Error(), `table: note link "javascript:alert(1)" must use http, https, or mailto`) {
		t.Fatalf("a review note with an unsafe link must be refused: %v", err)
	}

	serverURL := serveSagaForTest(t, root, repo)
	page := getPage(t, serverURL+"/reviews/pr-7")
	for _, want := range []string{
		// The Item's hotspot popover and its drawer panel both carry the note.
		`<p class="element-note-label">Enqueue</p><p class="element-note-description">Enqueue now names Postgres.</p><div class="element-note-markdown"><p>Enqueue now opens a <strong>transaction</strong>.</p>`,
		`</header><div class="element-note-markdown review-item-note"><p>Enqueue now opens a <strong>transaction</strong>.</p>`,
		// The edge has no Item, so it gets a note target of its own.
		`data-element-note-target data-element-id="insert" data-element-name="insert" hidden>`,
		`<p>One <code>INSERT</code> per job; see <a href="https://example.com/design">the design</a>.</p>`,
		// An Item without a note still shows its label and description.
		`<p class="element-note-label">Jobs table</p><p class="element-note-description">The table jobs are stored in.</p></div></template>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("review page lacks %s", want)
		}
	}
}

func serveSagaForTest(t *testing.T, root, repo string) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	served := make(chan error, 1)
	go func() {
		served <- reviewserver.ListenManaged(ctx, root, repo, "127.0.0.1:0", false, io.Discard, reviewserver.ManagedOptions{
			Range:   gitdiff.Range{},
			OnReady: func(url string) error { ready <- url; return nil },
		})
	}()
	var serverURL string
	select {
	case serverURL = <-ready:
	case err := <-served:
		t.Fatalf("server failed before ready: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("server did not become ready")
	}
	t.Cleanup(func() {
		cancel()
		<-served
	})
	return serverURL
}

func getPage(t *testing.T, address string) string {
	t.Helper()
	response, err := http.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d\n%s", address, response.StatusCode, body)
	}
	return string(body)
}

// The skill's review-slide example is what an agent copies, so it must stay a
// valid request whose diagram renders and demonstrates a note.
func TestSkillReviewSlideExampleRendersWithANote(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "skills", "change-saga", "references", "diagrams.md"))
	if err != nil {
		t.Fatal(err)
	}
	section := string(data)[strings.Index(string(data), "### Review deck slides"):]
	start := strings.Index(section, "```json\n") + len("```json\n")
	example := section[start : start+strings.Index(section[start:], "```")]
	var request SlideTransactionRequest
	if err := json.Unmarshal([]byte(example), &request); err != nil || request.Diagram == nil {
		t.Fatalf("example request: %v", err)
	}
	if _, err := diagram.Render(*request.Diagram, diagram.Options{Title: request.Slide.Title, Description: request.Slide.Takeaway}); err != nil {
		t.Fatalf("example diagram does not render: %v", err)
	}
	noted := false
	for _, element := range request.Diagram.Elements {
		noted = noted || element.Note != ""
	}
	if !noted {
		t.Fatal("the review-slide example should demonstrate a note")
	}
}
