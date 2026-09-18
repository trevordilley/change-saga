package livingapp

import (
	"sort"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/readiness"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/sagaref"
)

const (
	historyReview = "review"
	historySource = "source"

	scopeSelf        = "self"
	scopeDescendants = "descendants"
)

// criterionFrame is one accepted criterion plus the identity every axis needs.
type criterionFrame struct {
	urn, id, statement string
	story              string
	currentRevision    string
	revisionHeads      []string
}

// visualTarget resolves a deck, slide, or Item target to its deck role and the
// Items it contains.
type visualTarget struct {
	deckRole string
	kind     sagaref.TargetKind
	chain    []string
	items    []*saga.Item
}

// assembler holds the indexes one Assemble call builds.
type assembler struct {
	in             StatusInputs
	storyRevision  map[string]string
	criteria       []criterionFrame
	byCriterion    map[string]*criterionFrame
	visual         map[string]visualTarget
	orphans        map[string]string
	testCases      map[string]*quality.TestCase
	currentRevs    map[string]string
	evidenceStale  map[string][]string
	stale          map[string]*StaleRecord
	affects        map[string]map[string]bool
	implicatedBy   map[string]map[string]bool
	implicatedKind map[string]string
}

// Assemble composes the status projection from already-loaded records. It is a
// pure function: the same inputs always produce byte-identical JSON.
func Assemble(in StatusInputs) Status {
	// Exceptions are annotated with citation resolution below; copy them so the
	// caller's records are never mutated.
	in.Exceptions = append([]coverage.Exception(nil), in.Exceptions...)
	a := &assembler{
		in: in, storyRevision: map[string]string{}, byCriterion: map[string]*criterionFrame{},
		visual: map[string]visualTarget{}, orphans: map[string]string{}, testCases: map[string]*quality.TestCase{},
		currentRevs: map[string]string{}, evidenceStale: map[string][]string{}, stale: map[string]*StaleRecord{},
		affects: map[string]map[string]bool{}, implicatedBy: map[string]map[string]bool{}, implicatedKind: map[string]string{},
	}
	status := Status{
		SagaID: in.SagaID, SagaVersion: in.SagaVersion, Capabilities: []Capability{}, Stories: []StoryStatus{}, Prototypes: []PrototypeStatus{},
		Stale: []StaleRecord{}, Diagnostics: append([]Diagnostic{}, in.Diagnostics...),
	}
	status.Policy = a.policy()
	status.Capabilities = a.capabilities()
	status.Stories = a.indexStories()
	a.indexVisual()
	a.indexOrphans()
	a.indexQuality()

	links := a.linksByCriterion()
	protoStatus, protoLinks, protoInputs := a.prototypeAxis()
	status.Prototypes = protoStatus
	for criterion, values := range protoLinks {
		links[criterion] = append(links[criterion], values...)
	}
	qualityEval := a.qualityAxis()
	for criterion, values := range qualityEval.links {
		links[criterion] = append(links[criterion], values...)
	}

	inputs := make([]coverage.CriterionInput, 0, len(a.criteria))
	for _, frame := range a.criteria {
		input := coverage.CriterionInput{
			URN: frame.urn, Story: frame.story, CurrentStoryRevision: frame.currentRevision,
			RevisionHeads: copyStrings(frame.revisionHeads), Links: links[frame.urn], Unsatisfied: map[coverage.Axis][]string{},
		}
		if values := qualityEval.unsatisfied[frame.urn]; len(values) > 0 {
			input.Unsatisfied[coverage.AxisQuality] = values
		}
		input.Unsatisfied[coverage.AxisUI] = []string{"no UI reference resource is recorded by this Change Saga version; only an explicit ui coverage exception resolves this axis"}
		inputs = append(inputs, input)
	}
	a.exceptionCitations()
	status.Axes = coverage.ProjectAxes(inputs, a.in.Exceptions, coverage.FeatureAxisPolicy())
	a.exceptionStale(status.Axes)

	status.Quality = a.finishQuality(qualityEval, status.Axes)
	status.ChangedSource = a.changedSource()

	stories := make([]readiness.Story, 0, len(in.Stories))
	for _, row := range status.Stories {
		story := readiness.Story{
			URN: row.Story, State: row.State, RevisionHeads: copyStrings(row.RevisionHeads), LifecycleHeads: copyStrings(row.LifecycleHeads),
			CurrentRevision: row.CurrentRevision, Criteria: []string{}, IdentityIssues: []string{},
		}
		for _, criterion := range row.Criteria {
			story.Criteria = append(story.Criteria, criterion.Criterion)
		}
		stories = append(stories, story)
	}
	facts := make([]readiness.QualityFact, 0, len(status.Quality.Facts))
	for _, fact := range status.Quality.Facts {
		facts = append(facts, readiness.QualityFact{
			Criterion: fact.Criterion, Kind: fact.Kind, Required: fact.Required, TestCase: fact.TestCase, RunResult: fact.RunResult,
			RunHeads: copyStrings(fact.RunHeads), EvidenceResolved: fact.EvidenceResolved, StaleReasons: copyStrings(fact.StaleReasons),
		})
	}
	uncoveredOrphans := make([]string, 0, len(status.ChangedSource.Orphans))
	for _, orphan := range status.ChangedSource.Orphans {
		uncoveredOrphans = append(uncoveredOrphans, orphan.DiffFile+"#"+itoa(orphan.Diff))
	}
	status.Readiness = readiness.EvaluateGates(readiness.GateInputs{
		Policy: status.Policy.Name, Stories: stories, Prototypes: protoInputs, Coverage: status.Axes,
		Quality: readiness.QualityDomain{Adoption: string(in.Quality.Adoption)}, QualityFacts: facts,
		ChangedSource: readiness.ChangedSourceAccounting{Complete: status.ChangedSource.Complete, Uncovered: status.ChangedSource.uncoveredURIs, Orphans: uncoveredOrphans},
		PeerReview:    in.PeerReview,
	})
	status.Stale = a.staleRecords()
	return status
}

