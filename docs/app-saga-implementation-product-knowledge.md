# App Saga implementation evidence: product knowledge

Status: canonical implementation/code-evidence pass for
`app.saga/___features/format.feature` and
`app.saga/___features/requirements.feature`. The source baseline is commit
`58541107888381e0d7f0332bee4f1ad353b0399e` on 2026-09-20. This pass changes
only those two feature trees and this audit; it does not change product source
code or any other feature.

## Outcome

The two owned feature slices now have reviewer-oriented Implementation
sections:

| Feature | Deck | Slides | Items | Exact code references | Current accepted criteria with code paths |
| --- | --- | ---: | ---: | ---: | ---: |
| `format` | `format-implementation` | 1 | 2 | 6 | 4 / 4 |
| `requirements` | `product-knowledge-implementation` | 4 | 13 | 51 | 52 / 52 |
| **Total** | **2** | **5** | **15** | **57** | **56 / 56** |

Each Item is related with a current, revision-pinned `explains` relation to
the exact acceptance criteria it realizes. Each Item description names the
matching design landmarks, and the existing `addresses` relations connect
those same criteria to those landmarks. The resulting supported traversal is:

```text
criterion ──addresses──> precise design landmark
    └───────explains───> implementation Item ──owns──> exact code/test references
```

The CLI's traceability response presents these as two criterion-rooted paths;
it does not currently persist or return a direct Item-to-landmark edge. The
alignment is therefore explicit in the shared criterion plus the Item's
landmark-specific description, not an invented unsupported relation.

## Implementation model

### Release visibility (`format`)

`release-visibility` explains the implementation of the four landmarks in the
owned design:

- `flag-records` covers stable flag identity, feature/story targets, explicit
  `on`/`off`/`retired` lifecycle, and retained retirement history through
  `requirements.AddFlag`, `SetFlagState`, and the focused flag-gating test;
- `release-projection` covers the `off-visible` behavior through the
  `livingapp` app projection and tests for direct gates, feature-wide gates,
  and `implemented_not_enabled`.

### Overview and vocabulary (`requirements`)

`overview-vocabulary` separates three runtime responsibilities:

- `overview-projection` composes name, pitch, description, current terms, and
  durable feature navigation while lifecycle history remains inspectable;
- `term-index` owns versioned term shape, story/record/source links, reverse
  presentation, and code-reference currency;
- `vocabulary-suggestions` derives optional newly introduced domain words from
  added declarations while excluding known terms, renames, and already
  referenced declarations.

### Story, prototype, and design lifecycle (`requirements`)

`knowledge-lifecycle` maps the independent graphs rather than flattening them:

- `story-history` covers immutable complete revisions, concurrent heads,
  criterion identity, citations, lifecycle events, and retained retirement;
- `stable-move` covers feature relocation with stable identity and relation
  validation;
- `prototypes` covers digest-verified local HTML and allowlisted external
  sources;
- `prototype-currency` covers pinned annotations, stale endpoints, and
  optional unlinked growth;
- `design-relations` covers typed design-to-requirement links, exact content
  and revision pins, bidirectional queryability, and derived staleness.

### Evidence traversal (`requirements`)

`evidence-paths-runtime` distinguishes the two directions:

- `traceability-projection` composes current persona, design, test, review
  target, and code paths; it labels missing/stale states and supports reverse
  lookup from an exact source range;
- `impact-projection` maps owned changed atoms back to requirements, test
  cases, and people, while preserving unowned changes and active-persona gaps.

### Quality evidence (`requirements`)

`quality-evidence-runtime` documents the implemented quality model without
claiming that this Saga has adopted or passed those tests:

- `test-definitions` covers criterion pins, ordered stable steps, one expected
  result, and complete definition revisions;
- `run-currency` covers immutable results pinned to test/evidence/source
  heads, stale earlier passes, visible failures, and the rule that progress is
  not proof;
- `quality-policy` covers required kinds and the `satisfied`, `excluded`, and
  `missing` projection. The authoritative source for an exclusion is a
  citation on the coverage-exception record, not a field on the quality policy.

## Reference selection

