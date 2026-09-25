package cli

import (
	"os"
	"testing"
)

// templateDirs are fixture templates built once per test binary and shared
// by copying; they outlive any one test, so they are removed here.
var templateDirs []string

func TestMain(m *testing.M) {
	code := m.Run()
	for _, dir := range templateDirs {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}
