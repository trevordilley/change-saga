package requirements

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/store"
)

// ConsolidateInput records a deliberate decision that Duplicate expresses the
// same proposal as Canonical. CriterionMap is mandatory and exhaustive: each
// current duplicate criterion URN maps to one current canonical criterion URN.
type ConsolidateInput struct {
	Duplicate    string
	Canonical    string
	EventID      string
	Parents      []string
	Reason       string
	CriterionMap map[string]string
	CreatedAt    time.Time
	// CurrencyInputs supplies current pins owned outside requirements. A
	// remapped relation with an external revision or content-digest pin is
	// refused unless its current value is supplied and exactly matches.
	CurrencyInputs StaleInputs
	// LoadCurrencyInputs, when present, refreshes cross-domain heads at the
	// validation point. Apply invokes it while holding the Saga writer lock.
	LoadCurrencyInputs func() (StaleInputs, error)
}

type RelationRemap struct {
	Existing    string `json:"existing"`
	Replacement string `json:"replacement"`
	From        string `json:"from"`
	To          string `json:"to"`
}

type ConsolidationPlan struct {
	Duplicate       string            `json:"duplicate"`
	Canonical       string            `json:"canonical"`
	CriterionMap    map[string]string `json:"criterion_map"`
	LifecycleState  LifecycleState    `json:"lifecycle_state"`
	LifecycleEvent  string            `json:"lifecycle_event"`
	RelationRemaps  []RelationRemap   `json:"relation_remaps"`
	PreservesIntent bool              `json:"preserves_accepted_intent"`
	Applied         bool              `json:"applied"`
}

type preparedConsolidation struct {
	plan         ConsolidationPlan
	event        LifecycleEvent
	duplicate    *Story
	replacements []Relation
	superseded   []Relation
}

// PreviewConsolidation validates the complete decision and returns every
// lifecycle and link change without writing.
func PreviewConsolidation(root, sagaID string, input ConsolidateInput) (ConsolidationPlan, error) {
	document, err := Load(root, sagaID)
	if err != nil {
		return ConsolidationPlan{}, err
	}
	if input.LoadCurrencyInputs != nil {
		input.CurrencyInputs, err = input.LoadCurrencyInputs()
		if err != nil {
			return ConsolidationPlan{}, err
		}
	}
	prepared, err := prepareConsolidation(&document, input)
	if err != nil {
		return ConsolidationPlan{}, err
	}
	return prepared.plan, nil
}

// ConsolidateProposal appends the honest terminal lifecycle decision, creates
// replacement links to the canonical proposal, and supersedes only the links
// it replaced. Validation and file preparation complete before the batch is
// published under the Saga writer lock.
func ConsolidateProposal(root, sagaID string, input ConsolidateInput) (ConsolidationPlan, error) {
	var plan ConsolidationPlan
	err := mutate(root, sagaID, func(document *Document) error {
		if input.LoadCurrencyInputs != nil {
			var err error
			input.CurrencyInputs, err = input.LoadCurrencyInputs()
			if err != nil {
				return err
			}
		}
		prepared, err := prepareConsolidation(document, input)
		if err != nil {
			return err
		}
		entries := make([]store.WriteBatchEntry, 0, 1+len(prepared.replacements)*2)
		appendJSON := func(path string, value any, exclusive bool) error {
			data, err := json.MarshalIndent(value, "", "  ")
			if err != nil {
				return err
			}
			entries = append(entries, store.WriteBatchEntry{Path: path, Data: append(data, '\n'), Exclusive: exclusive})
			return nil
		}
		for index := range prepared.replacements {
			replacement := prepared.replacements[index]
			path := filepath.Join(document.Root, filepath.FromSlash(relationPath(replacement.Feature, replacement.ID)))
			if err := appendJSON(path, replacement, true); err != nil {
				return err
			}
			existing := prepared.superseded[index]
			path = filepath.Join(document.Root, filepath.FromSlash(relationPath(existing.Feature, existing.ID)))
			if err := appendJSON(path, existing, false); err != nil {
				return err
			}
		}
		eventPath := filepath.Join(document.Root, filepath.FromSlash(eventPath(prepared.duplicate.Feature, prepared.duplicate.Identity.ID, prepared.event.ID)))
		if err := appendJSON(eventPath, prepared.event, true); err != nil {
			return err
		}
		if err := store.WriteBatch(entries); err != nil {
			return fmt.Errorf("commit consolidation batch: %w", err)
		}
		plan = prepared.plan
		plan.Applied = true
		return nil
	})
	return plan, err
}

