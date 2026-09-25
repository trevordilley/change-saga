package cli

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/reviewapp"
	"github.com/twentyideas/changesaga/internal/saga"
)

// querySchema is the one envelope every query operation writes.
const querySchema = "change-saga.ai/v1"

const (
	maxQueryPageSize     = 1000
	maxFragmentChunkSize = 1024 * 1024
)

// These transport-local requests deliberately contain no indexing or loading
// logic. A small adapter converts them to reviewapp requests once a session is
// opened; keeping this seam also makes the CLI contract independently testable.
type queryOpenOptions struct {
	SagaRoot    string
	SourceDir   string
	Range       gitdiff.Range
	SummaryOnly bool
	Operation   string
}

type overviewQuery struct{}

type childrenQuery struct {
	Parent string
	Cursor string
	Limit  int
}

type fragmentQuery struct {
	Target string
	Offset int64
	Limit  int
}

type fragmentDiffQuery struct {
	Target string
	Cursor string
	Limit  int
}

type diffOwnerQuery struct {
	Ref    string
	Cursor string
	Limit  int
}

type gapQuery struct {
	Kind   string
	Cursor string
	Limit  int
}

type mappingQuery struct {
	Target       string
	Sort         string
	MinimumScore int
	Cursor       string
	Limit        int
}

type claimQuery struct {
	Target string
	Status string
	Cursor string
	Limit  int
}

type verificationQuery struct {
	Claim  string
	Status string
	Cursor string
	Limit  int
}

// livingQuery is a transport-only request. The application package owns all
// graph composition, readiness policy, and cursor validation.
type livingQuery struct {
	Operation      string
	Filters        livingapp.Filters
	Cursor         string
	Limit          int
	ConflictCursor string
	ConflictLimit  int
}

type queryPage struct {
	Data any
	Page queryPageEnvelope
}

type querySession interface {
	Snapshot() string
	Overview(context.Context, overviewQuery) (any, error)
	Children(context.Context, childrenQuery) (queryPage, error)
	ReadFragment(context.Context, fragmentQuery) (any, error)
	FragmentDiffs(context.Context, fragmentDiffQuery) (queryPage, error)
	DiffOwners(context.Context, diffOwnerQuery) (queryPage, error)
	Gaps(context.Context, gapQuery) (queryPage, error)
	Mappings(context.Context, mappingQuery) (queryPage, error)
	Claims(context.Context, claimQuery) (queryPage, error)
	Verifications(context.Context, verificationQuery) (queryPage, error)
	Living(context.Context, livingQuery) (queryPage, error)
}

type querySessionOpener func(context.Context, queryOpenOptions) (querySession, error)

// Tests replace this constructor with a deterministic in-memory session.
var openQuerySession querySessionOpener = openReviewAppSession

type queryError struct {
	Code      string
	Message   string
	Retryable bool
	Details   any
}

func (e *queryError) Error() string { return e.Message }

type queryErrorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Details   any    `json:"details,omitempty"`
}

type queryPageEnvelope struct {
	Total      int     `json:"total"`
	Returned   int     `json:"returned"`
	HasMore    bool    `json:"has_more"`
	NextCursor *string `json:"next_cursor"`
}

type queryEnvelope struct {
	Schema   string              `json:"schema"`
	OK       bool                `json:"ok"`
	Snapshot string              `json:"snapshot,omitempty"`
	Data     any                 `json:"data,omitempty"`
	Page     *queryPageEnvelope  `json:"page,omitempty"`
	Error    *queryErrorEnvelope `json:"error,omitempty"`
}

type queryHelp struct {
	Usage                string                       `json:"usage"`
	Operations           []string                     `json:"operations,omitempty"`
	Operation            string                       `json:"operation,omitempty"`
	Purpose              string                       `json:"purpose,omitempty"`
	DataPaths            []string                     `json:"data_paths,omitempty"`
	Pagination           *queryPaginationDescription  `json:"pagination,omitempty"`
	AdditionalPagination []queryPaginationDescription `json:"additional_pagination,omitempty"`
}

type queryPaginationDescription struct {
	Kind           string `json:"kind"`
	CountedPath    string `json:"counted_path,omitempty"`
	TotalPath      string `json:"total_path,omitempty"`
	ReturnedPath   string `json:"returned_path,omitempty"`
	HasMorePath    string `json:"has_more_path,omitempty"`
	NextCursorPath string `json:"next_cursor_path,omitempty"`
	NextOffsetPath string `json:"next_offset_path,omitempty"`
	CursorFlag     string `json:"cursor_flag,omitempty"`
	LimitFlag      string `json:"limit_flag,omitempty"`
}

type querySchemaDescription struct {
	Operation            string                       `json:"operation"`
	Purpose              string                       `json:"purpose"`
	Usage                string                       `json:"usage"`
	DataPaths            []string                     `json:"data_paths"`
	Pagination           queryPaginationDescription   `json:"pagination"`
	AdditionalPagination []queryPaginationDescription `json:"additional_pagination,omitempty"`
}

var queryOperations = []string{
	"schema",
	"overview",
	"children",
	"fragment",
	"fragment-diffs",
	"slide",
	"slide-diffs",
	"diff-owners",
	"gaps",
	"mappings",
	"claims",
	"verifications",
	"context",
	"personas",
	"persona-references",
	"requirements",
	"requirement-history",
	"citations",
	"relations",
	"waves",
	"work-items",
	"work-events",
	"work-conflicts",
	"traceability",
	"readiness",
	"audit",
	"layers",
	"history",
	"inventory",
	"inventory-uses",
	"inventory-coverage",
	"inventory-selections",
	"terms",
	"term-references",
}

