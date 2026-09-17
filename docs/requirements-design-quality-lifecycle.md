# Requirements, Design, Implementation, and Quality in One Change Saga

Status: implementation-ready plan for the next report-container schema evolution.

## Decision

A feature Change Saga is one durable, version-controlled audit log from
inception through completed review. Requirements and quality remain report
surfaces. Prototypes are revisioned interactive HTML experiences or explicitly
allowed external embeds. UX and implementation use focused visual decks; UI
design may use pinned references or embeds; technical design uses the diagram
form appropriate to the relationship. Implementation evidence stays attached
to the smallest authored target that explains it, including v4 Items inside
embedded decks. All resources derive their identity from the one parent Saga ID
and participate in one queryable graph.

The next report-container version should be **v5**. Version 4 is already the
distinct slide-native format, and adding a new `___quality` reserved root to a
document that still declares v3 would make older v3 readers reject a document
whose manifest still claims to be v3. A v5 report reuses existing component
formats instead of rewriting them:

- v2 chapter, section, fragment, landmark, diff, claim, verification, and
  review records remain byte-compatible;
- v3 story, citation, work-plan, and existing relation records remain readable;
- v4 deck, slide, Item, asset, and Item evidence records remain byte-compatible
  inside `___slides/<deck-id>.deck/`; and
- v5 introduces only the relation extensions, coverage exceptions, and quality
  records described below.

This is an evolution of the merged hybrid report/slide model, not a second kind
of Saga and not a conversion of report content into slides.

## Baseline on `main`

This plan was checked against `main` at `56f0caf` (the merge of
`feature/hybrid-report-slide-saga`). The following are implemented and must be
reused rather than rebuilt:

| Existing capability | Current contract | Consequence for this evolution |
| --- | --- | --- |
| One report identity with embedded decks | A v3 `saga.json` owns report content and `___slides/<deck>.deck/` bundles. | Keep one manifest and one Saga ID. Do not add a design Saga or quality Saga. |
| Living requirements | `___requirements/stories/<id>.story/` contains an immutable identity, append-only full revisions, and append-only lifecycle events. | Extend the authoring UX and projections; do not introduce a second requirement model. |
| Interactive prototypes | `internal/prototypes` and the v3 prototype schemas already define HTML, external, and safe embedded sources; immutable revisions; element/text/region/provider selectors; and pinned story annotations. | Expose the existing model through CLI, query, and reviewer surfaces. Do not remodel prototypes as decks. |
| Acceptance-criterion identity | A criterion ID is stable within a story and its URN is `urn:change-saga:<saga>:story:<story>:criterion:<criterion>`. Removed IDs cannot be reused. | Keep this identity rule and make criterion mutations create story revisions. |
| Pinned relations | v3 relations have stable IDs, explicit endpoint URNs, revision/content pins, active/superseded state, and stale diagnostics. | Extend the endpoint matrix and source-digest support rather than create ad hoc link arrays. |
| Visual semantic nodes | v4 Deck -> Slide -> Item is a flat, independently mergeable representation; Items have selectors, reading order, and stable URNs. | Treat Decks, Slides, and Items as eligible visual-design targets. Do not copy requirement prose into them. |
| Exact slide diff ownership | Only Items may own `40-e` exact line/event diff records. | Preserve Item-only slide ownership. Deck and Slide traces reach diffs through explicit containment scope. |
| Hybrid traceability | Active, current `explains` relations already yield accepted criterion -> visual target -> Item -> diff paths and reverse lookup by diff or committed source head. | Preserve the v1 query and `explains` semantics. Add a separate design axis instead of relabeling review explanation as design. |
| Readiness projection | `internal/readiness` separates requirement, plan, and delivery axes; progress is not delivery evidence. | Add design and quality details as evidence-rich states, not a combined percentage. |
| Review UI | The report is the default Saga surface; embedded decks are a separate deck/thumbnail surface; Code Diff, Coverage, activity, comments, and slide approvals already exist. | Add Requirements and Quality report sections and trace drawers. Do not blend slide content into report chapters. |
| Atomic storage/query patterns | The store lock, exclusive creation, package rename, snapshot-bound cursors, strict JSON, real-directory checks, and deterministic pagination are established. | Every new writer and query must use these patterns. |

Current limitations that this evolution intentionally closes are: criterion
editing is only available by supplying a complete `story revise`; visual
`explains` links do not pin a visual content digest; `design-coverage` is named
in the living API contract but is not a public operation; requirements and
traceability are CLI-only in the reviewer; and there is no quality domain.

## Product model and boundaries

The sidebar is a stable information architecture, not a phase gate:

```text
Product
  Prototypes
    <prototype>
  Requirements
    <story>
      <acceptance criterion>
Design
  UX
    <flow deck>
  UI
    <reference or embed>
  Technical
    ERD
    System
    Data Flows
      <flow diagram>
Quality
  Test Cases
    <test case>
Implementation
  <implementation deck>
```

`Requirements` itself is its overview; it has no redundant `Overview` child.
The page begins with a concise **Rationale** sourced from the canonical report
overview, followed by the stories. This keeps one authored explanation of why
the change exists.

The navigation order is intentionally static even though authoring order is
not. Prototype-first is the common product-discovery path because interaction
often sharpens the stories. A Saga may instead begin with stories, and later UX,
technical-design, quality, or implementation discoveries may create explicit
new requirement revisions. The changing graph records that feedback without
reordering the interface or erasing the earlier intent.

The authored and derived flow is:

```text
Product discovery                       Report quality
Prototype <----> Story -> criterion <--- Test case -> steps -> run/evidence
                       |                         |
                       | addresses               | verifies
                       v                         v
            UX / UI / technical design ---> Implementation deck ---> diffs
                    (Deck, reference, diagram, Slide, or Item)
```

The arrows are typed claims, not proof. The engine can prove that endpoints,
pins, digests, source identities, and exact diff selectors agree. It cannot
prove that a design is sound, that code implements the design correctly, that
a test is sufficient, or that a passing test establishes the user's intent.

Report and deck presentation remain separate:

- Requirements are rendered as a report section with stories, criteria,
  history, coverage state, and links into decks.
- Technical design and architecture are rendered in embedded slide decks.
- Quality is rendered as a report section with test cases, steps, results, and
  evidence; a test may deep-link to a design Item or source diff.
- The report may contain prose that summarizes design or quality, but that
  prose does not replace the typed resources.
- Standalone v4 Sagas remain slide-only and do not gain v5 report roots.

