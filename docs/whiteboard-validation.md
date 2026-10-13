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
initial estimate. The example script uses exact pins in a separate copied Saga.
Its primary narrative contains **23 native entities, 279 full fields and 27
connections** across Delivery, Search and Authority sections. This is a selection
of the core model: all **76 distinct source entities** remain accessible through
20 original drawings in the same-canvas source appendix. The opening explicitly
discloses that distinction.

The connection audit distinguishes 14 physical foreign keys, nine application
mappings/projections, two transport explanations, one authorization explanation
and one repeated-entity appearance. Twenty-one lines carry recorded relationship
pins with exact owner/destination revisions; six explain source-backed behavior
without claiming another recorded relationship. The generation tests check all
native fields against source records, relationship endpoint pins, connectivity,
route/label/body clearance and unchanged input files.

The authored canvas is 14,400 × 24,300 units with an opening camera of 6,200 × 4,900.
It intentionally extends beyond the screen. The opening narrative measures about
20 screen pixels in the real 1440 × 1000 desktop viewer; native table summaries
and connection labels measure about 15.7 pixels, and the coverage note 16.4.
An authored entity summary replaces its field body until Details is requested,
using the shared renderer while retaining exact field geometry. Fit board is an atlas;
it is not the default reading scale. Readers follow connected sections and zoom
further for fields or open the evidence drawer for exact definitions and code.

Live Chromium checks verified native entity and relationship code at saved commit
`d3eafaf3ea09656f89b41ff990e91f0c8f4c74a5`, section/source focus, Back/Forward,
Details, offscreen keyboard restoration and no JavaScript errors. Original SVG
imports retain whole drawings and recorded bindings. Opaque SVG geometry and
field structure are not inferred; their saved directories provide evidence.

## Checks and limits

Integrated focused Go tests pass for diagram/canvas behavior, whiteboard storage,
source pins, technical navigation, viewer scripts and interactions. Agent checks
also pass scoped race tests, vet, TypeScript and focused Chromium cases. Six
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
