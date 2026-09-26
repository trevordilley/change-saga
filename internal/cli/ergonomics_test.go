package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"

	"github.com/twentyideas/changesaga/internal/saga"
)

// landmarkSaga returns a saga with one Markdown fragment carrying a heading
// landmark, plus the repository its comparison reads.
func landmarkSaga(t *testing.T) (root, repo string) {
	t.Helper()
	root, repo = coveredSaga(t)
	var output bytes.Buffer
	if err := AddChapter(context.Background(), []string{"--feature", testFeature, "--title", "Service", root, "service"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddFragment(context.Background(), []string{"--feature", testFeature, "--section", "service", "--name", "overview", "--title", "Service", root}, &output); err != nil {
		t.Fatal(err)
	}
	fragment := filepath.Join(testFeatureDir(root), "service.chapter", "overview.fragment")
	writeFile(t, filepath.Join(fragment, "content.md"), "# Service {#service-intro}\n\nProse.\n\n## Submit action {#submit-action}\n\nMore.\n")
	writeFile(t, filepath.Join(fragment, "___landmarks", "submit-action.landmark", "landmark.json"),
		`{"version":2,"id":"submit-action","label":"Submit action","selector":{"type":"heading","heading_id":"submit-action"}}`+"\n")
	assertValid(t, root)
	return root, repo
}

// A landmark lives inside a reserved directory that ordinary path resolution
// refuses to enter, so the shorthand is the only ergonomic way to name one
// without first knowing its full URN.
func TestCoverResolvesLandmarkShorthand(t *testing.T) {
	t.Parallel()
	root, repo := landmarkSaga(t)
	if output, err := runCover(t, "", "--repo", repo,
		"--target", testFeatureRel+"/service.chapter/overview.fragment#submit-action",
		"--path", "internal/service/handler.go", "--side", "new", "--lines", "3", "--name", "submit", root); err != nil {
		t.Fatalf("landmark shorthand: %v\n%s", err, output)
	}
	recorded := filepath.Join(testFeatureDir(root), "service.chapter", "overview.fragment", "___landmarks", "submit-action.landmark", saga.CodeDirName, "submit.json")
	if _, err := os.Stat(recorded); err != nil {
		t.Fatalf("evidence was not attached to the landmark: %v", err)
	}
	assertValid(t, root)

	report, err := buildReport(context.Background(), root, repo, gitdiff.Range{Against: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Targets) != 1 || !strings.Contains(report.Targets[0].Target, ":landmark:submit-action") {
		t.Fatalf("coverage should belong to the landmark: %#v", report.Targets)
	}
}

func TestCoverLandmarkErrorsNameTheAvailableLandmarks(t *testing.T) {
	t.Parallel()
	root, repo := landmarkSaga(t)
	_, err := runCover(t, "", "--repo", repo,
		"--target", testFeatureRel+"/service.chapter/overview.fragment#no-such-landmark",
		"--path", "internal/service/handler.go", "--side", "new", "--lines", "3", root)
	if err == nil {
		t.Fatal("an unknown landmark must fail")
	}
	if !strings.Contains(err.Error(), "submit-action") {
		t.Fatalf("error %q does not list the landmarks the fragment declares", err)
	}

	// A fragment with no landmarks at all should say where to create one rather
	// than leaving the author to guess the directory layout.
	_, err = runCover(t, "", "--repo", repo,
		"--target", "___overview/description.fragment#anything",
		"--path", "internal/service/handler.go", "--side", "new", "--lines", "3", root)
	if err == nil || !strings.Contains(err.Error(), "___landmarks/anything.landmark/landmark.json") {
		t.Fatalf("error %q does not explain how to declare the landmark", err)
	}
}

// An unresolvable target is the most common authoring mistake. The error has to
// point at the supported way to enumerate targets, not invite file spelunking.
func TestResolveTargetErrorPointsAtTheQueryAPI(t *testing.T) {
	t.Parallel()
	root, _ := landmarkSaga(t)
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = resolveTarget(document, testFeatureRel+"/service.chapter/missing", true)
	if err == nil {
		t.Fatal("expected an unresolvable target to fail")
	}
	for _, want := range []string{"change-saga query children", "Known targets:", ":landmark:submit-action"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q is missing %q", err, want)
		}
	}
}

func TestUnknownTargetURNIsAlsoExplained(t *testing.T) {
	t.Parallel()
	root, _ := landmarkSaga(t)
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = resolveTarget(document, "urn:change-saga:batch:fragment:ghost", true)
	if err == nil || !strings.Contains(err.Error(), "change-saga query children") {
		t.Fatalf("an unknown URN must be explained too, got %v", err)
	}
}

// "change-saga open -h" used to print "Usage of serve:", naming a command the
// user did not type and whose flags differ in default.
func TestCommandHelpNamesTheInvokedCommand(t *testing.T) {
	t.Parallel()
	var open bytes.Buffer
	if err := Serve(context.Background(), []string{"-h"}, &open, true); err == nil {
		t.Fatal("-h must report flag.ErrHelp so the process exits zero without serving")
	}
	if !strings.Contains(open.String(), "change-saga open ") {
		t.Fatalf("open -h did not describe open:\n%s", open.String())
	}
	if strings.Contains(open.String(), "Usage of serve") || strings.Contains(open.String(), "change-saga serve") {
		t.Fatalf("open -h still advertises serve:\n%s", open.String())
	}
	if strings.Contains(open.String(), "detach") {
		t.Fatalf("open -h exposes an internal lifecycle detail:\n%s", open.String())
	}
	if !strings.Contains(open.String(), "managed loopback reviewer") {
		t.Fatalf("open -h does not explain that the reviewer remains available:\n%s", open.String())
	}

	var serve bytes.Buffer
	if err := Serve(context.Background(), []string{"-h"}, &serve); err == nil {
		t.Fatal("-h must report flag.ErrHelp")
	}
	if !strings.Contains(serve.String(), "change-saga serve ") {
		t.Fatalf("serve -h did not describe serve:\n%s", serve.String())
	}

	var slide bytes.Buffer
	if err := AddSlide(context.Background(), []string{"-h"}, &slide); err == nil {
		t.Fatal("-h must report flag.ErrHelp")
	}
	for _, expected := range []string{"system model", "tradeoffs", "hidden coupling", "surprise a reviewer"} {
		if !strings.Contains(slide.String(), expected) {
			t.Fatalf("add-slide -h omitted %q:\n%s", expected, slide.String())
		}
	}
}

func TestOpenIsManagedByDefaultAndAcceptsLegacyDetachFlag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		args       []string
		want       []string
		background bool
	}{
		{name: "plain", args: []string{"review.saga"}, want: []string{"review.saga"}, background: true},
		{name: "legacy true", args: []string{"--detach", "review.saga"}, want: []string{"review.saga"}, background: true},
		{name: "legacy false", args: []string{"--detach=false", "review.saga"}, want: []string{"review.saga"}, background: false},
		{name: "legacy after option value", args: []string{"--repo", "/tmp/source", "--detach", "review.saga"}, want: []string{"--repo", "/tmp/source", "review.saga"}, background: true},
		{name: "positional", args: []string{"--", "--detach"}, want: []string{"--", "--detach"}, background: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, background, err := normalizeLegacyOpenDetach(test.args)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got, "\x00") != strings.Join(test.want, "\x00") || background != test.background {
				t.Fatalf("normalizeLegacyOpenDetach(%q) = %q, %t; want %q, %t", test.args, got, background, test.want, test.background)
			}
		})
	}
	if _, _, err := normalizeLegacyOpenDetach([]string{"--detach=maybe", "review.saga"}); err == nil {
		t.Fatal("invalid legacy detach value must fail")
	}
}

