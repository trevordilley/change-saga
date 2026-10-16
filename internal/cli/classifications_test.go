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

func TestSharedClassificationsPreserveReferencesAndRejectDuplicates(t *testing.T) {
	root := newAuthoredSaga(t)
	ctx := context.Background()
	classify := func(kind string, args ...string) {
		t.Helper()
		if err := Classification(ctx, kind, append(args, root), &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	feature := func(args ...string) {
		t.Helper()
		if err := Feature(ctx, append(args, root), &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	classify("label", "add", "--id", "devops", "--title", "DevOps")
	classify("label", "add", "--id", "devops", "--title", "DevOps")
	classify("milestone", "add", "--id", "launch", "--title", "Launch")
	for _, id := range []string{"first", "second"} {
		feature("add", "--id", id, "--title", id, "--status", "planned", "--label", "devops", "--milestone", "launch", "--request-id", id)
	}
	feature("add", "--id", "first", "--title", "first", "--status", "planned", "--label", "urn:change-saga:atomic:label:devops", "--milestone", "launch", "--request-id", "first")
	path := filepath.Join(root, applayout.FeatureRel("first"), applayout.FeaturePlanName)
	before, _ := os.ReadFile(path)
	if bytes.Contains(before, []byte("DevOps")) || bytes.Contains(before, []byte("Launch")) {
		t.Fatalf("feature copied display names: %s", before)
	}
	classify("label", "rename", "--label", "devops", "--title", "Platform operations")
	classify("milestone", "rename", "--milestone", "launch", "--title", "Summer launch")
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("renaming a shared entity rewrote feature membership")
	}
	doc, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %v", err, validation.Issues)
	}
	for _, id := range []string{"first", "second"} {
		plan := doc.FindFeature(id).Plan
		if doc.Classifications.Title("atomic", "label", plan.Labels[0]) != "Platform operations" || doc.Classifications.Title("atomic", "milestone", plan.Milestone) != "Summer launch" {
			t.Fatal("shared titles did not resolve")
		}
	}
	for _, kind := range []string{"label", "milestone"} {
		id := "devops"
		if kind == "milestone" {
			id = "launch"
		}
		if err := Query(ctx, []string{"history", "--saga", root, "--node", applayout.ClassificationURN("atomic", kind, id)}, &bytes.Buffer{}); err != nil {
			t.Fatalf("shared record history: %v", err)
		}
	}
	for _, kind := range []string{"label", "milestone"} {
		title := "platform operations"
		if kind == "milestone" {
			title = "SUMMER LAUNCH"
		}
		if err := Classification(ctx, kind, []string{"add", "--id", "duplicate", "--title", title, root}, &bytes.Buffer{}); err == nil {
			t.Fatal("duplicate title accepted")
		}
	}
	feature("set-plan", "--feature", "first", "--status", "built", "--priority", "1")
	doc, _, _ = saga.Load(root)
	if len(doc.FindFeature("first").Plan.Labels) != 1 || doc.FindFeature("first").Plan.Milestone == "" {
		t.Fatal("promotion lost references")
	}
	before, _ = os.ReadFile(path)
	for _, args := range [][]string{{"--label", "missing"}, {"--label", "urn:change-saga:other:label:devops"}, {"--label", "devops", "--label", "urn:change-saga:atomic:label:devops"}, {"--milestone", "missing"}, {"--label", "devops", "--clear-labels"}} {
		if err := Feature(ctx, append(append([]string{"set-plan", "--feature", "first"}, args...), root), &bytes.Buffer{}); err == nil {
			t.Fatalf("invalid assignment accepted: %v", args)
		}
		after, _ = os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal("invalid assignment changed plan")
		}
	}
	feature("set-plan", "--feature", "first", "--clear-labels", "--milestone", "")
	doc, _, _ = saga.Load(root)
	if p := doc.FindFeature("first").Plan; len(p.Labels) != 0 || p.Milestone != "" {
		t.Fatal("clear failed")
	}
	// A removed referenced entity is a validation error, not a silently invented label.
	if err := os.Remove(filepath.Join(root, applayout.LabelsDir, "devops.json")); err != nil {
		t.Fatal(err)
	}
	_, validation, err = saga.Load(root)
	if err != nil || validation.Valid {
		t.Fatalf("dangling reference accepted: %v %v", err, validation)
	}
}

func TestClassificationQueryCursorTracksRename(t *testing.T) {
	root := newAuthoredSaga(t)
	ctx := context.Background()
	for _, id := range []string{"a", "b"} {
		if err := Classification(ctx, "label", []string{"add", "--id", id, "--title", id, root}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := Query(ctx, []string{"labels", "--saga", root, "--limit", "1"}, &out); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Page struct {
			NextCursor string `json:"next_cursor"`
		}
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Page.NextCursor == "" {
		t.Fatal("missing cursor")
	}
	if err := Classification(ctx, "label", []string{"rename", "--label", "b", "--title", "Renamed", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Query(ctx, []string{"labels", "--saga", root, "--cursor", result.Page.NextCursor}, &out); err == nil || !strings.Contains(out.String(), "stale_snapshot") {
		t.Fatalf("stale cursor accepted: %v %s", err, out.String())
	}
}
