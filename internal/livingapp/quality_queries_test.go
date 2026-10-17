package livingapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
)

func TestQualityQueriesSeparateDefinitionsFromImmutableHistory(t *testing.T) {
	inputs := qualityFixture(t)
	s := &session{snapshot: "quality-fixture", quality: inputs.Quality, requirements: requirements.Document{SagaID: fixtureSaga, Stories: inputs.Stories}}
	for _, test := range inputs.Quality.TestCases {
		if test.Identity.ID == "happy" {
			s.requirements.Relations = append(s.requirements.Relations, verifies("happy-positive", "happy", "r1", "positive-path", storyR2))
		}
	}
	result, err := s.Query(context.Background(), Query{Operation: "quality", Filters: Filters{TestCase: testURN("happy")}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(result.Data)
	if strings.Contains(string(data), "run") || !strings.Contains(string(data), `"definition":"implementation_linked"`) || !strings.Contains(string(data), `"current":true`) {
		t.Fatalf("definition query: %s", data)
	}
	history, err := s.Query(context.Background(), Query{Operation: "quality-history", Filters: Filters{TestCase: "happy"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(history.Data)
	if !strings.Contains(string(data), `"legacy_runs"`) || !strings.Contains(string(data), `"result":"passed"`) {
		t.Fatalf("history lost: %s", data)
	}
	page, err := s.Query(context.Background(), Query{Operation: "quality", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !page.Page.HasMore || page.Page.NextCursor == nil {
		t.Fatalf("pagination: %+v", page.Page)
	}
	next, err := s.Query(context.Background(), Query{Operation: "quality", Limit: 1, Cursor: *page.Page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(page.Data)
	nextJSON, _ := json.Marshal(next.Data)
	if string(firstJSON) == string(nextJSON) {
		t.Fatal("cursor repeated case")
	}
	if _, err := s.Query(context.Background(), Query{Operation: "quality-history", Limit: 1, Cursor: *page.Page.NextCursor}); err == nil {
		t.Fatal("history accepted definition cursor")
	}
}

func TestQualityCodeStalenessDoesNotNeedARun(t *testing.T) {
	inputs := qualityFixture(t)
	for i := range inputs.Quality.TestCases {
		test := &inputs.Quality.TestCases[i]
		test.Runs = nil
		test.HeadRun = nil
		test.CurrentRun = nil
		test.RunHeads = nil
		if test.Identity.ID == "happy" {
			test.CurrentRevision.Steps[0].Criteria = []string{criterionURN("positive-path")}
			test.Evidence[0].Steps = []string{"act"}
			delete(inputs.QualityCode, test.Evidence[0].Code[0].Key())
		}
	}
	status := Assemble(inputs)
	if kind(t, status, "positive-path", string(quality.CoveragePositive)).State != kindStale {
		t.Fatal("unresolvable code must remain visibly stale without run records")
	}
	for _, test := range status.Quality.TestCases {
		if test.TestCase == testURN("happy") && test.Definition != "stale_evidence" {
			t.Fatalf("stage: %+v", test)
		}
	}
}
