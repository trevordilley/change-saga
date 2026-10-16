package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func whiteboardMetadataCommand(ctx context.Context, op string, args []string, out io.Writer) error {
	name := "whiteboard " + op
	flags := commandFlags(name, commandUsage[name], out)
	jsonOutput := flags.Bool("json", false, "machine-readable result")
	var id, role, expected, title, snapshot string
	var dryRun, move bool
	if op != "list" {
		flags.StringVar(&id, "id", "", "stable whiteboard document ID")
		flags.BoolVar(&dryRun, "dry-run", false, "validate without writing metadata")
		flags.StringVar(&snapshot, "expected-snapshot", "", "observed metadata snapshot from whiteboard list")
	}
	switch op {
	case "assign-role", "unassign-role":
		flags.StringVar(&role, "role", "", "erd, infrastructure or data-flows")
	case "rename":
		flags.StringVar(&title, "name", "", "new display title")
	}
	if op == "assign-role" {
		flags.StringVar(&expected, "expect", "", "observed role owner ID or URN, or none")
		flags.BoolVar(&move, "move", false, "move the observed previous role; requires --expected-snapshot")
	}
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || op != "list" && !livingid.ValidID(id) {
		return fmt.Errorf("requires --id with a valid whiteboard document ID and one Saga path (list requires only the path)")
	}
	root := flags.Arg(0)
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	if manifest.Version != 5 {
		return fmt.Errorf("whiteboards require a v5 Saga")
	}
	if op == "list" {
		return writeWhiteboardList(root, manifest.ID, *jsonOutput, out)
	}
	result, err := requirements.UpdateWhiteboardMetadata(ctx, root, manifest.ID, requirements.WhiteboardMetadataWrite{Operation: op, ID: id, Role: role, Expected: expected, Name: title, ExpectedSnapshot: snapshot, DryRun: dryRun, Move: move})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(out, result)
	}
	verb := "Applied"
	if dryRun {
		verb = "Would apply"
	}
	_, err = fmt.Fprintf(out, "%s whiteboard %s to %s\nSnapshot: %s\n", verb, op, result.Target, result.After.Snapshot())
	return err
}

type whiteboardListEntry struct {
	ID       string `json:"id"`
	Target   string `json:"target"`
	Name     string `json:"name"`
	Revision string `json:"revision"`
	Role     string `json:"role,omitempty"`
	Archived bool   `json:"archived"`
}

type whiteboardList struct {
	Snapshot    string                `json:"snapshot"`
	Roles       map[string]string     `json:"roles"`
	Whiteboards []whiteboardListEntry `json:"whiteboards"`
}

func loadWhiteboardList(root, sagaID string) (whiteboardList, error) {
	metadata, err := requirements.LoadWhiteboardMetadata(root, sagaID)
	if err != nil {
		return whiteboardList{}, err
	}
	ids, err := requirements.ListWhiteboardIDs(root)
	if err != nil {
		return whiteboardList{}, err
	}
	rows := []whiteboardListEntry{}
	for _, id := range ids {
		revision, err := requirements.LoadWhiteboard(root, sagaID, id, "")
		if err != nil {
			return whiteboardList{}, err
		}
		if revision == nil {
			continue
		}
		title := revision.Name
		if metadata.Boards[id].Name != "" {
			title = metadata.Boards[id].Name
		}
		target, _ := requirements.WhiteboardURN(sagaID, id)
		rows = append(rows, whiteboardListEntry{ID: id, Target: target, Name: title, Revision: revision.ID, Role: metadata.Role(id), Archived: metadata.Boards[id].Archived})
	}
	return whiteboardList{Snapshot: metadata.Snapshot(), Roles: metadata.Roles, Whiteboards: rows}, nil
}

func writeWhiteboardList(root, sagaID string, jsonOutput bool, out io.Writer) error {
	result, err := loadWhiteboardList(root, sagaID)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	for _, row := range result.Whiteboards {
		fmt.Fprintf(out, "%s: %s (revision %s; role %s; archived %t)\n", row.ID, row.Name, row.Revision, row.Role, row.Archived)
	}
	_, err = fmt.Fprintf(out, "Snapshot: %s\n", result.Snapshot)
	return err
}
