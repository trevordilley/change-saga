package gitexec

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Answers that depend on refs and configuration (where a checkout's
// repository is, what origin points at, what HEAD or a branch names) are
// remembered across commands too, but each is served only while its
// repository's state digest is unchanged. The digest covers the contents of
// HEAD, every loose ref, packed-refs, the reftable table list, the
// repository and global configuration files, and the Git environment. Git
// rewrites every one of those through a lock file, so any change that could
// move an answer changes the digest, whatever the file system's timestamp
// resolution. Configuration pulled in through include directives and the
// system-wide configuration file are not covered.

// maxStateBytes bounds how much a digest reads; a repository whose refs
// exceed it is simply never remembered.
const maxStateBytes = 16 << 20

type repoLocation struct {
	top       string
	gitDir    string
	commonDir string
	// between are the directories from the queried one up to, but not
	// including, the top level; a .git appearing in any of them would
	// change the answer.
	between []string
	topInfo os.FileInfo
}

var locations sync.Map // gitEnvironment + absolute directory -> *repoLocation

// TopLevel returns what `git -C dir rev-parse --show-toplevel` prints,
// trimmed. The answer is remembered for the process while dir, the
// directories above it, and the repository's own directory are unchanged.
// A failure is Git's message and is never remembered.
func TopLevel(ctx context.Context, dir string) (string, error) {
	location, err := locate(ctx, dir)
	if err != nil {
		return "", err
	}
	return location.top, nil
}

func locate(ctx context.Context, dir string) (*repoLocation, error) {
	abs, absErr := filepath.Abs(dir)
	key := gitEnvironment() + "\x00" + abs
	remember := absErr == nil && remembers(ctx)
	if remember {
		if cached, ok := locations.Load(key); ok && cached.(*repoLocation).stillAt(abs) {
			return cached.(*repoLocation), nil
		}
	}
	output, err := CombinedOutput(ctx, "-C", dir, "rev-parse", "--show-toplevel", "--absolute-git-dir", "--git-common-dir")
	if err != nil {
		// Git's own message, or why Git could not run at all.
		if message := strings.TrimSpace(string(output)); message != "" {
			return nil, errors.New(message)
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(output), "\r\n"), "\n")
	if len(lines) != 3 {
		return nil, fmt.Errorf("unexpected rev-parse output %q", output)
	}
	location := &repoLocation{top: strings.TrimSpace(lines[0]), gitDir: filepath.FromSlash(strings.TrimSpace(lines[1])), commonDir: filepath.FromSlash(strings.TrimSpace(lines[2]))}
	if !filepath.IsAbs(location.commonDir) {
		location.commonDir = filepath.Join(abs, location.commonDir)
	}
	if remember && location.record(abs) {
		locations.Store(key, location)
	}
	return location, nil
}

// record notes the directories whose .git entries decide this answer. It
// reports false when the top level cannot be found above dir, in which case
// the answer is not remembered.
func (location *repoLocation) record(dir string) bool {
	topInfo, err := os.Stat(filepath.FromSlash(location.top))
	if err != nil {
		return false
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	for current := real; ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if err != nil {
			return false
		}
		if os.SameFile(info, topInfo) {
			location.topInfo = topInfo
			return true
		}
		location.between = append(location.between, current)
		if filepath.Dir(current) == current {
			return false
		}
	}
}

func (location *repoLocation) stillAt(dir string) bool {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return false
	}
	topInfo, err := os.Stat(filepath.FromSlash(location.top))
	if err != nil || !os.SameFile(topInfo, location.topInfo) {
		return false
	}
	if _, err := os.Stat(location.gitDir); err != nil {
		return false
	}
	for _, between := range location.between {
		if _, err := os.Lstat(filepath.Join(between, ".git")); !errors.Is(err, fs.ErrNotExist) {
			return false
		}
	}
	return true
}

// gitEnvironment is the part of the environment that can change what Git
// answers: every GIT_ variable and where global configuration lives.
func gitEnvironment() string {
	var relevant []string
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "GIT_") || strings.HasPrefix(entry, "HOME=") || strings.HasPrefix(entry, "XDG_CONFIG_HOME=") || strings.HasPrefix(entry, "USERPROFILE=") {
			relevant = append(relevant, entry)
		}
	}
	sort.Strings(relevant)
	return strings.Join(relevant, "\x00")
}

type refAnswer struct {
	digest string
	value  []byte
}

var refAnswers sync.Map // repository key + query -> refAnswer

// RepoOutput returns what `git -C repo args...` prints, remembered across
// commands while the repository's refs and configuration are unchanged. Use
// it only for queries answered by those alone, such as a remote's URL or
// what HEAD names. Failures are never remembered.
func RepoOutput(ctx context.Context, repo string, args ...string) ([]byte, error) {
	return rememberRefs(ctx, repo, true, append([]string{"output"}, args...), func() ([]byte, error) {
		return Output(ctx, append([]string{"-C", repo}, args...)...)
	})
}