Prototypes may be temporarily unlinked during exploration. They cannot
contribute to readiness until a current annotation connects them to at least
one story or criterion. Full readiness ultimately requires accepted stories
with explicit criteria even when the prototype came first.

## On-disk layout

The proposed layout keeps merge boundaries per resource and leaves the merged
hybrid deck bundle unchanged:

```text
checkout-refund.saga/
|-- saga.json                              # version: 5; the only Saga manifest
|-- overview.fragment/                     # existing v2 report content
|-- ___requirements/
|   |-- prototypes/
|   |   |-- checkout.prototype/
|   |   |   |-- prototype.json            # existing v3 stable identity
|   |   |   `-- revisions/r2.revision/     # immutable HTML experience
|   |   `-- annotations/                   # pinned prototype -> requirement edges
|   |-- stories/
|   |   `-- refund-window.story/
|   |       |-- story.json                 # existing v3 identity
|   |       |-- revisions/
|   |       |   `-- r2.json                # existing v3 full revision
|   |       `-- events/
|   |           `-- accepted.json          # existing v3 lifecycle event
|   |-- citations/                         # existing v3 records
|   |-- relations/                         # v3 and v5 relation records
|   `-- coverage-exceptions/               # new, one immutable decision per file
|-- ___slides/
|   `-- refund-architecture.deck/           # existing embedded v4 bundle
|       |-- 10-d-....json
|       |-- 20-s-....json
|       |-- 20-s-....svg
|       |-- 30-i-....json
|       `-- 40-e-....json                  # exact diffs remain Item-owned
|-- ___workplan/                           # existing v3 plan, optional
|-- ___quality/
|   |-- policies/
|   |   `-- refund-cutoff-kinds.json        # per-criterion required test kinds
|   `-- test-cases/
|       `-- refund-before-deadline.test/
|           |-- test-case.json             # immutable identity
|           |-- revisions/
|           |   `-- r1.json                # full definition and ordered steps
|           |-- events/
|           |   `-- active.json            # lifecycle history
|           |-- evidence/
|           |   |-- test-code.json         # exact test diff mapping
|           |   `-- implementation.json    # exact implementation diff reference
|           `-- runs/
|               `-- ci-20260917.json        # immutable run/result event
|-- ___claims/                             # existing claims
|-- ___verifications/                      # existing verifications
`-- ___review/                             # existing review overlay
```

`___quality` is absent when the capability has not been adopted. An absent root
is `not_adopted`; an existing root with no test cases is `adopted_empty` and is
not quality-ready. All directories must be real directories and all records
regular files. New packages use the existing Saga writer lock, bounded record
sizes, exclusive creation, and atomic package publication.

The v5 manifest adds no mutable aggregates:

```json
{
  "$schema": "https://changesaga.dev/schema/v5/saga.schema.json",
  "version": 5,
  "id": "checkout-refund",
  "title": "Refund window",
  "source": {
    "repository": "https://github.com/acme/checkout.git",
    "base": "main",
    "head": "feature/refund-window"
  }
}
```

## Requirements and acceptance-criterion identity

### Normative identity rules

1. A story ID is stable for the semantic story and is never recycled.
2. A criterion ID is stable within that story. Its stable URN does not contain
   a revision ID.
3. A story revision is a complete snapshot. A relation to a story or criterion
   must also pin the exact story revision containing that criterion.
4. Editing criterion wording without changing the obligation preserves the
   criterion ID and creates a new story revision. Every prior relation becomes
   stale until explicitly refreshed because its revision pin no longer matches.
5. A materially different obligation receives a new criterion ID. The old ID is
   omitted from the new revision and remains historical. An omitted criterion
   ID can never be reintroduced, matching current validation.
6. Criteria do not have an independent acceptance lifecycle. They inherit the
   current story lifecycle and are active only while present in the unique
   current story revision. This prevents contradictory states such as an
   accepted criterion inside a rejected story. Criterion add/edit/remove
   commands are safe conveniences that create a full story revision.
7. Coverage exclusion is not criterion lifecycle. It is an explicit,
   revision-pinned decision on one coverage axis and never removes the
   criterion from requirements.

Example existing v3 revision, unchanged on disk:

```json
{
  "$schema": "https://changesaga.dev/schema/v3/story-revision.schema.json",
  "version": 3,
  "id": "r2",
  "story": "urn:change-saga:checkout-refund:story:refund-window",
  "parents": [
    "urn:change-saga:checkout-refund:story:refund-window:revision:r1"
  ],
  "title": "Allow an eligible refund before the deadline",
  "statement": "As a buyer, I can request an eligible refund before the 30-day deadline.",
  "priority": "must",
  "citations": [],
  "acceptance_criteria": [
    {
      "id": "before-deadline",
      "statement": "At 29 days 23:59:59 after settlement, an eligible request is accepted."
    },
    {
      "id": "at-or-after-deadline",
      "statement": "At 30 days or later, the request is rejected with the documented reason."
    }
  ],
  "created_at": "2026-09-17T18:00:00Z"
}
```

### CLI authoring

Keep the implemented commands and their semantics:

```text
change-saga story add SAGA --id ID --revision REV --event EVENT ...
change-saga story revise SAGA --story STORY --revision REV --parent HEAD ...
change-saga story set-state SAGA --story STORY --event EVENT --parent HEAD --state STATE
```

Add criterion-focused wrappers. Each wrapper loads the unique current story
revision, applies exactly one criterion mutation, writes a caller-named full
story revision, and refuses a stale `--parent`:

```text
change-saga criterion add SAGA \
  --story urn:change-saga:checkout-refund:story:refund-window \
  --parent urn:change-saga:checkout-refund:story:refund-window:revision:r1 \
  --revision r2 --id at-or-after-deadline \
  --statement "At 30 days or later, reject with the documented reason."

change-saga criterion revise SAGA --story STORY --criterion CRITERION \
  --parent REVISION --revision NEW_REVISION --statement TEXT

change-saga criterion remove SAGA --story STORY --criterion CRITERION \
  --parent REVISION --revision NEW_REVISION --reason TEXT
```

`remove` records the reason in the mutation result and commit guidance; the
history is the parent/child revision change, not an in-place tombstone. A
subsequent command cannot reuse the removed ID.

Human and AI workflows use the same store and validation path:

- Human: `story revise --edit` or `criterion revise --edit` opens a complete
  temporary revision in `$EDITOR`, then validates and commits it atomically.
- Structured/AI: every mutation accepts `--from FILE|-`, `--request-id`,
  `--json`, and an explicit parent head. Explicit IDs are required; the CLI does
  not silently generate semantic IDs for automation.