The references intentionally use named Go types/functions and focused tests
from the code/test audit. They avoid generated outputs, dependency locks,
temporary fixtures, broad package directories, and any file under `app.saga`
as implementation proof. The largest reference is
`(*session).criterionInputs` because that named function is the canonical
composition boundary for forward/reverse traceability; narrower helpers would
omit the cross-axis behavior the Item explains.

All references were authored by `cover --batch` against `HEAD`. The CLI
resolved `HEAD` to the full baseline commit and stored inclusive line ranges
plus content digests. A structured dry run resolved every record before the
write, then the same batch created 15 evidence files atomically.

## Traceability verification

The seven accepted owned stories were queried individually with
`query traceability --requirement <id>`. Every current criterion returned at
least one design target, one implementation Item in `review_targets`, and one
exact source location in `code_evidence`:

| Story | Criteria | Missing design | Missing implementation Item | Missing code evidence |
| --- | ---: | ---: | ---: | ---: |
| `controlled-release` | 4 | 0 | 0 | 0 |
| `design-intent` | 7 | 0 | 0 | 0 |
| `evidence-traversal` | 10 | 0 | 0 | 0 |
| `evolve-product-knowledge` | 7 | 0 | 0 | 0 |
| `overview-and-terms` | 11 | 0 | 0 | 0 |
| `prototype-feedback` | 6 | 0 | 0 | 0 |
| `quality-evidence` | 11 | 0 | 0 | 0 |

Forward inspection of `evidence-traversal:criterion-code` returns the precise
`criterion-paths` design landmark plus the `traceability-projection` Item and
its four exact Go references. Reverse inspection with
`--ref HEAD:internal/livingapp/compose.go#L350-L508` returns the seven criteria
explained by that Item and no unlinked code evidence.

## Coverage and honest gaps

After authoring, all 57 owned references are current: none is stale or
remapped. Feature-scoped health is `20/20` for `format` and `204/204` for
`requirements`.

App-wide status is design `18/18`, stories `25/25`, personas `25/25`, quality
`2/146`, and health `409/410`. Story/persona totals are code-bearing targets,
not accepted-story counts; the 15 new Items increase those totals without
changing the requirement population.

The gaps are deliberate and visible:

- Quality remains `2/146`. The 57 references include focused Go tests as
  implementation evidence, but no owned Saga quality test case, verification
  relation, or run was created. A source test is not silently promoted into a
  recorded passing quality result.
- The one app-wide health gap is the pre-existing stale
  `two-sides-sidebar.json#1` reference under `reviewer-app`. It is outside this
  task's feature boundary and was not repaired.
- Traceability still reports delivery-readiness blockers such as missing work
  items or immutable delivery evidence where applicable. Implementation-deck
  code references explain current code; they do not fabricate planning or
  merge evidence.
- There is no legal persisted relation from an implementation Item directly
  to a report-design landmark. The current relation matrix supports Item to
  story/criterion and work-item to design/criterion. This pass uses the legal
  criterion-centered paths and records the missing direct correlation below.

## CLI rough spots

Every entry records an observed authoring issue using the requested shape.

### Requirement ownership is absent from the requirements query

- **Intended goal:** enumerate accepted stories and criteria in only the two
  owned features through the supported structured read API.
- **Exact command:**
  `/tmp/change-saga-implementation-product-knowledge query requirements --saga app.saga --limit 200`
- **Observed behavior:** each requirement contains identity, heads, current
  revision, and lifecycle, but no feature ID or feature URN.
- **Impact:** feature scoping cannot be performed from this response alone;
  an automation must join against `status`, inspect storage paths, or issue a
  query for every already-known story ID.
- **Workaround:** enumerated the story packages under the two owned feature
  roots, then queried each ID for its current revision and lifecycle.
- **Smallest product improvement:** include `feature` on every requirement row
  and accept `--feature ID` on `query requirements`.

### Visual IDs collide globally without preflight or suggestions

- **Intended goal:** create a requirements implementation slide named
  `evidence-traversal` after creating the deck's earlier slides and Items.