func (a *assembler) policy() PolicyChoice {
	switch a.in.Policy {
	case readiness.PolicyFeature, readiness.PolicyCompatibility:
		return PolicyChoice{Name: a.in.Policy, Source: "explicit"}
	}
	if a.in.SagaVersion == quality.Version && a.in.Quality.Adoption != quality.NotAdopted && a.in.Quality.Adoption != "" {
		return PolicyChoice{Name: readiness.PolicyFeature, Source: "default for a v5 Saga that adopted quality"}
	}
	return PolicyChoice{Name: readiness.PolicyCompatibility, Source: "default: v" + itoa(a.in.SagaVersion) + " Saga without adopted quality keeps peer-review readiness; the new gates are guidance"}
}

func (a *assembler) capabilities() []Capability {
	state := func(adopted bool) string {
		if adopted {
			return string(quality.Adopted)
		}
		return string(quality.NotAdopted)
	}
	result := []Capability{
		{Name: "requirements", State: state(a.in.RequirementsAdopted)},
		{Name: "prototypes", State: state(a.in.Prototypes.Adopted)},
		{Name: "coverage_exceptions", State: state(a.in.ExceptionsAdopted)},
	}
	if !a.in.RequirementsAdopted {
		result[0].Reason = "no ___requirements root; stories, criteria, and relations are not recorded"
	}
	if a.in.Unavailable != "" {
		result[0].State, result[0].Reason = "unavailable", a.in.Unavailable
	}
	if !a.in.ExceptionsAdopted && a.in.SagaVersion != quality.Version {
		result[2].Reason = "coverage exceptions are v5 records"
	}
	adoption := string(a.in.Quality.Adoption)
	if adoption == "" {
		adoption = string(quality.NotAdopted)
	}
	result = append(result, Capability{Name: "quality", State: adoption, Reason: a.in.QualityReason})
	return result
}

