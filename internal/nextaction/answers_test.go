package nextaction

import (
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/livingapp"
)

// staleKinds is every record kind staleRecords answers, including the default.
// Each is derived here because a shape is only checked against the grammar
// when it is built: "quality evidence add --diff" panicked in status for
// months because no fixture ever held a stale quality-evidence record.
var staleKinds = []livingapp.StaleRecord{
	{Record: "urn:change-saga:checkout:relation:old", Kind: "relation", History: "review", Reasons: []string{"to criterion statement changed"},
		Type: "verifies", From: "urn:change-saga:checkout:test-case:happy", To: criterion("done"),
		Pins:    []livingapp.Pin{{Field: "to_revision", Pinned: "urn:change-saga:checkout:story:refund:revision:r1", Current: "urn:change-saga:checkout:story:refund:revision:r2"}},
		Affects: []string{criterion("done")}},
	{Record: "urn:change-saga:checkout:prototype:flow:annotation:a1", Kind: "prototype_annotation", History: "review", Reasons: []string{"story revision changed"},
		Pins:    []livingapp.Pin{{Field: "story_revision", Pinned: "urn:change-saga:checkout:story:refund:revision:r1", Current: "urn:change-saga:checkout:story:refund:revision:r2"}},
		Affects: []string{criterion("done")}},
	{Record: "urn:change-saga:checkout:coverage-exception:no-ui", Kind: "coverage_exception", History: "review", Axis: "ui", Reasons: []string{"story revision changed"},
		Pins:    []livingapp.Pin{{Field: "story_revision", Pinned: "urn:change-saga:checkout:story:refund:revision:r1", Current: "urn:change-saga:checkout:story:refund:revision:r2"}},
		Affects: []string{criterion("done")}},
	{Record: "urn:change-saga:checkout:quality-policy:kinds", Kind: "quality_policy", History: "review", Reasons: []string{"story revision changed"},
		Pins:    []livingapp.Pin{{Field: "story_revision", Pinned: "urn:change-saga:checkout:story:refund:revision:r1", Current: "urn:change-saga:checkout:story:refund:revision:r2"}},
		Affects: []string{criterion("done")}},
	{Record: "urn:change-saga:checkout:test-case:revised:run:ci-1", Kind: "test_run", History: "review", Reasons: []string{"test revision changed"},
		Pins: []livingapp.Pin{{Field: "test_revision", Pinned: "urn:change-saga:checkout:test-case:revised:revision:r1", Current: "urn:change-saga:checkout:test-case:revised:revision:r2"}}},
	{Record: "urn:change-saga:checkout:test-case:revised:evidence:e1", Kind: "quality_evidence", History: "review", Reasons: []string{"test revision changed"},
		Pins: []livingapp.Pin{{Field: "test_revision", Pinned: "urn:change-saga:checkout:test-case:revised:revision:r1", Current: "urn:change-saga:checkout:test-case:revised:revision:r2"}}},
	{Record: "urn:change-saga:checkout:something:else", Kind: "unknown_kind", History: "review", Reasons: []string{"pins moved"}},
}

// Every next action must print the command that answers it. An action that
// asks a question and offers no way to record the answer leaves the reader to
// work the grammar out from prose, which is the whole reason the grammar is
// published.
func TestEveryStaleRecordKindOffersACommandForItsAnswer(t *testing.T) {
	status := statusFixture()
	status.Stale = staleKinds
	for _, action := range Derive(status, saga) {
		if action.Question == nil {
			if action.Command == nil {
				t.Fatalf("%s is neither a command nor a question", action.ID)
			}
			continue
		}
		if len(action.Question.Options) == 0 {
			t.Fatalf("%s asks a question with no answers", action.ID)
		}
		actionable := 0
		for _, option := range action.Question.Options {
			if len(option.Commands) > 0 {
				actionable++
			} else if strings.TrimSpace(option.Effect) == "" {
				t.Fatalf("%s answer %q records nothing and says nothing", action.ID, option.Answer)
			}
			for _, command := range option.Commands {
				if _, ok := grammar.Lookup(command.Command); !ok {
					t.Fatalf("%s answer %q names %q, which the grammar does not publish", action.ID, option.Answer, command.Command)
				}
				if len(command.Argv) == 0 || command.Argv[0] != "change-saga" {
					t.Fatalf("%s answer %q has no runnable argv: %#v", action.ID, option.Answer, command.Argv)
				}
			}
		}
		if actionable == 0 {
			t.Fatalf("%s asks a question no answer can act on: %#v", action.ID, action.Question)
		}
	}
}