- A mutation response returns the stable resource URN, created revision/event
  URNs, paths, replay status, and current heads. It never records an `author`
  field; provenance is derived from Git.
- Multi-head stories must be reconciled with the existing multi-parent
  `story revise` and `story set-state` operations before a criterion wrapper can
  make a single-head edit.

Queries add a criterion-first projection without changing stored records:

```text
change-saga query requirements --saga SAGA --state accepted
change-saga query requirement-history --saga SAGA --requirement refund-window
change-saga query criteria --saga SAGA --story refund-window --status active
change-saga query criterion-history --saga SAGA --criterion CRITERION_URN
```

`criterion-history` returns every revision in which the ID appeared, its
statement in that revision, added/changed/removed classification, graph heads,
and the inherited story lifecycle at each point.

## Visual design and requirement coverage

### Relation evolution

Do not add a parallel `designs` relation. Extend the existing `addresses`
relation to accept a Deck, Slide, or Item source in addition to the existing
report design targets. Keep `explains` for reviewer-facing explanation and
backward-compatible hybrid traceability; an old `explains` edge must not start
counting as technical-design coverage merely because a new reader opens it.

New v5 relations add `scope`:

- `self` means only the source target is asserted to address the requirement.
- `descendants` is legal only for Deck or Slide sources and explicitly permits
  traversal through contained Slides/Items to Item-owned evidence.
- Item and report-design sources must use `self`.
- A visual `addresses` source must pin `from_content_digest`; its story or
  criterion target must pin `to_revision`.

Example Item-to-criterion relation:

```json
{
  "$schema": "https://changesaga.dev/schema/v5/relation.schema.json",
  "version": 5,
  "id": "deadline-guard-addresses-cutoff",
  "type": "addresses",
  "from": "urn:change-saga:checkout-refund:slide:decision-flow:item:deadline-guard",
  "to": "urn:change-saga:checkout-refund:story:refund-window:criterion:at-or-after-deadline",
  "scope": "self",
  "from_content_digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "to_revision": "urn:change-saga:checkout-refund:story:refund-window:revision:r2",
  "rationale": "The guard and rejection transition implement the deadline boundary.",
  "state": "active",
  "created_at": "2026-09-17T19:00:00Z"
}
```

Visual content digests are deterministic and exclude diff/review overlays:

- Item digest: canonical Item manifest plus the selected slide asset bytes.
- Slide digest: canonical Slide manifest, entrypoint bytes, and ordered Item
  manifest digests.
- Deck digest: canonical Deck manifest and ordered Slide digests.

A selector or asset edit therefore makes an Item relation stale. Adding a diff,
comment, approval, verification, or review event does not.

### Relation graph and type matrix

Persisted relation direction remains source -> target. Query paths may be
displayed from requirement outward for readability.

| Relation | Legal source | Legal target | Required pins | Traversal/readiness meaning |
| --- | --- | --- | --- | --- |
| `refines` | story or criterion | story or criterion | story revision pins when mutable | Navigational decomposition; not evidence. Existing acyclic rule remains. |
| `addresses` | report design target, Deck, Slide, or Item | story or criterion | source digest and target story revision | Counts current requirement-to-design coverage. `descendants` may traverse Deck/Slide containment. |
| `implements` | work item | report design target or criterion | work-item revision and design digest/story revision | Planning path only; progress never proves delivery. |
| `explains` | Deck, Slide, or Item | story or criterion | target story revision; v5 visual digest recommended | Existing review-explanation and reverse-code path. It does not count as design coverage. |
| `verifies` | claim, verification, or test case | criterion | criterion revision; test case also pins its revision | Claim/test intent. A test case needs a current passing run before quality readiness. |
| `supersedes` | resource | same resource kind | pins as applicable | Explicit replacement; no automatic lifecycle mutation. Existing acyclic rule remains. |
| `conflicts_with` | story or criterion | story or criterion | pins as applicable | Symmetric diagnostic; no inferred winner. |

Derived edges are not persisted relation records:

| Derived edge | Source -> target | Rule |
| --- | --- | --- |
| `contains` | Deck -> Slide -> Item | Exact parent IDs in validated v4 records. |
| `owns_diff` | Item -> diff URI | Valid current `40-e` record in the Item's embedded bundle. |
| `has_quality_evidence` | test case revision/run -> quality evidence | Exact URN reference in validated v5 records. |
| `matches_item_diff` | implementation-under-test evidence -> Item | Exact canonical diff URI equality in the same current source comparison. |

### Design coverage states

For each criterion in the unique current revision of an accepted story, return
all relation records and classify the criterion on the design axis as:

- `covered_direct`: one or more current `addresses` relations target the
  criterion directly;
- `covered_broad`: coverage exists only through a relation to the story; this
  retains current broad-link semantics but is visibly less precise;
- `excluded`: an active, current design exception exists;
- `gap`: no current relation or exception exists;
- `stale`: links exist, but their story revision or source digest is stale;
- `invalid`: an endpoint, revision, digest, relation scope, or graph is invalid;
  or
- `conflicted`: the story/relation source has multiple semantic heads.

Full design coverage means every accepted, active criterion is
`covered_direct`, `covered_broad`, or `excluded`, with no stale, invalid, or
conflicted record. Precision is reported separately: full criterion precision
requires every non-excluded criterion to be `covered_direct`. Thus a broad
story link is never hidden, but it does not become an unexplained percentage.

Example query:

```text
change-saga query design-coverage --saga checkout-refund.saga \
  --criterion urn:change-saga:checkout-refund:story:refund-window:criterion:at-or-after-deadline \
  --include stale,excluded,paths
```

The result includes `status`, `precision`, relation URN, source target, pinned
and current revisions/digests, stale reasons, exception rationale, and every
candidate path.

## Transitive requirement-to-design-to-diff semantics

A valid implementation trace path is exactly:

```text
accepted current criterion
  <- addresses@current-story-revision -
visual Item@current-content-digest
  -> owns_diff -> canonical current line/event diff URI
```

or, when explicitly scoped:

```text
accepted current criterion
  <- addresses(scope=descendants) - Deck or Slide@current-content-digest
  -> contains* -> Item -> owns_diff -> canonical current diff URI
```

The query must never infer an `addresses` edge from proximity, slide reading
order, matching words, code paths, work-item links, Git blame, or an AI model.
It may suggest candidate links in a separate non-readiness operation, but a
candidate is not coverage until persisted by an author.