// indexStories records every story and the accepted criteria the axes cover.
// A story with competing revision heads contributes every head's criteria so
// each cell reports the conflict instead of silently picking a head.
func (a *assembler) indexStories() []StoryStatus {
	rows := []StoryStatus{}
	for i := range a.in.Stories {
		story := &a.in.Stories[i]
		storyURN, _ := livingid.Story(a.in.SagaID, story.Identity.ID)
		row := StoryStatus{
			Story: storyURN, ID: story.Identity.ID, State: "conflicted", RevisionHeads: copyStrings(story.RevisionHeads),
			LifecycleHeads: copyStrings(story.LifecycleHeads), Criteria: []CriterionStatus{},
		}
		if story.CurrentLifecycle != nil {
			row.State = string(story.CurrentLifecycle.State)
		} else if len(story.LifecycleHeads) == 0 {
			row.State = "none"
		}
		heads := []requirements.Revision{}
		if story.CurrentRevision != nil {
			row.Title = story.CurrentRevision.Title
			row.CurrentRevision, _ = livingid.Revision(a.in.SagaID, story.Identity.ID, story.CurrentRevision.ID)
			a.storyRevision[storyURN] = row.CurrentRevision
			heads = append(heads, *story.CurrentRevision)
			for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
				urn, _ := livingid.Criterion(a.in.SagaID, story.Identity.ID, criterion.ID)
				row.Criteria = append(row.Criteria, CriterionStatus{Criterion: urn, ID: criterion.ID, Statement: criterion.Statement})
			}
		} else {
			for _, revision := range story.Revisions {
				urn, _ := livingid.Revision(a.in.SagaID, story.Identity.ID, revision.ID)
				if contains(story.RevisionHeads, urn) {
					heads = append(heads, revision)
				}
			}
		}
		if row.State == string(requirements.StateAccepted) {
			seen := map[string]bool{}
			for _, revision := range heads {
				for _, criterion := range revision.AcceptanceCriteria {
					urn, _ := livingid.Criterion(a.in.SagaID, story.Identity.ID, criterion.ID)
					if seen[urn] {
						continue
					}
					seen[urn] = true
					a.criteria = append(a.criteria, criterionFrame{
						urn: urn, id: criterion.ID, statement: criterion.Statement, story: storyURN,
						currentRevision: row.CurrentRevision, revisionHeads: copyStrings(story.RevisionHeads),
					})
				}
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Story < rows[j].Story })
	sort.Slice(a.criteria, func(i, j int) bool { return a.criteria[i].urn < a.criteria[j].urn })
	for i := range a.criteria {
		a.byCriterion[a.criteria[i].urn] = &a.criteria[i]
	}
	return rows
}

// indexVisual resolves deck, slide, and Item targets. Containment is exact
// parent identity from the validated v4 records; nothing is inferred from order.
func (a *assembler) indexVisual() {
	for _, deck := range a.in.Decks {
		deckItems := []*saga.Item{}
		for _, slide := range deck.Slides {
			deckItems = append(deckItems, slide.Items...)
			a.visual[slide.Target] = visualTarget{deckRole: deck.Role, kind: sagaref.TargetSlide, chain: []string{slide.Target}, items: append([]*saga.Item(nil), slide.Items...)}
			for _, item := range slide.Items {
				a.visual[item.Target] = visualTarget{deckRole: deck.Role, kind: sagaref.TargetItem, chain: []string{item.Target}, items: []*saga.Item{item}}
			}
		}
		a.visual[deck.Target] = visualTarget{deckRole: deck.Role, kind: sagaref.TargetDeck, chain: []string{deck.Target}, items: deckItems}
	}
}

func (a *assembler) indexOrphans() {
	for _, orphan := range a.in.Report.Orphans {
		a.orphans[orphanKey(orphan.Assignment.Target, orphan.Assignment.DiffFile, orphan.Assignment.Diff)] = orphan.Reason
	}
}

func orphanKey(target, file string, index int) string {
	return target + "\x00" + file + "\x00" + itoa(index)
}

// itemPath returns the containment hops from a relation source to one Item.
func (a *assembler) itemPath(source string, item *saga.Item) []string {
	if source == item.Target {
		return []string{item.Target}
	}
	slideTarget := ""
	for _, deck := range a.in.Decks {
		for _, slide := range deck.Slides {
			for _, candidate := range slide.Items {
				if candidate == item {
					slideTarget = slide.Target
				}
			}
		}
	}
	if source == slideTarget {
		return []string{source, item.Target}
	}
	return []string{source, slideTarget, item.Target}
}

// reach resolves which criteria a relation target covers: the criterion itself
// (direct) or every accepted criterion of the target story (broad).
func (a *assembler) reach(to string) ([]string, bool) {
	if _, ok := a.byCriterion[to]; ok {
		return []string{to}, false
	}
	result := []string{}
	for _, frame := range a.criteria {
		if frame.story == to {
			result = append(result, frame.urn)
		}
	}
	return result, true
}

