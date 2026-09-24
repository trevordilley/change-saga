# Technical inventory: cold handoff check

## Experiment

Assess whether a fresh implementation agent can recover the intended feature
from repository artifacts, without the design conversation or a manager
retelling the architecture. This is a documentation usability check, not a
review verdict or proof of implementation.

- Input commit: `02869a7d` on `feature/review-ux-improvements`.
- Independent workspace: `feature/inventory-cold-handoff`.
- Entry points supplied: `app.saga`, the `technical-inventory` feature/deck,
  repository guidance and the source-compatible CLI executable.
- Bounded assignment: reconstruct the feature, then plan entity revision
  lifecycle work and its interfaces to evidence, coverage and query consumers.
- No design briefing, conversation-history retrieval, implementation or Saga
  mutations. The assessment-only restriction is not a general policy against
  workers maintaining `app.saga` during implementation.

The worker was asked for its reconstruction, current/proposed distinction,
exact story/slide/code anchors, implementation sequence, consumer dependencies,
acceptance tests and unresolved questions. The manager independently checks
the result against the criteria below. No separate agent's positive opinion
counts as acceptance or approval.

## Evaluation criteria

These checks were recorded before receiving the worker's reconstruction:

1. Recover the purpose: a shared, cross-feature technical vocabulary serving
   Architects and Developers, reused by implementation and review decks.
2. Distinguish Systems, Components and data entities; describe an authored,
   selective ERD including logical payloads and persisted records, with holding
   resources separate from data and transformations separate from associations.
3. Preserve identity, immutable proposal history and exact saved revision pins.
   Separate proposed/implemented intent, active/retired lifecycle, comparison-
   relative newness, code currency and review approval.
4. Recognize that existing Component/System authoring and navigation do not
   implement proposal state, ERD entities or inherited subset coverage today.
5. Explain explicit selected evidence paths: only the chosen subset contributes,
   totals deduplicate but retain owners, and missing/conflicted links remain
   visible rather than granting whole-System coverage.
6. Plan a bounded implementation with named existing code boundaries and
   concrete tests, while identifying shared format/query/consumer contracts.
7. Escalate unresolved compatibility, wire encoding and cross-worker boundaries
   instead of inventing them or silently changing product semantics.

A useful result is not “the worker asked no questions.” It is that the worker
recovers the intended behavior, asks the right boundary questions and can act
once those boundaries are settled. One cold read is evidence about this handoff,
not a benchmark of all agents or proof that the documentation is complete.

## Results

**Outcome: successful architectural handoff; not yet a delivery-ready contract.**
The worker recovered the intended feature and produced a sensible lifecycle
implementation plan without the original conversation. The manager compared
its reconstruction against all seven checks above: each was addressed. This
does not demonstrate autonomous implementation or validate the future behavior.

The worker independently identified:

- The authored ERD and shared canonical inventory, including logical data versus
  holding resources and diagram/directory convergence on the same definition.
- Same-identity implemented → proposed successor → implemented history, explicit
  baseline pins, independent lifecycle/newness/health, and no implied approval.
- The actual Component/System-only foundation and the proposed status of ERD,
  revision intent, inherited subset coverage, roadmap filters and reverse uses.
- The selected path's real record-store example (14–40 within 14–96), exclusion
  of unselected members and independent selected-byte versus containing-code health.
- A bounded sequence: settle compatibility; extend records and validation;
  add public proposal/implementation authoring; expose one shared revision
  projection; integrate separately owned query, evidence and UI consumers.

It cited `inventory-proposed-and-existing:r3`,
`inventory-scoped-deck-coverage:r2`, `inventory-query-technical-context:r2`
and related exact criteria/Items, checked referenced source, and proposed
tests for immutable transitions, rejected evidence, retries, competing heads,
subset containment, overlap, drift, independent filters and snapshot cursors.
Those tests are a proposed implementation plan, not tests of shipped behavior.

### Gaps requiring a manager decision

The cold read exposed three specific seams inside the already-deferred format
gate. Source inspection by the manager confirmed the first two:

1. **Historical pin authoring:** current `requireDocumentation` accepts new
   links only when globally current; `WriteTechnical` permits retaining an
   unchanged existing member pin. That is not a contract for creating a new
   link into a saved historical design view. Define eligibility for new
   historical pins and when they can supply view-scoped coverage rather than
   merely remain readable. See `internal/cli/inventory.go:131` and
   `internal/requirements/inventory_mutation.go:60`.
2. **Implementation evidence source view:** `Resolver.Author` validates bytes
   at the supplied commit; it does not by itself establish currency at a
   selected delivery head. Define and record the source view used by an
   implemented transition. Do not let one worker interpret “resolvable” as
   historical existence while another assumes current delivery evidence.
   See `internal/coderesolve/resolve.go:96` and `internal/cli/inventory.go:90`.
3. **Mixed entity/relationship intent:** the design deliberately permits an
   implemented entity to contain a proposed relationship. Specify whether
   transition validation applies to the entity's evidence, each implemented
   edge, or both, without automatically promoting proposed edges.

Version/encoding, stable evidence-ID adoption, legacy reading/migration and
finite limits remain explicit format deliverables. These are shared contracts,
not choices individual workers should invent. Helper organization, deterministic
sorting and fixture names can remain local implementation choices.

