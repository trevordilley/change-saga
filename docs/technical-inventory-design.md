# Technical inventory: implementation design deck

Status: proposed design, not an implemented feature. The primary artifact is
the **Proposed technical design** section of the existing `technical-inventory`
implementation deck in `app.saga`. The user explicitly chose to develop the
implementation deck during architecture planning. Its earlier five slides
retain the current implementation; six appended slides identify the proposal.

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

The diagrams use public slide/Item commands. The complete-slide transaction
requires code evidence on every Item, so it cannot truthfully publish these
code-free proposal Items today. No dummy mappings were added to bypass that
constraint. Thirty-eight Item-to-criterion relations explain intent, not delivery.

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

## Still to design

These are reviewable design slices, not a complete implementation handoff.
Next slices must specify the data-entity/relationship contract and ER notation,
the concrete selection wire/storage contract, comparison/filter query contracts,
reverse-index invalidation and performance, proposal-aware validation,
compatibility, and implementation sequencing. No public API, schema, dependency
or product implementation changed.

In particular, retain strict coverage for implemented explanations while making
unimplemented proposals explicit. Decide how implemented baseline and proposed
successor selection interact with concurrent revision heads; the diagram is
not a merge/conflict-resolution algorithm.

## Checks

- Public query readback preserved all story parents/history and unchanged
  proposed lifecycle. The first slice's 17 relation pins were current; the
  second slice's 21 Item-to-criterion explanations were also read back at one
  snapshot with all pins current.
- `validate --json`: valid, with two warnings for proposal slides lacking
  Item-linked code. These are intentional evidence gaps, not implementation.
- `visual-qa`: both three-slide slices pass at 1280x720 and 1024x576, raw and actual
  reviewer surfaces, with no mechanical findings. Rendered slides were inspected.
- Read-only Chromium checks: all three slides navigate; exact code and canonical
  definitions open; story criteria are accessible; Escape restores focus;
  reload preserves the selected slide; 390px touch definition navigation works
  without horizontal document overflow. The complete diagram is scaled on
  mobile; the marked-place menu provides the readable semantic entry points.
- Second-slice read-only Chromium checks exercise all three slides' code and
  story drawers, Escape/focus, permalink reload, and 390px touch navigation from
  the real System to its record-store Component and back to the original slide.
- `go test ./internal/coverage ./internal/coderesolve` passes for the existing
  foundations (1.012s and 1.007s respectively). This does not test the proposed
  resolver, which is not implemented.
- Documentation links and whitespace checks pass. No Go implementation changed;
  the full Go/browser regression suites were not rerun for this authoring slice.

Local screenshots and the repeatable read-only interaction check are under
`.devswarm-temp/inventory-design/`, including `qa-erd/`, `qa-boundaries/`,
`qa-revisions/`, `existing-code-desktop.png`, `definition-narrow.png`, and
`erd-narrow.png`. The second slice adds `qa-selection/`, `qa-coverage/`,
`qa-impact/`, `selection-code-desktop.png`, `selection-definition-narrow.png`,
`selection-narrow.png`, and `check-scope-preview.cjs`. These QA outputs are not
Saga records or committed assets.
