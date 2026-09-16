# Hybrid Report and Slide Saga

Status: smallest coherent vertical slice, 2026-09-04

## Decision

The durable parent is a v3 Report Saga. It keeps the chapter-like living
documentation that explains requirements and acceptance criteria, overall
technical design, and work-plan/wave history. A report may additionally contain
several focused slide decks for complex implemented code changes.

Accepted user stories and their acceptance criteria are the closing-loop
backbone. Every Item that owns code evidence should be covered, directly or
through its slide or deck, by an active `explains` relation to a story or
criterion. That relation pins the story revision. A story-level link covers all
acceptance criteria in that revision; a criterion-level link records the more
precise claim.

This is composition, not conversion. A story, prototype, design document, or
work-plan event does not become a slide. Review authors add a deck only when a
visual sequence materially improves a reviewer's understanding of a specific
change, workflow, failure path, or surprise.

## Storage and identity

Embedded decks live at `___slides/<deck-id>.deck/`. Every bundle is flat and
contains one existing v4 deck record with its slide, Item, asset, and evidence
records. Reusing v4 component bytes keeps the slide contract singular while a
directory per deck gives concurrent authors an independent merge boundary.

The parent `saga.json` is the only Saga manifest. All embedded targets derive
from its ID, so report fragments and visual Items participate in the same
bidirectional coverage graph:

```text
urn:change-saga:<saga>:fragment:<fragment>
urn:change-saga:<saga>:deck:<deck>
urn:change-saga:<saga>:slide:<slide>
urn:change-saga:<saga>:slide:<slide>:item:<item>
```

The parent report is the overview, so embedded deck records use `role: change`.
Each bundle contains exactly one deck and its directory basename equals the
deck ID. V4 standalone Sagas retain their existing single flat root, overview
deck, and intentionally slide-only semantics.

## Included in this slice

- v3 loading and validation of zero or more embedded deck bundles;
- unchanged v2/v3 report reading and unchanged standalone v4 reading;
- v3 CLI authoring through `add-deck`, `add-slide`, `set-slide-content`, and
  `add-item`, while v2 still requires an explicit upgrade;
- Item-only exact diff ownership for embedded slides;
- revision-pinned `explains` relations from decks, slides, or Items to stories
  and acceptance criteria, without copying requirement prose into slide data;
- traceability queries that return story-to-slide-to-diff paths, support reverse
  lookup by exact diff URI or the current committed source head, and report
  unlinked Item evidence;
- one overview query that exposes report chapters/fragments and deck summaries,
  plus native `slide` and `slide-diffs` queries;
- flat slide/Item comment records and per-slide approvals under the parent Saga;
- a distinct Decks tab in the reviewer with thumbnails, slide navigation,
  presentation mode, Item hotspots, and linked-code affordances; and
- deterministic per-deck paths so separate decks merge independently.

## Explicitly staged

Prototype records already have an internal domain, but they still have no
public CLI, query, or reviewer UI. They are not surfaced by this slice. The
internal prototype directory also currently conflicts with the strict
requirements-root loader, so composition must be repaired before a public
prototype surface can be considered reliable.

This slice also does not auto-generate slides, turn report fragments into
slides, migrate v2/v3 reports to v4, or machine-judge whether a surprise is
editorially justified. Surprise remains a callout Item backed by exact diffs
and reviewer judgment.

The public query/API carries the new story-to-code paths. Reviewer UI badges and
click-through navigation for those paths are staged rather than implied by this
slice.

## Risks and follow-up

- Hybrid page rendering currently loads complete embedded slide content when
  decks exist; large-deck scale needs a deck-scoped lazy endpoint and budget.
- Slide review overlay files are flat at the parent root while authored slide
  and evidence records stay inside their deck bundle. Both are independent
  additions, but this split must remain explicit in maintenance tooling.
- The public v3 JSON manifest schema does not enumerate filesystem children;
  runtime validation is the normative gate for `___slides` bundle structure.
- Requirements, plan readiness, mapping coverage, story-linked slide evidence,
  and slide approval remain separate axes. A future UI must not collapse them
  into one completion score.
- `--commit` currently means the resolved source-head commit for the active
  committed comparison. It is unavailable for `WORKTREE` and does not perform
  commit-history or blame analysis for intermediate commits.
- Prototype orchestration and central validation of every living subtree remain
  follow-up work.
