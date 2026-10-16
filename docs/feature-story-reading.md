# Feature and story reading

Features have a brief immutable identity description and an optional substantial
Markdown essay. Stories keep their authored short value in `title` and full
need in `statement`: “As a harbor planner I can choose a safe departure time.”
No parser strips or rewrites existing authored content, and no persona is inferred.

```sh
change-saga feature set-overview --feature tide-charts --source feature.md change.saga
change-saga story add --feature tide-charts --id departure --revision r1 --event proposed \
  --title "Choose a safe departure time" \
  --statement "As a harbor planner I can choose a safe departure time" \
  --overview-source story.md --criterion "fresh=The forecast shows its observation time." change.saga
change-saga story revise --story urn:change-saga:harbor:story:departure \
  --parent urn:change-saga:harbor:story:departure:revision:r1 --revision r2 \
  --overview-source revised-story.md change.saga
```

Every overview is an essay. The owning project, feature, story, criterion, or
deck header is its entry point; do not add an “Overview” child or repeat an
“Overview” heading above the prose. Deck controls call this reading mode “Essay”.
The public `overview` fields, commands, and existing links remain compatible.

An Essay has four distinct parts, in order: its title, a one-line subtext under
the title, an abstract marked by a left border, and the essay body. The body is
ordinary unframed prose. Do not use the body as an abstract or repeat its opening
summary in both places. Missing optional parts remain absent.

Feature and Technical design essays accept `--subtext` and `--abstract` alongside
`--text` or `--source`. The abstract accepts Markdown; subtext is plain text.
Omitting these flags preserves existing values; an explicit empty string clears
them. Fragment queries expose both fields. Themes accept the same flags, and
deck essay objects accept `subtext` and `abstract`. Features without an authored
abstract continue to show their identity description; decks fall back to their
objective. The project pitch supplies subtext and its description supplies the
abstract.

Stories and criteria accept `--subtext`; their full `statement` supplies the
abstract, preserving the actual requirement. Their `overview` supplies the body.
Single-parent edits preserve omitted subtext, and an explicit empty string clears
it. Complete revision snapshots include it explicitly when wanted. Changing
subtext makes the affected obligation pins stale, just like changing other prose.

```sh
change-saga overview set-technical-design --title "Technical design" \
  --subtext "How the application stores, serves, and explains change." \
  --abstract "A concise summary of the approach and its main tradeoffs." \
  --source technical-design.md change.saga
```


Use several paragraphs and headings for Description, Purpose, Rationale, examples,
and boundaries as needed. `--text` on features and `--overview` on stories accept
inline Markdown. File source flags accept `-` for stdin; two inputs cannot consume
the same stdin. Raw HTML and dangerous URLs cannot execute in rendered Markdown.

Feature prose is the ordinary report fragment at
`___features/<id>.feature/overview.fragment/{fragment.json,content.md}`. Creation
uses a feature-qualified fragment ID; subsequent edits preserve an existing ID,
path, manifest, and anchors. Identity collisions, symlinks, and non-Markdown
packages are refused. `feature.json` is never rewritten. Git holds prose history.
`query context --feature ID` returns the optional overview descriptor;
`query fragment --target URN` reads its content and `query history --node URN`
reads its history.

Story `overview` is an optional Markdown string in each complete v3 revision.
Single-parent flag revisions inherit it when omitted; `--overview ""` clears it.
The editor round-trips it. Structured `--from` and multi-parent revisions are
complete snapshots: no prose is inherited from an unspecified winner. Criterion
add/revise/remove preserve the rest of the snapshot. Overview changes make
whole-story obligation pins stale; criterion pins continue to compare only their
own title, subtext, statement, and overview. Acceptance/lifecycle events and revision graph semantics are
unchanged. Queries expose current and historical overview values.

Feature pages put narrative and stories before delivery metrics. Story pages put
the full need, overview, and full acceptance statements before secondary metadata
and traceability. Native disclosures open with Enter or Space, including without
JavaScript; retained standalone criterion links show their complete detail page.
Story hash links open the targeted criterion or containing disclosures when
JavaScript is enabled. Fragment anchors and history actions remain available.

Old Sagas need no migration. Missing overviews remain absent, prior authored
content stays intact, and Product Themes keep their existing membership,
grouping, and rationale-deck behavior. Older readers that strictly reject unknown
revision fields need an updated reader before consuming newly authored overviews.

## Acceptance criteria

A criterion can also have an optional short `title` and Markdown `overview`
for rationale, examples, and boundaries. Its required `statement` remains the
full independent pass/fail obligation. The story page shows the title and full
statement together; expanding it reveals the explanation and evidence. The
criterion's own page leads with the same content and collapses traceability.
Without a title or overview, existing statements continue to display as before.

