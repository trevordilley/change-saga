# App Saga guided authoring and review design

Status: canonical UX and technical design for the authoring and review wave,
authored against the accepted requirement heads and merged audits on 2026-09-20.

## Scope and intent

This wave owns design under `agent-loop`, `reviewer-app`, and `reviews`, plus
the requirement relations whose source design is stored there. It deliberately
does not change requirements, personas, quality records, implementation decks,
product code, or code evidence.

The design keeps two useful foundations:

- coverage is an actionable omission report with no product verdict; and
- Documentation and Review are separate, lasting information spaces.

It moves exact command spelling, JSON fields, exit codes, and UI placement
under the human contracts those mechanics serve. Technical details remain in
protocol appendices and trust-boundary designs rather than becoming the reason
the feature exists.

## Canonical authoring contract

### First-change onboarding

The required first-change path is intentionally short: establish the Saga and
comparison, choose a reversible feature home, create a visual explanation, and
connect every changed line to the item that explains it. The path ends when the
current change is explained. Personas, stories, design, and test cases appear
afterward as one optional growth step, not as first-run debt.

The workflow distinguishes three states:

1. an actionable omission, such as an exact unexplained range;
2. optional product-knowledge growth; and
3. an evaluation failure, such as an ambiguous comparison or malformed Saga.

Only the third stops the workflow. Stable record identities make the initial
feature choice reversible without breaking review, requirement, comment, or
history links.

### Actionable coverage with no verdict

The primary coverage experience names its scope and presents implementation,
stories, personas, design, quality, and health separately. Each area has
concrete covered and uncovered entries. There is no blended score, traffic
light, celebration, or implicit ship decision.

A gap is a successful report. A report fails only when the system cannot make a
reliable claim. A separate selected-policy question may answer whether named
areas are complete, but it cannot widen its scope or turn the neutral report
into a global gate. Next actions remain stable, focused, inspectable, and safe
to decline.

### Bounded deterministic automation

Delegated work follows one loop: read a bounded snapshot, choose one documented
next action, preview or validate, write with an idempotency identity, and read
the affected projection again. Changed snapshots force replanning. Invalid
batches write nothing. Structured reads use stable envelopes and bounded
pagination, and all authored payloads remain inert data.

Automation may extend an author's reach, but a successful operation is not
evidence that the intended outcome was delivered.

### Parallel delivery coordination

Plans divide work into ordered, dependency-aware waves. Each item has one
objective, expected deliverables, owned surfaces, and an explicit release
condition. Workspace assignment identifies responsibility and progress exposes
activity. Neither one proves delivery.

Only immutable merged evidence releases dependent work. On resumption, a
contributor reads current assignments, dependencies, merged evidence, and
snapshot state before editing. Divergence, overlapping ownership, stale pins,
and conflicting heads are explicit stop conditions. The combined Saga is
validated after each wave.

## Canonical reviewer contract

### Lasting navigation and resumption

Documentation answers what the application is. Review answers what one change
proposes and whether its explanation is sufficient. The selected side,
comparison, chapter or slide, and focused item or file live in stable URLs.
Browser history reconstructs the same context.

Switching sides preserves the nearest meaningful target. When there is no
counterpart, the destination is that side's overview rather than a fabricated
link. Local progress may recommend the first unfinished slide or file, but it
cannot silently change the comparison or mark content complete.

Documentation pages remain free of approval and comment controls. Merged review
history stays reachable from changed documentation without becoming current
product truth.

### Safe local rendering and trust boundaries

Branch-authored content is untrusted even when served locally. The reviewer
binds only to loopback and independently checks Host and browser site context.
Browser-visible identities are Saga-relative; local filesystem paths remain
server-side diagnostics.

Markdown and text become inert application markup. Interactive SVG or HTML is
isolated in an opaque, networkless sandbox with no parent, navigation, popup,
form, download, or ambient-origin authority. Messages cross the boundary only
through a bounded contract that validates the source window, origin, message
kind, target, and payload.

