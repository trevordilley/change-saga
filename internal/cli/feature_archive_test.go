package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

// Archiving hides a feature that holds stories without removing anything, a
// repeat is a replayed no-op, and restore returns it to the status it had.
func TestFeatureArchiveKeepsRecordsAndRestoresStatus(t *testing.T) {
	t.Parallel()
	root := newAuthoredSaga(t)
	ctx := context.Background()
	if err := Feature(ctx, []string{"add", "--id", "later", "--title", "Later", "--status", "planned", "--priority", "3", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Story(ctx, []string{"add", root, "--feature", "later", "--id", "later-story", "--revision", "r1", "--event", "proposed", "--title", "Use it later", "--statement", "I can use it later"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Feature(ctx, []string{"remove", "--feature", "later", root}, &bytes.Buffer{}); err == nil {
		t.Fatal("a feature with a story must still refuse removal")
	}
	run := func(args ...string) contentEditOutput {
		t.Helper()
		var out bytes.Buffer
		if err := Feature(ctx, append(args, "--json", root), &out); err != nil {
			t.Fatalf("%v: %v %s", args, err, &out)
		}
		var result contentEditOutput
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if dry := run("archive", "--feature", "later", "--dry-run"); !dry.DryRun || dry.Replayed {
		t.Fatalf("dry run = %+v", dry)
	}
	if first := run("archive", "--feature", "later", "--reason", "Changing with the Swarm work"); first.Replayed || len(first.Changed) != 1 {
		t.Fatalf("archive = %+v", first)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %+v", err, validation.Issues)
	}
	plan := document.FindFeature("later").Plan
	if !plan.Archived || plan.ArchivedReason != "Changing with the Swarm work" || plan.Status != "planned" || plan.Priority != 3 {
		t.Fatalf("archived plan = %+v", plan)
	}
	if again := run("archive", "--feature", "later"); !again.Replayed {
		t.Fatalf("a repeated archive must replay: %+v", again)
	}
	if restored := run("restore", "--feature", "later"); restored.Replayed {
		t.Fatalf("restore = %+v", restored)
	}
	document, _, _ = saga.Load(root)
	plan = document.FindFeature("later").Plan
	if plan.Archived || plan.ArchivedReason != "" || plan.Status != "planned" || plan.Priority != 3 {
		t.Fatalf("restored plan = %+v", plan)
	}
	if again := run("restore", "--feature", "later"); !again.Replayed {
		t.Fatalf("a repeated restore must replay: %+v", again)
	}
	if document.FindFeature("later") == nil || len(document.FindFeature("later").Path) == 0 {
		t.Fatal("the feature must survive archive and restore")
	}
	assertValid(t, root)
}
