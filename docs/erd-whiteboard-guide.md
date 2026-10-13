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
one screen. Select entities and relationships to explain the system clearly;
a complete table or column inventory is not required. Every depicted concept
and relationship needs evidence. Use abstraction deliberately and distinguish
conceptual flows from recorded associations. Source revisions and historical
routes remain available internally for provenance and compatibility. Groups
are ordinary drawing groups; this feature does not change
entity holders or add inventory kinds.

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
Publication is atomic and rejects an update whose parent is no longer the
current revision. Structured `describe` preserves focused instance paths,
fields, semantic endpoints and exact source pins. The current whiteboard CLI
supports whole-document `publish`, `get`, `describe` and `check`; it does not yet
support the slide diagram transaction-based `edit` command.

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
relationship ID in `source.relationship`. For an association drawn owner →
destination, map declared owner cardinality to `tail` and destination cardinality to `head`: `1` → `only-one`, `0..1` →
`zero-or-one`, `0..many` → `zero-or-many`, and `1..many` → `one-or-many`. Use
`head_size` to keep both endpoint markers legible at the reading scale. Keep
unknown cardinality explicit instead of substituting an arrow or guessing.
Drawing a line does not create a recorded relationship. An explanatory line can
instead describe its meaning without claiming a recorded relationship.

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
finished drawing. Hover or focus an element to reveal the same action strip
used by implementation slides. Its information control opens the shared note
popover; documentation and code actions open the existing drawers. The strip
loads metadata for the exact saved pin on demand, and code is loaded when its
panel opens. Repeated appearances retain their own authored notes. Long
relationships place pointer actions beside the visible segment; keyboard focus
uses the cached diagram position.

A conceptual shape may pin an existing ERD as its broader evidence scope.
Its code action then uses the existing ERD directory evidence collector: saved
member and relationship references are grouped with their exact owners and
revisions. The drawer labels that broader scope explicitly; it does not claim
that all those references belong to the conceptual shape or infer a subset from
its prose. Missing members and members without code remain visible. Metadata
collection does not fetch source bytes; opening Code performs that work.

Story actions appear only when real implementation Items pin the exact saved
definition and have saved story links. The panel identifies those Items as the
source of the links. An entity pin alone does not create a story relationship,
and entity-wide links are not presented as relationship-specific evidence.
This does not create slide Items for an ERD. Bundled icons must be authored
explicitly: embedding an existing SVG does not add icons or infer notes from
its arbitrary descriptions.

The **Details** control starts off. Give a native entity a concise authored
`summary` to explain its role at overview scale. The shared entity renderer uses
the entity style for its title and a smaller wrapped summary in the same bounds;
the whiteboard swaps that summary with the complete fields when Details changes.
Source identity and connector endpoints do not change. For entities with an
authored summary, field names and types occupy compact adjacent columns so they
remain readable together when zoomed in. The fixed row metrics stay unchanged;
ordinary entities without summaries retain their original artwork. Standalone
diagrams keep their full fields by default. Structured field rows are hidden
until requested.
For supported authored SVGs, explicitly bound entity groups show
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
Hover controls, notes and reveal effects pause during dragging, wheel input and animated
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
one connected explanation from exact DayLight entity and relationship records.
It uses the Python standard library and reads a local saga copy; it neither
publishes a board nor changes source records. The DayLight source data is not
bundled in this repo. Run it against the matching copied inventory:

```sh
python3 docs/examples/author-daylight-whiteboard.py \
  /path/to/copied/daylight.saga /tmp/daylight-composition
```

The generated board and audit distinguish the selected visual model from the
larger source catalog. The catalog is evidence for authoring, not a requirement
to put every table, field or edge on the canvas. Review the generated source
pins and relationship evidence alongside the picture. Exact source revisions
remain fixed; generation must not silently adopt later definitions or modify
the input saga.

The visual target is the existing implementation-slide language: colored
sections, meaningful document/database shapes, short descriptions, annotated
connections and explicit outcomes. Spread that explanation over a whiteboard,
without imposing the slide's screen-sized boundary. Source metadata and field
details belong behind the existing evidence controls or in a chosen local
inspection view, rather than competing with the overview story.

Recorded associations retain their exact endpoint semantics and cardinalities.
Conceptual transport, derivation and authorization links are labeled as such
and do not claim new SQL foreign keys. In particular, file cursors are not
delivery receipts, provisioning is not transcript access, and membership and
exact-source consent are separate checks. Evidence describes committed code;
it does not establish live database state or complete implementation coverage.

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
