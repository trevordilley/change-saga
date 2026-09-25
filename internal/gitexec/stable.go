package gitexec

import (
	"bytes"
	"container/list"
	"context"
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
	if sessionFrom(ctx) == nil || !NamesObjects(objects...) {
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
