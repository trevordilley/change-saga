package changeview

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/twentyideas/changesaga/internal/saga"
)

// History is one record's past, read from Git's log of its record files: when
// it was introduced and what it replaced, and every commit that changed it,
// each with the comparison that opens it.
type History struct {
	NodeRef
	Files      []string       `json:"files"`
	Introduced *HistoryEvent  `json:"introduced,omitempty"`
	Replaced   []string       `json:"replaced"`
	Events     []HistoryEvent `json:"events"`
	// Uncommitted is set when the record's current files differ from the
	// last commit that holds them, or were never committed.
	Uncommitted bool `json:"uncommitted"`
}

// HistoryEvent is one commit that touched the record, with the comparison
// in which it happened: the landed change's base from its ___merges record,
// or else the commit's first parent. In a companion Saga the comparison is
// between the code commits the Saga's sync cursor named before and after.
type HistoryEvent struct {
	Reason
	Against string   `json:"against,omitempty"`
	Head    string   `json:"head"`
	Open    []string `json:"open"`
}

// NodeHistory reads the history of the record urn in the Saga at root.
func NodeHistory(ctx context.Context, root, urn string) (History, error) {
	document, validation, err := saga.Load(root)
	if err != nil {
		return History{}, err
	}
	if !validation.Valid {
		return History{}, fmt.Errorf("the Saga is invalid; run change-saga validate")
	}
	current, err := loadSnapshot(ctx, root, document)
	if err != nil {
		return History{}, err
	}
	node := current.inventory.Resolve(urn)
	if node == nil {
		return History{}, fmt.Errorf("no record %s in the Saga", urn)
	}
	history := History{NodeRef: refOf(node), Files: node.Files, Replaced: []string{}, Events: []HistoryEvent{}}
	location, err := Locate(ctx, root)
	if err != nil {
		history.Uncommitted = true
		return history, nil
	}
	paths := make([]string, 0, len(node.Files))
	for _, file := range node.Files {
		paths = append(paths, path.Join(location.Path, file))
	}
	commits, err := readCommits(ctx, location.Repo, "", "HEAD", paths...)
	if err != nil {
		return History{}, err
	}
	status, _ := exec.CommandContext(ctx, "git", append([]string{"-C", location.Repo, "status", "--porcelain", "--"}, paths...)...).Output()
	history.Uncommitted = len(commits) == 0 || len(strings.TrimSpace(string(status))) > 0
	merges := map[string]saga.Merge{}
	for _, merge := range document.Merges {
		merges[merge.Commit] = merge
	}
	for index := len(commits) - 1; index >= 0; index-- {
		commit := commits[index]
		event := HistoryEvent{Reason: commit.Reason, Head: commit.Commit}
		event.Touched = []string{TouchedRecord}
		if merge, ok := merges[commit.Commit]; ok {
			event.Against, event.Collapsed = merge.Base, merge.Commits
		} else if parent, err := revParse(ctx, location.Repo, commit.Commit+"^1"); err == nil {
			event.Against = parent
		}
		event.Against, event.Head = codeComparison(ctx, location, event.Against, event.Head)
		event.Open = []string{"change-saga", "open"}
		if event.Against != "" {
			event.Open = append(event.Open, "--against", event.Against)
		}
		event.Open = append(event.Open, "--head", event.Head, root)
		history.Events = append(history.Events, event)
	}
	if len(history.Events) > 0 {
		introduced := history.Events[len(history.Events)-1]
		history.Introduced = &introduced
		history.Replaced = replacedAt(ctx, location, commits[0].Commit, node.URN)
	}
	return history, nil
}

// codeComparison maps a Saga comparison to the code comparison that opens
// it. In the code repository they are the same commits; a companion Saga's
// commits name code commits through their sync cursors.
func codeComparison(ctx context.Context, location Location, against, head string) (string, string) {
	cursorAt := func(commit string) string {
		if commit == "" {
			return ""
		}
		data, err := exec.CommandContext(ctx, "git", "-C", location.Repo, "show", commit+":"+path.Join(location.Path, saga.CursorName)).Output()
		if err != nil {
			return ""
		}
		var cursor saga.Cursor
		if json.Unmarshal(data, &cursor) != nil || saga.ValidateCursor(cursor) != nil {
			return ""
		}
		return cursor.Commit
	}
	if headCursor := cursorAt(head); headCursor != "" {
		return cursorAt(against), headCursor
	}
	return against, head
}

// replacedAt pairs the Saga just before the introducing commit with the Saga
// at it and returns what the record replaced there.
func replacedAt(ctx context.Context, location Location, introduced, urn string) []string {
	replaced := []string{}
	parent, err := revParse(ctx, location.Repo, introduced+"^1")
	if err != nil {
		return replaced
	}
	temp, err := os.MkdirTemp("", "change-saga-history-")
	if err != nil {
		return replaced
	}
	defer os.RemoveAll(temp)
	load := func(commit, dir string) *Inventory {
		root, err := extract(ctx, location, commit, temp+"/"+dir)
		if err != nil {
			return &Inventory{Nodes: map[string]*Node{}}
		}
		at, err := loadSnapshot(ctx, root, nil)
		if err != nil {
			return &Inventory{Nodes: map[string]*Node{}}
		}
		return at.inventory
	}
	before, after := load(parent, "before"), load(introduced, "after")
	changes := diff(before, after)
	pair(changes, before, after)
	for _, change := range changes {
		if change.URN != urn || change.Pair == nil {
			continue
		}
		if change.Pair.With != "" {
			replaced = append(replaced, change.Pair.With)
		} else {
			replaced = append(replaced, change.Pair.Candidates...)
		}
	}
	return replaced
}
