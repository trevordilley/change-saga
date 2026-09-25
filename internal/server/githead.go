package server

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// headReader reads a repository's HEAD commit for every freshness check.
// Starting git for each check cost more than the check's walk of the Saga,
// so it reads HEAD from the repository's own files, as git does, and asks
// git whenever those files say anything it does not read plainly: a
// reftable store, a symbolic ref chain, a ref it cannot find.
type headReader struct {
	once      sync.Once
	gitDir    string
	commonDir string
	// plain is false when the repository's files cannot be read directly,
	// and every read asks git.
	plain bool
}

// head is the full object name HEAD resolves to in the repository at dir, or
// "" when it resolves to nothing, as git rev-parse HEAD reports it.
func (reader *headReader) head(ctx context.Context, dir string) string {
	reader.once.Do(func() {
		output, err := gitOutput(ctx, dir, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
		if err != nil {
			return
		}
		lines := strings.Split(output, "\n")
		if len(lines) != 2 {
			return
		}
		reader.gitDir, reader.commonDir = strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
		if _, err := os.Stat(filepath.Join(reader.commonDir, "reftable")); !errors.Is(err, fs.ErrNotExist) {
			return
		}
		reader.plain = reader.gitDir != "" && reader.commonDir != ""
	})
	if reader.plain {
		if oid, ok := reader.read(); ok {
			return oid
		}
	}
	head, _ := gitOutput(ctx, dir, "rev-parse", "HEAD")
	return head
}

// read resolves HEAD from the repository's files alone.
func (reader *headReader) read() (string, bool) {
	content, err := os.ReadFile(filepath.Join(reader.gitDir, "HEAD"))
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(string(content))
	if objectName(value) {
		return value, true
	}
	ref, symbolic := strings.CutPrefix(value, "ref: ")
	if !symbolic || !strings.HasPrefix(ref, "refs/heads/") || strings.Contains(ref, "..") {
		return "", false
	}
	if loose, err := os.ReadFile(filepath.Join(reader.commonDir, filepath.FromSlash(ref))); err == nil {
		oid := strings.TrimSpace(string(loose))
		return oid, objectName(oid)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	packed, err := os.ReadFile(filepath.Join(reader.commonDir, "packed-refs"))
	if err != nil {
		return "", false
	}
	scanner := bufio.NewScanner(bytes.NewReader(packed))
	for scanner.Scan() {
		oid, name, found := strings.Cut(scanner.Text(), " ")
		if found && name == ref && objectName(oid) {
			return oid, true
		}
	}
	return "", false
}

// objectName says whether value is a full SHA-1 or SHA-256 object name.
func objectName(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
