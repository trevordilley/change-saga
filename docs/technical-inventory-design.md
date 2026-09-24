# Technical inventory: implementation design deck

Status: proposed design, not an implemented feature. The primary artifact is
the **Proposed technical design** section of the existing `technical-inventory`
implementation deck in `app.saga`. The user explicitly chose to develop the
implementation deck during architecture planning. Its earlier five slides
retain the current implementation; nine appended slides identify the proposal.

## Decisions captured

- The ERD is an authored, human-consumable architectural explanation, not a
  generated schema dump. Authors choose meaningful entities, fields, keys,
  relationships and composition. Exhaustive columns/properties are not required.
- Logical payloads and persisted records both belong in the data model.
  Resources that carry/store them remain distinct from those entities.
- Diagram and directory lead to canonical entity definitions, then to
  feature-specific implementation slides and exact code. Reuse identity rather
  than copy an entity definition into each feature.
- A proposed successor of an existing entity keeps its stable identity and
  preserves the implemented baseline. Deck versions pin the revision they
  explain; later code evidence must not rewrite the committed proposal.
- Proposal status describes the selected revision. Newness is separately
  relative to a named comparison; neither implies verification or approval.

These decisions have interview provenance and immutable story revisions:
`erd-design-implications:r2`, `erd-entity-context:r2`, and
`inventory-proposed-and-existing:r3`. Their lifecycle remains proposed.

## Read the draft

1. `technical-inventory-design-boundaries`: existing record/query/drawer code
   boundaries and proposed extensions. Five Items have eight narrow source
   references pinned to `e7d312f3826d8a62e80a7b02a0f90949a0cd0fda`;
   three also open the existing canonical Component definitions.
2. `technical-inventory-authored-erd`: the illustrative PDF job/report example
   demonstrates selective detail, resource boundaries and drill-down. It is
   not Change Saga's own data model. The dashed production edge is not a
   foreign key; no unconfirmed cardinality is invented.
3. `technical-inventory-proposed-revisions`: one identity across baseline,
   proposal and evidence-backed implementation, with distinct saved deck pins.
4. `technical-inventory-selected-path`: a real System-to-Component path selects
   only lines 14–40 of the record-store evidence at lines 14–96. Other members
   and unselected lines contribute nothing to that Item's inherited coverage.
5. `technical-inventory-coverage-resolution`: named measurement scope, exact
   selection resolution, unique code totals, complete owner paths and explicit
   unresolved results. Links identify existing resolver/coverage foundations.
6. `technical-inventory-selection-impact`: proposed reverse impact through
   inventory usages, selected-byte drift versus broader semantic reassessment,
   and author-controlled repairs using the existing reconciliation boundary.
7. `technical-inventory-record-contract`: canonical data-entity identity,
   immutable intent, distinct holding resources, owned relationship definitions,
   and shared diagram/directory revision pins.
8. `technical-inventory-query-contract`: scoped proposal/newness filters,
   snapshot-bound pages, on-demand detail and a disposable reverse-usage index.
9. `technical-inventory-rollout`: compatibility, record/authoring, shared
   consumer and end-to-end verification gates; explicit conflict handling.

The diagrams use public slide/Item commands. The complete-slide transaction
requires code evidence on every Item, so it cannot truthfully publish these
code-free proposal Items today. No dummy mappings were added to bypass that
constraint. Fifty-nine Item-to-criterion relations explain intent, not delivery.

## Proposed selection and reconciliation contract

The following specifies the second design slice, not a shipped API or schema:

- Save the Item's explicit entity-revision path, containing evidence identity,
  and chosen source range at its pinned commit. Compute a digest for the chosen
  bytes, not the entire entity's implementation. That digest is selection
  integrity metadata, not a duplicate independent ownership mapping.
- A selection must lie within the referenced evidence at the saved revision.
  Missing references, ambiguous/conflicting intent and unavailable source are
  explicit unresolved results. Never substitute a newer entity revision,
  silently expand a range, or traverse all members just because a System is used.
- Reuse the existing exact-byte resolver for selected content. Pure movement
  can remain current; edits to the selected content require reassessment.
  An edit outside the subset may stale the containing entity reference without
  changing the subset's bytes. Return those two health facts separately: current
  selected bytes do not establish a healthy or semantically accurate entity.
