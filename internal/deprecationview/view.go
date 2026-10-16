// Package deprecationview projects authored deprecation assertions. It never
// converts coverage or relation traversal into an assertion about code.
package deprecationview

import (
	"context"
	"strings"
	"sync"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/requirements"
)

type Resolver interface {
	Resolve(context.Context, coderef.Reference, string) coderesolve.Resolution
}

type Evidence struct {
	Role        string                 `json:"role"`
	Owner       string                 `json:"owner"`
	Record      string                 `json:"record"`
	Reference   coderef.Reference      `json:"reference"`
	Resolution  coderesolve.Resolution `json:"resolution"`
	Provisional bool                   `json:"provisional,omitempty"`
	Match       string                 `json:"match,omitempty"`
}

type StoryLink struct {
	Relation       requirements.Relation         `json:"relation"`
	Currency       requirements.RelationCurrency `json:"currency"`
	Lifecycle      string                        `json:"lifecycle"`
	LifecycleHeads []string                      `json:"lifecycle_heads"`
}

type Assertion struct {
	RelatedCodeWarning string                                `json:"related_code_warning,omitempty"`
	Revision           string                                `json:"revision"`
	Definition         requirements.DeprecationRevision      `json:"definition"`
	TargetHealth       requirements.DeprecationTargetHealth  `json:"target_health"`
	ReplacementHealth  *requirements.DeprecationTargetHealth `json:"replacement_health,omitempty"`
	Code               []Evidence                            `json:"code"`
}

type Record struct {
	Deprecation      string                            `json:"deprecation"`
	Identity         requirements.RecordIdentity       `json:"identity"`
	State            string                            `json:"state"`
	RevisionHeads    []string                          `json:"revision_heads"`
	LifecycleHeads   []string                          `json:"lifecycle_heads"`
	CurrentRevision  *requirements.DeprecationRevision `json:"current_revision"`
	CurrentLifecycle *requirements.DeprecationEvent    `json:"current_lifecycle"`
	Assertions       []Assertion                       `json:"assertions"`
	Stories          []StoryLink                       `json:"stories"`
	History          *requirements.Deprecation         `json:"history,omitempty"`
}

type Filter struct {
	ID, Target, State, Scope, Story string
	Ref                             *coderef.Location
	History                         bool
}

func Records(ctx context.Context, doc requirements.Document, head string, resolver Resolver, filter Filter) []Record {
	rows := []Record{}
	targets := requirements.DeprecationTargets{Root: doc.Root, SagaID: doc.SagaID}
	var evaluator *requirements.RelationEvaluator
	for _, dep := range doc.Deprecations {
		urn, _ := requirements.DeprecationURN(doc.SagaID, dep.Identity.ID)
		if filter.ID != "" && filter.ID != urn && filter.ID != dep.Identity.ID {
			continue
		}
		row := Record{Deprecation: urn, Identity: dep.Identity, State: "conflicted", RevisionHeads: dep.RevisionHeads, LifecycleHeads: dep.LifecycleHeads, CurrentRevision: dep.CurrentRevision, CurrentLifecycle: dep.CurrentLifecycle, Assertions: []Assertion{}, Stories: []StoryLink{}}
		if dep.CurrentLifecycle != nil && dep.CurrentRevision != nil {
			row.State = string(dep.CurrentLifecycle.State)
		}
		if filter.State != "" && filter.State != "all" && filter.State != row.State {
			continue
		}
		// Select by authored heads before reading target domains or resolving
		// evidence. Keep every competing head when any candidate matches.
		if !matchesDefinition(dep, doc.SagaID, filter) {
			continue
		}
		for _, relation := range doc.Relations {
			if relation.From != urn {
				continue
			}
			confirmed := relation.Confirmed()
			parts := strings.Split(confirmed.To, ":")
			if len(parts) < 5 || parts[3] != "story" {
				continue
			}
			if evaluator == nil {
				evaluator = requirements.NewRelationEvaluator(doc)
			}
			link := StoryLink{Relation: relation, Currency: evaluator.Evaluate(relation, requirements.StaleInputs{}), Lifecycle: "missing", LifecycleHeads: []string{}}
			if story := doc.FindStory(parts[4]); story != nil {
				link.LifecycleHeads = story.LifecycleHeads
				link.Lifecycle = "conflicted"
				if story.CurrentLifecycle != nil {
					link.Lifecycle = string(story.CurrentLifecycle.State)
				}
			}
			row.Stories = append(row.Stories, link)
		}
		if filter.Story != "" {
			matched := false
			for _, link := range row.Stories {
				matched = matched || link.Relation.To == filter.Story || strings.HasPrefix(link.Relation.To, filter.Story+":criterion:")
			}
			if !matched {
				continue
			}
		}
		matched := false
		for _, revision := range dep.Revisions {
			revURN, _ := requirements.DeprecationRevisionURN(doc.SagaID, dep.Identity.ID, revision.ID)
			if !contains(dep.RevisionHeads, revURN) {
				continue
			}
			assertion := Assertion{Revision: revURN, Definition: revision, TargetHealth: targets.Observe(revision.Target), Code: []Evidence{}}
			if revision.Replacement != nil {
				health := targets.Observe(*revision.Replacement)
				assertion.ReplacementHealth = &health
			}
			for _, reference := range revision.Code {
				role := "related_code"
				if revision.Scope == "code" {
					role = "deprecated_code"
				}
				resolution := coderesolve.Resolution{State: coderesolve.Stale, Location: reference.Location(), Reason: "code repository was not inspected", Provisional: true}
				view := head
				if filter.Ref != nil {
					view = filter.Ref.Commit
				}
				if resolver != nil {
					resolution = resolver.Resolve(ctx, reference, view)
				}
				evidence := Evidence{Role: role, Owner: urn, Record: revURN, Reference: reference, Resolution: resolution, Provisional: resolution.Provisional}
				if filter.Ref != nil {
					evidence.Match = Match(reference, resolution, *filter.Ref)
				}
				assertion.Code = append(assertion.Code, evidence)
			}
			if filter.Ref != nil {
				related, err := targets.RelatedCode(revision.Target.Target)
				if err != nil {
					assertion.RelatedCodeWarning = err.Error()
				}
				for _, file := range related {
					for _, reference := range file.References {
						resolution := coderesolve.Resolution{State: coderesolve.Stale, Location: reference.Location(), Reason: "code repository was not inspected", Provisional: true}
						if resolver != nil {
							resolution = resolver.Resolve(ctx, reference, filter.Ref.Commit)
						}
						assertion.Code = append(assertion.Code, Evidence{Role: "related_target_code", Owner: revision.Target.Target, Record: file.Path, Reference: reference, Resolution: resolution, Provisional: resolution.Provisional, Match: Match(reference, resolution, *filter.Ref)})
					}
				}
			}

			targetMatch := filter.Target == "" || revision.Target.Target == filter.Target
			scopeMatch := filter.Scope == "" || revision.Scope == filter.Scope
			refMatch := filter.Ref == nil
			for _, evidence := range assertion.Code {
				refMatch = refMatch || evidence.Match != ""
			}
			matched = matched || (targetMatch && scopeMatch && refMatch)
			row.Assertions = append(row.Assertions, assertion)
		}
		if !matched {
			continue
		}
		if filter.History {
			history := dep
			row.History = &history
		}
		rows = append(rows, row)
	}
	return rows
}

