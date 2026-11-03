// Package planactivity keeps what agents say about a Plan's Tasks while they
// work: who started what, what they noted, what blocked them, and what they
// finished. It is a live view on top of the Plan, never part of it. The Plan's
// records stay the durable fact; losing this log loses nothing but the view.
package planactivity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/twentyideas/changesaga/internal/gitexec"
)

// The kinds of event a Task's log holds.
const (
	KindStart   = "start"
	KindNote    = "note"
	KindBlocked = "blocked"
	KindDone    = "done"
	// KindReleased says an agent let go of a Task, such as task set --state planned.
	KindReleased = "released"
)

const (
	// QuietAfter is how long a held Task goes without an update before it reads
	// as quiet. Agents are asked not to narrate, so a long gap is normal, and an
	// agent can crash: quiet never means failed.
	QuietAfter = 30 * time.Minute
	// MaxMessageRunes bounds one message, so one line stays one atomic append.
	MaxMessageRunes = 2000
	// maxAgentRunes bounds an agent's name.
	maxAgentRunes = 80
	// rotateAt is the size an agent's log rotates at, keeping one older file, so
	// a Plan's log holds at most twice this per agent.
	rotateAt = 128 << 10
	// retainFor is how long a log nobody has written to is kept.
	retainFor = 30 * 24 * time.Hour
	// rotationLockStale is how old a rotation lock must be to be some crashed
	// writer's, not a rotation still running.
	rotationLockStale = time.Minute
	openAttempts      = 8
	logSuffix         = ".jsonl"
	rotatedSuffix     = ".jsonl.1"
)

// NotInGit says why a Saga has no live activity when it is outside Git.
const NotInGit = "This Saga is not in a Git repository, so it has no live activity."

// Event is one thing an agent reported about a Task.
type Event struct {
	At      time.Time `json:"at"`
	Agent   string    `json:"agent"`
	Task    string    `json:"task"`
	Kind    string    `json:"kind"`
	Message string    `json:"message,omitempty"`
	// Branch is the Git branch of the checkout the agent reported from.
	Branch string `json:"branch,omitempty"`
}

// Log is where a Plan's activity is kept. The one implementation is local to a
// repository; a Log spanning machines could sit behind this later.
type Log interface {
	// Append adds one event to a Plan's log.
	Append(planID string, event Event) error
	// Read returns a Plan's events, oldest first. A Plan nobody has reported on
	// has none. A line that cannot be read is skipped.
	Read(planID string) ([]Event, error)
}

// Open finds the log of the Saga at root: the one in the common Git directory of
// the repository the Saga is committed to, so every worktree of that repository
// shares it. A Saga outside Git with a code checkout given by repo uses that
// checkout's. Without either there is no log, and reason says so.
func Open(ctx context.Context, root, repo, sagaID string) (log Log, reason string) {
	_, common, ok := gitexec.GitDirs(ctx, root)
	if !ok && repo != "" {
		_, common, ok = gitexec.GitDirs(ctx, repo)
	}
	if !ok {
		return nil, NotInGit
	}
	return &fileLog{root: filepath.Join(common, "change-saga", "activity", sagaID), cache: map[string]cachedFile{}}, ""
}