Full transitive implementation coverage means every accepted, active,
non-excluded criterion has at least one path whose relation pin, visual digest,
Item, evidence file, repository identity, base identity, product head identity,
and exact line/event selector are current. Global all-atoms-mapped remains a
separate omission invariant. Both conditions are required for the default
`ready_for_review` gate; neither proves correctness.

Many-to-many behavior is explicit:

- one Item may address several criteria;
- one criterion may be addressed by several Items or decks;
- one exact diff may appear in several paths and is returned once per path with
  a `shared_diff` diagnostic; overlap is permitted, as it is today;
- a story-level relation expands to each criterion in the pinned revision and
  is marked `precision: broad`;
- a Deck/Slide relation reaches child Items only with `scope: descendants`;
  there is no hidden containment assumption; and
- multiple valid paths are corroborating mappings, not ambiguity.

`ambiguous` is reserved for facts the engine cannot reduce deterministically:
multiple story/test revision heads, multiple current run heads, contradictory
active exceptions, or a source selector that resolves to zero/multiple visual
elements. Ambiguity blocks readiness and returns every competing head or
resolution candidate. It is not resolved by timestamps.

The existing v1 `traceability` response and reverse `--diff`/`--commit` lookup
remain byte-compatible. The expanded graph is exposed through API v2:

```text
change-saga query traceability --api-version 2 --saga SAGA --axis design-delivery
change-saga query traceability --api-version 2 --saga SAGA --diff SAGA_DIFF_URI
change-saga query traceability --api-version 2 --saga SAGA --commit 40_HEX_HEAD
```

Each v2 path is an ordered array of typed hops and includes `status`,
`precision`, `relation`, `evidence_file`, and stale/invalid diagnostics. Commit
lookup retains the current limitation: it is available only when the active
comparison has a committed source head, never for `WORKTREE`.

## Quality domain

### Stable resources and URNs

Quality records use the same ID grammar and graph rules as stories:

```text
urn:change-saga:<saga>:test-case:<test-case>
urn:change-saga:<saga>:test-case:<test-case>:revision:<revision>
urn:change-saga:<saga>:test-case:<test-case>:step:<step>
urn:change-saga:<saga>:test-case:<test-case>:event:<event>
urn:change-saga:<saga>:test-case:<test-case>:evidence:<evidence>
urn:change-saga:<saga>:test-case:<test-case>:run:<run>
urn:change-saga:<saga>:quality-policy:<policy>
urn:change-saga:<saga>:coverage-exception:<exception>
```

A test case has an immutable identity, append-only complete revisions, and an
append-only lifecycle-event graph. Lifecycle states are `proposed`, `active`,
`deprecated`, and `retired`. Definition lifecycle is distinct from execution
result. Test steps have IDs stable within the test case; their array order is
execution order, and a removed step ID cannot be reused for a different action.

Example test case revision:

```json
{
  "$schema": "https://changesaga.dev/schema/v5/test-case-revision.schema.json",
  "version": 5,
  "id": "r1",
  "test_case": "urn:change-saga:checkout-refund:test-case:refund-at-deadline",
  "parents": [],
  "title": "Reject a refund at the exact deadline",
  "coverage_kinds": ["negative", "edge"],
  "automation": "automated",
  "preconditions": [
    "An eligible settled purchase exists at exactly 30 days old."
  ],
  "steps": [
    {
      "id": "submit-request",
      "action": "Submit a refund request for the purchase.",
      "expected_result": "The request is rejected."
    },
    {
      "id": "inspect-reason",
      "action": "Inspect the response reason.",
      "expected_result": "The documented refund-window-expired reason is returned."
    }
  ],
  "expected_result": "No refund is created and the rejection is auditable.",
  "created_at": "2026-09-17T20:00:00Z"
}
```

`coverage_kinds` is a nonempty unique subset of `positive`, `negative`, and
`edge`. It describes what the test is intended to cover; the engine does not
infer it from prose. `automation` is `manual`, `automated`, or `hybrid` and does
not affect truth by itself.

A test case links to one or more criteria through current `verifies` relations:

```json
{
  "$schema": "https://changesaga.dev/schema/v5/relation.schema.json",
  "version": 5,
  "id": "refund-at-deadline-verifies-cutoff",
  "type": "verifies",
  "from": "urn:change-saga:checkout-refund:test-case:refund-at-deadline",
  "to": "urn:change-saga:checkout-refund:story:refund-window:criterion:at-or-after-deadline",
  "scope": "self",
  "from_revision": "urn:change-saga:checkout-refund:test-case:refund-at-deadline:revision:r1",
  "to_revision": "urn:change-saga:checkout-refund:story:refund-window:revision:r2",
  "rationale": "The case checks the inclusive rejection boundary and reason.",
  "state": "active",
  "created_at": "2026-09-17T20:05:00Z"
}
```

### Evidence and runs

Quality evidence is immutable and typed:

```json
{
  "$schema": "https://changesaga.dev/schema/v5/quality-evidence.schema.json",
  "version": 5,
  "id": "deadline-test-code",
  "test_case": "urn:change-saga:checkout-refund:test-case:refund-at-deadline",
  "test_revision": "urn:change-saga:checkout-refund:test-case:refund-at-deadline:revision:r1",
  "role": "test_implementation",
  "diffs": [
    "saga-diff://v1/line?base=...&end=88&head=...&path=internal%2Frefund_test.go&repository=...&side=new&start=41"
  ],
  "verifications": [],
  "citations": [],
  "supersedes": [],
  "created_at": "2026-09-17T21:00:00Z"
}
```

`role` is one of:

- `test_implementation`: exact diff selectors for test code; these participate
  in global diff coverage under the test-case target;
- `implementation_under_test`: exact implementation selectors; for a complete
  quality-to-implementation path, each selector must exactly match current
  Item-owned evidence rather than silently creating a second design owner; or
- `execution_artifact`: existing verification and/or citation URNs such as a CI
  result, screenshot, measurement, or recorded inspection.

An evidence record must contain at least one item across `diffs`,
`verifications`, and `citations`. `supersedes` names older evidence records;
current projection is graph-based, not latest-timestamp-wins.

Runs are append-only result events with parent heads, so concurrent results are
visible instead of arbitrarily ordered:

```json
{
  "$schema": "https://changesaga.dev/schema/v5/test-run.schema.json",
  "version": 5,
  "id": "ci-20260917-1",
  "test_case": "urn:change-saga:checkout-refund:test-case:refund-at-deadline",
  "test_revision": "urn:change-saga:checkout-refund:test-case:refund-at-deadline:revision:r1",
  "parents": [],
  "source": {
    "repository": "https://github.com/acme/checkout.git",
    "base": "0123456789abcdef0123456789abcdef01234567",
    "head": "product-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  },
  "result": "passed",
  "summary": "Boundary case passed in CI on Linux.",
  "command": "go test ./internal/refund -run TestRefundAtDeadline",
  "evidence": [
    "urn:change-saga:checkout-refund:test-case:refund-at-deadline:evidence:deadline-test-code"
  ],
  "executed_at": "2026-09-17T21:10:00Z"
}
```

