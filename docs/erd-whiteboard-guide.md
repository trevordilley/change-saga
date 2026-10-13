# Authoring an ERD whiteboard

The ERD whiteboard is the system's **one data-model document**. Author its
entities, relationships, nested sections and explanations directly using the
existing diagram grammar. Importing exact ERD revisions is optional and useful
for migrating existing artwork; separate ERD documents are not a prerequisite.

The AI authors the positions, sizes, narrative, annotations and connections.
The reader pans, zooms, focuses sections and inspects evidence within that same
document. The board fills the viewport beside the sidebar; its floating title
has an expandable description. Its outline names places within the model rather
than listing every source ERD as a separate document. Fit and Focus leave room
for the title and navigation controls. The full document can extend well beyond
one screen. Source revisions and historical routes remain available internally
for provenance and compatibility. Groups are ordinary drawing groups; this
feature does not change entity holders or add inventory kinds.

Scroll with a mouse wheel or trackpad, or drag, to pan. Shift + scroll pans
sideways on a vertical mouse wheel. Ctrl/Cmd + scroll (including trackpad pinch)
zooms around the pointer; the toolbar and keyboard controls remain available.

## Revision policy

Publishing the complete document creates an immutable JSON revision under
`___whiteboards/erd/revisions/`. Depicted entities and relationships use exact
source pins. Every optional imported frame also names an exact `target` and
`revision`.
An update names the current board revision as its parent. A board never follows
the latest revision of a child implicitly: adopting a revised ERD means changing
the frame's pin and publishing a new board revision. Older boards and ERDs stay
readable with their original pins. Repeated appearances can share a source pin
while retaining distinct instance paths.

## Publish and inspect

Author a `diagram.Document` JSON file using the existing styles, groups,
entities, text, notes, annotations and connectors. Use ordinary sections to
organize it and `connection` elements to connect appearances through their
instance paths. Optional `frame` elements import existing ERD content. Publish
the one document with:

```sh
change-saga diagram publish --whiteboard erd --revision r1 \
  --name "Application data" --from board.json change.saga
change-saga diagram describe --whiteboard erd change.saga
change-saga diagram describe --whiteboard erd \
  --focus cloud/identity/organization change.saga
change-saga diagram get --whiteboard erd --revision r1 change.saga
change-saga diagram check --whiteboard erd change.saga
```

For a subsequent revision, add `--parent r1` and choose a new revision ID.
The `--revision` option on reading commands selects historical board content.

## Native sections and relationships

A document can contain its own sections and entities without any `frame` or
ERD source record. This abbreviated example assumes the named entity revisions
and the `materializes` relationship already exist; substitute real source pins:

```json
{
  "version": 1, "width": 3000, "height": 1600,
  "elements": [
    { "id": "capture", "kind": "group", "shape": "section", "label": "Capture",
      "color": "yellow", "style": "boundary", "x": 80, "y": 200, "width": 1000, "height": 1000 },
    { "id": "search", "kind": "group", "shape": "section", "label": "Search",
      "color": "blue", "style": "boundary", "x": 1700, "y": 200, "width": 1000, "height": 1000 },
    { "id": "job", "parent": "capture", "kind": "node", "shape": "entity",
      "label": "Job", "style": "normal", "x": 100, "y": 160, "width": 600, "height": 300,
      "fields": [{ "name": "id", "type": "uuid", "key": "pk" }],
      "source": { "target": "urn:change-saga:app:data-entity:job",
        "revision": "urn:change-saga:app:data-entity:job:revision:r1" } },
    { "id": "document", "parent": "search", "kind": "node", "shape": "entity",
      "label": "Search document", "style": "normal", "x": 100, "y": 160, "width": 600, "height": 300,
      "source": { "target": "urn:change-saga:app:data-entity:document",
        "revision": "urn:change-saga:app:data-entity:document:revision:r1" } },
    { "id": "materializes", "kind": "connection", "style": "normal",
      "from": "capture/job", "to": "search/document", "label": "materializes",
      "label_box": { "x": 1160, "y": 380, "width": 440, "height": 100 },
      "source": { "target": "urn:change-saga:app:data-entity:job",
        "revision": "urn:change-saga:app:data-entity:job:revision:r1", "relationship": "materializes" } }
  ]
}
```

