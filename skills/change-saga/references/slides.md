# Authoring a slide

Read this reference to create or change a slide of a feature implementation,
pull-request review or Plan deck, and to run its checks. Read [query.md](query.md)
first when a Saga already exists. Use `change-saga --help`, command `-h`, and
`change-saga spec --json --command NAME` for current syntax.

Before storyboarding, read [visual storytelling](visual-storytelling.md) for
the first-draft standard: named ownership sections, meaningful color, visible
mechanisms and consequences, concrete examples, and a connected deck narrative.
Its worked compositions apply to slides and whiteboards; the API example below
shows publication mechanics, not the target depth of a finished review.

The `"diagram"` a slide publishes is written in the
[diagram source grammar](diagram-source.md). Attach code by the rules in
[evidence](evidence.md). Deck structure and its Essay are in [decks](decks.md).

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
[diagram source](diagram-source.md); the CLI renders it to SVG. Hand-author SVG only when a
composition needs what the source cannot express, and use self-contained HTML
when switching paths, stepping through states, changing an input, or comparing
behavior materially helps. Load no network dependencies, and make the default
state understandable without interaction.

## Make the depth readable

These principles apply to the in-depth Back of every deck: implementation,
review, and onboarding. Keep the Front simple and high-level, like a flash
card: a few short bullets with the main idea and takeaway.

- Make group membership explicit. Use the slide API's groups, regions, and
  sections to separate responsibilities, system boundaries, or segments of a
  flow. In diagram source, use `group` elements with `section`, `boundary`, or
  `rect` shapes and nest related elements inside them. Combine labeled frames,
  spacing, and consistent color so readers can see what belongs together and
  what belongs in a different group. Color should reinforce labeled structure.
- Keep visual meaning consistent across the deck. Give recurring domains the
  same colors and styles on each slide; use a compact labeled key when helpful.
  Distinguish systems with named boundaries, suitable shapes and icons, and
  clear heading hierarchy. Readers should recognize the areas and their roles
  at a glance. Use the audience's product terms consistently in labels.
- Make flows explicit. Label arrows with what moves or happens, and show where
  they cross system boundaries. Separate materially different paths, such as
  success and failure or alternate branches of a decision. Show an intentional
  absence when it matters to the explanation, such as "no further action",
  so readers can distinguish a completed path from an omitted one.
- Put useful text on the slide. Sentences, short paragraphs, and descriptions
  beside the elements they explain help readers understand responsibilities,
  behavior, reasons, and consequences. Text arranged within a slide's visual
  structure is easier to absorb than a large undifferentiated document block.
  Keep the explanation needed to understand the slide visible; use notes behind info buttons
  for supplemental depth.
- Aim for clarity and understanding of the system. Give readers enough detail
  to explain how the parts interact, what happens along the important paths,
  and why the design works this way. Organize detail with headings, sections,
  and readable text; split a crowded slide into connected slides when needed.
  Preserve the explanation when simplifying the layout.

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

Use `slide.section` in an `apply-slide` request (or `add-slide --review ID
--section "Request flow"`) to label divisions in a review outline, just as in a
feature implementation deck. Consecutive slides with the same label share one
divider; a different label starts a new division. An empty label ends the run,
and repeating an earlier label later starts a fresh division in place. Sections
do not reorder slides or introduce extra slides; slide and Item links, review
decisions, and the deck Essay keep their existing destinations.

Author review slides the same way: set `"review": ID` instead of `"deck"` (or
pass `apply-slide --review ID`). A review Item carries no `evidence` or
`criterion_links`, and may set `record` to a Saga record to open beside the
change. Call out each surprise (see "Reviewer surprises" below) with a
`"kind": "callout"` Item whose `about` names the responsible element's Item
and whose `body` gives expectation, actual behavior, reason, and consequence;
reviewers open callout details from compact warning controls on the element, and
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
       "note": "A miss reads the row and **writes it back** with a 60-second TTL; a database error reaches the caller and is not cached."}
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
later revisions and diagram edits. A callout's own evidence must include the
code that decides the behavior it states, where the outcome is determined,
not just code nearby: "a receiver given a release without X fails promotion"
cites the release validation that rejects such a release first, if one does,
even when another Item on the slide covers it. `review list --review ID`
marks a surprise with no code evidence.