Run results are `passed`, `failed`, `blocked`, or `skipped`. A current quality
result requires one run head, the current test revision, the current source
comparison identity, and resolved current evidence. A past pass becomes stale
when the test definition or product identity changes. A failed, blocked,
skipped, multi-head, or stale run does not satisfy quality readiness.

### Quality coverage and comparison with design

Default full quality coverage for an accepted criterion requires:

1. at least one active, current test case linked directly to the criterion;
2. at least one linked test whose declared kinds include `positive`;
3. every additional kind required by an explicit criterion quality policy;
4. a unique current `passed` run for each required kind; and
5. resolved evidence on the current source comparison.

The initial default requires `positive`; `negative` and `edge` remain visible
gaps but block only when a per-criterion policy requires them. Policies are
immutable records under `___quality/policies/`, avoiding a shared manifest map:

```json
{
  "$schema": "https://changesaga.dev/schema/v5/quality-policy.schema.json",
  "version": 5,
  "id": "refund-cutoff-kinds",
  "criterion": "urn:change-saga:checkout-refund:story:refund-window:criterion:at-or-after-deadline",
  "story_revision": "urn:change-saga:checkout-refund:story:refund-window:revision:r2",
  "required_kinds": ["positive", "negative", "edge"],
  "allowed_automation": ["automated", "manual"],
  "supersedes": [],
  "rationale": "The boundary has distinct success, rejection, and exact-cutoff risks.",
  "created_at": "2026-09-17T20:10:00Z"
}
```

There is one unsuperseded policy head per criterion/story revision. New-story
authoring should prompt for required kinds. Migration projects `positive` when
no policy exists and does not rewrite historical v3 story revisions.

The quality query returns `covered`, `missing_kind`, `not_run`, `failed`,
`blocked`, `stale`, `excluded`, `invalid`, or `conflicted` for each required
kind. It also reports observed positive/negative/edge tests even when not
required.

```text
change-saga query quality-coverage --saga SAGA --include tests,runs,evidence,paths
change-saga query coverage-comparison --saga SAGA --criterion CRITERION_URN
```

`coverage-comparison` is a matrix, not a score:

```json
{
  "criterion": "urn:change-saga:checkout-refund:story:refund-window:criterion:at-or-after-deadline",
  "design": {
    "status": "covered_direct",
    "targets": ["urn:change-saga:checkout-refund:slide:decision-flow:item:deadline-guard"]
  },
  "implementation": {
    "status": "covered",
    "diffs": ["saga-diff://v1/line?..."]
  },
  "quality": {
    "status": "covered",
    "required_kinds": ["positive", "negative", "edge"],
    "observed_kinds": ["positive", "negative", "edge"],
    "tests": ["urn:change-saga:checkout-refund:test-case:refund-at-deadline"]
  },
  "diagnostics": []
}
```

The derived requirement-to-quality-to-diff paths are:

```text
criterion <- verifies - test case@revision
  -> quality evidence(test_implementation) -> test diff

criterion <- verifies - test case@revision
  -> quality evidence(implementation_under_test)
  -> exact URI match -> design Item -> implementation diff
```

A manual case may have execution artifacts without a test-code diff. That can
satisfy an allowed manual quality policy, but the query honestly reports that
no Requirement -> Quality -> test-code Diff path exists.

### Quality CLI

```text
change-saga quality test add SAGA --id ID --revision r1 --event proposed --from test.json
change-saga quality test revise SAGA --test TEST_URN --parent REV --revision r2 --from test.json
change-saga quality test set-state SAGA --test TEST_URN --parent EVENT --event active --state active
change-saga quality policy set SAGA --criterion CRITERION --story-revision REV \
  --require positive --require negative --require edge
change-saga quality evidence add SAGA --test TEST_URN --test-revision REV \
  --role test_implementation --diff SAGA_DIFF_URI
change-saga quality run record SAGA --test TEST_URN --test-revision REV \
  --parent RUN_HEAD --result passed --evidence EVIDENCE_URN --command COMMAND
```

All support `--from FILE|-`, `--request-id`, and `--json`; batch evidence
creation preflights and validates the complete set before the first write.

## Coverage exceptions

Intentional exclusions are first-class immutable decisions, never a missing
link interpreted as intent:

```json
{
  "$schema": "https://changesaga.dev/schema/v5/coverage-exception.schema.json",
  "version": 5,
  "id": "docs-only-no-visual-design",
  "axis": "technical",
  "criterion": "urn:change-saga:checkout-refund:story:refund-window:criterion:support-playbook",
  "story_revision": "urn:change-saga:checkout-refund:story:refund-window:revision:r2",
  "rationale": "The obligation changes only an existing operational playbook template.",
  "citations": ["urn:change-saga:checkout-refund:citation:support-policy"],
  "supersedes": [],
  "created_at": "2026-09-17T20:30:00Z"
}
```

`axis` is one of `prototype`, `ux`, `ui`, `technical`, `quality`, or
`implementation`. The feature policy requires every axis by default, but an
axis may be explicitly inapplicable—for example, a repository-schema migration
may not require UI design. There is no exception from exact changed-source
accounting: documentation-only work still ends at its documentation diff. An
exception must pin the current story revision, include a rationale and at least
one citation, and have a single unsuperseded head per criterion/axis. A revision
change makes it stale.

```text
change-saga coverage-exception add SAGA --axis technical --criterion CRITERION \
  --story-revision REV --rationale TEXT --citation CITATION
change-saga coverage-exception supersede SAGA --exception EXCEPTION --with NEW_EXCEPTION
```

Exceptions appear as a separate count and row. They can satisfy the configured
gate, but never increment `covered` and never disappear into a denominator.

## Readiness semantics

Readiness remains a deterministic projection, not a persisted status, approval,
or percentage. The v2 response returns every axis and blocker:

| Gate | Required facts | Not inferred |
| --- | --- | --- |
| `requirements_ready` | Every in-scope story has one revision head and lifecycle head; accepted stories have active criteria; no invalid/stale identity graph. | Stakeholder agreement beyond recorded lifecycle. |
| `product_ready` | Every retained prototype is linked to a current story/criterion; every accepted criterion has required prototype coverage or a current exception. | That prototype behavior is desirable or feasible. |
| `design_ready` | Every accepted criterion has current UX, UI, and technical coverage or a current exception on each applicable axis; no invalid/conflicted links. | Design quality or correctness. |
| `implementation_trace_ready` | Every accepted criterion has a valid current path through its required artifacts to an exact Diff; global diff coverage is separately complete. | That the selected diff implements the criterion correctly. |
| `quality_ready` | Quality is adopted; every accepted criterion has required current test kinds or a current quality exception; required tests have one current passing run with resolved evidence. | Test sufficiency or absence of undiscovered defects. |
| `ready_for_review` | All preceding configured gates, immutable current evidence, no graph conflicts, and no failed current required run. | Reviewer approval. |
| `review_complete` | `ready_for_review` plus required current report/slide/test-case review decisions under review policy. | Merge authorization outside Change Saga. |

For compatibility, a v2/v3 Saga or a v5 Saga whose quality capability is not
adopted keeps the current `peer_review_ready` behavior unless the caller selects
the v5 feature-Saga policy. A newly initialized v5 feature Saga selects that
policy and therefore requires quality before review.

Suggested query:

```text
change-saga query readiness --api-version 2 --saga SAGA --policy feature
```

The summary returns counts for `covered_direct`, `covered_broad`, `excluded`,
`gap`, `stale`, `invalid`, and `conflicted` on each axis, followed by complete
criterion rows and blocker paths. A UI may show compact counts, but must not
replace them with one opaque percentage.

## Reviewer experience

The report remains the entry point. The sidebar always projects the stable
Product, Design, Quality, Implementation order defined above. It does not move
sections as work progresses and therefore never implies a waterfall. Within
Product, Prototypes precedes Requirements because prototype-first is the common
discovery path. Requirements itself is the overview and contains the Rationale;
there is no extra Overview child.

Requirements view:

- the initial view shows only Rationale, story title, description, and criteria;
- lifecycle, revision, identity, conflicts, and Git provenance remain under a
  deliberate details disclosure;
- each criterion has separate Prototype, UX, UI, Technical, Quality, and
  Implementation state chips;
- expanding a chip shows concrete paths, stale pins, exclusions, and gaps;
- a deck/slide/Item target opens the existing slide surface and highlights the
  Item; and
- history compares criterion statements by revision without changing the
  current report snapshot.

Quality view:

- test cases are grouped under linked criteria, with orphaned tests in an
  explicit work queue;
- ordered steps show action and expected result side by side;
- positive/negative/edge labels, lifecycle, current run status, source identity,
  and evidence are visible without opening raw JSON;
- implementation-under-test evidence deep-links through the matching Item to
  the existing code drawer;
- failed, stale, skipped, conflicting, and intentionally excluded states use
  text and icons in addition to color; and
- test-case approval is added as a review target kind, while slide approval
  remains slide-scoped and report approvals remain unchanged.

The top-level Code Diff and Coverage tabs remain source-oriented. Coverage adds
filters for requirement, design Item, test case, and evidence role. Activity
includes story/test lifecycle, run, exception, relation, and review events.
All derived sections and drawers are snapshot-bound; the UI swaps to a complete
new snapshot instead of combining old relations with new diffs.

## Audit history and AI authoring grammar

Git is the outer audit log: it records every committed Saga change alongside
the source change. Saga resources add semantic history inside that log:
immutable identities, append-only revisions and lifecycle events, pinned
relations with rationales, supersession, evidence, and review decisions. A v5
revision created after the initial revision adds a nonblank `change_reason`;
mutation commands expose it as required `--reason TEXT`. Git records the change
set while the resource records why its semantic meaning changed.

The CLI is also the machine-readable grammar for an authoring agent. Extend the
existing commands rather than creating parallel ways to write the same record:

```text
change-saga story ...                 # existing story identity and revisions
change-saga criterion ...             # existing criterion-safe wrappers
change-saga prototype add-html|add-external|revise|annotate
change-saga add-deck --role ux|implementation
change-saga design ...                # UI references and technical diagrams
change-saga quality ...               # test cases, policy, evidence, and runs
change-saga relation ...              # typed, pinned cross-resource edges
change-saga cover ...                 # exact terminal diff ownership
change-saga spec --json               # resources, relations, commands, invariants
change-saga status --json             # blockers and ordered next actions
change-saga validate
```

`spec --json` must describe the living v3/v5 resources and legal endpoint
matrix, not only the legacy report and v4 storage shapes. `status --json`
returns ordered `next_actions`. A deterministic action includes its valid
command shape; an action requiring product judgment, external access, or an
explicit exclusion instead contains one focused question for the author. The
agent repeats inspect, ask or mutate, validate, and re-evaluate until there are
no required current gaps. It must never infer correctness from that fixed point.

## Validation invariants

An implementation must enforce these invariants in both runtime validation and
published JSON Schemas where expressible:

1. Exactly one manifest owns the Saga ID; every local URN uses that ID.
2. V5 is a report container. It may embed v4 deck components but cannot declare
   v4 `presentation` or use a slide-native root.
3. Existing v3 story graphs retain one root, acyclicity, parent reachability,
   explicit multi-head conflicts, and non-reusable criterion IDs.
4. Accepted stories have at least one criterion in every current revision head.
5. Criterion mutations always create a complete story revision and never edit
   an existing revision.
6. Relation endpoint kinds, pins, digest algorithms, scope, and same-Saga rules
   obey the matrix above. Active `refines`/`supersedes` graphs are acyclic and
   `conflicts_with` remains canonical and symmetric.
7. Visual `addresses` pins the current canonical content digest. Review/diff
   overlay changes do not alter that digest.
8. Embedded exact diff evidence remains Item-only. A Deck or Slide reaches it
   only through validated containment and explicit `descendants` scope.
9. Every exact diff URI is canonical, selects a current line/event atom, and
   matches the Saga's repository/base/product-head identity to be current.
10. Test identity, revision, lifecycle, evidence-supersession, and run-parent
    graphs are acyclic and have one root. Multi-head definitions or runs are
    conflicts, not last-write-wins.
11. Every step ID is unique in a revision, every ordered step appears exactly
    once, action and expected result are nonblank, and removed IDs are not
    reused for a different meaning.
12. An active test case has at least one step, at least one `coverage_kind`, and
    at least one active direct `verifies` relation before it can satisfy quality.
13. A current passing run pins the current test revision and current source
    identity and resolves all referenced evidence. `progress=done`, a claim, or
    a past pass cannot substitute for it.
