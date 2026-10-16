package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/deprecationview"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewapp"
	"github.com/twentyideas/changesaga/internal/saga"
)

func queryDeprecations(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query deprecations", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("saga", "", "Saga root")
	repo := flags.String("repo", "", "code checkout")
	id := flags.String("deprecation", "", "annotation ID or URN")
	target := flags.String("target", "", "exact target URN, including a removed target")
	state := flags.String("state", "all", "active, withdrawn, conflicted or all")
	scope := flags.String("scope", "", "content or code")
	story := flags.String("story", "", "related story ID or URN, including retired stories")
	ref := flags.String("ref", "", "code location at any revision")
	head := flags.String("head", "HEAD", "source revision for evidence health")
	history := flags.Bool("history", false, "include immutable history; requires --deprecation")
	limit := flags.Int("limit", 50, "page size")
	cursor := flags.String("cursor", "", "snapshot-bound cursor")
	fail := func(code string, err error) error {
		return writeQueryFailure(out, &queryError{Code: code, Message: err.Error()})
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return writeQuerySuccess(out, "", queryHelpFor("deprecations"), nil)
		}
		return fail("invalid_argument", err)
	}
	if *root == "" || flags.NArg() != 0 || *limit < 1 || *limit > maxQueryPageSize || (*history && *id == "") {
		return fail("invalid_argument", fmt.Errorf("requires --saga and --limit 1..1000; --history requires --deprecation"))
	}
	if *state != "all" && *state != "active" && *state != "withdrawn" && *state != "conflicted" {
		return fail("invalid_argument", fmt.Errorf("invalid --state"))
	}
	if *scope != "" && *scope != "content" && *scope != "code" {
		return fail("invalid_argument", fmt.Errorf("invalid --scope"))
	}
	manifest, err := saga.ReadManifest(*root)
	if err != nil {
		return fail("invalid_saga", err)
	}
	doc, err := requirements.Load(*root, manifest.ID)
	if err != nil {
		return fail("invalid_saga", err)
	}
	if *id != "" {
		selected := *id
		if !livingid.ValidID(selected) {
			selected, err = requirements.ParseDeprecationURN(manifest.ID, selected)
			if err != nil {
				return fail("invalid_argument", err)
			}
		}
		if doc.FindDeprecation(selected) == nil {
			return fail("not_found", fmt.Errorf("deprecation does not exist"))
		}
	}
	if *target != "" {
		if _, err = requirements.DeprecationTargetKind(manifest.ID, *target); err != nil {
			return fail("invalid_argument", err)
		}
	}
	if *story != "" {
		normalized, e := exactStoryURN(manifest.ID, *story)
		if e != nil {
			return writeQueryFailure(out, e)
		}
		*story = normalized
	}
	checkout := firstNonEmpty(*repo, *root)
	changes := gitdiff.ChangeSet{}
	// Global/target discovery remains usable without a source checkout. Code
	// inspection requires a repository and never hides a failed source read.
	catalog, sourceErr := gitdiff.ReadCatalogRange(ctx, checkout, manifest.Source.Repository, gitdiff.Range{Head: *head}, gitdiff.ReadOptions{})
	var resolver *coderesolve.Resolver
	if sourceErr == nil {
		changes = gitdiff.ChangeSet{Mode: catalog.Mode, Repository: catalog.Repository, Head: catalog.Head, HeadOID: catalog.HeadOID}
		resolver, sourceErr = coderesolve.New(ctx, checkout)
	}
	if resolver != nil {
		defer resolver.Close()
	}
	if sourceErr != nil && *ref != "" {
		return fail("source_unavailable", sourceErr)
	}
	var location *coderef.Location
	if *ref != "" {
		value, e := resolveLocation(ctx, checkout, *ref)
		if e != nil {
			return fail("invalid_argument", e)
		}
		location = &value
	}
	snapshot, err := reviewapp.Snapshot(ctx, *root, changes)
	if err != nil {
		return fail("internal", err)
	}
	var evidenceResolver deprecationview.Resolver
	if resolver != nil {
		evidenceResolver = resolver
	}
	filter := deprecationview.Filter{ID: *id, Target: *target, State: *state, Scope: *scope, Story: *story, Ref: location, History: *history}
	rows := deprecationview.Records(ctx, doc, changes.HeadOID, evidenceResolver, filter)
	keyBytes, _ := json.Marshal(filter)
	key := "deprecations:v1:" + string(keyBytes)
	start, e := decodeTermsCursor(*cursor, key, snapshot, len(rows))
	if e != nil {
		return writeQueryFailure(out, e)
	}
	end := min(start+*limit, len(rows))
	page := queryPageEnvelope{Total: len(rows), Returned: end - start, HasMore: end < len(rows)}
	if page.HasMore {
		next := encodeTermsCursor(key, snapshot, end)
		page.NextCursor = &next
	}
	data := map[string]any{"head_oid": changes.HeadOID, "deprecations": rows[start:end], "absence_meaning": "no recorded deprecation; not proof of non-deprecation", "scope_rule": "content and its related code are not deprecated code; only explicit code-scope references assert code deprecation"}
	if sourceErr != nil {
		data["source_warning"] = sourceErr.Error()
	}
	if location != nil {
		data["ref"] = location.String()
	}
	return writeQuerySuccess(out, snapshot, data, &page)
}
