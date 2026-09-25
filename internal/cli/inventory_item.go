package cli

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/savedview"
	"github.com/twentyideas/changesaga/internal/technicalpolicy"
)

// itemInventoryLinks is one Item's requested documentation pin, optional
// saved view and selections. Previous is the Item's current manifest, if any:
// an unchanged pin/view is retained honestly and unchanged selections are not
// re-authored, so old pins survive definition succession.
type itemInventoryLinks struct {
	Documentation *saga.DocumentationLink
	View          string
	Selections    []saga.ItemSelection
	Previous      *saga.ItemManifest
	Checkout      string
}

// prepareItemInventoryLinks admits the documentation pin (current by default,
// or through an explicit saved view), authors each new selection's selected
// bytes at its own commit, and checks every selection structurally against the
// saved revisions. It returns the resolved view commit and the selections to
// persist. Nothing is written.
func prepareItemInventoryLinks(ctx context.Context, root string, manifest saga.Manifest, links itemInventoryLinks) (string, []saga.ItemSelection, error) {
	if links.Documentation == nil && (links.View != "" || len(links.Selections) > 0) {
		return "", nil, fmt.Errorf("a saved view and selections require a documentation pin")
	}
	var previous saga.ItemManifest
	if links.Previous != nil {
		previous = *links.Previous
	}
	if links.Documentation == nil {
		return "", nil, nil
	}
	d, err := requirements.LoadInventory(root, manifest.ID)
	if err != nil {
		return "", nil, err
	}
	checkout := firstNonEmpty(links.Checkout, root)
	requested := defaultSelectionCommits(&d, links.Selections)
	if err := precheckSelections(&d, *links.Documentation, requested); err != nil {
		return "", nil, err
	}
	viewOID, selections, err := authorItemLinks(ctx, checkout, manifest, links.View, requested, previous.Selections)
	if err != nil {
		return "", nil, err
	}
	if err := admitItemLinks(ctx, &d, root, checkout, manifest, saga.ItemManifest{Documentation: links.Documentation, DocumentationView: viewOID, Selections: selections}, links.Previous); err != nil {
		return "", nil, err
	}
	return viewOID, selections, nil
}

// authorItemLinks resolves a symbolic view once to a full commit and authors
// every selection not retained unchanged from previous: symbolic commits are
// resolved and the digest of exactly the selected bytes is computed (or a
// supplied digest verified). It needs the canonical source checkout.
func authorItemLinks(ctx context.Context, checkout string, manifest saga.Manifest, view string, requested, previous []saga.ItemSelection) (string, []saga.ItemSelection, error) {
	needsSource := view != ""
	for _, selection := range requested {
		needsSource = needsSource || !containsSelection(previous, selection)
	}
	if !needsSource {
		return view, requested, nil
	}
	if _, err := gitdiff.ReadCatalogRange(ctx, checkout, manifest.Source.Repository, gitdiff.Range{Head: "HEAD"}, gitdiff.ReadOptions{}); err != nil {
		return "", nil, err
	}
	viewOID := ""
	if view != "" {
		oid, err := resolveCommit(ctx, checkout, view)
		if err != nil {
			return "", nil, &savedview.Error{Reason: savedview.CommitUnavailable, Message: err.Error()}
		}
		viewOID = oid
	}
	if len(requested) == 0 {
		return viewOID, nil, nil
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return "", nil, err
	}
	defer resolver.Close()
	selections := make([]saga.ItemSelection, 0, len(requested))
	for _, selection := range requested {
		if !containsSelection(previous, selection) {
			location, err := resolveLocation(ctx, checkout, selection.Code.Location().String())
			if err != nil {
				return "", nil, fmt.Errorf("selection %s: %w", selection.ID, err)
			}
			authored, err := resolver.Author(ctx, location, selection.Code.Note)
			if err != nil {
				return "", nil, fmt.Errorf("selection %s: %w", selection.ID, err)
			}
			if selection.Code.Digest != "" && selection.Code.Digest != authored.Digest {
				return "", nil, &requirements.SelectionError{Code: requirements.SelectionDigestMismatch, Hop: -1, Message: "selection " + selection.ID + " digest does not match its bytes"}
			}
			selection.Code = authored
		}
		selections = append(selections, selection)
	}
	return viewOID, selections, nil
}

