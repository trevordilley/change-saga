package server

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestERDEvidenceDrawerPreservesMemberAndRelationshipPins(t *testing.T) {
	t.Parallel()
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
	write := func(kind, id string, previous saga.DocumentationLink, def requirements.TechnicalDefinition) saga.DocumentationLink {
		t.Helper()
		var visual []byte
		if kind == requirements.KindERD {
			visual = []byte(reportingSVG)
		}
		result, err := requirements.WriteTechnicalRevision(ctx, root, "test", requirements.TechnicalWrite{Visual: visual, Kind: kind, ID: id, RevisionID: "evidence", Parents: []string{previous.Revision}, Definition: def, Repository: manifest.Source.Repository, Resolver: resolver})
		if err != nil {
			t.Fatal(err)
		}
		return saga.DocumentationLink{Target: result.URN, Revision: result.Created[0]}
	}
	entity := inventory.Pinned(pins["report"]).TechnicalDefinition
	commit := entity.Code[0].Commit
	edgeRef, err := resolver.Author(ctx, coderef.Location{Commit: commit, Path: "fixture/worker.go", Start: 5, End: 6}, "Relationship provenance.")
	if err != nil {
		t.Fatal(err)
	}
	entity.Relationships[0].Code = []requirements.Evidence{{ID: "production", Reference: edgeRef}}
	report := write(requirements.KindDataEntity, "pdf-report", pins["report"], entity)
	erd := inventory.Pinned(pins["reporting"]).TechnicalDefinition
	erd.Directory = []saga.DocumentationLink{report, pins["job-r2"]}
	erd.Bindings[0].Entity = &report
	diagram := write(requirements.KindERD, "reporting", pins["reporting"], erd)
	// A newer entity definition must never replace the ERD's saved member.
	entity.Name = "Do not substitute this newer definition"
	_, err = requirements.WriteTechnicalRevision(ctx, root, "test", requirements.TechnicalWrite{Kind: requirements.KindDataEntity, ID: "pdf-report", RevisionID: "newer", Parents: []string{report.Revision}, Definition: entity, Repository: manifest.Source.Repository, Resolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	// Code moved after authoring. Resolved does not change the inspected pin.
	for _, name := range []string{"report.go", "worker.go"} {
		path := filepath.Join(repo, "fixture", name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		writeServerFile(t, path, "// later context\n"+string(content))
	}
	serverGit(t, repo, "add", "fixture")
	serverGit(t, repo, "commit", "-m", "move declarations")
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	drawer := func(pin saga.DocumentationLink, relationship string) (int, string) {
		return technicalGet(t, mux, "/api/documentation?"+url.Values{"target": {pin.Target}, "revision": {pin.Revision}, "relationship": {relationship}}.Encode())
	}
	if saga.ValidDocumentationLink("test", diagram) {
		t.Fatal("ERD inspection must not widen Item documentation admission")
	}
	status, body := drawer(diagram, "")
	for _, want := range []string{`data-erd-evidence`, `2 entities · 2 relationships`, `2 references linked · 2 verified at their saved commits · 2 resolved`, `2 members have no code references`, `data-documentation-target="` + report.Target + `" data-documentation-revision="` + report.Revision + `"`, `data-source-relationship="produced-from-job"`, `definition pin stale`, `data-evidence-state="missing"`, `do not establish complete ERD coverage`} {
		if status != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("drawer missing %q: %d %s", want, status, body)
		}
	}
	// Follow the exact target/revision emitted on the entity button.
	status, body = drawer(report, "")
	for _, want := range []string{entity.Code[0].Location().String(), `data-file-path="fixture/report.go"`, `<th scope="row">4</th><td><code data-code>type Report struct`, `data-evidence-integrity="verified"`, `data-evidence-resolution="current"`} {
		if status != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("entity selection missing %q: %d %s", want, status, body)
		}
	}
	if strings.Contains(body, "Do not substitute") || strings.Contains(body, "later context") {
		t.Fatal("entity evidence drifted from saved definition or code")
	}
	// The same drawer resolves the selected relationship's own evidence.
	status, body = drawer(report, "produced-from-job")
	for _, want := range []string{`Exact code for relationship produced-from-job`, edgeRef.Location().String(), `data-file-path="fixture/worker.go"`, `<th scope="row">5</th><td><code data-code>`, `Relationship provenance.`} {
		if status != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("relationship selection missing %q: %d %s", want, status, body)
		}
	}
	if strings.Contains(body, `data-file-path="fixture/report.go"`) {
		t.Fatal("relationship evidence replaced with owner's code")
	}
	if status, _ := drawer(report, "missing-edge"); status != http.StatusNotFound {
		t.Fatalf("missing relationship silently substituted: %d", status)
	}
	if status, body := drawer(pins["job-r2"], "for-report"); status != http.StatusOK || !strings.Contains(body, `data-documentation-no-code`) {
		t.Fatalf("code-free relationship fabricated evidence: %d %s", status, body)
	}
}

func TestERDEvidenceSeparatesLinkedVerifiedResolvedAndMissing(t *testing.T) {
	t.Parallel()
	root, repo, pins := dataModelFixture(t)
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	revision := inventory.Pinned(pins["reporting"])
	a := &app{root: root, sourceDir: repo}
	check := func(linked, verified, resolved, stale, unavailable, unverified int) {
		t.Helper()
		view := a.erdEvidence(context.Background(), inventory, revision)
		want := erdEvidenceCounts{Linked: linked, Verified: verified, Resolved: resolved, Stale: stale, Unavailable: unavailable, Unverified: unverified}
		if view.Counts != want || view.Missing != 3 {
			t.Fatalf("evidence counts = %+v missing=%d, want %+v missing=3", view.Counts, view.Missing, want)
		}
	}
	check(1, 1, 1, 0, 0, 0)
	path := filepath.Join(repo, "fixture/report.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeServerFile(t, path, strings.Replace(string(content), "JobID string", "JobID int", 1))
	serverGit(t, repo, "add", "fixture/report.go")
	serverGit(t, repo, "commit", "-m", "change referenced bytes")
	check(1, 1, 0, 1, 0, 0) // original digest still verifies, source view is stale
	report := inventory.Pinned(pins["report"])
	originalDigest := report.Code[0].Digest
	report.Code[0].Digest = "sha256:" + strings.Repeat("0", 64)
	check(1, 0, 0, 1, 0, 1) // readable bytes do not imply integrity
	report.Code[0].Commit = strings.Repeat("f", 40)
	check(1, 0, 0, 0, 1, 0) // absent original commit is unavailable, never verified
	// Even finding matching content in HEAD cannot verify an absent saved commit.
	writeServerFile(t, path, string(content))
	serverGit(t, repo, "add", "fixture/report.go")
	serverGit(t, repo, "commit", "-m", "restore matching bytes")
	report.Code[0].Digest = originalDigest
	check(1, 0, 0, 0, 1, 0)
	a.sourceDir = t.TempDir()
	check(1, 0, 0, 0, 1, 0) // repository unavailable, saved links remain visible
	revision.Directory = append(revision.Directory, saga.DocumentationLink{Target: pins["report"].Target, Revision: pins["report"].Target + ":revision:absent"})
	view := a.erdEvidence(context.Background(), inventory, revision)
	if view.Entities != 3 || view.Missing != 4 || view.Members[len(view.Members)-1].PinStatus != "missing" {
		t.Fatalf("missing revision disappeared: %+v", view)
	}
}
