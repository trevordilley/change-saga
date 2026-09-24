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

That boundary is correct and remains unchanged. The pull-request reviewer now
restores the visual collaboration layer that existed before the documentation
reframe: rectangle and ellipse shapes, freehand paths, highlights, sticky
notes, anchored discussion bubbles, movement, color changes, keyboard deletion,
and append-only undo/redo. Stories, Features, design records, and exact diffs
complement those marks as linked context; they do not replace the deck.

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

## Implemented slice

The product direction is deliberately narrow: **the pull request is a slide
deck**. The earlier native presentation contract introduced by `c20d367`
(`make slides the primary review surface`) and the current Implementation deck
are the interaction model. The individual review route now uses that same deck
viewer directly:

- the deck fills the entire pane beneath the application header by default;
- a compact thumbnail rail and one maximally fitted 16:9 slide are the only
  default review layout—there is no page heading, card, details section, or
  intermediate “present” step;
- Code Diff and Coverage are not competing tabs on an individual review;
- Previous/Next and unmodified Left/Right or Page Up/Page Down keys use the
  shared deck navigation, focus rules, slide position, and stable URL hashes;
- direct slide and Item hashes reveal the owning slide, including after a
  decision or comment redirect and reload;
- each semantic Item is projected onto its authored region or measured SVG
  element. Its on-slide affordances open the exact linked diff and affected
  living-Saga record in the existing drawer;
- slide decision and slide-comment controls remain available in one quiet
  overlay; Item comments and replies live with the Item evidence they discuss;
- the rail shows decision currency and open-thread state without turning those
  signals into a progress score or inferred completion;
- at narrow widths the rail becomes a horizontal filmstrip while the slide
  keeps the rest of the viewport.
- one persistent annotation toolbar operates on the active slide. All geometry
  is normalized to the slide, so fitting the deck, changing viewport width, or
  moving between slides does not move a mark away from its visual subject;
- a new mark opens its comment composer only after a valid anchor exists.
  Rectangle, ellipse, freehand, highlight, and sticky-note marks persist with
  their human attribution; replies remain attached to the mark's bubble;
- move, recolor, undo, redo, and delete append annotation events to the same
  thread. They never rewrite or erase the original comment or anchor.

All slides, exact Item targets, evidence, discussions, and ordinary forms stay
in the server response. The browser reuses the implementation-deck viewer and
landmark projection rather than maintaining a second client-side slideshow.

## Preserved contracts

- Approval still applies only to the complete review slide. Item evidence and
  comments do not become a second approval checklist.
- Decision and comment endpoints, review targets, reviewer seats, source commit
  pins, and one-record-per-file append behavior are unchanged.
- A slide becoming visible never records a viewed or approval event.
- Changes requested and out-of-date decisions remain visible in the rail and
  slide overlay; unresolved feedback is never hidden from state summaries.
- Mutation redirects keep their exact slide or Item hash. No save is inferred
  from navigation, and form text is submitted only by the form's labeled
  action.
- No dependency, schema, CLI contract, network asset, or storage change was
  introduced.

## Deferred work

- A browser-local “last location” could improve cross-session resume, but must
  be keyed to the exact review head and must not be confused with review truth.
- Unsaved composer text survives in-slide navigation because inactive slides
  remain in the DOM, but reload/navigation-away draft recovery is not yet
  implemented.
- Thread resolution/reopening is supported by lower layers but still needs a
  dedicated browser interaction with an explicit state-change confirmation;
  this slice adds replies only.
- Review-level coverage remains available to reports and APIs, but it is not a
  competing view inside the individual deck experience.

## Verification record

- `go test ./internal/server` passes, including the bounded-browser deep-link
  contract and focused review handlers.
- The Chromium pull-request review suite passes all four scenarios, including
  persisted decisions, comments and replies, source currency, exact Item
  permalinks, reload resume, keyboard navigation, Escape focus restoration,
  normalized annotation geometry, move/recolor/undo/redo/delete history, and
  the 390 px layout.
- `./scripts/check-docs-links.sh` checks 202 in-repository links successfully.
- Desktop and narrow before/after captures live under `.devswarm-temp` for the
  workspace handoff. The fixture and screenshots are deliberately not
  committed and contain no user review records.
