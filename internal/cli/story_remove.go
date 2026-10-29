package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// storyRemoveOutput is what story remove reports: what it deleted, and what
// still points at the story and now needs its author's attention.
type storyRemoveOutput struct {
	OK        bool                          `json:"ok"`
	Operation string                        `json:"operation"`
	Story     string                        `json:"story"`
	DryRun    bool                          `json:"dry_run"`
	Removed   []string                      `json:"removed"`
	Adjust    []livingapp.DanglingReference `json:"adjust"`
}

// storyRemove deletes a story outright. The Saga is the current state and Git
// keeps the history, so nothing is left pointing at a story that is gone:
// the records that exist only to link to it (relations and their repins,
// prototype annotations, quality policies, coverage exceptions) are deleted
// with it. Records with content of their own (slides, test cases, terms,
// flags, onboarding Items) are left as they are; the loss of what they
// covered is reported here and by status and check --covers health, with the
// command that adjusts each one.
func storyRemove(ctx context.Context, args []string, out io.Writer) error {
	name := "story remove"
	flags := commandFlags(name, commandUsage[name], out)
	storyFlag := flags.String("story", "", "story URN or ID")
	dryRun := flags.Bool("dry-run", false, "report what would be removed and adjusted without writing")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *storyFlag); err != nil {
		return err
	}
	root := flags.Arg(0)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	result := storyRemoveOutput{OK: true, Operation: name, DryRun: *dryRun, Removed: []string{}, Adjust: []livingapp.DanglingReference{}}
	err = store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		before, sagaID, err := fullValidation(root)
		if err != nil {
			return err
		}
		document, err := requirements.Load(root, sagaID)
		if err != nil {
			return err
		}
		storyID := *storyFlag
		if ref, err := livingid.Parse(storyID); err == nil {
			if ref.Kind != livingid.KindStory || ref.SagaID != sagaID {
				return fmt.Errorf("--story must be a story URN of this Saga or a story ID")
			}
			storyID = ref.ID
		}
		story := document.FindStory(storyID)
		if story == nil {
			return fmt.Errorf("story %q does not exist", *storyFlag)
		}
		storyURN, _ := livingid.Story(sagaID, storyID)
		result.Story = storyURN

		paths, err := storyRemovalPaths(absRoot, sagaID, document, *story, storyURN)
		if err != nil {
			return err
		}
		edit := &contentEdit{root: absRoot, dryRun: *dryRun}
		for _, path := range paths {
			if err := edit.remove(path); err != nil {
				_ = edit.rollback()
				return err
			}
		}
		if !*dryRun {
			after, _, err := fullValidation(root)
			if err == nil {
				err = newValidationErrors(before, after)
			}
			if err != nil {
				if rollbackErr := edit.rollback(); rollbackErr != nil {
					return fmt.Errorf("%w; restoring the Saga also failed: %v", err, rollbackErr)
				}
				return err
			}
		}
		result.Removed = append(result.Removed, edit.paths...)
		edit.discard()

		inputs, err := livingapp.LoadStatusInputs(ctx, livingapp.StatusOptions{SagaRoot: root})
		if err != nil {
			return err
		}
		removed := []string{}
		if *dryRun {
			removed = append(removed, storyURN)
		}
		for _, value := range livingapp.DanglingReferences(inputs, removed...) {
			for _, missing := range value.Missing {
				if missing == storyURN || strings.HasPrefix(missing, storyURN+":") {
					result.Adjust = append(result.Adjust, value)
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(result.Removed)
	if *jsonOutput {
		return writeJSON(out, result)
	}
	verb := "Removed"
	if result.DryRun {
		verb = "Would remove"
	}
	fmt.Fprintf(out, "%s %s and the records that only linked to it:\n", verb, result.Story)
	for _, path := range result.Removed {
		fmt.Fprintf(out, "  %s\n", path)
	}
	if len(result.Adjust) == 0 {
		fmt.Fprintln(out, "Nothing else pointed at it.")
		return nil
	}
	fmt.Fprintf(out, "These no longer cover what they did and need adjusting (status and check --covers health report them until they are):\n")
	for _, value := range result.Adjust {
		fmt.Fprintf(out, "  %s %s\n    %s\n", value.Kind, value.Record, value.Fix)
	}
	return nil
}

// storyRemovalPaths lists every file and directory that exists only for the
// story: the story itself, relations naming it or its criteria or revisions
// (in any state) with their repins, prototype annotations, quality policies,
// and coverage exceptions about its criteria.
func storyRemovalPaths(root, sagaID string, document requirements.Document, story requirements.Story, storyURN string) ([]string, error) {
	about := func(value string) bool { return value == storyURN || strings.HasPrefix(value, storyURN+":") }
	paths := []string{filepath.Join(root, filepath.FromSlash(requirements.StoryPackagePath(story.Feature, story.Identity.ID)))}
	for _, relation := range document.Relations {
		touches := relation.TouchesStory(storyURN)
		for _, repin := range relation.Repins {
			touches = touches || about(repin.FromRevision) || about(repin.ToRevision)
		}
		if !touches {
			continue
		}
		paths = append(paths, filepath.Join(root, filepath.FromSlash(requirements.RelationPath(relation.Feature, relation.ID))))
		repins := filepath.Join(root, filepath.FromSlash(requirements.RelationRepinsPath(relation.Feature, relation.ID)))
		if _, err := os.Lstat(repins); err == nil {
			paths = append(paths, repins)
		}
	}
	prototypeDocument, err := prototypes.Load(root, sagaID)
	if err != nil {
		return nil, err
	}
	for _, annotation := range prototypeDocument.Annotations {
		if about(annotation.Target) || about(annotation.StoryRevision) {
			prototypeID := annotation.Prototype[strings.LastIndex(annotation.Prototype, ":")+1:]
			paths = append(paths, filepath.Join(applayout.FeatureDir(root, annotation.Feature), applayout.RequirementsDir, "prototypes", "annotations", prototypeID+".prototype", annotation.ID+".json"))
		}
	}
	qualityDocument, err := quality.Load(root)
	if err != nil {
		return nil, err
	}
	for _, policy := range qualityDocument.Policies {
		if about(policy.Criterion) {
			paths = append(paths, filepath.Join(applayout.FeatureDir(root, policy.Feature), quality.RootDir, "policies", policy.ID+".json"))
		}
	}
	exceptions, err := filepath.Glob(filepath.Join(root, applayout.FeaturesDir, "*"+applayout.FeatureSuffix, applayout.RequirementsDir, "coverage-exceptions", "*.json"))
	if err != nil {
		return nil, err
	}
	for _, path := range exceptions {
		var record struct {
			Criterion     string `json:"criterion"`
			StoryRevision string `json:"story_revision"`
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if json.Unmarshal(data, &record) == nil && (about(record.Criterion) || about(record.StoryRevision)) {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// fullValidation runs every check validate runs and returns the Saga ID.
func fullValidation(root string) (saga.Validation, string, error) {
	document, validation, err := saga.Load(root)
	if err != nil {
		return saga.Validation{}, "", err
	}
	appendPrototypeIssues(root, document, &validation)
	appendQualityIssues(root, document, &validation)
	appendAppIssues(root, document, &validation)
	appendInventoryIssues(root, document, &validation)
	appendWhiteboardIssues(root, document, &validation)
	appendThemeIssues(root, &validation)
	return validation, document.Manifest.ID, nil
}