// A command with no flags produced a bare "Usage of install-skill:" banner and
// nothing else, which told the reader neither what it does nor how to use it.
func TestInstallSkillHelpExplainsTheCommand(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := InstallSkill([]string{"-h"}, &output); err == nil {
		t.Fatal("-h must report flag.ErrHelp")
	}
	text := output.String()
	if !strings.Contains(text, "change-saga install-skill") {
		t.Fatalf("install-skill -h omitted its usage line:\n%s", text)
	}
	if !strings.Contains(text, "prompt") {
		t.Fatalf("install-skill -h did not say what it prints:\n%s", text)
	}
	if strings.Contains(text, "Flags:") {
		t.Fatalf("a flagless command must not print an empty flag section:\n%s", text)
	}
}

func TestTopLevelHelpListsEveryDispatchedCommand(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	PrintHelp(&output)
	for _, command := range commandOrder {
		usage, ok := commandUsage[command]
		if !ok {
			t.Fatalf("command %q has no usage line", command)
		}
		if !strings.Contains(output.String(), usage) {
			t.Fatalf("help omits %q:\n%s", command, output.String())
		}
	}
}

func TestTopLevelHelpRecommendsTheAuthoringSkill(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	PrintHelp(&output)
	text := output.String()
	for _, want := range []string{
		"Using a coding agent?",
		"change-saga install-skill",
		"agent-agnostic bootstrap",
		"does not modify the repository or create a Saga",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("top-level help omitted %q:\n%s", want, text)
		}
	}
}

