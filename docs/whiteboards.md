# Whiteboards

Status: proposal. A prototype of the ERD whiteboard is being built on
`feature/erd-documents`. Nothing here is a format contract yet.

## The problem

A Saga can record dozens of data entities and explain them in many small ERDs.
Each ERD is a good, focused picture, but nothing places them in a whole. The
DayLight Saga has 76 data entities, 10 Components, one System and 20 ERDs. Its
ERD SVGs total 3.1 MB because shared entities are redrawn in several pictures,
and its authors tried to recover the big picture by hand with diagrams named
`cloud-overview`, `local-whole-model` and `local-cloud-overview`. A reader can
see the trees but not the forest.

## Two kinds of diagram

- **Slides** explain one thing. A review or implementation slide is an SVG
  that fits the slide and never needs zooming.
- **Whiteboards** show where everything sits. A whiteboard is a diagram of
  diagrams: it places smaller diagrams inside larger ones, connects their
  parts, and is read by zooming. Zooming out shows how the pieces relate;
  zooming in shows a single table or Component.

A slide element can link to a whiteboard element, so a reader can go from an
explanation to its place in the whole system.

## Three whiteboards per Saga

| Whiteboard | Groups | Atoms | Shows |
| --- | --- | --- | --- |
| Infra | Groups | Resources | Infrastructure and how data moves between it: databases, queues, buckets, hosts. |
| ERD | Domains, Data Models | Data entities | Every store's data and how the parts relate. An ERD is a collection of Data Models. |
| Data Flow | Systems | Components | Logic, and how data is transformed from one shape to the next. |

Each atom has one meaning. Components are units of logic and transformation;
they never own data. Resources are infrastructure; a data entity is held by a
Resource. Data entities are logical or persisted shapes of data.

These links are what connect the whiteboards. A data entity's holder is a
Resource on Infra; a Component that reads or writes an entity connects Data
Flow to the ERD.

## Records

- **Resource** and **Group** are new inventory kinds, for Infra.
- **Domain** is a new inventory kind: the grouping above a Data Model, such as
  `cloud` and `local`.
- A **Data Model** is what an ERD record is today: a focused diagram over a
  directory of data entities.
- **System** and **Component** keep their current meaning.

Data-entity `holders` are currently typed as Component pins, a stand-in for the
missing Resource kind, and `docs/erd-documents.md` tells authors to record each
store as a Component. With Resource available, holders gain a Resource pin. An
existing Component holder stays valid and readable, is reported as the legacy
form, and is never rewritten.

## Grammar

A whiteboard and each diagram on it use the existing
[diagram source](../skills/change-saga/references/diagrams.md#author-diagrams-as-source):
explicitly positioned elements, laid out by the author. The AI does the layout
here as it does for slides. Three additions:

- **`frame`** places a child diagram, by ID, at explicit bounds in its parent.
  The child keeps its own coordinate space and is scaled into the frame. Each
  child is its own record, so editing it never touches its parent, and it can
  be reused when frames are rearranged.
- **`record`** on an element names the Resource, data entity, Component or
  other record it depicts.
- **Cross-diagram edges** name exact elements inside frames by path, at any
  depth, for example
  `{"from": "cloud/identity/organization", "to": "cloud/oauth/client"}`. An
  edge always ends on its element, never on the enclosing frame.

A record appears at most once per whiteboard. Other diagrams reach it with a
cross-diagram edge instead of drawing it again; duplicated drawing is what
hides the forest today.

The author cannot aim points at an element inside a scaled child, so the
renderer routes cross-diagram edges. It composes the frame transforms to find
the element's exact position; the author may give bend points in the parent's
coordinates.

## Rendering and zoom

Whiteboards are SVG drawn by the server with the existing diagram renderer.
Frames are nested `<svg viewBox>` elements, which provide the scaling. A small
script pans and zooms by changing one transform and sets the current level of
detail. CSS hides detail below a frame's threshold: zoomed out, a frame shows
its title and summary; zoomed in, its contents. A cross-diagram edge whose
element is too small to draw ends at an anchor marker at that element's exact
position.

SVG keeps links, keyboard focus, screen readers, find-in-page, theme tokens and
the browser test selectors. Canvas would only pay off with tens of thousands of
visible elements, which level of detail avoids. If a Saga ever needs it, only
the renderer changes.

## Describe

`diagram describe` works at any depth:

```sh
change-saga diagram describe --whiteboard erd
change-saga diagram describe --whiteboard erd --focus cloud/identity
change-saga diagram describe --whiteboard erd --focus cloud/identity/organization
```

At a level, it describes that level's frames and the cross-diagram edges
between them, with each edge's exact endpoints, for example
`cloud/identity (9 entities) -> cloud/oauth via organization.id ->
client.organization_id`. The AI drills down with `--focus`.

## Editing

The viewer is read-only. Its main editing aid is **copy edit prompt**: select a
frame or element and copy a prompt naming the whiteboard, the element's path
and the matching `describe --focus` command, then add the change you want.
Comments may come later.

## Compatibility

Whiteboards and the new kinds are optional and additive. Existing ERDs, their
immutable revisions and SVGs stay valid. A hand-authored ERD SVG can be placed
in a frame for compatibility, but it cannot be described or linked into; a
describable whiteboard needs diagram-source Data Models. Nothing is migrated or
rewritten automatically. Older readers reject the new kinds, so upgrade readers
before adopting them.

## Prototype

1. Add `frame`, `record` and cross-diagram edges to diagram source, with nested
   rendering, level of detail and `describe --focus`.
2. Add a read-only ERD whiteboard with pan and zoom on the Data model page.
3. Have an agent author DayLight's ERD whiteboard from its recorded entities,
   with `cloud` and `local` Domains, and judge whether the whole reads.

Infra, Data Flow, Resource holders and slide-to-whiteboard links follow once
the ERD whiteboard works.

## Open questions

- How whiteboards and their frames are stored: one record per diagram, or a
  whiteboard record that owns its frames.
- Whether a frame's level-of-detail threshold is authored or derived from its
  rendered size.
- How cross-diagram edges are routed so that they stay readable.
