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
	"os/exec"
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
// resolution. Configuration counts every file Git reads: the repository's,
// the system and global files Git reports, and whatever they include.

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
	// config is the configuration digest the answer was given under:
	// core.worktree, core.bare, and safe.directory can all move or refuse
	// the top level.
	config string
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
	if absErr == nil {
		if cached, ok := locations.Load(key); ok && cached.(*repoLocation).stillAt(abs) {
			return cached.(*repoLocation), nil
		}
	}
	// Where the repository probably is, read without Git, so its
	// configuration can be digested before Git answers: a change made while
	// Git runs then shows as a different digest after it.
	guess, guessed := guessLocation(abs)
	var before string
	if guessed && absErr == nil {
		before, guessed = guess.digest(false)
	}
	// Standard output alone: GIT_TRACE and warnings write to standard error.
	// --path-format=absolute resolves the git directories physically, so a
	// symlink above the checkout cannot misplace them.
	output, err := Output(ctx, "-C", dir, "rev-parse", "--show-toplevel", "--path-format=absolute", "--git-dir", "--git-common-dir")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if message := strings.TrimSpace(string(exitErr.Stderr)); message != "" {
				return nil, errors.New(message)
			}
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(output), "\r\n"), "\n")
	if len(lines) != 3 {
		return nil, fmt.Errorf("unexpected rev-parse output %q", output)
	}
	location := &repoLocation{top: strings.TrimSpace(lines[0]), gitDir: filepath.FromSlash(strings.TrimSpace(lines[1])), commonDir: filepath.FromSlash(strings.TrimSpace(lines[2]))}
	if afterDiscovery != nil {
		afterDiscovery()
	}
	if guessed && absErr == nil && sameDirectory(guess.gitDir, location.gitDir) && sameDirectory(guess.commonDir, location.commonDir) {
		if after, ok := location.digest(false); ok && after == before && location.record(abs, before) {
			locations.Store(key, location)
		}
	}
	return location, nil
}

// afterDiscovery lets a test change the repository while Git has answered
// but before the answer is remembered.
var afterDiscovery func()

// guessLocation finds the git directories of dir's checkout without Git: the
// nearest .git above dir, following a gitdir file and a commondir file as
// Git does. It is only ever checked against Git's own answer.
func guessLocation(dir string) (*repoLocation, bool) {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, false
	}
	for current := real; ; current = filepath.Dir(current) {
		dotGit := filepath.Join(current, ".git")
		info, err := os.Lstat(dotGit)
		if err == nil {
			gitDir := dotGit
			if !info.IsDir() {
				data, err := os.ReadFile(dotGit)
				pointer, found := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
				if err != nil || !found {
					return nil, false
				}
				gitDir = filepath.FromSlash(strings.TrimSpace(pointer))
				if !filepath.IsAbs(gitDir) {
					gitDir = filepath.Join(current, gitDir)
				}
			}
			commonDir := gitDir
			if data, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
				commonDir = filepath.FromSlash(strings.TrimSpace(string(data)))
				if !filepath.IsAbs(commonDir) {
					commonDir = filepath.Join(gitDir, commonDir)
				}
			}
			return &repoLocation{gitDir: gitDir, commonDir: commonDir}, true
		}
		if !errors.Is(err, fs.ErrNotExist) || filepath.Dir(current) == current {
			return nil, false
		}
	}
}

// sameDirectory reports whether two paths name one existing directory.
func sameDirectory(left, right string) bool {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && leftInfo.IsDir() && os.SameFile(leftInfo, rightInfo)
}