- **Exact command:**
  `/tmp/change-saga-implementation-product-knowledge add-slide --feature requirements --deck product-knowledge-implementation --id evidence-traversal --title 'Forward evidence and reverse impact' --intent trace --layout diagram --takeaway 'The same adjacent graph supports criterion evidence paths, source reverse lookup, and changed-code impact without fabricated links.' --source docs/.impl-traversal.svg app.saga evidence-traversal`
- **Observed behavior:** the command failed with
  `slide id "evidence-traversal" is invalid or already used` because a design
  fragment already owns that app-global ID. Earlier commands in the shell
  sequence had already committed successfully.
- **Impact:** the namespace rule is not discoverable from `add-slide --help`,
  and a multi-command authoring sequence is only partially atomic.
- **Workaround:** renamed the slide to `evidence-paths-runtime`, validated the
  partial Saga, and continued.
- **Smallest product improvement:** make help state that all addressable visual
  IDs are app-global and add `--dry-run`/`--json` to `add-slide` so callers can
  preflight the exact ID before any surrounding mutations.

### Create commands lack structured output and a shared batch

- **Intended goal:** create 2 decks, 5 slides, and 15 Items deterministically
  and retain their canonical URNs for subsequent code and relation mutations.
- **Exact command:**
  `/tmp/change-saga-implementation-product-knowledge add-item --feature requirements --slide overview-vocabulary --id term-index --kind region --element-id term-index --label 'Term record and reverse index' --description 'Implements term-shape, bidirectional-links, and vocabulary-currency with versioned term definitions, story/source links, and resolved code health.' app.saga`
- **Observed behavior:** the create commands emit human-oriented `Added`,
  `Target`, and `Next` lines and expose no `--json` or multi-record batch flag.
- **Impact:** automation must parse prose or reconstruct predictable URNs, and
  a later failure leaves the earlier independently valid records committed.
- **Workaround:** used explicit stable IDs, reconstructed canonical URNs from
  the documented grammar, and ran `validate --json` after each authoring group.
- **Smallest product improvement:** add one atomic structured batch for
  deck/slide/Item creation and return the same mutation envelope as newer
  `--json` commands.

### Exact code selection is line-based rather than symbol-based

- **Intended goal:** pin durable named functions and focused tests in one
  atomic evidence batch.
- **Exact command:**
  `/tmp/change-saga-implementation-product-knowledge cover --batch docs/.impl-cover.json --dry-run --json app.saga`
- **Observed behavior:** the first dry run rejected record 1 because
  `internal/requirements/app_records_test.go#L136-L189` exceeded the file's
  172 lines. The structured failure was precise and the batch correctly wrote
  nothing, but only the first invalid record was returned.
- **Impact:** the author must manually translate symbol boundaries into
  inclusive lines, and several invalid spans require repeated dry runs or
  out-of-band file-length inspection.
- **Workaround:** resolved each named function/test boundary from source,
  narrowed four spans, reran the dry run to `ok: true` with 15 records and 57
  references, then executed the same batch without `--dry-run`.
- **Smallest product improvement:** support stable selectors such as
  `--symbol package.Func` / `--test TestName` and accumulate all independent
  batch validation failures in one structured response.

### Relation authoring has no atomic batch

- **Intended goal:** connect 15 Items to all 56 precise acceptance criteria
  with pinned `explains` relations.
- **Exact command:**
  `/tmp/change-saga-implementation-product-knowledge relation add --json --feature requirements --id impl-term-index-explains-term-record --type explains --from urn:change-saga:app:slide:overview-vocabulary:item:term-index --to urn:change-saga:app:story:overview-and-terms:criterion:term-record --rationale 'The term-index implementation realizes the term-record obligation represented by the term-shape design landmark.' app.saga`
- **Observed behavior:** `relation add` has useful structured output and pins
  both mutable endpoints, but accepts only one relation per invocation. Adding
  56 relations required 56 full load/lock/write cycles and could stop midway.
- **Impact:** large but conceptually atomic traceability updates are slow and
  expose partial state to later commands if any record fails.
- **Workaround:** generated deterministic IDs from Item and criterion IDs,
  stopped on the first error, counted all 56 files, and validated the complete
  Saga immediately afterward.