Request, response, decoding, archive, diff-expansion, and concurrency limits
fail closed. Failed or partial artifacts are not marked viewed. All runtime
assets are local, and normal review performs no outbound request.

## Canonical pull-request review contract

### Guided review deck

One pull request has at most one ordered review deck. It is a visual argument,
not a screenshot gallery or reformatted diff. Slides orient, explain, compare,
trace, prove, expose risk, and conclude. Each slide has one reviewer job, one
takeaway, and addressable items linking lasting documentation to exact changed
source.

A session resumes at the earliest slide that is unread, undecided, has an open
thread, or became out of date. Direct slide and item URLs, browser history, and
the selected comparison remain authoritative.

### Decisions, comments, and currency

Approve, request changes, and clear are append-only slide decisions that record
the reviewed head. They never declare the pull request approved. Comments
target a slide or item, and replies and resolution stay within that thread.
Drafts survive failed writes and focus returns to the invoking context after a
successful write.

A decision remains current only while both its slide and every linked code
target still match the reviewed state. Slide edits, evidence changes, repins,
and new heads preserve the old decision but mark it out of date. Comments keep
their historical target instead of silently following later content.

### Omissions and merged history

Review coverage identifies unexplained changed lines, stale or unavailable
item evidence, and stale documentation links. Empty or partial evidence is not
success. Each omission names its exact scope and offers the smallest safe next
authoring action. Complete mapping still does not prove correctness; reviewer
judgment remains human.

Merge freezes the range, deck, comments, decision versions, omissions, and
currency transitions as read-only history. Later documentation edits cannot be
absorbed into the historical review as though they were reviewed. Missing or
rewritten Git history is reported honestly; the application does not invent
attribution or currency.

## Visual and addressable design

The wave adds three SVG design fragments with semantic descriptions and
element-level landmarks:

- an authoring loop separating first-change value, bounded delegation, and the
  merged-evidence gate;
- a local trust map separating untrusted content, reviewer enforcement, and
  protected host authority; and
- a review state map showing session resumption, decision currency, and frozen
  merged history.

Markdown contracts also expose heading landmarks. `addresses` relations pin
those meaningful nodes to current criterion revisions and source content
digests, so later wording or design changes surface as stale rather than
silently drifting.

## Traceability and validation result

Commands were run with the repository CLI (`go run ./cmd/change-saga`) rather
than an installed binary.

- `validate --json app.saga`: valid, with three expected warnings that the new
  visual fragments do not yet have direct code evidence.
- `query relations --state stale`: zero stale relations after repinning the six
  pre-existing active relations affected by revised design content.
- `status --feature agent-loop`: four of four accepted stories have design.
- `status --feature reviewer-app`: the one accepted story owned by the feature,
  `safe-local-review`, has design.
- `status --feature reviews`: the one accepted story, `review-is-pr-deck`, has
  design.
- `query traceability`: zero design gaps across all 51 criteria of
  `bounded-automation`, `coverage-report`, `first-change`,
  `parallel-delivery-plan`, `safe-local-review`, and `review-is-pr-deck`.
- The reviewer navigation design also pins the four relevant
  `observe-or-compare` criteria: separate modes, documentation context, related
  reviews, and the derived label. Its six comparison-engine criteria remain in
  the `comparison` feature and are outside this wave's ownership.

The three visual warnings are intentional. This wave was instructed not to add
or repair exact code evidence; the visuals are requirement-traceable design,
not implementation claims.

## Required next-wave evidence repair

One pre-existing code reference is stale and was intentionally not repaired:

`app.saga/___features/reviewer-app.feature/___design/two-sides-design.chapter/documentation-and-review.fragment/___landmarks/documentation-side.landmark/___code/two-sides-sidebar.json`

