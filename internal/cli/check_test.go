package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A first change with only its implementation covered: status reports every
// other area as a gap and exits zero; check answers each question it is asked
// and nothing else.
func TestCheckAnswersOnlyTheNamedAreas(t *testing.T) {
	t.Parallel()
	root, repo := coveredSaga(t)
	batch := `{"path":"internal/service/handler.go","changed_lines":true,"note":"the whole new file"}`
	if out, err := runCover(t, batch, "--repo", repo, "--batch", "-", root); err != nil {
		t.Fatalf("cover: %v\n%s", err, out)
	}
	ctx := context.Background()

	var status bytes.Buffer
	if err := Status(ctx, []string{"--against", "main", "--repo", repo, root}, &status); err != nil {
		t.Fatalf("status with gaps must exit zero: %v\n%s", err, status.String())
	}
	if !strings.Contains(status.String(), "implementation  6/6 changed lines referenced by the Saga (6 by the implementation deck or narrative, 0 by test-case evidence)") {
		t.Fatalf("status reports implementation coverage:\n%s", status.String())
	}

	var covered bytes.Buffer
	if err := Check(ctx, []string{"--covers", "implementation", "--against", "main", "--repo", repo, root}, &covered); err != nil {
		t.Fatalf("check --covers implementation = %v\n%s", err, covered.String())
	}

	var gaps bytes.Buffer
	var exit *StatusError
	err := Check(ctx, []string{"--covers", "implementation,stories", "--against", "main", "--repo", repo, root}, &gaps)
	if !errors.As(err, &exit) || exit.Code != checkExitUncovered {
		t.Fatalf("check --covers implementation,stories = %v, want exit %d\n%s", err, checkExitUncovered, gaps.String())
	}
	text := gaps.String()
	if !strings.Contains(text, "reaches no story") || strings.Contains(text, "no deck Item references") || strings.Contains(text, "personas") || strings.Contains(text, "Next actions") {
		t.Fatalf("check prints only the named areas' gaps:\n%s", text)
	}

	var document checkDocument
	var encoded bytes.Buffer
	_ = Check(ctx, []string{"--covers", "stories,implementation", "--json", "--against", "main", "--repo", repo, root}, &encoded)
	if err := json.Unmarshal(encoded.Bytes(), &document); err != nil {
		t.Fatalf("check --json: %v\n%s", err, encoded.String())
	}
	if document.Covered || len(document.Areas) != 2 || document.Areas[0].Area != "stories" || document.Areas[1].Area != "implementation" || document.Scope.Kind != "change" {
		t.Fatalf("check --json names the asked areas in order: %+v", document)
	}

	if err := Check(ctx, []string{"--covers", "tests", "--against", "main", "--repo", repo, root}, &bytes.Buffer{}); err == nil || errors.As(err, &exit) {
		t.Fatalf("an unknown area is a usage error, not a gap: %v", err)
	}
}

// Status exits non-zero only when it cannot produce a trustworthy report,
// such as when two features hold the same story ID.
func TestStatusFailsOnlyWhenTheReportCannotBeTrusted(t *testing.T) {
	t.Parallel()
	root, repo := coveredSaga(t)
	addStory(t, root, testFeature, "pay")
	mustLiving(t, "feature add", featureCommand, "add", root, "--id", "other", "--title", "Other")
	stories := filepath.Join(root, "___features", testFeature+".feature", "___requirements", "stories")
	duplicate := filepath.Join(root, "___features", "other.feature", "___requirements", "stories")
	if err := os.MkdirAll(filepath.Dir(duplicate), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(duplicate, os.DirFS(stories)); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := Status(context.Background(), []string{"--against", "main", "--repo", repo, root}, &output)
	var exit *StatusError
	if err == nil || errors.As(err, &exit) || !strings.Contains(err.Error(), "cannot be trusted") {
		t.Fatalf("status on a Saga with a duplicate story ID = %v\n%s", err, output.String())
	}
	if err := Check(context.Background(), []string{"--covers", "health", "--against", "main", "--repo", repo, root}, &bytes.Buffer{}); err == nil || errors.As(err, &exit) {
		t.Fatalf("check on a broken Saga cannot answer: %v", err)
	}
}

// The public quality gate uses the same kind assessment as detailed status,
// rather than treating any verifies link as sufficient coverage.
func TestCheckQualityRequiresKindsOrAnExplicitPolicy(t *testing.T) {
	t.Parallel()
	root, repo := coveredSaga(t)
	story := addStory(t, root, testFeature, "quality-kinds").Resource
	acceptStory(t, root, story)
	criterion := story + ":criterion:fast"
	addCase := func(kind string) {
		t.Helper()
		result := runQuality(t, "", "test-case", "add", root, "--feature", testFeature,
			"--id", kind+"-definition", "--title", kind+" verification", "--kind", kind, "--automation", "manual",
			"--step", `{"id":"assert","action":"Inspect the specified scenario","expected_result":"The specified outcome holds"}`,
			"--expected-result", "The specified outcome holds")
		mustLiving(t, "relation add", relationCommand, "add", root, "--feature", testFeature,
			"--id", kind+"-verifies", "--type", "verifies", "--from", result.Resource, "--to", criterion, "--rationale", "This scenario verifies the criterion.")
	}
	check := func(wantGap bool, reasons ...string) {
		t.Helper()
		var output bytes.Buffer
		err := Check(context.Background(), []string{"--repo", repo, "--covers", "quality", "--json", root}, &output)
		var exit *StatusError
		if wantGap {
			if !errors.As(err, &exit) || exit.Code != checkExitUncovered {
				t.Fatalf("quality gate should report a gap: %v\n%s", err, output.String())
			}
		} else if err != nil {
			t.Fatalf("quality gate should be covered: %v\n%s", err, output.String())
		}
		for _, reason := range reasons {
			if !strings.Contains(output.String(), reason) {
				t.Fatalf("gate omitted %q: %s", reason, output.String())
			}
		}
	}
	addCase("positive")
	check(true, "required negative test: missing_kind", "required edge test: missing_kind")
	addCase("negative")
	check(true, "required edge test: missing_kind")
	runQuality(t, "", "policy", "set", root, "--feature", testFeature, "--id", "no-boundary",
		"--criterion", criterion, "--story-revision", story+":revision:r1", "--require", "positive", "--require", "negative",
		"--rationale", "This fixed scenario has no boundary condition; success and rejection are distinct.")
	check(false)
	addCase("edge")
	// Superseding the exception restores the full requirement. All three
	// definitions still satisfy it without code evidence or run records.
	sagaPrefix := strings.Split(story, ":story:")[0]
	runQuality(t, "", "policy", "set", root, "--feature", testFeature, "--id", "all-kinds",
		"--criterion", criterion, "--story-revision", story+":revision:r1", "--require", "positive", "--require", "negative", "--require", "edge",
		"--supersedes", sagaPrefix+":quality-policy:no-boundary", "--rationale", "Boundary behavior is now explicitly defined.")
	check(false)
}
