package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
)

// featureArchive archives or restores a feature. Archiving keeps every record
// the feature holds and its planned or built status; it only hides the
// feature from feature lists and coverage gaps, so restore is exact.
func featureArchive(operation string, args []string, out io.Writer) error {
	name := "feature " + operation
	return contentEditCommand(name, args, out, func() error {
		flags, dryRun, jsonOutput := contentEditFlags(name, out)
		selector := flags.String("feature", "", "existing feature ID or URN")
		var reason string
		if operation == "archive" {
			flags.StringVar(&reason, "reason", "", "why the feature is archived; one line")
		}
		if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
			return err
		}
		if err := requireLivingArgs(flags, *selector); err != nil {
			return err
		}
		reason = strings.TrimSpace(reason)
		return runContentEdit(contentEditRequest{operation: name, root: flags.Arg(0), dryRun: *dryRun, json: *jsonOutput,
			apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
				feature, err := applayout.Require(document.Root, document.Manifest.ID, *selector)
				if err != nil {
					return err
				}
				result.Resource = applayout.FeatureURN(document.Manifest.ID, feature.ID)
				plan := feature.Plan
				archive := operation == "archive"
				if plan.Archived == archive && (!archive || reason == "" || plan.ArchivedReason == reason) {
					result.Replayed = true
					return nil
				}
				plan.Schema, plan.Version = applayout.FeaturePlanSchemaURL, 5
				if plan.Status == "" {
					plan.Status = "built"
				}
				plan.Archived = archive
				if archive {
					if reason != "" {
						plan.ArchivedReason = reason
					}
					result.Changed = []string{"archived"}
				} else {
					plan.ArchivedReason = ""
					result.Changed = []string{"archived"}
				}
				if err := plan.Validate(); err != nil {
					return fmt.Errorf("--reason: %w", err)
				}
				path := filepath.Join(feature.Dir, applayout.FeaturePlanName)
				result.Paths = []string{edit.relative(path)}
				return edit.writeJSON(path, plan)
			},
		}, out)
	})
}
