package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

// A delivery lands a Roadmap feature's planned work in the existing feature it
// augments. Every record it holds moves to the same place beneath the
// recipient, so no URN changes and every relation, annotation, and slide link
// still resolves. The Roadmap feature itself (identity, plan, and overview
// essay) is then deleted; Git keeps it. The recipient's overview is marked as
// needing revision until feature set-overview rewrites it.

// deliveryContainers are the feature-relative directories whose entries are
// whole records that keep their identity wherever their feature is.
var deliveryContainers = []string{
	applayout.RequirementsDir + "/stories",
	applayout.RequirementsDir + "/citations",
	applayout.RequirementsDir + "/relations",
	applayout.RequirementsDir + "/" + requirements.RepinsDir,
	applayout.RequirementsDir + "/coverage-exceptions",
	applayout.RequirementsDir + "/prototypes",
	applayout.RequirementsDir + "/prototypes/annotations",
	quality.RootDir + "/test-cases",
	quality.RootDir + "/policies",
	applayout.SlidesDir,
}

// deliveryDropped are the Roadmap feature's own files, deleted with it.
var deliveryDropped = map[string]bool{applayout.FeatureManifestName: true, applayout.FeaturePlanName: true, applayout.FeatureOverview: true}

type deliveryMove struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type deliveredStory struct {
	Story     string   `json:"story"`
	Title     string   `json:"title"`
	Statement string   `json:"statement"`
	Overview  string   `json:"overview,omitempty"`
	Criteria  []string `json:"criteria"`
}

// deliveryOutput is what a delivery reports: what moved, what was removed,
// what it could not place, and what the recipient's revised overview must
// describe.
type deliveryOutput struct {
	OK        bool                          `json:"ok"`
	Operation string                        `json:"operation"`
	Resource  string                        `json:"resource"`
	Into      string                        `json:"into"`
	DryRun    bool                          `json:"dry_run"`
	Records   []string                      `json:"records"`
	Moved     []deliveryMove                `json:"moved"`
	Removed   []string                      `json:"removed"`
	Changed   []string                      `json:"changed"`
	Unplaced  []string                      `json:"unplaced"`
	Adjust    []livingapp.DanglingReference `json:"adjust"`
	Overview  string                        `json:"overview"`
	Stories   []deliveredStory              `json:"stories"`
	Next      string                        `json:"next"`
}