// ConfigOutput is RepoOutput for queries answered by configuration alone,
// such as a remote's URL, so moving a ref does not make them ask again.
func ConfigOutput(ctx context.Context, repo string, args ...string) ([]byte, error) {
	return rememberRefs(ctx, repo, false, append([]string{"config"}, args...), func() ([]byte, error) {
		return Output(ctx, append([]string{"-C", repo}, args...)...)
	})
}

func rememberRefs(ctx context.Context, repo string, withRefs bool, query []string, compute func() ([]byte, error)) ([]byte, error) {
	if !remembers(ctx) {
		return compute()
	}
	location, err := locate(ctx, repo)
	if err != nil {
		return compute()
	}
	digest, ok := location.stateDigest(ctx, withRefs)
	if !ok {
		return compute()
	}
	key := gitEnvironment() + "\x00" + location.gitDir + "\x00" + repo + "\x00" + strings.Join(query, "\x00")
	if cached, found := refAnswers.Load(key); found && cached.(refAnswer).digest == digest {
		return bytes.Clone(cached.(refAnswer).value), nil
	}
	// The digest is taken before Git answers, so a change made meanwhile
	// only makes the next lookup ask again.
	value, err := compute()
	if err == nil {
		refAnswers.Store(key, refAnswer{digest: digest, value: bytes.Clone(value)})
	}
	return value, err
}

// stateDigest is digest, computed once per session: no supported command
// changes refs or configuration while it runs.
func (location *repoLocation) stateDigest(ctx context.Context, withRefs bool) (string, bool) {
	session := sessionFrom(ctx)
	if session == nil {
		return location.digest(withRefs)
	}
	key := location.gitDir
	if withRefs {
		key = "refs\x00" + key
	}
	session.mu.Lock()
	cached, ok := session.digests[key]
	session.mu.Unlock()
	if ok {
		return cached, cached != ""
	}
	digest, ok := location.digest(withRefs)
	session.mu.Lock()
	session.digests[key] = digest
	session.mu.Unlock()
	return digest, ok
}

// refCacheable reports whether a revision's commit is decided by refs and
// immutable history alone. Full object IDs ask whether an object exists,
// abbreviated ones can become ambiguous as objects arrive, @{...} reads
// reflogs, and a path names no commit.
func refCacheable(revision string) bool {
	if revision == "" || IsObjectName(revision) || strings.ContainsAny(revision, ": \t\n\r\x00") || strings.Contains(revision, "@{") {
		return false
	}
	return !looksAbbreviated(revision)
}

func looksAbbreviated(revision string) bool {
	head := revision
	if index := strings.IndexAny(head, "^~"); index >= 0 {
		head = head[:index]
	}
	if len(head) < 4 {
		return false
	}
	for _, r := range head {
		if !('0' <= r && r <= '9' || 'a' <= r && r <= 'f' || 'A' <= r && r <= 'F') {
			return false
		}
	}
	return true
}

// digest summarizes everything a configuration answer depends on and, with
// withRefs, everything a ref answer depends on too.
func (location *repoLocation) digest(withRefs bool) (string, bool) {
	hash := sha256.New()
	budget := maxStateBytes
	add := func(path string) bool {
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			hash.Write([]byte("absent\x00" + path + "\x00"))
			return true
		}
		if err != nil || len(data) > budget {
			return false
		}
		budget -= len(data)
		hash.Write([]byte("file\x00" + path + "\x00"))
		hash.Write(data)
		hash.Write([]byte{0})
		return true
	}
	walk := func(root string) bool {
		complete := true
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) && path == root {
				hash.Write([]byte("absent\x00" + path + "\x00"))
				return nil
			}
			if err != nil {
				return err
			}
			if entry.IsDir() {
				hash.Write([]byte("dir\x00" + path + "\x00"))
				return nil
			}
			if !add(path) {
				complete = false
				return filepath.SkipAll
			}
			return nil
		})
		return err == nil && complete
	}
	files := append([]string{
		filepath.Join(location.gitDir, "config.worktree"),
		filepath.Join(location.commonDir, "config"),
	}, globalConfigFiles()...)
	// Remotes may still be defined the legacy way, one file per remote.
	roots := []string{filepath.Join(location.commonDir, "remotes"), filepath.Join(location.commonDir, "branches")}
	if withRefs {
		files = append(files,
			filepath.Join(location.gitDir, "HEAD"),
			filepath.Join(location.commonDir, "packed-refs"),
			filepath.Join(location.commonDir, "reftable", "tables.list"),
		)
		roots = append(roots, filepath.Join(location.commonDir, "refs"))
		if location.gitDir != location.commonDir {
			roots = append(roots, filepath.Join(location.gitDir, "refs"))
		}
	}
	for _, path := range files {
		if !add(path) {
			return "", false
		}
	}
	for _, root := range roots {
		if !walk(root) {
			return "", false
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), true
}

func globalConfigFiles() []string {
	var files []string
	if global := os.Getenv("GIT_CONFIG_GLOBAL"); global != "" {
		files = append(files, global)
	} else if home, err := os.UserHomeDir(); err == nil {
		files = append(files, filepath.Join(home, ".gitconfig"))
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			files = append(files, filepath.Join(xdg, "git", "config"))
		} else {
			files = append(files, filepath.Join(home, ".config", "git", "config"))
		}
	}
	return files
}
