# Technical inventory: first design-deck slice

Status: proposed design, not an implemented feature. The primary artifact is
the **Proposed technical design** section of the existing `technical-inventory`
implementation deck in `app.saga`. The user explicitly chose to develop the
implementation deck during architecture planning. Its earlier five slides
retain the current implementation; three appended slides identify the proposal.

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

The diagrams use public slide/Item commands. The complete-slide transaction
requires code evidence on every Item, so it cannot truthfully publish these
code-free proposal Items today. No dummy mappings were added to bypass that
constraint. Seventeen Item-to-criterion relations explain intent, not delivery.

## Still to design

This is the first reviewable slice, not a complete implementation handoff.
Next slices must specify the data-entity/relationship contract and ER notation,
selected evidence paths and coverage resolution, comparison/filter queries,
reverse usages, proposal-aware validation, compatibility, and implementation
sequencing. No public API, schema, dependency or product implementation changed.

In particular, retain strict coverage for implemented explanations while making
unimplemented proposals explicit. Decide how implemented baseline and proposed
successor selection interact with concurrent revision heads; the diagram is
not a merge/conflict-resolution algorithm.

## Checks

- Public query readback preserved all story parents/history and unchanged
  proposed lifecycle; all 17 new relation pins are current.
- `validate --json`: valid, with two warnings for proposal slides lacking
  Item-linked code. These are intentional evidence gaps, not implementation.
- `visual-qa`: all three slides pass at 1280x720 and 1024x576, raw and actual
  reviewer surfaces, with no mechanical findings. Rendered slides were inspected.
- Read-only Chromium checks: all three slides navigate; exact code and canonical
  definitions open; story criteria are accessible; Escape restores focus;
  reload preserves the selected slide; 390px touch definition navigation works
  without horizontal document overflow. The complete diagram is scaled on
  mobile; the marked-place menu provides the readable semantic entry points.
- Documentation links and whitespace checks pass. No Go implementation changed,
  so the full Go/browser regression suites were not rerun for this authoring slice.

Local screenshots and the repeatable read-only interaction check are under
`.devswarm-temp/inventory-design/`, including `qa-erd/`, `qa-boundaries/`,
`qa-revisions/`, `existing-code-desktop.png`, `definition-narrow.png`, and
`erd-narrow.png`. These QA outputs are not Saga records or committed assets.