func featureDeliver(ctx context.Context, args []string, out io.Writer) error {
	const name = "feature deliver"
	flags := commandFlags(name, commandUsage[name], out)
	selector := flags.String("feature", "", "planned (Roadmap) feature ID or URN to deliver")
	into := flags.String("into", "", "existing feature ID or URN that receives it; defaults to the feature's augments")
	dryRun := flags.Bool("dry-run", false, "report what would move and be removed without writing")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *selector); err != nil {
		return err
	}
	root := flags.Arg(0)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	result := deliveryOutput{OK: true, Operation: name, DryRun: *dryRun, Records: []string{}, Moved: []deliveryMove{}, Removed: []string{}, Changed: []string{},
		Unplaced: []string{}, Adjust: []livingapp.DanglingReference{}, Stories: []deliveredStory{}}
	err = store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		before, sagaID, err := fullValidation(root)
		if err != nil {
			return err
		}
		source, err := applayout.Require(absRoot, sagaID, *selector)
		if err != nil {
			return err
		}
		if !source.Plan.Planned() {
			return fmt.Errorf("feature %q is built; only a planned (Roadmap) feature is delivered. Move single stories with change-saga story move", source.ID)
		}
		recipientSelector := strings.TrimSpace(*into)
		if recipientSelector == "" {
			recipientSelector = source.Plan.Augments
		}
		if recipientSelector == "" {
			return fmt.Errorf("feature %q augments no feature; name the recipient with --into, or record it with change-saga feature set-plan --feature %s --augments FEATURE", source.ID, source.ID)
		}
		recipient, err := applayout.Require(absRoot, sagaID, recipientSelector)
		if err != nil {
			return fmt.Errorf("--into: %w", err)
		}
		if recipient.ID == source.ID {
			return fmt.Errorf("a feature cannot be delivered into itself")
		}
		sourceURN := applayout.FeatureURN(sagaID, source.ID)
		result.Resource, result.Into = sourceURN, applayout.FeatureURN(sagaID, recipient.ID)
		if err := describeDelivery(absRoot, sagaID, source.ID, &result); err != nil {
			return err
		}
		moves, unplaced, err := planDelivery(source.Dir, recipient.Dir)
		if err != nil {
			return err
		}
		for _, path := range unplaced {
			result.Unplaced = append(result.Unplaced, filepath.ToSlash(filepath.Join(source.Rel, path)))
		}
		if len(result.Unplaced) > 0 && !*dryRun {
			return fmt.Errorf("feature deliver cannot place these, and deleting %s would lose them; move or remove them first, then deliver again:\n  %s", source.Rel, strings.Join(result.Unplaced, "\n  "))
		}

		// Themes are read while the feature they list still exists.
		themes, err := applayout.FeatureThemes(absRoot, sagaID)
		if err != nil {
			return err
		}
		edit := &contentEdit{root: absRoot, dryRun: *dryRun}
		apply := func() error {
			for _, move := range moves {
				if err := edit.move(move[0], move[1]); err != nil {
					return err
				}
				result.Moved = append(result.Moved, deliveryMove{From: edit.relative(move[0]), To: edit.relative(move[1])})
			}
			if err := edit.remove(source.Dir); err != nil {
				return err
			}
			result.Removed = append(result.Removed, source.Rel)
			plan := recipient.Plan
			plan.Schema, plan.Version = applayout.FeaturePlanSchemaURL, 5
			if plan.Status == "" {
				plan.Status = "built"
			}
			pending := applayout.OverviewRevision{Feature: sourceURN, Stories: []string{}}
			for _, story := range result.Stories {
				pending.Stories = append(pending.Stories, story.Story)
			}
			kept := []applayout.OverviewRevision{}
			for _, existing := range plan.OverviewRevisionPending {
				if existing.Feature != sourceURN {
					kept = append(kept, existing)
				}
			}
			plan.OverviewRevisionPending = append(kept, pending)
			path := filepath.Join(recipient.Dir, applayout.FeaturePlanName)
			if err := edit.writeJSON(path, plan); err != nil {
				return err
			}
			result.Changed = append(result.Changed, edit.relative(path))
			for _, theme := range themes {
				members := make([]string, 0, len(theme.Features))
				for _, member := range theme.Features {
					if member != sourceURN {
						members = append(members, member)
					}
				}
				if len(members) == len(theme.Features) {
					continue
				}
				theme.Features = members
				path := filepath.Join(abs(absRoot, applayout.FeatureThemeRel(theme.ID)), "theme.json")
				if err := edit.writeJSON(path, theme); err != nil {
					return err
				}
				result.Changed = append(result.Changed, edit.relative(path))
			}
			return nil
		}
		if err := apply(); err != nil {
			_ = edit.rollback()
			return err
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
		edit.discard()

		inputs, err := livingapp.LoadStatusInputs(ctx, livingapp.StatusOptions{SagaRoot: root})
		if err != nil {
			return err
		}
		var removed []string
		if *dryRun {
			removed = []string{sourceURN}
		}
		for _, value := range livingapp.DanglingReferences(inputs, removed...) {
			for _, missing := range value.Missing {
				if missing == sourceURN || strings.HasPrefix(missing, sourceURN+":") {
					result.Adjust = append(result.Adjust, value)
					break
				}
			}
		}
		result.Next = "change-saga feature set-overview --feature " + recipient.ID + " --source FILE " + root
		return nil
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(out, result)
	}
	writeDelivery(out, result)
	return nil
}

// describeDelivery lists the URNs of the records the Roadmap feature holds,
// its overview essay, and its stories, which the recipient's revised
// overview must describe.
func describeDelivery(root, sagaID, featureID string, result *deliveryOutput) error {
	document, err := requirements.Load(root, sagaID)
	if err != nil {
		return err
	}
	for _, story := range document.Stories {
		if story.Feature != featureID {
			continue
		}
		urn, _ := livingid.Story(sagaID, story.Identity.ID)
		result.Records = append(result.Records, urn)
		value := deliveredStory{Story: urn, Criteria: []string{}}
		if revision := story.CurrentRevision; revision != nil {
			value.Title, value.Statement, value.Overview = revision.Title, revision.Statement, revision.Overview
			for _, criterion := range revision.AcceptanceCriteria {
				value.Criteria = append(value.Criteria, criterion.Statement)
			}
		}
		result.Stories = append(result.Stories, value)
	}
	for _, citation := range document.Citations {
		if citation.Feature == featureID {
			urn, _ := livingid.Citation(sagaID, citation.ID)
			result.Records = append(result.Records, urn)
		}
	}
	for _, relation := range document.Relations {
		if relation.Feature == featureID {
			result.Records = append(result.Records, "urn:change-saga:"+sagaID+":relation:"+relation.ID)
		}
	}
	prototypeDocument, err := prototypes.Load(root, sagaID)
	if err != nil {
		return err
	}
	for _, prototype := range prototypeDocument.Prototypes {
		if prototype.Feature == featureID {
			urn, _ := prototypes.PrototypeURN(sagaID, prototype.Identity.ID)
			result.Records = append(result.Records, urn)
		}
	}
	qualityDocument, err := quality.Load(root)
	if err != nil {
		return err
	}
	for _, testCase := range qualityDocument.TestCases {
		if testCase.Feature == featureID {
			urn, _ := qualityid.TestCase(sagaID, testCase.Identity.ID)
			result.Records = append(result.Records, urn)
		}
	}
	sagaDocument, _, err := saga.Load(root)
	if err != nil {
		return err
	}
	if feature := sagaDocument.FindFeature(featureID); feature != nil {
		for _, deck := range feature.Decks {
			result.Records = append(result.Records, deck.Target)
		}
		if overview := feature.Overview(); overview != nil {
			data, err := os.ReadFile(filepath.Join(overview.Directory, filepath.FromSlash(overview.Entrypoint)))
			if err != nil {
				return err
			}
			result.Overview = string(data)
		}
	}
	sort.Strings(result.Records)
	sort.Slice(result.Stories, func(i, j int) bool { return result.Stories[i].Story < result.Stories[j].Story })
	return nil
}

