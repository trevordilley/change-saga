# Diagram source grammar

The reference for the elements a slide or whiteboard diagram is written in, and
for revising a published one. Read [slides](slides.md) for how to publish a slide
and what it should explain, or [whiteboards](whiteboards.md) for a named board.

## Author diagrams as source

A diagram source is an ordered list of explicitly positioned elements; the CLI
renders it to the slide's SVG. Pass it as `"diagram"` instead of `"asset"` in
an `apply-slide` request, for an implementation or a review deck alike. `change-saga spec --json --section implementation_deck` publishes its vocabulary
under `implementation_deck.diagram_source`; list icons with `change-saga
diagram icons`.

- Kinds: `node` (shape `service`, `datastore`, `decision`, `rect`, `ellipse`,
  `boundary`, `triangle`, `hexagon`, `parallelogram`, `document`, `cloud`,
  `actor`, `queue`, `circle`, `star`, or `entity`), `edge` (`from`/`to` nodes
  plus explicit `points` or `path`), `text`, `group`, `graphic`
  (allowlisted SVG drawing markup), `sticky`, `annotation` (below), and `code`
  (literal API or library usage examples).
- Use a `code` element when the reviewer needs to see how an API or library is
  invoked. Supply literal `code`, an optional `language` display label,
  `line_numbers`, and `highlight_lines` (unique 1-based line numbers). Keep the
  example focused and place it in explicit `x`, `y`, `width`, and `height` bounds.
  The default `code` style is 18px Go Mono. Indentation and blank lines are
  preserved, tabs use four-column stops, and overflow is refused; source never
  wraps, shrinks, or executes. The bounds are 16,000 characters and 100 lines.
  Select the element with an `example` Item and attach real exact evidence to
  that Item; a displayed snippet is not itself code coverage.

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
- An ordinary `edge` must name `node` IDs in `from` and `to`; it cannot end on
  a `group`, including a section. Use real endpoint nodes inside the boundary.
  Composed `connection` elements instead use instance paths to non-decorative
  elements with explicit bounds; do not confuse the two endpoint contracts.
- Edge labels need a `label_box`; a node whose shape is too small for its
  label may place it in a `label_box` outside the shape. `align` is `start`,
  `middle`, or `end`.
- Text that does not fit its box is refused, never shrunk: widen the box,
  wrap, or shorten the text.
- Name styles; define custom ones under `styles`. Graphics use `currentColor`
  for their style's stroke instead of hard-coded colors.
- A custom style is complete, not a partial override: supply `fill`, `stroke`,
  `ink`, `font_size` (8–200) and `stroke_width` (0–50). `dash` is an optional
  boolean, default false, not a dash-pattern string. For example:

  ```json
  "styles": {
    "browser": {
      "fill": "@diagram-blue-tint", "stroke": "@diagram-blue-accent",
      "ink": "@diagram-blue-ink", "font_size": 20, "stroke_width": 2,
      "dash": false
    }
  }
  ```
- Prefer colour tokens over hex in custom styles: `"fill": "@diagram-primary-fill"`
  or `"stroke": "@accent"` follows light and dark mode and a theme, while a
  hex colour stays fixed in both. `change-saga spec --json --section theme` lists the tokens under
  `theme`, and `--section implementation_deck` under `diagram_source.color_tokens`.
- Text uses the bundled Go font, not arbitrary system glyphs. ASCII labels are
  a safe starting point; the font also supports some non-ASCII text, but not all
  Unicode. In particular, `✓` (U+2713) and `▷` (U+25B7) are rejected. Use words
  or a bundled Lucide `icon` discovered with `diagram icons`; use an edge
  terminator for an arrowhead. The missing-glyph error names the character and
  code point. There is currently no CLI character-set listing; validate chosen
  symbols in the scratch publication before using them throughout the board.
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
- Any semantic element may carry an optional `note`: useful detail beyond the
  visible label and description. Readers hover, focus, or tap its small info
  control at the element's bottom-left. The control fades in on node hover or
  keyboard focus; touch readers see it directly. Node hover reveals the icon
  without opening detail. `diagram describe` also prints the note. Explain responsibility,
  protocol, constraints, failure behavior, rationale, or an example the visible
  text does not already explain. Do not repeat or paraphrase the cell's text,
  and do not add a note to every element just for consistency. Omit it when
  there is no additional grounded information. Keep the explanation needed to
  understand the system visible on the detailed slide.
  Keep notes to a few sentences (at most 1,000 characters) of Markdown limited
  to bold, italics, inline code, lists, and http, https, or mailto links; raw
  HTML, headings, images, tables, and code blocks are refused. Surprises remain
  callout Items with their own always-visible warning controls and exact evidence.
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
