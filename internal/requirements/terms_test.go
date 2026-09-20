package requirements

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
)

const (
	testTermURN  = "urn:change-saga:test:term:testtaker"
	testStoryURN = "urn:change-saga:test:story:checkout"
)

func testtakerCode() coderef.Reference {
	return coderef.Reference{Commit: strings.Repeat("a", 40), Path: "internal/assessment/kinds.go", Start: 12, End: 12, Digest: "sha256:" + strings.Repeat("b", 64)}
}

func testtakerInput() AddTermInput {
	return AddTermInput{ID: "testtaker", RevisionID: "r1", EventID: "active", CreatedAt: testTime, RequestID: "add-testtaker",
		TermDefinition: TermDefinition{
			Name: "Testtaker", Definition: "One sitting of an assessment, not the person taking it.",
			Aliases: []string{"test taker", "sitting"}, Stories: []string{testStoryURN},
			Records: []string{"urn:change-saga:test:persona:buyer"}, Code: []coderef.Reference{testtakerCode()},
		}}
}

func TestATermIsALivingRecordThatNamesItsStoriesRecordsAndCode(t *testing.T) {
	root := newSaga(t)
	if _, err := AddStory(root, "test", storyInput("checkout", "r1", "proposed", nil)); err != nil {
		t.Fatal(err)
	}
	result, err := AddTerm(root, "test", testtakerInput())
	if err != nil {
		t.Fatal(err)
	}
	if result.URN != testTermURN || result.Path != "___overview/terms/testtaker.term" {
		t.Fatalf("result = %+v", result)
	}
	replay, err := AddTerm(root, "test", testtakerInput())
	if err != nil || !replay.Replayed {
		t.Fatalf("replaying the same request is a no-op: %+v, %v", replay, err)
	}
	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	term := document.FindTerm("testtaker")
	if term == nil || !term.Active() || term.CurrentRevision == nil || term.CurrentRevision.Name != "Testtaker" || len(term.CurrentRevision.Code) != 1 {
		t.Fatalf("term = %+v", term)
	}
	naming := document.TermsNaming()
	if got := naming[testStoryURN]; len(got) != 1 || got[0] != testTermURN {
		t.Fatalf("a story reaches the terms that name it: %v", naming)
	}
	if got := naming["urn:change-saga:test:persona:buyer"]; len(got) != 1 {
		t.Fatalf("a persona reaches the terms that name it: %v", naming)
	}

	// A revision is a complete snapshot from every current head.
	revise := ReviseTermInput{Term: testTermURN, ID: "r2", Parents: []string{}, CreatedAt: testTime,
		TermDefinition: TermDefinition{Name: "Testtaker", Definition: "One sitting of an assessment."}}
	if _, err := ReviseTerm(root, "test", revise); err == nil || !strings.Contains(err.Error(), "every current head") {
		t.Fatalf("a revision without its parent head was accepted: %v", err)
	}
	revise.Parents = []string{testTermURN + ":revision:r1"}
	if _, err := ReviseTerm(root, "test", revise); err != nil {
		t.Fatal(err)
	}
	if _, err := SetTermState(root, "test", SetTermStateInput{Term: testTermURN, ID: "retired", Parents: []string{testTermURN + ":event:active"}, State: TermRetired, Reason: "renamed to session", CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	document, err = Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	term = document.FindTerm("testtaker")
	if term.Active() || term.CurrentRevision.ID != "r2" || len(term.CurrentRevision.Stories) != 0 {
		t.Fatalf("revised and retired term = %+v", term.CurrentRevision)
	}
	if len(document.TermsNaming()) != 0 {
		t.Fatal("a term names only what its current revision names")
	}
}

func TestATermRefusesLinksThatDoNotExistAndMalformedContent(t *testing.T) {
	root := newSaga(t)
	cases := map[string]func(*AddTermInput){
		"story \"urn:change-saga:test:story:checkout\" does not exist": func(input *AddTermInput) {},
		"persona \"urn:change-saga:test:persona:ghost\" does not exist": func(input *AddTermInput) {
			input.Stories, input.Records = nil, []string{"urn:change-saga:test:persona:ghost"}
		},
		"cannot name itself": func(input *AddTermInput) { input.Stories, input.Records = nil, []string{testTermURN} },
		"repeats the name":   func(input *AddTermInput) { input.Aliases = []string{"testtaker"} },
		"definition is required": func(input *AddTermInput) {
			input.Definition = " "
		},
		"canonical persona, feature, flag, or term URN": func(input *AddTermInput) {
			input.Stories, input.Records = nil, []string{"urn:change-saga:test:story:checkout"}
		},
		"duplicates": func(input *AddTermInput) {
			input.Stories, input.Records = nil, nil
			input.Code = []coderef.Reference{testtakerCode(), testtakerCode()}
		},
		"digest": func(input *AddTermInput) {
			input.Stories, input.Records = nil, nil
			code := testtakerCode()
			code.Digest = "md5:x"
			input.Code = []coderef.Reference{code}
		},
	}
	for want, edit := range cases {
		input := testtakerInput()
		input.RequestID = ""
		edit(&input)
		if _, err := AddTerm(root, "test", input); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "___overview", "terms", "testtaker.term")); !os.IsNotExist(err) {
		t.Fatalf("a refused term left a package behind: %v", err)
	}
}

func TestATermNamingAMissingStoryFailsToLoad(t *testing.T) {
	root := newSaga(t)
	if _, err := AddStory(root, "test", storyInput("checkout", "r1", "proposed", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := AddTerm(root, "test", testtakerInput()); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "___features", "core.feature", "___requirements", "stories", "checkout.story")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, "test"); err == nil || !strings.Contains(err.Error(), "___overview/terms/testtaker.term/revisions/r1.json") {
		t.Fatalf("a dangling story link must name the revision that holds it: %v", err)
	}
}
