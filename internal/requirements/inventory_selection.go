package requirements

import (
	"context"
	"fmt"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/saga"
)

// SelectionCode is a stable reason a saved selection does not resolve.
type SelectionCode string

const (
	SelectionInvalid        SelectionCode = "invalid_selection"
	SelectionPathTooLong    SelectionCode = "path_too_long"
	SelectionMissingPin     SelectionCode = "missing_pin"
	SelectionUndeclared     SelectionCode = "undeclared_hop"
	SelectionNoEvidence     SelectionCode = "missing_evidence"
	SelectionOutside        SelectionCode = "outside_evidence"
	SelectionWholeFile      SelectionCode = "whole_file_selection"
	SelectionDigestMismatch SelectionCode = "selected_digest_mismatch"
)

// SelectionError reports the first failing hop (-1 for the whole selection).
type SelectionError struct {
	Code    SelectionCode
	Hop     int
	Message string
}

func (e *SelectionError) Error() string { return string(e.Code) + ": " + e.Message }

// SelectionResolution is the structural meaning of one saved selection: the
// exact pinned revision at every hop and the containing evidence reference.
type SelectionResolution struct {
	Hops      []*TechnicalRevision
	HopKinds  []string
	Evidence  Evidence
	OwnerEdge string // "" for the last hop's own code; else interaction/relationship ID
}

// ResolveSelection checks a selection against saved revisions only: each hop
// must exist exactly as pinned and be declared by the previous hop's saved
// revision, and the selected range must lie within the named evidence at the
// same commit and path. It never substitutes a newer revision, widens a
// range, or checks source bytes; code currency and the selected-byte digest
// are the source resolver's responsibility.
func (d *Inventory) ResolveSelection(documentation DocumentationLink, selection saga.ItemSelection) (SelectionResolution, error) {
	result := SelectionResolution{}
	fail := func(code SelectionCode, hop int, format string, args ...any) (SelectionResolution, error) {
		return SelectionResolution{}, &SelectionError{Code: code, Hop: hop, Message: fmt.Sprintf(format, args...)}
	}
	if len(selection.Path) > saga.MaxSelectionPath {
		return fail(SelectionPathTooLong, -1, "path has more than %d hops", saga.MaxSelectionPath)
	}
	if len(selection.Path) == 0 || selection.Path[0] != documentation {
		return fail(SelectionInvalid, -1, "path must start at the documentation pin")
	}
	for index, hop := range selection.Path {
		record := d.Find(hop.Target)
		if record == nil || record.Revision(hop.Revision) == nil {
			return fail(SelectionMissingPin, index, "%s is not a saved revision", hop.Revision)
		}
		if record.Kind != "component" && record.Kind != "system" && record.Kind != KindDataEntity {
			return fail(SelectionInvalid, index, "%s cannot be documented", hop.Target)
		}
		revision := record.Revision(hop.Revision)
		if index > 0 && !declaresHop(result.Hops[index-1], hop) {
			return fail(SelectionUndeclared, index, "%s is not declared by %s", hop.Revision, selection.Path[index-1].Revision)
		}
		result.Hops = append(result.Hops, revision)
		result.HopKinds = append(result.HopKinds, record.Kind)
	}
	last := result.Hops[len(result.Hops)-1]
	evidence, owner := last.EvidenceByID(selection.Evidence)
	if evidence == nil {
		return fail(SelectionNoEvidence, len(result.Hops)-1, "evidence %q is not in %s", selection.Evidence, selection.Path[len(selection.Path)-1].Revision)
	}
	code := selection.Code
	if code.WholeFile() {
		return fail(SelectionWholeFile, len(result.Hops)-1, "a selection must name an exact line range")
	}
	if code.Commit != evidence.Commit || code.Path != evidence.Path || code.Start < evidence.Start || code.End > evidence.End || code.Start > code.End {
		return fail(SelectionOutside, len(result.Hops)-1, "selected range must lie within evidence %q at the same commit and path", selection.Evidence)
	}
	result.Evidence, result.OwnerEdge = *evidence, owner
	return result, nil
}

// VerifySelectedBytes compares the saved selected-byte digest with the bytes
// at the selection's own commit, as authored by a source resolver (for example
// coderesolve.Resolver.Author). It checks the original selection, not currency.
func VerifySelectedBytes(ctx context.Context, author func(context.Context, coderef.Location, string) (coderef.Reference, error), selection saga.ItemSelection) error {
	authored, err := author(ctx, selection.Code.Location(), selection.Code.Note)
	if err != nil || authored.Digest != selection.Code.Digest {
		return &SelectionError{Code: SelectionDigestMismatch, Hop: -1, Message: fmt.Sprintf("selection %s digest does not match its bytes at %s", selection.ID, selection.Code.Commit)}
	}
	return nil
}

// declaresHop reports whether from's saved revision declares an exact link to
// next: a System member Component, a data-entity holder Component, or a
// relationship destination.
func declaresHop(from *TechnicalRevision, next DocumentationLink) bool {
	for _, pin := range from.Components {
		if pin == next {
			return true
		}
	}
	for _, holder := range from.Holders {
		if holder.Component == next {
			return true
		}
	}
	for _, edge := range from.Relationships {
		if edge.Destination == next {
			return true
		}
	}
	return false
}

// ComposeOverlay returns the baseline ERD directory without the overlay's
// proposed removals and with its pins substituted (same target) or added (new
// target), in baseline then overlay order. The canonical ERD is not modified.
func (d *Inventory) ComposeOverlay(overlay *TechnicalRevision) ([]DocumentationLink, error) {
	if overlay == nil || overlay.ERD == nil {
		return nil, fmt.Errorf("not an ERD overlay revision")
	}
	base := d.Pinned(*overlay.ERD)
	if base == nil {
		return nil, fmt.Errorf("overlay baseline %s is missing", overlay.ERD.Revision)
	}
	replacement := map[string]DocumentationLink{}
	for _, pin := range overlay.Pins {
		replacement[pin.Target] = pin
	}
	removed := map[string]bool{}
	for _, removal := range overlay.Removals {
		removed[removal.Target] = true
	}
	composed := make([]DocumentationLink, 0, len(base.Directory)+len(overlay.Pins))
	used := map[string]bool{}
	for _, pin := range base.Directory {
		if removed[pin.Target] {
			continue
		}
		if next, ok := replacement[pin.Target]; ok {
			pin = next
			used[pin.Target] = true
		}
		composed = append(composed, pin)
	}
	for _, pin := range overlay.Pins {
		if !used[pin.Target] {
			composed = append(composed, pin)
		}
	}
	return composed, nil
}