func hops(criterion, story string, broad bool, rest ...string) []string {
	path := []string{criterion}
	if broad {
		path = append(path, story)
	}
	return append(path, rest...)
}

// linksByCriterion projects every persisted relation onto the design and
// implementation axes. An addresses relation counts as design coverage on the
// axis its source belongs to; addresses and legacy explains relations both
// reach code through Items, and only a path ending at a current exact diff is
// implementation coverage.
func (a *assembler) linksByCriterion() map[string][]coverage.AxisLink {
	result := map[string][]coverage.AxisLink{}
	links := append([]Link(nil), a.in.Links...)
	sort.Slice(links, func(i, j int) bool { return links[i].URN < links[j].URN })
	for _, link := range links {
		if !link.Active || (link.Type != requirements.RelationAddresses && link.Type != requirements.RelationExplains) {
			continue
		}
		criteria, broad := a.reach(link.To)
		if len(criteria) == 0 {
			continue
		}
		stale, conflicts, invalid := link.reasons()
		a.staleRelation(link, stale, criteria)
		visual, isVisual := a.visual[link.From]
		for _, criterion := range criteria {
			story := a.byCriterion[criterion].story
			if link.Type == requirements.RelationAddresses {
				axis := coverage.AxisTechnical
				if isVisual && visual.deckRole == "ux" {
					axis = coverage.AxisUX
				}
				if ref, err := livingid.Parse(link.From); err == nil && ref.Kind == livingid.KindDesign || isVisual {
					result[criterion] = append(result[criterion], coverage.AxisLink{
						Axis: axis, Relation: link.URN, Source: link.From, Broad: broad, PinnedRevision: link.ToRevision,
						Paths: [][]string{hops(criterion, story, broad, link.From)}, StaleReasons: copyStrings(stale),
						ConflictReasons: copyStrings(conflicts), InvalidReasons: copyStrings(invalid),
					})
				}
			}
			if isVisual {
				value := a.implementationLink(link, visual, criterion, story, broad, stale)
				value.ConflictReasons, value.InvalidReasons = copyStrings(conflicts), copyStrings(invalid)
				result[criterion] = append(result[criterion], value)
			}
		}
	}
	return result
}

func (a *assembler) implementationLink(link Link, visual visualTarget, criterion, story string, broad bool, stale []string) coverage.AxisLink {
	value := coverage.AxisLink{
		Axis: coverage.AxisImplementation, Relation: link.URN, Source: link.From, Broad: broad, PinnedRevision: link.ToRevision,
		Paths: [][]string{}, Diffs: []string{}, StaleReasons: copyStrings(stale),
	}
	scope := link.Scope
	if scope == "" {
		scope = scopeSelf
	}
	items := visual.items
	if visual.kind != sagaref.TargetItem && scope != scopeDescendants {
		value.Unsatisfied = append(value.Unsatisfied, "scope self on a "+string(visual.kind)+" does not reach contained Items; relate the Item or use scope descendants")
		return value
	}
	owned, orphaned := 0, 0
	for _, item := range items {
		for _, file := range item.Diffs {
			for index, diff := range file.Diffs {
				owned++
				if reason, ok := a.orphans[orphanKey(item.Target, file.Path, index+1)]; ok {
					orphaned++
					a.addImplicated(criterion, "criterion", item.Target+" "+diff.URI+": "+reason)
					continue
				}
				value.Diffs = append(value.Diffs, diff.URI)
				value.Paths = append(value.Paths, hops(criterion, story, broad, append(a.itemPath(link.From, item), diff.URI)...))
			}
		}
	}
	value.Diffs = uniqueSorted(value.Diffs)
	value.Paths = uniquePaths(value.Paths)
	switch {
	case owned == 0:
		value.Unsatisfied = append(value.Unsatisfied, "no Item on this path owns an exact diff")
	case len(value.Diffs) == 0:
		value.StaleReasons = append(value.StaleReasons, itoa(orphaned)+" Item diff selectors do not match the current source comparison")
	}
	return value
}