// The help teaches scope-sensitive adoption: requirements-first and
// implementation-first are both valid when they match the user's work.
func TestTopLevelHelpDescribesIncrementalAdoption(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	PrintHelp(&output)
	text := output.String()
	for _, want := range []string{
		"one Saga per repository, change.saga", "monorepo", "Do what the user asks", "smallest scope", "focused code review",
		"valuable user stories", "accepted", "at least\none pass/fail criterion", "narrowest obligation",
		"implementation evidence", "implementation change", "cover",
		"reconcile --against main", "no verdict", "check --covers implementation",
		"Product:", "prototype", "user stories",
		"Design:", "UX, UI, and technical design",
		"Quality:", "test cases",
		"dependency-aware waves",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("top-level help omitted workflow guidance %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "A focused change:") > strings.Index(text, "Growing the Saga") {
		t.Fatalf("the focused change comes before growth:\n%s", text)
	}
	for _, unwanted := range []string{"The one thing asked", "never asked for up front", "big change", "starts with the big work", "Choose the workflow", "normal PR may be enough", "--mode", "upgrade"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("top-level help still offers %q:\n%s", unwanted, text)
		}
	}
}

func TestLivingCommandHelpExplainsParallelWorkflow(t *testing.T) {
	t.Parallel()
	checks := []struct {
		name string
		run  func() string
		want []string
	}{
		{name: "init", run: func() string {
			var output bytes.Buffer
			_ = Init(context.Background(), []string{"-h"}, &output)
			return output.String()
		}, want: []string{"repository's Saga", "change.saga", "one Saga per repository", "___overview", "cover the change", "implementation deck", "document existing code", "optional"}},
		{name: "story add", run: func() string {
			var output bytes.Buffer
			_ = Story(context.Background(), []string{"add", "-h"}, &output)
			return output.String()
		}, want: []string{"acceptance-criteria", "evolve alongside"}},
		{name: "story set-state", run: func() string {
			var output bytes.Buffer
			_ = Story(context.Background(), []string{"set-state", "-h"}, &output)
			return output.String()
		}, want: []string{"Before setting accepted", "at least one pass/fail criterion", "narrowest"}},
		{name: "criterion add", run: func() string {
			var output bytes.Buffer
			_ = Criterion(context.Background(), []string{"add", "-h"}, &output)
			return output.String()
		}, want: []string{"independent pass/fail criterion", "narrowest", "confirmed intent"}},
		{name: "plan add-wave", run: func() string {
			var output bytes.Buffer
			_ = Plan(context.Background(), []string{"add-wave", "-h"}, &output)
			return output.String()
		}, want: []string{"parallel workspace lanes", "converge", "not dependency"}},
		{name: "plan add-item", run: func() string {
			var output bytes.Buffer
			_ = Plan(context.Background(), []string{"add-item", "-h"}, &output)
			return output.String()
		}, want: []string{"independently assignable", "parallel work"}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			text := check.run()
			for _, want := range check.want {
				if !strings.Contains(text, want) {
					t.Fatalf("help omitted %q:\n%s", want, text)
				}
			}
		})
	}
}

// The installed skill is the contract an agent follows. It has to route reads
// through the versioned query API and name every operation, or an agent will
// fall back to reading saga files whose layout is not a compatibility promise.
func TestInstallSkillRoutesAgentsThroughTheQueryAPI(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := InstallSkill(nil, &output); err != nil {
		t.Fatal(err)
	}
	prompt := output.String()
	for _, operation := range queryOperations {
		if !strings.Contains(prompt, "`"+operation+"`") {
			t.Fatalf("the installed skill never names the %q operation", operation)
		}
		if !strings.Contains(prompt, queryUsage[operation]) {
			t.Fatalf("the installed skill omits the usage for %q", operation)
		}
		if strings.TrimSpace(queryPurpose[operation]) == "" {
			t.Fatalf("operation %q has no purpose description", operation)
		}
	}
	for _, want := range []string{
		"change-saga query",
		"error.code",
		"page.next_cursor",
		"--batch",
		"--dry-run",
		"<fragment>#<landmark-id>",
		"Never widen",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("the installed skill omits %q", want)
		}
	}
}

