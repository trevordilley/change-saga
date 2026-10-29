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
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// A remove deletes a record outright. The Saga is the current state and Git
// keeps the history, so nothing is left pointing at a record that is gone:
// the records that exist only to link to it (relations and their repins,
// prototype annotations, quality policies, coverage exceptions) are deleted
// with it. Records with content of their own (stories, slides, test cases,
// terms, flags, onboarding Items) are left as they are; the loss of what they
// covered is reported here and by status and check --covers health, with the
// command that adjusts each one.

// removalOutput is what a remove reports: what it deleted, and what still
// points at the removed record and now needs its author's attention.
type removalOutput struct {
	OK        bool                          `json:"ok"`
	Operation string                        `json:"operation"`
	Resource  string                        `json:"resource"`
	DryRun    bool                          `json:"dry_run"`
	Removed   []string                      `json:"removed"`
	Changed   []string                      `json:"changed"`
	Adjust    []livingapp.DanglingReference `json:"adjust"`
}

// removalPlan is what one record's removal deletes. Also names further
// records removed with it, such as a feature's stories; Owned are records
// deleted along with it, which a preview must not list as left to adjust.
type removalPlan struct {
	resource string
	also     []string
	owned    map[string]bool
	paths    []string
	// rewrites are membership lists that name the record, such as a feature
	// theme's features, rewritten without it.
	rewrites []removalRewrite
}

type removalRewrite struct {
	path  string
	value any
}

// removalCollector resolves the record a remove names and lists the files and
// directories that exist only for it.
type removalCollector func(root, sagaID string, document requirements.Document) (removalPlan, error)

