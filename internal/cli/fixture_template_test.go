package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// fixtureTemplate builds a fixture once per test binary and gives every
// caller a private copy of it. Building a repository and Saga costs a dozen
// Git processes, which dominate the suite on Windows; copying costs a few
// file writes. Git repositories and Sagas are relocatable, so a copy behaves
// exactly like a fresh build, down to its commit IDs.
type fixtureTemplate struct {
	once   sync.Once
	dir    string
	values map[string]string
}

// instantiate returns a fresh copy of the template and the values build
// reported (commit IDs and the like). build populates dir; callers locate
// what it built relative to the copy. Keep each Saga and repository in its
// own subdirectory, since tests may turn a Saga's parent into a repository.
func (template *fixtureTemplate) instantiate(t *testing.T, build func(t *testing.T, dir string) map[string]string) (string, map[string]string) {
	t.Helper()
	template.once.Do(func() {
		dir, err := os.MkdirTemp("", "cst")
		if err != nil {
			t.Fatal(err)
		}
		templateDirs.Store(dir, true)
		values := build(t, dir)
		template.dir, template.values = dir, values
	})
	if template.dir == "" {
		t.Fatal("the fixture template could not be built")
	}
	dir := shortTempDir(t)
	copyTree(t, template.dir, dir)
	return dir, template.values
}

// templateDirs outlive any one test and are removed by TestMain.
var templateDirs sync.Map

func TestMain(m *testing.M) {
	code := m.Run()
	templateDirs.Range(func(dir, _ any) bool {
		_ = os.RemoveAll(dir.(string))
		return true
	})
	os.Exit(code)
}

// copyTree copies the regular files and directories beneath from into to.
func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("copy %s: not a regular file", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}

// mkdir creates dir and returns it.
func mkdir(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