// record notes the directories whose .git entries decide this answer and the
// configuration digest it was given under. It reports false when the top
// level cannot be found above dir, in which case the answer is not
// remembered.
func (location *repoLocation) record(dir, config string) bool {
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
			location.topInfo, location.config = topInfo, config
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
	if config, ok := location.digest(false); !ok || config != location.config {
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
	// Missing directories would hash as absent files and hide every change.
	if !sameDirectory(location.gitDir, location.gitDir) || !sameDirectory(location.commonDir, location.commonDir) {
		return "", false
	}
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
	// A configuration file counts with every file it includes, whatever the
	// include's condition, followed as Git would.
	var addConfig func(path string, depth int) bool
	addConfig = func(path string, depth int) bool {
		if depth > maxIncludeDepth || !add(path) {
			return false
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return errors.Is(err, fs.ErrNotExist)
		}
		included, ok := includedConfigFiles(path, data)
		if !ok {
			return false
		}
		for _, include := range included {
			if !addConfig(include, depth+1) {
				return false
			}
		}
		return true
	}
	user, ok := userConfigFiles()
	if !ok {
		return "", false
	}
	for _, path := range append([]string{
		filepath.Join(location.gitDir, "config.worktree"),
		filepath.Join(location.commonDir, "config"),
	}, user...) {
		if !addConfig(path, 0) {
			return "", false
		}
	}
	// Remotes may still be defined the legacy way, one file per remote.
	roots := []string{filepath.Join(location.commonDir, "remotes"), filepath.Join(location.commonDir, "branches")}
	var files []string
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

// maxIncludeDepth matches Git's own limit on nested includes.
const maxIncludeDepth = 10

var userConfig sync.Map // gitEnvironment -> []string, or nil when unknown

// userConfigFiles are the system and global configuration files Git reads
// in this environment, as Git itself reports them. It reports false when
// this Git cannot say, and nothing that depends on configuration is then
// remembered.
func userConfigFiles() ([]string, bool) {
	key := gitEnvironment()
	if cached, ok := userConfig.Load(key); ok {
		files, _ := cached.([]string)
		return files, files != nil
	}
	files := []string{}
	system, err := exec.Command("git", "var", "GIT_CONFIG_SYSTEM").Output()
	switch {
	case err == nil:
		files = append(files, strings.Fields(string(system))...)
	case !noSystemConfig():
		userConfig.Store(key, nil)
		return nil, false
	}
	global, err := exec.Command("git", "var", "GIT_CONFIG_GLOBAL").Output()
	if err != nil {
		userConfig.Store(key, nil)
		return nil, false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(global)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, filepath.FromSlash(line))
		}
	}
	userConfig.Store(key, files)
	return files, true
}

// noSystemConfig reports whether GIT_CONFIG_NOSYSTEM turns the system file
// off, in which case git var reports no path for it.
func noSystemConfig() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GIT_CONFIG_NOSYSTEM"))) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

// includedConfigFiles lists the files a configuration file includes through
// [include] and [includeIf] path entries, resolved as Git resolves them. It
// reports false for any path it cannot resolve exactly (escapes, %(prefix)),
// so nothing is remembered on a guess.
func includedConfigFiles(file string, data []byte) ([]string, bool) {
	var included []string
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if strings.HasPrefix(line, "[") {
			end := strings.IndexByte(line, ']')
			if end < 0 {
				return nil, false
			}
			header := strings.ToLower(strings.TrimSpace(line[1:end]))
			section = strings.TrimSpace(strings.SplitN(strings.SplitN(header, "\"", 2)[0], " ", 2)[0])
			line = strings.TrimSpace(line[end+1:])
		}
		if line == "" || line[0] == '#' || line[0] == ';' || (section != "include" && section != "includeif") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "path") {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "\"") {
			closing := strings.IndexByte(value[1:], '"')
			if closing < 0 {
				return nil, false
			}
			value = value[1 : closing+1]
		} else if cut := strings.IndexAny(value, "#;"); cut >= 0 {
			value = strings.TrimSpace(value[:cut])
		}
		if value == "" || strings.ContainsAny(value, "\\\"") || strings.Contains(value, "%(") {
			return nil, false
		}
		switch {
		case strings.HasPrefix(value, "~/"):
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, false
			}
			value = filepath.Join(home, value[2:])
		case !strings.HasPrefix(value, "/") && !filepath.IsAbs(filepath.FromSlash(value)):
			// Git counts a leading slash as absolute on Windows too.
			value = filepath.Join(filepath.Dir(file), value)
		}
		included = append(included, filepath.FromSlash(value))
	}
	return included, true
}
