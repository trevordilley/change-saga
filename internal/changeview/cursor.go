package changeview

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path"
	"strings"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/saga"
)

// CursorCommit is one Saga commit and the code commit its sync cursor
// named.
type CursorCommit struct {
	SagaCommit string
	Cursor     string
}

// CursorHistory lists, newest first, every Saga commit that recorded a sync
// cursor and the code commit it named.
func CursorHistory(ctx context.Context, location Location) ([]CursorCommit, error) {
	file := path.Join(location.Path, saga.CursorName)
	history := []CursorCommit{}
	if _, err := revParse(ctx, location.Repo, "HEAD"); err != nil {
		// A Saga repository with no commits yet has no cursor history.
		return history, nil
	}
	output, err := exec.CommandContext(ctx, "git", "-C", location.Repo, "log", "--format=%H", "--", file).Output()
	if err != nil {
		return nil, err
	}
	for _, commit := range strings.Fields(string(output)) {
		data, err := exec.CommandContext(ctx, "git", "-C", location.Repo, "show", commit+":"+file).Output()
		if err != nil {
			continue
		}
		var cursor saga.Cursor
		decoder := json.NewDecoder(bytes.NewReader(data))
		if decoder.Decode(&cursor) != nil || saga.ValidateCursor(cursor) != nil {
			continue
		}
		history = append(history, CursorCommit{SagaCommit: commit, Cursor: cursor.Commit})
	}
	return history, nil
}

type sides struct {
	base, head  string
	diagnostics []Diagnostic
}

// companionSides finds the Saga snapshots of a companion Saga: the Saga
// commit whose cursor matched the merge-base, and at head the working tree
// when its cursor names head, or else the Saga commit whose cursor did. When
// no cursor matched the merge-base exactly, the newest cursor that is an
// ancestor of it stands in, and the approximation is reported.
func companionSides(ctx context.Context, location Location, checkout string, changes gitdiff.ChangeSet) (sides, error) {
	result := sides{}
	history, err := CursorHistory(ctx, location)
	if err != nil {
		return sides{}, err
	}
	for _, entry := range history {
		if entry.Cursor == changes.BaseOID {
			result.base = entry.SagaCommit
			break
		}
	}
	if result.base == "" {
		for _, entry := range history {
			if isAncestor(ctx, checkout, entry.Cursor, changes.BaseOID) {
				result.base = entry.SagaCommit
				result.diagnostics = append(result.diagnostics, Diagnostic{Code: "saga_cursor_approximate",
					Message: "no Saga commit documents the merge-base " + short(changes.BaseOID) + "; the Saga at " + short(entry.SagaCommit) + ", which documents its ancestor " + short(entry.Cursor) + ", stands in"})
				break
			}
		}
	}
	if result.base == "" {
		result.diagnostics = append(result.diagnostics, Diagnostic{Code: "saga_cursor_missing",
			Message: "no Saga commit's sync cursor documents the merge-base " + short(changes.BaseOID) + " or an ancestor; every record is treated as added"})
	}
	working, ok, err := saga.ReadCursor(location.Repo + "/" + location.Path)
	if err != nil {
		return sides{}, err
	}
	if ok && working.Commit == changes.HeadOID {
		return result, nil
	}
	for _, entry := range history {
		if entry.Cursor == changes.HeadOID {
			result.head = entry.SagaCommit
			return result, nil
		}
	}
	documented := "no sync cursor"
	if ok {
		documented = "its sync cursor names " + short(working.Commit)
	}
	result.diagnostics = append(result.diagnostics, Diagnostic{Code: "saga_cursor_behind",
		Message: "the Saga does not document head " + short(changes.HeadOID) + " (" + documented + "); run change-saga sync after updating it"})
	return result, nil
}

// isAncestor asks the code repository, never the Saga's, about ancestry.
func isAncestor(ctx context.Context, checkout, ancestor, descendant string) bool {
	_, err := gitexec.Output(ctx, "-C", checkout, "merge-base", "--is-ancestor", ancestor, descendant)
	return err == nil
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