func prepareConsolidation(document *Document, input ConsolidateInput) (preparedConsolidation, error) {
	duplicateRef, err := livingid.Parse(input.Duplicate)
	if err != nil || duplicateRef.Kind != livingid.KindStory || duplicateRef.SagaID != document.SagaID {
		return preparedConsolidation{}, fmt.Errorf("duplicate must be a canonical story URN in saga %q", document.SagaID)
	}
	canonicalRef, err := livingid.Parse(input.Canonical)
	if err != nil || canonicalRef.Kind != livingid.KindStory || canonicalRef.SagaID != document.SagaID {
		return preparedConsolidation{}, fmt.Errorf("canonical must be a canonical story URN in saga %q", document.SagaID)
	}
	if duplicateRef.ID == canonicalRef.ID {
		return preparedConsolidation{}, fmt.Errorf("duplicate and canonical must be different stories")
	}
	duplicate, canonical := findStory(document, duplicateRef.ID), findStory(document, canonicalRef.ID)
	if duplicate == nil || canonical == nil {
		return preparedConsolidation{}, fmt.Errorf("duplicate and canonical stories must both exist")
	}
	if duplicate.CurrentRevision == nil || canonical.CurrentRevision == nil || duplicate.CurrentLifecycle == nil || canonical.CurrentLifecycle == nil {
		return preparedConsolidation{}, fmt.Errorf("consolidation refuses revision or lifecycle ambiguity; both stories must have one current head")
	}
	if !sameSet(input.Parents, duplicate.LifecycleHeads) {
		return preparedConsolidation{}, fmt.Errorf("lifecycle parents must name every current duplicate head (got %v, want %v)", input.Parents, duplicate.LifecycleHeads)
	}
	if strings.TrimSpace(input.Reason) == "" {
		return preparedConsolidation{}, fmt.Errorf("consolidation reason is required")
	}
	if !livingid.ValidID(input.EventID) {
		return preparedConsolidation{}, fmt.Errorf("event id is not a stable identifier")
	}
	if canonical.CurrentLifecycle.State == StateRejected || canonical.CurrentLifecycle.State == StateRetired {
		return preparedConsolidation{}, fmt.Errorf("canonical story is %s; consolidation requires a live canonical proposal", canonical.CurrentLifecycle.State)
	}
	state := StateRejected
	preservesAccepted := duplicate.CurrentLifecycle.State != StateAccepted
	switch duplicate.CurrentLifecycle.State {
	case StateProposed, StateDeferred:
		state = StateRejected
	case StateAccepted:
		if canonical.CurrentLifecycle.State != StateAccepted {
			return preparedConsolidation{}, fmt.Errorf("accepted duplicate intent is preserved only by an accepted canonical story")
		}
		state, preservesAccepted = StateRetired, true
	default:
		return preparedConsolidation{}, fmt.Errorf("duplicate story is already %s and cannot be consolidated", duplicate.CurrentLifecycle.State)
	}
	criterionMap, err := validateCriterionMap(document.SagaID, duplicate, canonical, input.CriterionMap)
	if err != nil {
		return preparedConsolidation{}, err
	}
	transactionLinks, err := transactionOwnedCriterionLinks(document, duplicateRef.ID)
	if err != nil {
		return preparedConsolidation{}, err
	}
	if len(transactionLinks) > 0 {
		return preparedConsolidation{}, fmt.Errorf("consolidation refuses partial graph retirement: transaction-owned criterion links must be rewritten with apply-slide first: %s", strings.Join(transactionLinks, ", "))
	}
	event := LifecycleEvent{Schema: LifecycleEventSchemaURL, Version: Version, ID: input.EventID, Story: input.Duplicate, Parents: copyStrings(input.Parents), State: state, Reason: strings.TrimSpace(input.Reason) + "; canonical: " + input.Canonical, CreatedAt: mutationTime(input.CreatedAt)}
	if err := validateEvent(event, document.SagaID, duplicate.Identity.ID); err != nil {
		return preparedConsolidation{}, err
	}
	for _, existing := range duplicate.Events {
		if existing.ID == event.ID {
			return preparedConsolidation{}, fmt.Errorf("lifecycle event id %q already exists", event.ID)
		}
	}
	candidateStory := *duplicate
	candidateStory.Events = append(append([]LifecycleEvent{}, duplicate.Events...), event)
	if err := validateStoryGraphs(&candidateStory, document.SagaID, document.citationIDs(), document.personaIDs()); err != nil {
		return preparedConsolidation{}, err
	}
	now := event.CreatedAt
	knownRelationIDs := map[string]bool{}
	for _, relation := range document.Relations {
		knownRelationIDs[relation.ID] = true
	}
	var replacements, superseded []Relation
	var remaps []RelationRemap
	for _, relation := range document.Relations {
		if relation.State != RelationActive {
			continue
		}
		from, fromChanged := consolidationEndpoint(relation.From, input.Duplicate, input.Canonical, criterionMap)
		to, toChanged := consolidationEndpoint(relation.To, input.Duplicate, input.Canonical, criterionMap)
		if !fromChanged && !toChanged {
			continue
		}
		if err := validateConsolidationRelationCurrency(document, relation, input.CurrencyInputs); err != nil {
			return preparedConsolidation{}, fmt.Errorf("relation %q cannot be remapped: %w", relation.ID, err)
		}
		replacement := relation.Confirmed()
		replacement.ID = relationReplacementID(relation.ID, canonicalRef.ID)
		if knownRelationIDs[replacement.ID] {
			return preparedConsolidation{}, fmt.Errorf("replacement relation id %q already exists; consolidation refuses ambiguous overwrite", replacement.ID)
		}
		knownRelationIDs[replacement.ID] = true
		replacement.From, replacement.To = from, to
		if requirementEndpoint(from) {
			replacement.FromRevision, err = currentRequirementRevision(document, from)
			if err != nil {
				return preparedConsolidation{}, fmt.Errorf("remap relation %q source: %w", relation.ID, err)
			}
		}
		if requirementEndpoint(to) {
			replacement.ToRevision, err = currentRequirementRevision(document, to)
			if err != nil {
				return preparedConsolidation{}, fmt.Errorf("remap relation %q target: %w", relation.ID, err)
			}
		}
		if replacement.Type == RelationConflictsWith && replacement.To < replacement.From {
			replacement.From, replacement.To = replacement.To, replacement.From
			replacement.FromRevision, replacement.ToRevision = replacement.ToRevision, replacement.FromRevision
			replacement.FromContentDigest, replacement.ToContentDigest = replacement.ToContentDigest, replacement.FromContentDigest
		}
		replacement.CreatedAt = now
		replacement.RequestID = ""
		replacement.State, replacement.SupersededAt, replacement.SupersedeRequestID = RelationActive, nil, ""
		replacement.Repins = nil
		if err := validateRelation(replacement, document.SagaID, replacement.ID); err != nil {
			return preparedConsolidation{}, fmt.Errorf("remap relation %q: %w", relation.ID, err)
		}
		old := relation
		old.State, old.SupersededAt, old.SupersedeRequestID = RelationSuperseded, &now, ""
		if err := validateRelation(old, document.SagaID, old.ID); err != nil {
			return preparedConsolidation{}, err
		}
		replacements, superseded = append(replacements, replacement), append(superseded, old)
		oldURN, _ := relationURN(document.SagaID, relation.ID)
		newURN, _ := relationURN(document.SagaID, replacement.ID)
		remaps = append(remaps, RelationRemap{Existing: oldURN, Replacement: newURN, From: replacement.From, To: replacement.To})
	}
	candidateDocument := *document
	candidateDocument.Stories = append([]Story{}, document.Stories...)
	for index := range candidateDocument.Stories {
		if candidateDocument.Stories[index].Identity.ID == duplicate.Identity.ID {
			candidateDocument.Stories[index] = candidateStory
		}
	}
	candidateDocument.Relations = append([]Relation{}, document.Relations...)
	for _, old := range superseded {
		for index := range candidateDocument.Relations {
			if candidateDocument.Relations[index].ID == old.ID {
				candidateDocument.Relations[index] = old
			}
		}
	}
	candidateDocument.Relations = append(candidateDocument.Relations, replacements...)
	if err := validateRelationSet(&candidateDocument); err != nil {
		return preparedConsolidation{}, fmt.Errorf("consolidated relation set is invalid: %w", err)
	}
	eventURN, _ := StoryEventURN(document.SagaID, duplicate.Identity.ID, event.ID)
	sort.Slice(remaps, func(i, j int) bool { return remaps[i].Existing < remaps[j].Existing })
	plan := ConsolidationPlan{Duplicate: input.Duplicate, Canonical: input.Canonical, CriterionMap: criterionMap, LifecycleState: state, LifecycleEvent: eventURN, RelationRemaps: remaps, PreservesIntent: preservesAccepted}
	return preparedConsolidation{plan: plan, event: event, duplicate: duplicate, replacements: replacements, superseded: superseded}, nil
}

