package cli

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

func featureSetPlan(args []string, out io.Writer) error {
	const name = "feature set-plan"
	flags := commandFlags(name, commandUsage[name], out)
	selector := flags.String("feature", "", "feature ID or URN")
	status := flags.String("status", "", "planned or built; omitted preserves it")
	milestone := flags.String("milestone", "", "version or milestone label; empty clears it")
	priority := flags.Int("priority", 0, "lower positive numbers come first; zero clears the rank")
	jsonOutput := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *selector); err != nil {
		return err
	}
	given := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if !given["status"] && !given["milestone"] && !given["priority"] {
		return fmt.Errorf("provide --status, --milestone, or --priority")
	}
	root := flags.Arg(0)
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	var feature applayout.Feature
	err = store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		var err error
		feature, err = applayout.Require(root, manifest.ID, *selector)
		if err != nil {
			return err
		}
		plan := feature.Plan
		plan.Schema = applayout.FeaturePlanSchemaURL
		plan.Version = 5
		if given["status"] {
			plan.Status = *status
		}
		if given["milestone"] {
			plan.Milestone = strings.TrimSpace(*milestone)
		}
		if given["priority"] {
			plan.Priority = *priority
		}
		if err := plan.Validate(); err != nil {
			return err
		}
		return store.WriteJSON(filepath.Join(feature.Dir, applayout.FeaturePlanName), plan, false)
	})
	if err != nil {
		return err
	}
	urn := applayout.FeatureURN(manifest.ID, feature.ID)
	return writeLivingMutation(out, name, urn, feature.Rel+"/"+applayout.FeaturePlanName, []string{urn}, nil, false, *jsonOutput)
}

// Roadmap discovery returns ordinary feature identities in product priority order.
func queryRoadmap(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query roadmap", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("saga", "", "Saga root")
	cursor := flags.String("cursor", "", "snapshot-bound continuation")
	limit := flags.Int("limit", 50, "page size, 1 to 100")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return writeQuerySuccess(out, "", queryHelpFor("roadmap"), nil)
		}
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: err.Error()})
	}
	if *root == "" || flags.NArg() != 0 || *limit < 1 || *limit > 100 || len(*cursor) > 4096 {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: queryUsage["roadmap"]})
	}
	document, validation, err := saga.LoadOutline(*root)
	if err != nil {
		return writeQueryFailure(out, normalizeQueryError(err))
	}
	if !validation.Valid {
		return writeQueryFailure(out, &queryError{Code: "invalid_saga", Message: "Saga validation failed", Details: validation})
	}
	type row struct {
		ID     string                `json:"id"`
		Target string                `json:"target"`
		Title  string                `json:"title"`
		Plan   applayout.FeaturePlan `json:"plan"`
		Themes []string              `json:"themes"`
	}
	rows := []row{}
	for _, f := range document.Features {
		if !f.Plan.Planned() {
			continue
		}
		r := row{ID: f.ID, Target: f.Target, Title: f.Title, Plan: f.Plan, Themes: []string{}}
		for _, t := range document.FeatureThemes {
			for _, ref := range t.Features {
				if ref == f.Target {
					r.Themes = append(r.Themes, t.Target)
				}
			}
		}
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool { return applayout.PriorityBefore(rows[i].Plan.Priority, rows[j].Plan.Priority) })
	encoded, _ := json.Marshal(rows)
	snapshot := fmt.Sprintf("sha256:%x", sha256.Sum256(encoded))
	key := "roadmap:" + document.Manifest.ID
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
	return writeQuerySuccess(out, snapshot, map[string]any{"features": rows[offset:end]}, &page)
}
