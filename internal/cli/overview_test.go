package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestOverviewPartsAreWrittenOnceAndThenReplaced(t *testing.T) {
	t.Parallel()
	root := newAuthoredSaga(t)
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if document.OverviewPart(applayout.OverviewPitch) != nil {
		t.Fatal("the pitch is a gap until it is written")
	}
	var output bytes.Buffer
	if err := overviewCommand(context.Background(), []string{"set-pitch", "--text", "Assessments that grade themselves.", "--json", root}, &output, nil); err != nil {
		t.Fatal(err)
	}
	var result fragmentContentOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.Path != "___overview/pitch.fragment/content.md" || result.Target != saga.FragmentTarget("atomic", "atomic-pitch") {
		t.Fatalf("result = %#v, %v\n%s", result, err, output.String())
	}
	output.Reset()
	if err := overviewCommand(context.Background(), []string{"set-pitch", "--source", "-", root}, &output, strings.NewReader("A better pitch.")); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(filepath.Join(root, "___overview", "pitch.fragment", "content.md"))
	if err != nil || string(written) != "A better pitch.\n" {
		t.Fatalf("pitch = %q, %v", written, err)
	}
	assertValid(t, root)
	document, _, err = saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if pitch := document.OverviewPart(applayout.OverviewPitch); pitch == nil || pitch.Title != "Elevator pitch" {
		t.Fatalf("pitch = %#v", pitch)
	}
	for _, args := range [][]string{
		{"set-pitch", "--text", "  ", root},
		{"set-pitch", "--text", "a", "--source", "b", root},
		{"set-name", "--text", "a", root},
	} {
		if err := overviewCommand(context.Background(), args, &bytes.Buffer{}, nil); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
	if err := AddFragment(context.Background(), []string{"--app", "overview", "--name", "notes", "--title", "Notes", root}, &bytes.Buffer{}); err == nil {
		t.Fatal("the overview is formal; free report content is refused")
	}
}

func TestAnythingElseInTheOverviewIsInvalid(t *testing.T) {
	t.Parallel()
	root := newAuthoredSaga(t)
	writeFile(t, filepath.Join(root, "___overview", "notes.fragment", "fragment.json"), `{"version":2,"id":"notes","title":"Notes","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeFile(t, filepath.Join(root, "___overview", "notes.fragment", "content.md"), "Notes\n")
	_, validation, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid || len(validation.Issues) == 0 || !strings.Contains(validation.Issues[0].Message, "the overview holds only") {
		t.Fatalf("validation = %#v", validation)
	}
}
