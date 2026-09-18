package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/workplan"
)

// relationHeads gathers every current head a relation can be pinned to:
// requirements heads come from the document itself, and work-item, design,
// and (on a v5 Saga) test-case heads are loaded from their owning domains.
type relationHeads struct {
	document  requirements.Document
	inputs    requirements.StaleInputs
	testCases map[string]quality.TestCase
}

func loadRelationHeads(root string) (relationHeads, error) {
	document, err := requirements.Load(root, "")
	if err != nil {
		return relationHeads{}, err
	}
	inputs := requirements.StaleInputs{CurrentRevisions: map[string]string{}, CurrentContentDigests: map[string]string{}, Missing: map[string]bool{}}
	plan, validation, err := workplan.Load(root)
	if err != nil {
		return relationHeads{}, fmt.Errorf("load work plan: %w", err)
	}
	if !validation.Valid {
		return relationHeads{}, fmt.Errorf("the work plan is invalid; run change-saga validate")
	}
	for id, item := range plan.WorkItems {
		urn, err := livingid.WorkItem(document.SagaID, id)
		if err != nil {
			continue
		}
		if len(item.Heads) == 1 {
			inputs.CurrentRevisions[urn] = item.Heads[0]
		} else if len(item.Heads) > 1 {
			if inputs.ConflictedRevisions == nil {
				inputs.ConflictedRevisions = map[string][]string{}
			}
			inputs.ConflictedRevisions[urn] = item.Heads
		}
	}
	report, _, err := saga.Load(root)
	if err != nil {
		return relationHeads{}, fmt.Errorf("load saga: %w", err)
	}
	digests, err := saga.CurrentDesignContentDigests(report)
	if err != nil {
		return relationHeads{}, fmt.Errorf("index design content: %w", err)
	}
	for urn, digest := range digests {
		inputs.CurrentContentDigests[urn] = digest
	}
	heads := relationHeads{document: document, testCases: map[string]quality.TestCase{}}
	qualityDocument, err := quality.Load(root)
	if err != nil {
		return relationHeads{}, fmt.Errorf("load quality: %w", err)
	}
	testCaseHeads := map[string][]string{}
	for _, testCase := range qualityDocument.TestCases {
		heads.testCases[testCase.Identity.ID] = testCase
		testCaseHeads[testCase.Identity.ID] = testCase.RevisionHeads
	}
	inputs.SetTestCaseHeads(document.SagaID, testCaseHeads)
	heads.inputs = inputs
	return heads, nil
}

// relationCurrencies reports the currency of every relation in the Saga at
// root. It is the one entry point commands use; the pin comparison itself is
// requirements.EvaluateRelations.
func relationCurrencies(root string) ([]requirements.RelationCurrency, error) {
	heads, err := loadRelationHeads(root)
	if err != nil {
		return nil, err
	}
	return requirements.EvaluateRelations(heads.document, heads.inputs), nil
}

// currentRevisionHead returns the unique current revision of a revision-pinned
// endpoint, or explains why a pin cannot be defaulted.
func (heads relationHeads) currentRevisionHead(endpoint string) (string, bool, error) {
	if ref, err := qualityid.Parse(endpoint); err == nil && ref.Kind == qualityid.KindTestCase {
		testCase, ok := heads.testCases[ref.ID]
		if !ok {
			return "", true, fmt.Errorf("test case %q does not exist", endpoint)
		}
		return uniqueRevisionHead(endpoint, testCase.RevisionHeads)
	}
	ref, err := livingid.Parse(endpoint)
	if err != nil {
		return "", false, nil
	}
	switch ref.Kind {
	case livingid.KindStory, livingid.KindCriterion:
		storyID := ref.ID
		if ref.Kind == livingid.KindCriterion {
			storyID = ref.ParentID
		}
		for _, story := range heads.document.Stories {
			if story.Identity.ID == storyID {
				return uniqueRevisionHead(endpoint, story.RevisionHeads)
			}
		}
		return "", true, fmt.Errorf("story for %q does not exist", endpoint)
	case livingid.KindWorkItem:
		if head, ok := heads.inputs.CurrentRevisions[endpoint]; ok {
			return head, true, nil
		}
		if conflicting, ok := heads.inputs.ConflictedRevisions[endpoint]; ok {
			return uniqueRevisionHead(endpoint, conflicting)
		}
		return "", true, fmt.Errorf("work item %q does not exist", endpoint)
	}
	return "", false, nil
}

func uniqueRevisionHead(endpoint string, revisionHeads []string) (string, bool, error) {
	if len(revisionHeads) != 1 {
		return "", true, fmt.Errorf("%s has %d revision heads (%s); pass its revision pin explicitly", endpoint, len(revisionHeads), strings.Join(revisionHeads, ", "))
	}
	return revisionHeads[0], true, nil
}

// checkTestCasePin refuses a verifies link to a test case or test revision the
// quality domain does not know, so a new relation is never born invalid.
func (heads relationHeads) checkTestCasePin(endpoint, revision string) error {
	ref, err := qualityid.Parse(endpoint)
	if err != nil || ref.Kind != qualityid.KindTestCase {
		return nil
	}
	testCase, ok := heads.testCases[ref.ID]
	if !ok {
		return fmt.Errorf("test case %q does not exist", endpoint)
	}
	if revision == "" {
		return nil
	}
	for _, known := range testCase.Revisions {
		if urn, _ := qualityid.Revision(heads.document.SagaID, ref.ID, known.ID); urn == revision {
			return nil
		}
	}
	return fmt.Errorf("test-case revision %q does not exist", revision)
}

type relationStatusOutput struct {
	OK        bool                            `json:"ok"`
	Operation string                          `json:"operation"`
	Relations []requirements.RelationCurrency `json:"relations"`
}

func relationStatus(_ context.Context, args []string, out io.Writer) error {
	name := "relation status"
	usage := commandUsage[name]
	flags := commandFlags(name, usage, out)
	relation := flags.String("relation", "", "optional canonical relation URN")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags); err != nil {
		return err
	}
	currencies, err := relationCurrencies(flags.Arg(0))
	if err != nil {
		return err
	}
	if *relation != "" {
		selected := []requirements.RelationCurrency{}
		for _, currency := range currencies {
			if currency.Relation == *relation {
				selected = append(selected, currency)
			}
		}
		if len(selected) == 0 {
			return fmt.Errorf("relation %q does not exist", *relation)
		}
		currencies = selected
	}
	if *jsonOutput {
		return writeJSON(out, relationStatusOutput{OK: true, Operation: name, Relations: currencies})
	}
	for _, currency := range currencies {
		fmt.Fprintf(out, "%s %s\n", currency.Relation, currency.Status)
		for _, reason := range currency.Reasons {
			detail := ""
			if reason.Pinned != "" {
				detail = " (pinned " + reason.Pinned
				if len(reason.Current) > 0 {
					detail += ", current " + strings.Join(reason.Current, ", ")
				}
				detail += ")"
			}
			fmt.Fprintf(out, "  %s: %s%s\n", reason.Code, reason.Message, detail)
		}
	}
	return nil
}
