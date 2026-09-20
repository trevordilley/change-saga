package saga

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
)

const testCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// testReference is a well-formed reference; loading never resolves it, so
// its digest need not match any repository.
func testReference(path string, start, end int) coderef.Reference {
	return coderef.Reference{Commit: testCommit, Path: path, Start: start, End: end, Digest: coderef.DigestBytes([]byte(path))}
}

func referenceJSON(t *testing.T, references ...coderef.Reference) string {
	t.Helper()
	data, err := json.Marshal(references)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestClaimAndVerificationRecordsFailClosed(t *testing.T) {
	line := referenceJSON(t, testReference("app.go", 1, 1))
	noted := testReference("app.go", 1, 1)
	noted.Note = "claims explain themselves"
	withNote := referenceJSON(t, noted)
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{name: "claim filename mismatch", files: map[string]string{"___claims/wrong.json": fmt.Sprintf(`{"version":2,"id":"claim-1","target":"urn:change-saga:test:fragment:overview","kind":"behavior","statement":"Ready is true.","evidence":%s,"created_at":"2026-08-21T12:00:00Z"}`, line)}, want: "must match filename"},
		{name: "claim target missing", files: map[string]string{"___claims/claim-1.json": fmt.Sprintf(`{"version":2,"id":"claim-1","target":"urn:change-saga:test:fragment:missing","kind":"behavior","statement":"Ready is true.","evidence":%s,"created_at":"2026-08-21T12:00:00Z"}`, line)}, want: "claim target does not exist"},
		{name: "claim evidence carries a note", files: map[string]string{"___claims/claim-1.json": fmt.Sprintf(`{"version":2,"id":"claim-1","target":"urn:change-saga:test:fragment:overview","kind":"behavior","statement":"Ready is true.","evidence":%s,"created_at":"2026-08-21T12:00:00Z"}`, withNote)}, want: "cannot carry a note"},
		{name: "claim evidence is a diff URI", files: map[string]string{"___claims/claim-1.json": `{"version":2,"id":"claim-1","target":"urn:change-saga:test:fragment:overview","kind":"behavior","statement":"Ready is true.","evidence":["saga-diff://v1/line?path=app.go"],"created_at":"2026-08-21T12:00:00Z"}`}, want: "cannot unmarshal"},
		{name: "verification claim missing", files: map[string]string{"___verifications/check-1.json": `{"version":2,"id":"check-1","claim":"missing","status":"unverified","summary":"Not checked.","created_at":"2026-08-21T12:00:00Z"}`}, want: "unknown claim"},
		{name: "reserved record is directory", files: map[string]string{"___claims/not-json.txt/file": "hidden"}, want: "regular .json files"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validation, report := loadIssues(t, buildSaga(t, test.files))
			if validation.Valid || !strings.Contains(report, test.want) {
				t.Fatalf("issues:\n%s", report)
			}
		})
	}
}

const validSagaJSON = `{"version":5,"id":"test","title":"A saga","source":{"repository":"https://example.test/acme/app.git"}}`

// buildSaga writes a minimal valid saga and then applies the caller's overlay,
// so each case states only the thing under test.
func buildSaga(t *testing.T, files map[string]string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "test.saga")
	base := map[string]string{
		"saga.json":                       validSagaJSON,
		"overview.fragment/fragment.json": `{"version":2,"id":"overview","title":"Overview","media_type":"text/markdown","entrypoint":"content.md"}`,
		"overview.fragment/content.md":    "The whole story.\n",
	}
	for rel, body := range files {
		base[rel] = body
	}
	for rel, body := range base {
		if body == "" {
			continue
		}
		writeTestFile(t, filepath.Join(root, filepath.FromSlash(inFeature(rel))), body)
	}
	return root
}

func loadIssues(t *testing.T, root string) (Validation, string) {
	t.Helper()
	_, validation, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var messages []string
	for _, issue := range validation.Issues {
		messages = append(messages, issue.Severity+" "+issue.Path+": "+issue.Message)
	}
	return validation, strings.Join(messages, "\n")
}

