package server

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/testfixture"
)

func TestDiagramAnnotationExactPinsAndLazyCode(t *testing.T) {
	root, repo := termSaga(t)
	pins, err := testfixture.WriteERDWhiteboardFixture(context.Background(), root, repo)
	if err != nil {
		t.Fatal(err)
	}
	// A hover metadata request must work without reading a source checkout.
	mux := newMux(&app{root: root, sourceDir: filepath.Join(t.TempDir(), "missing-checkout"), template: serverTemplate(t)})
	request := func(pin saga.DocumentationLink, relationship, panel string) (int, string) {
		return technicalGet(t, mux, "/api/diagram-annotation?"+url.Values{"target": {pin.Target}, "revision": {pin.Revision}, "relationship": {relationship}, "panel": {panel}}.Encode())
	}
	status, body := request(pins["cloud-record"], "", "")
	if status != 200 || !strings.Contains(body, `class="landmark-affordance"`) || !strings.Contains(body, `data-documentation-revision="`+pins["cloud-record"].Revision+`"`) {
		t.Fatalf("metadata %d: %s", status, body)
	}
	if strings.Contains(body, "data-file-path") || strings.Contains(body, "data-open-stories") || strings.Contains(body, "data-documentation-item") {
		t.Fatal("metadata fabricated evidence or resolved code", body)
	}
	status, body = request(pins["cloud-record"], "sync", "")
	if status != 200 || !strings.Contains(body, `data-source-relationship="sync"`) {
		t.Fatalf("relationship %d: %s", status, body)
	}
	for _, panel := range []string{"", "code"} {
		if status, _ := request(pins["cloud-record"], "missing", panel); status != 404 {
			t.Fatalf("missing relationship fell back to owner: %d", status)
		}
		absent := pins["cloud-record"]
		absent.Revision = absent.Target + ":revision:missing"
		if status, _ := request(absent, "", panel); status != 404 {
			t.Fatalf("missing pin replaced: %d", status)
		}
	}
}

func TestDiagramAnnotationStoryProvenanceUsesRealExactItems(t *testing.T) {
	root := filepath.Join(t.TempDir(), "visual.saga")
	writeEmbeddedSlideFixture(t, root)
	document, validation, err := saga.LoadNarrative(root)
	if err != nil || !validation.Valid {
		t.Fatalf("fixture: %v", err)
	}
	slide := document.Decks[0].Slides[0]
	fragment := findFragmentByTarget(document, slide.Target)
	pin := saga.DocumentationLink{Target: "urn:change-saga:visual:data-entity:record", Revision: "urn:change-saga:visual:data-entity:record:revision:r1"}
	fragment.Landmarks[0].ItemMeta.Documentation = &pin
	story := "urn:change-saga:visual:story:checkout"
	records := requirements.Document{SagaID: "visual", Stories: []requirements.Story{{Identity: requirements.StoryIdentity{ID: "checkout"}, RevisionHeads: []string{story + ":revision:r1"}, CurrentRevision: &requirements.Revision{ID: "r1", Story: story, Title: "Safe checkout"}}}, Relations: []requirements.Relation{
		{ID: "real", Type: requirements.RelationExplains, From: fragment.Landmarks[0].Target, To: story, ToRevision: story + ":revision:r1", State: requirements.RelationActive, Scope: requirements.ScopeSelf},
		{ID: "broad", Type: requirements.RelationExplains, From: slide.Target, To: story, ToRevision: story + ":revision:r1", State: requirements.RelationActive, Scope: requirements.ScopeSelf},
	}}
	got, err := diagramAnnotationStories(document, records, pin, "example", "Saved record")
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 1 || len(got.Links) != 1 || len(got.Legacy) != 0 || got.Provenance == "" || !strings.HasPrefix(got.Links[0].ItemHref, "/?view=slides#") {
		t.Fatalf("provenance: %#v", got)
	}
	erdPin := saga.DocumentationLink{Target: "urn:change-saga:visual:erd:scope", Revision: "urn:change-saga:visual:erd:scope:revision:r1"}
	got, err = diagramAnnotationStories(document, records, erdPin, "scope", "Scope containing the record")
	if err != nil || got.Count != 0 {
		t.Fatalf("ERD inherited member Item story: %#v %v", got, err)
	}
	other := pin
	other.Revision = other.Target + ":revision:r2"
	got, err = diagramAnnotationStories(document, records, other, "other", "Saved record")
	if err != nil || got.Count != 0 {
		t.Fatalf("different revision inherited story: %#v %v", got, err)
	}
}