```sh
change-saga criterion revise --story urn:change-saga:harbor:story:departure \
  --criterion urn:change-saga:harbor:story:departure:criterion:fresh \
  --parent urn:change-saga:harbor:story:departure:revision:r2 --revision r3 \
  --title "Know how fresh the forecast is" --overview-source freshness.md change.saga
```

`criterion add` accepts the same prose flags. Use `--overview` for inline Markdown
or `--overview-source -` for stdin. On criterion revision, omitted fields are
preserved and explicit empty title/overview clears them. Structured criterion
mutations are patches with the same omission behavior; full story revisions
remain complete snapshots. The editor may change only the selected criterion's
title, subtext, statement, and overview. Queries return these fields in current and
historical story snapshots.

Changing a criterion's title, statement, or overview requires reassessing its
own evidence pins and whole-story pins; unchanged sibling criteria carry forward.
The existing `criterion_statement_changed` reason code covers changes to any of
these authored fields. Old Sagas need no migration. Older strict readers reject
new criterion fields, so update readers before authoring criterion prose.

## Related reading surfaces

Product themes use the same reading typography and surfaces for their authored
Markdown and feature links; rationale decks expand below them. Personas lead with
their own description, followed by the stories and vocabulary that serve them.
Term pages put the full definition before aliases, related records, and evidence
status, with code in a native disclosure. Test cases lead with preconditions,
numbered actions and their expected outcomes, then the overall expected result;
lifecycle and coverage follow, with evidence and run history in disclosures.
These presentation changes use existing fields and require no data migration.

## Roadmap and built features

Features and Roadmap are two views of the same feature records. Features shows
built capabilities; Roadmap shows capabilities the product owner plans to build.
Built and planned act as mutually exclusive system categories; ordinary labels
and themes allow multiple memberships. Both views use the same Essay, stories,
acceptance criteria, design, quality, and deck
pages. A feature keeps its canonical `/features/<id>` URL and URN when its
planning status changes. Story acceptance and evidence coverage do not determine
whether a feature is built; that is an explicit product decision.

```sh
change-saga label add --id devops --title "DevOps" change.saga
change-saga milestone add --id v2 --title "v2.0" change.saga
change-saga feature add --id offline --title "Offline reading" --status planned \
  --priority 1 --milestone v2 --label devops change.saga
change-saga feature set-overview --feature offline --source offline.md change.saga
change-saga milestone rename --milestone v2 --title "Summer launch" change.saga
change-saga feature set-plan --feature offline --priority 2 change.saga
change-saga feature set-plan --feature offline --status built change.saga
change-saga query roadmap --saga change.saga --limit 50
```

Priorities are positive numbers, with lower numbers first. Unranked work follows;
zero clears a rank. Ties retain feature creation order. A milestone is an optional
shared record naming a version or launch; it does not imply a date or commitment.
Labels and milestones are defined once and features reference their canonical
URNs. Create them first, then assign by ID or URN. Renaming a shared record updates
every display without rewriting feature references. Duplicate names within each
kind are rejected. Legacy free-text milestones remain readable; new assignments
require shared records. Omitted planning fields are preserved; `--milestone ""` clears the
milestone. Repeat `--label ID|URN` to assign multiple labels; on `set-plan` this
replaces the label set. Omission preserves labels; `--clear-labels` removes them.
Promotion to built preserves labels, milestone and priority history in Git, along
with all feature content, links, and theme memberships. `feature add` defaults to
built; pass `--status planned` when authoring future work. Legacy features without
planning metadata retain their existing placement in Features.

Features and Roadmap each support None, Theme, Persona, Label, and Milestone
grouping, even for a single feature. Theme
membership can span built and planned work; each section displays only its own
members, and ungrouped features remain visible. Priorities order features within
each group. Each section remembers its own grouping preference. Labels and milestones are
searchable in both directories, including without JavaScript. Themes remain
shared entities with their own Essay and rationale decks; labels provide simple
categories. A feature can appear in multiple label or theme groups while retaining
one identity.

Use `query labels` and `query milestones` with `--saga PATH` to discover existing
records before creating any. Both queries support snapshot-bound pagination.
`label rename --label ID|URN --title TEXT PATH` and the equivalent `milestone
rename` edit the shared name once.

The immutable identity remains in `feature.json`; editable planning metadata
lives beside it in `plan.json`. `query roadmap` returns ordinary feature identities,
plans, and theme URNs using snapshot-bound pagination. Follow its cursors to
completion. `query context --feature ID` includes the same plan so agents can
read a priority and then use the normal feature context and authoring commands.
