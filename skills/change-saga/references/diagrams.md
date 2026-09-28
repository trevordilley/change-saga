# Diagrams, narrative, and exact evidence

Read this reference when the task creates or changes a feature implementation
deck, pull-request review deck, narrative fragment, landmark, code evidence,
claim, or verification. Read [query.md](query.md) first when a Saga already
exists. Use `change-saga --help`, command `-h`, and `change-saga spec --json`
for current syntax.

## Choose the artifact

Every deck has a Deck -> Slide -> Item spine. A feature's single living
implementation deck explains the domain's current implementation. Update its
affected slides rather than creating one deck per change. A pull-request review
deck explains a transition and its reasoning. An onboarding deck teaches the
app without owning code. Requirements, prototypes, and design are not slides.

Chapters, sections, and fragments carry longer design notes, overviews, and
walkthroughs. Use Markdown to orient and connect visual artifacts, not as the
default container. A substantial chapter should lead with a diagram,
interactive walkthrough, or concrete before/after example and state its
boundary, invariants, decisions, risks, compatibility concerns, and proof.

## Storyboard before drawing

For each proposed slide, identify privately:

- the specific reviewer question, intent, and one-sentence takeaway;
- the minimum system model or meaningful surprise it explains;
- the relationship the picture must make visible;
- the visual form that truthfully encodes that relationship; and
- the nodes, edges, states, regions, transitions, examples, or callouts that
  will become evidence-bearing Items.

Choose the form from the relationship:

- system context for actors, external systems, and trust or ownership
  boundaries;
- architecture/composition for containment, dependencies, and responsibility;
- data flow for direction, transformations, storage, consumers, and retry or
  failure paths;
- sequence for participants, time, calls, responses, and exceptional returns;
- state machine for states, labeled events, guards, and terminal states;
- entity-relationship for entities, keys, ownership, and cardinality;
- decision flow for predicates, branches, joins, loops, and outcomes;
- matched before/after axes for a meaningful delta;
- failure path for trigger, propagation, containment, cleanup, recovery, and
  observable outcome; and
- evidence view for a concrete claim or risk and its tests or measurements.

`--intent` names the slide's rhetorical job and `--layout` its canvas
arrangement; neither supplies the diagram's meaning. A row of cards is valid
only when membership or matched comparison is the relationship. Boxes joined
only by reading order are an outline, not a diagram.

Author stable architecture, boundaries, data models, flows, and sequences as a
diagram source (below); the CLI renders it to SVG. Hand-author SVG only when a
composition needs what the source cannot express, and use self-contained HTML
when switching paths, stepping through states, changing an input, or comparing
behavior materially helps. Load no network dependencies, and make the default
state understandable without interaction.

## Author diagrams as source

A diagram source is an ordered list of explicitly positioned elements; the CLI
renders it to the slide's SVG. Pass it as `"diagram"` instead of `"asset"` in
an `apply-slide` request, for an implementation or a review deck alike. `change-saga spec --json` publishes its vocabulary
under `implementation_deck.diagram_source`; list icons with `change-saga
diagram icons`.

- Kinds: `node` (shape `service`, `datastore`, `decision`, `rect`, `ellipse`,
  `boundary`, `triangle`, `hexagon`, `parallelogram`, `document`, `cloud`,
  `actor`, `queue`, `circle`, `star`, or `entity`), `edge` (`from`/`to` nodes
  plus explicit `points` or `path`), `text`, `group`, `graphic`
  (allowlisted SVG drawing markup), `sticky`, and `annotation` (below).
- Pick a shape for what a node is: `actor` for a person or external role,
  `queue` for a queue or stream, `document` for a file or report, `cloud` for
  an external or hosted system, `datastore` for storage. The shapes from
  `triangle` on center their icon, label, and detail inside the outline and
  default to `middle` alignment; a `circle` needs equal width and height. The
  description omits purely geometric shapes, so a label must say what the
  node is.