// staleRelation records a stale relation with the pins requirements reported.
// Invalid and conflicted relations surface in their cells instead.
func (a *assembler) staleRelation(link Link, stale, criteria []string) {
	if link.Currency != requirements.CurrencyStale {
		return
	}
	a.markStale(link.URN, "relation", historyReview, stale, link.pins(), criteria)
	scope := ""
	if link.Version == requirements.V5RelationVersion {
		scope = link.Scope
	}
	a.describeStale(link.URN, string(link.Type), link.From, link.To, "", scope)
}

func (a *assembler) currentRevision(endpoint string) string {
	if frame, ok := a.byCriterion[endpoint]; ok {
		return frame.currentRevision
	}
	if revision, ok := a.storyRevision[endpoint]; ok {
		return revision
	}
	if revision, ok := a.currentRevs[endpoint]; ok {
		return revision
	}
	// A criterion of a story that is not accepted is still pinned to its story.
	if ref, err := livingid.Parse(endpoint); err == nil && ref.Kind == livingid.KindCriterion {
		story, _ := livingid.Story(ref.SagaID, ref.ParentID)
		return a.storyRevision[story]
	}
	return ""
}

// prototypeAxis projects current prototype annotations onto the prototype
// axis and derives the product_ready prototype inputs.
func (a *assembler) prototypeAxis() ([]PrototypeStatus, map[string][]coverage.AxisLink, []readiness.Prototype) {
	stories := []prototypes.StoryInput{}
	storyCriteria := map[string][]string{}
	for _, frame := range a.criteria {
		storyCriteria[frame.story] = append(storyCriteria[frame.story], frame.urn)
	}
	for i := range a.in.Stories {
		urn, _ := livingid.Story(a.in.SagaID, a.in.Stories[i].Identity.ID)
		criteria := []string{}
		if current := a.in.Stories[i].CurrentRevision; current != nil {
			for _, criterion := range current.AcceptanceCriteria {
				value, _ := livingid.Criterion(a.in.SagaID, a.in.Stories[i].Identity.ID, criterion.ID)
				criteria = append(criteria, value)
			}
		}
		stories = append(stories, prototypes.StoryInput{URN: urn, CurrentRevision: a.storyRevision[urn], Criteria: criteria})
	}
	for _, prototype := range a.in.Prototypes.Prototypes {
		if prototype.CurrentRevision != nil {
			urn, _ := prototypes.PrototypeURN(a.in.SagaID, prototype.Identity.ID)
			a.currentRevs[urn], _ = prototypes.RevisionURN(a.in.SagaID, prototype.Identity.ID, prototype.CurrentRevision.ID)
		}
	}
	projection := prototypes.Compose(a.in.Prototypes, prototypes.CompositionInputs{Stories: stories})
	links := map[string][]coverage.AxisLink{}
	current := map[string][]string{}
	staleLinks := map[string][]string{}
	for _, annotation := range projection.Annotations {
		prototypeID := urnID(annotation.Annotation.Prototype)
		urn, _ := prototypes.AnnotationURN(a.in.SagaID, prototypeID, annotation.Annotation.ID)
		if annotation.Current {
			current[annotation.Annotation.Prototype] = append(current[annotation.Annotation.Prototype], urn)
		} else {
			staleLinks[annotation.Annotation.Prototype] = append(staleLinks[annotation.Annotation.Prototype], urn)
		}
		criteria, broad := a.reach(annotation.Annotation.Target)
		if len(annotation.StaleReasons) > 0 {
			pins := []Pin{{Field: "story_revision", Pinned: annotation.Annotation.StoryRevision, Current: a.currentRevision(annotation.Annotation.Target)}}
			if annotation.Annotation.PrototypeRevision != "" {
				pins = append(pins, Pin{Field: "prototype_revision", Pinned: annotation.Annotation.PrototypeRevision, Current: a.currentRevs[annotation.Annotation.Prototype]})
			}
			a.markStale(urn, "prototype_annotation", historyReview, annotation.StaleReasons, pins, criteria)
		}
		for _, criterion := range criteria {
			links[criterion] = append(links[criterion], coverage.AxisLink{
				Axis: coverage.AxisPrototype, Source: urn, Broad: broad, PinnedRevision: annotation.Annotation.StoryRevision,
				Paths:        [][]string{hops(criterion, a.byCriterion[criterion].story, broad, urn, annotation.Annotation.Prototype)},
				StaleReasons: copyStrings(annotation.StaleReasons),
			})
		}
	}
	rows := []PrototypeStatus{}
	inputs := []readiness.Prototype{}
	for _, prototype := range a.in.Prototypes.Prototypes {
		urn, _ := prototypes.PrototypeURN(a.in.SagaID, prototype.Identity.ID)
		row := PrototypeStatus{
			Prototype: urn, State: "conflicted", Retained: true, RevisionHeads: copyStrings(prototype.RevisionHeads),
			CurrentLinks: uniqueSorted(current[urn]), StaleLinks: uniqueSorted(staleLinks[urn]),
		}
		input := readiness.Prototype{URN: urn, Retained: true, CurrentLinks: row.CurrentLinks, StaleReasons: []string{}}
		if prototype.CurrentRevision != nil {
			row.State = string(prototype.CurrentRevision.State)
			row.CurrentRevision, _ = prototypes.RevisionURN(a.in.SagaID, prototype.Identity.ID, prototype.CurrentRevision.ID)
			row.Retained = prototype.CurrentRevision.State != prototypes.StateRetired
			input.Retained = row.Retained
		} else {
			input.StaleReasons = append(input.StaleReasons, "prototype has multiple revision heads")
		}
		rows = append(rows, row)
		inputs = append(inputs, input)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Prototype < rows[j].Prototype })
	return rows, links, inputs
}

