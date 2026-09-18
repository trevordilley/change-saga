package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestUpgradeToV3PreservesV2ContentWithoutAdoptingOptionalCapabilities(t *testing.T) {
	root := newAuthoredSaga(t)
	before, err := os.ReadFile(filepath.Join(root, "overview.fragment", "fragment.json"))
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := Upgrade(context.Background(), []string{"--to", "3", root}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "format v2 to v3") {
		t.Fatalf("unexpected output: %q", output.String())
	}
	for _, name := range []string{"___requirements", "___design", "___workplan"} {
		if _, err := os.Lstat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("upgrade unexpectedly adopted %s: %v", name, err)
		}
	}
	after, err := os.ReadFile(filepath.Join(root, "overview.fragment", "fragment.json"))
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("v2 component changed: equal=%v err=%v", bytes.Equal(after, before), err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("upgraded Saga = valid %v, err %v, issues %#v", validation.Valid, err, validation.Issues)
	}
	if document.Manifest.Version != saga.CurrentSagaVersion || document.Manifest.Schema != saga.V3SchemaURL {
		t.Fatalf("manifest was not upgraded: %#v", document.Manifest)
	}
	if document.Section.Fragments[0].ID == "" {
		t.Fatal("v2 narrative did not remain readable")
	}
}

func TestUpgradeFailureRestoresExactV2ManifestAndNoLivingRoots(t *testing.T) {
	steps := []string{"after-stage", "before-manifest", "after-manifest"}
	for _, step := range steps {
		t.Run(step, func(t *testing.T) {
			root := newAuthoredSaga(t)
			before, err := os.ReadFile(filepath.Join(root, "saga.json"))
			if err != nil {
				t.Fatal(err)
			}
			upgradeFaultHook = func(current string) error {
				if current == step {
					return errors.New("injected upgrade failure")
				}
				return nil
			}
			t.Cleanup(func() { upgradeFaultHook = nil })

			err = Upgrade(context.Background(), []string{"--to", "3", root}, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("Upgrade error = %v", err)
			}
			upgradeFaultHook = nil
			after, readErr := os.ReadFile(filepath.Join(root, "saga.json"))
			if readErr != nil || !bytes.Equal(after, before) {
				t.Fatalf("manifest changed after %s: equal=%v err=%v", step, bytes.Equal(after, before), readErr)
			}
			for _, name := range livingRootDirectories {
				if _, statErr := os.Lstat(filepath.Join(root, name)); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("failed upgrade left %s: %v", name, statErr)
				}
			}
			assertValid(t, root)
			matches, globErr := filepath.Glob(filepath.Join(filepath.Dir(root), ".change-saga-upgrade-*"))
			if globErr != nil || len(matches) != 0 {
				t.Fatalf("failed upgrade left staging state: %v, err=%v", matches, globErr)
			}
		})
	}
}