- Draw an ERD with `entity` nodes: the label names the table and `fields`
  lists its rows. An edge's `from_field`/`to_field` names the row it ends on;
  aim its points at that row's middle, `header + 28 * index + 14` below the
  entity's top, where `header` is the style's `font_size * 1.25 + 16` (43.5
  for `normal`):

  ```json
  {"id": "orders", "kind": "node", "shape": "entity", "label": "orders",
   "x": 600, "y": 480, "width": 320, "height": 128, "style": "normal",
   "fields": [{"name": "id", "type": "uuid", "key": "pk"},
              {"name": "customer_id", "type": "uuid", "key": "fk"},
              {"name": "total", "type": "numeric"}]},
  {"id": "places", "kind": "edge", "from": "orders", "to": "customers",
   "from_field": "customer_id", "to_field": "id", "style": "secondary",
   "tail": "zero-or-many", "head": "only-one",
   "points": [{"x": 600, "y": 565.5}, {"x": 360, "y": 537.5}]}
  ```

  `diagram describe` reads it as `fields: id uuid pk, customer_id uuid fk,
  total numeric` and `places: orders.customer_id -> customers.id
  tail=zero-or-many head=only-one`.
- `icon` names any bundled Lucide icon; search by what it depicts with
  `change-saga diagram icons --query storage`.
- Every coordinate is explicit and local to the parent group. Nothing is laid
  out, resized, or rerouted for you: moving a node leaves its edges where they
  are, so update their `points` in the same batch.
- Element order is reading order. Put participants, then messages in time
  order, so the description reads in sequence.
- A group with `shape` `rect` or `boundary` draws a frame and parents its
  contents; use one for containers, lanes, trust boundaries, and `alt`/`loop`
  fragments, so moving the frame moves what it contains.
- Edges end in terminators: `head` and `tail` take `arrow`, `open`, `triangle`,
  `diamond`, `filled-diamond`, `circle`, `dot`, `bar`, or an ERD cardinality
  (`one`, `only-one`, `zero-or-one`, `many`, `one-or-many`, `zero-or-many`).
  For an ERD, draw `orders -> customers` with `"tail": "zero-or-many", "head":
  "only-one"` and put `N` and `1` in `tail_label`/`head_label` with their own
  boxes. Use `curve: "smooth"` for a flowing line through its points, `line:
  "dashed"` or `"dotted"` for optional or asynchronous flows, and a style with
  a thick `stroke_width` for an emphatic arrow; its head grows with it.
- Edge labels need a `label_box`; a node whose shape is too small for its
  label may place it in a `label_box` outside the shape. `align` is `start`,
  `middle`, or `end`.
- Text that does not fit its box is refused, never shrunk: widen the box,
  wrap, or shorten the text.
- Name styles; define custom ones under `styles`. Graphics use `currentColor`
  for their style's stroke instead of hard-coded colors.
- Prefer colour tokens over hex in custom styles: `"fill": "@diagram-primary-fill"`
  or `"stroke": "@accent"` follows light and dark mode and a theme, while a
  hex colour stays fixed in both. `change-saga spec` lists the tokens under
  `theme` (and `diagram.color_tokens`).
- Mark lifelines and chrome `decorative`, and a title unless it carries a
  `note` (a decorative element is hidden from readers, so it cannot). Everything else is semantic,
  appears in the description, and needs a `description` when its label does
  not stand alone. A semantic element cannot sit in a decorative group.
- Give every Item an `element` selector naming a semantic element ID.
- Board elements annotate a drawing. A group with shape `section` is a tinted
  frame with its label in a title tab; nest sections for sub-areas. A
  `sticky` is a coloured square of wrapped text. An `annotation` is a
  `bubble` (its pointer reaches `target`, an element-local point, or else the
  box of the element it is `about`), a numbered `pin` (`width` is its
  diameter), a translucent `highlight`, or a `bracket` whose point faces
  `side`; a highlight or bracket label needs a `label_box`. Set `about` to the
  element a sticky or annotation explains, so `diagram describe` lists it as a
  note about that element. `color` picks from yellow, pink, blue, green,
  purple, and gray. A sticky or bubble is visible commentary; a surprise still
  belongs in a callout Item.

  ```json
  {"id": "ingest", "kind": "group", "shape": "section", "label": "Ingest", "color": "blue",
   "x": 60, "y": 100, "width": 560, "height": 360, "style": "normal"},
  {"id": "why", "kind": "sticky", "label": "Writes are keyed by batch, so a retry overwrites itself.",
   "about": "store", "x": 840, "y": 420, "width": 200, "height": 200, "style": "normal"},
  {"id": "hot", "kind": "annotation", "shape": "bubble", "label": "Hot path", "about": "store",
   "x": 1080, "y": 300, "width": 180, "height": 90, "style": "normal"},
  {"id": "first", "kind": "annotation", "shape": "pin", "label": "1", "about": "queue",
   "x": 272, "y": 152, "width": 32, "style": "normal"}
  ```
