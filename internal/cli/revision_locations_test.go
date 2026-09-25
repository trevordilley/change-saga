package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/quality"
)

// Every command that accepts a code location resolves its revision the same
// way: HEAD, a branch, or an abbreviated commit pins the full commit.
func TestQualityEvidenceCodeAcceptsAnyRevision(t *testing.T) {
	t.Parallel()
	root := newQualityFixture(t)
	definition := `{"id":"deadline","title":"Reject at deadline","coverage_kinds":["negative"],"automation":"automated",
		"steps":[{"id":"submit","action":"Submit a refund request.","expected_result":"The request is rejected."}],
		"expected_result":"No refund is created."}`
	runQuality(t, definition, "test-case", "add", root, "--feature", testFeature, "--from", "-")
	codeRepo, pinned := qualityCode(t, "internal/refund_test.go")
	commit, _, _ := strings.Cut(pinned, ":")
	runQuality(t, "", "evidence", "add", root, "--test", qualityTestURN, "--role", "test_implementation", "--repo", codeRepo, "--code", "HEAD:internal/refund_test.go#L3-L12")
	document, err := quality.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	evidence := document.TestCases[0].Evidence
	if len(evidence) != 1 || len(evidence[0].Code) != 1 || evidence[0].Code[0].Location().String() != pinned {
		t.Fatalf("HEAD was not pinned to %s: %#v", commit, evidence)
	}
}

func TestCoverRefAcceptsAnyRevision(t *testing.T) {
	t.Parallel()
	root, repo := coveredSaga(t)
	output, err := runCover(t, "", "--repo", repo, "--target", "___overview/description.fragment", "--ref", "HEAD:internal/service/handler.go#L3", "--name", "head", root)
	if err != nil {
		t.Fatalf("cover --ref HEAD: %v\n%s", err, output)
	}
	if strings.Contains(output, "HEAD:") {
		t.Fatalf("cover recorded the symbolic revision instead of the commit:\n%s", output)
	}
}

// Reverse evidence lookup takes a current location: traceability --ref at
// HEAD finds evidence pinned at an older commit whose lines only moved.
func TestTraceabilityRefFindsEvidenceAtTheCurrentCommit(t *testing.T) {
	t.Parallel()
	root, repo := coveredSaga(t)
	var output bytes.Buffer
	if err := AddDeck(context.Background(), []string{"--feature", testFeature, "--objective", "Explain the change.", root, "implementation"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddSlide(context.Background(), []string{"--deck", "implementation", "--intent", "explain", "--layout", "diagram", root, "consts"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(context.Background(), []string{"--slide", "consts", "--kind", "callout", "--id", "a", "--element-id", "slide-title", "--description", "Constant A.", "--body", "A is one.", root}, &output); err != nil {
		t.Fatal(err)
	}
	if output, err := runCover(t, "", "--repo", repo, "--target", "urn:change-saga:batch:slide:consts:item:a", "--ref", "HEAD:internal/service/handler.go#L3", "--name", "a", root); err != nil {
		t.Fatalf("cover: %v\n%s", err, output)
	}
	writeFile(t, filepath.Join(repo, "internal", "service", "handler.go"), "package service\n\n// Moved down.\n// Twice.\nconst A = 1\nconst B = 2\nconst C = 3\n")
	git(t, repo, "commit", "-am", "move A")
	query := func(ref string) string {
		var out bytes.Buffer
		if err := Query(context.Background(), []string{"traceability", "--saga", root, "--repo", repo, "--ref", ref}, &out); err != nil {
			t.Fatalf("traceability --ref %s: %v\n%s", ref, err, out.String())
		}
		return out.String()
	}
	if found := query("HEAD:internal/service/handler.go#L5"); !strings.Contains(found, `"item":"urn:change-saga:batch:slide:consts:item:a"`) {
		t.Fatalf("the moved line did not find its evidence:\n%s", found)
	}
	if found := query("HEAD:internal/service/handler.go#L3"); !strings.Contains(found, `"unlinked_code_evidence":[]`) {
		t.Fatalf("a line the evidence no longer covers matched:\n%s", found)
	}
}