// admitItemLinks checks an authored Item's pin, view and selections against
// the inventory without writing. An unchanged pin/view pair is retained as-is
// (it was admitted when authored); anything new must be admitted now.
func admitItemLinks(ctx context.Context, d *requirements.Inventory, root, checkout string, manifest saga.Manifest, item saga.ItemManifest, previous *saga.ItemManifest) error {
	pin := item.Documentation
	if pin == nil {
		return nil
	}
	if !saga.ValidDocumentationLink(manifest.ID, *pin) {
		return fmt.Errorf("documentation requires a canonical Component, System or data-entity target and its revision URN")
	}
	if (item.DocumentationView != "" || len(item.Selections) > 0 || strings.Contains(pin.Target, ":"+requirements.KindDataEntity+":")) && d.Format < 2 {
		return requirements.ErrFormatRequired
	}
	retained := previous != nil && previous.Documentation != nil && *previous.Documentation == *pin && previous.DocumentationView == item.DocumentationView
	if !retained {
		if err := admitDocumentationPin(ctx, d, root, checkout, manifest, *pin, item.DocumentationView); err != nil {
			return err
		}
	}
	for _, selection := range item.Selections {
		if _, err := d.ResolveSelection(*pin, selection); err != nil {
			return fmt.Errorf("selection %s: %w", selection.ID, err)
		}
	}
	if problems := saga.ItemSelectionProblems(manifest.ID, item); len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// defaultSelectionCommits lets a selection omit its commit: it then names the
// containing evidence's own commit, the only commit a subset may use.
func defaultSelectionCommits(d *requirements.Inventory, selections []saga.ItemSelection) []saga.ItemSelection {
	out := append([]saga.ItemSelection(nil), selections...)
	for i := range out {
		if out[i].Code.Commit != "" || len(out[i].Path) == 0 {
			continue
		}
		if revision := d.Pinned(out[i].Path[len(out[i].Path)-1]); revision != nil {
			if evidence, _ := revision.EvidenceByID(out[i].Evidence); evidence != nil {
				out[i].Code.Commit = evidence.Commit
			}
		}
	}
	return out
}

// precheckSelections reports path and evidence failures before any source is
// read, so a wrong hop or evidence ID is named rather than a bytes error. Range
// containment is checked after symbolic commits are authored.
func precheckSelections(d *requirements.Inventory, pin saga.DocumentationLink, selections []saga.ItemSelection) error {
	for _, selection := range selections {
		_, err := d.ResolveSelection(pin, selection)
		var selectionErr *requirements.SelectionError
		if errors.As(err, &selectionErr) && selectionErr.Code != requirements.SelectionOutside && selectionErr.Code != requirements.SelectionWholeFile {
			return fmt.Errorf("selection %s: %w", selection.ID, err)
		}
	}
	return nil
}

func containsSelection(values []saga.ItemSelection, value saga.ItemSelection) bool {
	for _, existing := range values {
		if reflect.DeepEqual(existing, value) {
			return true
		}
	}
	return false
}

// admitDocumentationPin applies technicalpolicy.AdmitPin: without a view only
// the current revision of an active, unconflicted record is admitted; with a
// view the exact pin must be that view's unambiguous active binding.
func admitDocumentationPin(ctx context.Context, d *requirements.Inventory, root, checkout string, manifest saga.Manifest, pin saga.DocumentationLink, viewOID string) error {
	request := technicalpolicy.PinRequest{Requested: technicalpolicy.Pin{Target: pin.Target, Revision: pin.Revision}, Global: savedview.GlobalHealth(d, pin.Target)}
	if viewOID != "" {
		view, err := savedview.Load(ctx, checkout, root, manifest, viewOID)
		if err != nil {
			return err
		}
		facts := view.Facts(pin.Target)
		request.View = &facts
	}
	admission := technicalpolicy.AdmitPin(request)
	if !admission.Admitted {
		if viewOID == "" {
			return fmt.Errorf("documentation target %s is %s; new pins must be current unless admitted by --documentation-view (%s)", pin.Target, d.LinkStatus(pin), admission.Reason)
		}
		return fmt.Errorf("documentation pin %s is not admitted by saved view %s: %s", pin.Revision, viewOID, admission.Reason)
	}
	return nil
}

// readItemSelections reads a strict JSON array of selections ("" means none).
// Code commits may be symbolic and digests may be omitted; they are authored.
func readItemSelections(path string) ([]saga.ItemSelection, error) {
	if path == "" {
		return nil, nil
	}
	data, err := readBoundedInput(path, 1<<20, "selections")
	if err != nil {
		return nil, err
	}
	selections := []saga.ItemSelection{}
	if err := requirements.DecodeInventoryJSON(data, &selections); err != nil {
		return nil, fmt.Errorf("selections: %w", err)
	}
	if len(selections) == 0 {
		return nil, nil
	}
	return selections, nil
}
