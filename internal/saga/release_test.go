package saga

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const releaseTestManifest = `{"version":5,"id":"test","title":"A saga","source":{"repository":"https://example.test/repo.git"}}`

func writeRelease(t *testing.T, root, id string, change func(*ReleaseManifest)) {
	t.Helper()
	manifest := ReleaseManifest{Schema: ReleaseSchemaURL, Version: ReleaseRecordVersion, ID: id, ReleaseVersion: "v" + id,
		CreatedAt: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)}
	if change != nil {
		change(&manifest)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(ReleaseDir(root, id), ReleaseManifestName), string(data))
}

func releaseSaga(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "change.saga")
	writeTestFile(t, filepath.Join(root, ManifestName), releaseTestManifest)
	return root
}

func issuesText(validation Validation) string {
	return fmt.Sprintf("%#v", validation.Issues)
}

func TestSagaWithoutReleasesIsUnaffected(t *testing.T) {
	document, validation, err := Load(releaseSaga(t))
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v, %s", err, issuesText(validation))
	}
	if document.Releases != nil {
		t.Fatalf("releases = %#v", document.Releases)
	}
}

func TestReleaseLoadsWithItsVersionAsGiven(t *testing.T) {
	root := releaseSaga(t)
	released := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	writeRelease(t, root, "next", func(m *ReleaseManifest) {
		m.ReleaseVersion = "1.0.0+build.5 (the kraken)"
		m.From, m.To = "v0.8.0", "v0.9.0"
		m.Milestone = "urn:change-saga:test:milestone:mvp"
		m.ReleasedAt, m.ReleasedCommit = &released, strings.Repeat("a", 40)
	})
	document, validation, err := Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v, %s", err, issuesText(validation))
	}
	release := document.FindRelease("next")
	if release == nil || release.ReleaseVersion != "1.0.0+build.5 (the kraken)" || !release.Released() {
		t.Fatalf("release = %#v", release)
	}
	if release.Target != "urn:change-saga:test:release:next" || release.Path != "___releases/next.release" {
		t.Fatalf("identity = %q, %q", release.Target, release.Path)
	}
}

func TestReleaseRefusals(t *testing.T) {
	released := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		change func(*ReleaseManifest)
		want   string
	}{
		"no version":        {func(m *ReleaseManifest) { m.ReleaseVersion = "" }, "release_version"},
		"padded version":    {func(m *ReleaseManifest) { m.ReleaseVersion = " v1 " }, "release_version"},
		"foreign milestone": {func(m *ReleaseManifest) { m.Milestone = "urn:change-saga:other:milestone:mvp" }, "milestone"},
		"time, no commit":   {func(m *ReleaseManifest) { m.ReleasedAt = &released }, "together"},
		"commit, no time":   {func(m *ReleaseManifest) { m.ReleasedCommit = strings.Repeat("a", 40) }, "together"},
		"short commit": {func(m *ReleaseManifest) {
			m.ReleasedAt, m.ReleasedCommit = &released, "abc123"
		}, "full commit"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			root := releaseSaga(t)
			writeRelease(t, root, "next", test.change)
			_, validation, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}
			if validation.Valid || !strings.Contains(issuesText(validation), test.want) {
				t.Fatalf("want an issue naming %q, got %s", test.want, issuesText(validation))
			}
		})
	}
}

func TestTwoReleasesOfOneVersionAreCaught(t *testing.T) {
	root := releaseSaga(t)
	same := func(m *ReleaseManifest) { m.ReleaseVersion = "v1.0.0" }
	writeRelease(t, root, "branch-a", same)
	writeRelease(t, root, "branch-b", same)
	_, validation, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid || !strings.Contains(issuesText(validation), `version \"v1.0.0\" is already release`) {
		t.Fatalf("issues = %s", issuesText(validation))
	}
}

func TestReleaseDirectoryNamesAreChecked(t *testing.T) {
	root := releaseSaga(t)
	writeTestFile(t, filepath.Join(root, ReleasesDir, "stray.json"), `{}`)
	writeRelease(t, root, "next", func(m *ReleaseManifest) { m.ID = "other" })
	writeTestFile(t, filepath.Join(ReleaseDir(root, "next"), "surprise.txt"), "hello")
	_, validation, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"real <id>.release directories", "must match its directory", "unknown entry in a release"} {
		if !strings.Contains(issuesText(validation), want) {
			t.Fatalf("missing %q in %s", want, issuesText(validation))
		}
	}
}