### Discoverability and readiness

The detailed design note was discoverable by filename but was not named by a
queried deck Item. It contains important baseline, legacy-intent and relationship
rules. The rollout slide's `compatibility` Item now explicitly names
`docs/technical-inventory-design.md`; that note links to this assessment and its
open decisions. This is a discoverable repository-path pointer, not a new
renderer hyperlink feature or a fake code-evidence mapping.

The older Component/System document describes its first pass; its ERD exclusion
does not override the newer explicitly proposed design. Complete-slide
transactions still require evidence on every Item: code-free entity authoring
alone will not finish proposal-deck authoring. Keep that integration gap in the
work plan; do not fill it with invented code mappings.

The worker reported a complete read of 13 proposed stories, 14 slides, 59 current
proposal relations and five canonical inventory definitions at snapshot
`sha256:70d80d98be1e22825128ee8bcaa70c1de5c9f33011b3185cdb9e4e20435f2886`.
Its feature audit returned exit 8, `ready=false`: 107 errors (52 missing Item
evidence, 31 missing criterion explanations, 24 missing Item intent), seven
informational cross-feature links and no unresolved conflicts. Those delivery
gaps remain visible; passing this cold read does not waive them. The worker
inspected source/tests but did not run Go/browser suites or reverify old visual
claims. No work items existed to define implementation ownership at the input
commit. A manager must supply that boundary before assigning code work.

## Minimum next task packet

The next bounded assignment is **format and compatibility contract**, not
“implement entity lifecycle.” Its packet should contain:

- The exact starting commit and source-compatible CLI, plus
  [the implementation design](technical-inventory-design.md) and this assessment.
- `inventory-proposed-and-existing:r3` (explicit-status, evidence-distinction,
  history, same-identity-successor, evidence-backed-transition),
  `inventory-query-technical-context:r2` (truthful-results, scoped-roadmap), and
  `inventory-design-to-review:r2` (compare-plan, visible-evolution).
- Deck targets `technical-inventory-record-contract` (revision/links),
  `technical-inventory-query-contract` (scope/page) and
  `technical-inventory-rollout` (compatibility/records/conflicts), under the
  canonical `urn:change-saga:change-saga:slide:` prefix.
- Explicit ownership of the shared schema/model/query contracts; downstream
  owners consume the agreed projection/resolver rather than implementing
  competing interpretations. No UI or coverage implementation is delegated by
  that assignment. Current extension points are `internal/requirements/inventory.go`,
  `inventory_mutation.go` in that directory, `internal/coderef/coderef.go`,
  `internal/cli/inventory.go` and `internal/cli/query_inventory.go`.
- Required output: concrete record/selection/query shapes, compatibility matrix,
  migration/retry behavior and finite bounds; decisions for all three seams
  above; a shared baseline/proposal/delivery fixture and pass/fail cases. Do not
  publish a changed format without implementation, tests, spec and release note
  together, as required by CONTRIBUTING.
- Completion boundary: manager reviews the public contract before lifecycle
  authoring and its consumers proceed. No guessed version migration, automatic
  repins, inferred implementation, feedback resolution or review approvals.

This packet supplies scope, ownership and decision authority. It does not need
to repeat the product architecture that the worker already recovered correctly.

## Evidence-currency repair

The manager independently found two stale selectors in the five earlier
current-implementation slides at the input commit:

- `technical-inventory-authoring:item:validate`: the referenced validation
  range changed when a missing-review-deck guard was added.
- `technical-inventory-verification:item:records`: inserting that regression
  test changed a range that included public authoring/query tests.

Source comparison confirmed the explanations still held. Public
`replace-coverage` repaired the two evidence records with focused current
ranges, including the new guard and regression. The existing snapshot-cursor
reference retained its original commit, range and digest; the separate model
test record was untouched. The replaced files remain recoverable in Git.
No visual, story, definition, approval or review-comment record was changed.
The independent workspace continued reading the original input commit.

Checks after the repair:

- `go test ./internal/requirements ./internal/cli -run '^TestInventory' -count=1`
  passes (requirements 0.500s; CLI 3.516s), run through a tracked one-shot.
- Public `query slide-diffs` readback at one snapshot reports all seven
  selectors across these two Items current, none stale.
- `validate --json` is valid with the same two intentional warnings for the
  code-free ERD/revision proposal slides. These are not implementation evidence.
- `reconcile --against 02869a7d --json` after the repair reports no queue entries
  for `technical-inventory`, with 23 remaining repository-wide health gaps.
  The source comparison has identical base/head; this checks current evidence
  against the working-tree Saga, not new product-code coverage.
- The rollout slide's description-only change passes `visual-qa` at 1280x720
  and 1024x576, raw and reviewer surfaces; the reviewer rendering was inspected.
  Local screenshots remain under `.devswarm-temp/inventory-design/qa-rollout/`.
- Documentation links pass: 225 repository links across 86 files; 31 external
  links skipped. Whitespace checks pass.
- No renderer/product code or diagram geometry changed. No interactive browser
  regression or complete repository test suite was rerun for this assessment.

The assessment workspace was archived after its clean worktree and absence of
tracked processes were confirmed. Its worktree remains recoverable; nothing
was merged or deleted.