func TestDiagramAnnotationCodeRemainsLazyAndPinned(t *testing.T) {
	root, repo := termSaga(t)
	commit := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	digest, err := coderef.DigestRange([]byte(serverKinds), 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	definition := requirements.TechnicalDefinition{Name: "Original store", Explanation: "The shared typed implementation.", Code: []requirements.Evidence{{Reference: coderef.Reference{Commit: commit, Path: "kinds.go", Start: 6, End: 6, Digest: digest, Note: "Original saved evidence."}}}}
	target := "urn:change-saga:test:component:store"
	if _, err := requirements.WriteTechnical(root, "test", "component", "store", "r1", nil, definition, true); err != nil {
		t.Fatal(err)
	}
	definition.Name = "New store"
	definition.Code[0].Reference.Note = "New evidence note."
	if _, err := requirements.WriteTechnical(root, "test", "component", "store", "r2", []string{target + ":revision:r1"}, definition, false); err != nil {
		t.Fatal(err)
	}
	application := &app{root: root, sourceDir: filepath.Join(t.TempDir(), "missing-checkout"), template: serverTemplate(t)}
	mux := newMux(application)
	href := "/api/diagram-annotation?" + url.Values{"target": {target}, "revision": {target + ":revision:r1"}}.Encode()
	status, body := technicalGet(t, mux, href)
	if status != 200 || !strings.Contains(body, "Original store") || !strings.Contains(body, "data-target-code-href") || strings.Contains(body, "Original saved evidence") || strings.Contains(body, "data-file-path") || strings.Contains(body, "New store") {
		t.Fatalf("metadata resolved code or substituted current revision: %d %s", status, body)
	}
	application.sourceDir = repo
	status, body = technicalGet(t, mux, href+"&panel=code")
	for _, want := range []string{"data-target-code-response", "Original saved evidence.", `data-evidence-integrity="verified"`, "KindTesttaker"} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("pinned code missing %s: %d %s", want, status, body)
		}
	}
}

func TestDiagramAnnotationERDScopeUsesExactMembersAndLazyCode(t *testing.T) {
	root, repo, pins := dataModelFixture(t)
	ctx := context.Background()
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := coderesolve.New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	write := func(kind, id, revision string, previous saga.DocumentationLink, def requirements.TechnicalDefinition) saga.DocumentationLink {
		t.Helper()
		var visual []byte
		if kind == requirements.KindERD {
			visual = []byte(reportingSVG)
		}
		result, err := requirements.WriteTechnicalRevision(ctx, root, "test", requirements.TechnicalWrite{Kind: kind, ID: id, RevisionID: revision, Parents: []string{previous.Revision}, Definition: def, Visual: visual, Repository: manifest.Source.Repository, Resolver: resolver})
		if err != nil {
			t.Fatal(err)
		}
		return saga.DocumentationLink{Target: result.URN, Revision: result.Created[0]}
	}
	entity := inventory.Pinned(pins["report"]).TechnicalDefinition
	entity.Code[0].Note = "Entity ownership note."
	shared := entity.Code[0]
	shared.ID = "relationship-same-location"
	shared.Note = "Relationship ownership note."
	entity.Relationships[0].Code = []requirements.Evidence{shared}
	report := write(requirements.KindDataEntity, "pdf-report", "grouped", pins["report"], entity)
	erd := inventory.Pinned(pins["reporting"]).TechnicalDefinition
	if len(erd.Code) != 0 {
		t.Fatal("fixture ERD must have no own code")
	}
	erd.Directory = []saga.DocumentationLink{report, pins["job-r2"]}
	erd.Bindings[0].Entity = &report
	scope := write(requirements.KindERD, "reporting", "grouped", pins["reporting"], erd)
	entity.Name = "Newer entity must not replace saved member"
	entity.Code[0].Note = "Newer note must not leak"
	write(requirements.KindDataEntity, "pdf-report", "after-grouped", report, entity)
	// Git tracing detects source blob resolution during a metadata request.
	trace := filepath.Join(t.TempDir(), "git-trace")
	t.Setenv("GIT_TRACE", trace)
	application := &app{root: root, sourceDir: repo, template: serverTemplate(t)}
	mux := newMux(application)
	href := "/api/diagram-annotation?" + url.Values{"target": {scope.Target}, "revision": {scope.Revision}}.Encode()
	status, body := technicalGet(t, mux, href)
	if status != 200 || !strings.Contains(body, `Open 1 linked code references for ERD scope: Reporting data`) || !strings.Contains(body, "data-target-code-href") {
		t.Fatalf("ERD metadata %d: %s", status, body)
	}
	for _, forbidden := range []string{"data-file-path", "Entity ownership note", "data-open-stories", "data-documentation-item"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metadata leaked %s: %s", forbidden, body)
		}
	}
	traced, err := os.ReadFile(trace)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if strings.Contains(string(traced), "cat-file") {
		t.Fatalf("metadata read source blobs: %s", traced)
	}
	status, body = technicalGet(t, mux, href+"&panel=code")
	for _, want := range []string{`data-target-code-count="1"`, `data-diagram-erd-scope`, `Broader pinned ERD scope`, `data-evidence-member="` + report.Revision + `"`, `data-source-relationship="produced-from-job"`, `definition pin stale`, `Entity ownership note.`, `Relationship ownership note.`, `Members without linked code`, `No code references are recorded for this member.`, `data-evidence-integrity="verified"`} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("ERD code missing %q: %d %s", want, status, body)
		}
	}
	if strings.Count(body, `data-diagram-evidence-group`) != 1 || strings.Count(body, `data-file-path="fixture/report.go"`) != 1 || strings.Contains(body, "Newer") {
		t.Fatalf("dedup or exact provenance failed: %s", body)
	}
	traced, err = os.ReadFile(trace)
	if err != nil || !strings.Contains(string(traced), "cat-file") {
		t.Fatalf("code panel did not resolve saved bytes: %v %s", err, traced)
	}
	if status, _ := technicalGet(t, mux, href+"&relationship=produced-from-job"); status != 404 {
		t.Fatalf("ERD inherited a member relationship as its own: %d", status)
	}
	for _, panel := range []string{"", "code"} {
		status, _ = technicalGet(t, mux, "/api/diagram-annotation?"+url.Values{"target": {scope.Target}, "revision": {scope.Target + ":revision:absent"}, "panel": {panel}}.Encode())
		if status != 404 {
			t.Fatalf("absent ERD revision substituted: %d", status)
		}
	}
}

