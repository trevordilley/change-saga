# Decks, Essays, and narrative

Read this reference to create a deck or change its structure, publish its Essay,
or author chapters, sections, fragments and landmarks. Read [query.md](query.md)
first when a Saga already exists. Individual slides are in [slides](slides.md).

## Choose the artifact

Use long-form essays for Product overviews, features and user stories, and slide
decks for implementation and code reviews.

Every deck has a Deck -> Slide -> Item spine. A feature's single living
implementation deck explains the domain's current implementation. Update its
affected slides rather than creating one deck per change. A pull-request review
deck explains a transition and its reasoning. An onboarding deck teaches the
app without owning code. Requirements, prototypes, and design are not slides. Requests to describe
implemented features belong in actual Product story records (see
[stories.md](stories.md)); deck Items may reference those stories.

Every deck opens with an Essay: an authored report combining explanation, tables,
and visualizations. A Technical deck's overview is the detailed implementation
account that its slides make digestible through diagrams and explanation. Explain responsibilities, data models, execution
paths, invariants, failure handling, and tradeoffs with enough depth to work on
the implementation.
A slide tour or reading guide does not replace this technical account.
Every slide has a short bullet-summary Front and a visual
Back. Keep precise evidence on stable Back Items. Overview citations name a
slide and optionally an Item, so code, diffs, and stories are reached through
that slide. Reference every slide from the authored Overview. That coverage
check is separate from exact code coverage: a citation never covers an
otherwise unexplained code line. A generated directory is not authored coverage.
Use the installed CLI help and spec for the overview and Front authoring
commands and field limits. Publish a deck report with `change-saga deck overview
--deck DECK --file overview.json change.saga`, or `--review ID` for a review;
use `deck overview --deck DECK --check change.saga` to require authored coverage.
The JSON body uses `[label](annotation:ID)` citations and
`![diagram](slide:SLIDE_ID)` to reuse a same-deck visual. Each annotation is
`{"id":"ID","label":"label","slide":"SLIDE_ID","item":"ITEM_ID"}`; `item`
is optional. Only citations actually present in rendered report content earn
overview coverage, never unused definitions or example text in a code block.
Supply `slide.front` as a list of summary bullets in complete-slide requests.

Chapters, sections, and fragments carry longer design notes, overviews, and
walkthroughs. Use Markdown to orient and connect visual artifacts, not as the
default container. A substantial chapter should lead with a diagram,
interactive walkthrough, or concrete before/after example and state its
boundary, invariants, decisions, risks, compatibility concerns, and proof.

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
