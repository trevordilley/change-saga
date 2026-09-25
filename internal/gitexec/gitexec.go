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
	mu   sync.Mutex
	cond *sync.Cond
	memo map[string]*call
	// pools holds the batch processes of each invocation. Parallel callers
	// each get their own process, up to maxBatchProcesses.
	pools map[string]*batchPool
	// failures counts batch processes that broke, per invocation. A Git that
	// cannot serve an invocation at all must not cost a spawn per question.
	failures map[string]int
	// digests holds each repository's ref and configuration digest, taken
	// once per session; "" means the repository cannot be summarized.
	digests map[string]string
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
	if sessionFrom(ctx) != nil {
		return ctx, func() {}
	}
	return BeginDetached(ctx)
}

// BeginDetached starts a session of its own even when ctx carries one, for
// work that outlives its caller, such as a build a request starts in the
// background: the caller's session ends when the caller returns.
func BeginDetached(ctx context.Context) (context.Context, func()) {
	session := &Session{memo: map[string]*call{}, pools: map[string]*batchPool{}, failures: map[string]int{}, digests: map[string]string{}}
	session.cond = sync.NewCond(&session.mu)
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
	var idle []*batch
	for _, pool := range session.pools {
		idle = append(idle, pool.idle...)
		pool.idle = nil
	}
	session.cond.Broadcast()
	session.mu.Unlock()
	// A process still in use is stopped when its caller releases it.
	for _, process := range idle {
		process.close()
	}
}

// maxBatchFailures is how many broken processes an invocation may leave
// before the session stops starting it and callers spawn one-shot commands.
const maxBatchFailures = 3

// maxBatchProcesses bounds the processes one invocation may run at once.
const maxBatchProcesses = 4

type batchPool struct {
	idle []*batch
	live int
}

// acquire returns an idle process for args, starting one while fewer than
// maxBatchProcesses run and otherwise waiting for one to be released. It
// returns nil when the session is closed or Git cannot serve args; callers
// then spawn a one-shot command instead. The caller must call release.
func (session *Session) acquire(args []string, sentinel bool) (process *batch, release func()) {
	key := strings.Join(args, "\x00")
	var retired []*batch
	defer func() {
		for _, broken := range retired {
			broken.close()
		}
	}()
	session.mu.Lock()
	defer session.mu.Unlock()
	pool := session.pools[key]
	if pool == nil {
		pool = &batchPool{}
		session.pools[key] = pool
	}
	for {
		if session.closed || session.failures[key] >= maxBatchFailures {
			return nil, nil
		}
		for len(pool.idle) > 0 {
			candidate := pool.idle[len(pool.idle)-1]
			pool.idle = pool.idle[:len(pool.idle)-1]
			if !candidate.isBroken() {
				return candidate, session.releaser(pool, candidate)
			}
			pool.live--
			retired = append(retired, candidate)
			session.failures[key]++
		}
		if session.failures[key] >= maxBatchFailures {
			return nil, nil
		}
		if pool.live < maxBatchProcesses {
			started, err := startBatch(args, sentinel)
			if err != nil {
				session.failures[key] = maxBatchFailures
				return nil, nil
			}
			pool.live++
			return started, session.releaser(pool, started)
		}
		session.cond.Wait()
	}
}

func (session *Session) releaser(pool *batchPool, process *batch) func() {
	return func() {
		session.mu.Lock()
		if session.closed {
			pool.live--
			session.mu.Unlock()
			process.close()
			return
		}
		pool.idle = append(pool.idle, process)
		session.cond.Broadcast()
		session.mu.Unlock()
	}
}