// queryPurpose says what each operation answers. It is keyed by the same
// operation names the dispatcher uses so documentation generated from it — the
// skill's query reference in particular — cannot describe an operation the CLI
// does not have, or omit one it does.
var queryPurpose = map[string]string{
	"inventory":            "Component/System/data-entity/ERD definitions, pinned graph links, exact code health and optional selected-record history; explicit intent and comparison-relative newness filters, declared feature scope and a separate unresolved page",
	"inventory-coverage":   "which tracked code at one source revision the current Component/System definitions account for: covered and uncovered ranges with every owner, stale references, and unresolved or excluded owners, separate from deck and review coverage",
	"inventory-selections": "saved implementation Item selections with their declared path, containing evidence, pin health, and separately resolved selected-byte and containing-evidence health; whether each contributes inherited deck coverage",
	"inventory-uses":       "declared reverse uses of one technical identity: implementation and review deck Items and technical owners, with bounded transitive paths and explicit completeness",
	"schema":               "the response paths and pagination contract for a query operation; no saga is required",
	"overview":             "saga identity, source comparison, coverage summary, and the top of the hierarchy",
	"children":             "one level of children under a target; a fragment's children are its landmarks",
	"fragment":             "bounded fragment content by byte range, without reading files directly",
	"fragment-diffs":       "the changed atoms a saga, chapter, section, fragment, or landmark references, and its stale references",
	"slide":                "bounded visual slide content and its ordered semantic Items",
	"slide-diffs":          "the changed atoms a slide Item references",
	"diff-owners":          "in a comparison (--against), the narrative targets whose code references hold the changed lines or file at a code location, and the terms whose code contains each line; for any line, use traceability --ref",
	"gaps":                 "uncovered atoms, stale selectors, and overlapping coverage",
	"mappings":             "coverage records ranked by breadth and justification signals so scrutiny starts at the weakest mappings",
	"claims":               "falsifiable author assertions, exact evidence, current mapping state, and latest verification result",
	"verifications":        "append-only verification history for author claims",
	"context":              "a compact, bounded feature-owned index with one-story expansion for exact intent, provenance, visual Items, code links, terms, neighbors, gaps, and conflicts",
	"personas":             "current persona definitions and lifecycle heads, selected exactly by stable ID or canonical URN",
	"persona-references":   "direct explicit incoming and outgoing persona references with provenance and declared coverage",
	"requirements":         "current requirement definitions and lifecycle heads without fabricating winners for conflicts",
	"requirement-history":  "append-only revision and lifecycle history in deterministic graph order",
	"citations":            "immutable requirement provenance records",
	"relations":            "typed current, stale, and superseded living-Saga relations",
	"waves":                "ordered work-plan coordination cohorts and derived item counts",
	"work-items":           "current work-item definitions, progress, explicit dependency blockers, workspaces, and merge evidence",
	"work-events":          "normalized append-only progress, workspace, merge, and contract events",
	"work-conflicts":       "deterministically identified work-plan conflicts and competing heads",
	"traceability":         "current story-to-design/work/review/code/test paths (design that addresses the whole story is also listed as broad), reverse lookup by a code location at any revision (remapped as staleness is) or by pinned commit, and transitive blockers",
	"readiness":            "independent requirement, plan, and delivery coverage axes; only immutable delivery evidence gates peer-review readiness",
	"audit":                "a complete feature handoff audit: broad intent, exact Item evidence, criterion explanations, stale pins/selectors, cross-feature links, exceptions, and unresolved conflicts",
	"history":              "when a record was introduced, what it replaced, and every commit that changed it, each with the command that opens that comparison",
	"terms":                "the project's vocabulary: each term's independent definition maturity and implementation-evidence availability, definition, aliases, links, and exact code health at the head; omitted legacy assessments are unknown, and evidence availability never proves implementation",
	"term-references":      "direct explicit incoming and outgoing term references with provenance and declared coverage",
	"layers":               "one comparison's Changed records (each with before and after), Affected records (with why), and Code (hunks grouped under the records that reference them, plus unreferenced lines)",
}