It pins lines 101–109 of `internal/server/appnav.go` at commit
`b851addd875500f0fb4cc37bb0272a3447bfd5da`; those lines changed by current head
`f3474a00191075c86c35833ff291e244bf1ea846`. The next evidence wave must inspect
the current navigation implementation and either replace that range with the
smallest current source or retire the evidence if the design is no longer
implemented. It must not blindly repin the old digest.

The same next wave should add the smallest current code evidence for each new
visual fragment, or explicitly record that implementation has not yet caught up
with the canonical design.

## Structured CLI dogfooding findings

| ID | Author goal | Interface exercised | Observation | Impact and recovery | Smallest improvement |
| --- | --- | --- | --- | --- | --- |
| F1 | Revise an existing technical-design fragment | `set-fragment-content` | The generic mutation rejected technical design and said to use `change-saga design`; this was a useful safety guard. | No write occurred. Retried with `design set-fragment-content`. | Include the complete corrected command, including the resolved target and feature. |
| F2 | Create a technical-design chapter | `add-chapter --feature agent-loop ...` | The generic command successfully created a narrative chapter beside `___design`, even though a technical-design namespace exists. A later `design add-chapter` reported only that the ID was invalid or used. | Briefly produced out-of-scope records. Removed only the newly created files and empty `___code` directory, then recreated them with `design add-*`. | When a feature already has `___design`, require an explicit narrative/design choice before writing, and report the conflicting path when an ID is already reserved. |
| F3 | Address the existing two-sides fragment | `design set-fragment-content --target ...` | The directory name suggested `documentation-and-review`, while the stable fragment ID is `two-sides-overview`. The failure printed a long global target list. | Required a separate `query children` call to resolve the chapter's one child. | On a near path/ID miss, return scoped candidates from the containing chapter first. |
| F4 | Create content and then populate it from stdin | `design add-fragment`, then `design set-fragment-content --source -` | The safe stdin content path works well, but creation and initial content are separate mutations when no source file is prepared. | Doubled calls for every fragment; interruption can leave empty but valid artifacts. | Let `design add-fragment --source -` atomically create and populate a fragment. |
| F5 | Make design headings and diagram regions traceable | `add-landmark` | Heading validation and automatic SVG element bounds were clear and reliable. The suggested next action was always `cover`, even for a technical-design landmark whose next task was a requirement relation. | Author had to reconstruct every `relation add` command. | For design landmarks, suggest both code coverage and `addresses` relation shapes, with the current content digest previewed. |
| F6 | Pin a complete design to criteria | `relation add` and `relation repin` | Defaulted current revision and content-digest pins were explicit and excellent. There is no atomic relation batch, so this wave required many independent calls. | Slow authoring and a larger partial-progress window; recovery was repeated stale-relation and traceability queries. | Add dry-run and atomic JSONL batch support for relation add/repin with per-entry rationales. |
| F7 | Understand why relations were stale | `query relations --state stale` | The query identified stale relations but did not expose a usable stale reason in its returned relation objects. | Required inspecting prior pins and relying on repin output to confirm the changed content digest. | Return source/target currency, old and current pins, and a concise reason for every stale relation. |
| F8 | Validate a design-only wave | `validate --json` | Validation is successful, but every new SVG warns about missing direct code despite the deliberate no-code-evidence scope. | Warnings are expected but indistinguishable from forgotten evidence without this external note. | Support an explicit planned-evidence state or a design-only validation profile while retaining the warning in full release validation. |
| F9 | Query coverage and delivery gaps | `status`, `query traceability`, and `query gaps` | Dedicated queries are precise, but `status` remains very large and traceability mixes design gaps with work-item and immutable-delivery blockers. | Required temporary files and `jq` projections to isolate design completeness. | Add `query traceability --blocker design_missing` and a compact `status --summary` projection. |

## Provenance note

The accepted heads and merged audit documents in this repository were the
authoritative inputs. DayLight Local and Team retrieval was attempted for prior
work provenance but was unavailable because the desktop service was not
running; this is recorded as a retrieval gap, not evidence that no other work
exists.