func validateConsolidationRelationCurrency(document *Document, relation Relation, inputs StaleInputs) error {
	confirmed := relation.Confirmed()
	for _, pin := range []struct {
		name, endpoint, revision, digest string
	}{
		{"from", confirmed.From, confirmed.FromRevision, confirmed.FromContentDigest},
		{"to", confirmed.To, confirmed.ToRevision, confirmed.ToContentDigest},
	} {
		endpoint, err := parseEndpoint(pin.endpoint)
		if err != nil {
			return err
		}
		if pin.digest != "" {
			if _, ok := inputs.CurrentContentDigests[pin.endpoint]; !ok {
				return fmt.Errorf("%s content digest for %s has no supplied current value", pin.name, pin.endpoint)
			}
		}
		if pin.revision != "" && !isRequirement(endpoint.Kind) {
			_, current := inputs.CurrentRevisions[pin.endpoint]
			_, conflicted := inputs.ConflictedRevisions[pin.endpoint]
			if !current && !conflicted {
				return fmt.Errorf("%s revision for %s has no supplied current head", pin.name, pin.endpoint)
			}
		}
	}
	currency := EvaluateRelation(*document, relation, inputs)
	if currency.Status != CurrencyCurrent {
		codes := make([]string, 0, len(currency.Reasons))
		for _, reason := range currency.Reasons {
			codes = append(codes, reason.Endpoint+":"+reason.Code)
		}
		return fmt.Errorf("relation is %s (%s)", currency.Status, strings.Join(codes, ", "))
	}
	return nil
}

