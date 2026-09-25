package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/changeview"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/inventoryview"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewapp"
	"github.com/twentyideas/changesaga/internal/saga"
)

// inventoryReadFacts are the explicit, separately derived facts a summary
// reports: the selected revisions and their intent, identity newness relative
// to a named comparison, declared scope paths and declared use counts.
type inventoryReadFacts struct {
	Selected   []inventoryview.SelectedPin `json:"selected"`
	Newness    string                      `json:"newness,omitempty"`
	ScopePaths []inventoryview.ScopePath   `json:"scope_paths,omitempty"`
	Uses       inventoryview.UseCounts     `json:"uses"`
	// SelectedCode is code health of selected revisions other than the
	// current one; code_health always describes the current revision.
	SelectedCode []inventoryCodeHealth `json:"selected_code_health,omitempty"`
}

type inventoryCandidate struct {
	Target string
	Kind   string
	inventoryReadFacts
}

type inventoryUnresolved struct {
	Target     string                      `json:"target"`
	Kind       string                      `json:"kind,omitempty"`
	Reasons    []string                    `json:"reasons"`
	Heads      []string                    `json:"revision_heads"`
	Lifecycle  []string                    `json:"lifecycle_heads"`
	Selected   []inventoryview.SelectedPin `json:"selected"`
	ScopePaths []inventoryview.ScopePath   `json:"scope_paths,omitempty"`
}

type inventoryFilters struct {
	Kind    string `json:"kind,omitempty"`
	Target  string `json:"target,omitempty"`
	Intent  string `json:"intent,omitempty"`
	New     bool   `json:"new"`
	Feature string `json:"feature,omitempty"`
}

type inventoryComparisonView struct {
	Against     string                  `json:"against"`
	BaseOID     string                  `json:"base_oid"`
	HeadOID     string                  `json:"head_oid"`
	Baseline    string                  `json:"baseline"` // known | absent | unknown
	SagaCommit  string                  `json:"saga_commit,omitempty"`
	Diagnostics []changeview.Diagnostic `json:"diagnostics"`
}

type inventoryScopeView struct {
	Feature   string `json:"feature"`
	Targets   int    `json:"targets"`
	Truncated bool   `json:"truncated"`
	CycleCut  bool   `json:"cycle_cut"`
}

type inventoryCompleteness struct {
	UnresolvedPage queryPageEnvelope `json:"unresolved_page"`
	Includes       []string          `json:"includes"`
	Excludes       []string          `json:"excludes"`
}

type inventoryReadData struct {
	Head         string                   `json:"head_oid"`
	Records      []inventoryEntry         `json:"records"`
	Unresolved   []inventoryUnresolved    `json:"unresolved"`
	Filters      inventoryFilters         `json:"filters"`
	Comparison   *inventoryComparisonView `json:"comparison,omitempty"`
	Scope        *inventoryScopeView      `json:"scope,omitempty"`
	Completeness inventoryCompleteness    `json:"completeness"`
}

type inventoryComparison struct {
	requested   bool
	baseline    inventoryview.Baseline
	changes     gitdiff.ChangeSet
	sagaCommit  string
	diagnostics []changeview.Diagnostic
}

func (c inventoryComparison) view() *inventoryComparisonView {
	if !c.requested {
		return nil
	}
	state := "unknown"
	if c.baseline.Known {
		state = "known"
		if c.baseline.Absent {
			state = "absent"
		}
	}
	return &inventoryComparisonView{Against: c.changes.Base, BaseOID: c.changes.BaseOID, HeadOID: c.changes.HeadOID, Baseline: state, SagaCommit: c.sagaCommit, Diagnostics: append([]changeview.Diagnostic{}, c.diagnostics...)}
}

