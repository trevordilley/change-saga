# Deck Overview, Front, and Back

A feature's **Technical** section explains the current implementation. Its
**Design → Architecture** section describes the technical design. These labels
replace the ambiguous Implementation and Design → Technical labels in reviewer
navigation. Existing `implementation` roles, keys, URLs, and stable slide/Item
links retain their spelling; no directory or identity migration is needed.

## Author the right record

When asked to describe implemented features, query the existing Product
features, personas, and stories, inspect the code, and create or revise actual
story records. Capture user outcomes and observable acceptance criteria there.
Keep inferred intent proposed until confirmed. A feature list in a deck does
not replace Product records. Technical slide Items can reference those real
stories and explain the mechanisms that fulfill them.

## Three ways to read a deck

Every deck has an **Overview**, a report combining written explanation, tables,
and visuals. It provides the connected account of the deck and cites every
slide. For a Technical deck, this is the detailed **technical overview**: the
implementation strategy and technical design that the slides distill into
pictures. Explain component responsibilities, data models, execution paths,
invariants, failure handling, and design tradeoffs at the depth needed to work
on the implementation. The report should stand on its own as a technical
document; a reading guide or slide directory alone is insufficient.
Each slide has two faces:

- **Front** is a short bullet summary, like a flash card.
- **Back** is the detailed diagram or visualization. It retains the existing
  visual asset, stable Items, exact code evidence, and linked stories.

Front and Back are two views of the same stable slide, not new approval or
coverage targets. Existing slides without authored Front bullets derive a
summary from their takeaway and Item labels. Existing decks without an authored
Overview display a generated slide directory, explicitly identified as a
fallback. This keeps old decks readable without claiming authored coverage.

## Authored citations go through slides

An Overview contains a Markdown `body` and an `annotations` array. Each
annotation has an `id`, a `label`, a `slide` reference, and an optional `item`
reference. Cite it in the body with `[reader-facing label](annotation:ID)`.
The slide must belong to the same deck, and the optional Item must belong to
that slide. Stable URNs or local slide IDs keep the association independent of
slide titles, ranks, or filenames.

These authored annotations are distinct from reviewer-drawn comments. They do
not hold their own code references or story records. Hovering or focusing a
citation reveals the resolved slide/Item and its available evidence; its links
open the corresponding code, diff, or story. Following an Item link opens the
Back so the linked visual region remains meaningful.

Overview coverage asks whether each slide has a valid citation in the authored report body.
Unused annotation definitions do not earn coverage.
Exact code coverage still asks whether the deck's Items explain the relevant
lines. Overview coverage is transitive only when those underlying code links
exist and pass their own checks. Adding a citation cannot cover an unexplained
line. Broken slides, cross-deck links, and missing Items are diagnosed and do
not earn overview coverage.

## Author and check

Publish a report from an Overview JSON file:

```sh
change-saga deck overview --deck request-flow --file overview.json --dry-run change.saga
change-saga deck overview --deck request-flow --file overview.json change.saga
change-saga deck overview --deck request-flow --check change.saga
```

For a pull-request deck, use `--review REVIEW_ID` in place of `--deck`.
The explicit Overview check fails when the authored report is missing or any
slide remains uncited. Ordinary validation keeps legacy missing-overview cases
as warnings. Continue running the relevant exact code-coverage check as well.
`change-saga query overview --saga change.saga` exposes deck reports with
resolved slide/Item targets, generated status, covered and uncovered slides,
and validation diagnostics; it does not duplicate evidence.

For example, if the deck contains a `request-path` slide with a `router` Item:

```json
{
  "body": "# Request flow\n\nThe [router](annotation:routing) establishes ownership.\n\n| Stage | Responsibility |\n| --- | --- |\n| Router | Select the handler |\n\n![Request diagram](slide:request-path)",
  "annotations": [
    {"id": "routing", "label": "Routing and its evidence", "slide": "request-path", "item": "router"}
  ]
}
```

The `slide:` image syntax reuses a same-deck slide visual rather than adding a
second asset or evidence record. References to unsupported or missing visuals
are diagnosed. Add citations for every other slide in the deck too. In a
complete slide authoring request, add `"front": ["One useful takeaway", "A second
brief point"]` inside `slide`; the existing diagram or asset remains the Back.
`add-slide` also accepts repeated `--front` bullets. Use a diagram `code` element
for an API or library usage example, with an `example` Item retaining exact
evidence; see [code examples](code-examples.md).

## Viewed is personal reading progress

The **Viewed** checkbox is an explicit, reversible reading marker. A default local reader profile makes it a one-click action; optional profile
switching keeps different local readers separate. Navigation, hover, focus, comments,
and approval decisions never mark a slide Viewed. The visible count reports
reading progress separately from decisions and coverage.

Viewed markers are stored in browser local storage, scoped to the Saga, deck,
slide, and selected local reader identity. They survive reload and navigation for that
browser profile and origin, including the server port. They do not sync between
browsers, ports, or machines, and clearing site data removes them. The local
reader identity or optional name is a reading profile, not authentication or the Git identity used
for attributed review records. Use a consistent server address and port when
resuming. This deliberately avoids adding shared review events for personal
reading progress.

## Compatibility

The optional `front` and `overview` fields preserve existing deck, slide, and
Item identities and evidence records. New readers accept old decks and show
the fallbacks described above. Older tolerant readers continue to show the
existing visual asset but omit the new presentation content; older strict
schema readers may reject the additive fields. Upgrade those readers before
authoring the new fields. No evidence migration or duplicated coverage records
are needed.