var queryUsage = map[string]string{
	"inventory":            "change-saga query inventory --saga PATH [--kind component|system|data-entity|erd|erd-overlay] [--target URN [--history]] [--feature ID|URN] [--intent proposed|implemented|unspecified] [--new] [--cursor TOKEN] [--limit N] [--conflict-cursor TOKEN] [--conflict-limit N] [--repo PATH] [--against REV] [--head REV]",
	"inventory-coverage":   "change-saga query inventory-coverage --saga PATH [--path PREFIX]... [--kind component|system] [--state ranges|covered|uncovered|stale|unresolved|excluded] [--cursor TOKEN] [--limit N] [--repo PATH] [--head REV]",
	"inventory-selections": "change-saga query inventory-selections --saga PATH [--feature ID|URN] [--item URN] [--state eligible|ineligible|unresolved] [--cursor TOKEN] [--limit N] [--repo PATH] [--head REV]",
	"inventory-uses":       "change-saga query inventory-uses --saga PATH --target URN [--revision URN] [--depth N] [--role implementation_item|review_item|system_member|data_holder|relationship_destination|erd_directory|erd_overlay] [--cursor TOKEN] [--limit N] [--repo PATH]",
	"":                     "change-saga query <operation> --saga PATH [--repo PATH] [--against REV [--head REV]] [operation flags]",
	"schema":               "change-saga query schema <operation>",
	"overview":             "change-saga query overview --saga PATH [--repo PATH] [--against REV [--head REV]]",
	"children":             "change-saga query children --saga PATH --parent TARGET [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"fragment":             "change-saga query fragment --saga PATH --target FRAGMENT [--offset N] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"fragment-diffs":       "change-saga query fragment-diffs --saga PATH --target TARGET [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"slide":                "change-saga query slide --saga PATH --target SLIDE [--offset N] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"slide-diffs":          "change-saga query slide-diffs --saga PATH --target ITEM [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"diff-owners":          "change-saga query diff-owners --saga PATH --ref LOCATION [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"gaps":                 "change-saga query gaps --saga PATH [--kind uncovered|stale|overlap] [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"mappings":             "change-saga query mappings --saga PATH [--target TARGET] [--sort scrutiny|target|path] [--minimum-score N] [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"claims":               "change-saga query claims --saga PATH [--target TARGET] [--status unverified|verified|failed|inconclusive] [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"verifications":        "change-saga query verifications --saga PATH [--claim ID] [--status unverified|verified|failed|inconclusive] [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"context":              "change-saga query context --saga PATH --feature ID|URN [--expand STORY-ID|URN] [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"personas":             "change-saga query personas --saga PATH [--persona ID|URN] [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"persona-references":   "change-saga query persona-references --saga PATH --persona ID|URN [--cursor TOKEN] [--limit N] [--conflict-cursor TOKEN] [--conflict-limit N] [--repo PATH] [--against REV [--head REV]]",
	"requirements":         "change-saga query requirements --saga PATH [--feature ID|URN] [--requirement ID|URN] [--state STATE] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]",
	"requirement-history":  "change-saga query requirement-history --saga PATH --requirement ID|URN [--cursor TOKEN] [--limit N] [--against REV [--head REV]]",
	"citations":            "change-saga query citations --saga PATH [--citation ID|URN] [--requirement ID|URN] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]",
	"relations":            "change-saga query relations --saga PATH [--relation ID|URN] [--type TYPE] [--from URN] [--to URN] [--state STATE] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]",
	"waves":                "change-saga query waves --saga PATH [--wave ID|URN] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]",
	"work-items":           "change-saga query work-items --saga PATH [--item ID|URN] [--wave ID|URN] [--status STATE] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]",
	"work-events":          "change-saga query work-events --saga PATH [--item ID|URN] [--kind KIND] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]",
	"work-conflicts":       "change-saga query work-conflicts --saga PATH [--item ID|URN] [--wave ID|URN] [--kind KIND] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]",
	"traceability":         "change-saga query traceability --saga PATH [--requirement ID|URN] [--criterion ID|URN] [--ref LOCATION | --commit OID] [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"readiness":            "change-saga query readiness --saga PATH [--requirement ID|URN] [--status ready|blocked] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]",
	"audit":                "change-saga query audit --saga PATH --feature ID|URN [--repo PATH] [--head REV]",
	"history":              "change-saga query history --saga PATH --node URN",
	"terms":                "change-saga query terms --saga PATH [--term ID|URN] [--story ID|URN] [--ref LOCATION] [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]",
	"term-references":      "change-saga query term-references --saga PATH --term ID|URN [--cursor TOKEN] [--limit N] [--conflict-cursor TOKEN] [--conflict-limit N] [--repo PATH] [--against REV [--head REV]]",
	"layers":               "change-saga query layers --saga PATH --against REV [--head REV] [--layer changed|affected|code] [--repo PATH]",
}

// Query executes one read-only application query and writes exactly one JSON
// value for every ordinary outcome, including help and invalid input. The
// returned StatusError only communicates the already-rendered exit status to
// the process entrypoint; callers must not print it.
func Query(ctx context.Context, args []string, out io.Writer) error {
	return queryWithOpener(ctx, args, out, openQuerySession)
}

func queryWithOpener(ctx context.Context, args []string, out io.Writer, open querySessionOpener) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		return writeQuerySuccess(out, "", queryHelp{Usage: queryUsage[""], Operations: queryOperations}, nil)
	}

	operation := args[0]
	if _, ok := queryUsage[operation]; !ok {
		return writeQueryFailure(out, &queryError{
			Code:    "invalid_argument",
			Message: "unknown query operation",
			Details: map[string]any{"allowed": queryOperations},
		})
	}
	if operation == "schema" {
		return writeQuerySchema(args[1:], out)
	}
	if operation == "history" {
		if len(args) > 1 && isHelpArg(args[1]) {
			return writeQuerySuccess(out, "", queryHelpFor(operation), nil)
		}
		return queryHistory(ctx, args[1:], out)
	}
	if operation == "inventory" {
		return queryInventory(ctx, args[1:], out)
	}
	if operation == "inventory-uses" {
		return queryInventoryUses(ctx, args[1:], out)
	}
	if operation == "inventory-coverage" {
		return queryInventoryCoverage(ctx, args[1:], out)
	}
	if operation == "inventory-selections" {
		return queryInventorySelections(ctx, args[1:], out)
	}
	if operation == "terms" {
		if len(args) > 1 && isHelpArg(args[1]) {
			return writeQuerySuccess(out, "", queryHelpFor(operation), nil)
		}
		return queryTerms(ctx, args[1:], out)
	}
	if operation == "layers" {
		if len(args) > 1 && isHelpArg(args[1]) {
			return writeQuerySuccess(out, "", queryHelpFor(operation), nil)
		}
		return queryLayers(ctx, args[1:], out)
	}

	request, options, help, err := parseQuery(operation, args[1:])
	if help {
		return writeQuerySuccess(out, "", queryHelpFor(operation), nil)
	}
	if err != nil {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: err.Error()})
	}
	options.SummaryOnly = operation == "overview" || operation == "children"
	options.Operation = operation
	if operation == "slide" || operation == "slide-diffs" {
		if _, err := saga.ReadManifest(options.SagaRoot); err != nil {
			return writeQueryFailure(out, normalizeQueryError(err))
		}
	}

	session, err := open(ctx, options)
	if err != nil {
		return writeQueryFailure(out, normalizeQueryError(err))
	}

	var result any
	var responsePage *queryPageEnvelope
	switch request := request.(type) {
	case overviewQuery:
		result, err = session.Overview(ctx, request)
	case childrenQuery:
		var page queryPage
		page, err = session.Children(ctx, request)
		result, responsePage = page.Data, &page.Page
	case fragmentQuery:
		result, err = session.ReadFragment(ctx, request)
	case fragmentDiffQuery:
		var page queryPage
		page, err = session.FragmentDiffs(ctx, request)
		result, responsePage = page.Data, &page.Page
	case diffOwnerQuery:
		// The ref's commit may be any revision, as everywhere a location is
		// accepted; an unresolvable one is left for the session to refuse.
		if location, resolveErr := resolveLocation(ctx, firstNonEmpty(options.SourceDir, options.SagaRoot), request.Ref); resolveErr == nil {
			request.Ref = location.String()
		}
		var page queryPage
		page, err = session.DiffOwners(ctx, request)
		result, responsePage = page.Data, &page.Page
	case gapQuery:
		var page queryPage
		page, err = session.Gaps(ctx, request)
		result, responsePage = page.Data, &page.Page
	case mappingQuery:
		var page queryPage
		page, err = session.Mappings(ctx, request)
		result, responsePage = page.Data, &page.Page
	case claimQuery:
		var page queryPage
		page, err = session.Claims(ctx, request)
		result, responsePage = page.Data, &page.Page
	case verificationQuery:
		var page queryPage
		page, err = session.Verifications(ctx, request)
		result, responsePage = page.Data, &page.Page
	case livingQuery:
		if request.Filters.Ref != "" {
			checkout := firstNonEmpty(options.SourceDir, options.SagaRoot)
			resolver, locateErr := locateReferences(ctx, checkout, &request.Filters)
			if locateErr != nil {
				return writeQueryFailure(out, locateErr)
			}
			defer resolver.Close()
		}
		var page queryPage
		page, err = session.Living(ctx, request)
		result, responsePage = page.Data, &page.Page
	default:
		err = errors.New("unsupported query request")
	}
	if err != nil {
		return writeQueryFailure(out, normalizeQueryError(err))
	}
	if err := writeQuerySuccess(out, session.Snapshot(), result, responsePage); err != nil {
		return err
	}
	if report, ok := result.(livingapp.AuditReport); ok && !report.Ready {
		return &StatusError{Code: report.ExitCode}
	}
	return nil
}

