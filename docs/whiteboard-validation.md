# ERD whiteboard validation

Validated on 2026-10-13 on `feature/erd-whiteboard`, using isolated implementation
worktrees followed by manager integration and independent review. Local previews
use copied Sagas; source repositories are read-only. A final SHA-256 comparison
of all 2,816 original DayLight Saga files found no changes.

## One native document

The native storage regression publishes nested sections, entities and a recorded
cross-section relationship with **no ERD inventory records**. Structured describe
retains full instance paths, endpoint identity and exact source revisions.
The browser fixture adds real saved code references and verifies native entity
and relationship inspection, fields, section focus, history, reload, reduced
motion, shared reveal replay and accessibility. Native section culling removes
hidden controls from the tab order and restores them when returning.

A valid board replaces the ERD/entity catalog in the sidebar with one Data model
entry and its authored section outline. Imported source material remains in a
collapsed provenance panel. Section focus does not create a fictitious ERD or
inherit the previously selected import's evidence pin. A mixed native/imported
fixture verifies those transitions through Back and Forward.

## Existing content and composition

The representative fixture retains three ERDs, structured diagrams and an opaque
SVG, repeated source appearances, nested transforms, annotations and semantic
connections. Focused tests cover exact historical routes, source revision pins,
connector simplification, authored bends, safe SVG metadata and compatibility.

Camera updates are RAF-batched. Geometry is cached; overscan culling changes
visibility only when necessary and retains source metadata. Shared slide hover,
note, evidence and reveal behavior is reused without creating slide Item records.
Normal wheel and trackpad scrolling pan, Shift supports horizontal mouse-wheel
panning, and Ctrl/Cmd wheel zooms around the pointer.

## DayLight source and authored model

Discovery located 20 source ERDs in the readable DayLight Saga, more than the
initial estimate. The example script migrates their exact entity and relationship
pins into **one native document: 76 entities, 675 fields and 120 recorded
relationships**. No imported frames or source SVG geometry are required for
coverage. The original ERD directories serve as migration and provenance input.

All recorded relationships have exact owner and destination revision pins and
saved code evidence. The audit distinguishes **103 physical foreign keys and
17 application associations**, plus four separately identified narrative
connections. Six records with no recorded incident relationships remain visible
without invented connections. The tests compare complete entity-pin sets,
field names/types/keys and owner-revision/relationship/destination tuples against
the source, rather than checking counts alone. A mocked SVG-read prohibition
verifies that completeness does not depend on original drawings.

Crow’s-foot endpoints preserve each association’s recorded owner and destination
cardinality. In particular, the reverse policy-history relationship remains
`1 → 0..many`, distinct from the current-policy selector’s `0..1 → 1`.
The diagram does not infer identifying relationships from line color or style.
The opening retains a selected core narrative; focus and zoom reveal supporting
relationships, and Details exposes full fields. The document can extend beyond
one screen; Fit remains an atlas, not the initial reading scale.

The authored canvas is 27,000 × 43,891 units. Its 43 reading groups contain
actual entity members, so focused structured descriptions agree with visual
focus. The 99 supporting display labels were removed without dropping lines,
cardinalities or semantic labels from notes and descriptions. At the real
1440 × 1000 app opening, summaries and connection labels measure about 15.6
screen pixels and narrative text about 19.5 pixels. Fields use the shared
renderer’s row typography and require further zoom or source inspection.
Summary entities keep field names and types in compact adjacent columns. A live
100% zoom check reads Project fields at 16 pixels with both columns inside the
canvas; across all 675 fields, the widest type-column endpoint is 500.245
diagram units. Ordinary ERD artwork, row heights and endpoint geometry remain
unchanged.

Publication through the public CLI produced revision `r7` with parent `r6`;
`check` reports valid and `get` round-trips the authored JSON. Focused entity
inspection retains its exact pin and three incident relationships; its reading
group reports both member tables and four incident relationships. The inverse
policy relationship retains its `only-one` and `zero-or-many` markers.

Live Chromium checks verify shared action/info/code controls, exact entity and
relationship evidence, field toggling, section history/reload and offscreen
keyboard restoration, with no JavaScript errors. Initial annotation requests
are zero. Positive story-panel behavior is covered by a fixture with real
Item links; no story evidence is invented for DayLight nodes without such links.

The shared annotation action strip uses the implementation slides’ real server
templates and delegated controls. Authored notes use the same sanitized Markdown
popover. Metadata loads for an exact pin on demand; saved code loads only when
its panel opens. Story actions require actual implementation Item links to the
same saved entity revision, and show that Item provenance. Relationship actions
do not inherit entity-wide story links. No fictitious slide Items are created. Keyboard and delayed-response
regressions cover Escape, forward note Tab, pan cleanup, navigation and revisits.
A live long-connector check verifies a slow pointer move can reach its action
strip and open the exact relationship, even when its label is offscreen.
Oversized entity cards use the same pointer-local fallback when their authored
anchor is outside the viewport. Independent review closed all reported focus,
loading, pointer and field-legibility findings against the final live build.

## Checks and limits

Integrated focused Go tests pass for diagram/canvas behavior, whiteboard storage,
source pins, technical navigation, viewer scripts and interactions. Agent checks
also pass scoped race tests, vet, TypeScript and focused Chromium cases. Eight
real-data Python authoring tests pass. The native fixture includes axe checks.

The [performance report](whiteboard-performance.md) preserves the paired baseline
and optimized measurements for the earlier 21-appearance scene. The new native
scene is a different workload and is measured separately; its results must not
be presented as a paired improvement against that old baseline.

The broad Go run has a previously reproduced failure in
`TestInstalledSkillRoutesFocusedTasks`: an unrelated authoring compatibility
router contains 168 words. A later broad concurrent run also encountered a busy
writer timeout and an unrelated dogfood documentation-page failure. Focused
feature checks passed; broad runs were stopped rather than represented as green.

Browser visual validation uses desktop Chromium. Infra, Systems, direct layout
editing, inferred SVG internals and automatic routing remain outside this ERD
implementation. Historical ERDs remain readable as provenance and compatibility
records; separate child ERDs are not required to author a native model.
Whiteboards retain focused structured describe and atomic parent-checked
publication; slide-style transactional `diagram edit` is not yet supported.