func currentRequirementRevision(document *Document, value string) (string, error) {
	ref, err := livingid.Parse(value)
	if err != nil || (ref.Kind != livingid.KindStory && ref.Kind != livingid.KindCriterion) {
		return "", fmt.Errorf("%q is not a story or criterion", value)
	}
	storyID := ref.ID
	if ref.Kind == livingid.KindCriterion {
		storyID = ref.ParentID
	}
	story := findStory(document, storyID)
	if story == nil || len(story.RevisionHeads) != 1 || story.CurrentRevision == nil {
		return "", fmt.Errorf("story %q has no single current revision", storyID)
	}
	return story.RevisionHeads[0], nil
}

const maxSlideTransactionBytes = 8 << 20

type consolidationTransactionRecord struct {
	Current   string                             `json:"current"`
	Revisions []consolidationTransactionRevision `json:"revisions"`
}

type consolidationTransactionRevision struct {
	Snapshot string                         `json:"snapshot"`
	Items    []consolidationTransactionItem `json:"items"`
}

type consolidationTransactionItem struct {
	CriterionLinks []consolidationTransactionLink `json:"criterion_links"`
}

type consolidationTransactionLink struct {
	ID        string `json:"id"`
	Criterion string `json:"criterion"`
}

// transactionOwnedCriterionLinks finds current complete-slide links that the
// requirements relation batch cannot rewrite. Consolidation refuses them
// explicitly instead of retiring a story while leaving a partial live graph.
func transactionOwnedCriterionLinks(document *Document, duplicateStoryID string) ([]string, error) {
	directories := []string{filepath.Join(document.Root, applayout.OnboardingDir)}
	for _, feature := range document.Features {
		directories = append(directories, filepath.Join(feature.Dir, applayout.SlidesDir))
	}
	var affected []string
	rootCount := len(directories)
	for directoryIndex := 0; directoryIndex < len(directories); directoryIndex++ {
		directory := directories[directoryIndex]
		info, err := os.Lstat(directory)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, fmt.Errorf("inspect transaction-owned criterion links: %s must be a real directory", relative(document.Root, directory))
		}
		entries, err := os.ReadDir(directory)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			// Complete-slide records live in a deck bundle, one level below
			// ___slides (or ___onboarding). Keep flat records compatible, but
			// never recursively walk arbitrary directories or follow symlinks.
			if directoryIndex < rootCount && strings.HasSuffix(entry.Name(), ".deck") {
				directories = append(directories, filepath.Join(directory, entry.Name()))
				continue
			}
			if !strings.HasPrefix(entry.Name(), "25-t-") || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			path := filepath.Join(directory, entry.Name())
			entryInfo, err := os.Lstat(path)
			if err != nil || entryInfo.Mode()&os.ModeSymlink != 0 || !entryInfo.Mode().IsRegular() {
				return nil, fmt.Errorf("inspect transaction-owned criterion links: %s must be a real regular file", relative(document.Root, path))
			}
			if entryInfo.Size() > maxSlideTransactionBytes {
				return nil, fmt.Errorf("inspect transaction-owned criterion links: %s exceeds %d bytes", relative(document.Root, path), maxSlideTransactionBytes)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			var record consolidationTransactionRecord
			if err := json.Unmarshal(data, &record); err != nil {
				return nil, fmt.Errorf("inspect transaction-owned criterion links in %s: %w", relative(document.Root, path), err)
			}
			var current *consolidationTransactionRevision
			for index := range record.Revisions {
				if record.Revisions[index].Snapshot == record.Current {
					if current != nil {
						return nil, fmt.Errorf("inspect transaction-owned criterion links in %s: current snapshot appears more than once", relative(document.Root, path))
					}
					current = &record.Revisions[index]
				}
			}
			if current == nil {
				return nil, fmt.Errorf("inspect transaction-owned criterion links in %s: current snapshot does not name a revision", relative(document.Root, path))
			}
			for itemIndex, item := range current.Items {
				for linkIndex, link := range item.CriterionLinks {
					ref, parseErr := livingid.Parse(link.Criterion)
					if parseErr != nil || ref.Kind != livingid.KindCriterion || ref.SagaID != document.SagaID || ref.ParentID != duplicateStoryID {
						continue
					}
					linkID := link.ID
					if linkID == "" {
						linkID = fmt.Sprintf("index-%d", linkIndex)
					}
					affected = append(affected, fmt.Sprintf("%s#revisions/%s/items/%d/criterion_links/%s", relative(document.Root, path), current.Snapshot, itemIndex, linkID))
				}
			}
		}
	}
	sort.Strings(affected)
	return affected, nil
}