// TestLoadRejectsMalformedMetadata is the adversarial table for records that a
// published JSON Schema rejects. Every case here used to load cleanly, which
// meant an engine validating against schema/v2 and this runtime disagreed about
// whether the same saga was well formed.
func TestLoadRejectsMalformedMetadata(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{{
		name:  "repository carries credentials",
		files: map[string]string{"saga.json": `{"version":5,"id":"test","title":"A saga","source":{"repository":"https://user:secret@example.test/a.git"}}`},
		want:  "userinfo",
	}, {
		name:  "repository keeps ssh userinfo",
		files: map[string]string{"saga.json": `{"version":5,"id":"test","title":"A saga","source":{"repository":"ssh://git@example.test/acme/app.git"}}`},
		want:  `use "ssh://example.test/acme/app.git"`,
	}, {
		name:  "media type is not a published type",
		files: map[string]string{"overview.fragment/fragment.json": `{"version":2,"id":"overview","title":"O","media_type":"image/","entrypoint":"content.md"}`},
		want:  "unsupported media_type",
	}, {
		name:  "media type smuggles a header break",
		files: map[string]string{"overview.fragment/fragment.json": "{\"version\":2,\"id\":\"overview\",\"title\":\"O\",\"media_type\":\"image/png\\r\\nX-Evil: 1\",\"entrypoint\":\"content.md\"}"},
		want:  "unsupported media_type",
	}, {
		name:  "entrypoint uses a backslash separator",
		files: map[string]string{"overview.fragment/fragment.json": `{"version":2,"id":"overview","title":"O","media_type":"text/markdown","entrypoint":"sub\\content.md"}`},
		want:  "backslash",
	}, {
		name:  "entrypoint is an absolute path",
		files: map[string]string{"overview.fragment/fragment.json": `{"version":2,"id":"overview","title":"O","media_type":"text/markdown","entrypoint":"/etc/passwd"}`},
		want:  "relative to its fragment package",
	}, {
		name:  "entrypoint is a Windows drive path",
		files: map[string]string{"overview.fragment/fragment.json": `{"version":2,"id":"overview","title":"O","media_type":"text/markdown","entrypoint":"C:/windows/win.ini"}`},
		want:  "relative to its fragment package",
	}, {
		name:  "entrypoint traverses out of the package",
		files: map[string]string{"overview.fragment/fragment.json": `{"version":2,"id":"overview","title":"O","media_type":"text/markdown","entrypoint":"../saga.json"}`},
		want:  "inside its fragment package",
	}, {
		name:  "entrypoint addresses reserved metadata",
		files: map[string]string{"overview.fragment/fragment.json": `{"version":2,"id":"overview","title":"O","media_type":"text/markdown","entrypoint":"___code/a.json"}`},
		want:  "reserved fragment path",
	}, {
		name:  "entrypoint names the fragment manifest",
		files: map[string]string{"overview.fragment/fragment.json": `{"version":2,"id":"overview","title":"O","media_type":"text/markdown","entrypoint":"fragment.json"}`},
		want:  "reserved fragment path",
	}, {
		name:  "evidence file selects nothing",
		files: map[string]string{"___code/empty.json": `{"version":2,"references":[]}`},
		want:  "at least one code reference",
	}}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			validation, report := loadIssues(t, buildSaga(t, testCase.files))
			if validation.Valid {
				t.Fatalf("saga should be invalid; issues:\n%s", report)
			}
			if !strings.Contains(report, testCase.want) {
				t.Fatalf("expected an issue containing %q; got:\n%s", testCase.want, report)
			}
		})
	}
}