// exceptionCitations resolves exception citations against recorded citations;
// the schema cannot express that a cited URN exists.
func (a *assembler) exceptionCitations() {
	known := map[string]bool{}
	for _, citation := range a.in.Citations {
		urn, _ := livingid.Citation(a.in.SagaID, citation.ID)
		known[urn] = true
	}
	for i := range a.in.Exceptions {
		exception := &a.in.Exceptions[i]
		exception.UnresolvedCitations = nil
		for _, citation := range exception.Citations {
			if !known[citation] {
				exception.UnresolvedCitations = append(exception.UnresolvedCitations, citation)
			}
		}
	}
}

func (a *assembler) exceptionStale(projection coverage.AxisProjection) {
	for _, exception := range projection.Exceptions {
		if exception.State != coverage.ExceptionStale {
			continue
		}
		a.markStale(exception.Exception.URN, "coverage_exception", historyReview, exception.Reasons,
			[]Pin{{Field: "story_revision", Pinned: exception.Exception.StoryRevision, Current: a.currentRevision(exception.Exception.Criterion)}},
			[]string{exception.Exception.Criterion})
		a.describeStale(exception.Exception.URN, "", "", exception.Exception.Criterion, string(exception.Exception.Axis), "")
	}
}

func (a *assembler) markStale(record, kind, history string, reasons []string, pins []Pin, affects []string) {
	existing := a.stale[record]
	if existing == nil {
		existing = &StaleRecord{Record: record, Kind: kind, History: history, Reasons: []string{}, Pins: []Pin{}, Affects: []string{}}
		a.stale[record] = existing
		a.affects[record] = map[string]bool{}
	}
	existing.Reasons = uniqueSorted(append(existing.Reasons, reasons...))
	for _, pin := range pins {
		duplicate := false
		for _, known := range existing.Pins {
			if known == pin {
				duplicate = true
			}
		}
		if !duplicate {
			existing.Pins = append(existing.Pins, pin)
		}
	}
	for _, value := range affects {
		a.affects[record][value] = true
	}
}

func (a *assembler) describeStale(record, kind, from, to, axis, scope string) {
	if value := a.stale[record]; value != nil {
		value.Type, value.From, value.To, value.Axis, value.Scope = kind, from, to, axis, scope
	}
}

func (a *assembler) staleRecords() []StaleRecord {
	result := make([]StaleRecord, 0, len(a.stale))
	for record, value := range a.stale {
		copied := *value
		copied.Affects = uniqueSorted(mapKeysBool(a.affects[record]))
		sort.Slice(copied.Pins, func(i, j int) bool { return copied.Pins[i].Field < copied.Pins[j].Field })
		result = append(result, copied)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].History != result[j].History {
			return result[i].History == historyReview
		}
		return result[i].Record < result[j].Record
	})
	return result
}