func TestUpgradeRejectsUnsupportedTargetsAndRepeatUpgrade(t *testing.T) {
	root := newAuthoredSaga(t)
	for _, args := range [][]string{{root}, {"--to", "2", root}, {"--to", "4", root}} {
		if err := Upgrade(context.Background(), args, &bytes.Buffer{}); err == nil {
			t.Fatalf("Upgrade(%v) succeeded", args)
		}
	}
	if err := Upgrade(context.Background(), []string{"--to", "3", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	before := rootEntryNames(t, root)
	if err := Upgrade(context.Background(), []string{"--to", "3", root}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("repeat Upgrade error = %v", err)
	}
	if after := rootEntryNames(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("repeat upgrade changed root entries: before=%v after=%v", before, after)
	}
}

func TestUpgradeJSONResultIsBounded(t *testing.T) {
	root := newAuthoredSaga(t)
	var output bytes.Buffer
	if err := Upgrade(context.Background(), []string{"--to", "3", "--json", root}, &output); err != nil {
		t.Fatal(err)
	}
	var result struct {
		OK      bool     `json:"ok"`
		From    int      `json:"from"`
		To      int      `json:"to"`
		Created []string `json:"created"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.From != 2 || result.To != 3 || !reflect.DeepEqual(result.Created, livingRootDirectories) {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestUpgradePreservesValidV2SymlinkedContent(t *testing.T) {
	root := newAuthoredSaga(t)
	link := filepath.Join(root, "overview.fragment", "alias.md")
	if err := os.Symlink("content.md", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := Upgrade(context.Background(), []string{"--to", "3", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("content symlink changed: info=%v err=%v", info, err)
	}
	assertValid(t, root)
}

func rootEntryNames(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	return names
}

// upgradeTreeBytes returns every regular file's bytes keyed by its slash path.
func upgradeTreeBytes(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		files[filepath.ToSlash(relative)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func addUpgradeStory(t *testing.T, root, id string) error {
	t.Helper()
	var output bytes.Buffer
	err := Story(context.Background(), []string{
		"add", root, "--id", id, "--revision", "r1", "--event", "proposed",
		"--title", "Checkout", "--statement", "As a buyer I can check out", "--priority", "must",
		"--criterion", "fast=Checkout finishes promptly", "--request-id", id + "-request", "--json",
	}, &output)
	if err != nil {
		return fmt.Errorf("%w: %s", err, output.String())
	}
	return nil
}

func TestUpgradeToV5ChangesOnlyTheManifestAndComposesEveryComponent(t *testing.T) {
	root := newLivingSaga(t)
	if err := addUpgradeStory(t, root, "checkout"); err != nil {
		t.Fatal(err)
	}
	before := upgradeTreeBytes(t, root)
	var beforeManifest map[string]any
	if err := json.Unmarshal([]byte(before["saga.json"]), &beforeManifest); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := Upgrade(context.Background(), []string{"--to", "5", root}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Upgraded "+root+" from Saga format v3 to v5") || !strings.Contains(output.String(), "quality: not_adopted") {
		t.Fatalf("unexpected output: %q", output.String())
	}
	after := upgradeTreeBytes(t, root)
	for path, data := range before {
		if path != "saga.json" && after[path] != data {
			t.Fatalf("upgrade changed component %s", path)
		}
	}
	if len(after) != len(before) {
		t.Fatalf("upgrade changed the file set: before %d after %d", len(before), len(after))
	}
	var afterManifest map[string]any
	if err := json.Unmarshal([]byte(after["saga.json"]), &afterManifest); err != nil {
		t.Fatal(err)
	}
	if afterManifest["version"] != float64(5) || afterManifest["$schema"] != saga.V5SchemaURL {
		t.Fatalf("manifest was not upgraded: %v", afterManifest)
	}
	delete(beforeManifest, "version")
	delete(beforeManifest, "$schema")
	delete(afterManifest, "version")
	delete(afterManifest, "$schema")
	if !reflect.DeepEqual(afterManifest, beforeManifest) {
		t.Fatalf("upgrade changed manifest fields other than version/$schema:\nbefore %v\nafter  %v", beforeManifest, afterManifest)
	}

	assertValid(t, root)
	document, err := requirements.Load(root, "")
	if err != nil || len(document.Stories) != 1 {
		t.Fatalf("requirements on v5 = %d stories, err %v", len(document.Stories), err)
	}
	qualityDocument, err := quality.Load(root)
	if err != nil || qualityDocument.Adoption != quality.NotAdopted {
		t.Fatalf("quality on upgraded v5 = %#v, err %v", qualityDocument.Adoption, err)
	}
	if err := os.MkdirAll(filepath.Join(root, quality.RootDir, "test-cases"), 0o755); err != nil {
		t.Fatal(err)
	}
	assertValid(t, root)
	if qualityDocument, err = quality.Load(root); err != nil || qualityDocument.Adoption != quality.AdoptedEmpty {
		t.Fatalf("adopted quality on upgraded v5 = %#v, err %v", qualityDocument.Adoption, err)
	}
}

func TestUpgradeToV5DryRunReportsCapabilitiesAndWritesNothing(t *testing.T) {
	root := newLivingSaga(t)
	before := upgradeTreeBytes(t, root)
	var output bytes.Buffer
	if err := Upgrade(context.Background(), []string{"--to", "5", "--dry-run", "--json", root}, &output); err != nil {
		t.Fatal(err)
	}
	var result struct {
		OK           bool                `json:"ok"`
		Operation    string              `json:"operation"`
		DryRun       bool                `json:"dry_run"`
		From         int                 `json:"from"`
		To           int                 `json:"to"`
		Capabilities []upgradeCapability `json:"capabilities"`
		Blockers     []string            `json:"blockers"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, capability := range result.Capabilities {
		states[capability.Name] = capability.State
	}
	if !result.OK || !result.DryRun || result.Operation != "upgrade" || result.From != 3 || result.To != 5 || len(result.Blockers) != 0 ||
		states["quality"] != "not_adopted" || states["requirements"] != "not_adopted" || states["coverage_exceptions"] != "not_adopted" {
		t.Fatalf("unexpected dry run result: %s", output.String())
	}
	if after := upgradeTreeBytes(t, root); !reflect.DeepEqual(after, before) {
		t.Fatal("dry run changed the Saga")
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(root), ".change-saga-upgrade-*")); len(matches) != 0 {
		t.Fatalf("dry run left staging state: %v", matches)
	}
}

func TestUpgradeToV5DryRunReportsBlockersWithoutWriting(t *testing.T) {
	root := newLivingSaga(t)
	manifestPath := filepath.Join(root, "saga.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	// A noncanonical repository spelling is only a v3 warning but a v5 error.
	writeFile(t, manifestPath, strings.Replace(string(data), "https://example.test/acme/app.git", "https://Example.test/acme/app.git", 1))
	assertValid(t, root)
	before := upgradeTreeBytes(t, root)
	var output bytes.Buffer
	err = Upgrade(context.Background(), []string{"--to", "5", "--dry-run", root}, &output)
	if err == nil || !strings.Contains(output.String(), "cannot upgrade") || !strings.Contains(output.String(), "must be canonical") {
		t.Fatalf("dry run error = %v, output %q", err, output.String())
	}
	if err := Upgrade(context.Background(), []string{"--to", "5", root}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "must be canonical") {
		t.Fatalf("upgrade with blocker error = %v", err)
	}
	if after := upgradeTreeBytes(t, root); !reflect.DeepEqual(after, before) {
		t.Fatal("blocked upgrade changed the Saga")
	}
}

func TestUpgradeToV5RefusesOtherSourceVersions(t *testing.T) {
	v2 := newAuthoredSaga(t)
	before := upgradeTreeBytes(t, v2)
	if err := Upgrade(context.Background(), []string{"--to", "5", v2}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "upgrade --to 3 first") {
		t.Fatalf("v2 -> v5 error = %v", err)
	}
	if after := upgradeTreeBytes(t, v2); !reflect.DeepEqual(after, before) {
		t.Fatal("refused v2 -> v5 changed the Saga")
	}

	v5 := newLivingSaga(t)
	if err := Upgrade(context.Background(), []string{"--to", "5", v5}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Upgrade(context.Background(), []string{"--to", "5", v5}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "already format v5") {
		t.Fatalf("repeat v5 error = %v", err)
	}

	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	writeFile(t, filepath.Join(repo, "README.md"), "base\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	v4 := filepath.Join(t.TempDir(), "slides.saga")
	if err := Init(context.Background(), []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", "--mode", "slides", v4}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Upgrade(context.Background(), []string{"--to", "5", v4}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "slide-native") {
		t.Fatalf("v4 -> v5 error = %v", err)
	}
	if err := Upgrade(context.Background(), []string{"--to", "6", v4}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "--to must be 3 or 5") {
		t.Fatalf("--to 6 error = %v", err)
	}
}

func TestUpgradeToV5FailureRestoresExactV3Manifest(t *testing.T) {
	for _, step := range []string{"after-stage", "before-manifest", "after-manifest"} {
		t.Run(step, func(t *testing.T) {
			root := newLivingSaga(t)
			before := upgradeTreeBytes(t, root)
			upgradeFaultHook = func(current string) error {
				if current == step {
					return errors.New("injected upgrade failure")
				}
				return nil
			}
			t.Cleanup(func() { upgradeFaultHook = nil })
			err := Upgrade(context.Background(), []string{"--to", "5", root}, &bytes.Buffer{})
			upgradeFaultHook = nil
			if err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("Upgrade error = %v", err)
			}
			if after := upgradeTreeBytes(t, root); !reflect.DeepEqual(after, before) {
				t.Fatalf("failed upgrade at %s changed the Saga", step)
			}
			if manifest, err := saga.ReadManifest(root); err != nil || manifest.Version != saga.CurrentSagaVersion {
				t.Fatalf("manifest after failure = %#v, %v", manifest, err)
			}
			if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(root), ".change-saga-upgrade-*")); len(matches) != 0 {
				t.Fatalf("failed upgrade left staging state: %v", matches)
			}
		})
	}
}

// A coverage exception is a v5 record stored beside the v3 requirement roots.
// The requirements loader must tolerate it without reading it, or every story,
// criterion, citation, and relation command breaks once one exception exists.
func TestStoryAuthoringSucceedsBesideV5CoverageExceptions(t *testing.T) {
	root := newLivingSaga(t)
	if err := addUpgradeStory(t, root, "checkout"); err != nil {
		t.Fatal(err)
	}
	exceptions := filepath.Join(root, "___requirements", "coverage-exceptions")
	if err := os.Mkdir(exceptions, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(exceptions, "docs-only.json"), `{"version":5,"id":"docs-only"}`)
	if err := addUpgradeStory(t, root, "refund"); err == nil || !strings.Contains(err.Error(), "requires a format v5 saga") {
		t.Fatalf("v3 story beside coverage exceptions error = %v", err)
	}
	if err := os.RemoveAll(exceptions); err != nil {
		t.Fatal(err)
	}

	if err := Upgrade(context.Background(), []string{"--to", "5", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(exceptions, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(exceptions, "docs-only.json"), `{"version":5,"id":"docs-only"}`)
	if err := addUpgradeStory(t, root, "refund"); err != nil {
		t.Fatalf("story add beside coverage exceptions: %v", err)
	}
	if err := Citation(context.Background(), []string{
		"add", root, "--id", "policy", "--kind", "url", "--title", "Policy", "--reference", "https://example.test/policy",
		"--request-id", "citation-request", "--json",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("citation add beside coverage exceptions: %v", err)
	}
	document, err := requirements.Load(root, "")
	if err != nil || len(document.Stories) != 2 || len(document.Citations) != 1 {
		t.Fatalf("requirements beside coverage exceptions = %d stories, %d citations, err %v", len(document.Stories), len(document.Citations), err)
	}
	if manifest, err := saga.ReadManifest(root); err != nil || manifest.Version != saga.ReportV5SagaVersion {
		t.Fatalf("authoring changed the v5 manifest: %#v, %v", manifest, err)
	}
	assertValid(t, root)
}

func TestDowngradeToV3RequiresNoV5OnlyRecords(t *testing.T) {
	root := newLivingSaga(t)
	v3 := upgradeTreeBytes(t, root)
	if err := Upgrade(context.Background(), []string{"--to", "5", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, blocker := range []string{quality.RootDir, filepath.Join("___requirements", "coverage-exceptions")} {
		t.Run(filepath.Base(blocker), func(t *testing.T) {
			if err := os.MkdirAll(filepath.Join(root, blocker), 0o755); err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(filepath.Join(root, blocker))
			before := upgradeTreeBytes(t, root)
			var output bytes.Buffer
			if err := Upgrade(context.Background(), []string{"--to", "3", "--dry-run", root}, &output); err == nil || !strings.Contains(output.String(), "v5-only record root") {
				t.Fatalf("dry-run downgrade error = %v, output %q", err, output.String())
			}
			if err := Upgrade(context.Background(), []string{"--to", "3", root}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "never discards records") {
				t.Fatalf("downgrade error = %v", err)
			}
			if after := upgradeTreeBytes(t, root); !reflect.DeepEqual(after, before) {
				t.Fatal("blocked downgrade changed the Saga")
			}
		})
	}
	if err := os.RemoveAll(filepath.Join(root, "___requirements")); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Upgrade(context.Background(), []string{"--to", "3", root}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Downgraded "+root+" from Saga format v5 to v3") {
		t.Fatalf("unexpected output: %q", output.String())
	}
	if after := upgradeTreeBytes(t, root); !reflect.DeepEqual(after, v3) {
		t.Fatal("v3 -> v5 -> v3 round trip is not byte-identical")
	}
}