- Preserve the Item → pinned entity path → selected source provenance. Count
  each resolved source location once in the measured union, retaining every
  owner and overlap. Do not award coverage to unselected code. A different Item
  may independently explain that code; exclusion is specific to this selection.
- Name each report's source view/comparison, denominator and code scope.
  Inventory coverage, implementation coverage and PR review coverage remain
  distinct. HEAD currency does not use deletion-side validity as proof of
  current documentation. Missing proposal evidence does not count as implemented.
- Extend the reverse-usage index and reconciliation queue to expose affected
  entity definitions, exact selections, Items and features, with declared paths
  and reasons. Preserve pre-existing debt and unknown baselines. Undeclared
  dependencies remain a visibility gap, not evidence of no impact.
- Byte-stale selections and broader semantic impact require different messages.
  Inspect both; change only definitions, selections and explanations whose
  meaning or evidence no longer holds. Keep old definitions and saved review
  pins intact. Re-run reports and relevant tests; never autoapprove or mark
  feedback resolved as a consequence of a clean report.

The diagrams' code links ground extension points only. The second slice adds
seven exact references pinned to `a0ddce8def26d380a77de0e119892d95abc48274`;
they do not claim an inventory selection resolver already exists. Its real
System and Component links open existing saved definitions through public queries.

Implementation tests to specify with the eventual contract include out-of-range
selection refusal, missing/conflicting paths, bounded traversal, pure source
movement, edits inside versus outside a subset, deletions, overlapping selections,
distinct report scopes, absent baselines, concurrent definition revisions, and
preservation of historical review pins. Query results must expose incomplete
reads instead of silently presenting a truncated graph as complete.

## Proposed record and read contracts

This third slice is an architecture proposal, not new CLI syntax or a published
wire/schema contract. Field names and numeric bounds still require the format
gate below. Its three code links are pinned to `5d6e0d41`; they ground current
extension points, not delivery of the proposal.

### Canonical records and authored ERD

- Keep stable identity, immutable revision history and active/retired lifecycle
  separate. Add explicit proposed/implemented intent to revisions across Systems,
  Components and data entities. Code health remains an independently resolved
  fact. Legacy records without intent report unspecified, not inferred proposed
  or implemented; deliberate authoring supplies the missing assessment.
- A proposed successor names its implemented baseline revision explicitly when
  one exists, and retains that pin separately from revision parents. A wholly
  new proposal explicitly has no implemented baseline. Do not select a baseline
  by timestamp, nearest ancestor or available code alone. Multiple current heads
  remain conflicts; a saved historical pin is still readable but not current.
- Data entities contain purpose and a selective, author-curated set of fields
  and key roles. A holding resource is a separate pinned Component, not a second
  identity for the entity. More than one resource may hold a logical entity;
  references explain their roles rather than forcing a database-table model.
- An entity revision owns outgoing relationship definitions, each with a stable
  owner-local ID and a pinned destination entity revision. Incoming relationships
  are derived; do not duplicate an edge in both endpoint definitions. Changes
  retain the edge ID and append an owner revision. Moving ownership requires
  explicit migration, not silently changing the relationship identity.
- Each relationship declares its meaning (association or production/
  transformation), explanation and proposed/implemented intent, independently
  of the owning entity's existence. Thus an existing entity can propose a new
  relationship without labeling its whole implemented baseline as nonexistent.
  Evidence may support the edge independently of the entity's other code.
- Use crow's-foot endpoints for declared association cardinalities, with a
  readable text equivalent such as `1` or `0..many`. Use labeled directional
  flow arrows for production/transformation; they are not foreign-key lines.
  Unknown cardinality is explicitly unknown, never guessed. Key roles do not
  imply a physical FK constraint unless declared. These are authored notation
  rules, not schema introspection or a generator.
- The application-wide ERD root owns the authored visual and its entity/edge
  bindings, not copies of canonical definitions. Diagram selections and the
  expandable directory resolve the same saved revision pins. A design overlay
  supplies explicit proposed pins against a named baseline view; the canonical
  diagram is not silently rewritten by an unrelated feature proposal. Directory
  membership and omitted visual detail remain visible so a selective diagram
  does not pretend to show the entire model.