- Any semantic element may carry an optional `note`: depth on demand that a
  reader sees, rendered, when they hover, focus, or tap the element, and that
  `diagram describe` prints. Use it for the why its label cannot hold: an
  edge's protocol or failure behavior, a node's responsibility, the context of
  a title. An Item's element already shows the Item's label, description, and
  callout body on hover, so a note adds to them rather than repeating them.
  Keep notes to a few sentences (at most 1,000 characters) of Markdown limited
  to bold, italics, inline code, lists, and http, https, or mailto links; raw
  HTML, headings, images, tables, and code blocks are refused. Notes are
  optional detail: a surprise still belongs in a callout Item, which is
  prominent. Never manufacture detail to fill a note; omit it instead.
- A diagram may unfold in reading order with `"reveal": "fade"`: the
  reviewer fades its elements in, step by step, each time the slide is shown.
  Use it when order carries meaning, such as a request's path or a
  before-and-after, not as decoration. Elements enter in reading order
  unless they set an integer `step`; equal steps appear together, and no
  element appears before its group. Decorative chrome is present from the
  start. Thumbnails, exports, landmark links, and readers who prefer reduced
  motion see the finished drawing, so it must read completely without the
  animation. Turn it on with `{"op": "canvas", "set": {"reveal": "fade"}}` and
  let an edge enter with its target node:

  ```json
  {"id": "reply", "kind": "edge", "from": "api", "to": "client", "points": [{"x": 900, "y": 360}, {"x": 300, "y": 360}], "head": "arrow", "style": "secondary", "step": 3}
  ```

  `diagram describe` prints `Reveal: fade in N steps` and each element's
  `step=N`; check that order reads as intended.

Revise a published diagram with `change-saga diagram edit --slide TARGET
--expected SNAPSHOT --request-id ID --from OPS.json <saga>`. Operations are
`add` (optionally `before` an ID), `update` (`set` fields; `null` removes one,
so `{"op": "update", "id": "miss", "set": {"note": null}}` clears a note),
`move` (`dx`, `dy`), `remove` (`cascade` for dependents), `style`, `align`,
`distribute`, and `canvas`. The edit republishes through `apply-slide` with
the slide's Items, evidence, and criterion links unchanged, so stale evidence
or criteria still refuse it. Name a review slide by its URN or with
`--review ID`. Use `change-saga diagram get` for one element's
exact properties before editing it, and `change-saga diagram describe` to read
the slide; do not read or edit the stored source or SVG. Validation reports
every problem in a batch at once; fix them all before retrying. Run
`change-saga diagram check` before handoff to confirm every published SVG still
matches its source.

## Complete-slide publication

Discover `apply-slide` in the installed CLI before use; otherwise use the existing
focused slide, Item, relation, and evidence commands without inventing the
transaction.

When available, `apply-slide --from FILE|- [--review ID] [--repo PATH]
[--dry-run] [--json] <saga>` accepts one versioned, complete desired slide of
an implementation or review deck: the visual asset or diagram source, slide
metadata, and ordered semantic Items and selectors; an implementation slide's
Items also carry exact code evidence and pinned Item-level criterion links.
Create requires `expected_snapshot: "absent"`; update requires the exact snapshot from the preceding result or
`query slide`. Inspect authoring heads and preserve every conflicting head.
For divergent history, use `operation: "reconcile"` with `expected_snapshots`
containing every reported head, omitting the singular `expected_snapshot`.
Preview with `--dry-run` before publishing. Preserve the stable `request_id`:
an identical retry is a no-op and reuse with different content is rejected.

A request supplies exactly one of `asset` or `diagram`; with a diagram,
`media_type` is `image/svg+xml` and may be omitted.

