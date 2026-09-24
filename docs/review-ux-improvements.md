# Review UX improvements

Status: implemented and visually verified first pass, 2026-09-24
Scope: the v5 pull-request slide reviewer in `internal/server/reviews.go` and
`internal/server/appjs.go`

## Product correction

A pull-request review is not a new review application wrapped around a deck. It
uses the implementation-deck UX directly: thumbnail rail, one fitted authored
slide, the established slide controls, and optional fullscreen presentation.
The only review-specific adaptation is that an Item's code control resolves its
reference against the review's base/head range and therefore opens a diff.
Affected Story, Feature, design, and quality records remain linked context.

The complete slide is still the decision target. An Item remains the exact
evidence, permalink, and discussion target. Seeing a slide never records a
decision, and Item evidence never becomes a second approval checklist.

## Reference audit

The current branch was compared with the released reviewer in `v0.1.1`, run
locally from the tag rather than inferred from old design documents. The
reference establishes the intended interaction:

- the deck is the default full review pane; Present only removes chrome;
- a numbered thumbnail rail and one 16:9 canvas provide navigation;
- the slide's upper-right strip contains marked places, permalink, annotation,
  approve, request-changes, and comment controls;
- Item actions are quiet landmark affordances revealed by hover, focus, direct
  link, or the always-available marked-places menu;
- the annotation button opens a compact palette beside the slide controls:
  undo, redo, select, comment, highlight, rectangle, freehand, sticky note,
  color, and selection deletion;
- marks and their discussion bubbles stay on the slide.

The historical commits used to validate that behavior include `c20d367`
(slides as the primary review surface), `baee293` (permalinks and annotation
colors), `e154ce8` (append-only undo/redo), `bb9d6d5` and `34258b4`
(editing and deletion), `62bd0d3` (sticky notes), and `0f13bb3` /
`9edd95d` (mark-attached discussion and drawing feedback).

## Current findings

1. The first branch pass invented a pull-request-only rail, persistent evidence
   labels, a large review menu, and a bottom annotation toolbar. Although it
   made the review slide-first, it did not preserve the implementation-deck UX.
2. Code references were present but the persistent `Code · N` badges made the
   slide look unlike an implementation deck. The established marked-place and
   landmark affordances already provide keyboard, touch, and pointer routes.
3. Annotation storage and projection were successfully restored, including
   normalized geometry, append-only create/update/delete records, reload
   persistence, and read-only merged reviews. Its presentation was wrong:
   tools were always visible and did not use the released compact palette.
4. The initial annotation client did not preview a mark while drawing and once
   rehydrated stale geometry during edits. Geometry projection and edit
   rollback now use the latest mutable anchor; live draft markup is part of
   this correction.
5. The first pass suppressed Present, Code Diff, and Coverage on an individual
   review. That confused “the deck is already the default” with “the ordinary
   implementation-deck secondary surfaces must disappear.”

## Coherent implementation slice

- Restore the released slide rail/canvas hierarchy and quiet upper-right action
  strip on the individual review route.
- Restore Present as optional fullscreen chrome removal while keeping the deck
  as the default route and visible surface.
- Restore Code Diff and Coverage as secondary tabs. They never displace the
  deck on entry.
- Render review Items with the same marked-place list, permalink, affected
  record, and linked-code affordances as implementation slides. The linked-code
  drawer contains the review-range diff.
- Restore the compact, per-slide annotation palette. It starts closed, opens
  from the marker icon, closes with Escape with focus restored, and offers the
  historical slide tools supported by the current model.
- Keep live drawing feedback, normalized hit testing, move/resize/recolor,
  append-only undo/redo/delete, replies, and reload persistence.
- Keep approval explicit and slide-scoped. Request changes opens a note
  composer; comment opens slide discussion. Decision history and source
  currency remain inspectable without dominating the canvas.
- Preserve stable slide and Item hashes, exact base/head evidence, human/AI
  attribution, unresolved thread visibility, and frozen-review read-only state.

## Deferred issues

- Text-selection highlighting from the legacy document renderer is not claimed
  for raster/iframe review slides. The current review model supports a spatial
  highlight region, which is what the slide palette exposes.
- Thread resolve/reopen still needs a dedicated explicit control.
- Cross-session draft recovery and a head-keyed local resume pointer remain
  separate work; neither may be treated as review truth.
- No schema, CLI, dependency, or public API change belongs to this UI
  correction.

## Verification record

All persisted mutations used disposable review fixtures; `app.saga` remained
read-only.

Change Saga itself was used as the final documentation and coverage check:

- `change-saga validate --json app.saga` reports the repository's real app Saga
  as valid. `status --json --against main --head HEAD app.saga` is deliberately
  not presented as complete: because the branch-local review stays disposable,
  the real Saga truthfully reports this branch's implementation mapping gaps.
- A short-path disposable copy contains one `review-ux-improvements` review with
  four authored slides and twelve Items. Its evidence was rebuilt against the
  final branch head with `cover --changed-lines` so every old/new changed-line
  atom belongs to exactly one explanatory Item. `review list --json` is the
  acceptance report for the review range, current decisions, open discussion,
  and review-deck coverage.
- The disposable review was served by the current binary and exercised as the
  actual deck, including Item-to-diff navigation, affected-record navigation,
  slide annotation persistence after reload, decision currency, Coverage, and
  desktop/narrow layouts. No mutation was written to `app.saga`.

- Focused Go review, Saga annotation-model, review-store, JavaScript, deck, and
  slide tests pass: `go test ./internal/saga ./internal/reviewstore
  ./internal/server -run 'Review|Annotation|AppJavaScript|Deck|Slide' -count=1`.
- The Chromium pull-request review suite passes all five scenarios. It covers
  exact diff and affected-record drawers, slide decisions and currency,
  comment/reply persistence, keyboard and narrow navigation, stable hashes,
  Escape/focus restoration, live drawing feedback, normalized geometry,
  move/resize/recolor/undo/redo/delete values, reload projection, and frozen
  review behavior.
- The E2E TypeScript suite type-checks with `tsc --noEmit`.
- `./scripts/check-docs-links.sh` checks 202 in-repository links.
- A broader `go test ./internal/server ./internal/saga
  ./internal/reviewstore` run reached its tracked five-minute host timeout with
  no emitted failure; the focused package coverage above is the completed Go
  result and the timeout is not represented as a pass.
- Before captures from the rejected intermediate UI and final desktop,
  annotation-palette, and 390 px captures live under
  `.devswarm-temp/review-ux-*.png` and
  `.devswarm-temp/review-ux-screenshots/`. These uncommitted artifacts contain
  no user review records.
