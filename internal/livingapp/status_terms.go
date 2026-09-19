package livingapp

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/vocabulary"
)

// OverviewStatus reports the overview's four parts. The name is saga.json's
// title and is always present; the pitch, description, and terms are
// optional, and each absent one is listed in Gaps. A gap is a report, never
// an error, and never part of a readiness gate.
type OverviewStatus struct {
	Name        string        `json:"name"`
	Pitch       *OverviewPart `json:"pitch"`
	Description *OverviewPart `json:"description"`
	// Terms counts the active terms.
	Terms int      `json:"terms"`
	Gaps  []string `json:"gaps"`
}

// OverviewPart is one written overview fragment.
type OverviewPart struct {
	Target string `json:"target"`
	Path   string `json:"path"`
}

// TermStatus is one term, the stories and records it names, and the health
// of the code it references, viewed at the head. A stale reference names
// exactly the term to update after a rename.
type TermStatus struct {
	Term            string     `json:"term"`
	ID              string     `json:"id"`
	Name            string     `json:"name,omitempty"`
	Definition      string     `json:"definition,omitempty"`
	Aliases         []string   `json:"aliases"`
	State           string     `json:"state"`
	Stories         []string   `json:"stories"`
	Records         []string   `json:"records"`
	Code            []TermCode `json:"code"`
	Stale           bool       `json:"stale"`
	RevisionHeads   []string   `json:"revision_heads"`
	LifecycleHeads  []string   `json:"lifecycle_heads"`
	CurrentRevision string     `json:"current_revision,omitempty"`
	LifecycleHead   string     `json:"lifecycle_head,omitempty"`
}

// TermCode is one of a term's code references viewed at the head: current
// (at Head, which differs from Pinned when the lines only moved) or stale
// with the reason.
type TermCode struct {
	Pinned coderef.Location  `json:"pinned"`
	State  coderesolve.State `json:"state"`
	Head   *coderef.Location `json:"head,omitempty"`
	Moved  bool              `json:"moved,omitempty"`
	Reason string            `json:"reason,omitempty"`
}

// TermSuggestion is a growth suggestion from a comparison: a declaration the
// change added that looks like new vocabulary (an enum member or a constant
// of a named type) and that no term references or names. It is never a gap
// and never blocks.
type TermSuggestion struct {
	Name string `json:"name"`
	// Suggested is the domain word the identifier spells, the name to offer
	// the author: ResolutionExcluded in Resolution suggests "excluded". Name
	// stays the identifier the term's code reference points at.
	Suggested string           `json:"suggested_name"`
	Container string           `json:"container,omitempty"`
	Kind      vocabulary.Kind  `json:"kind"`
	Language  string           `json:"language"`
	Location  coderef.Location `json:"location"`
}

// Blobs reads file content at a commit. *coderesolve.Resolver implements it.
type Blobs interface {
	Blob(ctx context.Context, commit, path string) ([]byte, bool, error)
}

// overviewStatus projects the overview's parts.
func overviewStatus(doc *saga.Saga, terms []requirements.Term) OverviewStatus {
	status := OverviewStatus{Gaps: []string{}}
	if doc == nil {
		return status
	}
	status.Name = doc.Manifest.Title
	part := func(name string) *OverviewPart {
		if fragment := doc.OverviewPart(name); fragment != nil {
			return &OverviewPart{Target: fragment.Target, Path: fragment.Path}
		}
		return nil
	}
	status.Pitch, status.Description = part(applayout.OverviewPitch), part(applayout.OverviewDescription)
	for _, term := range terms {
		if term.Active() {
			status.Terms++
		}
	}
	if status.Pitch == nil {
		status.Gaps = append(status.Gaps, "pitch")
	}
	if status.Description == nil {
		status.Gaps = append(status.Gaps, "description")
	}
	if status.Terms == 0 {
		status.Gaps = append(status.Gaps, "terms")
	}
	return status
}

// resolveTermCode views every current term reference at the head.
func resolveTermCode(ctx context.Context, terms []requirements.Term, changes gitdiff.ChangeSet, resolver coverage.Resolver) map[string]coderesolve.Resolution {
	result := map[string]coderesolve.Resolution{}
	for _, term := range terms {
		if term.CurrentRevision == nil {
			continue
		}
		for _, reference := range term.CurrentRevision.Code {
			if resolver == nil || changes.HeadOID == "" {
				result[reference.Key()] = coderesolve.Resolution{State: coderesolve.Stale, Location: reference.Location(), Reason: "no source repository is available to resolve the reference"}
				continue
			}
			result[reference.Key()] = resolver.Resolve(ctx, reference, changes.HeadOID)
		}
	}
	return result
}