// runRemoval parses the flags every remove shares, then deletes what collect
// lists under the Saga lock, keeping the change only when it introduces no
// validation error, and reports what is left pointing at the record.
func runRemoval(ctx context.Context, name string, args []string, out io.Writer, recordFlag, recordHelp string, collect func(value string) removalCollector) error {
	flags := commandFlags(name, commandUsage[name], out)
	record := flags.String(recordFlag, "", recordHelp)
	dryRun := flags.Bool("dry-run", false, "report what would be removed and adjusted without writing")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *record); err != nil {
		return err
	}
	root := flags.Arg(0)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	result := removalOutput{OK: true, Operation: name, DryRun: *dryRun, Removed: []string{}, Changed: []string{}, Adjust: []livingapp.DanglingReference{}}
	err = store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		before, sagaID, err := fullValidation(root)
		if err != nil {
			return err
		}
		document, err := requirements.Load(root, sagaID)
		if err != nil {
			return err
		}
		plan, err := collect(*record)(absRoot, sagaID, document)
		if err != nil {
			return err
		}
		result.Resource = plan.resource
		edit := &contentEdit{root: absRoot, dryRun: *dryRun}
		seen := map[string]bool{}
		for _, path := range plan.paths {
			if seen[path] {
				continue
			}
			seen[path] = true
			if err := edit.remove(path); err != nil {
				_ = edit.rollback()
				return err
			}
		}
		removedCount := len(edit.paths)
		for _, rewrite := range plan.rewrites {
			if err := edit.writeJSON(rewrite.path, rewrite.value); err != nil {
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
		result.Removed = append(result.Removed, edit.paths[:removedCount]...)
		result.Changed = append(result.Changed, edit.paths[removedCount:]...)
		edit.discard()

		inputs, err := livingapp.LoadStatusInputs(ctx, livingapp.StatusOptions{SagaRoot: root})
		if err != nil {
			return err
		}
		resources := append([]string{plan.resource}, plan.also...)
		var removed []string
		if *dryRun {
			removed = resources
		}
		for _, value := range livingapp.DanglingReferences(inputs, removed...) {
			if plan.owned[value.Record] {
				continue
			}
		missing:
			for _, missing := range value.Missing {
				for _, resource := range resources {
					if missing == resource || strings.HasPrefix(missing, resource+":") {
						result.Adjust = append(result.Adjust, value)
						break missing
					}
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
	verb, summary := "Removed", "Removed %d files or directories and changed %d; %d records elsewhere now need adjusting.\n"
	if result.DryRun {
		verb, summary = "Would remove", "Dry run: would remove %d files or directories and change %d; %d records elsewhere would need adjusting. Nothing was written.\n"
	}
	fmt.Fprintf(out, summary, len(result.Removed), len(result.Changed), len(result.Adjust))
	fmt.Fprintf(out, "%s %s and the records that only linked to it:\n", verb, result.Resource)
	for _, path := range result.Removed {
		fmt.Fprintf(out, "  %s\n", path)
	}
	for _, path := range result.Changed {
		fmt.Fprintf(out, "  %s (no longer names it)\n", path)
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

// recordID accepts a canonical URN of kind in this Saga, or a bare ID.
func recordID(value, sagaID, kind string) (string, error) {
	prefix := "urn:change-saga:" + sagaID + ":" + kind + ":"
	if id, ok := strings.CutPrefix(value, prefix); ok && livingid.ValidID(id) {
		return id, nil
	}
	if strings.HasPrefix(value, "urn:") || !livingid.ValidID(value) {
		return "", fmt.Errorf("--%s must be a %s URN of this Saga or a %s ID", kind, kind, kind)
	}
	return value, nil
}

func storyRemove(ctx context.Context, args []string, out io.Writer) error {
	return runRemoval(ctx, "story remove", args, out, "story", "story URN or ID", func(value string) removalCollector {
		return func(root, sagaID string, document requirements.Document) (removalPlan, error) {
			id, err := recordID(value, sagaID, "story")
			if err != nil {
				return removalPlan{}, err
			}
			story := document.FindStory(id)
			if story == nil {
				return removalPlan{}, fmt.Errorf("story %q does not exist", value)
			}
			urn, _ := livingid.Story(sagaID, id)
			links, err := linkRecordsAbout(root, sagaID, document, urn)
			if err != nil {
				return removalPlan{}, err
			}
			return removalPlan{resource: urn, paths: append([]string{abs(root, requirements.StoryPackagePath(story.Feature, id))}, links...)}, nil
		}
	})
}

func personaRemove(ctx context.Context, args []string, out io.Writer) error {
	return runRemoval(ctx, "persona remove", args, out, "persona", "persona URN or ID", func(value string) removalCollector {
		return func(root, sagaID string, document requirements.Document) (removalPlan, error) {
			id, err := recordID(value, sagaID, "persona")
			if err != nil {
				return removalPlan{}, err
			}
			if document.FindPersona(id) == nil {
				return removalPlan{}, fmt.Errorf("persona %q does not exist", value)
			}
			urn, _ := requirements.PersonaURN(sagaID, id)
			return removalPlan{resource: urn, paths: []string{abs(root, requirements.PersonaPackagePath(id))}}, nil
		}
	})
}

func termRemove(ctx context.Context, args []string, out io.Writer) error {
	return runRemoval(ctx, "term remove", args, out, "term", "term URN or ID", func(value string) removalCollector {
		return func(root, sagaID string, document requirements.Document) (removalPlan, error) {
			id, err := recordID(value, sagaID, "term")
			if err != nil {
				return removalPlan{}, err
			}
			if document.FindTerm(id) == nil {
				return removalPlan{}, fmt.Errorf("term %q does not exist", value)
			}
			urn, _ := requirements.TermURN(sagaID, id)
			return removalPlan{resource: urn, paths: []string{abs(root, requirements.TermPackagePath(id))}}, nil
		}
	})
}

func flagRemove(ctx context.Context, args []string, out io.Writer) error {
	return runRemoval(ctx, "flag remove", args, out, "flag", "flag URN or ID", func(value string) removalCollector {
		return func(root, sagaID string, document requirements.Document) (removalPlan, error) {
			id, err := recordID(value, sagaID, "flag")
			if err != nil {
				return removalPlan{}, err
			}
			if document.FindFlag(id) == nil {
				return removalPlan{}, fmt.Errorf("flag %q does not exist", value)
			}
			urn, _ := requirements.FlagURN(sagaID, id)
			return removalPlan{resource: urn, paths: []string{abs(root, requirements.FlagPackagePath(id))}}, nil
		}
	})
}

func citationRemove(ctx context.Context, args []string, out io.Writer) error {
	return runRemoval(ctx, "citation remove", args, out, "citation", "citation URN or ID", func(value string) removalCollector {
		return func(root, sagaID string, document requirements.Document) (removalPlan, error) {
			id, err := recordID(value, sagaID, "citation")
			if err != nil {
				return removalPlan{}, err
			}
			for _, citation := range document.Citations {
				if citation.ID != id {
					continue
				}
				urn, _ := livingid.Citation(sagaID, id)
				links, err := linkRecordsAbout(root, sagaID, document, urn)
				if err != nil {
					return removalPlan{}, err
				}
				return removalPlan{resource: urn, paths: append([]string{abs(root, requirements.CitationPath(citation.Feature, id))}, links...)}, nil
			}
			return removalPlan{}, fmt.Errorf("citation %q does not exist", value)
		}
	})
}

func prototypeRemove(ctx context.Context, args []string, out io.Writer) error {
	return runRemoval(ctx, "prototype remove", args, out, "prototype", "prototype URN or ID", func(value string) removalCollector {
		return func(root, sagaID string, _ requirements.Document) (removalPlan, error) {
			id, err := recordID(value, sagaID, "prototype")
			if err != nil {
				return removalPlan{}, err
			}
			document, err := prototypes.Load(root, sagaID)
			if err != nil {
				return removalPlan{}, err
			}
			prototype := document.Find(id)
			if prototype == nil {
				return removalPlan{}, fmt.Errorf("prototype %q does not exist", value)
			}
			urn, _ := prototypes.PrototypeURN(sagaID, id)
			// Its annotations are links that exist only to pin it to stories.
			paths := []string{abs(root, prototypes.PrototypePath(prototype.Feature, id))}
			for _, annotation := range document.Annotations {
				if annotation.Prototype == urn {
					paths = append(paths, abs(root, prototypes.AnnotationDir(annotation.Feature, id)))
				}
			}
			return removalPlan{resource: urn, paths: paths}, nil
		}
	})
}

func testCaseRemove(ctx context.Context, args []string, out io.Writer) error {
	return runRemoval(ctx, "quality test-case remove", args, out, "test-case", "test-case URN or ID", func(value string) removalCollector {
		return func(root, sagaID string, document requirements.Document) (removalPlan, error) {
			id, err := recordID(value, sagaID, "test-case")
			if err != nil {
				return removalPlan{}, err
			}
			qualityDocument, err := quality.Load(root)
			if err != nil {
				return removalPlan{}, err
			}
			for _, testCase := range qualityDocument.TestCases {
				if testCase.Identity.ID != id {
					continue
				}
				urn, _ := qualityid.TestCase(sagaID, id)
				links, err := linkRecordsAbout(root, sagaID, document, urn)
				if err != nil {
					return removalPlan{}, err
				}
				return removalPlan{resource: urn, paths: append([]string{abs(root, quality.TestCasePath(testCase.Feature, id))}, links...)}, nil
			}
			return removalPlan{}, fmt.Errorf("test case %q does not exist", value)
		}
	})
}

func abs(root, rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

// linkRecordsAbout lists the records that exist only to link to urn or to
// something beneath it (a story's criteria and revisions): relations in any
// state with their repins, prototype annotations, quality policies, and
// coverage exceptions.
func linkRecordsAbout(root, sagaID string, document requirements.Document, urn string) ([]string, error) {
	about := func(value string) bool { return value == urn || strings.HasPrefix(value, urn+":") }
	var paths []string
	for _, relation := range document.Relations {
		touches := about(relation.From) || about(relation.To) || about(relation.FromRevision) || about(relation.ToRevision)
		for _, repin := range relation.Repins {
			touches = touches || about(repin.FromRevision) || about(repin.ToRevision)
		}
		if !touches {
			continue
		}
		paths = append(paths, abs(root, requirements.RelationPath(relation.Feature, relation.ID)))
		repins := abs(root, requirements.RelationRepinsPath(relation.Feature, relation.ID))
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
			paths = append(paths, filepath.Join(abs(root, prototypes.AnnotationDir(annotation.Feature, prototypeID)), annotation.ID+".json"))
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

// featureRemoveWithContent deletes a feature and everything inside it: its
// report, stories, prototypes, test cases, decks, and work plan. Relations and
// other links elsewhere that point at anything inside it are deleted, and
// feature themes stop listing it. What other features still reference is
// reported to adjust.
func featureRemoveWithContent(ctx context.Context, args []string, out io.Writer) error {
	return runRemoval(ctx, "feature remove", args, out, "feature", "feature URN or ID", func(value string) removalCollector {
		return func(root, sagaID string, document requirements.Document) (removalPlan, error) {
			id, err := recordID(value, sagaID, "feature")
			if err != nil {
				return removalPlan{}, err
			}
			if _, ok := applayout.Find(document.Features, id); !ok {
				return removalPlan{}, fmt.Errorf("feature %q does not exist", value)
			}
			featureURN := applayout.FeatureURN(sagaID, id)
			featureDir := applayout.FeatureDir(root, id)
			plan := removalPlan{resource: featureURN, owned: map[string]bool{}, paths: []string{featureDir}}
			inside := func(path string) bool { return strings.HasPrefix(path, featureDir+string(filepath.Separator)) }
			targets := []string{featureURN}
			for _, story := range document.Stories {
				if story.Feature == id {
					urn, _ := livingid.Story(sagaID, story.Identity.ID)
					targets, plan.owned[urn] = append(targets, urn), true
				}
			}
			for _, citation := range document.Citations {
				if citation.Feature == id {
					urn, _ := livingid.Citation(sagaID, citation.ID)
					targets = append(targets, urn)
				}
			}
			qualityDocument, err := quality.Load(root)
			if err != nil {
				return removalPlan{}, err
			}
			for _, testCase := range qualityDocument.TestCases {
				if testCase.Feature == id {
					urn, _ := qualityid.TestCase(sagaID, testCase.Identity.ID)
					targets, plan.owned[urn] = append(targets, urn), true
				}
			}
			prototypeDocument, err := prototypes.Load(root, sagaID)
			if err != nil {
				return removalPlan{}, err
			}
			for _, prototype := range prototypeDocument.Prototypes {
				if prototype.Feature == id {
					urn, _ := prototypes.PrototypeURN(sagaID, prototype.Identity.ID)
					targets = append(targets, urn)
					plan.paths = append(plan.paths, abs(root, prototypes.AnnotationDir(prototype.Feature, prototype.Identity.ID)))
				}
			}
			if sagaDocument, _, err := saga.Load(root); err == nil {
				if feature := sagaDocument.FindFeature(id); feature != nil {
					for _, deck := range feature.Decks {
						for _, slide := range deck.Slides {
							for _, item := range slide.Items {
								plan.owned[item.Target] = true
							}
						}
					}
				}
			}
			plan.also = targets[1:]
			for _, target := range targets {
				links, err := linkRecordsAbout(root, sagaID, document, target)
				if err != nil {
					return removalPlan{}, err
				}
				for _, link := range links {
					if !inside(link) {
						plan.paths = append(plan.paths, link)
					}
				}
			}
			kept := plan.paths[:0]
			for _, path := range plan.paths {
				if path == featureDir || !inside(path) {
					kept = append(kept, path)
				}
			}
			plan.paths = kept
			themes, err := applayout.FeatureThemes(root, sagaID)
			if err != nil {
				return removalPlan{}, err
			}
			for _, theme := range themes {
				members := make([]string, 0, len(theme.Features))
				for _, member := range theme.Features {
					if member != featureURN {
						members = append(members, member)
					}
				}
				if len(members) != len(theme.Features) {
					theme.Features = members
					plan.rewrites = append(plan.rewrites, removalRewrite{path: filepath.Join(abs(root, applayout.FeatureThemeRel(theme.ID)), "theme.json"), value: theme})
				}
			}
			return plan, nil
		}
	})
}