// readInventoryBaseline reads the inventory of the Saga snapshot documenting
// the comparison base. Any failure leaves the baseline unknown.
func readInventoryBaseline(ctx context.Context, root, checkout, sagaID string, changes gitdiff.ChangeSet) inventoryComparison {
	result := inventoryComparison{requested: true, changes: changes, baseline: inventoryview.Baseline{Commit: changes.BaseOID}}
	side, diagnostics := changeview.BaseSide(ctx, root, checkout, changes)
	result.diagnostics = diagnostics
	if side.Source == "" {
		result.diagnostics = append(result.diagnostics, changeview.Diagnostic{Code: "baseline_unknown", Message: "no Saga snapshot documents the comparison base"})
		return result
	}
	result.sagaCommit = side.Commit
	err := changeview.ReadSnapshot(ctx, root, side, func(snapshot string) error {
		manifest, err := saga.ReadManifest(snapshot)
		if err != nil {
			return err
		}
		if manifest.ID != sagaID {
			return fmt.Errorf("the baseline Saga has identity %q, not %q", manifest.ID, sagaID)
		}
		inventory, err := requirements.LoadInventory(snapshot, sagaID)
		if err != nil {
			return err
		}
		result.baseline.Inventory = &inventory
		return nil
	})
	switch {
	case err == nil:
		result.baseline.Known = true
	case changeview.IsSagaAbsent(err):
		result.baseline.Known, result.baseline.Absent = true, true
	default:
		result.diagnostics = append(result.diagnostics, changeview.Diagnostic{Code: "baseline_unknown", Message: err.Error()})
	}
	return result
}

type inventoryRead struct {
	index      *inventoryview.Index
	scope      *inventoryview.Scope
	comparison inventoryComparison
	kind       string
	target     string
	intent     string
	onlyNew    bool
}

func (r inventoryRead) scopeView() *inventoryScopeView {
	if r.scope == nil {
		return nil
	}
	return &inventoryScopeView{Feature: r.scope.Feature, Targets: len(r.scope.Entries), Truncated: r.scope.Truncated, CycleCut: r.scope.CycleCut}
}

func (r inventoryRead) completeness(page queryPageEnvelope) inventoryCompleteness {
	c := inventoryCompleteness{UnresolvedPage: page,
		Includes: []string{"Component and System records selected by kind/target", "explicit revision intent (legacy revisions are unspecified)", "declared uses from implementation and review deck Items and technical owners"},
		Excludes: []string{"undeclared dependencies in prose, SVG text or code", "unresolved records are never removed by intent or newness filters; read the unresolved page"},
	}
	if r.scope != nil {
		c.Includes = append(c.Includes, "feature scope through declared Item pins and member pins at each saved revision")
	}
	if r.comparison.requested {
		c.Includes = append(c.Includes, "identity newness relative to the named comparison base")
	}
	return c
}

// partition returns matching resolvable records in inventory order and every
// unresolved candidate in target order. Filters apply only to the former.
func (r inventoryRead) partition() ([]inventoryCandidate, []inventoryUnresolved) {
	scoped := map[string]inventoryview.ScopeEntry{}
	if r.scope != nil {
		for _, entry := range r.scope.Entries {
			scoped[entry.Target] = entry
		}
	}
	matched := []inventoryCandidate{}
	unresolved := []inventoryUnresolved{}
	consider := func(target, kind string, entry *inventoryview.ScopeEntry) {
		var pins []inventoryview.Pin
		c := inventoryCandidate{Target: target, Kind: kind}
		if entry != nil {
			pins, c.ScopePaths = entry.Pins, entry.Paths
		}
		selected, reasons := r.index.Select(target, pins)
		c.Selected = selected
		c.Uses = r.index.Counts(target)
		if r.comparison.requested {
			c.Newness = inventoryview.Newness(target, r.comparison.baseline)
		}
		if len(reasons) > 0 {
			u := inventoryUnresolved{Target: target, Kind: kind, Reasons: reasons, Heads: []string{}, Lifecycle: []string{}, Selected: selected, ScopePaths: c.ScopePaths}
			if record := r.index.Record(target); record != nil {
				u.Heads, u.Lifecycle = record.RevisionHeads, record.LifecycleHeads
			}
			unresolved = append(unresolved, u)
			return
		}
		if r.intent != "" {
			keep := false
			for _, s := range selected {
				keep = keep || string(s.Intent) == r.intent
			}
			if !keep {
				return
			}
		}
		if r.onlyNew && c.Newness != inventoryview.NewnessNew {
			return
		}
		matched = append(matched, c)
	}
	for _, record := range r.index.Inventory.Records {
		if (r.target != "" && record.Target != r.target) || (r.kind != "" && record.Kind != r.kind) {
			continue
		}
		if r.scope != nil {
			entry, ok := scoped[record.Target]
			if !ok {
				continue
			}
			delete(scoped, record.Target)
			consider(record.Target, record.Kind, &entry)
			continue
		}
		consider(record.Target, record.Kind, nil)
	}
	// Scope pins naming a record that does not exist remain visible.
	missing := []string{}
	for target := range scoped {
		if (r.target == "" || target == r.target) && (r.kind == "" || inventoryKindOf(target) == r.kind) {
			missing = append(missing, target)
		}
	}
	sort.Strings(missing)
	for _, target := range missing {
		entry := scoped[target]
		consider(target, inventoryKindOf(target), &entry)
	}
	sort.SliceStable(unresolved, func(i, j int) bool { return unresolved[i].Target < unresolved[j].Target })
	return matched, unresolved
}

