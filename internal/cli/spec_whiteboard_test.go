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
			Purpose, Root, Storage, Schema, Focus string
			SourcePins                            string `json:"source_pins"`
			Kinds, Unsupported                    []string
			RevisionVersion                       int             `json:"revision_version"`
			DiagramSource                         json.RawMessage `json:"diagram_source"`
			History, Commands, Compatibility      map[string]string
		} `json:"whiteboards"`
	}
	if err := json.Unmarshal(output.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	board := spec.Whiteboards
	if len(board.Kinds) != 1 || board.Kinds[0] != "erd" || board.Root != requirements.WhiteboardDir || board.Storage != requirements.WhiteboardDir+"/erd/revisions/<id>.json" || board.Schema != requirements.WhiteboardSchemaURL || board.RevisionVersion != 1 {
		t.Fatalf("whiteboard storage or supported kinds drifted: %+v", board)
	}
	var grammar bytes.Buffer
	if err := json.Compact(&grammar, board.DiagramSource); err != nil {
		t.Fatal(err)
	}
	shared, err := json.Marshal(diagram.Contract())
	if err != nil || !bytes.Equal(grammar.Bytes(), shared) {
		t.Fatalf("whiteboards must expose the existing shared diagram grammar: %v", err)
	}
	if len(board.Commands) != 4 || board.Commands["publish"] != commandUsage["diagram publish"] {
		t.Fatalf("whiteboard commands drifted: %#v", board.Commands)
	}
	for _, op := range []string{"publish", "get", "describe", "check"} {
		if !strings.Contains(board.Commands[op], "diagram "+op+" --whiteboard erd") || !strings.Contains(board.Commands[op], "--revision ID") {
			t.Errorf("missing implemented command or revision selector: %s", board.Commands[op])
		}
	}
	for _, op := range []string{"get", "describe", "check"} {
		if !strings.Contains(board.Commands[op], "--focus PATH") {
			t.Errorf("missing read focus: %s", board.Commands[op])
		}
	}
	if !strings.Contains(board.Commands["get"], "--id PATH") || strings.Contains(board.Commands["publish"], "--focus") {
		t.Fatal("focus must remain a read selector, including the existing get alias")
	}
	for key, want := range map[string]string{"revisions": "immutable complete", "parent": "observed unique current", "reads": "never a fallback"} {
		if !strings.Contains(board.History[key], want) {
			t.Errorf("history %s lost %q: %s", key, want, board.History[key])
		}
	}
	if !strings.Contains(board.Purpose, "technical-design") || !strings.Contains(board.Compatibility["absent"], "remain valid") || !strings.Contains(board.Compatibility["older_readers"], "reject") || strings.Join(board.Unsupported, ";") != "infrastructure or system whiteboards;whiteboard edit transactions" {
		t.Fatalf("scope/compatibility limits missing: %+v", board)
	}
	if !strings.Contains(board.SourcePins, "exact target and revision URNs") || !strings.Contains(board.SourcePins, "never follow newer") {
		t.Fatal("source pins must stay exact")
	}
	output.Reset()
	if err := Spec(nil, &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Technical design can use an ERD whiteboard", "Only kind erd is supported", "___whiteboards/erd/revisions/<id>.json", "diagram publish --whiteboard erd", "diagram get|describe|check --whiteboard erd", "--focus PATH", "share the implementation deck's diagram source grammar", "current board revision ID as --parent", "Older readers reject an adopted ___whiteboards", "absence remains valid"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("text spec omitted %q", want)
		}
	}
}