### Exact selection storage and conflict behavior

An Item's proposed selection stores a stable selection ID, an ordered path of
canonical target/revision pins, an evidence identity within its owning revision,
and the chosen source commit/path/start/end plus a digest of the selected bytes.
Evidence identities must be stable IDs, not mutable array positions. The
containing reference and its digest remain available to distinguish containing
evidence drift from selected-byte drift. Schema encoding and adoption of IDs
for legacy references belong to the compatibility gate; they are not invented
in the existing records by this authoring change.

Validate every adjacent graph hop at its saved revision and every subset
against the containing reference. An explicit implemented transition appends a
revision with scoped, resolvable evidence and preserves the proposal. Existing
implemented records that later lose evidence retain their asserted intent and
report a health failure; they do not become proposed. Conflicting definitions
cannot be auto-repaired by selecting the newest pin. A deliberate reconciliation
must name all competing parents and the intended resulting definition, using
the existing append-only model. No operation here implies review approval.

### Query contracts and derived index

The read design extends `query inventory` rather than creating a parallel
source of truth. Exact command flags and versioned response fields are deferred
to the public-contract gate; the proposed behaviors are:

1. Select kinds and a feature/design scope through declared links. Resolve the
   intended revision pins in that scope, not only a global latest revision.
   Return the paths that establish membership; shared entities can have several.
2. Filter explicit revision phase independently from comparison-relative
   identity introduction. A newness request without a baseline fails clearly.
   Missing/unreadable baseline is unknown, not an empty inventory or proof that
   every entity is new. Ordinary changes to an existing identity are not new
   entities. Relationship additions remain distinguishable within a revision.
3. Return bounded summaries with identity, selected pin, heads, explicit intent,
   lifecycle, comparison identity, separate code/pin health, scope paths and
   completeness. Matching records and unresolved/conflicting owners have
   separately resumable pages; filters must not hide unresolved candidates.
4. Expand one identity for selected code paths, reverse Item/deck/feature usages
   or history on request. Traverse only declared links with cycle detection and
   explicit depth/result bounds. Report truncation and continuation, not a
   partial result described as complete or an empty page described as no uses.
5. Bind cursors to the Saga snapshot, source view, selected revision context,
   operation and filters. Reject mixed snapshots; restart the read. A derived
   reverse index may cache these records locally but is disposable, never
   committed authoritative state. Key reuse to those inputs and invalidate on
   changes to assets/bindings, inventory, relations, source view or implementation
   of the index. Cache failure falls back to a bounded rebuild or explicit error.

Keep snapshot cost measurable: the existing inventory query pages before code
resolution but still loads the inventory and calculates a snapshot. Pagination
alone does not prove constant-time reads. Benchmark cold and warm enumeration,
detail and reverse-use expansion on increasing record/edge counts; count source
resolutions and peak memory. Do not promise performance gains without those
measurements. No new network service, dependency or hosted index is proposed.

## Implementation gates and remaining decisions

