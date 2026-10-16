package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
)

func runEntity(t *testing.T, root, kind string, args ...string) (contentEditOutput, error) {
	t.Helper()
	var out bytes.Buffer
	args = append(args, "--json", root)
	var err error
	switch kind {
	case "feature":
		err = Feature(context.Background(), args, &out)
	case "feature-theme":
		err = FeatureTheme(context.Background(), args, &out)
	default:
		err = Classification(context.Background(), kind, args, &out)
	}
	var result contentEditOutput
	if decodeErr := json.Unmarshal(out.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode %v: %v\n%s", err, decodeErr, &out)
	}
	return result, err
}

func entityFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.Contains(entry.Name(), "lock") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = string(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func addEntityFeature(t *testing.T, root, id string) {
	t.Helper()
	if err := Feature(context.Background(), []string{"add", "--id", id, "--title", "Original", "--description", "Preserve this", "--request-id", "request-" + id, root}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestFeatureRenamePreservesIdentityHistoryAndDryRun(t *testing.T) {
	root := newAuthoredSaga(t)
	addEntityFeature(t, root, "rename-me")
	addStory(t, root, "rename-me", "kept")
	before := entityFiles(t, root)
	target := applayout.FeatureURN("atomic", "rename-me")
	dry, err := runEntity(t, root, "feature", "rename", "--feature", target, "--title", "Renamed", "--dry-run")
	if err != nil || !dry.OK || !dry.DryRun || dry.Resource != target || !reflect.DeepEqual(dry.Changed, []string{"title"}) {
		t.Fatalf("dry: %+v %v", dry, err)
	}
	if !reflect.DeepEqual(before, entityFiles(t, root)) {
		t.Fatal("dry-run wrote files")
	}
	result, err := runEntity(t, root, "feature", "rename", "--feature", target, "--title", "Renamed")
	if err != nil || result.Replayed {
		t.Fatalf("rename: %+v %v", result, err)
	}
	rel := filepath.FromSlash(applayout.FeatureRel("rename-me") + "/feature.json")
	after := entityFiles(t, root)
	var oldRecord, newRecord applayout.FeatureManifest
	if err := json.Unmarshal([]byte(before[rel]), &oldRecord); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(after[rel]), &newRecord); err != nil {
		t.Fatal(err)
	}
	oldRecord.Title = "Renamed"
	if !reflect.DeepEqual(oldRecord, newRecord) {
		t.Fatalf("identity/metadata changed: %+v", newRecord)
	}
	delete(before, rel)
	delete(after, rel)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rename changed story, plan, or unrelated files")
	}
	replay, err := runEntity(t, root, "feature", "rename", "--feature", "rename-me", "--title", "Renamed")
	if err != nil || !replay.Replayed || len(replay.Paths) != 0 {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	assertValid(t, root)
}

func TestFeatureRemoveIsEmptyOnlyAndAccountsForMetadata(t *testing.T) {
	root := newAuthoredSaga(t)
	addEntityFeature(t, root, "empty")
	before := entityFiles(t, root)
	dry, err := runEntity(t, root, "feature", "remove", "--feature", "empty", "--dry-run")
	if err != nil || !dry.DryRun || len(dry.Removed) != 1 || len(dry.Paths) != 3 {
		t.Fatalf("dry removal: %+v %v", dry, err)
	}
	if !reflect.DeepEqual(before, entityFiles(t, root)) {
		t.Fatal("dry removal wrote files")
	}
	result, err := runEntity(t, root, "feature", "remove", "--feature", "empty")
	if err != nil || !reflect.DeepEqual(result.Removed, dry.Removed) || !reflect.DeepEqual(result.Paths, dry.Paths) {
		t.Fatalf("remove: %+v %v", result, err)
	}
	if _, err := os.Stat(applayout.FeatureDir(root, "empty")); !os.IsNotExist(err) {
		t.Fatalf("feature remains: %v", err)
	}
	addEntityFeature(t, root, "occupied")
	addStory(t, root, "occupied", "kept")
	before = entityFiles(t, root)
	for _, dry := range []bool{true, false} {
		args := []string{"remove", "--feature", "occupied"}
		if dry {
			args = append(args, "--dry-run")
		}
		result, err := runEntity(t, root, "feature", args...)
		if err == nil || result.OK || !strings.Contains(result.Error.Message, "authored content or history") {
			t.Fatalf("accepted content removal: %+v %v", result, err)
		}
	}
	if !reflect.DeepEqual(before, entityFiles(t, root)) {
		t.Fatal("refusal changed files")
	}
	assertValid(t, root)
}

func TestFeatureRemoveRefusesThemeAndHistoricalDependencies(t *testing.T) {
	root := newAuthoredSaga(t)
	addEntityFeature(t, root, "held")
	if err := FeatureTheme(context.Background(), []string{"add", "--id", "group", "--title", "Group", "--text", "Rationale", "--feature", "held", root}, io.Discard); err != nil {
		t.Fatal(err)
	}
	before := entityFiles(t, root)
	for _, dry := range []bool{true, false} {
		args := []string{"remove", "--feature", "held"}
		if dry {
			args = append(args, "--dry-run")
		}
		result, err := runEntity(t, root, "feature", args...)
		if err == nil || !strings.Contains(result.Error.Message, "group.theme/theme.json") {
			t.Fatalf("dependency accepted: %+v %v", result, err)
		}
	}
	if !reflect.DeepEqual(before, entityFiles(t, root)) {
		t.Fatal("refusal changed files")
	}
	// An immutable flag revision remains a dependency after its current revision changes.
	if err := FeatureTheme(context.Background(), []string{"set-features", "--theme", "group", root}, io.Discard); err != nil {
		t.Fatal(err)
	}
	target := applayout.FeatureURN("atomic", "held")
	if err := FeatureFlag(context.Background(), []string{"add", "--id", "gate", "--revision", "r1", "--event", "off", "--description", "Gates held", "--target", target, root}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := FeatureFlag(context.Background(), []string{"revise", "--flag", "urn:change-saga:atomic:flag:gate", "--revision", "r2", "--parent", "urn:change-saga:atomic:flag:gate:revision:r1", "--description", "Now gates core", "--target", applayout.FeatureURN("atomic", testFeature), root}, io.Discard); err != nil {
		t.Fatal(err)
	}
	result, err := runEntity(t, root, "feature", "remove", "--feature", "held", "--dry-run")
	if err == nil || !strings.Contains(result.Error.Message, "revisions/r1.json") {
		t.Fatalf("historical dependency accepted: %+v %v", result, err)
	}
}

func TestFeatureThemeRenameRemoveAndDeckProtection(t *testing.T) {
	root := newAuthoredSaga(t)
	if err := FeatureTheme(context.Background(), []string{"add", "--id", "group", "--title", "Group", "--text", "Authored rationale", "--feature", testFeature, root}, io.Discard); err != nil {
		t.Fatal(err)
	}
	before, err := applayout.FeatureThemes(root, "atomic")
	if err != nil {
		t.Fatal(err)
	}
	files := entityFiles(t, root)
	result, err := runEntity(t, root, "feature-theme", "rename", "--theme", "group", "--title", "New group", "--dry-run")
	if err != nil || !result.DryRun || !reflect.DeepEqual(files, entityFiles(t, root)) {
		t.Fatalf("dry rename: %+v %v", result, err)
	}
	if _, err := runEntity(t, root, "feature-theme", "rename", "--theme", applayout.FeatureThemeURN("atomic", "group"), "--title", "New group"); err != nil {
		t.Fatal(err)
	}
	after, err := applayout.FeatureThemes(root, "atomic")
	if err != nil {
		t.Fatal(err)
	}
	before[0].Title = "New group"
	if !reflect.DeepEqual(before, after) {
		t.Fatal("theme rename lost metadata")
	}
	if err := AddDeck(context.Background(), []string{"--feature-theme", "group", "--id", "rationale", "--objective", "Explain", root, "rationale"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	result, err = runEntity(t, root, "feature-theme", "remove", "--theme", "group")
	if err == nil || !strings.Contains(result.Error.Message, "authored content or history") {
		t.Fatalf("deleted deck: %+v %v", result, err)
	}
	if err := RemoveDeck(context.Background(), []string{"--deck", "urn:change-saga:atomic:deck:rationale", root}, io.Discard); err != nil {
		t.Fatal(err)
	}
	result, err = runEntity(t, root, "feature-theme", "remove", "--theme", "group")
	if err != nil || len(result.Removed) != 1 {
		t.Fatalf("remove empty theme: %+v %v", result, err)
	}
	doc, validation, err := saga.Load(root)
	if err != nil || !validation.Valid || doc.FindFeature(testFeature) == nil {
		t.Fatalf("theme removal affected feature: %v %v", err, validation)
	}
}

func TestClassificationRemoveRequiresExplicitUnassignment(t *testing.T) {
	for _, kind := range []string{"label", "milestone"} {
		t.Run(kind, func(t *testing.T) {
			root := newAuthoredSaga(t)
			if err := Classification(context.Background(), kind, []string{"add", "--id", "shared", "--title", "Shared", root}, io.Discard); err != nil {
				t.Fatal(err)
			}
			if err := Feature(context.Background(), []string{"set-plan", "--feature", testFeature, "--" + kind, "shared", root}, io.Discard); err != nil {
				t.Fatal(err)
			}
			before := entityFiles(t, root)
			for _, dry := range []bool{true, false} {
				args := []string{"remove", "--" + kind, "shared"}
				if dry {
					args = append(args, "--dry-run")
				}
				result, err := runEntity(t, root, kind, args...)
				if err == nil || !strings.Contains(result.Error.Message, "plan.json") {
					t.Fatalf("dependency accepted: %+v %v", result, err)
				}
			}
			if !reflect.DeepEqual(before, entityFiles(t, root)) {
				t.Fatal("refused removal changed files")
			}
			args := []string{"set-plan", "--feature", testFeature}
			if kind == "label" {
				args = append(args, "--clear-labels")
			} else {
				args = append(args, "--milestone", "")
			}
			if err := Feature(context.Background(), append(args, root), io.Discard); err != nil {
				t.Fatal(err)
			}
			before = entityFiles(t, root)
			dry, err := runEntity(t, root, kind, "remove", "--"+kind, "shared", "--dry-run")
			if err != nil || !dry.DryRun || !reflect.DeepEqual(before, entityFiles(t, root)) {
				t.Fatalf("dry removal: %+v %v", dry, err)
			}
			result, err := runEntity(t, root, kind, "remove", "--"+kind, applayout.ClassificationURN("atomic", kind, "shared"))
			if err != nil || !reflect.DeepEqual(result.Removed, dry.Removed) {
				t.Fatalf("remove: %+v %v", result, err)
			}
			assertValid(t, root)
		})
	}
}

func TestEntityEditsRejectCrossSagaAndUnknownFields(t *testing.T) {
	root := newAuthoredSaga(t)
	addEntityFeature(t, root, "safe")
	before := entityFiles(t, root)
	for _, selector := range []string{"../safe", "urn:change-saga:other:feature:safe", "missing"} {
		result, err := runEntity(t, root, "feature", "rename", "--feature", selector, "--title", "Changed")
		if err == nil || result.OK {
			t.Fatalf("bad selector accepted: %+v %v", result, err)
		}
	}
	if !reflect.DeepEqual(before, entityFiles(t, root)) {
		t.Fatal("invalid selector changed files")
	}
	path := filepath.Join(applayout.FeatureDir(root, "safe"), "feature.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("{\n"), []byte("{\n  \"future_metadata\": true,\n"), 1)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := runEntity(t, root, "feature", "rename", "--feature", "safe", "--title", "Changed"); err == nil {
		t.Fatal("unknown fields silently discarded")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("invalid metadata changed")
	}
}

func TestEntityReferenceScannerIncludesEscapedURNsAndRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	target := "urn:change-saga:test:feature:x"
	writeFile(t, filepath.Join(root, "history.json"), `{"reference":"urn\u003achange-saga:test:feature:x"}`)
	refs, err := entityReferences(root, filepath.Join(root, "removed"), target)
	if err != nil || !reflect.DeepEqual(refs, []string{"history.json"}) {
		t.Fatalf("escaped reference missed: %v %v", refs, err)
	}
	writeFile(t, filepath.Join(root, "history.json"), `{"reference":"urn:change-saga:test:feature:xyz"}`)
	refs, err = entityReferences(root, filepath.Join(root, "removed"), target)
	if err != nil || len(refs) != 0 {
		t.Fatalf("prefix overmatch: %v %v", refs, err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := entityReferences(root, filepath.Join(root, "removed"), target); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestEntityEditsRejectForeignOwnersAndSymlinkedCatalogs(t *testing.T) {
	for _, kind := range []string{"feature", "feature-theme", "label", "milestone"} {
		t.Run(kind, func(t *testing.T) {
			root := newAuthoredSaga(t)
			selector := kind
			catalog := applayout.FeaturesDir
			switch kind {
			case "feature":
				addEntityFeature(t, root, "safe")
			case "feature-theme":
				selector, catalog = "theme", applayout.FeatureThemesDir
				if err := FeatureTheme(context.Background(), []string{"add", "--id", "safe", "--title", "Safe", "--text", "Rationale", root}, io.Discard); err != nil {
					t.Fatal(err)
				}
			default:
				catalog = applayout.ClassificationDir(kind)
				if err := Classification(context.Background(), kind, []string{"add", "--id", "safe", "--title", "Safe", root}, io.Discard); err != nil {
					t.Fatal(err)
				}
			}
			before := entityFiles(t, root)
			result, err := runEntity(t, root, kind, "remove", "--"+selector, "urn:change-saga:foreign:"+kind+":safe")
			if err == nil || result.OK || !reflect.DeepEqual(before, entityFiles(t, root)) {
				t.Fatalf("foreign removal accepted: %+v %v", result, err)
			}
			outside := filepath.Join(t.TempDir(), "catalog")
			if err := os.Rename(filepath.Join(root, catalog), outside); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, catalog)); err != nil {
				t.Fatal(err)
			}
			before = entityFiles(t, outside)
			result, err = runEntity(t, root, kind, "remove", "--"+selector, "safe")
			if err == nil || result.OK || !reflect.DeepEqual(before, entityFiles(t, outside)) {
				t.Fatalf("symlink removal accepted: %+v %v", result, err)
			}
		})
	}
}