// inventoryKindOf reads the kind segment of a canonical technical URN.
func inventoryKindOf(target string) string {
	parts := strings.Split(target, ":")
	if len(parts) < 5 {
		return ""
	}
	return parts[3]
}

func selectedCodeHealth(ctx context.Context, resolver *coderesolve.Resolver, r requirements.TechnicalRecord, selected []inventoryview.SelectedPin, head string) []inventoryCodeHealth {
	var health []inventoryCodeHealth
	for _, s := range selected {
		rev := r.Revision(s.Pin.Revision)
		if rev == nil || rev == r.CurrentRevision {
			continue
		}
		health = append(health, inventoryCodeHealthOf(ctx, resolver, s.Pin.Revision, rev, head)...)
	}
	return health
}

// inventoryCodeHealthOf resolves every reference a revision owns, including
// interaction and relationship evidence, with its evidence ID and edge intent.
func inventoryCodeHealthOf(ctx context.Context, resolver *coderesolve.Resolver, owner string, rev *requirements.TechnicalRevision, head string) []inventoryCodeHealth {
	health := []inventoryCodeHealth{}
	for _, owned := range inventoryview.Evidence(rev) {
		health = append(health, inventoryCodeHealth{Owner: owner + owned.Suffix(), Evidence: owned.Evidence.ID, Intent: string(owned.Intent), Reference: owned.Evidence.Location().String(), Resolution: resolver.Resolve(ctx, owned.Evidence.Reference, head)})
	}
	return health
}

// inventoryLinksOf reports every pin a revision declares with its status.
func inventoryLinksOf(ix *inventoryview.Index, owner string, rev *requirements.TechnicalRevision) []inventoryLinkHealth {
	links := []inventoryLinkHealth{}
	add := func(role string, pin saga.DocumentationLink) {
		links = append(links, inventoryLinkHealth{Owner: owner, Role: role, Link: pin, Status: ix.Status(pin)})
	}
	for _, pin := range rev.Components {
		add("member", pin)
	}
	for _, holder := range rev.Holders {
		add("holder", holder.Component)
	}
	for _, edge := range rev.Relationships {
		add("relationship:"+edge.ID, edge.Destination)
	}
	for _, pin := range rev.Directory {
		add("directory", pin)
	}
	if rev.ERD != nil {
		add("baseline_erd", *rev.ERD)
	}
	for _, pin := range rev.Pins {
		add("overlay_pin", pin)
	}
	return links
}

func inventoryKind(kind string) bool {
	for _, k := range requirements.TechnicalKinds {
		if k.Kind == kind {
			return true
		}
	}
	return false
}

type inventoryUsesData struct {
	Subject      inventoryUsesSubject  `json:"subject"`
	Uses         []inventoryview.Use   `json:"uses"`
	Completeness inventoryUsesComplete `json:"completeness"`
}

type inventoryUsesSubject struct {
	Target   string `json:"target"`
	Revision string `json:"revision,omitempty"`
	Kind     string `json:"kind"`
	Exists   bool   `json:"exists"`
}

type inventoryUsesComplete struct {
	Complete  bool     `json:"complete"`
	Depth     int      `json:"depth"`
	DepthCut  bool     `json:"depth_cut"`
	CycleCut  bool     `json:"cycle_cut"`
	Truncated bool     `json:"truncated"`
	Includes  []string `json:"includes"`
	Excludes  []string `json:"excludes"`
}

