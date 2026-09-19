package changeview

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// Location is where a Saga lives in its own Git repository.
type Location struct {
	// Repo is the top level of the Git repository holding the Saga.
	Repo string
	// Path is the Saga directory relative to Repo, in slash form.
	Path string
}

// Locate finds the Git repository that holds the Saga at root.
func Locate(ctx context.Context, root string) (Location, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Location{}, err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	output, err := exec.CommandContext(ctx, "git", "-C", abs, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return Location{}, fmt.Errorf("the Saga at %s is not in a Git repository", root)
	}
	repo := strings.TrimSpace(string(output))
	if resolved, err := filepath.EvalSymlinks(repo); err == nil {
		repo = resolved
	}
	relative, err := filepath.Rel(repo, abs)
	if err != nil {
		return Location{}, err
	}
	return Location{Repo: repo, Path: filepath.ToSlash(relative)}, nil
}

// errAbsent reports that the Saga did not exist at the commit.
var errAbsent = errors.New("the Saga does not exist at that commit")

// extract writes the Saga as of commit into a new directory beneath dest and
// returns that directory. It returns errAbsent when the commit has no Saga.
func extract(ctx context.Context, location Location, commit, dest string) (string, error) {
	if location.Path == "" || location.Path == "." {
		return "", fmt.Errorf("a Saga at the root of its repository cannot be read at a commit")
	}
	command := exec.CommandContext(ctx, "git", "-C", location.Repo, "archive", "--format=tar", commit, "--", location.Path)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		if strings.Contains(stderr.String(), "did not match any files") {
			return "", errAbsent
		}
		return "", fmt.Errorf("read the Saga at %s: %s", commit, strings.TrimSpace(stderr.String()))
	}
	reader := tar.NewReader(bytes.NewReader(output))
	root := filepath.Join(dest, filepath.FromSlash(location.Path))
	prefix := strings.TrimSuffix(location.Path, "/") + "/"
	wrote := false
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		name := path.Clean(header.Name)
		if !strings.HasPrefix(name+"/", prefix) || strings.Contains(name, "..") {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			data, err := io.ReadAll(reader)
			if err != nil {
				return "", err
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				return "", err
			}
			wrote = true
		}
	}
	if !wrote {
		return "", errAbsent
	}
	return root, nil
}
