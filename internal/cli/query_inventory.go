package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/inventoryview"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewapp"
	"github.com/twentyideas/changesaga/internal/saga"
)

type inventoryCodeHealth struct {
	Owner      string                 `json:"owner"`
	Evidence   string                 `json:"evidence,omitempty"`
	Intent     string                 `json:"intent,omitempty"`
	Reference  string                 `json:"reference"`
	Resolution coderesolve.Resolution `json:"resolution"`
}
type inventoryLinkHealth struct {
	Owner  string                 `json:"owner"`
	Role   string                 `json:"role,omitempty"`
	Link   saga.DocumentationLink `json:"link"`
	Status string                 `json:"status"`
}
type inventoryEntry struct {
	Kind             string                          `json:"kind"`
	Target           string                          `json:"target"`
	RevisionHeads    []string                        `json:"revision_heads"`
	LifecycleHeads   []string                        `json:"lifecycle_heads"`
	CurrentRevision  *requirements.TechnicalRevision `json:"current_revision"`
	CurrentLifecycle *requirements.TechnicalEvent    `json:"current_lifecycle"`
	History          *requirements.TechnicalRecord   `json:"history,omitempty"`
	Code             []inventoryCodeHealth           `json:"code_health"`
	Links            []inventoryLinkHealth           `json:"links"`
	inventoryReadFacts
}

