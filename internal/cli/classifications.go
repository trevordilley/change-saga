package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

func Classification(_ context.Context, kind string, args []string, out io.Writer) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		return livingFamilyHelp(kind, []string{"add", "rename", "remove"}, out)
	}
	if args[0] == "remove" {
		return entityEdit(kind, args, out)
	}
	err := classificationMutation(kind, args, out)
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, kind+" "+args[0], err)
	}
	return err
}
func classificationMutation(kind string, args []string, out io.Writer) error {
	op := args[0]
	name := kind + " " + op
	if op != "add" && op != "rename" {
		return fmt.Errorf("usage: %s", commandUsage[kind])
	}
	flags := commandFlags(name, commandUsage[name], out)
	selector := "id"
	if op == "rename" {
		selector = kind
	}
	id := flags.String(selector, "", "stable "+kind+" ID or URN")
	title := flags.String("title", "", "shared display name")
	jsonOutput := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(normalizeLivingArgs(args[1:])); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *id, *title); err != nil {
		return err
	}
	root := flags.Arg(0)
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	resolvedID, ok := applayout.ClassificationID(manifest.ID, kind, *id)
	if !ok {
		return fmt.Errorf("invalid %s ID or cross-Saga URN", kind)
	}
	value := applayout.Classification{Schema: applayout.ClassificationSchema(kind), Version: 5, ID: resolvedID, Title: strings.TrimSpace(*title)}
	if err := applayout.ValidateClassification(value, kind); err != nil {
		return err
	}
	var replayed bool
	err = store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		catalog, err := applayout.ReadClassifications(root)
		if err != nil {
			return err
		}
		exists := false
		for _, record := range catalog.Records(kind) {
			if record.ID == value.ID {
				exists = true
				if record.Title == value.Title {
					replayed = true
					return nil
				}
				if op == "add" {
					return fmt.Errorf("%s %q already exists; use %s rename", kind, value.ID, kind)
				}
			} else if strings.EqualFold(record.Title, value.Title) {
				return fmt.Errorf("%s title %q already exists as %q; reference that record", kind, value.Title, record.ID)
			}
		}
		if op == "rename" && !exists {
			return fmt.Errorf("%s %q does not exist", kind, value.ID)
		}
		if !exists && len(catalog.Records(kind)) >= applayout.MaxFeatures {
			return fmt.Errorf("too many %s records", kind)
		}
		dir, err := store.EnsureDirWithin(root, filepath.Join(root, applayout.ClassificationDir(kind)))
		if err != nil {
			return err
		}
		return store.WriteJSON(filepath.Join(dir, value.ID+".json"), value, !exists)
	})
	if err != nil {
		return err
	}
	target := applayout.ClassificationURN(manifest.ID, kind, value.ID)
	return writeLivingMutation(out, name, target, applayout.ClassificationDir(kind)+"/"+value.ID+".json", []string{target}, nil, replayed, *jsonOutput)
}
func resolvePlanAssignments(root, sagaID string, plan *applayout.FeaturePlan, milestone, labels bool) error {
	catalog, err := applayout.ReadClassifications(root)
	if err != nil {
		return err
	}
	if milestone && plan.Milestone != "" {
		record, err := catalog.Resolve(sagaID, "milestone", plan.Milestone)
		if err != nil {
			return err
		}
		plan.Milestone = applayout.ClassificationURN(sagaID, "milestone", record.ID)
	}
	if labels {
		for i, selector := range plan.Labels {
			record, err := catalog.Resolve(sagaID, "label", selector)
			if err != nil {
				return err
			}
			plan.Labels[i] = applayout.ClassificationURN(sagaID, "label", record.ID)
		}
		slices.Sort(plan.Labels)
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	return catalog.ValidatePlan(sagaID, *plan)
}
func queryClassifications(kind string, args []string, out io.Writer) error {
	operation := kind + "s"
	flags := flag.NewFlagSet("query "+operation, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("saga", "", "Saga root")
	cursor := flags.String("cursor", "", "continuation")
	limit := flags.Int("limit", 50, "page size")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return writeQuerySuccess(out, "", queryHelpFor(operation), nil)
		}
		return writeQueryFailure(out, normalizeQueryError(err))
	}
	if *root == "" || flags.NArg() != 0 || *limit < 1 || *limit > 100 || len(*cursor) > 4096 {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: queryUsage[operation]})
	}
	manifest, err := saga.ReadManifest(*root)
	if err != nil {
		return writeQueryFailure(out, normalizeQueryError(err))
	}
	catalog, err := applayout.ReadClassifications(*root)
	if err != nil {
		return writeQueryFailure(out, normalizeQueryError(err))
	}
	type row struct {
		applayout.Classification
		Target string `json:"target"`
	}
	rows := []row{}
	for _, record := range catalog.Records(kind) {
		rows = append(rows, row{record, applayout.ClassificationURN(manifest.ID, kind, record.ID)})
	}
	data, _ := json.Marshal(rows)
	snapshot := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	key := operation + ":" + manifest.ID
	offset, cursorErr := decodeTermsCursor(*cursor, key, snapshot, len(rows))
	if cursorErr != nil {
		return writeQueryFailure(out, cursorErr)
	}
	end := min(offset+*limit, len(rows))
	page := queryPageEnvelope{Total: len(rows), Returned: end - offset, HasMore: end < len(rows)}
	if page.HasMore {
		next := encodeTermsCursor(key, snapshot, end)
		page.NextCursor = &next
	}
	return writeQuerySuccess(out, snapshot, map[string]any{operation: rows[offset:end]}, &page)
}
