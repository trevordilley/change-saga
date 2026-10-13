# Product themes

Product Themes are optional categories of Features, distinct from visual
styling (`theme`). Membership is many-to-many and lives on the theme; never
relocate or duplicate Features or stories to group them.

Default to creating or revising the requested stories. Offer theme creation as
an optional follow-up when related features would benefit from a shared
explanation; do not automatically create themes or rationale decks as part of
story authoring. Proceed with themes when the user requests them or accepts
the offer, including direction already given in the conversation. A pending
or declined offer must not block stories. Reuse existing themes where relevant.

Discover `query feature-themes --saga PATH` (paginated summaries) and select
`--theme ID|URN` to read Markdown overview and membership. Use `query schema
feature-themes` and `spec --json` for the current contract.

Author with `feature-theme add --id ID --title TEXT --source FILE --feature
ID... PATH`, `feature-theme set-overview --theme ID --source FILE PATH`, and
`feature-theme set-features --theme ID --feature ID... PATH`. On set-features,
omitting every --feature clears the membership set. Exact retries are idempotent.

Create the normal rationale deck with `add-deck --feature-theme ID --id DECK
--objective TEXT PATH NAME`; use the existing add-slide/add-item or apply-slide,
diagram, and deck-overview workflow. Rationale Items may optionally reference an
existing persona, feature, or story using `record` and may cite validated code.
They need no code minimum and cannot establish implementation criterion coverage.

The viewer's Product themes pages show the authored overview and rationale deck.
With four or more features, the menu beside Features offers None, Theme (when
themes exist), and Persona. Theme is the default grouping when available; this
display preference never authorizes creating themes. Smaller sets stay flat.
Persona membership derives from current story personas including proposed
stories. Keep ungrouped features visible and preserve their canonical links.
