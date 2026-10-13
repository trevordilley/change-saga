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

func TestFeatureThemeAuthoringAndNormalDecks(t *testing.T) {
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	ctx := context.Background()
	var out bytes.Buffer
	run := func(args ...string) {
		t.Helper()
		out.Reset()
		if err := FeatureTheme(ctx, append(args, root), &out); err != nil {
			t.Fatalf("theme %v: %v\n%s", args, err, &out)
		}
	}
	before, err := os.ReadFile(filepath.Join(testFeatureDir(root), "feature.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"reliable", "trusted"} {
		run("add", "--id", id, "--title", id, "--text", "# Why\n\n**Authored** rationale.", "--feature", testFeature)
	}
	run("add", "--id", "reliable", "--title", "reliable", "--text", "# Why\n\n**Authored** rationale.", "--feature", testFeature, "--json")
	if !strings.Contains(out.String(), `"replayed": true`) {
		t.Fatalf("retry: %s", &out)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid || len(document.FeatureThemes) != 2 {
		t.Fatalf("load: %v %+v", err, validation)
	}
	for _, args := range [][]string{{"set-features", "--theme", "reliable", "--feature", "missing"}, {"set-features", "--theme", "reliable", "--feature", "urn:change-saga:other:feature:" + testFeature}, {"add", "--id", "../escape", "--title", "bad", "--text", "bad"}} {
		if err := FeatureTheme(ctx, append(args, root), &out); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	run("set-overview", "--theme", "reliable", "--text", "# Revised rationale")
	if err := AddDeck(ctx, []string{"--feature-theme", "reliable", "--id", "why-reliable", "--objective", "Why these features belong together", root, "rationale"}, &out); err != nil {
		t.Fatal(err)
	}
	// The same transactional renderer accepts a rationale slide and validates
	// supplied code. Code is optional and criterion implementation is not implied.
	request := slideTransactionRequest(t, repo, base, commit, sagaID, "theme-create", "create", "absent", "worker")
	request.Deck = "why-reliable"
	request.Items[0].CriterionLinks = nil
	request.Items[0].Record = applayout.FeatureURN(sagaID, testFeature)
	result, err := ApplySlideTransaction(ctx, root, base, repo, request, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySlideTransaction(ctx, root, base, repo, request, false); err != nil {
		t.Fatalf("transaction retry: %v", err)
	}
	document, validation, err = saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("deck load: %v %+v", err, validation)
	}
	theme := document.FindFeatureTheme("reliable")
	if len(theme.Decks) != 1 || len(theme.Decks[0].Slides) != 1 || len(document.Decks) != 1 {
		t.Fatalf("theme deck improperly classified: %+v", theme)
	}
	if theme.Decks[0].Slides[0].AuthoringSnapshot != result.Snapshot {
		t.Fatal("theme slide not transactional")
	}
	out.Reset()
	if err := Query(ctx, []string{"feature-themes", "--saga", root, "--theme", theme.Target}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Revised rationale", "why-reliable", applayout.FeatureURN(sagaID, testFeature)} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("query misses %q: %s", want, &out)
		}
	}
	out.Reset()
	if err := Query(ctx, []string{"schema", "feature-themes"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "data.feature_themes") {
		t.Fatalf("discovery: %s", &out)
	}
	// Empty code is legitimate rationale; malformed supplied code still fails.
	request.RequestID = "theme-no-code"
	request.Slide.ID = "why-no-code"
	request.Items[0].Evidence = nil
	if _, err := ApplySlideTransaction(ctx, root, base, repo, request, false); err != nil {
		t.Fatalf("rationale without code: %v", err)
	}
	request.RequestID = "theme-bad-code"
	request.Slide.ID = "why-bad-code"
	bad := slideTransactionRequest(t, repo, base, commit, sagaID, "bad-ref", "create", "absent", "worker")
	request.Items[0].Evidence = bad.Items[0].Evidence
	request.Items[0].Evidence[0].References[0].Digest = "sha256:" + strings.Repeat("0", 64)
	if _, err := ApplySlideTransaction(ctx, root, base, repo, request, false); err == nil {
		t.Fatal("accepted invalid supplied code")
	}
	out.Reset()
	if err := Query(ctx, []string{"feature-themes", "--saga", root, "--limit", "1"}, &out); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Snapshot string            `json:"snapshot"`
		Page     queryPageEnvelope `json:"page"`
	}
	if err := json.Unmarshal(out.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Snapshot == "" || !page.Page.HasMore || page.Page.NextCursor == nil || strings.Contains(out.String(), "Revised rationale") {
		t.Fatalf("unbounded discovery %s", &out)
	}
	cursor := *page.Page.NextCursor
	out.Reset()
	if err := Query(ctx, []string{"feature-themes", "--saga", root, "--limit", "1", "--cursor", cursor}, &out); err != nil {
		t.Fatal(err)
	}
	run("set-features", "--theme", "trusted")
	out.Reset()
	if err := Query(ctx, []string{"feature-themes", "--saga", root, "--limit", "1", "--cursor", cursor}, &out); err == nil || !strings.Contains(out.String(), "stale_snapshot") {
		t.Fatalf("stale cursor accepted: %s", &out)
	}
	out.Reset()
	if err := Query(ctx, []string{"history", "--saga", root, "--node", theme.Target}, &out); err != nil {
		t.Fatalf("history: %v %s", err, &out)
	}

	after, _ := os.ReadFile(filepath.Join(testFeatureDir(root), "feature.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("theme membership rewrote feature identity")
	}
	// Theme-only mutations do not change story paths or definitions.
	if _, err := os.Stat(filepath.Join(testFeatureDir(root), "___requirements", "stories", "run.story")); err != nil {
		t.Fatal(err)
	}
}

func TestFeatureThemeLegacyAndSafePaths(t *testing.T) {
	root := newAuthoredSaga(t)
	var out bytes.Buffer
	if err := Query(context.Background(), []string{"feature-themes", "--saga", root}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"feature_themes":[]`) {
		t.Fatal(out.String())
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, applayout.FeatureThemesDir)); err != nil {
		t.Fatal(err)
	}
	if err := FeatureTheme(context.Background(), []string{"add", "--id", "x", "--title", "X", "--text", "why", root}, &out); err == nil {
		t.Fatal("wrote through theme root symlink")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("mutated outside root")
	}
}

func TestFeatureThemeOnlyNormalAuthoringAndMutationIndex(t *testing.T) {
	root := newAuthoredSaga(t)
	ctx := context.Background()
	var out bytes.Buffer
	if err := FeatureTheme(ctx, []string{"add", "--id", "t", "--title", "Theme", "--text", "Why this matters", root}, &out); err != nil {
		t.Fatal(err)
	}
	if err := AddDeck(ctx, []string{"--feature-theme", "t", "--id", "why", "--objective", "Explain the theme", root, "why"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := AddSlide(ctx, []string{"--deck", "why", "--intent", "explain", "--layout", "hero", root, "reason"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(ctx, []string{"--slide", "reason", "--id", "point", "--kind", "node", "--label", "The reason", "--description", "Why the theme matters", "--element-id", "slide-title", root}, &out); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(ctx, []string{"--slide", "reason", "--id", "ghost", "--kind", "node", "--description", "Invalid member", "--element-id", "slide-title", "--record", "urn:change-saga:atomic:feature:ghost", root}, &out); err == nil {
		t.Fatal("accepted missing Item record")
	}
	doc, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %+v", err, validation)
	}
	if len(doc.Decks) != 0 || len(doc.Onboarding) != 0 || len(doc.ThemeDecks()) != 1 {
		t.Fatal("theme became an implementation deck")
	}
	for _, issue := range validation.Issues {
		if strings.Contains(issue.Message, "linked code") {
			t.Fatalf("rationale required code: %+v", issue)
		}
	}
	index, v, err := saga.LoadMutationIndex(root)
	if err != nil || !v.Valid {
		t.Fatalf("index: %v %+v", err, v)
	}
	target := saga.ItemTarget(doc.Manifest.ID, "reason", "point")
	if index.Targets[target] == "" {
		t.Fatalf("theme-only index lost %s", target)
	}
	if err := ReviseDeck(ctx, []string{"--deck", "why", "--title", "Authored rationale", root}, &out); err != nil {
		t.Fatal(err)
	}
	if err := ReviseItem(ctx, []string{"--item", target, "--label", "Revised reason", root}, &out); err != nil {
		t.Fatal(err)
	}
	if err := RemoveItem(ctx, []string{"--item", target, root}, &out); err != nil {
		t.Fatal(err)
	}
	if err := RemoveDeck(ctx, []string{"--deck", "why", root}, &out); err != nil {
		t.Fatal(err)
	}
}
