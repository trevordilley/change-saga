# Feature and story reading

Features have a brief immutable identity description and an optional substantial
Markdown overview. Stories keep their authored short value in `title` and full
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
own title, statement, and overview. Acceptance/lifecycle events and revision graph semantics are
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
title, statement, and overview. Queries return these fields in current and
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