The connection crosses section boundaries but belongs to the same document.
Nested groups use local coordinates; inspection reports complete instance paths.
Add substantial overview narrative, local notes and evidence as the real model
requires. The canvas and opening camera can grow independently of the content.

## Optional imported content

A frame uses ordinary geometry plus its exact source pin:

```json
{
  "id": "identity",
  "kind": "frame",
  "parent": "cloud",
  "label": "Identity and tenancy",
  "summary": "Principals belong to organizations",
  "x": 40,
  "y": 100,
  "width": 900,
  "height": 600,
  "style": "normal",
  "source": {
    "target": "urn:change-saga:app:erd:identity",
    "revision": "urn:change-saga:app:erd:identity:revision:r3"
  }
}
```

The child retains its coordinate system and scales into the authored frame.
Nested frame transforms accumulate. Ordinary groups contribute their position
to the instance path and geometry; they do not create new source records.

Source references also belong on depicted entities, relationships and
explanations. For a relationship, pin its owning data entity and supply the
relationship ID in `source.relationship`. Drawing a line does not create a
recorded relationship. An explanatory line can instead describe its meaning
without claiming a recorded relationship.

## Compose an explanation at each scale

Use the same `diagram.Document` grammar and renderer used for slide diagrams:
styles, text, groups, annotations, notes, entities and source references remain
shared capabilities. Directly authored sections and optional imported content
share one spatial explanation and one revision history.

Start with a large headline, a short explanation of the system, and three or
four primary areas connected by meaningful relationships. Place the high-level
model beside this narrative and arrange detail along its reading path. Use
ordinary groups as narrative anchors where useful. Give surrounding areas room;
a large canvas does not need to fit on screen at normal zoom.

An optional root `viewport` defines the opening camera in board coordinates:

```json
"viewport": { "x": 0, "y": 0, "width": 3600, "height": 2400 }
```

This rectangle is separate from the full canvas dimensions. Size opening text
for this rectangle and the actual viewer area, not for fit-all. For example,
at 1140 × 760 screen pixels, the rectangle above gives a scale of about 0.317:
64-unit body text reads at about 20 pixels and a 100-unit headline at 32 pixels.
Allow for title/navigation overlays and check smaller viewports in the viewer.
Fit is an explicit atlas view for orientation, not the readability target.

Begin with the core data flow and its entities, then grow the document with
related sections. When reusing existing artwork, keep its intrinsic aspect ratio:
for source dimensions
`sw × sh` and an authored frame width `w`, use frame height `w * sh / sw`.
The full frame belongs to the child drawing. Reserve a separate caption band
**above** it for the title and summary: roughly three lines of the frame style's
font size plus padding. Account for that band when spacing adjacent frames,
callouts and connector routes.

Use a handful of larger core areas, smaller satellites and composite overviews
where they help explain the model. Give different meaningful neighborhoods
colored, labeled background sections around their related diagrams and narrative.
Reuse ordinary `group` elements with `shape: "section"` and a palette `color`
(`yellow`, `pink`, `blue`, `green`, `purple`, or `gray`). The shared renderer
supplies a subtle theme tint, border and contrasting title tab; the style controls
font size and stroke width. Keep narrative ink theme-aware too, for example
`@diagram-normal-ink`, so it remains readable on dark section tints.

```json
{
  "id": "delivery-area", "kind": "group", "shape": "section",
  "label": "Delivery", "color": "yellow", "style": "area",
  "x": 3910, "y": 60, "width": 6030, "height": 2720
}
```

Define `area` in `styles` with a suitable `font_size` and `stroke_width`.
Emit backgrounds before their contents. Reserve room for the title tab, complete
frame caption bands, callouts and connector labels, with generous gutters.
Use different extents to follow the explanation; preserve useful overviews,
satellites and whitespace instead of assigning every ERD a rigid tile. Labels
carry the meaning even without color. Color describes the reading organization;
it does not assert another source record, SQL boundary, deployment or authority
rule. Sections reuse the existing grammar and palette, not a new taxonomy.

Use `parent` and relative coordinates when a section should contain and move its
children; sections can nest. For an established board whose focus paths must
stay stable, a root-level section can serve as a labeled background without
reparenting existing elements. Those elements keep their current instance paths.
Author a reading direction with large headings and short source-backed callouts.
At the opening camera, the narrative and principal diagram structure should
be readable. Individual fields belong at focused detail scale. A fit-all atlas
may reduce even narrative text below reading size.