1. **Format/read compatibility:** agree the actual version gate, field shapes,
   stable reference IDs and finite limits in SPEC, schemas and `spec --json`.
   The current reader rejects unknown inventory kinds and unsupported Saga
   versions; adding a directory is not backward compatible. A new reader should
   preserve legacy definitions and pinned history using an explicit read adapter.
   Any opt-in migration must preserve identities, old revisions and review
   records; unsupported old readers must refuse rather than discard content.
   Versioning and release notes follow [the release policy](releasing.md#versioning-policy).
2. **Records/public authors:** implement shared revision intent, data entities,
   owned relationships, ERD bindings and exact selections with validation and
   public authoring commands. Include code-free proposals, evidence-backed
   implementation transitions, explicit parents and safe retry/concurrency tests.
3. **Shared consumers:** implement the path resolver, bounded queries and derived
   reverse uses before connecting the authored ERD directory, existing deck
   drawers, coverage and reconciliation. Retain separate report denominators,
   proposal warnings, byte currency and semantic affectedness. An unreferenced
   non-proposed entity outside a valid newness exception asks the user for a
   reconciliation choice; it is not deleted or attached to an arbitrary feature.
4. **Workflow proof:** fixture-test proposed design → implementation → review →
   documentation reconciliation, including persisted evidence transitions,
   conflicting heads, old pins, subsets and overlapping owners. Exercise actual
   desktop/touch navigation, focus/Escape, offline assets and reload persistence.
   Update documentation and the change review; never synthesize approvals.

Before product implementation, review the proposed relationship ownership,
legacy intent handling and baseline/view composition above, then settle their
exact public encoding and compatibility/migration tests in gate 1. This is the
next concrete slice; no public API, schema, dependency or product code changed
in these design commits. Numerical bounds, an executable format migration and
benchmarks are intentionally not claimed complete.

The [cold handoff assessment](technical-inventory-handoff-check.md) confirms a
fresh agent can reconstruct the intent without this conversation. It also
identifies three format-gate decisions to settle explicitly: authoring new
historical pins in a selected view, the source view required for implementation
evidence, and promotion rules for implemented entities with proposed
relationships. Its task packet bounds that next assignment; the assessment is
not a delivery-readiness or review verdict.

The [view and implementation policy contract](technical-inventory-policy-contract.md)
now resolves those three behavioral decisions: explicit saved-view admission,
original and delivery-commit evidence validation, and independent entity/edge
intent. It defines the first bounded executable policy slice and its acceptance
matrix. Persisted encoding and public writer integration remain behind the
compatibility gate; this is not a silent relaxation of today's authoring rules.

## Checks

- Public query readback preserved all story parents/history and unchanged
  proposed lifecycle. The first slice's 17 relation pins were current; the
  second slice's 21 Item-to-criterion explanations were also read back at one
  snapshot with all pins current. The third slice adds 21 active explanations,
  read back at one snapshot with current pins, and three non-stale code mappings.
- `validate --json`: valid, with two warnings for proposal slides lacking
  Item-linked code. These are intentional evidence gaps, not implementation.
- `visual-qa`: both three-slide slices pass at 1280x720 and 1024x576, raw and actual
  reviewer surfaces, with no mechanical findings. Rendered slides were inspected.
  The third three-slide slice passes the same checks; manual inspection prompted
  shorter labels to preserve space inside the diagram nodes.
- Read-only Chromium checks: all three slides navigate; exact code and canonical
  definitions open; story criteria are accessible; Escape restores focus;
  reload preserves the selected slide; 390px touch definition navigation works
  without horizontal document overflow. The complete diagram is scaled on
  mobile; the marked-place menu provides the readable semantic entry points.
- Second-slice read-only Chromium checks exercise all three slides' code and
  story drawers, Escape/focus, permalink reload, and 390px touch navigation from
  the real System to its record-store Component and back to the original slide.
- Third-slice read-only Chromium checks pass for all three new slides: code and
  story drawers, Escape/focus return, desktop permalink reload and 390px touch
  story drawers/reload/context retention with no horizontal document overflow.
  The 390px canvas still scales the diagram very small and persistent touch
  controls obscure some node text. Readable story drawers do not resolve that
  existing renderer limitation; no mobile-canvas polish is claimed here.
- `go test ./internal/coverage ./internal/coderesolve` passes for the existing
  foundations (1.012s and 1.007s respectively). This does not test the proposed
  resolver, which is not implemented.
- Documentation links and whitespace checks pass. No Go implementation changed;
  the full Go/browser regression suites were not rerun for this authoring slice.
  The third slice's link check covers 223 repository links across 85 files;
  31 external links were skipped. No Go tests were rerun for this docs-only slice.

Local screenshots and the repeatable read-only interaction check are under
`.devswarm-temp/inventory-design/`, including `qa-erd/`, `qa-boundaries/`,
`qa-revisions/`, `existing-code-desktop.png`, `definition-narrow.png`, and
`erd-narrow.png`. The second slice adds `qa-selection/`, `qa-coverage/`,
`qa-impact/`, `selection-code-desktop.png`, `selection-definition-narrow.png`,
`selection-narrow.png`, and `check-scope-preview.cjs`. These QA outputs are not
Saga records or committed assets.
The third slice adds `qa-record-contract/`, `qa-query-contract/`, `qa-rollout/`,
`contract-code-desktop.png`, `contract-stories-narrow.png`, `contract-narrow.png`
and `check-contract-preview.cjs` in the same local QA directory.
