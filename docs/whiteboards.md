# Whiteboards for technical design

A Technical design explains how an application works through one approach essay
and three whiteboards. The essay is the starting point at `/technical`; supporting
pages expand decisions and link back into the relevant diagrams.

| Part | What it explains | Document / route |
| --- | --- | --- |
| General approach | Architecture, responsibilities, important decisions and tradeoffs, with links into the diagrams. | `/technical` |
| ERD | Entities and relationships with proper crow's-foot cardinality and optionality. | `data-model` · `/technical/erd` |
| Infrastructure | The actual layout of platforms, deployed services, resources, boundaries and connections. | `infrastructure` · `/technical/whiteboards/infrastructure` |
| Data flows | How data moves through the application, including key services, stores, transformations, inputs and outputs. | `data-flows` · `/technical/whiteboards/data-flows` |

Systems and Components are no longer Technical design navigation areas. Existing
inventory definitions remain readable through saved evidence links. Authors can
use the shared diagram grammar to explain platforms and services directly.

Write the essay and optional supporting pages as Markdown:

```sh
change-saga overview set-technical-design --source approach.md change.saga
change-saga overview set-technical-design --page reliability \
  --title "Reliability decisions" --source reliability.md change.saga
```

The main essay lives in `___overview/technical-design.fragment`; supporting
pages live in `___overview/technical-design-<page>.fragment` and open at
`/technical/pages/<page>`. Later writes replace content while preserving fragment
identity; Git records the prose history. `--title` sets the title on creation.
Link with ordinary Markdown, for example `[Infrastructure](/technical/whiteboards/infrastructure)`
or `[Retry path](/technical/whiteboards/data-flows?focus=delivery/retry)`.

For ERD relationships, use the renderer's `head` and `tail` markers:
`only-one`, `zero-or-one`, `one-or-many`, or `zero-or-many`, according to the
recorded relationship. The crow's foot goes at the many end; circles and bars
express optionality and required participation. An arrow or a text “N” does
not replace a crow's-foot marker. Infrastructure and data-flow connections use
directed arrows where direction matters, with labels explaining their meaning.

Each whiteboard is a cohesive spatial document with nested sections and room to
pan, zoom and inspect detail. All three use the shared diagram grammar and
independent revision histories. They are authoring conventions, not new
inventory kinds or different JSON formats.

## Named documents

A Saga can hold several independent whiteboards. Each has a document ID, title
and immutable revision history. All use the same shared diagram grammar and
whiteboard JSON contract; data models, infrastructure and data flows are
authoring subjects, not kinds. Use `change-saga whiteboard
publish|get|describe|check --id ID`. For example, publish separate documents:

```sh
change-saga whiteboard publish --id data-model --revision r1 \
  --name "Application data" --from data-model.json change.saga
change-saga whiteboard publish --id infrastructure --revision r1 \
  --name "Infrastructure" --from infrastructure.json change.saga
change-saga whiteboard publish --id data-flows --revision r1 \
  --name "Data flows" --from data-flows.json change.saga
```

The files contain complete diagram documents. Each board is stored under
`___whiteboards/<board-id>/revisions/<revision-id>.json` and opens at
`/technical/whiteboards/<id>`. `--id` always names the document; `--focus PATH`
selects an element inside it. There is no kind selector or `diagram` command alias.
The existing Data model page at `/technical/erd` uses `data-model` by default.
Use `whiteboard list --json` to inspect effective roles and
`whiteboard assign-role --id ID --role erd --expect ID_OR_none` to assign an
existing board. Role assignment preserves its ID, content, history, and named
URLs. The sidebar contains ERD, Infrastructure, and Data flows; other boards
remain discoverable from the Technical Design overview. See
[authoring operations](authoring-operations.md) for reassignment, rename,
archival, dry-run and concurrency checks.

The [data-model authoring guide](erd-whiteboard-guide.md) gives a worked example
of the shared grammar, revision policy and viewer. Current source adapters resolve
exact ERD and data-entity pins and import ERD frames. Component and system inventory
pins are not supported, regardless of the subject or name of the whiteboard.
Existing feature `___design` fragments are not automatically migrated.

## First application: one data-model document

Use the `data-model` document for **one cohesive explanation of the data model**,
expressed on a large, zoomable whiteboard.
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

Publishing creates a complete immutable revision of the selected whiteboard document.
If an imported frame changes, adopting a different child revision is an explicit
edit and publication; historical boards never silently follow new source heads.
Older ERDs and revisions remain readable for compatibility. No holder semantics
or inventory kinds are changed.

## Inspection and viewer behavior

`whiteboard describe --id data-model` provides meaningful structured inspection of the
whole model, a section, entity, connection or imported region, including source references:

```sh
change-saga whiteboard describe --id data-model change.saga
change-saga whiteboard describe --id data-model --focus cloud/identity change.saga
change-saga whiteboard describe --id data-model --focus cloud/identity/organization change.saga
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
Those are the product tests for the first application. Infrastructure and system
design should use the same principle: one cohesive document for each design,
with space to explain its boundaries, relationships and evidence.