// locateReferences resolves a --ref at any revision to its commit and places
// every reference there through the same remapping that decides staleness,
// so a current location finds the evidence pinned at an older commit.
func locateReferences(ctx context.Context, checkout string, filters *livingapp.Filters) (*coderesolve.Resolver, *queryError) {
	location, err := resolveLocation(ctx, checkout, filters.Ref)
	if err != nil {
		return nil, &queryError{Code: "invalid_argument", Message: "--ref: " + err.Error()}
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return nil, &queryError{Code: "source_unavailable", Message: err.Error(), Retryable: true}
	}
	filters.Ref = location.String()
	located := map[string]coderesolve.Resolution{}
	filters.Locate = func(reference coderef.Reference) (coderef.Location, bool) {
		key := reference.Key()
		at, seen := located[key]
		if !seen {
			at = resolver.Resolve(ctx, reference, location.Commit)
			located[key] = at
		}
		return at.Location, at.Current()
	}
	return resolver, nil
}

func writeQuerySchema(args []string, out io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && isHelpArg(args[0])) {
		return writeQuerySuccess(out, "", queryHelp{Usage: queryUsage["schema"], Operations: queryDataOperations()}, nil)
	}
	if len(args) != 1 {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "schema requires exactly one query operation"})
	}
	operation := args[0]
	if operation == "schema" || queryUsage[operation] == "" {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "unknown schema operation", Details: map[string]any{"allowed": queryDataOperations()}})
	}
	return writeQuerySuccess(out, "", querySchemaFor(operation), nil)
}

func queryDataOperations() []string {
	return append([]string(nil), queryOperations[1:]...)
}

func queryHelpFor(operation string) queryHelp {
	description := querySchemaFor(operation)
	return queryHelp{
		Usage: queryUsage[operation], Operation: operation, Purpose: description.Purpose,
		DataPaths: description.DataPaths, Pagination: &description.Pagination, AdditionalPagination: description.AdditionalPagination,
	}
}