// termStatuses projects every term and its code health.
func termStatuses(sagaID string, terms []requirements.Term, code map[string]coderesolve.Resolution) []TermStatus {
	result := []TermStatus{}
	for _, term := range terms {
		urn, _ := requirements.TermURN(sagaID, term.Identity.ID)
		row := TermStatus{
			Term: urn, ID: term.Identity.ID, State: "conflicted", Aliases: []string{}, Stories: []string{}, Records: []string{}, Code: []TermCode{},
			RevisionHeads: copyStrings(term.RevisionHeads), LifecycleHeads: copyStrings(term.LifecycleHeads),
		}
		if revision := term.CurrentRevision; revision != nil {
			row.Name, row.Definition = revision.Name, revision.Definition
			row.Aliases = append(row.Aliases, revision.Aliases...)
			row.Stories = append(row.Stories, revision.Stories...)
			row.Records = append(row.Records, revision.Records...)
			row.CurrentRevision, _ = requirements.TermRevisionURN(sagaID, term.Identity.ID, revision.ID)
			for _, reference := range revision.Code {
				resolution := code[reference.Key()]
				value := TermCode{Pinned: reference.Location(), State: coderesolve.Stale, Reason: resolution.Reason}
				if resolution.Current() {
					location := resolution.Location
					value = TermCode{Pinned: reference.Location(), State: coderesolve.Current, Head: &location, Moved: resolution.Moved}
				} else if value.Reason == "" {
					value.Reason = "the reference could not be resolved at the head"
				}
				row.Stale = row.Stale || value.State == coderesolve.Stale
				row.Code = append(row.Code, value)
			}
		}
		if term.CurrentLifecycle != nil {
			row.State = string(term.CurrentLifecycle.State)
			row.LifecycleHead, _ = requirements.TermEventURN(sagaID, term.Identity.ID, term.CurrentLifecycle.ID)
		}
		result = append(result, row)
	}
	return result
}

// SuggestTerms finds the declarations a comparison added that look like new
// vocabulary and that no term references or names. Only added lines count,
// a declaration whose container and name already existed at the base is not
// new, and a declaration that replaced one a term referenced is left to that
// term's stale reference, which already asks for the rename to be recorded.
func SuggestTerms(ctx context.Context, terms []requirements.Term, changes gitdiff.ChangeSet, resolver coverage.Resolver, blobs Blobs) []TermSuggestion {
	result := []TermSuggestion{}
	if changes.Mode != gitdiff.ModeCompare || blobs == nil || changes.BaseOID == changes.HeadOID {
		return result
	}
	added := map[string]map[int]bool{}
	for _, atom := range changes.Atoms {
		if atom.Kind == "line" && atom.Side == "new" && vocabulary.Supported(atom.Path) {
			if added[atom.Path] == nil {
				added[atom.Path] = map[int]bool{}
			}
			added[atom.Path][atom.Line] = true
		}
	}
	if len(added) == 0 {
		return result
	}
	spellings := map[string]bool{}
	var references []coderef.Reference
	for _, term := range terms {
		if term.CurrentRevision == nil {
			continue
		}
		for _, spelling := range append([]string{term.CurrentRevision.Name}, term.CurrentRevision.Aliases...) {
			spellings[normalizeSpelling(spelling)] = true
		}
		references = append(references, term.CurrentRevision.Code...)
	}
	paths := make([]string, 0, len(added))
	for path := range added {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		head, found, err := blobs.Blob(ctx, changes.HeadOID, path)
		if err != nil || !found {
			continue
		}
		declarations := vocabulary.Detect(path, head)
		if len(declarations) == 0 {
			continue
		}
		base, _, _ := blobs.Blob(ctx, changes.BaseOID, path)
		baseDeclarations := vocabulary.Detect(path, base)
		existed := map[string]bool{}
		for _, declaration := range baseDeclarations {
			existed[declaration.Container+"\x00"+declaration.Name] = true
		}
		headNames := map[string]bool{}
		for _, declaration := range declarations {
			headNames[declaration.Container+"\x00"+declaration.Name] = true
		}
		// Containers where a termed declaration was renamed or removed: the
		// term's stale reference covers what replaced it.
		renamed := map[string]bool{}
		var current []coderef.Location
		for _, reference := range references {
			if resolver == nil {
				continue
			}
			if at := resolver.Resolve(ctx, reference, changes.HeadOID); at.Current() && at.Location.Path == path {
				current = append(current, at.Location)
				continue
			}
			at := resolver.Resolve(ctx, reference, changes.BaseOID)
			if !at.Current() || at.Location.Path != path {
				continue
			}
			for _, declaration := range baseDeclarations {
				inside := at.Location.WholeFile() || declaration.Line >= at.Location.Start && declaration.Line <= at.Location.End
				if inside && !headNames[declaration.Container+"\x00"+declaration.Name] {
					renamed[declaration.Container] = true
				}
			}
		}
		for _, declaration := range declarations {
			switch {
			case !added[path][declaration.Line]:
			case existed[declaration.Container+"\x00"+declaration.Name]:
			case renamed[declaration.Container]:
			case spellings[normalizeSpelling(declaration.Name)] || spellings[normalizeSpelling(strings.TrimPrefix(declaration.Name, declaration.Container))]:
			case referenced(current, declaration.Line):
			default:
				result = append(result, TermSuggestion{
					Name: declaration.Name, Suggested: vocabulary.Humanize(declaration.Name, declaration.Container),
					Container: declaration.Container, Kind: declaration.Kind, Language: declaration.Language,
					Location: coderef.Location{Commit: changes.HeadOID, Path: path, Start: declaration.Line, End: declaration.Line},
				})
			}
		}
	}
	return result
}

func referenced(locations []coderef.Location, line int) bool {
	for _, location := range locations {
		if location.WholeFile() || line >= location.Start && line <= location.End {
			return true
		}
	}
	return false
}

// normalizeSpelling folds case and drops every non-alphanumeric character, so
// "Test taker", "test_taker", and "TESTTAKER" are one spelling.
func normalizeSpelling(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
		}
	}
	return builder.String()
}

// TermStatuses projects terms with their code viewed at the head of changes.
func TermStatuses(ctx context.Context, sagaID string, terms []requirements.Term, changes gitdiff.ChangeSet, resolver coverage.Resolver) []TermStatus {
	return termStatuses(sagaID, terms, resolveTermCode(ctx, terms, changes, resolver))
}
