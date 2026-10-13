# ERD whiteboard validation

Validated on 2026-10-13, using local commits on `feature/erd-whiteboard`.
The implementation was split across isolated composition, storage/CLI, and
viewer worktrees, then integrated and independently reviewed.

## Representative composition

`internal/testfixture/whiteboard.go` creates three independent ERDs: two
structured diagrams and one existing SVG. The board has authored overview
text, large summaries, a production connection pinned to its recorded
relationship, a callout, an ordinary group, repeated source entities, and
local entity fields and annotations. The SVG appearance retains its bindings
but does not invent internal geometry.

The integration test checks Saga compatibility, board validation, source pins,
focused fields and local explanations, incident connections, and repeated
opaque bindings. The browser fixture additionally exercises nested ERDs.

Chromium checks cover:

- Overview/detail switching, pan, zoom, fit, keyboard focus and inspection.
- Pinned source navigation and the independent ERD page.
- Accessible controls and no serious accessibility violations in the board.
- Mixed nested detail: each connector meets its visible endpoint independently,
  retains authored bends and markers, and keeps its semantic endpoint paths.
- Local detail annotations remain visible when an unrelated frame is collapsed.
- Existing authored ERD selection and large-ERD navigation regressions.

Independent screenshots at a 1440 × 1000 viewport showed readable summaries
at fit and readable entity fields and local callouts at focus. The representative
board's overview summaries measured approximately 19 screen pixels.

## DayLight composition

Repository/workspace discovery located the readable
`docs/daylight-application-saga` checkout at commit
`bc699a81e8f375727507869511dd3c3e3e61035e`. It contained **20** ERDs, more than
the initial estimate. Validation used a separate copied Saga and an isolated
QA repository; it did not write to DayLight's repository. SHA-256 comparison
of all 2,816 copied source files confirmed the originals were unchanged.

The public CLI published a board with all 20 exact ERD revision pins, authored
summaries paraphrased from those revisions, an overview callout, and a
source-referenced delivery-admission connection. `diagram check --whiteboard
erd` passed. Focused description of `cloud-identity` returned the frame and
nine explicitly bound instances with their original entity/relationship pins.

In Chromium, all 20 frames collapsed to useful overview summaries at fit;
summary text measured approximately 15 screen pixels. Focusing Identity and
tenancy and zooming further made the organization-members fields, types, keys,
and relationship labels readable. The browser reported no JavaScript errors.

These DayLight sources are SVGs. Their whole drawing and recorded bindings are
available, but their internal geometry is not inferred; entity inspection uses
the original ERD page, and board connections target whole frames. The
representative structured fixture covers internal endpoint targeting instead.

## Checks and limits

Focused Go tests cover compatibility, immutable child pins and board history,
nested transforms, repeated appearances, annotations, source validation, SVG
viewBox offsets, invalid graphs, and stored source/visual consistency. The
diagram and requirements race tests pass. Build, vet, TypeScript, and the
focused Chromium tests pass.

The broad Go suite has a pre-existing failure in
`TestInstalledSkillRoutesFocusedTasks`: the authoring compatibility router
contains 168 words. The same failure was reproduced before implementation;
unrelated skill documentation was left unchanged.

Browser visual validation used desktop Chromium. Infra, Systems, direct
manipulation editing, inferred SVG geometry, automatic layout, and elaborate
connector routing are outside this ERD prototype.