func querySchemaFor(operation string) querySchemaDescription {
	paths := map[string][]string{
		"overview":             {"data.saga", "data.source", "data.root", "data.overview_fragments", "data.chapters", "data.decks", "data.coverage"},
		"children":             {"data.children"},
		"fragment":             {"data.target", "data.content.data", "data.content.next_offset", "data.assets", "data.landmarks"},
		"fragment-diffs":       {"data.selectors", "data.atoms", "data.stale"},
		"slide":                {"data.target", "data.intent", "data.layout", "data.section", "data.takeaway", "data.content.data", "data.assets", "data.items", "data.items[].documentation", "data.reading_order", "data.authoring_snapshot", "data.authoring_heads", "data.authoring_conflict"},
		"slide-diffs":          {"data.selectors", "data.atoms", "data.stale"},
		"diff-owners":          {"data.atoms", "data.atoms[].terms"},
		"gaps":                 {"data.gaps"},
		"mappings":             {"data.mappings"},
		"claims":               {"data.claims"},
		"verifications":        {"data.verifications"},
		"context":              {"data.feature", "data.projection", "data.completeness", "data.stories", "data.neighbors", "data.terms", "data.visuals", "data.gaps", "data.conflicts", "data.expanded"},
		"personas":             {"data.personas"},
		"persona-references":   {"data.subject", "data.references", "data.counts", "data.completeness"},
		"requirements":         {"data.requirements"},
		"requirement-history":  {"data.events"},
		"citations":            {"data.citations"},
		"relations":            {"data.relations"},
		"waves":                {"data.waves"},
		"work-items":           {"data.items"},
		"work-events":          {"data.events"},
		"work-conflicts":       {"data.conflicts"},
		"traceability":         {"data.criteria", "data.unlinked_code_evidence"},
		"readiness":            {"data.summary", "data.requirements"},
		"audit":                {"data.feature", "data.status", "data.complete", "data.ready", "data.exit_code", "data.summary", "data.findings", "data.exceptions", "data.intentional_risks", "data.unresolved_conflicts"},
		"history":              {"data.introduced", "data.replaced", "data.events", "data.uncommitted"},
		"inventory":            {"data.head_oid", "data.records", "data.records[].code_health", "data.records[].links", "data.records[].history", "data.records[].selected", "data.records[].newness", "data.records[].scope_paths", "data.records[].uses", "data.records[].selected_code_health", "data.unresolved", "data.filters", "data.comparison", "data.scope", "data.completeness"},
		"inventory-coverage":   {"data.head_oid", "data.scope", "data.summary", "data.state", "data.entries", "data.completeness"},
		"inventory-selections": {"data.head_oid", "data.selections", "data.selections[].resolution", "data.completeness"},
		"inventory-uses":       {"data.subject", "data.uses", "data.uses[].path", "data.completeness"},
		"terms":                {"data.head_oid", "data.ref", "data.terms"},
		"term-references":      {"data.subject", "data.references", "data.counts", "data.completeness"},
		"layers":               {"data.summary", "data.changed", "data.affected", "data.code.groups", "data.code.unreferenced", "data.saga", "data.diagnostics"},
	}
	countedPaths := map[string]string{
		"children":             "data.children",
		"fragment-diffs":       "data.selectors",
		"slide-diffs":          "data.selectors",
		"diff-owners":          "data.atoms",
		"gaps":                 "data.gaps",
		"mappings":             "data.mappings",
		"claims":               "data.claims",
		"verifications":        "data.verifications",
		"context":              "data.stories",
		"personas":             "data.personas",
		"persona-references":   "data.references",
		"requirements":         "data.requirements",
		"requirement-history":  "data.events",
		"citations":            "data.citations",
		"relations":            "data.relations",
		"waves":                "data.waves",
		"work-items":           "data.items",
		"work-events":          "data.events",
		"work-conflicts":       "data.conflicts",
		"traceability":         "data.criteria",
		"readiness":            "data.requirements",
		"inventory":            "data.records",
		"inventory-uses":       "data.uses",
		"inventory-coverage":   "data.entries",
		"inventory-selections": "data.selections",
		"terms":                "data.terms",
		"term-references":      "data.references",
	}
	pagination := queryPaginationDescription{Kind: "none"}
	if operation == "fragment" || operation == "slide" {
		pagination = queryPaginationDescription{Kind: "byte-offset", NextOffsetPath: "data.content.next_offset"}
	} else if operation != "overview" && operation != "audit" && operation != "layers" && operation != "history" {
		pagination = queryPaginationDescription{
			Kind: "cursor", CountedPath: countedPaths[operation], TotalPath: "page.total", ReturnedPath: "page.returned",
			HasMorePath: "page.has_more", NextCursorPath: "page.next_cursor",
		}
	}
	description := querySchemaDescription{
		Operation: operation, Purpose: queryPurpose[operation], Usage: queryUsage[operation],
		DataPaths: append([]string(nil), paths[operation]...), Pagination: pagination,
	}
	if operation == "inventory" {
		description.AdditionalPagination = []queryPaginationDescription{{
			Kind: "cursor", CountedPath: "data.unresolved",
			TotalPath: "data.completeness.unresolved_page.total", ReturnedPath: "data.completeness.unresolved_page.returned",
			HasMorePath: "data.completeness.unresolved_page.has_more", NextCursorPath: "data.completeness.unresolved_page.next_cursor",
			CursorFlag: "--conflict-cursor", LimitFlag: "--conflict-limit",
		}}
	}
	if operation == "persona-references" || operation == "term-references" {
		description.AdditionalPagination = []queryPaginationDescription{{
			Kind: "cursor", CountedPath: "data.completeness.unresolved_owners",
			TotalPath: "data.completeness.unresolved_page.total", ReturnedPath: "data.completeness.unresolved_page.returned",
			HasMorePath: "data.completeness.unresolved_page.has_more", NextCursorPath: "data.completeness.unresolved_page.next_cursor",
			CursorFlag: "--conflict-cursor", LimitFlag: "--conflict-limit",
		}}
	}
	return description
}