// queryInventoryUses pages declared reverse uses of one technical identity.
func queryInventoryUses(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query inventory-uses", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("saga", "", "app Saga root")
	target := flags.String("target", "", "canonical technical URN")
	flags.String("repo", "", "accepted for uniformity; declared uses read no source")
	revision := flags.String("revision", "", "exact revision URN")
	depth := flags.Int("depth", 0, "additional declared owner hops")
	var roles stringList
	flags.Var(&roles, "role", "implementation_item, review_item or system_member; repeatable")
	limit := flags.Int("limit", 50, "page size")
	cursor := flags.String("cursor", "", "snapshot-bound page cursor")
	fail := func(code string, err error) error {
		return writeQueryFailure(out, &queryError{Code: code, Message: err.Error()})
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return writeQuerySuccess(out, "", queryHelpFor("inventory-uses"), nil)
		}
		return fail("invalid_argument", err)
	}
	if *root == "" || *target == "" || flags.NArg() != 0 || *limit < 1 || *limit > min(maxQueryPageSize, inventoryview.MaxLimit) || *depth < 0 || *depth > inventoryview.MaxDepth {
		return fail("invalid_argument", fmt.Errorf("requires --saga and --target; --depth 0..%d; valid --limit", inventoryview.MaxDepth))
	}
	for _, role := range roles {
		switch role {
		case inventoryview.RoleImplementationItem, inventoryview.RoleReviewItem, inventoryview.RoleSystemMember, inventoryview.RoleDataHolder, inventoryview.RoleRelationship, inventoryview.RoleERDDirectory, inventoryview.RoleERDOverlay:
		default:
			return fail("invalid_argument", fmt.Errorf("unknown --role %s", role))
		}
	}
	manifest, err := saga.ReadManifest(*root)
	if err != nil {
		return fail("invalid_saga", err)
	}
	if err := inventoryQueryTarget(manifest.ID, *target); err != nil {
		return fail("invalid_argument", err)
	}
	if *revision != "" && !strings.HasPrefix(*revision, *target+":revision:") {
		return fail("invalid_argument", fmt.Errorf("--revision must be a revision URN of --target"))
	}
	changes := gitdiff.ChangeSet{}
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
	ix := inventoryview.Build(document, &d)
	subject := inventoryUsesSubject{Target: *target, Revision: *revision, Kind: inventoryKindOf(*target), Exists: ix.Record(*target) != nil}
	all := ix.Uses(*target, inventoryview.UseOptions{Revision: *revision, Depth: *depth, Roles: roles, Limit: inventoryview.MaxLimit})
	key := "inventory-uses:v1\x00" + *target + "\x00" + *revision + "\x00" + fmt.Sprint(*depth) + "\x00" + strings.Join(roles, ",")
	start, cursorErr := decodeTermsCursor(*cursor, key, snapshot, all.Total)
	if cursorErr != nil {
		return writeQueryFailure(out, cursorErr)
	}
	result := ix.Uses(*target, inventoryview.UseOptions{Revision: *revision, Depth: *depth, Roles: roles, Offset: start, Limit: *limit})
	page := queryPageEnvelope{Total: result.Total, Returned: len(result.Uses), HasMore: start+len(result.Uses) < result.Total}
	if page.HasMore {
		next := encodeTermsCursor(key, snapshot, start+len(result.Uses))
		page.NextCursor = &next
	}
	after, err := reviewapp.Snapshot(ctx, *root, changes)
	if err != nil {
		return fail("internal", err)
	}
	if after != snapshot {
		return fail("stale_snapshot", fmt.Errorf("the Saga changed while reading; restart the query"))
	}
	return writeQuerySuccess(out, snapshot, inventoryUsesData{Subject: subject, Uses: result.Uses, Completeness: inventoryUsesComplete{
		Complete: result.Complete, Depth: *depth, DepthCut: result.DepthCut, CycleCut: result.CycleCut, Truncated: result.Truncated,
		Includes: []string{"implementation and review deck Items whose documentation pins the target", "technical owners whose revisions declare the target, marked current or historical", "transitive uses through current owner revisions up to --depth"},
		Excludes: []string{"undeclared mentions in prose, SVG text or code", "uses through superseded owner revisions (reported, not traversed)"},
	}}, &page)
}

