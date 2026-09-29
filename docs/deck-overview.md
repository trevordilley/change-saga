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
slide. Each slide has two faces:

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

Overview coverage asks whether each slide has a valid authored reference.
Exact code coverage still asks whether the deck's Items explain the relevant
lines. Overview coverage is transitive only when those underlying code links
exist and pass their own checks. Adding a citation cannot cover an unexplained
line. Broken slides, cross-deck links, and missing Items are diagnosed and do
not earn overview coverage.

## Viewed is personal reading progress

The **Viewed** checkbox is an explicit, reversible reading marker. Select a
local reviewer name before marking slides. Navigation, hover, focus, comments,
and approval decisions never mark a slide Viewed. The visible count reports
reading progress separately from decisions and coverage.

Viewed markers are stored in browser local storage, scoped to the Saga, deck,
slide, and local reviewer name. They survive reload and navigation for that
browser profile and origin, including the server port. They do not sync between
browsers, ports, or machines, and clearing site data removes them. The local
reviewer name is a reading profile, not authentication or the Git identity used
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