Actual child ERDs remain visible at overview scale. The viewer adds frame titles
and summaries above their drawings instead of replacing them with summary
cards. `detail_scale` controls connector simplification and internal interaction,
not whether the diagram is visible. Displayed connectors can meet enclosing
frames while descriptions retain their semantic endpoint paths. Optional
explicit bends control routes; the viewer does not run a layout engine.

Use existing annotations and notes inside structured children for local
explanations. Overview text needs its own authored styles. Font size depends on the authored camera: a small three-ERD fixture can use
34-unit overview text while a spacious board needs larger type. An annotation's explicit `target` point is
local to that annotation, not the board. `visibility: "detail"` inside a frame
follows that frame's detail state independently of other frames.

Focus a section or imported region, then select an entity or relationship to
inspect its meaning and source pin. Detailed fields are available on demand
through diagram controls and source inspection. The Details control restores
authored group content. Entity summaries retain the positions and source
inspection of the same bound entities. Safe SVG bindings can expose selectable
entities; this does not turn arbitrary SVG text into structured fields or
guarantee targeting of individual field rows. Changing displayed detail never
changes the source evidence. Repeated appearances use paths such as
`cloud/identity/organization` and `cross-store/organization`; neither path
changes the identity of the underlying entity.

## Shared presentation and evidence

The board uses the slide diagram grammar and renderer, including authored
styles, icons, groups, annotations, notes and code blocks. Structured reveal
steps can be replayed in the focused diagram; reduced-motion viewers see the
finished drawing. Hover or focus a noted grammar element to read its note in
the existing slide note popover; Escape dismisses it. Bound entities and
relationships share a light hover/focus outline and the existing evidence
drawer. This does not create slide Items or fragment-owned landmarks for an
ERD. Bundled icons must be authored explicitly: embedding an existing SVG does
not add icons or infer notes from its arbitrary descriptions.

The **Details** control starts off. Structured field rows are hidden until
requested. For supported authored SVGs, explicitly bound entity groups show
larger names in their original measured bounds; Details restores their exact
original contents. Relationship geometry and source identities stay unchanged.
This summarizes entities without claiming to identify fields in arbitrary SVGs.

Select an entity or relationship to open the same documentation and code drawer
used by slide references, including entities and relationships authored directly
in a native section. A section is a drawing group and does not invent an ERD
directory or evidence record. For an imported region, **Evidence** opens its
exact source ERD directory and owned relationships. Counts distinguish linked references, verified
saved bytes, current resolution, stale or unavailable references, and members
without references. None of these counts establishes complete model coverage.
Code inspection preserves the saved commit, path and lines even when current
code has moved. Opening a current definition never changes the board's pins.
The document does not claim aggregate code coverage merely because all its
drawn elements have source pins; inspect the saved references themselves.

## Camera and rendering cost

Camera input is batched once per animation frame. Geometry and selectable
controls are cached when the page opens; panning does not remeasure each SVG
or rewrite every control. Offscreen frames and structured decorations stop
drawing outside a 180-screen-pixel overscan margin. Their source identity and
DOM remain available, so focus, deep links and source navigation restore the
same instance. A keyboard control that leaves the rendered region transfers
focus to the viewport rather than becoming an invisible tab stop.

Bound SVG entity summaries are built when their frame first enters the margin.
Hover notes and reveal effects pause during dragging, wheel zoom and animated
focus. Visible diagrams and overview callouts stay drawn. Opaque SVG internals
are culled as a whole diagram; this is not network streaming or a guarantee
that arbitrary SVG annotations can be individually deferred. Source SVG parse
cost and visible SVG layout remain. See the [reproducible measurements](whiteboard-performance.md)
for the observed gains and their limits.

## Existing SVGs and structured children

An existing ERD SVG can be embedded as a whole drawing. Its source link and
recorded directory/bindings remain available. Safe, explicitly bound SVG entity
regions can be selectable and inspectable without a structured child document.
An SVG without structured source
does not become a structured diagram through embedding: arbitrary internal
elements and geometry cannot be inferred. Descriptions report this limit;
internal connection targeting requires known bounds. Whole-frame connections
work without internal geometry.

