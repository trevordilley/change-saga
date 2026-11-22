package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/currentrecord"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/store"
)

func TestStoryReviseCurrentContentDoesNotNormalizeUnrelatedLegacyConflict(t *testing.T) {
	root := newLivingSaga(t)
	story := sourcedStory(t, root)
	// This is a genuine legacy fork, not an invalid JSON stand-in. Whole-Saga
	// normalization must refuse to choose either independently authored value.
	board := filepath.Join(root, requirements.WhiteboardDir, "unrelated", "revisions")
	if err := os.MkdirAll(board, 0755); err != nil {
		t.Fatal(err)
	}
	doc := diagram.Document{Version: 1, Width: 400, Height: 300, Elements: []diagram.Element{{ID: "node", Kind: "node", Shape: "rect", Label: "Kept", X: 10, Y: 10, Width: 200, Height: 100, Style: "normal"}}}
	for _, id := range []string{"left", "right"} {
		value := map[string]any{"$schema": requirements.WhiteboardSchemaURL, "version": 1, "id": id, "parents": []string{}, "name": id, "document": doc, "created_at": time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}
		if err := store.WriteJSON(filepath.Join(board, id+".json"), value, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := currentrecord.Normalize(root); err == nil {
		t.Fatal("fixture must block global normalization")
	}
	before := entityFiles(t, root)
	old := *currentStoryRevisionForTest(t, root)
	var out bytes.Buffer
	if err := Story(context.Background(), []string{"revise", "--story", story, "--title", "Updated scope", "--statement", "As a buyer, I can use the revised access levels", "--json", root}, &out); err != nil {
		t.Fatalf("unrelated whiteboard blocked current story edit: %v\n%s", err, &out)
	}
	var result livingMutationOutput
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || !result.OK || result.Resource != story {
		t.Fatalf("edit result: %s (%v)", &out, err)
	}
	current := *currentStoryRevisionForTest(t, root)
	old.ID, old.Title, old.Statement = current.ID, "Updated scope", "As a buyer, I can use the revised access levels"
	if !reflect.DeepEqual(old, current) {
		t.Fatalf("edit lost omitted content: before %+v, after %+v", old, current)
	}
	path := filepath.FromSlash(requirements.StoryPackagePath(testFeature, "checkout") + "/current.json")
	after := entityFiles(t, root)
	delete(before, path)
	delete(after, path)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("story edit changed unrelated files, pins, or legacy values")
	}
	if err := currentrecord.Normalize(root); err == nil {
		t.Fatal("edit hid the unrelated legacy conflict")
	}
	// Strict target decoding is retained even though sibling normalization is skipped.
	target := filepath.Join(root, path)
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(raw), `"content": {`, `"content": {"unsupported": true,`, 1)
	if err := os.WriteFile(target, []byte(bad), 0644); err != nil {
		t.Fatal(err)
	}
	files := entityFiles(t, root)
	if err := Story(context.Background(), []string{"revise", "--story", story, "--title", "Invalid", root}, &out); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("strict decoding was lost: %v", err)
	}
	if !reflect.DeepEqual(files, entityFiles(t, root)) {
		t.Fatal("failed strict decoding wrote files")
	}
}