// Branch is the branch checked out at dir, or empty when it has none.
func Branch(ctx context.Context, dir string) string {
	output, err := gitexec.Output(ctx, "-C", dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// Identity names who is reporting: the override, else the DevSwarm workspace,
// else the Git branch. getenv reads the environment.
func Identity(override string, getenv func(string) string, branch string) string {
	if agent := strings.TrimSpace(override); agent != "" {
		if runes := []rune(agent); len(runes) > maxAgentRunes {
			return string(runes[:maxAgentRunes])
		}
		return agent
	}
	if name := strings.TrimSpace(getenv("DEVSWARM_BUILDER_NAME")); name != "" {
		// DevSwarm appends the start of its repository id to the workspace's name.
		if id := getenv("DEVSWARM_REPO_ID"); len(id) >= 8 {
			name = strings.TrimSuffix(name, "-"+id[:8])
		}
		return name
	}
	if branch != "" {
		return branch
	}
	return "unknown"
}

// fileLog keeps one small append-only file per agent, so parallel agents never
// write to the same file and their logs never conflict. It lives under the Git
// directory, so it is never committed and never changes the Saga's fingerprint.
type fileLog struct {
	root  string
	mutex sync.Mutex
	// cache keeps each file's parsed events, so a reader left polling reads only
	// the files that changed.
	cache map[string]cachedFile
}

type cachedFile struct {
	size    int64
	modTime time.Time
	events  []Event
}

// safeName matches an agent's name when it can be a file's name as it stands.
var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

// agentFile is the file an agent writes. A name that cannot be a file name, such
// as a branch with a slash, gets a digest so it stays distinct from a similar one.
func agentFile(agent string) string {
	if safeName.MatchString(agent) {
		return agent + logSuffix
	}
	clean := strings.Map(func(r rune) rune {
		if r < utf8.RuneSelf && (r == '-' || r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			return r
		}
		return '-'
	}, agent)
	if len(clean) > 60 {
		clean = clean[:60]
	}
	digest := sha256.Sum256([]byte(agent))
	return fmt.Sprintf("%s-%x%s", strings.TrimLeft(clean, "-."), digest[:6], logSuffix)
}

func (log *fileLog) planDir(planID string) string { return filepath.Join(log.root, planID) }

// Append writes the event as one line in one write to a file opened for
// appending, which the operating system keeps whole among concurrent writers.
func (log *fileLog) Append(planID string, event Event) error {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	event.Message = boundMessage(event.Message)
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	dir := log.planDir(planID)
	path := filepath.Join(dir, agentFile(event.Agent))
	info, statErr := os.Stat(path)
	if errors.Is(statErr, fs.ErrNotExist) {
		pruneQuietLogs(dir, time.Now())
	} else if statErr == nil && info.Size() >= rotateAt {
		rotate(path)
	}
	file, err := openForAppend(dir, path)
	if err != nil {
		return err
	}
	defer file.Close()
	// A crashed writer can leave a torn line. Ending it here keeps it from taking
	// this event with it when the file is read.
	if info, err := file.Stat(); err == nil && info.Size() > 0 {
		last := make([]byte, 1)
		if _, err := file.ReadAt(last, info.Size()-1); err == nil && last[0] != '\n' {
			line = append([]byte{'\n'}, line...)
		}
	}
	_, err = file.Write(append(line, '\n'))
	return err
}

// openForAppend tries a few times, because Windows refuses an open for a moment
// while another writer is rotating the file. A real failure costs only the wait.
func openForAppend(dir, path string) (*os.File, error) {
	var err error
	for attempt := range openAttempts {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 10 * time.Millisecond)
		}
		if err = os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		var file *os.File
		if file, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o644); err == nil {
			return file, nil
		}
	}
	return nil, err
}

func boundMessage(message string) string {
	message = strings.TrimSpace(message)
	if utf8.RuneCountInString(message) <= MaxMessageRunes {
		return message
	}
	return string([]rune(message)[:MaxMessageRunes-1]) + "…"
}

// rotate moves an agent's full log aside. Whoever holds the lock does it; anyone
// else just appends, and a writer that already opened the file finishes in the
// rotated one, which is still read.
func rotate(path string) {
	lock := path + ".rotating"
	if info, err := os.Stat(lock); err == nil && time.Since(info.ModTime()) > rotationLockStale {
		_ = os.Remove(lock)
	}
	file, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_ = file.Close()
	defer os.Remove(lock)
	if info, err := os.Stat(path); err == nil && info.Size() >= rotateAt {
		_ = os.Rename(path, strings.TrimSuffix(path, logSuffix)+rotatedSuffix)
	}
}

// pruneQuietLogs removes logs nobody has written to for retainFor. It runs when
// an agent first reports on a Plan, which is rare enough to afford a listing.
func pruneQuietLogs(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if info, err := entry.Info(); err == nil && now.Sub(info.ModTime()) > retainFor {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

// Read merges every agent's files. An absent directory is simply no activity.
func (log *fileLog) Read(planID string) ([]Event, error) {
	dir := log.planDir(planID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	events := []Event{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, logSuffix) && !strings.HasSuffix(name, rotatedSuffix) {
			continue
		}
		events = append(events, log.readFile(filepath.Join(dir, name))...)
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].At.Before(events[j].At) })
	return events, nil
}

// readFile answers from the cache while the file's size and time are unchanged.
func (log *fileLog) readFile(path string) []Event {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	log.mutex.Lock()
	cached, found := log.cache[path]
	log.mutex.Unlock()
	if found && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
		return cached.events
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	events := parseEvents(data)
	log.mutex.Lock()
	log.cache[path] = cachedFile{size: info.Size(), modTime: info.ModTime(), events: events}
	log.mutex.Unlock()
	return events
}

