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

func TestRoadmapReusesFeatureIdentityAndContent(t *testing.T) {
	root, repo := coveredSaga(t)
	ctx := context.Background()
	run := func(args ...string) {
		t.Helper()
		if err := Feature(ctx, append(args, root), &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := Classification(ctx, "milestone", []string{"add", "--id", "v2", "--title", "v2.0", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	run("add", "--id", "future", "--title", "Future", "--status", "planned", "--milestone", "v2", "--priority", "1", "--request-id", "future")
	run("add", "--id", "future", "--title", "Future", "--status", "planned", "--milestone", "v2", "--priority", "1", "--request-id", "future")
	run("set-overview", "--feature", "future", "--text", "An ordinary feature Essay.", "--abstract", "The planned outcome.")
	identity := filepath.Join(root, applayout.FeatureRel("future"), "feature.json")
	before, err := os.ReadFile(identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := Story(ctx, []string{"add", root, "--feature", "future", "--id", "future-story", "--revision", "r1", "--event", "proposed", "--title", "Use future capability", "--statement", "I can use the planned capability"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := FeatureTheme(ctx, []string{"add", root, "--id", "growth", "--title", "Growth", "--text", "Expand the product.", "--feature", "future"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Query(ctx, []string{"roadmap", "--saga", root}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"future"`, `"priority":1`, `"milestone":"urn:change-saga:batch:milestone:v2"`, ":feature-theme:growth"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s: %s", want, &out)
		}
	}
	run("set-plan", "--feature", "future", "--priority", "2")
	doc, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %+v", err, validation)
	}
	plan := doc.FindFeature("future").Plan
	if plan.Status != "planned" || plan.Priority != 2 || plan.Milestone != applayout.ClassificationURN(doc.Manifest.ID, "milestone", "v2") {
		t.Fatalf("patch lost metadata: %+v", plan)
	}
	out.Reset()
	if err := Query(ctx, []string{"context", "--saga", root, "--repo", repo, "--feature", "future"}, &out); err != nil || !strings.Contains(out.String(), `"status":"planned"`) {
		t.Fatalf("context: %v %s", err, &out)
	}
	git(t, filepath.Dir(root), "init", "-b", "main")
	git(t, root, "config", "user.name", "Test")
	git(t, root, "config", "user.email", "test@example.test")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "Plan future capability")
	run("set-plan", "--feature", "future", "--status", "built")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "Deliver future capability")
	out.Reset()
	if err := Query(ctx, []string{"history", "--saga", root, "--node", doc.FindFeature("future").Target}, &out); err != nil || !strings.Contains(out.String(), "Deliver future capability") {
		t.Fatalf("feature history omitted planning edit: %v %s", err, &out)
	}
	after, _ := os.ReadFile(identity)
	if !bytes.Equal(before, after) {
		t.Fatal("planning changed immutable identity")
	}
	doc, validation, err = saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %+v", err, validation)
	}
	if f := doc.FindFeature("future"); f.Plan.Planned() || f.Plan.Milestone != applayout.ClassificationURN(doc.Manifest.ID, "milestone", "v2") || f.Overview() == nil {
		t.Fatalf("promotion lost feature: %+v", f)
	}
	out.Reset()
	if err := Query(ctx, []string{"roadmap", "--saga", root}, &out); err != nil || !strings.Contains(out.String(), `"features":[]`) {
		t.Fatalf("built feature remained on Roadmap: %v %s", err, &out)
	}
	run("set-plan", "--feature", "future", "--status", "planned", "--milestone", "", "--priority", "0")
	doc, _, _ = saga.Load(root)
	if p := doc.FindFeature("future").Plan; p.Priority != 0 || p.Milestone != "" {
		t.Fatalf("clear: %+v", p)
	}
	for _, args := range [][]string{{"--status", "unknown"}, {"--priority", "-1"}, {"--milestone", "first\nsecond"}} {
		if err := Feature(ctx, append([]string{"set-plan", root, "--feature", "future"}, args...), &out); err == nil {
			t.Fatalf("invalid plan accepted: %v", args)
		}
	}
}

func TestRoadmapCursorTracksPlanningAndPriority(t *testing.T) {
	root := newAuthoredSaga(t)
	ctx := context.Background()
	for _, id := range []string{"later", "first", "unranked"} {
		args := []string{"add", root, "--id", id, "--title", id, "--status", "planned"}
		if id != "unranked" {
			rank := "2"
			if id == "first" {
				rank = "1"
			}
			args = append(args, "--priority", rank)
		}
		if err := Feature(ctx, args, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	query := func(cursor string) (string, error) {
		var out bytes.Buffer
		args := []string{"roadmap", "--saga", root, "--limit", "1"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		err := Query(ctx, args, &out)
		return out.String(), err
	}
	first, err := query("")
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Page struct {
			NextCursor string `json:"next_cursor"`
		}
		Data struct{ Features []struct{ ID string } }
	}
	if err := json.Unmarshal([]byte(first), &page); err != nil {
		t.Fatal(err)
	}
	if page.Data.Features[0].ID != "first" {
		t.Fatalf("priority order: %s", first)
	}
	second, err := query(page.Page.NextCursor)
	if err != nil || !strings.Contains(second, `"id":"later"`) {
		t.Fatalf("page two: %v %s", err, second)
	}
	if err := Classification(ctx, "milestone", []string{"add", "--id", "launch", "--title", "Launch", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Feature(ctx, []string{"set-plan", root, "--feature", "later", "--milestone", "launch"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if result, err := query(page.Page.NextCursor); err == nil || !strings.Contains(result, "stale_snapshot") {
		t.Fatalf("stale cursor accepted: %v %s", err, result)
	}
}
