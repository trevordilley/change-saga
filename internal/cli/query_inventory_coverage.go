package cli

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/inventoryview"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewapp"
	"github.com/twentyideas/changesaga/internal/saga"
)

type inventoryCoverageData struct {
	Head         string                        `json:"head_oid"`
	Scope        inventoryCoverageScope        `json:"scope"`
	Summary      inventoryview.CoverageSummary `json:"summary"`
	State        string                        `json:"state"`
	Entries      []any                         `json:"entries"`
	Completeness inventoryCoverageComplete     `json:"completeness"`
}

type inventoryCoverageScope struct {
	Paths    []string `json:"paths"`
	Kinds    []string `json:"kinds"`
	Excludes []string `json:"excludes"`
}

type inventoryCoverageComplete struct {
	Measures []string `json:"measures"`
	Limits   []string `json:"limits"`
}

// queryInventoryCoverage reports which code at one source revision the
// inventory's current definitions account for. It is separate from
// implementation-deck and review coverage and never a verdict.
func queryInventoryCoverage(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query inventory-coverage", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("saga", "", "app Saga root")
	repo := flags.String("repo", "", "source checkout")
	head := flags.String("head", "HEAD", "source revision to measure")
	kind := flags.String("kind", "", "component or system")
	state := flags.String("state", "ranges", "ranges, covered, uncovered, stale, unresolved or excluded")
	var paths stringList
	flags.Var(&paths, "path", "repository path prefix in scope; repeatable")
	limit := flags.Int("limit", 50, "page size")
	cursor := flags.String("cursor", "", "snapshot-bound page cursor")
	fail := func(code string, err error) error {
		return writeQueryFailure(out, &queryError{Code: code, Message: err.Error()})
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return writeQuerySuccess(out, "", queryHelpFor("inventory-coverage"), nil)
		}
		return fail("invalid_argument", err)
	}
	states := map[string]bool{"ranges": true, "covered": true, "uncovered": true, "stale": true, "unresolved": true, "excluded": true}
	if *root == "" || flags.NArg() != 0 || *limit < 1 || *limit > maxQueryPageSize || !states[*state] || (*kind != "" && *kind != "component" && *kind != "system") {
		return fail("invalid_argument", fmt.Errorf("requires --saga; valid --kind, --state and --limit"))
	}
	for _, p := range paths {
		if err := coderef.ValidatePath(strings.TrimSuffix(p, "/")); err != nil {
			return fail("invalid_argument", fmt.Errorf("--path %q: %w", p, err))
		}
	}
	manifest, err := saga.ReadManifest(*root)
	if err != nil {
		return fail("invalid_saga", err)
	}
	checkout := firstNonEmpty(*repo, *root)
	catalog, err := gitdiff.ReadCatalogRange(ctx, checkout, manifest.Source.Repository, gitdiff.Range{Head: *head}, gitdiff.ReadOptions{})
	if err != nil {
		return fail("source_unavailable", err)
	}
	changes := gitdiff.ChangeSet{Mode: catalog.Mode, Repository: catalog.Repository, Head: catalog.Head, BaseOID: catalog.BaseOID, HeadOID: catalog.HeadOID}
	snapshot, err := reviewapp.Snapshot(ctx, *root, changes)
	if err != nil {
		return fail("internal", err)
	}
	inventory, err := requirements.LoadInventory(*root, manifest.ID)
	if err != nil {
		return fail("invalid_saga", err)
	}
	files, err := listTrackedFiles(ctx, checkout, changes.HeadOID, paths)
	if err != nil {
		return fail("source_unavailable", err)
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return fail("source_unavailable", err)
	}
	defer resolver.Close()
	kinds := []string{}
	if *kind != "" {
		kinds = append(kinds, *kind)
	}
	report, err := inventoryview.Coverage(ctx, &inventory, inventoryview.CoverageInput{Head: changes.HeadOID, Paths: paths, Files: files, Kinds: kinds}, resolver)
	if err != nil {
		return writeQueryFailure(out, &queryError{Code: "scope_too_large", Message: err.Error()})
	}
	entries := []any{}
	switch *state {
	case "ranges", "covered", "uncovered":
		for _, r := range report.Ranges {
			if *state == "ranges" || r.State == *state {
				entries = append(entries, r)
			}
		}
	case "stale":
		for _, s := range report.Stale {
			entries = append(entries, s)
		}
	case "unresolved":
		for _, u := range report.Unresolved {
			entries = append(entries, u)
		}
	case "excluded":
		for _, u := range report.Excluded {
			entries = append(entries, u)
		}
	}
	key := "inventory-coverage:v1\x00" + changes.HeadOID + "\x00" + strings.Join(paths, "\x01") + "\x00" + *kind + "\x00" + *state
	start, cursorErr := decodeTermsCursor(*cursor, key, snapshot, len(entries))
	if cursorErr != nil {
		return writeQueryFailure(out, cursorErr)
	}
	end := min(start+*limit, len(entries))
	page := queryPageEnvelope{Total: len(entries), Returned: end - start, HasMore: end < len(entries)}
	if page.HasMore {
		next := encodeTermsCursor(key, snapshot, end)
		page.NextCursor = &next
	}
	after, err := reviewapp.Snapshot(ctx, *root, changes)
	if err != nil {
		return fail("internal", err)
	}
	if after != snapshot {
		return fail("stale_snapshot", fmt.Errorf("the Saga changed while reading; restart the query"))
	}
	return writeQuerySuccess(out, snapshot, inventoryCoverageData{
		Head: changes.HeadOID, State: *state, Summary: report.Summary, Entries: entries[start:end],
		Scope: inventoryCoverageScope{Paths: append([]string{}, paths...), Kinds: kinds, Excludes: []string{"Saga directories", "binary files (counted, not measured)", "untracked and uncommitted files"}},
		Completeness: inventoryCoverageComplete{
			Measures: []string{"lines of tracked files at head_oid inside scope", "only the unique current revision of each active Component/System; each line counts once and keeps every owner", "System interaction evidence as owner#interaction"},
			Limits:   []string{"This is inventory coverage only: implementation-deck and review coverage have separate denominators.", "Stale references account for nothing; retired, proposed and conflicted owners are listed with reasons, not counted.", "Covered lines prove an explicit reference, not a correct or complete explanation."},
		},
	}, &page)
}

// listTrackedFiles lists regular tracked files at commit under the prefixes,
// excluding Saga directories.
func listTrackedFiles(ctx context.Context, checkout, commit string, prefixes []string) ([]string, error) {
	args := []string{"-C", checkout, "ls-tree", "-r", "-z", "--full-tree", commit, "--"}
	args = append(args, prefixes...)
	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list files at %s: %s", commit, strings.TrimSpace(stderr.String()))
	}
	files := []string{}
	for _, entry := range strings.Split(string(output), "\x00") {
		meta, path, ok := strings.Cut(entry, "\t")
		if !ok || !strings.HasPrefix(meta, "100") || gitdiff.IsSagaPath(path) {
			continue
		}
		files = append(files, path)
		if len(files) > inventoryview.MaxCoverageFiles {
			break
		}
	}
	sort.Strings(files)
	return files, nil
}
