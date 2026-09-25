// Package gitexec runs Git on behalf of one command without spawning a new
// process for every question.
//
// Process creation dominates the cost of asking Git small questions, and on
// Windows it costs tens of milliseconds per process. A command that locates
// the repository, resolves revisions, and diffs many commit pairs used to pay
// that for every answer. A Session scopes answers to one command: read-only
// queries are memoized, commits are resolved through one long-lived
// `git cat-file --batch-check`, and commit-pair diffs stream through one
// long-lived `git diff-tree --stdin` per option set.
//
// No supported command mutates Git state, so answers cannot go stale within
// a session. Without a session every call spawns Git exactly as before.
package gitexec

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"sync"
)

type sessionKey struct{}

// Session holds the memoized answers and batch processes of one command.
type Session struct {
	mu      sync.Mutex
	memo    map[string]*call
	batches map[string]*batch
	closed  bool
}

type call struct {
	done   chan struct{}
	output []byte
	err    error
}

// Begin attaches a session to ctx. A context that already carries one keeps
// it, so a command called from another shares its caller's session and the
// returned end function is a no-op. The outermost end function stops every
// batch process and waits for it to exit; on Windows a live child holds its
// working directory open, so it must not outlive the command.
func Begin(ctx context.Context) (context.Context, func()) {
	if _, ok := ctx.Value(sessionKey{}).(*Session); ok {
		return ctx, func() {}
	}
	session := &Session{memo: map[string]*call{}, batches: map[string]*batch{}}
	return context.WithValue(ctx, sessionKey{}, session), session.close
}

func sessionFrom(ctx context.Context) *Session {
	session, _ := ctx.Value(sessionKey{}).(*Session)
	return session
}

// Output runs `git args...` and returns its standard output, memoized within
// the session. Use it only for queries whose answer depends on nothing but
// committed Git state and repository configuration.
func Output(ctx context.Context, args ...string) ([]byte, error) {
	return run(ctx, false, args)
}

// CombinedOutput is Output with standard error interleaved, for callers that
// report Git's own message on failure.
func CombinedOutput(ctx context.Context, args ...string) ([]byte, error) {
	return run(ctx, true, args)
}

func run(ctx context.Context, combined bool, args []string) ([]byte, error) {
	session := sessionFrom(ctx)
	if session == nil {
		return spawn(ctx, combined, args)
	}
	key := strings.Join(args, "\x00")
	if combined {
		key = "combined\x00" + key
	}
	session.mu.Lock()
	if existing, ok := session.memo[key]; ok {
		session.mu.Unlock()
		select {
		case <-existing.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return bytes.Clone(existing.output), existing.err
	}
	current := &call{done: make(chan struct{})}
	session.memo[key] = current
	session.mu.Unlock()

	current.output, current.err = spawn(ctx, combined, args)
	if ctx.Err() != nil {
		// A cancelled caller's failure is not Git's answer.
		session.mu.Lock()
		delete(session.memo, key)
		session.mu.Unlock()
	}
	close(current.done)
	return bytes.Clone(current.output), current.err
}

func spawn(ctx context.Context, combined bool, args []string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", args...)
	if combined {
		return command.CombinedOutput()
	}
	return command.Output()
}

func (session *Session) close() {
	session.mu.Lock()
	session.closed = true
	batches := session.batches
	session.batches = map[string]*batch{}
	session.mu.Unlock()
	for _, batch := range batches {
		batch.close()
	}
}

// batchFor returns the session's live process for args, starting one when
// none is running. It returns nil when there is no session or Git cannot be
// started; callers then spawn a one-shot command instead.
func (session *Session) batchFor(args []string, sentinel bool) *batch {
	key := strings.Join(args, "\x00")
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return nil
	}
	if existing, ok := session.batches[key]; ok && !existing.isBroken() {
		return existing
	}
	started, err := startBatch(args, sentinel)
	if err != nil {
		return nil
	}
	if previous, ok := session.batches[key]; ok {
		go previous.close()
	}
	session.batches[key] = started
	return started
}