func queryInventory(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query inventory", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("saga", "", "app Saga root")
	repo := flags.String("repo", "", "source checkout")
	target := flags.String("target", "", "canonical technical URN")
	kind := flags.String("kind", "", "component, system, data-entity, erd or erd-overlay")
	history := flags.Bool("history", false, "include immutable history for the selected target")
	head := flags.String("head", "HEAD", "source revision to observe")
	against := flags.String("against", "", "comparison baseline for newness")
	intent := flags.String("intent", "", "proposed, implemented or unspecified")
	onlyNew := flags.Bool("new", false, "only identities introduced since --against")
	feature := flags.String("feature", "", "feature ID or URN whose declared links scope the read")
	limit := flags.Int("limit", 50, "page size")
	cursor := flags.String("cursor", "", "snapshot-bound page cursor")
	conflictLimit := flags.Int("conflict-limit", 0, "unresolved page size")
	conflictCursor := flags.String("conflict-cursor", "", "snapshot-bound unresolved page cursor")
	fail := func(code string, err error) error {
		return writeQueryFailure(out, &queryError{Code: code, Message: err.Error()})
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return writeQuerySuccess(out, "", queryHelpFor("inventory"), nil)
		}
		return fail("invalid_argument", err)
	}
	if *conflictLimit == 0 {
		*conflictLimit = *limit
	}
	if *root == "" || flags.NArg() != 0 || *limit < 1 || *limit > maxQueryPageSize || *conflictLimit < 1 || *conflictLimit > maxQueryPageSize || (*kind != "" && !inventoryKind(*kind)) || (*history && *target == "") {
		return fail("invalid_argument", fmt.Errorf("requires --saga; valid --kind, --limit and --conflict-limit; --history requires --target"))
	}
	if *intent != "" && *intent != "proposed" && *intent != "implemented" && *intent != "unspecified" {
		return fail("invalid_argument", fmt.Errorf("--intent must be proposed, implemented or unspecified"))
	}
	if *onlyNew && *against == "" {
		return fail("invalid_argument", fmt.Errorf("--new requires a named comparison baseline (--against REV)"))
	}
	manifest, err := saga.ReadManifest(*root)
	if err != nil {
		return fail("invalid_saga", err)
	}
	featureID := ""
	if *feature != "" {
		id, ok := applayout.FeatureFromURN(manifest.ID, *feature)
		if !ok {
			return fail("invalid_argument", fmt.Errorf("--feature must be a feature ID or canonical URN"))
		}
		featureID = id
	}
	checkout := firstNonEmpty(*repo, *root)
	// Only the resolved endpoints matter here; the catalog avoids reading patches.
	catalog, err := gitdiff.ReadCatalogRange(ctx, checkout, manifest.Source.Repository, gitdiff.Range{Against: *against, Head: *head}, gitdiff.ReadOptions{})
	if err != nil {
		return fail("source_unavailable", err)
	}
	changes := gitdiff.ChangeSet{Mode: catalog.Mode, Repository: catalog.Repository, Base: catalog.Base, Head: catalog.Head, BaseOID: catalog.BaseOID, HeadOID: catalog.HeadOID}
	snapshot, err := reviewapp.Snapshot(ctx, *root, changes)
	if err != nil {
		return fail("internal", err)
	}
	d, err := requirements.LoadInventory(*root, manifest.ID)
	if err != nil {
		return fail("invalid_saga", err)
	}
	document, _, err := saga.Load(*root)
	if err != nil {
		return fail("invalid_saga", err)
	}
	if *target != "" {
		if err := inventoryQueryTarget(manifest.ID, *target); err != nil {
			return fail("invalid_argument", err)
		}
		if d.Find(*target) == nil && featureID == "" {
			return fail("not_found", fmt.Errorf("target does not exist"))
		}
	}
	read := inventoryRead{index: inventoryview.Build(document, &d), kind: *kind, target: *target, intent: *intent, onlyNew: *onlyNew}
	if featureID != "" {
		found := document.FindFeature(featureID)
		if found == nil {
			return fail("not_found", fmt.Errorf("feature does not exist"))
		}
		scope := read.index.FeatureScope(found)
		read.scope = &scope
	}
	if *against != "" {
		read.comparison = readInventoryBaseline(ctx, *root, checkout, manifest.ID, changes)
		if *onlyNew && !read.comparison.baseline.Known {
			return writeQueryFailure(out, &queryError{Code: "baseline_unknown", Message: "the Saga at the comparison baseline could not be read; newness is unknown, not every identity is new", Details: read.comparison.view()})
		}
	}
	matched, unresolved := read.partition()
	selected := []requirements.TechnicalRecord{}
	for _, m := range matched {
		selected = append(selected, *read.index.Record(m.Target))
	}
	key := "inventory:v2\x00" + *kind + "\x00" + *target + "\x00" + fmt.Sprint(*history) + "\x00" + *intent + "\x00" + fmt.Sprint(*onlyNew) + "\x00" + featureID + "\x00" + changes.BaseOID + "\x00" + changes.HeadOID
	start, cursorErr := decodeTermsCursor(*cursor, key+"\x00records", snapshot, len(selected))
	if cursorErr != nil {
		return writeQueryFailure(out, cursorErr)
	}
	end := min(start+*limit, len(selected))
	page := queryPageEnvelope{Total: len(selected), Returned: end - start, HasMore: end < len(selected)}
	if page.HasMore {
		next := encodeTermsCursor(key+"\x00records", snapshot, end)
		page.NextCursor = &next
	}
	unresolvedStart, cursorErr := decodeTermsCursor(*conflictCursor, key+"\x00unresolved", snapshot, len(unresolved))
	if cursorErr != nil {
		return writeQueryFailure(out, cursorErr)
	}
	unresolvedEnd := min(unresolvedStart+*conflictLimit, len(unresolved))
	unresolvedPage := queryPageEnvelope{Total: len(unresolved), Returned: unresolvedEnd - unresolvedStart, HasMore: unresolvedEnd < len(unresolved)}
	if unresolvedPage.HasMore {
		next := encodeTermsCursor(key+"\x00unresolved", snapshot, unresolvedEnd)
		unresolvedPage.NextCursor = &next
	}
	resolver, err := coderesolve.New(ctx, firstNonEmpty(*repo, *root))
	if err != nil {
		return fail("source_unavailable", err)
	}
	defer resolver.Close()
	entries := []inventoryEntry{}
	for _, r := range selected[start:end] {
		entry := inventoryEntry{Kind: r.Kind, Target: r.Target, RevisionHeads: r.RevisionHeads, LifecycleHeads: r.LifecycleHeads, CurrentRevision: r.CurrentRevision, CurrentLifecycle: r.CurrentLifecycle, Code: []inventoryCodeHealth{}, Links: []inventoryLinkHealth{}}
		if *history {
			copy := r
			entry.History = &copy
		}
		entry.inventoryReadFacts = matched[start+len(entries)].inventoryReadFacts
		entry.SelectedCode = selectedCodeHealth(ctx, resolver, r, entry.Selected, changes.HeadOID)
		if rev := r.CurrentRevision; rev != nil {
			entry.Code = inventoryCodeHealthOf(ctx, resolver, r.Target, rev, changes.HeadOID)
			entry.Links = inventoryLinksOf(read.index, r.Target, rev)
		}
		entries = append(entries, entry)
	}
	currentSnapshot, err := reviewapp.Snapshot(ctx, *root, changes)
	if err != nil {
		return fail("internal", err)
	}
	if currentSnapshot != snapshot {
		return fail("stale_snapshot", fmt.Errorf("inventory changed while reading; restart the query"))
	}
	return writeQuerySuccess(out, snapshot, inventoryReadData{
		Head: changes.HeadOID, Records: entries, Unresolved: unresolved[unresolvedStart:unresolvedEnd],
		Filters:      inventoryFilters{Kind: *kind, Target: *target, Intent: *intent, New: *onlyNew, Feature: featureID},
		Comparison:   read.comparison.view(),
		Scope:        read.scopeView(),
		Completeness: read.completeness(unresolvedPage),
	}, &page)
}
