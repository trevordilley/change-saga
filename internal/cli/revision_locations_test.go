package cli

import (
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/quality"
)

// Every command that accepts a code location resolves its revision the same
// way: HEAD, a branch, or an abbreviated commit pins the full commit.
func TestQualityEvidenceCodeAcceptsAnyRevision(t *testing.T) {
	root := newQualityFixture(t)
	definition := `{"id":"deadline","title":"Reject at deadline","coverage_kinds":["negative"],"automation":"automated",
		"steps":[{"id":"submit","action":"Submit a refund request.","expected_result":"The request is rejected."}],
		"expected_result":"No refund is created."}`
	runQuality(t, definition, "test-case", "add", root, "--epic", testEpic, "--from", "-")
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
	root, repo := coveredSaga(t)
	output, err := runCover(t, "", "--repo", repo, "--target", "___overview/description.fragment", "--ref", "HEAD:internal/service/handler.go#L3", "--name", "head", root)
	if err != nil {
		t.Fatalf("cover --ref HEAD: %v\n%s", err, output)
	}
	if strings.Contains(output, "HEAD:") {
		t.Fatalf("cover recorded the symbolic revision instead of the commit:\n%s", output)
	}
}
