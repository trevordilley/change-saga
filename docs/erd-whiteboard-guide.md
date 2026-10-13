# Authoring an ERD whiteboard

An ERD whiteboard composes existing ERD revisions using the diagram grammar.
The AI authors the positions, sizes, narrative, annotations and connections;
the Data model viewer provides read-only pan, zoom, fit and focus. The board fills
the viewport beside the sidebar. Its floating title has an expandable description
using the shared data-model overview. Sidebar and board ERD links focus their
embedded appearances in the same canvas. “Diagrams on this board” is a focus
picker with pin metadata. Repeated appearances have distinct focus URLs. Fit
and Focus leave room for the title and navigation controls. Source ERDs remain
independently revisioned internally, and explicit historical routes remain
compatible. Groups are ordinary drawing groups; this feature does not change
entity holders or add inventory kinds.

## Revision policy

Every frame names an exact source `target` and `revision`. Publishing a board
creates an immutable JSON revision under `___whiteboards/erd/revisions/`.
An update names the current board revision as its parent. A board never follows
the latest revision of a child implicitly: adopting a revised ERD means changing
the frame's pin and publishing a new board revision. Older boards and ERDs stay
readable with their original pins. Repeated appearances can share a source pin
while retaining distinct instance paths.

## Publish and inspect

Author a `diagram.Document` JSON file using the existing styles, groups,
entities, text, notes, annotations and connectors. Add `frame` elements to
embed ERDs, and `connection` elements to connect appearances through their
instance paths. Publish it with:

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

A frame uses ordinary geometry plus its source pin:

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
shared capabilities. Frames compose actual pinned ERDs within that grammar.
The board is a spatial explanation, not a separately maintained set of summary
cards or a screenshot of a slide deck.

Start with a large headline, a short explanation of the system, and three or
four primary areas connected by meaningful labels. Place the high-level system
ERD beside this narrative, then put chosen detail ERDs farther along the reading
path. Use ordinary groups as narrative anchors where useful. Give surrounding
areas room; a large canvas does not need to fit on screen at normal zoom.

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

Begin with a few core ERDs and compose their actual drawings into a spatial
explanation. Keep the source SVG's intrinsic aspect ratio: for source dimensions
`sw × sh` and an authored frame width `w`, use frame height `w * sh / sw`.
The full frame belongs to the child drawing. Reserve a separate caption band
**above** it for the title and summary: roughly three lines of the frame style's
font size plus padding. Account for that band when spacing adjacent frames,
callouts and connector routes.

Use a handful of larger core areas, smaller satellites and composite overviews
where they help explain the model. Group related views through proximity and
whitespace; avoid giving every ERD the same slot in a tiled index. Author a
reading direction with large section labels and short source-backed callouts.
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

Focus a frame to read its child diagram, then an entity or relationship to
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
finished drawing. Select a noted element to read its note. This does not create
slide Items or fragment-owned landmarks for an ERD.

The **Details** control starts off. Structured field rows are hidden until
requested. For supported authored SVGs, explicitly bound entity groups show
larger names in their original measured bounds; Details restores their exact
original contents. Relationship geometry and source identities stay unchanged.
This summarizes entities without claiming to identify fields in arbitrary SVGs.

Select an entity or relationship to open the same documentation and code drawer
used by slide references. **Evidence** opens the focused ERD's exact directory
and its owned relationships. Counts distinguish linked references, verified
saved bytes, current resolution, stale or unavailable references, and members
without references. None of these counts establishes complete model coverage.
Code inspection preserves the saved commit, path and lines even when current
code has moved. Opening a current definition never changes the board's pins.

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
`camera-proposal.json`, and a diagnostic `layout-preview.html` with initial and
full-canvas SVG assets. The manifest records all 20 exact pins, explanations,
intrinsic dimensions, revision-file digests and verified SVG digests. Missing
pins or visual digest mismatches stop generation; future revisions are never
adopted implicitly. Source revision and visual files are checked again before
writing the outputs. Generation does not publish or modify the source saga.

The 10000 × 10000 canvas opens at `{x: 0, y: 0, width: 3600, height: 2400}`.
The 100-unit headline, 82-unit area headings and 64-unit body text explain the
system beside the actual cross-store ERD. Three ordinary group anchors—
`delivery-guide`, `search-guide`, and `authority-guide`—describe what happens
and connect to the system model with labeled explanatory lines. The diagram is
a composite view of the same models, not an additional processing stage.

Delivery ERDs sit to the right: durable observation, destination debt, exact
source admission and hosted corpus. Local scope, index progress and retrieval
sit below, with episode projections and the whole local model farther out.
Project authority, identity and consent anchor the next area; destinations,
OAuth and invitations continue it. Provisioning, messaging and the whole cloud
directory occupy the perimeter. Optional tour rectangles and anchor IDs are
listed in `camera-proposal.json`; these are authoring suggestions, not an extra
tour schema or automatic camera sequence.

All 20 source ERDs remain embedded, at their intrinsic aspect ratios, across
21 frame appearances. `system-return` repeats the exact `local-cloud-overview`
pin near the far perimeter. It has a distinct instance path and focus target;
repetition does not create another entity or a different source revision.
Every frame reserves a separate 160-unit title band. The script checks frame,
caption and narrative-text bounds and overlaps, as well as unique element IDs.
The diagnostic preview starts at the opening camera and offers a Fit atlas
button; source diagrams are never replaced by cards.

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

Run the shared renderer to check exact text wrapping and source resolution,
then use the application viewer for real connector routing, camera, Details,
inspection and focus checks. The diagnostic HTML is only a layout proof.
Publishing is a separate delivery step after that review.