Edge descriptions and notes make claims too, and carry no evidence of their
own. State only what the connected elements' Items show; a claim beyond them
("only a validated runtime reaches a receiver" when validation checks the
whole package) belongs on an Item that covers the code, or should be cut. A revision that drops a covered Item is
refused until its coverage is removed. Read review slides with `diagram
describe` or `query slide`, and check them with `diagram check` and
`visual-qa --review ID`.

## Compose semantic, reviewable visuals

One slide carries one intent, one takeaway of at most 180 characters, and one
bounded 16:9 composition. Aim for 1-7 primary semantic Items on a standard slide;
this is a focus guideline, not a reason to strip away explanatory structure.
Every non-decorative node, edge, region, transition, statement, risk, metric,
example, and callout belongs in `reading_order`, with a label, a semantic
description that stands without the picture, and an element or normalized
region selector.

Only Items own deck code evidence. Deck- and slide-level coverage is refused.
A callout may name another Item with `--about` and may own exact evidence for
its claim. Keep callout bodies at most 240 characters. Short explanatory
paragraphs belong beside the mechanisms they explain. If several competing
ideas or too many primary Items make the slide hard to read, split at a reviewer
question or choose a better composition; do not replace explanation with labels
or move the essential story into hover notes.

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
and give the actual behavior and consequence exact evidence from the code that
decides them; an element
`note` supplies optional depth; a surprise uses a callout Item so its warning
control remains visibly distinct. Ground the
contrast in source material or a plausible reviewer mental model. Do not
manufacture novelty; when none exists, teach system shape, risk boundaries,
and verification instead.

### Four visual audits

1. **Silhouette:** without text or color, topology still communicates
   containment, flow, sequence, state, entity structure, branching, or
   comparison.
2. **Relationship:** every relationship essential to the takeaway is encoded
   as an edge, boundary, lane, nesting, cardinality, axis, or transition.
3. **Understanding:** the visible diagram and text let a reader explain the
   system's groups, responsibilities, and interactions. When there is a
   surprise, they can name the highest-consequence deviation, its reason,
   and its tradeoff.
4. **Contact sheet:** recurring domains retain their colors, labels, and styles;
   repeated visual grammar represents the same underlying relationship.
   Unrelated slides have not collapsed into identical cards.

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

## Handoff checks

When the user needs tests for a review, run `review test-plan --review ID
--json <saga>`. Every test case should verify a story's acceptance criteria;
automated cases also need exact `test_implementation` evidence naming their
test code. Those declared links let a change reach a story and then its tests,
even when the test file itself did not change. Missing links, unresolved code
and manual-only obligations remain gaps. Record a run command with actual run
results; never invent a historical run to fill a missing command.

Check every sentence before handing off a deck; no command reads the prose,
so `check`, `validate`, `diagram check`, and `visual-qa` all pass a deck
whose sentences are wrong. Give a fresh-context reader, ideally a sub-agent
that did not write the deck, each slide's `diagram describe`. It reads every
takeaway, Item description, callout body, edge description, and note against
the code that Item lists (`path:start-end` under each Item) and the code the
claim depends on: what runs before or after it, validation, and error paths.
It flags each sentence that is false, overstated ("each artifact" when one is
connected), or unverifiable from the code, and the author revises it with
`diagram edit` or `apply-slide`.

Read the deck in order without relying on author knowledge. Confirm its visual
forms and four audits, exact evidence, useful collapsed-file notes, current
code agreement, accessible semantics, and explicit uncertainty. Query until
no requested changed line is uncovered, no reference is stale, and each
overlap is defensible. Verify claims independently; complete coverage is not
verification. Remove scaffold content, run validation, and use the relevant
bounded queries before handoff.
