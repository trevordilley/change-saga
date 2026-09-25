# Technical inventory: read, coverage and reconciliation implementation

Status: implemented on `feature/inventory-query-coverage-claude` for today's
Component/System records and deck Items. Record intent, persisted evidence IDs,
Item selections and data entities come from the records contract and are wired
separately (see [Waiting on the records contract](#waiting-on-the-records-contract)).
The [design](technical-inventory-design.md) and
[policy contract](technical-inventory-policy-contract.md) remain the intent;
this note describes delivered behavior only.

## Shared projection: `internal/inventoryview`

Pure, read-only and rebuilt per snapshot. Nothing here is persisted or cached.

- `Build(document, inventory) *Index` indexes declared links only: implementation
  and review deck Items' documentation pins, and System member pins of every
  revision.
- `Index.Uses(target, UseOptions)` returns reverse uses with their role
  (`implementation_item`, `review_item`, `system_member`), feature/review,
  deck/slide/Item URNs, the pin and its status, and the declared `path` from
  the using owner's pin down to the target. Upward traversal follows only
  *current* owner revisions; superseded owner revisions are listed with
  `owner_current: false` but not traversed. `DepthCut`, `CycleCut` and
  `Truncated` (10,000-visit budget) are reported separately. `Complete` means
  none happened. `Total` is exact unless `Truncated`.
- `Index.Counts(target)` summarizes uses by role; `Index.FeatureScope(feature)`
  returns every pin reachable from a feature's implementation Items through
  declared member pins *at each saved revision*, with the paths that establish
  it. A missing pinned revision ends its path; nothing newer is substituted.
- `Index.Select(target, pins)` returns the revisions a read answers about (the
  unique current revision, or exactly the scoped pins) plus unresolved reasons:
  `missing_record`, `missing_revision`, `competing_revision_heads`,
  `competing_lifecycle_heads`.
- `RevisionIntent(rev)` reads explicit intent. Legacy revisions read
  `unspecified`; intent is never inferred from code, lifecycle or age.
- `Newness(target, Baseline)` classifies identity introduction relative to a
  named base: `new`, `existing` (including revised identities) or `unknown`
  when the base could not be read. A base where the Saga did not exist is
  `known` and `absent`, so every identity is new there.
- `ResolveSelection(ctx, inventory, Selection, view, resolver)` validates each
  path hop as declared by its predecessor's saved revision, finds evidence by
  its revision-unique ID, requires a non-whole-file subset contained in that
  evidence, and verifies the subset digest at its own commit. It then views
  selected bytes and containing evidence at `view` separately: an edit outside
  the subset reports `outside_subset_changed` (semantic reassessment) while
  the selection stays eligible; an edit inside reports `selected_bytes_stale`.
  Non-current pins make a resolved selection ineligible (`noncurrent_pin`,
  `retired_pin`); missing/conflicted pins and undeclared hops leave it
  unresolved. Legacy references have no persisted ID and are not selectable.
- `Coverage(ctx, inventory, CoverageInput, source)` is the inventory coverage
  measurement described below.

## CLI consumers

| Command | Delivered behavior |
| --- | --- |
| `query inventory` | Existing fields plus per-record `selected` (pins, status, intent), `uses` counts, `newness` with `--against`, `scope_paths` with `--feature`, and `selected_code_health` for non-current scoped revisions. `--intent` and `--new` filter resolvable records only; unresolved candidates are always on `data.unresolved` with a separate `--conflict-cursor/--conflict-limit` page. `--new` without `--against` is `invalid_argument`; with an unreadable base it is `baseline_unknown`. |
| `query inventory-uses` | Pages declared uses of one target, optionally one exact revision, by role and bounded `--depth` (0–8), with completeness flags. |
| `query inventory-coverage` | Tracked, non-Saga text files at `--head` under repeated `--path` prefixes, measured against the unique current revision of each active Component/System, including interaction evidence. Each line counts once and keeps every owner; overlap, stale references, unresolved (conflicted) and excluded (retired/proposed) owners are separate `--state` pages. |
| `reconcile` | New `inventory` section and queue entries (see below). |

Every cursor is bound to the Saga snapshot, source endpoints, operation and
filters; a changed Saga returns `stale_snapshot`. Inventory coverage is labeled
as distinct from implementation-deck coverage and review coverage.

`changeview.BaseSide` exposes the change view's rule for choosing the Saga
snapshot that documents a comparison base (same repository or companion sync
cursor), so newness and reconciliation read the same baseline.

## Reconciliation

`reconcile --against BASE` adds, without writing anything:

- **Definition evidence stale at head** (`inventory_evidence`), classified per
  reference against the base inventory as `regression`, `introduced`,
  `pre_existing` or `baseline_unknown`, listing the implementation Items that
  use the definition through declared paths.
- **Referenced code changed in the comparison** (`inventory_definition`,
  `reassess`) for current references whose lines changed.
- **Competing heads** (`inventory_definition`, `unresolved`).
- **Item documentation pins** that are stale, retired, missing or conflicted,
  routed to the owning slide.
- **Unreferenced definitions** (`inventory_unreferenced`, `needs_user_choice`)
  with no declared implementation-deck use, unless explicitly proposed or new
  against a readable base (listed as `unreferenced_exempt`). An unknown base
  never exempts a definition. The guidance asks the user; nothing is deleted,
  retired, reclassified or attached to a feature.

## Measurements

App Saga, 5 inventory records, 3 runs each on a shared, loaded machine
(wall-clock includes process start, snapshot hashing and Git):
`query inventory` 0.57–0.76 s, 26 MB RSS; `--feature` 0.40–0.71 s;
`--against` 0.86–1.18 s, 31 MB; `inventory-uses --depth 8` 0.18 s, 24 MB;
`inventory-coverage` (whole repository, 36,679 lines in two prefixes) 0.48–0.81 s,
49 MB. For comparison `query context --feature` took 0.90–1.20 s.

Synthetic projection benchmarks (`internal/inventoryview/bench_test.go`;
n Components, n/8 eight-member Systems, one Item per record):

| n | Build index | Deep uses of one Component | Use counts for every record | One feature scope |
| --- | --- | --- | --- | --- |
| 100 | 0.09 ms, 169 KB | 4 µs | 0.09 ms | 10 µs |
| 1,000 | 3.1 ms, 1.9 MB | 6 µs | 0.9 ms | 65 µs |
| 8,000 | 41 ms, 15 MB | 11 µs | 5.8 ms | 1.7 ms |

The first run measured 296 ms to build the 8,000-record index because pin
status used the inventory's linear `Find`; the index now answers status from
its own map (a parity test keeps it equal to `LinkStatus`). No cache was
added: at these sizes snapshot hashing, Saga loading and source resolution
dominate, not the in-memory index.

## Waiting on the records contract

Records contract commit #1 (`1fa8db15`) supplies `EffectiveIntent`,
`EvidenceByID`, `requirements.ResolveSelection` with stable reason codes, Item
`selections`, data entities with holders and relationships, and
`documentation_view`. After the parent imports it, this slice will: read
explicit intent; replace the evidence lookup seam with `EvidenceByID`; call the
records structural check and layer pin, byte and containing health on top;
follow holders and relationship destinations as declared edges; and count
eligible Item selections as inherited implementation-deck coverage with the
Item → path → selected-location provenance. Until then no selection is
persisted, so inherited coverage is zero rather than inferred from pins.

## Verification

`go test ./internal/inventoryview` and
`go test ./internal/cli -run 'TestInventory|TestReconcil|TestSkill|TestQueryGolden|TestInstallSkill'`
pass. The CLI fixtures commit a Saga inside its source repository with an
absent-Saga commit, an unreadable-Saga commit, a base and a head, and cover
filters, scope paths, conflicted records under filters, both cursor pages,
coverage overlap and staleness, and reconciliation debt classes. The full race
suite was not rerun; `TestTopLevelHelpDescribesIncrementalAdoption` is a known
baseline failure.
