package cli

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSetupCIReadOnlyAndArguments(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	var out bytes.Buffer
	if err := SetupCI([]string{"--repo", repo}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Repository: "+repo) || !strings.Contains(out.String(), "# Set up CI") {
		t.Fatalf("missing guide or repository: %s", out.String())
	}
	files, err := os.ReadDir(repo)
	if err != nil || len(files) != 0 {
		t.Fatalf("guidance changed the repository: %v, %v", files, err)
	}
	if err := SetupCI([]string{"--help"}, &out); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("help: %v", err)
	}
	file := filepath.Join(repo, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--repo", file}, {"--repo", file + "/missing"}, {"unexpected"}, {"--unknown"}} {
		if err := SetupCI(args, io.Discard); err == nil {
			t.Errorf("accepted invalid arguments: %v", args)
		}
	}
	if err := SetupCI([]string{"--repo", repo}, setupCIFailedWriter{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("lost output failure: %v", err)
	}
}

type setupCIFailedWriter struct{}

func (setupCIFailedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// Execute the actual printed recipe so documentation cannot silently teach a
// classifier that skips application tests for mixed edits or code renames.
func TestSetupCIExampleClassifiesGitChanges(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is needed to exercise the printed GitHub runner recipe")
	}
	script := setupCIExample(t, "          python3 - <<'PY'\n", "          PY\n")
	for _, tc := range []struct {
		name, oldPath, newPath, want string
	}{
		{"saga only", "", "change.saga/story.json", "false"},
		{"spaces and newline", "", "change.saga/a b\nc.json", "false"},
		{"code only", "", "src/app.go", "true"},
		{"mixed", "", "src/new.go", "true"},
		{"similar prefix", "", "change.saga-backup/file", "true"},
		{"nested saga", "", "apps/change.saga/file", "true"},
		{"code moved into saga", "src/app.go", "change.saga/app.go", "true"},
		{"saga moved into code", "change.saga/old.json", "src/app.go", "true"},
		{"code deleted", "src/app.go", "", "true"},
		{"saga deleted", "change.saga/old.json", "", "false"},
		{"empty diff", "", "", "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && strings.Contains(tc.newPath, "\n") {
				t.Skip("Windows filenames cannot contain newlines")
			}
			repo := t.TempDir()
			git := func(args ...string) string {
				t.Helper()
				cmd := exec.Command("git", args...)
				cmd.Dir = repo
				cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v: %s", args, err, out)
				}
				return strings.TrimSpace(string(out))
			}
			write := func(path, text string) {
				t.Helper()
				full := filepath.Join(repo, path)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			git("init", "-q")
			if tc.oldPath != "" {
				write(tc.oldPath, "original")
			}
			git("add", ".")
			git("-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-qm", "base")
			base := git("rev-parse", "HEAD")
			if tc.oldPath != "" {
				if err := os.Remove(filepath.Join(repo, tc.oldPath)); err != nil {
					t.Fatal(err)
				}
			}
			if tc.newPath != "" {
				write(tc.newPath, "original")
			}
			if tc.name == "mixed" {
				write("change.saga/story.json", "story")
			}
			git("add", "-A")
			git("-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-qm", "head")
			head := git("rev-parse", "HEAD")
			for _, event := range []string{"pull_request", "merge_group"} {
				output := filepath.Join(t.TempDir(), "output")
				cmd := exec.Command(python, "-c", script)
				cmd.Dir = repo
				cmd.Env = append(os.Environ(), "BASE_SHA="+base, "HEAD_SHA="+head, "EVENT_NAME="+event, "GITHUB_OUTPUT="+output)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("classifier: %v: %s", err, out)
				}
				got, err := os.ReadFile(output)
				if err != nil || string(got) != "application="+tc.want+"\n" {
					t.Fatalf("%s: got %q, %v; want %s", event, got, err, tc.want)
				}
			}
			// An unavailable base must not manufacture a successful skip.
			cmd := exec.Command(python, "-c", script)
			cmd.Dir = repo
			cmd.Env = append(os.Environ(), "BASE_SHA=missing", "HEAD_SHA="+head, "EVENT_NAME=pull_request", "GITHUB_OUTPUT="+filepath.Join(t.TempDir(), "output"))
			if err := cmd.Run(); err == nil {
				t.Fatal("classifier accepted an unavailable base")
			}
		})
	}
}

func setupCIExample(t *testing.T, start, end string) string {
	t.Helper()
	_, body, found := strings.Cut(setupCIPrompt, start)
	if !found {
		t.Fatalf("missing recipe start %q", start)
	}
	body, _, found = strings.Cut(body, end)
	if !found {
		t.Fatalf("missing recipe end %q", end)
	}
	lines := strings.Split(body, "\n")
	for i := range lines {
		lines[i] = strings.TrimPrefix(lines[i], "          ")
	}
	return strings.Join(lines, "\n")
}