Each Item needs a resolving selector. An implementation Item also needs
focused line-range evidence whose digest matches the named commit and an exact
criterion link with its current story revision and rationale; a review Item
carries neither (see below). The transaction refuses whole-file evidence, broad
Deck/Slide criterion ownership, stale criteria, unsafe asset paths, and
selector-breaking replacements. Its atomic boundary is one slide; it does not
atomically include story edits, another slide, a Git commit, or external work.
Once a slide is transaction-managed, update its complete desired state through
`apply-slide`; older partial slide, Item, and evidence mutations refuse it.
`apply-slide --print-current SLIDE <saga>` prints the slide's complete current
request with `expected_snapshot` and a fresh `request_id`: edit the field that
changes and apply it, never rebuild the slide by hand (applied unchanged, it
publishes nothing; give the slide's target when its ID is shared by another
deck's slide). Stale evidence moves
with `repin --accept-proposed` without a request at all.
After a post-publication durability error, query the current snapshot before
retrying; the published record and its referenced asset must remain intact.

### Review deck slides

Author review slides the same way: set `"review": ID` instead of `"deck"` (or
pass `apply-slide --review ID`). A review Item carries no `evidence` or
`criterion_links`, and may set `record` to a Saga record to open beside the
change. Call out each surprise (see "Reviewer surprises" below) with a
`"kind": "callout"` Item whose `about` names the responsible element's Item
and whose `body` gives expectation, actual behavior, reason, and consequence;
reviewers see callouts in a Surprises panel on the slide, and
`review list` names them. A minimal review slide with a diagram source and
one surprise:

```json
{
  "version": 1, "operation": "create", "request_id": "pr-42-architecture",
  "review": "pr-42", "expected_snapshot": "absent",
  "slide": {
    "id": "architecture", "title": "Reads go through the cache", "rank": 10,
    "intent": "explain", "layout": "diagram",
    "takeaway": "Reads hit the cache first and fall through to the database on a miss.",
    "reading_order": ["cache", "database", "stale-reads"]
  },
  "diagram": {
    "version": 1, "width": 1280, "height": 720,
    "elements": [
      {"id": "title", "kind": "text", "label": "Reads go through the cache", "x": 60, "y": 28,
       "width": 800, "height": 48, "style": "title", "decorative": true},
      {"id": "cache", "kind": "node", "shape": "service", "label": "Read-through cache",
       "x": 120, "y": 240, "width": 300, "height": 110, "style": "primary"},
      {"id": "database", "kind": "node", "shape": "datastore", "label": "Database",
       "x": 760, "y": 240, "width": 280, "height": 120, "style": "normal"},
      {"id": "miss", "kind": "edge", "from": "cache", "to": "database", "head": "arrow",
       "points": [{"x": 420, "y": 295}, {"x": 760, "y": 295}], "label": "on a miss",
       "label_box": {"x": 520, "y": 255, "width": 140, "height": 28}, "style": "secondary",
       "note": "A miss reads the row and **writes it back** with a 5-minute TTL; a database error reaches the caller and is not cached."}
    ]
  },
  "items": [
    {"id": "cache", "rank": 10, "kind": "node", "label": "Read-through cache",
     "description": "Why reads now go through the cache.",
     "selector": {"type": "element", "element_id": "cache"}},
    {"id": "database", "rank": 20, "kind": "node", "label": "Database",
     "description": "Reached only on a cache miss.",
     "selector": {"type": "element", "element_id": "database"}},
    {"id": "stale-reads", "rank": 30, "kind": "callout", "label": "Reads can be stale",
     "about": "cache",
     "body": "You would expect writes to invalidate the cache; entries expire after 60s instead, so a read can lag a write by up to a minute. Invalidation would couple every writer to the cache.",
     "description": "Why the cache expires entries instead of invalidating them.",
     "selector": {"type": "element", "element_id": "cache"}}
  ]
}
```

After publishing, cover each Item's code with `change-saga cover --target
ITEM-URN --path PATH --changed-lines` (cover a surprise's actual behavior,
such as the expiry setting, from its callout); that coverage stays attached across
later revisions and diagram edits. A revision that drops a covered Item is
refused until its coverage is removed. Read review slides with `diagram
describe` or `query slide`, and check them with `diagram check` and
`visual-qa --review ID`.

## Compose semantic, reviewable visuals

One slide carries one intent, one takeaway of at most 180 characters, and one
bounded 16:9 composition. Use 1-7 primary semantic Items on a standard slide.
Every non-decorative node, edge, region, transition, statement, risk, metric,
example, and callout belongs in `reading_order`, with a label, a semantic
description that stands without the picture, and an element or normalized
region selector.

Only Items own deck code evidence. Deck- and slide-level coverage is refused.
A callout may name another Item with `--about` and may own exact evidence for
its claim. Keep callout bodies at most 240 characters. If the slide needs a
paragraph or more than seven primary Items, split the argument or choose a
better composition; do not hide prose in SVG or HTML.

Show a concrete input, important intermediate state, output or side effect,
and a consequential edge or failure path where relevant. At 1280x720 and
1024x576, verify clipping, overlap, legibility, focus geometry, scan order,
and a useful narrow linear reading. A custom layout needs a rationale and does
not waive accessibility or evidence requirements.

### Reviewer surprises

After establishing the smallest useful system model, foreground behavior that
breaks a reasonable maintainer expectation: counterintuitive public behavior,
intentional convention changes, hidden coupling, displaced costs, ordering or
ownership constraints, tradeoffs, rejected alternatives, and important
failure or recovery behavior.

For each material surprise, show expectation, actual behavior, rationale, and
consequence together. Attach a callout Item to the responsible visual element
and give the actual behavior and consequence exact evidence; an element
`note` is optional detail a reader has to seek out, never the place for a
surprise. Ground the
contrast in source material or a plausible reviewer mental model. Do not
manufacture novelty; when none exists, teach system shape, risk boundaries,
and verification instead.

### Four visual audits

1. **Silhouette:** without text or color, topology still communicates
   containment, flow, sequence, state, entity structure, branching, or
   comparison.
2. **Relationship:** every relationship essential to the takeaway is encoded
   as an edge, boundary, lane, nesting, cardinality, axis, or transition.
3. **Surprise:** a reviewer can name the system model, highest-consequence
   deviation, its reason, and its tradeoff.
4. **Contact sheet:** repeated visual grammar represents the same underlying
   relationship; unrelated slides have not collapsed into identical cards.

Run these before chasing coverage. Coverage cannot rescue a generic visual.

### Mechanical visual QA

Run `change-saga visual-qa` after authoring or revising slides. It renders raw
assets and the real reviewer at 1280x720 and 1024x576, then emits a contact
sheet and `visual-qa.json`. Use selectors to narrow to the requested feature,
deck, or slide. Keep output outside the Saga in either a new directory or an
existing directory bearing the visual-QA managed marker.

Treat missing/empty selectors, clipping, text overflow, and reviewer-surface
failures as mechanical defects. Overlap is a warning because it can be
intentional. The report explicitly leaves semantic arrow direction
`not_evaluated`; inspect relationship meaning yourself. A successful render
does not prove the diagram, evidence, or product claim is correct.

## Narrative fragments and landmarks

Create chapters, sections, and fragments with their public commands. Install
or replace a fragment entrypoint only with `set-fragment-content`; revise or
remove the owning record with the matching command. Do not edit fragment
package metadata directly.

Create landmarks for independently discussable headings, concepts, states,
controls, nodes, and edges, not decorative shapes or every sentence. Preserve
stable lowercase heading anchors. Give every meaningful visual landmark a
description that works without geometry, color, or position. SVG element
bounds become links automatically; use normalized hotspots or image regions
only where needed. Evidence attached to a landmark already reaches the owning
fragment's design relation; do not duplicate it at fragment scope.

Every concrete prose claim about implementation, behavior, an invariant, or a
data transition needs a focused footnote citation or deliberately
evidence-bearing heading. Make a footnote definition an exact-text landmark
and attach only substantiating code. Requirement provenance citations do not
replace implementation evidence.

## Evidence discipline

Code references are pinned to a commit and digest of the exact bytes. Pure
line movement may remap them; changed bytes make them stale. Repair or remove
stale evidence through its public command while retaining its history.
`references --stale`, `reconcile`, `review list`, and `review refresh-coverage`
show each stale reference's proposed range (its start and end mapped through
the diff, widened over lines inserted inside) with the diff inside it and the
nearest hunk on each side. After reading it, `repin --accept-proposed --record
FILE [--reference N]` (or `--target URN`, `--review ID`) keeps the note and
owner and pins the proposal, also on `apply-slide` slides. A reference with
nothing proposed (deleted file, rewritten range, only braces or blank lines
left) is refused: re-cover it by hand.

- Attach code to the narrowest Item or landmark that actually explains it.
- Each reference note says both what changed and why this target owns it.
- Keep one file and coherent reason per record; split different reasons.
- Preserve deletions, migrations, generated files, snapshots, lockfiles, and
  vendored changes as explicit evidence rather than hiding them in broad
  ranges.
- Use changed-lines coverage only when every changed line in that file belongs
  to the same focused target. `cover --batch -` may plan several focused
  records atomically, and `--dry-run` previews supported mutations. Never widen
  a selector merely because a batch makes broad coverage convenient.
- Treat overlap as intentional only for distinct reviewer journeys. Repair an
  existing record with `replace-coverage`, not a duplicate.
- Use `query mappings --sort scrutiny` after coverage. Never hand-edit a
  record to make stale evidence pass.

A landmark target may be passed back as its returned URN or, where the command
accepts it, as `<fragment>#<landmark-id>`. Use returned targets rather than
deriving storage paths.

An Item may relate to a story or criterion with a pinned, self-scoped relation.
Prefer a criterion when the visual explains one obligation; use the story only
when it genuinely applies to every criterion. A slide's summary is derived
from Item relations. Legacy deck or slide links remain readable but do not
apply automatically to every Item.

## Claims and verification

Use `add-claim` for a falsifiable assertion such as an invariant,
compatibility promise, measured performance result, security property, or
test outcome. Claim evidence is separate from coverage. Append a result with
`verify-claim` only after performing its named method; record a reproducible
command where possible. Use `unverified` when it was not checked. Claims and
results are append-only.

## Handoff checks

Read the deck in order without relying on author knowledge. Confirm its visual
forms and four audits, exact evidence, useful collapsed-file notes, current
code agreement, accessible semantics, and explicit uncertainty. Query until
no requested changed line is uncovered, no reference is stale, and each
overlap is defensible. Verify claims independently; complete coverage is not
verification. Remove scaffold content, run validation, and use the relevant
bounded queries before handoff.

## Reusable Components and Systems

When supported by the current CLI, use `component` and `system` for canonical
technical documentation, distinct from terms. Components identify meaningful
logic/transformation with exact code. Systems pin Components and explain their
directed interactions with scoped evidence. Discover the complete JSON
contract through `spec --json` and command help. Author definitions with
`add|revise --from`; lifecycle changes use `set-state`. Preserve every parent
head and all code provenance.

An implementation Item may link a canonical definition with `--documentation`
and `--documentation-revision`, or the optional `documentation` field of an
`apply-slide` Item. Keep contextual labels and exact Item evidence: a System
link grants no coverage of that System's code. Read the saved revision before
explicitly repinning; never rewrite old review content or approve a slide as a
side effect. The drawer opens the explanation, its Components and exact code,
then returns to the same implementation slide. Use `query inventory` for
current heads, code and Component pin health, and selected-record history;
`query inventory-uses` for the Items and Systems that declare a definition; and
`query inventory-coverage` for code the inventory accounts for. `reconcile`
reports stale definition evidence, Item pins that are not current, and
definitions without implementation-deck use. Inventory is not part of generic
`references`/`repin`, feature audits or comparison layers; do not infer those
integrations.

Proposed/implemented intent, data entities, ERDs and exact Item selections are
inventory format 2. Use them only when `___inventory/format.json` exists; never
run `inventory adopt-format` without the user's explicit decision. Under format
2 every new revision states `intent`. Record a design as `proposed` (with
`baseline` naming the implemented revision it changes, or `none`) before code
exists; record delivery with a new `implemented` revision and `--delivery` at
the commit whose code you verified, keeping the proposal in history. Give each
reference a stable evidence `id`. Leave a relationship or interaction
`proposed` until its own evidence exists; nothing is promoted by implication.
To show only the relevant code of a documented entity, add `--selections`
naming the path of saved pins, the evidence ID and the subset range. Pin an
older revision only with `--documentation-view` naming the saved Saga commit
that bound it. Intent and selections are never approval or proof of coverage.
