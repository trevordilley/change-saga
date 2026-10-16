# Product themes

A product theme explains why related features belong together. Themes are
optional: an existing Saga needs no migration, and features and stories retain
their paths and stable URNs. A feature can belong to several themes, or none.
Older installed CLI readers do not understand the new `___feature-themes`
metadata; use a reader with product-theme support after adopting it.
The `theme` command still controls visual styling; product authoring uses
`feature-theme`.

Story authoring is the default. Agents should offer theme creation as an
optional follow-up when it would help explain related features, and continue
creating or revising the requested stories without waiting for that decision.
Create themes and their rationale decks only when the user requests them or
accepts the offer; prior authorization counts. Existing themes can be reused.
The viewer defaulting to Theme grouping does not imply automatic theme creation.

When themes are requested, create a theme with an authored Markdown overview,
then author its rationale deck using the ordinary deck, slide, and Item commands:

```sh
change-saga feature-theme add --id reliable-fieldwork \
  --title "Reliable fieldwork" --source rationale.md \
  --feature offline-edit --feature sync change.saga
change-saga add-deck --feature-theme reliable-fieldwork --id fieldwork-rationale \
  --objective "Explain why uninterrupted fieldwork matters" change.saga rationale
change-saga add-slide --deck fieldwork-rationale --id fieldwork-why \
  --intent explain --layout diagram --source why.svg change.saga why
change-saga add-item --slide fieldwork-why --id continuity --kind node \
  --element-id continuity --label "Work survives interruptions" \
  --record urn:change-saga:APP:feature:offline-edit change.saga
```

Use the Saga's actual ID in record URNs. `--record` is optional for theme Items;
when supplied it must name an existing persona, feature, or story in this Saga.
The normal `apply-slide` transaction, diagram tools, `deck overview`,
`set-slide-content`, revise/remove commands, and `visual-qa --deck` also work.
Transaction-managed slides retain their usual snapshot and mutation guards.
Theme decks explain rationale: they need no code evidence or implementation
criterion links. Optional supplied code and technical-definition pins are still
validated. Theme decks cannot establish implementation criterion coverage.

```sh
change-saga feature-theme set-overview --theme reliable-fieldwork \
  --source revised-rationale.md change.saga
change-saga feature-theme set-features --theme reliable-fieldwork \
  --feature offline-edit --feature sync change.saga
# Omitting --feature removes all memberships, without deleting any feature:
change-saga feature-theme set-features --theme reliable-fieldwork change.saga
```

Theme mutations use the Saga writer lock and atomic record replacement. An exact
retry is a no-op; retrying `add` with different content is rejected. Membership
is a sorted set. Missing members, cross-Saga URNs, unsafe IDs, symlinked reserved
paths, oversized records, unknown JSON fields, and duplicate stored members are
rejected. Overview text is bounded to 256 KiB; the serialized record to 1 MiB;
a Saga can contain at most 1,000 themes and each theme at most 1,000 features.

Each theme lives at `___feature-themes/<id>.theme/theme.json`, with a stable
`urn:change-saga:<saga>:feature-theme:<id>` identity. The record contains its
Markdown `overview` and canonical feature URNs in `features`; Git preserves
edits. Its optional, single rationale deck lives in that directory's `___slides`
root and uses the existing v4 deck/slide/Item format with role `feature-theme`.
The UI shows an explicit gap until the deck is authored.

For every nonempty collection, the menu beside Features or Roadmap controls
grouping for its directory and sidebar: None, Theme (when themes exist),
Persona, Label, or Milestone. Theme is the default when available. Each section
remembers its grouping independently, including on feature deep links. Labels and
milestones are shared entities referenced by features; see
[feature organization](feature-story-reading.md#roadmap).
Persona membership comes from current story revisions, including proposed
stories, deduplicated per feature and persona. Every grouped view keeps
ungrouped features visible. Choosing a grouping reloads the page and saves the
choice for subsequent navigation; feature deep links remain `/features/<id>`.
The Product themes directory opens authored overviews and normal rationale
decks. Repeated feature rows have independent navigation state and unique
presentation IDs, with the same canonical destinations.

```sh
change-saga query feature-themes --saga change.saga --limit 50
change-saga query feature-themes --saga change.saga --theme reliable-fieldwork
change-saga query schema feature-themes
change-saga query overview --saga change.saga --deck urn:change-saga:APP:deck:fieldwork-rationale
change-saga query slide --saga change.saga --target urn:change-saga:APP:slide:fieldwork-why
change-saga query history --saga change.saga --node urn:change-saga:APP:feature-theme:reliable-fieldwork
```

Theme discovery lists compact summaries (50 by default, maximum 100 per page),
including feature counts and deck targets. Selecting a theme also returns its
authored overview and membership. Continue with `page.next_cursor`; cursors are
bound to the query and current theme/deck snapshot and reject stale reads.
Slides and Items use the normal overview/children/slide query surfaces. `spec
--json` publishes the authoring grammar and schema resource.