// inventoryQueryTarget accepts a canonical URN of any inventory kind.
func inventoryQueryTarget(sagaID, target string) error {
	prefix := "urn:change-saga:" + sagaID + ":"
	rest, ok := strings.CutPrefix(target, prefix)
	kind, id, found := strings.Cut(rest, ":")
	if !ok || !found || !inventoryKind(kind) || strings.Contains(id, ":") || id == "" {
		return fmt.Errorf("--target must be a canonical technical URN of this Saga")
	}
	return nil
}

type inventorySelectionsData struct {
	Head         string                        `json:"head_oid"`
	Selections   []inventoryview.ItemSelection `json:"selections"`
	Completeness inventoryCoverageComplete     `json:"completeness"`
}

// queryInventorySelections explains every saved implementation Item
// selection: its declared path, containing evidence and separately resolved
// pin, selected-byte and containing-evidence health at one source revision.
func queryInventorySelections(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query inventory-selections", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("saga", "", "app Saga root")
	repo := flags.String("repo", "", "source checkout")
	head := flags.String("head", "HEAD", "source revision to view")
	feature := flags.String("feature", "", "feature ID or URN")
	itemTarget := flags.String("item", "", "implementation Item URN")
	state := flags.String("state", "", "eligible, ineligible or unresolved")
	limit := flags.Int("limit", 50, "page size")
	cursor := flags.String("cursor", "", "snapshot-bound page cursor")
	fail := func(code string, err error) error {
		return writeQueryFailure(out, &queryError{Code: code, Message: err.Error()})
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return writeQuerySuccess(out, "", queryHelpFor("inventory-selections"), nil)
		}
		return fail("invalid_argument", err)
	}
	if *root == "" || flags.NArg() != 0 || *limit < 1 || *limit > maxQueryPageSize || (*state != "" && *state != "eligible" && *state != "ineligible" && *state != "unresolved") {
		return fail("invalid_argument", fmt.Errorf("requires --saga; valid --state and --limit"))
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
	document, _, err := saga.Load(*root)
	if err != nil {
		return fail("invalid_saga", err)
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return fail("source_unavailable", err)
	}
	defer resolver.Close()
	all := inventoryview.ItemSelections(ctx, document, &inventory, changes.HeadOID, resolver)
	selected := []inventoryview.ItemSelection{}
	for _, s := range all {
		if (featureID != "" && s.Feature != featureID) || (*itemTarget != "" && s.Item != *itemTarget) {
			continue
		}
		switch {
		case *state == "eligible" && !s.Result.Eligible, *state == "ineligible" && (s.Result.Eligible || s.Result.State != "resolved"), *state == "unresolved" && s.Result.State == "resolved":
			continue
		}
		// Attachment is the structural-and-current-pins test coverage uses.
		s.Attached = s.Result.State == "resolved" && s.Result.PinsCurrent
		selected = append(selected, s)
	}
	key := "inventory-selections:v1\x00" + changes.HeadOID + "\x00" + featureID + "\x00" + *itemTarget + "\x00" + *state
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
	after, err := reviewapp.Snapshot(ctx, *root, changes)
	if err != nil {
		return fail("internal", err)
	}
	if after != snapshot {
		return fail("stale_snapshot", fmt.Errorf("the Saga changed while reading; restart the query"))
	}
	return writeQuerySuccess(out, snapshot, inventorySelectionsData{Head: changes.HeadOID, Selections: selected[start:end], Completeness: inventoryCoverageComplete{
		Measures: []string{"every saved selection of every implementation deck Item, in deck order", "structure from the saved revisions; pin status; selected bytes and containing evidence viewed separately at head_oid"},
		Limits:   []string{"attached selections add only their selected lines to implementation-deck coverage, through the Item; unselected code in the same entity earns nothing", "review deck Items never inherit coverage", "eligibility is byte and pin currency, not proof that the explanation is correct"},
	}}, &page)
}

// inheritedSelections returns the eligible Item selections that contribute
// labeled inherited coverage at head. An unreadable inventory contributes
// nothing; it never widens coverage.
func inheritedSelections(ctx context.Context, document *saga.Saga, head string, resolver *coderesolve.Resolver) []coverage.InheritedReference {
	inventory, err := requirements.LoadInventory(document.Root, document.Manifest.ID)
	if err != nil {
		return nil
	}
	inherited, _ := inventoryview.InheritedReferences(ctx, document, &inventory, head, resolver)
	return inherited
}
