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

The tracked `app.saga` now contains the `review-ux-improvements` review and its
four slides. The review follows `feature/review-ux-improvements` against `main`;
its source range is independent from the living documentation's HEAD currency.
The original disposable fixture remains available as historical test material.
Its smoke-test discussion was not imported as genuine reviewer feedback.

The living annotation design, implementation explanations and exact references
have been reconciled with the implemented review behavior. The public
`reconcile --against main --json app.saga` report keeps review coverage,
current documentation health and remaining reassessment work separate.

The original implementation verification included:

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

## Child integration review — 2026-09-24

The reconciliation, PR performance, and Component/System workspaces were
independently reviewed and merged into this branch, not into `main`:

- `32e35985`: CLI reconciliation and current documentation evidence.
- `32a4f90d`: request-local diff reuse, bounded Item-code batching, and guarded
  asynchronous review writes. Superseded synchronous implementation references
  were removed through the public coverage API; their history remains in Git.
- `89404fc6`: canonical, revision-pinned Component/System documentation and
  in-slide navigation. Item documentation controls coexist with async feedback.

Integration inspection found the restored deck CSS also applied to Code Diff:
it hid the file tree and confined the diff to the sidebar column. Deck-only
layout is now scoped to slide mode; Code Diff keeps its ordinary workspace and
mobile file-tree overlay. Present respects its hidden state outside the deck.
Returning from a selected diff restores the prior slide permalink as well as
the active slide. A missing review deck also no longer causes inventory
validation to panic: the existing missing-deck diagnostic is preserved.

The integration regression navigates between two review slides and two changed
files at 1440 px and 390 px. It checks diff geometry, file-tree controls, absence
of page overflow, and the return permalink. Before/after screenshots are in
`.devswarm-temp/integration-validation/`. The existing Terms directory test was
updated for the already-shipped separate definition-maturity and implementation-
evidence columns, without changing those contracts.

### Remaining reconciliation work

At integration commit `89404fc6`, the public reconciliation report found 805 of
828 living references current (182 remapped) and 23 stale: six regressions and
17 introduced stale references. Structural validation passed; that is **not**
a claim that documentation is current. Remaining references include review
ownership, contextual review, query surfaces, storage, and vocabulary records.
Each needs semantic reassessment before replacement, not a blanket repin.

The original four-slide review follows this growing integration branch. Its
coverage consequently expands beyond the UI slice: 668 of 10,569 changed lines
were covered at that commit, with 9,901 unexplained and 11 stale references.
The CLI child's live review range also collapses after its base branch absorbs
its head. Freezing that completed review needs to retain its original range;
the current `repin` operation also rewrites living references, so it was not
used indiscriminately against an older merge. The Component/System review at
that point covered 2,912 of 2,912 lines with no stale references. These are
historical integration measurements, not approvals or final branch totals.

The first pass of Component/System inventory does not yet participate in every
general reconciliation/repin path; see
[its documented boundaries](component-system-inventory.md). No child archive
removes these outstanding tasks or the child worktree and commit history.

### Integration verification

- Chromium: all 50 scenarios pass with two workers (2.0 minutes), including
  inventory navigation, async writes, annotations, and review/source currency.
  The final return-permalink guard and settled mobile screenshot were checked
  again with the focused Code Diff scenario (1 passed, 13.9 s).
- Focused race-enabled Go checks pass: reconciliation/public evidence repair
  (CLI, 41.727 s); review/annotation/snapshot checks (server 50.835 s,
  reviewstore 1.860 s, Saga 2.092 s); inventory, pinned documentation,
  async/snapshot and JavaScript checks (server 4.723 s, CLI 6.007 s,
  requirements 2.111 s). The missing-deck regression first reproduced the panic
  and then passed with the guard (1.804 s under the race detector).
- TypeScript type-check, CLI build and documentation links pass (221 local
  links across 84 files). Desktop preview was also inspected in Chrome against
  actual `app.saga`; persisted interaction tests use disposable fixtures.
- A broader server selection timed out after 121 seconds in
  `TestDocumentationPagesHaveNoApprovalOrCommentControls`. It is not counted
  as a pass; the full Go suite has not completed in this integration run.
- The first combined browser run exposed an obsolete five-column Terms test
  expectation and a viewport assumption in the new layout test; both were
  corrected before the passing run. The new test also reproduced the actual
  file-tree/layout failure and lost return permalink before their fixes.
