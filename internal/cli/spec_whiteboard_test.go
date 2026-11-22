package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
)

func TestSpecExposesImplementedWhiteboardContract(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := Spec([]string{"--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Whiteboards struct {
			Purpose, Root, Storage, Schema, Focus, Identity string
			SourcePins                                      string `json:"source_pins"`
			Kinds, Unsupported                              []string
			RecordVersion                                   int             `json:"record_version"`
			DiagramSource                                   json.RawMessage `json:"diagram_source"`
			History, Commands, Compatibility                map[string]string
		} `json:"whiteboards"`
		Deck struct {
			DiagramSource json.RawMessage `json:"diagram_source"`
		} `json:"implementation_deck"`
	}
	if err := json.Unmarshal(output.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	board := spec.Whiteboards
	if len(board.Kinds) != 0 || !strings.Contains(board.Identity, "--id selects a named document") || board.Root != requirements.WhiteboardDir || board.Storage != requirements.WhiteboardDir+"/<board-id>/current.json" || board.Schema != "https://changesaga.dev/schema/v5/current-record.schema.json" || board.RecordVersion != 1 {
		t.Fatalf("whiteboard storage or supported kinds drifted: %+v", board)
	}
	var boardGrammar map[string]json.RawMessage
	if err := json.Unmarshal(board.DiagramSource, &boardGrammar); err != nil {
		t.Fatal(err)
	}
	var boardInput, boardStorage string
	if err := json.Unmarshal(boardGrammar["request_field"], &boardInput); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(boardGrammar["storage"], &boardStorage); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(boardInput, "complete diagram.Document") || !strings.Contains(boardInput, "whiteboard publish --id ID --from") || !strings.Contains(boardStorage, "editable document inside ___whiteboards/<board-id>/current.json") {
		t.Fatal("whiteboard grammar must describe board input/storage instead of slide assets")
	}
	shared := diagram.Contract()
	slideGrammar, err := json.Marshal(shared)
	if err != nil {
		t.Fatal(err)
	}
	var deckGrammar bytes.Buffer
	if err := json.Compact(&deckGrammar, spec.Deck.DiagramSource); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(deckGrammar.Bytes(), slideGrammar) {
		t.Fatal("whiteboard metadata overrides changed the slide contract")
	}
	for _, key := range []string{"request_field", "storage"} {
		delete(boardGrammar, key)
		delete(shared, key)
	}
	actual, err := json.Marshal(boardGrammar)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := json.Marshal(shared)
	if err != nil || !bytes.Equal(actual, expected) {
		t.Fatalf("whiteboards must retain the shared grammar apart from input/storage: %v", err)
	}
	if len(board.Commands) != 10 || board.Commands["publish"] != commandUsage["whiteboard publish"] {
		t.Fatalf("whiteboard commands drifted: %#v", board.Commands)
	}
	for _, op := range []string{"list", "rename", "assign-role", "unassign-role", "archive", "restore"} {
		if board.Commands[op] == "" || board.Commands[op] != commandUsage["whiteboard "+op] {
			t.Errorf("missing metadata operation %s", op)
		}
	}
	for _, op := range []string{"publish", "get", "describe", "check"} {
		if board.Commands[op] != commandUsage["whiteboard "+op] || !strings.Contains(board.Commands[op], "whiteboard "+op+" --id ID") || strings.Contains(board.Commands[op], "--revision") || strings.Contains(board.Commands[op], "--parent") {
			t.Errorf("missing current command or obsolete authoring flags: %s", board.Commands[op])
		}
	}
	for _, op := range []string{"get", "describe", "check"} {
		if !strings.Contains(board.Commands[op], "--focus PATH") {
			t.Errorf("missing read focus: %s", board.Commands[op])
		}
	}
	if !strings.Contains(board.Commands["get"], "--id ID") || strings.Contains(board.Commands["publish"], "--focus") {
		t.Fatal("document ID and focus must remain separate selectors")
	}
	for key, want := range map[string]string{"revisions": "one editable current", "parent": "Git owns branching", "reads": "never persist normalization"} {
		if !strings.Contains(board.History[key], want) {
			t.Errorf("history %s lost %q: %s", key, want, board.History[key])
		}
	}
	if !strings.Contains(board.Purpose, "technical-design") || !strings.Contains(board.Compatibility["absent"], "remain valid") || !strings.Contains(board.Compatibility["older_readers"], "reject") || strings.Join(board.Unsupported, ";") != "whiteboard edit transactions" {
		t.Fatalf("scope/compatibility limits missing: %+v", board)
	}
	if !strings.Contains(board.SourcePins, "stable targets and exact content digests") || !strings.Contains(board.SourcePins, "never follow current content") {
		t.Fatal("source pins must stay exact")
	}
	output.Reset()
	if err := Spec(nil, &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Git is the revision history", "Existing identity files keep stable URNs", "___whiteboards/<id>/current.json", "whiteboard publish --id ID", "[--focus PATH]", "Their diagram grammar and renderer are shared", "query history --node URN", "Normal reads never write normalization", "ambiguous legacy content is retained"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("text spec omitted %q", want)
		}
	}
}
