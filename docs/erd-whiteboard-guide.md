# Authoring an ERD whiteboard

An ERD whiteboard composes existing ERD revisions using the diagram grammar.
The AI authors the positions, sizes, summaries, annotations and connections;
the Data model viewer provides read-only pan, zoom, fit and focus. ERDs remain
independent documents. Groups are ordinary drawing groups, and this feature
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

Begin with two or three ERDs. Give every frame a short title and meaningful
summary, then author overview text and callouts large enough to read at fit.
Use connections to explain the relationships between areas. A collection of
small diagram thumbnails is not a useful overview.

Use the existing annotations and notes inside structured children for local
explanations. The viewer can collapse child detail to frame summaries at low
scale. Displayed connectors can meet the enclosing frames while descriptions
retain their semantic endpoint paths. Optional authored bends provide simple
route control; the viewer does not run a layout engine.

Overview text needs its own authored styles. In the three-ERD fixture, a
34-unit overview font remains readable at fit while children use their normal
22-unit detail font. An annotation's explicit `target` point is local to that
annotation, not the board. `visibility: "detail"` inside a frame follows that
frame's detail state even when another frame is collapsed.

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