14. `implementation_under_test` selectors must exactly match current Item-owned
    selectors to produce a Quality -> Diff path. Near/overlapping ranges are
    diagnostics, not matches.
15. An exception is current only for its exact criterion story revision and
    axis, with rationale and citation. Competing active exception heads block.
16. Readiness, coverage, source currency, verification, approval, and
    correctness remain separate concepts and separate response fields.
17. Mutation failure leaves no partial files. Reads never write, execute a test,
    fetch a URL, or resolve external content as a side effect.

## Migration and backward compatibility

### Reader/writer matrix

| Document | New reader | Old reader | New writer behavior |
| --- | --- | --- | --- |
| v2 report | Unchanged | Unchanged | Must explicitly upgrade before v3/v5 roots. |
| v3 living/hybrid report | Unchanged, including current v1 queries | Unchanged | Existing commands keep writing v3 until explicit upgrade. |
| v4 slide-native | Unchanged | Unchanged | Never auto-converted to v5; an explicit rewrite may embed decks in a new report Saga. |
| v5 report | Full support | Clean unsupported-version error | May reuse v2/v3/v4 component bytes and write v5 relation/quality records. |

`change-saga upgrade --to 5 SAGA` accepts v3 and stages a complete copy, changes
only the manifest version/schema, validates all existing components under the
v5 composition rules, then atomically publishes. It does not invent quality
tests, design links, exceptions, or coverage policy. V2 may upgrade directly
only by running the same v2 -> v3 structural checks internally. `--dry-run`
reports unsupported records and resulting capability states.

Mixed relation versions are deliberate: unchanged v3 relations remain valid
history, while new or refreshed visual-design/test-case relations use v5.
Existing `explains` relations retain their current descendant trace behavior in
API v1 and are labeled `legacy_review_explanation` in API v2. They do not become
`addresses` automatically. An opt-in migration assistant may propose v5
relations, but only an explicit write persists them.

Downgrade to v3 is allowed only when `___quality` and v5 exceptions are absent
and no v5 relation exists. It must never discard records. V5 component support,
minimum CLI version, schema URLs, `SPEC.md`, `change-saga spec`, the authoring
skill, and changelog must ship together.

Compatibility risks and mitigations:

| Risk | Mitigation |
| --- | --- |
| Version 5 appears to supersede slide-native v4. | Documentation calls v5 a report container and retains explicit v4 slide-native mode; there is no numeric-mode inference. |
| Story-level links look more precise than they are. | Preserve them as `covered_broad`, return expansion paths, and report criterion precision separately. |
| A visual edit silently leaves an old relation current. | Require canonical visual source digests for v5 `addresses`; exclude overlay bytes from digest. |
| Test pass is reused after code or test changes. | Pin both test revision and full source comparison identity. |
| Quality duplicates Item ownership of implementation code. | `implementation_under_test` requires exact equality with Item-owned evidence and is a reference, not another design owner. |
| Existing consumers break on larger query payloads. | Keep API v1 byte shape; add API v2 operations/fields and bounded pagination. |
| Large decks/tests make the report eager and slow. | Requirements/Quality SSR returns bounded summaries; deck, steps, runs, evidence, and paths load by scoped endpoints and snapshot cursors. |
| Central loader/CLI files become merge hotspots. | Implement leaf packages first and serialize only the small registry/dispatcher integration commits. |

## Implementation phases

Each phase is independently reviewable and leaves the repository in a valid,
tested state. Later phases depend on earlier contracts but no phase requires an
unmerged parallel branch to compile.

### Phase 0: freeze the v5 contract

- Add this contract's final decisions to `SPEC.md` and publish closed draft
  schemas for v5 manifest, relation, exception, test case, revision, lifecycle,
  evidence, run, and quality policy.
- Add schema/runtime parity tables and golden fixtures; do not enable v5 writes.
- Freeze visual digest canonicalization and API v2 response schemas.
- Exit: schemas validate examples, v2/v3/v4 regression fixtures are byte- and
  behavior-unchanged, and unknown v5 is still refused by production writers.

### Phase 1: v5 composition and read-only quality domain

- Add v5 manifest loading and `___quality` strict loader/validator in new
  `internal/quality` and `internal/qualityid` leaf packages.
- Extend the composition layer and snapshot fingerprint; expose read-only
  `criteria`, `criterion-history`, `quality`, and `quality-history` queries.
- Exit: hand-authored v5 fixtures load/query; all operations are read-only and
  bounded; old formats remain unchanged.

### Phase 2: requirement authoring ergonomics

- Add criterion wrappers, structured `--from`, editor workflow, optimistic
  parent checks, and complete mutation output.
- Reuse `internal/requirements`, `store.WithSagaLock`, request replay, and
  append-only story revision rules.
- Exit: human/AI commands produce the same bytes, conflicts are explicit, and
  failed batches leave the tree unchanged.

### Phase 3: visual design links and design coverage

- Add v5 relation parsing/writing, expanded `addresses` matrix, `scope`, visual
  digests, stale evaluation, exceptions, and `design-coverage` API v2.
- Reuse existing Deck/Slide/Item targets and embedded bundle loader.
- Exit: every design state and broad/direct distinction has golden query tests;
  no exact diff traversal is required yet.

### Phase 4: transitive design-to-diff traceability

- Generalize the existing hybrid review-evidence index into typed graph hops.
- Add `contains`, `owns_diff`, current/stale selector evaluation, reverse diff
  and commit lookup, shared-diff diagnostics, and API v2 traceability.
- Keep API v1 output unchanged.
- Exit: forward/reverse many-to-many cases are deterministic and full coverage
  follows only explicit, current paths.

### Phase 5: quality mutations and coverage

- Add test-case, policy, evidence, and run writers; extend `verifies` for test
  cases; compute required/observed kinds and Requirement -> Quality paths.
- Integrate test-code evidence with global diff coverage and exact-match
  implementation evidence with Item ownership.
- Exit: quality coverage handles manual/automated cases, stale runs, failures,
  multi-head conflicts, exceptions, and orphaned tests.

### Phase 6: readiness composition

- Extend `internal/readiness` pure inputs/results with evidence-rich design,
  implementation, and quality states.
- Add feature/legacy policies without storing readiness or changing current v1
  peer-review behavior.
- Exit: gate truth tables and blocker paths are exhaustive, deterministic, and
  contain no percentages.

### Phase 7: reviewer UI and migration

- Add derived Requirements and Quality report sections, path drawers, filters,
  test-case review targets, activity entries, accessibility states, and scoped
  lazy endpoints.
