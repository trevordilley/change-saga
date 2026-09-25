package gitexec

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxRequest keeps every request far below the smallest pipe buffer (4 KiB on
// Windows), so writing a request never blocks while Git is writing an answer.
const maxRequest = 1024

// batch is one long-lived Git process that answers a request per line.
type batch struct {
	mu       sync.Mutex
	command  *exec.Cmd
	stdin    io.WriteCloser
	stdout   *bufio.Reader
	sentinel string
	broken   bool
	exited   chan struct{}
}

func startBatch(args []string, withSentinel bool) (*batch, error) {
	command := exec.Command("git", args...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	started := &batch{command: command, stdin: stdin, stdout: bufio.NewReaderSize(stdout, 64*1024), exited: make(chan struct{})}
	if withSentinel {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			started.close()
			return nil, err
		}
		// A leading colon is never part of an object name, so diff-tree
		// echoes the line instead of reading it as a commit.
		started.sentinel = "::change-saga-end-" + hex.EncodeToString(random[:])
	}
	go func() {
		_ = command.Wait()
		close(started.exited)
	}()
	return started, nil
}

func (b *batch) isBroken() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.broken
}

// roundTrip writes request and reads one answer with read, or, when read is
// nil, everything before the echoed sentinel line. Any failure marks the
// process broken so the session starts a fresh one.
func (b *batch) roundTrip(ctx context.Context, request string, read func(*bufio.Reader) ([]byte, error)) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.broken {
		return nil, errors.New("git batch process is unavailable")
	}
	if b.sentinel != "" {
		request += b.sentinel + "\n"
	}
	stop := context.AfterFunc(ctx, func() { _ = b.command.Process.Kill() })
	defer stop()
	answer, err := b.exchange(request, read)
	if err != nil {
		b.broken = true
		_ = b.command.Process.Kill()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return answer, nil
}

func (b *batch) exchange(request string, read func(*bufio.Reader) ([]byte, error)) ([]byte, error) {
	if _, err := io.WriteString(b.stdin, request); err != nil {
		return nil, err
	}
	if read != nil {
		return read(b.stdout)
	}
	end := []byte(b.sentinel + "\n")
	var answer []byte
	for {
		chunk, err := b.stdout.ReadSlice('\n')
		answer = append(answer, chunk...)
		if err == nil && bytes.HasSuffix(answer, end) {
			before := len(answer) - len(end)
			// The sentinel follows either the start of the answer, a line,
			// or a NUL-terminated record (-z output).
			if before == 0 || answer[before-1] == '\n' || answer[before-1] == 0 {
				return answer[:before], nil
			}
		}
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
	}
}

func (b *batch) close() {
	_ = b.stdin.Close()
	select {
	case <-b.exited:
	case <-time.After(5 * time.Second):
		_ = b.command.Process.Kill()
		<-b.exited
	}
}

// ResolveCommit resolves revision to a commit object name the way
// `git rev-parse --verify --end-of-options <revision>^{commit}` does, using
// the session's batch process for repo, or without a session, rev-parse. A
// branch or HEAD is remembered across commands until the repository's refs
// or configuration change. It reports false whenever it cannot answer: no
// session for a revision that is not remembered by refs, a revision that
// cannot be sent on one line, a missing or ambiguous object, or a failed
// process. Callers then run rev-parse themselves, which also keeps Git's own
// error message for the user.
func ResolveCommit(ctx context.Context, repo, revision string) (string, bool) {
	if !refCacheable(revision) {
		return resolveCommit(ctx, repo, revision)
	}
	// A branch or HEAD names the same commit until refs change, so its
	// answer is remembered across commands under the repository's digest.
	commit, err := rememberRefs(ctx, repo, true, []string{"commit", revision}, func() ([]byte, error) {
		if commit, ok := resolveCommit(ctx, repo, revision); ok {
			return []byte(commit), nil
		}
		if sessionFrom(ctx) != nil {
			return nil, errUnanswered
		}
		output, err := exec.CommandContext(ctx, "git", "-C", repo, "rev-parse", "--verify", "--quiet", "--end-of-options", revision+"^{commit}").Output()
		if commit := strings.TrimSpace(string(output)); err == nil && IsObjectName(commit) {
			return []byte(commit), nil
		}
		return nil, errUnanswered
	})
	return string(commit), err == nil
}

var errUnanswered = errors.New("git could not answer")

func resolveCommit(ctx context.Context, repo, revision string) (string, bool) {
	session := sessionFrom(ctx)
	if session == nil || revision == "" || strings.ContainsAny(revision, "\n\r\x00") || len(revision) > maxRequest {
		return "", false
	}
	process, release := session.acquire(objectsArgs(repo), false)
	if process == nil {
		return "", false
	}
	defer release()
	answer, err := process.roundTrip(ctx, "info "+revision+"^{commit}\n", readLine)
	if err != nil {
		return "", false
	}
	fields := strings.Fields(string(answer))
	if len(fields) != 3 || fields[1] != "commit" || !IsObjectName(fields[0]) {
		return "", false
	}
	return fields[0], true
}

