package gitexec

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// stableLimit bounds the memory the process-wide cache holds. A long-running
// server keeps it for its lifetime, so it evicts least recently used answers.
const stableLimit = 64 << 20

// stable holds answers fixed by the full object IDs they name. Commits and
// trees are content-addressed: a commit's ID covers its parents and tree, so
// its ancestry, its merge-bases with another commit, and the diff between two
// commits can never change while those commits exist.
var stable = &answerCache{entries: map[string]*list.Element{}, order: list.New(), limit: stableLimit}

type answerCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List
	size    int
	limit   int
}

type cachedAnswer struct {
	key   string
	value []byte
}

// Stable returns compute's answer for key, remembered for the life of the
// process while it keeps succeeding. Use it only for answers fixed by the
// full object IDs in objects (ancestry, merge-bases, diffs), never for
// whether they exist: gc prunes unreachable commits, as after a squash merge.
// A remembered answer is therefore served only after the session's cat-file
// process confirms every object is still in repo, which starts no new
// process. Without a session it computes the answer and remembers nothing.
// Failures are never remembered.
func Stable(ctx context.Context, repo string, objects []string, key []string, compute func() ([]byte, error)) ([]byte, error) {
	if sessionFrom(ctx) == nil || !NamesObjects(objects...) || rewritesHistory(ctx, repo) {
		return compute()
	}
	joined := strings.Join(append([]string{repo}, key...), "\x00")
	if value, ok := stable.get(joined); ok {
		if present(ctx, repo, objects) {
			return value, nil
		}
		stable.remove(joined)
	}
	value, err := compute()
	if err == nil {
		stable.put(joined, value)
	}
	return value, err
}

// StableDiff is Stable for a diff between commits. Git also reads attributes
// from the checkout, nested .gitattributes files included, and no key here
// covers them; so a long-running process, whose requests may outlive an
// edit to one, remembers a diff only for the session that asked.
func StableDiff(ctx context.Context, repo string, objects []string, key []string, compute func() ([]byte, error)) ([]byte, error) {
	if remembers(ctx) {
		return Stable(ctx, repo, objects, key, compute)
	}
	session := sessionFrom(ctx)
	if session == nil {
		return compute()
	}
	joined := strings.Join(append([]string{repo}, key...), "\x00")
	session.mu.Lock()
	value, ok := session.diffs[joined]
	session.mu.Unlock()
	if ok {
		return bytes.Clone(value), nil
	}
	value, err := compute()
	if err == nil {
		session.mu.Lock()
		session.diffs[joined] = bytes.Clone(value)
		session.mu.Unlock()
	}
	return value, err
}

// rewritesHistory reports whether repo can make a commit's ID stop fixing
// its ancestry or content: grafts, a shallow boundary, and replace refs all
// change what Git reports for the same IDs. It is asked once per session.
func rewritesHistory(ctx context.Context, repo string) bool {
	session := sessionFrom(ctx)
	session.mu.Lock()
	rewritten, known := session.rewritten[repo]
	session.mu.Unlock()
	if known {
		return rewritten
	}
	rewritten = true
	if location, err := locate(ctx, repo); err == nil && os.Getenv("GIT_REPLACE_REF_BASE") == "" {
		rewritten = exists(filepath.Join(location.commonDir, "info", "grafts")) ||
			exists(filepath.Join(location.commonDir, "shallow")) ||
			hasFiles(filepath.Join(location.commonDir, "refs", "replace")) ||
			packedReplaceRefs(filepath.Join(location.commonDir, "packed-refs"))
	}
	session.mu.Lock()
	session.rewritten[repo] = rewritten
	session.mu.Unlock()
	return rewritten
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

// hasFiles reports whether the tree at root holds any file, or cannot be
// read.
func hasFiles(root string) bool {
	found := false
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found || (err != nil && !errors.Is(err, fs.ErrNotExist))
}

// packedReplaceRefs reports whether packed-refs names a replace ref, or
// cannot be read.
func packedReplaceRefs(path string) bool {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return err != nil || bytes.Contains(data, []byte(" refs/replace/"))
}

// present reports whether repo holds every object, asking the session's
// cat-file process.
func present(ctx context.Context, repo string, objects []string) bool {
	for _, object := range objects {
		object, _, _ = strings.Cut(object, ":")
		kind, ok := objectInfo(ctx, repo, object)
		if !ok || kind == "missing" {
			return false
		}
	}
	return true
}

func (cache *answerCache) remove(key string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if element, ok := cache.entries[key]; ok {
		answer := element.Value.(*cachedAnswer)
		cache.order.Remove(element)
		delete(cache.entries, key)
		cache.size -= len(answer.key) + len(answer.value)
	}
}

func (cache *answerCache) get(key string) ([]byte, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	element, ok := cache.entries[key]
	if !ok {
		return nil, false
	}
	cache.order.MoveToFront(element)
	return bytes.Clone(element.Value.(*cachedAnswer).value), true
}

func (cache *answerCache) put(key string, value []byte) {
	// One answer may not crowd out the rest of the cache.
	if len(value) > cache.limit/16 {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if element, ok := cache.entries[key]; ok {
		cache.order.MoveToFront(element)
		return
	}
	cache.entries[key] = cache.order.PushFront(&cachedAnswer{key: key, value: bytes.Clone(value)})
	cache.size += len(key) + len(value)
	for cache.size > cache.limit {
		oldest := cache.order.Back()
		answer := oldest.Value.(*cachedAnswer)
		cache.order.Remove(oldest)
		delete(cache.entries, answer.key)
		cache.size -= len(answer.key) + len(answer.value)
	}
}

// NamesObjects reports whether every name is a full object ID, or a full
// commit or tree ID followed by ":path", so that Stable may cache answers
// about them.
func NamesObjects(names ...string) bool {
	for _, name := range names {
		object, _, _ := strings.Cut(name, ":")
		if !IsObjectName(object) {
			return false
		}
	}
	return true
}