func validateCriterionMap(sagaID string, duplicate, canonical *Story, given map[string]string) (map[string]string, error) {
	want, targets := map[string]bool{}, map[string]bool{}
	for _, criterion := range duplicate.CurrentRevision.AcceptanceCriteria {
		urn, _ := criterionURN(sagaID, duplicate.Identity.ID, criterion.ID)
		want[urn] = true
	}
	for _, criterion := range canonical.CurrentRevision.AcceptanceCriteria {
		urn, _ := criterionURN(sagaID, canonical.Identity.ID, criterion.ID)
		targets[urn] = true
	}
	if len(given) != len(want) {
		return nil, fmt.Errorf("criterion mapping must name every current duplicate criterion exactly once (got %d, want %d)", len(given), len(want))
	}
	result, usedTargets := map[string]string{}, map[string]bool{}
	for from, to := range given {
		if !want[from] {
			return nil, fmt.Errorf("criterion mapping source %q is not a current duplicate criterion", from)
		}
		if !targets[to] {
			return nil, fmt.Errorf("criterion mapping target %q is not a current canonical criterion", to)
		}
		if usedTargets[to] {
			return nil, fmt.Errorf("criterion mapping target %q is ambiguous; each canonical criterion may be named once", to)
		}
		usedTargets[to], result[from] = true, to
	}
	return result, nil
}

func consolidationEndpoint(value, duplicate, canonical string, criteria map[string]string) (string, bool) {
	if value == duplicate {
		return canonical, true
	}
	if mapped, ok := criteria[value]; ok {
		return mapped, true
	}
	return value, false
}

func requirementEndpoint(value string) bool {
	ref, err := livingid.Parse(value)
	return err == nil && (ref.Kind == livingid.KindStory || ref.Kind == livingid.KindCriterion)
}

func relationReplacementID(existing, canonical string) string {
	value := store.Slug(existing + "--to--" + canonical)
	if len(value) > 128 {
		value = strings.Trim(value[:128], "-")
	}
	return value
}