func parseQuery(operation string, args []string) (any, queryOpenOptions, bool, error) {
	flags := flag.NewFlagSet("query "+operation, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sagaRoot := flags.String("saga", "", "saga root")
	sourceDir := flags.String("repo", "", "source repository checkout")
	opening := registerOpenFlags(flags)

	var parent, target, ref, cursor, conflictCursor, state, kind, sortOrder, claim string
	var feature, expand, requirement, persona, term, citation, relation, from, to, wave, item, criterion, commit string
	var offset int64
	var limit optionalInt
	var conflictLimit optionalInt
	var minimumScore optionalInt
	switch operation {
	case "children":
		flags.StringVar(&parent, "parent", "", "parent target URN")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "fragment", "slide":
		flags.StringVar(&target, "target", "", "fragment target URN")
		flags.Int64Var(&offset, "offset", 0, "content byte offset")
		flags.Var(&limit, "limit", "content byte limit")
	case "fragment-diffs", "slide-diffs":
		flags.StringVar(&target, "target", "", "saga target URN")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "diff-owners":
		flags.StringVar(&ref, "ref", "", "code location in the comparison: <commit>:<path>[#L<start>[-L<end>]]")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "gaps":
		flags.StringVar(&kind, "kind", "", "uncovered, stale, or overlap")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "mappings":
		flags.StringVar(&target, "target", "", "optional narrative target URN")
		flags.StringVar(&sortOrder, "sort", "scrutiny", "scrutiny, target, or path")
		flags.Var(&minimumScore, "minimum-score", "minimum scrutiny score from 0 to 100")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "claims":
		flags.StringVar(&target, "target", "", "optional narrative target URN")
		flags.StringVar(&state, "status", "", "unverified, verified, failed, or inconclusive")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "verifications":
		flags.StringVar(&claim, "claim", "", "optional claim id")
		flags.StringVar(&state, "status", "", "unverified, verified, failed, or inconclusive")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "context":
		flags.StringVar(&feature, "feature", "", "feature ID or URN")
		flags.StringVar(&expand, "expand", "", "story ID or URN to expand")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "personas":
		flags.StringVar(&persona, "persona", "", "optional persona ID or canonical URN")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "persona-references":
		flags.StringVar(&persona, "persona", "", "persona ID or canonical URN")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
		flags.StringVar(&conflictCursor, "conflict-cursor", "", "unresolved-owner pagination cursor")
		flags.Var(&conflictLimit, "conflict-limit", "unresolved-owner page size; defaults to --limit or 100")
	case "term-references":
		flags.StringVar(&term, "term", "", "term ID or canonical URN")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
		flags.StringVar(&conflictCursor, "conflict-cursor", "", "unresolved-owner pagination cursor")
		flags.Var(&conflictLimit, "conflict-limit", "unresolved-owner page size; defaults to --limit or 100")
	case "requirements":
		flags.StringVar(&feature, "feature", "", "optional feature ID or URN")
		flags.StringVar(&requirement, "requirement", "", "optional requirement ID or URN")
		flags.StringVar(&state, "state", "", "optional lifecycle state")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "requirement-history":
		flags.StringVar(&requirement, "requirement", "", "requirement ID or URN")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "citations":
		flags.StringVar(&citation, "citation", "", "optional citation ID or URN")
		flags.StringVar(&requirement, "requirement", "", "optional requirement ID or URN")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "relations":
		flags.StringVar(&relation, "relation", "", "optional relation ID or URN")
		flags.StringVar(&kind, "type", "", "optional relation type")
		flags.StringVar(&from, "from", "", "optional source endpoint URN")
		flags.StringVar(&to, "to", "", "optional target endpoint URN")
		flags.StringVar(&state, "state", "", "active, superseded, stale, or current")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "waves":
		flags.StringVar(&wave, "wave", "", "optional wave ID or URN")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "work-items":
		flags.StringVar(&item, "item", "", "optional work-item ID or URN")
		flags.StringVar(&wave, "wave", "", "optional wave ID or URN")
		flags.StringVar(&state, "status", "", "optional progress state")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "work-events":
		flags.StringVar(&item, "item", "", "optional work-item ID or URN")
		flags.StringVar(&kind, "kind", "", "progress, workspace, merge, or contract")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "work-conflicts":
		flags.StringVar(&item, "item", "", "optional work-item ID or URN")
		flags.StringVar(&wave, "wave", "", "optional wave ID or URN")
		flags.StringVar(&kind, "kind", "", "optional conflict kind")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "traceability":
		flags.StringVar(&requirement, "requirement", "", "optional requirement ID or URN")
		flags.StringVar(&criterion, "criterion", "", "optional criterion ID or URN")
		flags.StringVar(&ref, "ref", "", "optional code location at any revision; selects evidence whose lines, remapped to that commit, overlap it")
		flags.StringVar(&commit, "commit", "", "optional Git commit; selects evidence pinned at it")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "readiness":
		flags.StringVar(&requirement, "requirement", "", "optional requirement ID or URN")
		flags.StringVar(&state, "status", "", "ready or blocked")
		flags.StringVar(&cursor, "cursor", "", "pagination cursor")
		flags.Var(&limit, "limit", "page size")
	case "audit":
		flags.StringVar(&feature, "feature", "", "feature ID or canonical feature URN")
	}

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, queryOpenOptions{}, true, nil
		}
		return nil, queryOpenOptions{}, false, fmt.Errorf("invalid flags for %s: %w", operation, err)
	}
	if flags.NArg() != 0 {
		return nil, queryOpenOptions{}, false, fmt.Errorf("%s accepts no positional arguments", operation)
	}
	cursorSet, conflictCursorSet := false, false
	flags.Visit(func(value *flag.Flag) {
		if value.Name == "cursor" {
			cursorSet = true
		}
		if value.Name == "conflict-cursor" {
			conflictCursorSet = true
		}
	})
	if cursorSet && cursor == "" {
		return nil, queryOpenOptions{}, false, errors.New("--cursor cannot be empty")
	}
	if conflictCursorSet && conflictCursor == "" {
		return nil, queryOpenOptions{}, false, errors.New("--conflict-cursor cannot be empty")
	}
	if strings.TrimSpace(*sagaRoot) == "" {
		return nil, queryOpenOptions{}, false, errors.New("--saga is required")
	}
	maxLimit := maxQueryPageSize
	if operation == "fragment" || operation == "slide" {
		maxLimit = maxFragmentChunkSize
	}
	if limit.set && (limit.value < 1 || limit.value > maxLimit) {
		return nil, queryOpenOptions{}, false, fmt.Errorf("--limit must be between 1 and %d", maxLimit)
	}
	if conflictLimit.set && (conflictLimit.value < 1 || conflictLimit.value > maxQueryPageSize) {
		return nil, queryOpenOptions{}, false, fmt.Errorf("--conflict-limit must be between 1 and %d", maxQueryPageSize)
	}
	if offset < 0 {
		return nil, queryOpenOptions{}, false, errors.New("--offset cannot be negative")
	}
	if minimumScore.set && (minimumScore.value < 0 || minimumScore.value > 100) {
		return nil, queryOpenOptions{}, false, errors.New("--minimum-score must be between 0 and 100")
	}
	if operation == "children" && strings.TrimSpace(parent) == "" {
		return nil, queryOpenOptions{}, false, errors.New("--parent is required")
	}
	if (operation == "fragment" || operation == "fragment-diffs" || operation == "slide" || operation == "slide-diffs") && strings.TrimSpace(target) == "" {
		return nil, queryOpenOptions{}, false, errors.New("--target is required")
	}
	if operation == "diff-owners" && strings.TrimSpace(ref) == "" {
		return nil, queryOpenOptions{}, false, errors.New("--ref is required")
	}
	if operation == "requirement-history" && strings.TrimSpace(requirement) == "" {
		return nil, queryOpenOptions{}, false, errors.New("--requirement is required")
	}
	if operation == "context" && strings.TrimSpace(feature) == "" {
		return nil, queryOpenOptions{}, false, errors.New("--feature is required")
	}
	if operation == "persona-references" && strings.TrimSpace(persona) == "" {
		return nil, queryOpenOptions{}, false, errors.New("--persona is required; use query personas to discover stable IDs")
	}
	if operation == "term-references" && strings.TrimSpace(term) == "" {
		return nil, queryOpenOptions{}, false, errors.New("--term is required; use query terms --limit N to discover stable IDs")
	}
	if operation == "context" && expand != "" && cursor != "" {
		return nil, queryOpenOptions{}, false, errors.New("--expand and --cursor are mutually exclusive")
	}
	if operation == "gaps" && kind != "" && kind != "uncovered" && kind != "stale" && kind != "overlap" {
		return nil, queryOpenOptions{}, false, errors.New("--kind must be uncovered, stale, or overlap")
	}
	if operation == "mappings" && sortOrder != "scrutiny" && sortOrder != "target" && sortOrder != "path" {
		return nil, queryOpenOptions{}, false, errors.New("--sort must be scrutiny, target, or path")
	}
	if (operation == "claims" || operation == "verifications") && state != "" && state != "unverified" && state != "verified" && state != "failed" && state != "inconclusive" {
		return nil, queryOpenOptions{}, false, errors.New("--status must be unverified, verified, failed, or inconclusive")
	}
	if operation == "requirements" && state != "" && state != "proposed" && state != "accepted" && state != "deferred" && state != "rejected" && state != "retired" && state != "conflicted" {
		return nil, queryOpenOptions{}, false, errors.New("--state must be proposed, accepted, deferred, rejected, retired, or conflicted")
	}
	if operation == "relations" && state != "" && state != "active" && state != "superseded" && state != "stale" && state != "current" {
		return nil, queryOpenOptions{}, false, errors.New("--state must be active, superseded, stale, or current")
	}
	if operation == "traceability" {
		if ref != "" && commit != "" {
			return nil, queryOpenOptions{}, false, fmt.Errorf("--ref and --commit are mutually exclusive")
		}
		if revision, _, found := strings.Cut(ref, ":"); ref != "" && (!found || revision == "") {
			return nil, queryOpenOptions{}, false, fmt.Errorf("--ref must be a code location <commit>:<path>[#L<start>[-L<end>]]")
		}
		if commit != "" {
			decoded, decodeErr := hex.DecodeString(commit)
			if decodeErr != nil || len(decoded) != 20 && len(decoded) != 32 {
				return nil, queryOpenOptions{}, false, fmt.Errorf("--commit must be a 40- or 64-character hexadecimal Git object ID")
			}
			commit = strings.ToLower(commit)
		}
	}
	if operation == "work-items" && state != "" && state != "planned" && state != "ready" && state != "in_progress" && state != "blocked" && state != "done" && state != "cancelled" && state != "conflicted" {
		return nil, queryOpenOptions{}, false, errors.New("--status is not a valid progress state")
	}
	if operation == "work-events" && kind != "" && kind != "progress" && kind != "workspace" && kind != "merge" && kind != "contract" {
		return nil, queryOpenOptions{}, false, errors.New("--kind must be progress, workspace, merge, or contract")
	}
	if operation == "readiness" && state != "" && state != "ready" && state != "blocked" {
		return nil, queryOpenOptions{}, false, errors.New("--status must be ready or blocked")
	}
	if operation == "audit" && strings.TrimSpace(feature) == "" {
		return nil, queryOpenOptions{}, false, errors.New("--feature is required")
	}
	if operation == "audit" && strings.TrimSpace(*opening.against) != "" {
		return nil, queryOpenOptions{}, false, errors.New("audit observes one --head; --against is not supported")
	}

	options := queryOpenOptions{SagaRoot: *sagaRoot, SourceDir: *sourceDir, Range: opening.rng()}
	switch operation {
	case "overview":
		return overviewQuery{}, options, false, nil
	case "children":
		return childrenQuery{Parent: parent, Cursor: cursor, Limit: limit.value}, options, false, nil
	case "fragment", "slide":
		return fragmentQuery{Target: target, Offset: offset, Limit: limit.value}, options, false, nil
	case "fragment-diffs", "slide-diffs":
		return fragmentDiffQuery{Target: target, Cursor: cursor, Limit: limit.value}, options, false, nil
	case "diff-owners":
		return diffOwnerQuery{Ref: ref, Cursor: cursor, Limit: limit.value}, options, false, nil
	case "gaps":
		return gapQuery{Kind: kind, Cursor: cursor, Limit: limit.value}, options, false, nil
	case "mappings":
		return mappingQuery{Target: target, Sort: sortOrder, MinimumScore: minimumScore.value, Cursor: cursor, Limit: limit.value}, options, false, nil
	case "claims":
		return claimQuery{Target: target, Status: state, Cursor: cursor, Limit: limit.value}, options, false, nil
	case "verifications":
		return verificationQuery{Claim: claim, Status: state, Cursor: cursor, Limit: limit.value}, options, false, nil
	case "context", "personas", "persona-references", "term-references", "requirements", "requirement-history", "citations", "relations", "waves", "work-items", "work-events", "work-conflicts", "traceability", "readiness", "audit":
		filters := livingapp.Filters{
			Feature: feature, Expand: expand, Requirement: requirement, Persona: persona, Term: term, Kind: firstNonempty(kind, criterion), Citation: citation,
			Relation: relation, From: from, To: to, Wave: wave, Item: item, Ref: ref, Commit: commit,
		}
		if operation == "requirements" || operation == "relations" {
			filters.State = state
		} else if operation == "work-items" || operation == "readiness" {
			filters.Status = state
		}
		return livingQuery{Operation: operation, Filters: filters, Cursor: cursor, Limit: limit.value, ConflictCursor: conflictCursor, ConflictLimit: conflictLimit.value}, options, false, nil
	default:
		return nil, queryOpenOptions{}, false, fmt.Errorf("unknown query operation %q", operation)
	}
}