// TestLoadAcceptsPortableStructure guards the other direction: nothing above
// may start rejecting a saga the schema still accepts.
func TestLoadAcceptsPortableStructure(t *testing.T) {
	root := buildSaga(t, map[string]string{
		"overview.fragment/fragment.json":             `{"version":2,"id":"overview","title":"Overview","media_type":"text/html","entrypoint":"assets/index.html"}`,
		"overview.fragment/content.md":                "",
		"overview.fragment/assets/index.html":         `<p id="intro">Hello</p>`,
		"backend.chapter/chapter.json":                `{"version":2,"id":"backend","title":"Backend","order":10}`,
		"backend.chapter/flow.fragment/fragment.json": `{"version":2,"id":"flow","title":"Flow","media_type":"image/svg+xml","entrypoint":"image.svg"}`,
		"backend.chapter/flow.fragment/image.svg":     `<svg xmlns="http://www.w3.org/2000/svg"><rect id="box"/></svg>`,
	})
	validation, report := loadIssues(t, root)
	if !validation.Valid {
		t.Fatalf("portable saga should be valid; issues:\n%s", report)
	}
	if !strings.Contains(report, "") && len(validation.Issues) != 0 {
		t.Fatalf("unexpected issues:\n%s", report)
	}
}

func TestLoadWarnsAboutUnportableNames(t *testing.T) {
	root := buildSaga(t, map[string]string{
		"overview.fragment/fragment.json": `{"version":2,"id":"overview","title":"Overview","media_type":"text/markdown","entrypoint":"aux.md"}`,
		"overview.fragment/content.md":    "",
		"overview.fragment/aux.md":        "Story.\n",
	})
	validation, report := loadIssues(t, root)
	if !validation.Valid {
		t.Fatalf("an unportable name is a warning, not an error; issues:\n%s", report)
	}
	if !strings.Contains(report, "reserved Windows device name") {
		t.Fatalf("expected a Windows portability warning; got:\n%s", report)
	}
}

// A symlink is reported as a non-directory by os.ReadDir, so an unguarded
// loader silently skips it and calls the saga valid while an entire chapter is
// invisible.
func TestLoadRejectsSymlinkedEntities(t *testing.T) {
	if _, err := os.Lstat("/"); err != nil {
		t.Skip("no filesystem")
	}
	cases := []struct {
		name string
		link string
	}{
		{name: "chapter", link: "elsewhere.chapter"},
		{name: "fragment", link: "elsewhere.fragment"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := buildSaga(t, nil)
			outside := filepath.Join(filepath.Dir(root), "outside")
			if err := os.MkdirAll(outside, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, testCase.link)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			validation, report := loadIssues(t, root)
			if validation.Valid {
				t.Fatalf("a symlinked %s must not be silently ignored; issues:\n%s", testCase.name, report)
			}
			if !strings.Contains(report, "symlink") {
				t.Fatalf("expected a symlink issue; got:\n%s", report)
			}
		})
	}
}

// A saga.json written by change-saga init must load without a single issue.
func TestExistingCanonicalManifestStaysClean(t *testing.T) {
	for _, repository := range []string{
		"https://github.com/acme/payments.git",
		"https://example.test/acme/app.git",
		"file:///srv/repos/app",
	} {
		root := buildSaga(t, map[string]string{
			"saga.json": fmt.Sprintf(`{"version":5,"id":"test","title":"A saga","source":{"repository":%q}}`, repository),
		})
		validation, report := loadIssues(t, root)
		if !validation.Valid || len(validation.Issues) != 0 {
			t.Fatalf("repository %q should load cleanly; issues:\n%s", repository, report)
		}
	}
}

func TestNoncanonicalRepositoryIsAnError(t *testing.T) {
	root := buildSaga(t, map[string]string{
		"saga.json": `{"version":5,"id":"test","title":"A saga","source":{"repository":"HTTPS://Example.TEST:443/acme/app.git/"}}`,
	})
	validation, report := loadIssues(t, root)
	if validation.Valid {
		t.Fatalf("a noncanonical repository identity must be rejected; issues:\n%s", report)
	}
	if !strings.Contains(report, "must be canonical") || !strings.Contains(report, "https://example.test/acme/app.git") {
		t.Fatalf("expected a canonicalization error naming the canonical form; got:\n%s", report)
	}
}