// objectInfo reports object's type, or "missing", through the session's
// cat-file process for repo.
func objectInfo(ctx context.Context, repo, object string) (string, bool) {
	session := sessionFrom(ctx)
	if session == nil || object == "" || strings.ContainsAny(object, "\n\r\x00") || len(object) > maxRequest {
		return "", false
	}
	process, release := session.acquire(objectsArgs(repo), false)
	if process == nil {
		return "", false
	}
	defer release()
	answer, err := process.roundTrip(ctx, "info "+object+"\n", readLine)
	if err != nil {
		return "", false
	}
	if trimmed := strings.TrimSuffix(string(answer), "\n"); strings.HasSuffix(trimmed, " missing") || strings.HasSuffix(trimmed, " ambiguous") {
		return "missing", true
	}
	fields := strings.Fields(string(answer))
	if len(fields) != 3 {
		return "", false
	}
	return fields[1], true
}

// objectsArgs is the one cat-file process a session keeps per repository for
// both revision lookups (info) and object reads (contents). --batch-command
// needs Git 2.36; an older Git fails the first request, and the session
// retires the invocation and falls back to one-shot commands.
func objectsArgs(repo string) []string {
	return []string{"-C", repo, "cat-file", "--batch-command=%(objectname) %(objecttype) %(objectsize)"}
}

// DiffTree returns what `git diff <from> <to>` prints for two commits, read
// from a long-lived `git -C repo diff-tree --stdin` started with args. args
// must be the rest of the invocation, including --stdin, --no-commit-id, -r,
// and any pathspec; one process serves every pair diffed with the same args.
// from and to must be full object IDs. It reports false when it cannot
// answer, and the caller then runs the one-shot diff instead.
func DiffTree(ctx context.Context, repo string, args []string, from, to string) ([]byte, bool) {
	session := sessionFrom(ctx)
	if session == nil || !IsObjectName(from) || !IsObjectName(to) {
		return nil, false
	}
	// diff-tree exits on a commit the repository lacks, as a pinned
	// reference's can be after gc; ask cat-file first so one absent commit
	// does not cost the process, and leave its error to the one-shot diff.
	for _, commit := range []string{from, to} {
		if kind, ok := objectInfo(ctx, repo, commit); !ok || kind != "commit" {
			return nil, false
		}
	}
	process, release := session.acquire(append([]string{"-C", repo}, args...), true)
	if process == nil {
		return nil, false
	}
	defer release()
	// diff-tree reads "<commit> <parent>": the newer side comes first.
	answer, err := process.roundTrip(ctx, to+" "+from+"\n", nil)
	if err != nil {
		return nil, false
	}
	return answer, true
}

// ReadObject reads an object the way `git cat-file --batch` reports it,
// through the session's batch process for repo: its type and content, or
// type "missing" when name names no single object. It reports false when it
// cannot answer, and the caller then reads the object itself.
func ReadObject(ctx context.Context, repo, name string) (string, []byte, bool) {
	session := sessionFrom(ctx)
	if session == nil || name == "" || strings.ContainsAny(name, "\n\r\x00") || len(name) > maxRequest {
		return "", nil, false
	}
	process, release := session.acquire(objectsArgs(repo), false)
	if process == nil {
		return "", nil, false
	}
	defer release()
	var objectType string
	content, err := process.roundTrip(ctx, "contents "+name+"\n", func(reader *bufio.Reader) ([]byte, error) {
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		// A name Git cannot resolve is echoed back, spaces and all.
		if trimmed := strings.TrimSuffix(header, "\n"); strings.HasSuffix(trimmed, " missing") || strings.HasSuffix(trimmed, " ambiguous") {
			objectType = "missing"
			return nil, nil
		}
		fields := strings.Fields(header)
		if len(fields) != 3 {
			return nil, fmt.Errorf("unexpected cat-file header %q", strings.TrimSpace(header))
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 {
			return nil, fmt.Errorf("unexpected cat-file header %q", strings.TrimSpace(header))
		}
		content := make([]byte, size+1)
		if _, err := io.ReadFull(reader, content); err != nil {
			return nil, err
		}
		objectType = fields[1]
		return content[:size], nil
	})
	if err != nil {
		return "", nil, false
	}
	return objectType, content, true
}

func readLine(reader *bufio.Reader) ([]byte, error) { return reader.ReadBytes('\n') }

// IsObjectName reports whether value is a full SHA-1 or SHA-256 object name.
func IsObjectName(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !('0' <= r && r <= '9' || 'a' <= r && r <= 'f') {
			return false
		}
	}
	return true
}
