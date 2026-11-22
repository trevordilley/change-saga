package currentrecord

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/sagalineage"
)

// GitContent retrieves matching historical content from Git, never a successor
// and never a permanent Saga history cache. Commit and File identify the actual
// Git object inspected. A missing commit/content is an explicit unresolved pin.
type GitContent struct {
	Content      json.RawMessage
	Commit, File string
}

// GitSource identifies the actual repository for an extracted saved view.
// Commit bounds lookup to that view's ancestors; an empty Commit means HEAD.
type GitSource struct {
	Repository, SagaPath, Commit string
}

func FindGitContent(root, packagePath, digest string) (GitContent, error) {
	top, err := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return GitContent{}, fmt.Errorf("historical pin requires its Git repository: %w", err)
	}
	prefix, err := exec.Command("git", "-C", root, "rev-parse", "--show-prefix").Output()
	if err != nil {
		return GitContent{}, err
	}
	return FindGitContentAt(GitSource{Repository: strings.TrimSpace(string(top)), SagaPath: strings.TrimSpace(string(prefix))}, packagePath, digest)
}

func FindGitContentAt(source GitSource, packagePath, digest string) (GitContent, error) {
	if !ValidDigest(digest) {
		return GitContent{}, fmt.Errorf("historical content requires a SHA-256 digest")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	gitDir := source.Repository
	run := func(args ...string) ([]byte, error) {
		data, err := exec.CommandContext(ctx, "git", append([]string{"-C", gitDir}, args...)...).Output()
		if len(data) > 32<<20 {
			return nil, fmt.Errorf("Git history result exceeds 32 MiB")
		}
		return data, err
	}
	head := source.Commit
	if head == "" {
		head = "HEAD"
	}
	lineage := sagalineage.Of(ctx, gitDir, strings.TrimSuffix(source.SagaPath, "/"))
	paths := []string{path.Join(source.SagaPath, filepath.ToSlash(packagePath))}
	for _, move := range lineage.Moves {
		paths = append(paths, path.Join(move.From, filepath.ToSlash(packagePath)))
	}
	args := append([]string{"log", "--max-count=512", "--format=%H", head, "--"}, paths...)
	log, err := run(args...)
	if err != nil {
		return GitContent{}, err
	}
	commits := strings.Fields(string(log))
	for _, commit := range commits {
		sagaPath, exists := lineage.PathAt(ctx, gitDir, commit)
		if !exists {
			continue
		}
		recordPath := path.Join(sagaPath, filepath.ToSlash(packagePath))
		treeArgs := []string{"ls-tree", "-r", "--name-only", commit}
		if sagaPath != "" {
			treeArgs = append(treeArgs, "--", strings.TrimSuffix(sagaPath, "/"))
		}
		tree, err := run(treeArgs...)
		if err != nil {
			return GitContent{}, err
		}
		files := strings.Split(strings.TrimSpace(string(tree)), "\n")
		read := func(file string) (json.RawMessage, error) {
			data, err := run("show", commit+":"+file)
			if err != nil {
				return nil, err
			}
			if len(data) > MaxBytes {
				return nil, fmt.Errorf("historical record exceeds %d bytes", MaxBytes)
			}
			return data, nil
		}
		pins := &Pins{sources: map[string]json.RawMessage{}, digests: map[string]string{}, visiting: map[string]bool{}}
		pins.source = func(pin string) (json.RawMessage, error) {
			parts := strings.Split(pin, ":")
			if len(parts) != 7 || parts[5] != "revision" {
				return nil, fmt.Errorf("invalid historical pin %s", pin)
			}
			kind, id, revision := parts[3], parts[4], parts[6]
			suffix := "/" + id + "." + kind + "/revisions/" + revision + ".json"
			if kind == "test-case" {
				suffix = "/" + id + ".test/revisions/" + revision + ".json"
			}
			if kind == "prototype" {
				suffix = "/" + id + ".prototype/revisions/" + revision + ".revision/revision.json"
			}
			if kind == "whiteboard" {
				suffix = "/___whiteboards/" + id + "/revisions/" + revision + ".json"
			}
			matches := []string{}
			for _, file := range files {
				if strings.HasSuffix("/"+file, suffix) {
					matches = append(matches, file)
				}
			}
			if len(matches) != 1 {
				return nil, fmt.Errorf("historical pin %s has %d sources at Git commit %s", pin, len(matches), commit)
			}
			return read(matches[0])
		}
		for _, file := range files {
			if !strings.HasPrefix(file, recordPath+"/") {
				continue
			}
			legacy := strings.Contains(file, "/revisions/") && (strings.HasSuffix(file, ".json") || strings.HasSuffix(file, "/revision.json"))
			if !legacy && file != path.Join(recordPath, Name) {
				continue
			}
			data, err := read(file)
			if err != nil {
				return GitContent{}, err
			}
			if !legacy {
				var record Record
				if err := json.Unmarshal(data, &record); err != nil {
					return GitContent{}, err
				}
				data = record.Content
			}
			normalized, err := pins.Normalize(data)
			if err != nil {
				continue
			}
			var value any
			if err := json.Unmarshal(normalized, &value); err != nil {
				return GitContent{}, err
			}
			got, err := Digest(value)
			if err != nil {
				return GitContent{}, err
			}
			if got == digest {
				content, err := Content(value)
				return GitContent{Content: content, Commit: commit, File: file}, err
			}
		}
	}
	return GitContent{}, fmt.Errorf("content %s is unavailable in Git for %s; its pin remains stale/unresolved", digest, packagePath)
}