func TestDiagramERDScopeMissingMembersAndSharedProvenance(t *testing.T) {
	root, _, pins := dataModelFixture(t)
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	revision := inventory.Pinned(pins["reporting"])
	report := inventory.Pinned(pins["report"])
	shared := report.Code[0]
	shared.Note = "Second member cites the same saved bytes."
	job := inventory.Pinned(pins["job-r2"])
	job.Code = []requirements.Evidence{shared, shared} // Repeated location/owner/note must not duplicate its provenance.
	job.Relationships[0].Code = []requirements.Evidence{shared}
	missing := saga.DocumentationLink{Target: pins["report"].Target, Revision: pins["report"].Target + ":revision:absent"}
	revision.Directory = append(revision.Directory, missing, pins["job-r2"])
	// A referenced destination outside the directory is never followed.
	report.Relationships[0].Destination = saga.DocumentationLink{Target: "urn:change-saga:test:data-entity:outside", Revision: "urn:change-saga:test:data-entity:outside:revision:r1"}
	inventory.Find(pins["job-r2"].Target).CurrentRevision = nil // Honest unresolved/conflicting head status.
	scope := diagramERDEvidence(inventory, revision)
	if len(scope.Groups) != 1 || len(scope.Groups[0].Owners) != 3 {
		t.Fatalf("shared ownership lost or duplicated: %+v", scope)
	}
	if len(scope.WithoutCode) != 2 || scope.WithoutCode[1].Pin != missing || scope.WithoutCode[1].PinStatus != "missing" {
		t.Fatalf("missing member lost: %+v", scope.WithoutCode)
	}
	if scope.Groups[0].Owners[1].PinStatus != "conflicted" {
		t.Fatalf("unknown head status hidden: %+v", scope.Groups[0].Owners)
	}
	var body strings.Builder
	err = serverTemplate(t).ExecuteTemplate(&body, "diagram-annotation-code", diagramAnnotationCodeView{targetCodeView: targetCodeView{DOMID: "scope", Title: "Broader scope", ReferenceCount: 1}, Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{missing.Revision, `data-evidence-pin-status="missing"`, `Nothing was substituted.`, `definition pin conflicted`, `Second member cites the same saved bytes.`} {
		if !strings.Contains(body.String(), want) {
			t.Fatalf("missing provenance %q: %s", want, body.String())
		}
	}
	if strings.Contains(body.String(), "outside:revision:r1") {
		t.Fatal("recursed into relationship destination")
	}
	revision.Directory = []saga.DocumentationLink{missing}
	onlyMissing := diagramERDEvidence(inventory, revision)
	if len(onlyMissing.Groups) != 0 || len(onlyMissing.WithoutCode) != 1 || onlyMissing.WithoutCode[0].PinStatus != "missing" {
		t.Fatalf("absent member fabricated code from a current revision: %+v", onlyMissing)
	}
}
