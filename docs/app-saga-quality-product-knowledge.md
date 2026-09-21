# App Saga product-knowledge quality audit

Date: 2026-09-20

## Scope and method

This audit covers only `app.saga/___features/format.feature`,
`app.saga/___features/requirements.feature`, and this file. No product source or
other feature directory was changed.

The quality report was derived from the accepted story heads, the owned design
and implementation decks, `docs/app-saga-code-test-map.md`, and focused tests
already present in the repository. All Saga mutations used the repository-local
Change Saga CLI. The resulting test cases use ordered actions and pass/fail
expected results; their `verifies` relations point directly at current
acceptance criteria.

## Authored quality model

The two owned features now contain 17 test cases:

- 15 active automated cases with current passed runs;
- 2 proposed cases that deliberately have no run because focused executable
  proof is incomplete;
- 56 direct test-case-to-criterion `verifies` relations;
- 32 current evidence records containing 92 exact code references; and
- positive, negative, and edge coverage where the existing tests substantiate
  those paths.

The active cases cover feature gating, story revision and provenance, design
currency, prototype safety and annotations, requirement/code impact,
bidirectional traceability, vocabulary, ordered quality definitions, policy
exclusions, and run currency. Each test-implementation record points to focused
test functions. Each implementation-under-test record reuses the exact runtime
selector already owned by an implementation Item, so the requirement-to-test
and requirement-to-design-to-Item paths converge on the same code.

The cases are:

| Feature | Active cases | Proposed cases |
| --- | --- | --- |
| `format` | Show what each feature flag gates; keep off-gated behavior documented but disabled | Show every flag state without losing retirement history |
| `requirements` | Revise stories without losing identity or provenance; navigate design before code; add safe prototypes; keep annotations current; move stories without breaking links; traverse criterion evidence; show impact and persona gaps; browse overview and feature slices; follow vocabulary links; suggest vocabulary; define criterion tests; project policies and exclusions; keep run truth current | Exercise every story lifecycle state and preserve retirement history |

## Honest gaps

Four criteria remain intentionally inactive rather than being credited with an
unexecuted test:

1. `controlled-release / flag-state`
2. `controlled-release / retired-history`
3. `evolve-product-knowledge / lifecycle`
4. `evolve-product-knowledge / retired-history`

The repository has partial lifecycle behavior, but the focused tests do not yet
exercise every on/off/retired flag view or every proposed/accepted/deferred/
rejected/retired story transition while proving complete retained history. The
two proposed cases document those expectations without inventing evidence or a
passing run.

## Direct design evidence

Aggregate validation previously warned that four visual fragments had no
direct code evidence even though their criteria and implementation Items had
current code paths. The quality pass added four direct evidence records with 27
exact references to the runtime and focused tests for:

- `knowledge-topology`
- `overview-vocabulary-model`
- `story-design-lifecycle`
- `evidence-traversal-map`

`change-saga validate --json app.saga` now reports `valid: true` with no
issues in this worktree.

## Verification

The exact package command rerun before handoff was:

```text
hivecontrol exec oneshot 10m -- go test ./internal/areas ./internal/cli ./internal/coverage ./internal/impact ./internal/livingapp ./internal/prototypes ./internal/quality ./internal/readiness ./internal/requirements ./internal/server ./internal/vocabulary
```

Every package passed. The server suite completed in 147.776 seconds. Before
the final merge from `main`, status reported 15 current passed runs, zero stale
or failed authored runs, complete relation/reference health, and no validation
warnings.

## Change Saga CLI dogfooding

What worked well:

- immutable test revisions, evidence, and runs made the difference between
  planned coverage and executed proof explicit;
- evidence batching provided an all-or-nothing path for the largest reference
  writes; and
- validation immediately distinguished indirect traceability from the direct
  code evidence required for visual design fragments.

Rough spots encountered:

| Intended goal | Exact command | Observed behavior | Impact | Workaround | Smallest product improvement |
| --- | --- | --- | --- | --- | --- |
| Query only quality coverage | `go run ./cmd/change-saga query quality-coverage --saga app.saga --limit 10` | The CLI returned `unknown query operation`, although the lifecycle design documents this query. | Required parsing the much larger status document. | Projected `.coverage.areas.quality` and `.quality.facts` with `jq`. | Implement the documented paginated query or remove it from the contract and add a bounded status projection. |
| Plan many test cases atomically | `go run ./cmd/change-saga quality test-case add --help` | A case can be read from one structured request, but there is no batch or dry-run option. | Seventeen cases required independent mutations and interruption could leave a valid partial report. | Used stable IDs, request IDs, fail-fast execution, and full validation after the corpus was written. | Add atomic JSONL batch and dry-run support for add/revise/set-state. |
| Create all criterion links atomically | `go run ./cmd/change-saga relation add --help` | Relations are added one at a time with no batch or dry-run. | Fifty-six precise links required fifty-six mutations. | Used deterministic relation IDs, then queried relation currency and traceability. | Add an all-or-nothing relation batch with per-entry diagnostics. |
| Run and record a test without a truth gap | `go run ./cmd/change-saga quality run record --help` | The command explicitly records but never executes the command, and it has no batch mode. | Automation must keep execution success and record creation synchronized itself. | Executed the complete package command first, checked its exit status, then retained only passed run records. | Add an opt-in execute-and-record command with captured exit status, bounded output digest, and atomic multi-case recording. |
| Select durable Go test and runtime symbols | `go run ./cmd/change-saga quality evidence add --help` | Evidence accepts file/range locations but no Go symbol or test name selector. | Authors must discover and maintain line ranges manually. | Located named functions with repository search, pinned exact ranges, and kept runtime and tests in separate evidence records. | Add language-aware `--symbol` and `--test` selectors that resolve to canonical ranges and store the symbol hint. |
| Inspect a concise quality summary | `go run ./cmd/change-saga status --json app.saga` | The response includes every quality fact and next action and is too large for routine terminal inspection. | Ordinary output truncates and obscures the summary. | Piped the response directly to bounded `jq` projections. | Add `status --summary` and rely on paginated queries for detailed facts. |
| Understand why visual design still warned | `go run ./cmd/change-saga validate --json app.saga` | Current criterion-to-design and criterion-to-Item-to-code paths did not satisfy the direct-code rule for a visual fragment. | A fully traversable implementation graph still emitted four warnings. | Added truthful direct code evidence to each fragment. | Include the required evidence shape and a suggested bounded query in the warning, or explicitly document why transitive implementation evidence is insufficient. |

## Handoff facts

- No production code was changed.
- No passing run was created for the two proposed lifecycle cases.
- All 15 active authored cases were rerun successfully.
- The four owned visual-fragment warnings were eliminated with direct evidence.
- Final app-wide counts must be taken after merging the other two disjoint
  quality branches into this workspace.