type optionalInt struct {
	value int
	set   bool
}

func (v *optionalInt) String() string { return strconv.Itoa(v.value) }

func (v *optionalInt) Set(raw string) error {
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return errors.New("must be an integer")
	}
	v.value, v.set = parsed, true
	return nil
}

func isHelpArg(arg string) bool { return arg == "help" || arg == "-h" || arg == "--help" }

func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func writeQuerySuccess(out io.Writer, snapshot string, data any, page *queryPageEnvelope) error {
	if page == nil {
		page = &queryPageEnvelope{Total: 1, Returned: 1}
	}
	return encodeQueryEnvelope(out, queryEnvelope{
		Schema:   querySchema,
		OK:       true,
		Snapshot: snapshot,
		Data:     data,
		Page:     page,
	})
}

func writeQueryFailure(out io.Writer, queryErr *queryError) error {
	if err := encodeQueryEnvelope(out, queryEnvelope{
		Schema: querySchema,
		OK:     false,
		Error: &queryErrorEnvelope{
			Code:      queryErr.Code,
			Message:   queryErr.Message,
			Retryable: queryErr.Retryable,
			Details:   queryErr.Details,
		},
	}); err != nil {
		return err
	}
	return &StatusError{Code: queryExitCode(queryErr.Code)}
}

