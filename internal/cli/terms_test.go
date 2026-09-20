package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/nextaction"
	"github.com/twentyideas/changesaga/internal/requirements"
)

const kindsGo = "package assessment\n\ntype Kind string\n\nconst (\n\tKindTesttaker Kind = \"testtaker\"\n\tKindProctor   Kind = \"proctor\"\n)\n"

// newTermSaga initializes a Saga documenting repo, with a story to name.
func newTermSaga(t *testing.T, repo string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "atomic.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &output); err != nil {
		t.Fatal(err)
	}
	addTestApp(t, root)
	if err := Story(context.Background(), []string{"add", "--feature", testFeature, "--id", "sit-assessment", "--revision", "r1", "--event", "proposed",
		"--title", "Sit an assessment", "--statement", "As a candidate, I sit an assessment", "--priority", "high", root}, &output); err != nil {
		t.Fatalf("story add: %v\n%s", err, output.String())
	}
	return root
}

func TestATermPinsItsCodeAndGoesStaleWhenTheCodeIsRenamed(t *testing.T) {
	repo, commit := sourceRepo(t, map[string]string{"kinds.go": kindsGo})
	root := newTermSaga(t, repo)
	var output bytes.Buffer
	if err := Term(context.Background(), []string{"add", "--id", "testtaker", "--name", "Testtaker", "--definition", "One sitting of an assessment.",
		"--alias", "test taker", "--story", "sit-assessment", "--ref", "HEAD:kinds.go#L6", "--repo", repo, "--json", root}, &output); err != nil {
		t.Fatalf("term add: %v\n%s", err, output.String())
	}
	assertValid(t, root)
	document, err := requirements.Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	term := document.FindTerm("testtaker")
	if term == nil || len(term.CurrentRevision.Code) != 1 || term.CurrentRevision.Code[0].Commit != commit ||
		term.CurrentRevision.Stories[0] != "urn:change-saga:atomic:story:sit-assessment" {
		t.Fatalf("term = %#v", term.CurrentRevision)
	}

	health := func() referencesOutput {
		t.Helper()
		var output bytes.Buffer
		if err := References(context.Background(), []string{"--repo", repo, "--allow-repository-mismatch", "--json", root}, &output); err != nil {
			t.Fatalf("references: %v\n%s", err, output.String())
		}
		var result referencesOutput
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if result := health(); result.Total != 1 || result.Current != 1 || result.References[0].Kind != "term" {
		t.Fatalf("a new term reference is current: %#v", result)
	}
	// A rename changes the defining line, so the reference goes stale and its
	// owner names exactly the term to update.
	writeFile(t, filepath.Join(repo, "kinds.go"), strings.Replace(kindsGo, "KindTesttaker", "KindCandidate", 1))
	git(t, repo, "commit", "-am", "rename testtaker")
	result := health()
	if result.Stale != 1 || result.References[0].Owner != "urn:change-saga:atomic:term:testtaker" {
		t.Fatalf("a renamed constant must stale the term that names it: %#v", result)
	}
	// Revising the term at the renamed line makes it current again.
	output.Reset()
	if err := Term(context.Background(), []string{"revise", "--term", "urn:change-saga:atomic:term:testtaker", "--revision", "r2",
		"--parent", "urn:change-saga:atomic:term:testtaker:revision:r1", "--name", "Testtaker", "--definition", "One sitting of an assessment.",
		"--ref", "HEAD:kinds.go#L6", "--repo", repo, root}, &output); err != nil {
		t.Fatalf("term revise: %v\n%s", err, output.String())
	}
	if result := health(); result.Stale != 0 || result.Current != 1 {
		t.Fatalf("a revised term is current: %#v", result)
	}
	if err := Term(context.Background(), []string{"add", "--id", "bad", "--name", "Bad", "--definition", "x", "--ref", "nope:kinds.go#L1", "--repo", repo, root}, &bytes.Buffer{}); err == nil {
		t.Fatal("an unknown revision must be refused")
	}
}

func statusOf(t *testing.T, root, repo string, rng gitdiff.Range) statusDocument {
	t.Helper()
	document, err := buildStatus(context.Background(), root, repo, rng, true, "")
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func actionsIn(document statusDocument, category nextaction.Category) []nextaction.Action {
	var result []nextaction.Action
	for _, action := range document.NextActions {
		if action.Category == category {
			result = append(result, action)
		}
	}
	return result
}

func TestAComparisonSuggestsNewTerminologyWithoutBlocking(t *testing.T) {
	repo, _ := sourceRepo(t, map[string]string{"kinds.go": kindsGo})
	root := newTermSaga(t, repo)
	if err := Term(context.Background(), []string{"add", "--id", "testtaker", "--name", "Testtaker", "--definition", "One sitting of an assessment.",
		"--story", "sit-assessment", "--ref", "HEAD:kinds.go#L6", "--repo", repo, root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	observed := statusOf(t, root, repo, gitdiff.Range{})
	if observed.Overview.Terms != 1 || len(observed.Terms) != 1 || observed.Terms[0].Stale || len(observed.NewTerminology) != 0 {
		t.Fatalf("observed terms = %#v overview %#v", observed.Terms, observed.Overview)
	}
	if strings.Join(observed.Overview.Gaps, ",") != "pitch" {
		t.Fatalf("the missing pitch is a gap and nothing else is: %v", observed.Overview.Gaps)
	}

	git(t, repo, "checkout", "-b", "grader")
	writeFile(t, filepath.Join(repo, "kinds.go"), strings.Replace(kindsGo, "\tKindProctor   Kind = \"proctor\"\n", "\tKindProctor   Kind = \"proctor\"\n\tKindGrader    Kind = \"grader\"\n\tmaxGraders         = 3\n", 1))
	git(t, repo, "commit", "-am", "add graders")
	compared := statusOf(t, root, repo, gitdiff.Range{Against: "main"})
	if len(compared.NewTerminology) != 1 || compared.NewTerminology[0].Name != "KindGrader" || compared.NewTerminology[0].Location.Start != 8 {
		t.Fatalf("new terminology = %#v", compared.NewTerminology)
	}
	growth := []nextaction.Action{}
	for _, action := range actionsIn(compared, nextaction.CategoryGrowth) {
		if strings.HasPrefix(action.ID, "growth:term:") {
			growth = append(growth, action)
		}
	}
	if len(growth) != 1 || growth[0].Question == nil || len(growth[0].Question.Options) != 2 || growth[0].Area != nextaction.AreaTerms ||
		!strings.Contains(growth[0].Reason, "new terminology") || compared.NextActions[len(compared.NextActions)-1].Category != nextaction.CategoryGrowth {
		t.Fatalf("growth = %#v", growth)
	}
	// The suggestion offers the domain word the identifier spells, and keeps
	// the identifier as the term's code reference.
	if compared.NewTerminology[0].Suggested != "grader" || !strings.Contains(growth[0].Reason, "Kind.KindGrader") {
		t.Fatalf("suggested name = %q, reason %q", compared.NewTerminology[0].Suggested, growth[0].Reason)
	}
	define := growth[0].Question.Options[0].Commands[0]
	if strings.Join(define.Argv, " ") != "change-saga term add --id grader --name grader --definition TEXT --ref "+compared.NewTerminology[0].Location.String()+" "+root {
		t.Fatalf("argv = %v", define.Argv)
	}
	for _, action := range compared.NextActions {
		if strings.Contains(action.Resource, ":term:") && action.Category != nextaction.CategoryGrowth {
			t.Fatalf("terminology must never be required: %#v", action)
		}
	}
	// Naming the new value in a term, even without referencing it, answers
	// the suggestion.
	if err := Term(context.Background(), []string{"add", "--id", "grader", "--name", "Grader", "--definition", "Scores a sitting.", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if again := statusOf(t, root, repo, gitdiff.Range{Against: "main"}); len(again.NewTerminology) != 0 {
		t.Fatalf("a named declaration is not new terminology: %#v", again.NewTerminology)
	}

	// Renaming the termed constant stales the term instead of suggesting a new one.
	writeFile(t, filepath.Join(repo, "kinds.go"), strings.Replace(kindsGo, "KindTesttaker", "KindCandidate", 1))
	git(t, repo, "commit", "-am", "rename testtaker")
	renamed := statusOf(t, root, repo, gitdiff.Range{Against: "main"})
	if len(renamed.NewTerminology) != 0 {
		t.Fatalf("a rename of a termed declaration is not new terminology: %#v", renamed.NewTerminology)
	}
	stale := []nextaction.Action{}
	for _, action := range actionsIn(renamed, nextaction.CategoryStale) {
		if strings.HasPrefix(action.ID, "stale:term:") {
			stale = append(stale, action)
		}
	}
	if len(stale) != 1 || stale[0].Resource != "urn:change-saga:atomic:term:testtaker" || stale[0].Area != nextaction.AreaHealth || stale[0].Command.Command != "term revise" {
		t.Fatalf("stale term action = %#v", stale)
	}
	inputs := []string{}
	for _, input := range stale[0].Command.Inputs {
		inputs = append(inputs, input.Flag)
	}
	if strings.Join(inputs, ",") != "revision,ref" {
		t.Fatalf("a stale term's revision is prefilled except its new id and code: %v", inputs)
	}
}

func queryData(t *testing.T, args ...string) map[string]any {
	t.Helper()
	var output bytes.Buffer
	if err := Query(context.Background(), args, &output); err != nil {
		t.Fatalf("query %v: %v\n%s", args, err, output.String())
	}
	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil || !envelope.OK {
		t.Fatalf("query %v: %v\n%s", args, err, output.String())
	}
	return envelope.Data
}

func termURNs(data map[string]any) string {
	values := []string{}
	for _, value := range data["terms"].([]any) {
		values = append(values, value.(map[string]any)["term"].(string))
	}
	return strings.Join(values, ",")
}

func TestALineOfCodeAndAStoryReachTheirTerms(t *testing.T) {
	repo, _ := sourceRepo(t, map[string]string{"kinds.go": kindsGo})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	root := newTermSaga(t, repo)
	if err := Term(context.Background(), []string{"add", "--id", "testtaker", "--name", "Testtaker", "--definition", "One sitting.",
		"--story", "sit-assessment", "--ref", "HEAD:kinds.go#L6", "--repo", repo, root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Term(context.Background(), []string{"add", "--id", "proctor", "--name", "Proctor", "--definition", "Watches a sitting.",
		"--ref", "HEAD:kinds.go#L7", "--repo", repo, root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	const testtaker, proctor = "urn:change-saga:atomic:term:testtaker", "urn:change-saga:atomic:term:proctor"
	if got := termURNs(queryData(t, "terms", "--saga", root, "--repo", repo, "--ref", "HEAD:kinds.go#L6")); got != testtaker {
		t.Fatalf("the constant's line reaches its term: %s", got)
	}
	if got := termURNs(queryData(t, "terms", "--saga", root, "--repo", repo, "--ref", "HEAD:kinds.go")); got != proctor+","+testtaker {
		t.Fatalf("a whole file reaches every term in it: %s", got)
	}
	if got := termURNs(queryData(t, "terms", "--saga", root, "--repo", repo, "--story", "sit-assessment")); got != testtaker {
		t.Fatalf("a story reaches the terms that belong to it: %s", got)
	}
	if got := termURNs(queryData(t, "terms", "--saga", root, "--repo", repo, "--term", "proctor")); got != proctor {
		t.Fatalf("--term = %s", got)
	}

	// In a comparison, a changed line lists the terms whose code contains it.
	git(t, repo, "checkout", "-b", "reword")
	writeFile(t, filepath.Join(repo, "kinds.go"), strings.Replace(kindsGo, `"proctor"`, `"invigilator"`, 1))
	git(t, repo, "commit", "-am", "reword proctor")
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	data := queryData(t, "diff-owners", "--saga", root, "--repo", repo, "--against", "main", "--ref", head+":kinds.go#L7")
	atoms := data["atoms"].([]any)
	if len(atoms) != 1 {
		t.Fatalf("atoms = %#v", atoms)
	}
	if terms := atoms[0].(map[string]any)["terms"].([]any); len(terms) != 0 {
		t.Fatalf("a changed line whose term reference went stale is not claimed by it at the head: %v", terms)
	}
	base := strings.TrimSpace(git(t, repo, "merge-base", "main", "HEAD"))
	data = queryData(t, "diff-owners", "--saga", root, "--repo", repo, "--against", "main", "--ref", base+":kinds.go#L7")
	if terms := data["atoms"].([]any)[0].(map[string]any)["terms"].([]any); len(terms) != 1 || terms[0] != proctor {
		t.Fatalf("the removed line names the term it defined: %v", terms)
	}
	// The changed lines are grouped under the term in the Code layer. (This
	// Saga is not in Git, so every record counts as added rather than
	// affected; the Code layer does not depend on that.)
	layers := queryData(t, "layers", "--saga", root, "--repo", repo, "--against", "main")
	grouped := false
	for _, value := range layers["code"].(map[string]any)["groups"].([]any) {
		grouped = grouped || value.(map[string]any)["urn"] == proctor
	}
	if !grouped {
		t.Fatalf("the changed line is grouped under the term that names it: %v", layers["code"])
	}
}