// parseEvents reads whole lines. A last line with no end may be half written, so
// it waits for the next read; any other line that does not parse is skipped.
func parseEvents(data []byte) []Event {
	end := bytes.LastIndexByte(data, '\n')
	if end < 0 {
		return nil
	}
	events := []Event{}
	for _, line := range bytes.Split(data[:end], []byte{'\n'}) {
		var event Event
		if json.Unmarshal(line, &event) == nil && event.Task != "" && event.Agent != "" {
			events = append(events, event)
		}
	}
	return events
}

// Ago says how long since at, in the largest whole unit.
func Ago(now, at time.Time) string {
	switch elapsed := now.Sub(at); {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm ago", int(elapsed.Minutes()))
	case elapsed < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(elapsed.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(elapsed.Hours()/24))
	}
}

// State is where a Task's activity leaves it.
type State string

const (
	// StateNone means nobody is on the Task and it was not finished here.
	StateNone     State = ""
	StateWorking  State = "working"
	StateBlocked  State = "blocked"
	StateFinished State = "finished"
)

// Holder is an agent currently on a Task, and the last thing it said.
type Holder struct {
	Agent, Branch, Message string
	At                     time.Time
	Kind                   string
}

// TaskActivity is one Task's events and what they add up to.
type TaskActivity struct {
	// Holders are the agents whose last word is not done or released, most recent first.
	Holders []Holder
	// Finished is the latest done event that nothing has happened after.
	Finished *Event
	// Latest is the most recent event, of any kind.
	Latest Event
	// Events are every event on the Task, oldest first.
	Events []Event
}

// State says what the Task's latest activity means.
func (activity *TaskActivity) State() State {
	switch {
	case activity == nil:
		return StateNone
	case len(activity.Holders) > 0 && activity.Holders[0].Kind == KindBlocked:
		return StateBlocked
	case len(activity.Holders) > 0:
		return StateWorking
	case activity.Finished != nil:
		return StateFinished
	}
	return StateNone
}

// Quiet says agents hold the Task but nobody has said anything for a while. It
// reports silence, never a failure.
func (activity *TaskActivity) Quiet(now time.Time) bool {
	return activity != nil && len(activity.Holders) > 0 && now.Sub(activity.Holders[0].At) > QuietAfter
}

// Agents names who holds the Task, most recent first.
func (activity *TaskActivity) Agents() []string {
	if activity == nil {
		return nil
	}
	agents := make([]string, 0, len(activity.Holders))
	for _, holder := range activity.Holders {
		agents = append(agents, holder.Agent)
	}
	return agents
}

// Summarize groups events, oldest first, by Task.
func Summarize(events []Event) map[string]*TaskActivity {
	tasks := map[string]*TaskActivity{}
	type lastWord = map[string]Event
	words := map[string]lastWord{}
	for _, event := range events {
		activity := tasks[event.Task]
		if activity == nil {
			activity = &TaskActivity{}
			tasks[event.Task] = activity
			words[event.Task] = lastWord{}
		}
		activity.Events = append(activity.Events, event)
		activity.Latest = event
		words[event.Task][event.Agent] = event
		switch event.Kind {
		case KindDone:
			done := event
			activity.Finished = &done
		case KindReleased:
			activity.Finished = nil
		}
	}
	for id, activity := range tasks {
		for agent, word := range words[id] {
			if word.Kind != KindDone && word.Kind != KindReleased {
				activity.Holders = append(activity.Holders, Holder{Agent: agent, Branch: word.Branch, Message: word.Message, At: word.At, Kind: word.Kind})
			}
		}
		sort.Slice(activity.Holders, func(i, j int) bool {
			if !activity.Holders[i].At.Equal(activity.Holders[j].At) {
				return activity.Holders[i].At.After(activity.Holders[j].At)
			}
			return activity.Holders[i].Agent < activity.Holders[j].Agent
		})
		// A done with work after it no longer describes the Task; one from another
		// agent while someone still works reads as that work.
		if activity.Finished != nil && len(activity.Holders) > 0 && activity.Holders[0].At.After(activity.Finished.At) {
			activity.Finished = nil
		}
		// An agent that finished ends the Task even when another's start is older.
		if activity.Finished != nil {
			kept := activity.Holders[:0]
			for _, holder := range activity.Holders {
				if holder.At.After(activity.Finished.At) {
					kept = append(kept, holder)
				}
			}
			activity.Holders = kept
		}
	}
	return tasks
}