func encodeQueryEnvelope(out io.Writer, envelope queryEnvelope) error {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(envelope)
}

func normalizeQueryError(err error) *queryError {
	var domainErr *queryError
	if errors.As(err, &domainErr) {
		if _, ok := queryExitCodes[domainErr.Code]; ok {
			return domainErr
		}
	}
	var applicationErr *reviewapp.Error
	if errors.As(err, &applicationErr) {
		code := string(applicationErr.Code)
		if _, ok := queryExitCodes[code]; ok {
			return &queryError{Code: code, Message: applicationErr.Message, Retryable: applicationErr.Retryable, Details: applicationErr.Details}
		}
	}
	var livingErr *livingapp.Error
	if errors.As(err, &livingErr) {
		code := string(livingErr.Code)
		if _, ok := queryExitCodes[code]; ok {
			return &queryError{Code: code, Message: livingErr.Message, Retryable: livingErr.Retryable, Details: livingErr.Details}
		}
	}
	return &queryError{Code: "internal", Message: "an unexpected error occurred"}
}

var queryExitCodes = map[string]int{
	"invalid_argument":   2,
	"invalid_saga":       3,
	"stale_snapshot":     4,
	"conflict":           4,
	"not_found":          5,
	"unsafe_path":        6,
	"unsupported_media":  6,
	"too_large":          6,
	"source_unavailable": 7,
	"internal":           1,
}

func queryExitCode(code string) int {
	if exit, ok := queryExitCodes[code]; ok {
		return exit
	}
	return 1
}