- Add atomic `upgrade --to 5`, dry-run/downgrade checks, documentation, skill,
  and a self-reviewing v5 example Saga.
- Exit: browser, accessibility, performance, migration, and no-reload review
  mutation suites pass on small and large fixtures.

Suggested code ownership keeps merge conflicts bounded:

```text
internal/quality       records, load, validate, mutations
internal/qualityid     quality URN parsing/building
internal/requirements  criterion wrappers and v5 relation compatibility
internal/livingapp     graph composition and query DTOs
internal/readiness     pure gate projection only
internal/cli           thin adapters and schema discovery
internal/server        UI/HTTP projection only; no domain rules
```

## Test strategy

1. **Schema parity:** every enum, required field, URI grammar, endpoint matrix,
   transition, size limit, and `additionalProperties: false` rule matches Go.
2. **URN/property tests:** round-trip every new URN; reject wrong Saga IDs,
   noncanonical spellings, malformed nesting, and ID reuse.
3. **History graphs:** linear, branched, reconciled, missing-parent, multiple
   root, cycle, removed-criterion/step reuse, and timestamp-skew cases.
4. **Mutation atomicity:** before/inside-lock validation, request replay,
   concurrent parent changes, batch rollback, symlink/path traversal, and
   record-size limits.
5. **Relation matrix:** every legal/illegal endpoint pair, required pins,
   visual digest changes, overlay-only nonchanges, scope, stale source/target,
   and mixed v3/v5 records.
6. **Coverage graphs:** direct/broad/excluded/gap/stale/invalid/conflicted,
   Deck/Slide descendants, direct Items, shared diffs, overlap, no-match, and
   reverse lookup by diff/commit.
7. **Quality:** ordered steps, coverage-kind policies, manual evidence,
   exact test diffs, implementation exact-match, stale source identity, failed
   and concurrent runs, superseded evidence, orphan tests, and exceptions.
8. **Readiness truth tables:** every gate independently varied; progress and
   approval never substitute for evidence; existing legacy behavior retained.
9. **Migration:** canonical v2, v3 living, v3 hybrid, standalone v4, v5 empty,
   and v5 complete fixtures; dry-run/no-partial-write and downgrade refusal.
10. **Query contracts:** v1 golden bytes, v2 schema discovery, deterministic
    sorting/pagination, cursor snapshot invalidation, bounded response sizes,
    reverse filters, and no-write assertions.
11. **Reviewer/browser:** keyboard and screen-reader labels, non-color status,
    deep links, no-reload comments/approvals, stale snapshot refresh, and lazy
    loading under the existing performance budgets.
12. **Security:** hostile HTML/SVG remains sandboxed, evidence/citation URLs are
    never fetched during load, commands are displayed but never executed by
    queries/viewer, and paths cannot escape the Saga/source roots.

## Lifecycle walkthrough

1. **Inception:** `init --mode report --format 5` creates one empty report Saga.
   Requirements and quality are `not_adopted`; readiness explains that state.
2. **First story:** `story add` creates identity, r1, and proposed event.
   Criterion URNs are stable immediately. Requirements show draft; design and
   quality show explicit gaps, not zero-percent charts.
3. **Acceptance:** a lifecycle event accepts the story. The current revision
   and every criterion become the traceability backbone.
4. **Design:** the author creates an embedded v4-compatible deck, slides, and
   Items, then adds digest-pinned `addresses` relations. Design coverage shows
   direct/broad paths, gaps, and any exceptions.
5. **Revision:** criterion wording changes under the same ID in r2. Every r1
   design/test relation becomes stale. The author refreshes or supersedes links;
   nothing silently follows the new text.
6. **Implementation:** exact current diffs are attached to design Items. The
   trace query derives Requirement -> Design -> Item -> Diff and reports
   unlinked Item evidence and accepted criteria without a valid path.
7. **Quality definition:** test cases with ordered steps and expected results
   are added and linked directly to criteria. The comparison view exposes
   positive/negative/edge gaps beside design gaps.
8. **Verification:** test-code and implementation-under-test evidence are
   recorded, then a run pins the current test revision and source identity.
   Passing results satisfy configured quality kinds; failures remain visible.
9. **Review ready:** requirements, design, implementation traces, global diff
   coverage, and quality gates are all current. The system says why each gate
   passes but does not claim the change is correct.
10. **Completed review:** reviewers inspect report, decks, code, tests, and
    evidence; current required review targets are approved. `review_complete`
    is derived. The same Saga retains its complete inception-to-review history.

## Open decisions and recommendations

| Decision | Recommendation | Reason / consequence |
| --- | --- | --- |
| Container version | Use v5 report mode. | V4 is occupied; a v3 in-place root addition is falsely versioned and breaks old v3 readers. |
| Criterion lifecycle | Inherit story lifecycle; mutate criteria through story revisions. | Avoids two contradictory lifecycle graphs while preserving stable criterion history. |
| Visual design relation | Expand `addresses`; keep `explains` unchanged. | Reuses typed relations and prevents existing review links from silently becoming design claims. |
| Container traversal | Require explicit `scope: descendants`. | Makes Deck/Slide-to-Item inference reviewable and deterministic. |
| Story-level design links | Count as `covered_broad`, but report precision separately. | Preserves current broad semantics without pretending to identify a particular criterion element. |
| Required test kinds | Default `positive`; declare additional negative/edge kinds per criterion policy. | Requiring all three universally is wasteful; inferring them from prose is unreliable. |
| Passing result selection | Parent-head graph, not latest timestamp. | Concurrent CI/manual runs remain visible and reconcilable. |
| Implementation-under-test mapping | Require exact equality with Item-owned diff URIs. | Prevents near-range heuristics from claiming a quality/design connection. |
| Exclusions | Allow cited design/quality exceptions; never delivery exceptions. | Keeps intentional non-applicability explicit without erasing accepted obligations. |
| Query compatibility | Preserve API v1; add API v2 schemas. | Existing agents remain stable while new clients receive typed paths and states. |
| Review unit | Add test-case review targets; keep slide approval slide-scoped. | Review decisions follow the authored unit without changing established slide behavior. |
| Author identity | Continue deriving it from Git. | Human and AI flows share one trustworthy provenance mechanism and persisted schemas avoid unverifiable author claims. |

The only decision that should be revisited before Phase 0 freezes schemas is
whether quality must be adopted for every newly initialized v5 Saga or only for
the `feature` readiness policy. The recommended choice is policy-based: keep
the format usable for documentation/operational Sagas, but make the new-feature
template select `feature` and require quality before review.
