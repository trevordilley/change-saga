package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewapp"
	"github.com/twentyideas/changesaga/internal/saga"
)

type inventoryCodeHealth struct {
	Owner      string                 `json:"owner"`
	Reference  string                 `json:"reference"`
	Resolution coderesolve.Resolution `json:"resolution"`
}
type inventoryLinkHealth struct {
	Owner  string                 `json:"owner"`
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
}

func queryInventory(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query inventory", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("saga", "", "app Saga root")
	repo := flags.String("repo", "", "source checkout")
	target := flags.String("target", "", "canonical Component/System URN")
	kind := flags.String("kind", "", "component or system")
	history := flags.Bool("history", false, "include immutable history for the selected target")
	head := flags.String("head", "HEAD", "source revision to observe")
	limit := flags.Int("limit", 50, "page size")
	cursor := flags.String("cursor", "", "snapshot-bound page cursor")
	fail := func(code string, err error) error {
		return writeQueryFailure(out, &queryError{Code: code, Message: err.Error()})
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return writeQuerySuccess(out, "", queryHelpFor("inventory"), nil)
		}
		return fail("invalid_argument", err)
	}
	if *root == "" || flags.NArg() != 0 || *limit < 1 || *limit > maxQueryPageSize || (*kind != "" && *kind != "component" && *kind != "system") || (*history && *target == "") {
		return fail("invalid_argument", fmt.Errorf("requires --saga; valid --kind and --limit; --history requires --target"))
	}
	manifest, err := saga.ReadManifest(*root)
	if err != nil {
		return fail("invalid_saga", err)
	}
	changes, err := gitdiff.ReadRange(ctx, firstNonEmpty(*repo, *root), manifest.Source.Repository, gitdiff.Range{Head: *head}, gitdiff.ReadOptions{})
	if err != nil {
		return fail("source_unavailable", err)
	}
	snapshot, err := reviewapp.Snapshot(ctx, *root, changes)
	if err != nil {
		return fail("internal", err)
	}
	d, err := requirements.LoadInventory(*root, manifest.ID)
	if err != nil {
		return fail("invalid_saga", err)
	}
	selected := []requirements.TechnicalRecord{}
	if *target != "" {
		if _, err := inventoryTarget(manifest.ID, "", *target); err != nil {
			return fail("invalid_argument", err)
		}
		if d.Find(*target) == nil {
			return fail("not_found", fmt.Errorf("target does not exist"))
		}
	}
	for _, r := range d.Records {
		if (*target == "" || r.Target == *target) && (*kind == "" || r.Kind == *kind) {
			selected = append(selected, r)
		}
	}
	key := "inventory:" + *kind + ":" + *target + fmt.Sprint(*history)
	start, cursorErr := decodeTermsCursor(*cursor, key, snapshot, len(selected))
	if cursorErr != nil {
		return writeQueryFailure(out, cursorErr)
	}
	end := min(start+*limit, len(selected))
	page := queryPageEnvelope{Total: len(selected), Returned: end - start, HasMore: end < len(selected)}
	if page.HasMore {
		next := encodeTermsCursor(key, snapshot, end)
		page.NextCursor = &next
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
		if rev := r.CurrentRevision; rev != nil {
			for _, ref := range rev.Code {
				entry.Code = append(entry.Code, inventoryCodeHealth{Owner: r.Target, Reference: ref.Location().String(), Resolution: resolver.Resolve(ctx, ref.Reference, changes.HeadOID)})
			}
			for _, edge := range rev.Interactions {
				for _, ref := range edge.Code {
					entry.Code = append(entry.Code, inventoryCodeHealth{Owner: r.Target + "#" + edge.ID, Reference: ref.Location().String(), Resolution: resolver.Resolve(ctx, ref.Reference, changes.HeadOID)})
				}
			}
			for _, pin := range rev.Components {
				entry.Links = append(entry.Links, inventoryLinkHealth{Owner: r.Target, Link: pin, Status: d.LinkStatus(pin)})
			}
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
	return writeQuerySuccess(out, snapshot, struct {
		Head    string           `json:"head_oid"`
		Records []inventoryEntry `json:"records"`
	}{changes.HeadOID, entries}, &page)
}
