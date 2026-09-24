# Review UX improvements

Status: implemented first pass, 2026-09-23  
Scope: the v5 pull-request slide reviewer in `internal/server/reviews.go` and
`internal/server/appjs.go`

## Current-product audit

This audit was performed against the current slide/Item reviewer, not inferred
from the earlier chapter/fragment audit. `app.saga` was used read-only to
inspect realistic product navigation. Because it has no pull-request review
records, persisted interactions were exercised on a disposable three-slide
review and separate source repository under `.devswarm-temp`.

The current product has two deliberately different visual surfaces:

- living implementation decks are documentation. Their slides and Items link
  to stories and exact code, but have no approval, comment, or annotation
  controls;
- pull-request review decks are the change-review surface. The complete slide
  is the decision target and Items are the precise evidence/comment targets.

That boundary is correct and remains unchanged. The current pull-request
reviewer supports slide decisions, slide/Item comments, and append-only replies
in its storage and HTTP handler. It does **not** implement rectangle, freehand,
highlight, sticky-note, or other spatial annotation anchors. The legacy audit's
annotation recommendations therefore are not presented as current capability.

### Prioritized findings

1. **P0 — no bounded review step or reliable return point.** Every slide,
   Item diff, decision form, and comment form appeared in one long document.
   The URL only became useful after a mutation redirect; there was no visible
   slide position, authored sequence, Previous/Next action, or active-slide
   state.
2. **P0 — replies existed in the model but not the browser.** Review comments
   were readable, and the comment handler already accepted `reply_to`, but the
   page only exposed forms that started new root threads.
3. **P1 — repeated controls outweighed the visual argument.** Decision and
   comment textareas were always present on every target. On a multi-slide
   review, controls competed with the diagram and evidence before the reviewer
   had chosen an action.
4. **P1 — orientation and state were separated.** Exact decision currency and
   open-thread counts existed in the report but not in a navigable slide
   overview. A reviewer had to scan the entire page to locate needs-attention
   slides.
5. **P1 — source mechanics dominated entry.** Base/head hashes and following
   ref appeared above the first visual argument. They are useful diagnostics,
   but not the primary answer to “what should I review?”
6. **P1 — narrow layouts inherited desktop density.** Slides became a tall
   sequence of diagram, decision form, Items, diffs, and comment forms without
   a compact way to retain deck position.

## Implemented first slice

The review deck is now a focused, URL-addressable slide workspace:

- only one review slide is active at a time;
- the authored visual is the dominant canvas; Item evidence, diffs, and
  discussion live in a secondary **Slide details** disclosure that opens
  automatically for an exact Item permalink;
- a slide rail shows authored order, thumbnail, exact current decisions,
  decision currency, and nonzero open-thread counts;
- Previous/Next and unmodified Left/Right or Page Up/Page Down keys move within
  the deck when focus is not in an editable control;
- selecting a slide writes its stable DOM target to the URL; direct slide and
  Item hashes reveal the owning slide before anchor navigation, so reload and
  mutation redirects return to the exact context;
- no-hash entry starts at the authored first slide. The UI does not infer that
  a decision means completion, because Change Saga records decisions rather
  than a team verdict;
- decision, new-comment, and reply composers use explicit disclosure controls,
  remain keyboard/touch accessible, close with Escape while restoring summary
  focus, and retain ordinary form submission as a no-framework fallback;
- existing discussions expose an in-place Reply action backed by the existing
  append-only `reply_to` behavior;
- source range/currency remains available behind a labeled disclosure;
- below 1050 px the rail becomes a horizontal navigator, and below 720 px the
  visual and slide decision stack into one column with full-width actions.

The implementation deliberately leaves all slides and exact Item content in
the server response. The browser only changes presentation and URL-owned
selection. This preserves stable targets, permalink resolution, review report
currency, and ordinary form posts without adding a client-side review model.

## Preserved contracts

- Approval still applies only to the complete review slide. Item evidence and
  comments do not become a second approval checklist.
- Decision and comment endpoints, review targets, reviewer seats, source commit
  pins, and one-record-per-file append behavior are unchanged.
- A slide becoming visible never records a viewed or approval event.
- Changes requested and out-of-date decisions remain visible in the rail and
  on the slide; unresolved feedback is never hidden from state summaries.
- Mutation redirects keep their exact slide or Item hash. No save is inferred
  from navigation, and form text is submitted only by the form's labeled
  action.
- No dependency, schema, CLI contract, network asset, or storage change was
  introduced.

## Deferred work

- Spatial annotation tools require an explicit current review anchor/storage
  design; none is implied by this presentation work.
- Diagram-Item hit regions are not yet projected over pull-request review
  visuals. Items remain exact, readable targets below the slide.
- A browser-local “last location” could improve cross-session resume, but must
  be keyed to the exact review head and must not be confused with review truth.
- Unsaved composer text survives in-slide navigation because inactive slides
  remain in the DOM, but reload/navigation-away draft recovery is not yet
  implemented.
- Thread resolution/reopening is supported by lower layers but still needs a
  dedicated browser interaction with an explicit state-change confirmation;
  this slice adds replies only.
- The separate Code Diff workspace retains its existing behavior and deserves
  its own focused visual audit rather than being redesigned incidentally here.

## Verification record

- `go test ./internal/server` passes, including the bounded-browser deep-link
  contract and focused review handlers.
- The Chromium pull-request review suite passes all three scenarios, including
  persisted decisions, comments and replies, source currency, exact Item
  permalinks, reload resume, keyboard navigation, Escape focus restoration,
  and the 390 px layout.
- `./scripts/check-docs-links.sh` checks 202 in-repository links successfully.
- Desktop and narrow before/after captures live under `.devswarm-temp` for the
  workspace handoff. The fixture and screenshots are deliberately not
  committed and contain no user review records.
