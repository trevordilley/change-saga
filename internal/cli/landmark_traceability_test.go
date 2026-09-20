package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/areas"
)

// twoSidesSaga is the dogfooding case that found this: a design fragment whose
// design addresses a story's criterion, with its concrete implementation
// claims attached to heading landmarks rather than owned at fragment scope,
// which is what validate asks an author to do. It returns the Saga, the
// checkout, and the two landmark URNs that own the code.
func twoSidesSaga(t *testing.T) (root, repo string, landmarks []string) {
	t.Helper()
	root, repo = coveredSaga(t)
	run := func(name string, command func(context.Context, []string, io.Writer) error, args ...string) {
		t.Helper()
		var output bytes.Buffer
		if err := command(context.Background(), args, &output); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, output.String())
		}
	}
	run("story add", Story, "add", "--feature", testFeature, "--id", "two-sides", "--revision", "r1", "--event", "proposed",
		"--title", "The reviewer app has two sides", "--statement", "As a reviewer I read documentation and reviews apart so that neither hides the other",
		"--persona", personaURNFor("batch"), "--criterion", "reviews-on-one-side=Reviews appear only on the review side.", root)
	run("design add-chapter", Design, "add-chapter", "--feature", testFeature, "--id", "two-sides-design", "--title", "Two sides", root, "two-sides-design")
	run("design add-fragment", Design, "add-fragment", "--feature", testFeature, "--section", "two-sides-design.chapter",
		"--id", "two-sides-overview", "--name", "documentation-and-review", "--title", "Documentation and Review", root)

	content := strings.Join([]string{
		"# Documentation and Review {#documentation-and-review}",
		"",
		"## Documentation is what the app is {#documentation-side}",
		"",
		"The sidebar keeps reviews off the documentation side.",
		"",
		"## Reviews live on their own side {#review-side}",
		"",
		"The header decides which side a page belongs to.",
		"",
	}, "\n")
	var output bytes.Buffer
	if err := setFragmentContentScoped(context.Background(), []string{"--target", "two-sides-overview", "--source", "-", root}, &output, strings.NewReader(content), designAuthoring); err != nil {
		t.Fatalf("set fragment content: %v\n%s", err, output.String())
	}

	commit := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	fragmentRel := testFeatureRel + "/___design/two-sides-design.chapter/documentation-and-review.fragment"
	for _, landmark := range []struct {
		id, label, lines string
	}{
		{"documentation-side", "Documentation is what the app is", "L3-L4"},
		{"review-side", "Reviews live on their own side", "L5"},
	} {
		run("add-landmark", AddLandmark, "--target", fragmentRel, "--heading-id", landmark.id, "--label", landmark.label, root)
		var covered bytes.Buffer
		if err := Cover(context.Background(), []string{"--against", "main", "--repo", repo,
			"--target", fragmentRel + "/___landmarks/" + landmark.id + ".landmark",
			"--ref", commit + ":internal/service/handler.go#" + landmark.lines, "--note", "The code this heading explains.", root}, &covered); err != nil {
			t.Fatalf("cover %s: %v\n%s", landmark.id, err, covered.String())
		}
		landmarks = append(landmarks, "urn:change-saga:batch:fragment:two-sides-overview:landmark:"+landmark.id)
	}

	// The fragment's design addresses the criterion; only the fragment does.
	run("relation add", Relation, "add", "--feature", testFeature, "--id", "two-sides-addresses", "--type", "addresses",
		"--from", "urn:change-saga:batch:fragment:two-sides-overview",
		"--to", "urn:change-saga:batch:story:two-sides:criterion:reviews-on-one-side",
		"--rationale", "The fragment explains how reviews stay on the review side.", root)
	assertValid(t, root)

	// The fragment's own relation must be current, or this fixture would prove
	// nothing about what a landmark inherits.
	var currency bytes.Buffer
	if err := Relation(context.Background(), []string{"status", root}, &currency); err != nil {
		t.Fatalf("relation status: %v\n%s", err, currency.String())
	}
	if !strings.Contains(currency.String(), "urn:change-saga:batch:relation:two-sides-addresses current") {
		t.Fatalf("the fragment's relation is not current:\n%s", currency.String())
	}
	return root, repo, landmarks
}

func statusCoverage(t *testing.T, root string, args ...string) areas.Report {
	t.Helper()
	var output bytes.Buffer
	_ = Status(context.Background(), append(append([]string{"--json"}, args...), root), &output)
	var document statusDocument
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("status --json: %v\n%s", err, output.String())
	}
	return document.Coverage
}

// Moving a fragment's code references onto heading landmarks is what validate
// asks for, and it must not drop that code out of the story chain. A landmark
// is a heading inside the fragment, so the fragment's design explains it.
func TestLandmarkCodeReachesTheStoryItsFragmentAddresses(t *testing.T) {
	root, repo, landmarks := twoSidesSaga(t)
	stories := statusCoverage(t, root, "--repo", repo).Areas.Stories
	reached := map[string][]string{}
	for _, entry := range stories.CoveredEntries {
		reached[entry.Resource] = entry.Via
	}
	for _, landmark := range landmarks {
		via := reached[landmark]
		if len(via) != 1 || via[0] != "urn:change-saga:batch:story:two-sides" {
			t.Fatalf("%s reaches %v, want the story its fragment addresses; stories = %d/%d %+v",
				landmark, via, stories.Covered, stories.Total, stories.UncoveredEntries)
		}
	}
	if personas := statusCoverage(t, root, "--repo", repo).Areas.Personas; personas.Covered != len(landmarks) {
		t.Fatalf("personas = %d/%d, want every landmark to reach the persona its story serves: %+v", personas.Covered, personas.Total, personas)
	}
}