- **Smallest product improvement:** add `relation add --batch FILE|- --dry-run`
  with all-or-nothing validation and the existing JSON mutation envelope.

### Traceability cannot correlate a design landmark with its implementation Item

- **Intended goal:** inspect the complete requirement → design → implementation
  → code chain for one criterion.
- **Exact command:**
  `/tmp/change-saga-implementation-product-knowledge query traceability --saga app.saga --criterion urn:change-saga:app:story:evidence-traversal:criterion:criterion-code --limit 20`
- **Observed behavior:** the response correctly returns design targets,
  `review_targets`, code evidence, and paths, but returns separate
  `criterion → landmark` and `criterion → Item → code` paths. The v5 relation
  matrix has no legal Item-to-landmark edge.
- **Impact:** consumers can prove both paths are current but cannot ask which
  implementation Item realizes which of several design landmarks on the same
  criterion without reading the Item description.
- **Workaround:** used one focused Item per implementation concern, named the
  exact design landmarks in its description, and linked both the landmark and
  Item to the same atomic criteria.
- **Smallest product improvement:** add a typed non-coverage relation from an
  implementation Item to a report-design target, or return an explicit
  criterion-mediated `design_realized_by` correlation in traceability output.

### Feature-scoped status retains an app-wide stale summary

- **Intended goal:** confirm that the two owned feature slices have no stale
  relations or references.
- **Exact command:**
  `/tmp/change-saga-implementation-product-knowledge status --json --feature format app.saga`
- **Observed behavior:** feature health is correctly `20/20`, while the top
  level `summary.stale` remains `1` because it includes the unrelated stale
  `reviewer-app` reference.
- **Impact:** a consumer reading only the summary can incorrectly conclude
  that the scoped feature contains stale evidence.
- **Workaround:** treated `coverage.areas.health` as the scoped result and
  inspected `stale_references` plus each assignment's feature ownership.
- **Smallest product improvement:** either scope the top-level summary with
  `--feature` or label it `app_summary` and add a separate `scope_summary`.

### Reference inventory cannot filter or report feature ownership

- **Intended goal:** prove every new reference in the two owned features is
  current without mixing it with unrelated evidence.
- **Exact command:**
  `/tmp/change-saga-implementation-product-knowledge references --json app.saga`
- **Observed behavior:** the command reports useful aggregate totals and owner
  URNs, but has no `--feature` filter and rows do not carry feature ownership.
- **Impact:** owned reference counts require knowledge of the slide IDs or a
  separate join against the Saga tree.
- **Workaround:** filtered owner URNs for the five newly authored slide IDs and
  separately confirmed 57 current, 0 remapped, and 0 stale owned references.
- **Smallest product improvement:** accept `--feature` and include `feature` on
  every reference row.

## Verification commands

All Saga mutations used the repository-local binary freshly built from
`./cmd/change-saga`; the relevant final checks are:

```text
/tmp/change-saga-implementation-product-knowledge validate --json app.saga
/tmp/change-saga-implementation-product-knowledge status --json app.saga
/tmp/change-saga-implementation-product-knowledge status --json --feature format app.saga
/tmp/change-saga-implementation-product-knowledge status --json --feature requirements app.saga
/tmp/change-saga-implementation-product-knowledge references --json app.saga
/tmp/change-saga-implementation-product-knowledge query traceability --saga app.saga --requirement <owned-story> --limit 100
/tmp/change-saga-implementation-product-knowledge query traceability --saga app.saga --ref HEAD:internal/livingapp/compose.go#L350-L508 --limit 100
```

Validation passes with no schema errors or warnings. Targeted Go test results
also pass for:

```text
./internal/applayout  ./internal/saga          ./internal/requirements
./internal/prototypes ./internal/quality       ./internal/livingapp
./internal/readiness  ./internal/coverage      ./internal/impact
./internal/vocabulary ./internal/server
```

`git merge main` reported `Already up to date` at `585411078883`. The full
post-merge Saga validation, status assertions, stale-relation query, reference
inventory, all seven story traceability queries, and the targeted Go tests were
then rerun successfully. All five slide SVGs were rendered at 1280 pixels and
visually checked for clipping, selector placement, and legibility.