ERDs may also carry a `diagram` source. Publication renders the existing diagram
grammar into the ERD's SVG visual so that the ERD remains independently readable.
This structured source enables nested composition, entity/field descriptions
and internal targeting without reverse-engineering SVG geometry.

The loader verifies that a structured ERD's stored SVG matches its source.
Future renderer changes must preserve this rendering contract or explicitly
evolve the structured-source format; there is no per-revision renderer dispatch
in this prototype. Existing SVG-only ERDs do not have this dependency.

## Verification

`internal/requirements/whiteboard_native_test.go` publishes and describes a
document with nested sections and a recorded relationship crossing them,
without creating any child ERD record. Native viewer checks additionally cover
field detail, evidence inspection, section navigation and history.

The representative fixture in `internal/testfixture/whiteboard.go` combines
two structured ERDs with entity fields and local callouts, one existing SVG
ERD, an ordinary group, a recorded production connection and an overview
annotation. It deliberately repeats the same entity sources across views.

Review the opening camera, an optional fit-to-board atlas, and a focused detail
view in a real browser. From the opening view, a reader should be able to explain
what happens and choose the next area. At detail, fields and local explanations
should be readable, with a path back to the system and original source revision.
Verify all embedded appearances remain reachable, including repeated pins with
distinct instance paths. Check the opening after navigation controls are present;
a standalone diagnostic SVG does not verify viewer interaction.

See the [implementation validation record](whiteboard-validation.md) for the
representative and real-data browser checks.

## Reproducible DayLight composition

The [example authoring script](examples/author-daylight-whiteboard.py) composes
20 existing DayLight ERDs from exact revision IDs. It uses only the Python
standard library and reads a local saga copy; it neither publishes a board nor
changes source records. The DayLight source data is not bundled in this repo.
Run it against the matching copied inventory:

```sh
python3 docs/examples/author-daylight-whiteboard.py \
  /path/to/copied/daylight.saga /tmp/daylight-composition
```

The outputs are `daylight-board.json`, `source-pins.json`, `source-audit.json`,
`camera-proposal.json`. Optional diagnostic previews use a supplied shared-renderer
SVG, as described below. The manifest records all 20 exact pins, explanations,
intrinsic dimensions, revision-file digests and verified SVG digests. Missing
pins or visual digest mismatches stop generation; future revisions are never
adopted implicitly. Source revision and visual files are checked again before
writing the outputs. Generation does not publish or modify the source saga.

The primary composition uses native, source-pinned entities and explicit
relationships in Delivery, Search and Authority sections. Its opening camera
and typography establish a readable starting point rather than fitting the
whole model on one screen. Cross-section connections carry their recorded
relationship references or explicitly state their explanatory meaning.

The existing source drawings remain in a deep-detail appendix within the same
canvas, preserving the original explanations and model coverage. They are
migration/provenance material rather than the document's primary outline.
Exact frame pins and instance paths keep those appearances inspectable. The
script validates layout and source consistency before writing its outputs.

The audit explicitly distinguishes committed-code documentation from live
database observation. All current pins are SVG-only ERDs: their directories
and bindings retain evidence, but embedding does not infer structured field
geometry. Their complete fields remain available through source inspection.
Narrative paraphrases come from the pinned explanations; explanatory connectors
carry source pins and descriptions and do not assert new SQL foreign keys.
In particular, file cursors are not delivery receipts, provisioning is not
transcript access, and membership and exact-source consent are separate checks.
The cross-store ERD identifies its payload-to-delivery line as the physical FK;
other displayed mappings keep their documented meaning.

Run the shared composition renderer to check exact text wrapping and source
resolution. The script deliberately does not implement a second renderer. Once
you have its full-canvas SVG for this exact document, regenerate with
`--rendered-svg /path/to/renderer-check.svg` to produce `layout-preview.html`,
`initial-preview.svg`, and `layout-preview.svg`. The preview starts at the
opening camera and offers a Fit atlas button. Inspect the full layout and each
colored neighborhood in both light and dark themes, checking tab clearance and
text contrast; source diagrams remain visible.

Then use the application viewer for real connector routing, camera, Details,
inspection and focus checks. A standalone shared-renderer SVG is only a layout
proof, not a viewer-interaction check. Publishing is a separate delivery step
after that review. Preserve the observed parent revision when publishing the
approved update.
