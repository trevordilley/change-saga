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
own statements. Acceptance/lifecycle events and revision graph semantics are
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
