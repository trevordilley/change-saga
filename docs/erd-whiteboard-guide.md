# Authoring an ERD whiteboard

An ERD whiteboard composes existing ERD revisions using the diagram grammar.
The AI authors the positions, sizes, summaries, annotations and connections;
the Data model viewer provides read-only pan, zoom, fit and focus. The board fills
the viewport beside the sidebar. Its floating title has an expandable description
using the shared data-model overview; diagram sources and directories open in a
separate overlay. Fit and Focus leave room for the title and navigation controls.
ERDs remain independent documents. Groups are ordinary drawing groups, and this feature
does not change entity holders or add inventory kinds.

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
At fit, diagrams should remain recognizable and major annotations readable;
every individual field need not be readable until focused.

Actual child ERDs remain visible at overview scale. The viewer adds frame titles
and summaries above their drawings instead of replacing them with summary
cards. `detail_scale` controls connector simplification and internal interaction,
not whether the diagram is visible. Displayed connectors can meet enclosing
frames while descriptions retain their semantic endpoint paths. Optional
explicit bends control routes; the viewer does not run a layout engine.

Use existing annotations and notes inside structured children for local
explanations. Overview text needs its own authored styles. In the three-ERD
fixture, a 34-unit overview font remains readable at fit while children use
their normal 22-unit detail font. An annotation's explicit `target` point is
local to that annotation, not the board. `visibility: "detail"` inside a frame
follows that frame's detail state independently of other frames.

Focus a frame to read its child diagram, then an entity or relationship to
inspect its meaning and source pin. Repeated appearances use paths such as
`cloud/identity/organization` and `cross-store/organization`; neither path
changes the identity of the underlying entity.

## Existing SVGs and structured children

An existing ERD SVG can be embedded as a whole drawing. Its source link and
recorded directory/bindings remain available. An SVG without structured source
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

Review both a fit-to-board view and a focused detail view in a real browser.
At fit, a reader should be able to explain the main areas and their connections.
At detail, fields and local explanations should be readable, with a path back
to the whole and to the original source revision.

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

The outputs are `daylight-board.json`, `source-pins.json`, and a diagnostic
`layout-preview.html` with its SVG asset. The manifest records every exact pin,
source explanation, intrinsic dimensions and verified SVG digest. Missing pins
or digest mismatches stop generation; future revisions are never adopted
implicitly. The script checks frame and caption-band bounds and overlaps before
writing the board. The preview shows the source SVGs at the authored sizes; use
the application viewer to verify real connector behavior, inspection and focus.

The composition's reading path is capture, the cross-store composite, source
admission and cloud corpus. The composite is another view of those models, not
an extra processing stage. Local search and episode views form a separate left
cluster; project authority and identity anchor the lower cloud cluster, with
consent, invitations, provisioning and messaging around them. Whole-model
directories sit at the perimeter as reference views. Sizes reflect narrative
emphasis and source proportions rather than a uniform grid.

Callouts paraphrase pinned explanations. Connections describe explanatory
mappings and delivery context with their own pins; they do not invent SQL
foreign keys. The authored caption band is 192 board units for a 51.2-unit frame
font. Publishing remains an explicit subsequent action after browser review.
