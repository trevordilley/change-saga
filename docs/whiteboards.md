# One data-model document on a whiteboard

Status: revised product direction for `feature/erd-whiteboard`. ERD first.
The [authoring guide](erd-whiteboard-guide.md) describes the implemented grammar,
revision policy and viewer. Infrastructure will eventually use the same document
model; it is outside this implementation.

## The document

The system has **one ERD document**, expressed on a large, zoomable whiteboard.
It explains the system data model through selected entities, relationships,
meaningful sections, narrative, annotations and evidence. It is one cohesive
explanation with room to spread out. Its size is determined by what makes the model readable,
not by a slide, one screen, or a fixed SVG viewport.

This supersedes the earlier framing of a board as a catalog of independent ERD
documents. Diagrams within diagrams are a composition technique. They do not
require the author or reader to maintain a collection of separate documents.
New content can be authored directly in the whiteboard, with sections containing
entities and connections crossing section boundaries. A section is part of the
same document, not another inventory record.

The whiteboard is the comprehensive explanatory document, not an exhaustive
schema inventory. Choose the concepts and relationships that help a human
understand the system. It need not depict every table, column or recorded edge.
What it does depict must have evidence; omission is preferable to an invented
association. Original drawings need not remain on the canvas. Historical source
records can retain detail without turning the board into their catalog.

Existing ERDs are useful migration material. Their artwork, explanations, exact
revisions and source bindings can be reused internally. That compatibility does
not define the document's outline or oblige the reader to navigate the old ERD
list. Historical records remain readable; the data-model page presents the
cohesive document.

DayLight motivates the change: its many focused drawings describe one larger
model. A good result carries their richness forward while making the connections
between capture, delivery, search, identity and authority explicit. Placing the
old drawings beside one another is only an intermediate migration step.

## Reading at different scales

The opening view explains the overall model with legible titles, summaries,
callouts and meaningful relationships. It gives readers a direction and context.
It does not need to show every part of the document at once. Fit-to-board is an
optional map for orientation, not the required reading scale.

Readers pan or zoom into a section, follow a relationship into another section,
and inspect entities or fields as needed. Local explanations and code evidence
remain available at detail. Dense field lists should not obscure the high-level
data model. An outline navigates places within this one document; it is not a
catalog of source documents. The whiteboard fills the space beside the sidebar,
with a floating title and expandable description.

## Shared composition grammar

Reuse the existing
[diagram source](../skills/change-saga/references/diagrams.md#author-diagrams-as-source)
and renderer: styles, icons, entities, text, groups, annotations, callouts, notes,
code blocks and connectors. The AI authors the layout, typography and reading
order. The viewer starts read-only.

- Ordinary **groups**, including `shape: "section"`, organize related content.
  Groups can nest and contribute local coordinates without creating new records.
- **Entities and relationships** can be authored directly in the document and
  pinned to their recorded definitions. Child ERD documents are optional.
- A **frame** can reuse exact existing ERD artwork or structured source where
  helpful. Its child coordinates scale into the authored region. Frames can nest.
- A **connection** can cross any section boundary and address a particular
  appearance by path. Its semantic endpoints remain intact when the displayed
  connector is simplified at overview scale.

Use the visual language of the existing implementation diagrams: colored
section headers, document and database shapes where their meanings fit, a few
clear concepts, short explanations, callouts and deliberately labeled links.
Color should distinguish responsibilities or outcomes, not decorate an otherwise
dense table grid. Give the story generous space and large overview text. Keep
selected field detail local and optional. Author straightforward connector routes
with optional bends.
Avoid an elaborate automatic layout or routing engine. Shared slide presentation
capabilities apply where their semantics fit; ERDs do not need fabricated slide
Items or fragment-owned evidence records.

## Relationships and evidence

Every meaningful line should explain how its endpoints relate. Distinguish a
recorded data relationship from transport, derivation, authorization context or
a repeated appearance of the same entity. Neither proximity nor a drawn line
creates a foreign key. Cross-section connections need the same source diligence
as connections inside a section. Use the shared crow’s-foot markers for declared
association cardinality at both ends, including optionality. Preserve explicit
unknown cardinalities instead of guessing. Directional transport and production
connections keep their distinct semantics.

Pin depicted entities and recorded relationships to exact definitions. A
relationship reference includes its owning entity revision and relationship ID.
Preserve commit/path/line evidence through the existing reference features.
Overview explanations also identify their sources. A reader should be able to
inspect the meaning and evidence without leaving the model's spatial context.
Hover and keyboard focus use the implementation slides’ annotation action strip,
with the same note, documentation, code and story controls. Resolve those actions
from actual references: an entity does not acquire story ownership merely because
it appears near another element, and a slide Item’s links retain that provenance.
Repeated appearances remain allowed and have distinct instance paths while
sharing the same underlying source identity.

Publishing creates a complete immutable revision of the one whiteboard document.
If an imported frame changes, adopting a different child revision is an explicit
edit and publication; historical boards never silently follow new source heads.
Older ERDs and revisions remain readable for compatibility. No holder semantics
or inventory kinds are changed.

## Inspection and viewer behavior

`diagram describe` provides meaningful structured inspection of the whole model,
a section, entity, connection or imported region, including source references:

```sh
change-saga diagram describe --whiteboard erd change.saga
change-saga diagram describe --whiteboard erd --focus cloud/identity change.saga
change-saga diagram describe --whiteboard erd --focus cloud/identity/organization change.saga
```

Pan, zoom, fit and focus keep readers in the same document. Focus URLs and browser
history restore a place in it. Offscreen content may stop drawing and detail can
be deferred, but restoration, keyboard access and source inspection must remain
predictable. Visible explanations must not disappear merely to improve a score.

An existing SVG can be reused as a whole drawing. Internal targeting and
structured inspection depend on actual structured source or explicit bindings
with known geometry. Do not infer fields, relationships or evidence from pixels.
Expose those limits honestly while retaining whole-region connections and the
source directory where available.

## Acceptance

A representative native document must work without creating child ERD records:
multiple sections, pinned entities, cross-section connections, overview narrative,
field detail and source inspection all belong to the same whiteboard revision.
The migrated DayLight document must preserve the existing model's richness and
connect its meaningful parts rather than presenting a gallery of thumbnails.

Review the opening, regional and detailed views in a real browser. Can a reader
explain the system model, follow the relationships between sections, choose the
detail they need, and trace it to code? Does navigation stay within one document?
Those are the product tests. Later, one infrastructure whiteboard document will
serve the equivalent purpose for infrastructure; there is no requirement for it
to fit in a single screen-sized diagram either.
