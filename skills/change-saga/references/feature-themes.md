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
For every nonempty collection, the menu beside Features or Roadmap offers None,
Theme (when themes exist), Persona, Label, and Milestone. Theme is the default grouping when available; this
display preference never authorizes creating themes. Each section remembers its
own preference independently.
Persona membership derives from current story personas including proposed
stories. Keep ungrouped features visible and preserve their canonical links.

Themes can group both planned and built features. The Features and Roadmap
sections filter the same membership records by explicit feature planning status;
never duplicate a feature or a theme to move work between them. Roadmap uses the
same grouping controls, including None, and keeps ungrouped work visible.

## Shared labels and milestones

Labels are lightweight shared categories. Themes remain shared entities with an
Essay and rationale decks, so use a theme when the grouping needs an explanation.
Milestones are shared named targets, such as a version or launch, without implied
dates. All three can organize built and planned work without duplicating features.

Discover existing records with `query labels --saga PATH` and `query milestones
--saga PATH`; follow pagination before creating records. Author with `label add
--id ID --title TEXT PATH` or `milestone add --id ID --title TEXT PATH`. Names must
be unique within each kind. Rename with `label rename --label ID|URN --title TEXT
PATH` or `milestone rename --milestone ID|URN --title TEXT PATH`.

Assign with `feature add` or `feature set-plan --feature ID`, using repeatable
`--label ID|URN` and one `--milestone ID|URN`. Set-plan replaces labels only when
supplied; `--clear-labels` removes them, and `--milestone ""` clears the milestone.
Omitted fields are preserved. References stay canonical URNs in the feature plan;
renaming a shared record changes every displayed name without changing membership.
Legacy free-text milestones remain readable; new assignments require a shared
record. Use public authoring commands, never direct metadata edits.