// Match keeps uncertain evidence visible at its recorded path. Only an exact
// current match is evidence that the selected bytes carry the assertion.
func Match(reference coderef.Reference, resolution coderesolve.Resolution, location coderef.Location) string {
	if resolution.Current() && !resolution.Provisional && resolution.Location.Path == location.Path && overlaps(resolution.Location, location) {
		return "exact"
	}
	if (!resolution.Current() || resolution.Provisional) && (reference.Path == location.Path || resolution.Location.Path == location.Path) {
		return "uncertain_path"
	}
	return ""
}
func overlaps(a, b coderef.Location) bool {
	return a.Start == 0 || b.Start == 0 || (a.Start <= b.End && b.Start <= a.End)
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// matchesDefinition is only a cheap candidate filter. Exact-code matching still
// resolves references, including moved and uncertain evidence, below it.
func matchesDefinition(dep requirements.Deprecation, sagaID string, filter Filter) bool {
	for _, revision := range dep.Revisions {
		urn, _ := requirements.DeprecationRevisionURN(sagaID, dep.Identity.ID, revision.ID)
		if contains(dep.RevisionHeads, urn) && (filter.Target == "" || revision.Target.Target == filter.Target) && (filter.Scope == "" || revision.Scope == filter.Scope) {
			return true
		}
	}
	return false
}

// ContentSnapshot retains immutable content annotations for one generation of
// the Saga's documentation files. It never caches code resolution, which also
// depends on the inspected commit and repository. Callers must not mutate rows.
type ContentSnapshot struct {
	document requirements.Document
	records  sync.Map
}

type contentRecord struct {
	once sync.Once
	rows []Record
}

func NewContentSnapshot(document requirements.Document) *ContentSnapshot {
	return &ContentSnapshot{document: document}
}

// Records selects whole records by any current candidate's exact target. A
// conflicted record retains all candidates, and withdrawn records stay visible.
func (snapshot *ContentSnapshot) Records(ctx context.Context, targets map[string]bool) []Record {
	rows := []Record{}
	for _, dep := range snapshot.document.Deprecations {
		matched := false
		for _, revision := range dep.Revisions {
			urn, _ := requirements.DeprecationRevisionURN(snapshot.document.SagaID, dep.Identity.ID, revision.ID)
			matched = matched || (contains(dep.RevisionHeads, urn) && targets[revision.Target.Target])
		}
		if !matched {
			continue
		}
		value, _ := snapshot.records.LoadOrStore(dep.Identity.ID, &contentRecord{})
		cached := value.(*contentRecord)
		cached.once.Do(func() {
			cached.rows = Records(ctx, snapshot.document, "", nil, Filter{ID: dep.Identity.ID})
		})
		rows = append(rows, cached.rows...)
	}
	return rows
}