func (a *assembler) addImplicated(resource, kind, via string) {
	if a.implicatedBy[resource] == nil {
		a.implicatedBy[resource] = map[string]bool{}
	}
	a.implicatedBy[resource][via] = true
	a.implicatedKind[resource] = kind
}

// changedSource keeps the global omission invariant separate from every axis.
// Current test-code evidence accounts for the test atoms it selects, as the
// quality contract requires; nothing else can excuse an unaccounted atom.
func (a *assembler) changedSource() ChangedSource {
	report := a.in.Report
	result := ChangedSource{
		SchemaValid: report.SchemaValid, Atoms: len(a.in.Changes.Atoms), Uncovered: []UncoveredPath{}, Orphans: []OrphanRef{},
		TestOwned: []TestOwned{}, Implicated: []Implicated{},
		Note: "Transitivity proves accepted criteria reach code; it cannot prove nothing else changed. Every changed atom must be owned by some target, and no coverage exception applies here.",
	}
	testOwned := a.testOwnedAtoms()
	byPath := map[string]int{}
	for _, atom := range report.Uncovered {
		if owner, ok := testOwned[atom.Key]; ok {
			owner.Atom = atom.URI
			result.TestOwned = append(result.TestOwned, owner)
			continue
		}
		result.uncoveredURIs = append(result.uncoveredURIs, atom.URI)
		byPath[firstNonEmpty(atom.Path, atom.NewPath, atom.OldPath)]++
	}
	for path, count := range byPath {
		result.Uncovered = append(result.Uncovered, UncoveredPath{Path: path, Atoms: count})
	}
	sort.Slice(result.Uncovered, func(i, j int) bool { return result.Uncovered[i].Path < result.Uncovered[j].Path })
	result.UncoveredAtoms = len(result.uncoveredURIs)
	affects := a.targetCriteria()
	for _, orphan := range report.Orphans {
		ref := OrphanRef{
			Target: orphan.Assignment.Target, DiffFile: orphan.Assignment.DiffFile, Diff: orphan.Assignment.Diff,
			URI: orphan.Reference.URI, Reason: orphan.Reason, Affects: uniqueSorted(affects[orphan.Assignment.Target]),
		}
		result.Orphans = append(result.Orphans, ref)
		a.markStale(orphan.Assignment.DiffFile+"#"+itoa(orphan.Assignment.Diff), "diff_selector", historySource, []string{orphan.Reason},
			[]Pin{{Field: "uri", Pinned: orphan.Reference.URI}}, ref.Affects)
	}
	for resource, via := range a.implicatedBy {
		result.Implicated = append(result.Implicated, Implicated{Resource: resource, Kind: a.implicatedKind[resource], Via: uniqueSorted(mapKeysBool(via))})
	}
	sort.Slice(result.Implicated, func(i, j int) bool { return result.Implicated[i].Resource < result.Implicated[j].Resource })
	sort.Slice(result.TestOwned, func(i, j int) bool { return result.TestOwned[i].Atom < result.TestOwned[j].Atom })
	result.Complete = report.SchemaValid && result.UncoveredAtoms == 0 && len(result.Orphans) == 0
	return result
}

// targetCriteria maps every relation source and its contained Items to the
// criteria reached through active addresses or explains relations.
func (a *assembler) targetCriteria() map[string][]string {
	result := map[string][]string{}
	for _, link := range a.in.Links {
		if !link.Active || (link.Type != requirements.RelationAddresses && link.Type != requirements.RelationExplains) {
			continue
		}
		criteria, _ := a.reach(link.To)
		if len(criteria) == 0 {
			criteria = []string{link.To}
		}
		result[link.From] = append(result[link.From], criteria...)
		if visual, ok := a.visual[link.From]; ok && (visual.kind == sagaref.TargetItem || link.Scope == scopeDescendants) {
			for _, item := range visual.items {
				result[item.Target] = append(result[item.Target], criteria...)
			}
		}
	}
	return result
}

func mapKeysBool(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	return result
}

func itoa(value int) string { return strconv.Itoa(value) }

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func testCaseURN(sagaID, id string) string {
	urn, _ := qualityid.TestCase(sagaID, id)
	return urn
}

func joinReasons(values []string) string { return strings.Join(uniqueSorted(values), "; ") }
