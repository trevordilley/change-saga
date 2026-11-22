package prototypes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/store"
)

// publishHTML prepares immutable asset bytes before the single current record
// references them. Asset digests are evidence pins, not content revision IDs.
func publishHTML(dir, source string) (string, error) {
	assets, err := store.EnsureDirWithin(dir, filepath.Join(dir, "assets"))
	if err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(assets, ".change-saga-stage-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err := copyHTMLSource(source, filepath.Join(stage, "html")); err != nil {
		return "", err
	}
	digest, err := digestTree(filepath.Join(stage, "html"))
	if err != nil {
		return "", err
	}
	final := filepath.Join(assets, strings.TrimPrefix(digest, "sha256:"))
	if info, err := os.Lstat(final); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("prototype asset must be a real directory")
		}
		actual, err := digestTree(filepath.Join(final, "html"))
		if err != nil || actual != digest {
			return "", fmt.Errorf("prototype asset digest mismatch")
		}
		return digest, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(stage, final); err != nil {
		return "", err
	}
	if err := store.SyncDir(assets); err != nil {
		return "", err
	}
	return digest, nil
}
