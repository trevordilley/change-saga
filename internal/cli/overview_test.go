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

func TestDataModelOverviewAuthoringAndGitHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := newAuthoredSaga(t)
	git(t, filepath.Dir(root), "init", "-b", "main")
	git(t, root, "config", "user.name", "Test")
	git(t, root, "config", "user.email", "test@example.test")
	var output bytes.Buffer
	for i, body := range []string{"Local SQLite and cloud Postgres.", "Local SQLite synchronizes to cloud Postgres."} {
		output.Reset()
		if err := overviewCommand(ctx, []string{"set-data-model", "--source", "-", "--json", root}, &output, strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
		var result fragmentContentOutput
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Target != saga.FragmentTarget("atomic", "atomic-data-model") || result.MediaType != "text/markdown" {
			t.Fatalf("result: %+v", result)
		}
		document, validation, err := saga.Load(root)
		if err != nil || !validation.Valid {
			t.Fatalf("load: %v %+v", err, validation)
		}
		fragment := document.OverviewPart(applayout.OverviewDataModel)
		if fragment == nil {
			t.Fatal("missing overview")
		}
		data, err := os.ReadFile(filepath.Join(fragment.Directory, fragment.Entrypoint))
		if err != nil || string(data) != body+"\n" {
			t.Fatalf("readback: %q %v", data, err)
		}
		// Updating prose preserves the stable package identity.
		if i == 0 {
			git(t, root, "add", ".")
			git(t, root, "commit", "-m", "data model overview")
		}
	}
	old := git(t, root, "show", "HEAD:atomic.saga/___overview/data-model.fragment/content.md")
	if !strings.Contains(old, "Local SQLite and cloud Postgres.") {
		t.Fatalf("historical overview: %s", old)
	}
	output.Reset()
	if err := Query(ctx, []string{"history", "--saga", root, "--node", saga.FragmentTarget("atomic", "atomic-data-model")}, &output); err != nil {
		t.Fatalf("history: %v %s", err, &output)
	}
	if !strings.Contains(output.String(), "data model overview") {
		t.Fatalf("history did not discover overview commit: %s", &output)
	}
}

func TestDataModelOverviewRequiresMarkdownAndSafePackage(t *testing.T) {
	t.Parallel()
	root := newAuthoredSaga(t)
	dir := filepath.Join(root, applayout.OverviewDir, applayout.OverviewDataModel)
	writeFile(t, filepath.Join(dir, "fragment.json"), `{"version":2,"id":"model","media_type":"text/html","entrypoint":"content.html"}`)
	writeFile(t, filepath.Join(dir, "content.html"), "<p>Model</p>")
	_, validation, err := saga.Load(root)
	if err != nil || validation.Valid {
		t.Fatalf("accepted HTML overview: %v %+v", err, validation)
	}
	if err := Overview(context.Background(), []string{"set-data-model", "--text", "replacement", root}, &bytes.Buffer{}); err == nil {
		t.Fatal("overwrote non-Markdown package")
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	if err := os.Symlink(external, dir); err != nil {
		t.Fatal(err)
	}
	if err := Overview(context.Background(), []string{"set-data-model", "--text", "replacement", root}, &bytes.Buffer{}); err == nil {
		t.Fatal("followed overview package symlink")
	}
	entries, err := os.ReadDir(external)
	if err != nil || len(entries) != 0 {
		t.Fatal("wrote outside the Saga")
	}
}