// planDelivery pairs every record entry beneath source with the same place
// beneath recipient, and lists, relative to source, what holds content no
// delivery container accounts for: report chapters and fragments other than
// the overview, design, the work plan, and anything unrecognized.
func planDelivery(source, recipient string) (moves [][2]string, unplaced []string, err error) {
	container := map[string]bool{}
	for _, rel := range deliveryContainers {
		container[rel] = true
	}
	var walk func(rel string) error
	walk = func(rel string) error {
		entries, err := os.ReadDir(filepath.Join(source, filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			child := entry.Name()
			if rel != "" {
				child = rel + "/" + entry.Name()
			}
			switch {
			case rel == "" && deliveryDropped[child]:
			case container[child]:
				if err := walk(child); err != nil {
					return err
				}
			case container[rel]:
				moves = append(moves, [2]string{filepath.Join(source, filepath.FromSlash(child)), filepath.Join(recipient, filepath.FromSlash(child))})
			case entry.IsDir() && holdsContainer(child, container):
				if err := walk(child); err != nil {
					return err
				}
			default:
				has, err := holdsFiles(filepath.Join(source, filepath.FromSlash(child)))
				if err != nil {
					return err
				}
				if has {
					unplaced = append(unplaced, child)
				}
			}
		}
		return nil
	}
	if err := walk(""); err != nil {
		return nil, nil, err
	}
	sort.Strings(unplaced)
	return moves, unplaced, nil
}

func holdsContainer(rel string, container map[string]bool) bool {
	for value := range container {
		if strings.HasPrefix(value, rel+"/") {
			return true
		}
	}
	return false
}

func holdsFiles(path string) (bool, error) {
	found := false
	err := filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found, err
}

func writeDelivery(out io.Writer, result deliveryOutput) {
	if result.DryRun {
		fmt.Fprintf(out, "Dry run: would move %d records into %s and remove %s; nothing was written.\n", len(result.Moved), result.Into, result.Resource)
	} else {
		fmt.Fprintf(out, "Moved %d records into %s and removed %s.\n", len(result.Moved), result.Into, result.Resource)
	}
	verb := map[bool]string{true: "Would move", false: "Moved"}[result.DryRun]
	if len(result.Records) > 0 {
		fmt.Fprintf(out, "%s, each keeping its URN:\n", verb)
		for _, urn := range result.Records {
			fmt.Fprintf(out, "  %s\n", urn)
		}
	}
	verb = map[bool]string{true: "Would remove", false: "Removed"}[result.DryRun]
	for _, path := range result.Removed {
		fmt.Fprintf(out, "%s %s with its plan and overview essay (Git keeps them).\n", verb, path)
	}
	for _, path := range result.Changed {
		fmt.Fprintf(out, "  changed %s\n", path)
	}
	if len(result.Unplaced) > 0 {
		fmt.Fprintf(out, "Cannot place these; deliver refuses until they are moved or removed:\n")
		for _, path := range result.Unplaced {
			fmt.Fprintf(out, "  %s\n", path)
		}
	}
	if len(result.Adjust) > 0 {
		fmt.Fprintf(out, "These pointed at the Roadmap feature and need adjusting (status and check --covers health report them until they are):\n")
		for _, value := range result.Adjust {
			fmt.Fprintf(out, "  %s %s\n    %s\n", value.Kind, value.Record, value.Fix)
		}
	}
	fmt.Fprintf(out, "\nRoadmap overview of %s:\n", result.Resource)
	if strings.TrimSpace(result.Overview) == "" {
		fmt.Fprintln(out, "  (none)")
	} else {
		for _, line := range strings.Split(strings.TrimRight(result.Overview, "\n"), "\n") {
			fmt.Fprintf(out, "  %s\n", line)
		}
	}
	fmt.Fprintf(out, "\nDelivered stories:\n")
	if len(result.Stories) == 0 {
		fmt.Fprintln(out, "  (none)")
	}
	for _, story := range result.Stories {
		fmt.Fprintf(out, "  %s %q\n    %s\n", story.Story, story.Title, story.Statement)
		for _, criterion := range story.Criteria {
			fmt.Fprintf(out, "    - %s\n", criterion)
		}
	}
	fmt.Fprintf(out, "\nNext: revise the overview of %s to describe these stories; status and check --covers health report it until you do:\n  %s\n", result.Into, result.Next)
}
