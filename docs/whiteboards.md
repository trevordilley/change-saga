# ERD whiteboard

Status: proposal for `feature/erd-documents`. Start with ERDs; use what we learn
before considering Infra or Systems. Nothing here is a format contract yet.

## The idea

A whiteboard is a diagram of diagrams. It places existing ERDs together on a
large, zoomable surface, with connections and explanatory text showing how
they fit together. The AI authors the layout; the reader pans and zooms.

The zoomed-out view must be as useful as the zoomed-in view. Authors compose
an explanation at each scale: the overview explains the major areas and their
relationships, while the detail explains individual entities and fields.
Annotations, callouts, labels and connections give both views their meaning.

DayLight's in-progress ERD motivates this: the author describes roughly eleven
diagrams that are really parts of one larger picture. That picture is too
large to read at a single fixed size. A whiteboard lets a reader see the whole,
then zoom into an individual diagram and its tables without leaving it.

The smaller diagrams remain useful on their own. Embedding them gives them a
place in the larger ERD and lets their existing detail do the work.

## Composition

Extend the existing
[diagram source](../skills/change-saga/references/diagrams.md#author-diagrams-as-source)
with enough grammar to embed, position and connect diagrams:

- A **frame** references a child diagram and gives its position and size. The
  child retains its own coordinates and scales into the frame. Frames can nest.
- A **source reference** identifies the record and revision a diagram or element
  depicts. Reuse existing ERD directory pins and bindings where available.
- A **connection** can address a frame or an element inside it, for example
  `cloud/identity/organization` to `cloud/oauth/client`. The path selects a
  particular appearance on the board.

The author can use ordinary groups, labels, annotations, callouts, notes and
connectors at the board level and inside embedded diagrams. An overview
callout might explain why two stores exist or how records move between them;
a local annotation might explain a particular relationship. These explanations
reference their sources just as the diagrams do. A group called "Cloud" is a
diagram group. Existing ERD and data-entity records supply the source material.

A source record can appear in several embedded diagrams. Each appearance
references the same source, while keeping its own visual identity. Authors
choose the repetition and grouping that make the picture understandable.

## Referencing the source

Authors must be diligent about source references. A reader or AI should be
able to follow an embedded diagram to its source ERD and revision, and a
depicted entity or relationship to its recorded definition. Labels alone do
not establish those links. An explanatory connector should make its meaning
clear; drawing a line does not create a recorded data relationship.

Moving or scaling a frame changes its placement on the board. Source revisions
and entity pins retain their existing meaning. The prototype needs to settle
how a board adopts a newer child revision and exposes the version it displays.

## Reading and authoring

Start with a read-only SVG viewer on the Data model page, using the existing
renderer with pan, zoom, fit-to-board and focus-on-diagram controls. At an
overview scale, titles, summaries, annotations, callouts and connections should
be legible and explain the whole. Their placement and size are part of the
authored composition. Zooming in reveals the embedded diagrams and their local
explanations. Try a simple detail threshold if the overview is too crowded,
while preserving the context needed to understand each view.

Keep the meaning of a connection when simplifying its display. An edge between
two entities may meet their enclosing frames when the detail is collapsed,
while inspection still identifies the underlying endpoints. Resolve endpoint
positions through the frame transforms; begin with simple connectors and
optional authored bends.

Extend `diagram describe` so an AI can inspect the board, a child diagram or an
element, including annotations, callouts, source references and connections.
Proposed commands:

```sh
change-saga diagram describe --whiteboard erd
change-saga diagram describe --whiteboard erd --focus cloud/identity
change-saga diagram describe --whiteboard erd --focus cloud/identity/organization
```

An existing SVG can be embedded as a whole diagram with a source link. Detailed
inspection and connections into it depend on available structured source or
bindings; the prototype should expose that distinction. Existing ERDs and
their immutable revisions remain independently readable.

## First prototype

1. Embed two or three existing ERDs on one board, with explicit source
   references, positions and sizes.
2. Author overview annotations, callouts and connections, and support pan,
   zoom and focused inspection.
3. Compose the rest of DayLight's ERD and adjust the layout and visible detail
   until the whole picture reads well.

The test is practical at both scales: without zooming in, can someone explain
the major areas, how they relate, and the points the overview calls out? After
zooming in, can they understand a table and its relationships while retaining
that context? Can they trace the diagrams and explanations to their sources?
Use those results to guide further work. Infra and Systems are later
possibilities.