func TestValidateFixAddsMissingHeadingAnchors(t *testing.T) {
	t.Parallel()
	root, _ := coveredSaga(t)
	fragment := overviewFragment(root)
	writeFile(t, filepath.Join(fragment, "content.md"), "# Overview\n\nProse.\n\n## Risks\n")

	var before bytes.Buffer
	if err := Validate(context.Background(), []string{root}, &before); err != nil {
		t.Fatalf("a saga with unanchored headings is still valid: %v", err)
	}
	if !strings.Contains(before.String(), "should declare a stable anchor") {
		t.Fatalf("expected anchor warnings before the fix:\n%s", before.String())
	}

	var fixed bytes.Buffer
	if err := Validate(context.Background(), []string{"--fix", "--json", root}, &fixed); err != nil {
		t.Fatalf("validate --fix: %v\n%s", err, fixed.String())
	}
	var result struct {
		Valid  bool         `json:"valid"`
		Issues []saga.Issue `json:"issues"`
		Fixes  []AnchorFix  `json:"fixes"`
	}
	if err := json.Unmarshal(fixed.Bytes(), &result); err != nil {
		t.Fatalf("validate --fix --json is not one JSON value: %v\n%s", err, fixed.String())
	}
	if len(result.Fixes) != 2 {
		t.Fatalf("expected two anchors to be added: %#v", result.Fixes)
	}
	if result.Fixes[0].Path != "___overview/description.fragment/content.md" || result.Fixes[0].Anchor != "overview" {
		t.Fatalf("unexpected fix record: %#v", result.Fixes[0])
	}
	for _, issue := range result.Issues {
		if strings.Contains(issue.Message, "stable anchor") {
			t.Fatalf("the anchor warning survived --fix: %#v", issue)
		}
	}
	content, err := os.ReadFile(filepath.Join(fragment, "content.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "# Overview {#overview}") || !strings.Contains(string(content), "## Risks {#risks}") {
		t.Fatalf("headings were not anchored:\n%s", content)
	}
	assertValid(t, root)
}

// --fix must be a no-op on an already-anchored saga, so it is safe to run in a
// loop or a pre-handoff check without generating churn.
func TestValidateFixIsANoOpWhenNothingIsMissing(t *testing.T) {
	t.Parallel()
	root, _ := coveredSaga(t)
	writeFile(t, filepath.Join(overviewFragment(root), "content.md"), "# Overview {#overview}\n")
	var output bytes.Buffer
	if err := Validate(context.Background(), []string{"--fix", "--json", root}, &output); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Fixes []AnchorFix `json:"fixes"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Fixes) != 0 {
		t.Fatalf("nothing should have been fixed: %#v", result.Fixes)
	}
}

// The end-to-end shape matters as much as the in-memory one: an agent polls
// status --json until "uncovered" is empty, and a null there is a crash.
// Status has no verdict: this Saga records no story, and status still exits
// zero because its report can be trusted.
func TestStatusJSONReportsEmptyCollectionsOnSuccess(t *testing.T) {
	t.Parallel()
	root, repo := coveredSaga(t)
	// One record references the add event and the added lines exactly, so
	// nothing overlaps.
	batch := `{"path":"internal/service/handler.go","changed_lines":true,"note":"the whole new file"}`
	if out, err := runCover(t, batch, "--repo", repo, "--batch", "-", root); err != nil {
		t.Fatalf("cover: %v\n%s", err, out)
	}

	var output bytes.Buffer
	if err := Status(context.Background(), []string{"--against", "main", "--json", "--repo", repo, root}, &output); err != nil {
		t.Fatalf("status with no story = %v, want exit 0\n%s", err, output.String())
	}
	var report map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("status --json is not one JSON value: %v\n%s", err, output.String())
	}
	if string(report["complete"]) != "true" {
		t.Fatalf("expected a complete saga:\n%s", output.String())
	}
	for _, field := range []string{"uncovered", "overlaps", "stale_references", "saga_changes", "schema_issues"} {
		value, present := report[field]
		if !present || string(value) != "[]" {
			t.Fatalf("status --json %q = %s (present=%v), want []", field, value, present)
		}
	}
}
