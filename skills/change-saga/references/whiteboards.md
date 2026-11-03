# Technical design and whiteboards

Read this reference to author technical design: the approach essay, its supporting
pages, and the ERD, Infrastructure, Data flows and Architecture whiteboards. Read
[query.md](query.md) first when a Saga already exists. Elements are written in the
[diagram source grammar](diagram-source.md); all whiteboards share that grammar,
nested sections, cross-section connections and the revision contract. Before
storyboarding a board, read [visual storytelling](visual-storytelling.md).

## Technical design

Technical Design includes an Architecture whiteboard alongside ERD, Infrastructure
and Data flows. For codebase structure, patterns, state ownership and technology
choices, read [architecture guidance](references/architecture.md) before drawing:
gather code evidence and boundary rules, build a map with question-led sections,
generate and visually inspect a scratch board, then independently fact-check its
claims before publication. Reuse the shared whiteboard format; feature decks
link to the wider architectural context.

Technical design is one approach essay plus four whiteboards: ERD (`data-model`),
Infrastructure (`infrastructure`), Data flows (`data-flows`) and Architecture
(`architecture`). Read [architecture guidance](architecture.md) when explaining
codebase structure, patterns and technology choices. The essay explains
how the application works, the major decisions and tradeoffs, and links into the
boards. Use `overview set-technical-design --source FILE PATH`; add a supporting
page with `--page ID --title TITLE` and its own `--subtext`. Supporting pages
open at `/technical/pages/ID` and are linked from the Technical Design essay,
not extra sidebar children. An empty subtext currently displays a generic
Technical Design subtitle; it does not inherit the main essay's authored subtext.
Systems and Components are no longer design navigation areas.

Those IDs are defaults, not required identities. Before creating a board,
inspect `change-saga whiteboard list --json change.saga`. Reuse an existing
document with `whiteboard assign-role`; see [reorganize.md](reorganize.md) for
observed-owner checks, role reassignment, rename and archival. Do not copy a
board or rename its ID merely to give it the ERD role.

The ERD uses proper crow's-foot `head`/`tail` markers (`only-one`, `zero-or-one`,
`one-or-many`, `zero-or-many`) matching recorded cardinality and optionality.
Infrastructure shows the actual platform and resource layout. Data flows show
movement through key services, stores and transformations, with directed and
labeled connections. Link from the essay to `/technical/erd`,
`/technical/whiteboards/infrastructure`, `/technical/whiteboards/data-flows`,
and `/technical/whiteboards/architecture`;
append `?focus=SECTION_PATH` for a specific section.

## Named whiteboards

Author each technical-design document with native groups, shapes, text,
annotations and connections. Layout, color, large overview text and callouts
should explain selected concepts. For data-model diagrams, a complete table or
column inventory is not required. Pin depicted data entities and recorded
associations to exact source revisions, including relationship IDs. Distinguish
conceptual flows from recorded relationships and use declared crow's-foot
cardinalities. Existing ERD frames are optional imports. Component and system inventory pins are not
supported whiteboard composition sources; use an element's `documentation` pin to inspect a Component or System.

Use `change-saga whiteboard publish --id ID --revision r1 --name TITLE --from board.json PATH`
for the complete document. Subsequent publications require `--parent` naming
the current board revision; source adoption is explicit. Use `whiteboard describe
--id ID [--revision ID] [--focus PATH] --format json PATH`, `whiteboard get
--id ID [--revision ID] [--focus PATH] PATH` and `whiteboard check --id ID PATH`
to inspect and validate it. `--id` always selects the document; `--focus` selects
an element inside it. There is no kind selector or `diagram` command alias.
A whiteboard has no `diagram edit` transaction; it uses immutable complete
publications. Distinct instance paths identify repeated appearances. Opaque
imported SVGs remain whole diagrams; do not infer internal geometry or fabricate
slide Items for them.

Publication currently has no `--dry-run`, and `visual-qa` selects slides, not
whiteboards. Iterate complete publications in a scratch Saga, then inspect the
opening board and every `?focus=SECTION_PATH` in the reader in light and dark
mode. See [architecture](architecture.md) ("Generate, validate and inspect") for the
generator, overflow-repair and fact-check workflow. `open` starts a managed
reader; `change-saga serve status SCRATCH_SAGA` finds it and `change-saga serve
stop SCRATCH_SAGA` stops it. Always supply the scratch path so unrelated readers
are left running.

Revisions live at `___whiteboards/<board-id>/revisions/<revision-id>.json`;
each document has independent history and must satisfy the shared whiteboard
JSON contract. Read it at `/technical/whiteboards/<id>`. Use `data-model` for
the document shown by convention at `/technical/erd`; other names can explain
other designs without changing the supported source adapters. Existing feature
`___design` fragments are not automatically migrated.

Whiteboard elements own optional `evidence`, `criterion_links`, `record`, and
`documentation` metadata. Pin code here, not as a repository URL in the note:

```json
{
  "id": "resolver", "kind": "node", "shape": "rect", "label": "Resolver",
  "x": 80, "y": 100, "width": 260, "height": 120, "style": "normal",
  "evidence": [{"commit": "HEAD", "path": "internal/coderesolve/resolver.go",
                "start": 10, "end": 30, "note": "Resolves the saved bytes."}]
}
```

Use an actual verified path/range. `whiteboard publish --repo CHECKOUT` resolves
the commit-ish and authors the digest; if a digest is supplied, it must match.
The stored revision contains full exact references. `code` still means code-block
text. `criterion_links` uses the same `{id, criterion, story_revision, rationale}`
shape as complete-slide Items. `record` names a canonical same-Saga record.
`documentation` uses `{target, revision}` for a Component, System or data entity;
this inspection link does not transfer that definition's code ownership.
Inventory-format-2 `selections` and `documentation_view` carry over from Items
unchanged: an explicit path of exact definition pins, a stable evidence ID and
an exact selected code range with its own digest and note. The CLI verifies
saved hops, containment and selected bytes. Only eligible selections contribute
inherited coverage; stale, retired, conflicted or missing pins stay visible.
Changes to the containing evidence outside the subset still require semantic
reassessment. Inspect with `query inventory-selections --item ELEMENT_URN`.
Decorative elements cannot carry these semantic links.

Slide selectors, hotspots, slide/rank ownership and review decisions do not
carry over: the native element already supplies geometry, reading order and
annotation shape, and a whiteboard remains living documentation. Use native
annotations and notes for callouts rather than fabricating slide Items.

The viewer reuses the slide action strip and drawer for code, stories and
records. When referenced code changes, the element is stale in the reader,
`references`, `status`, health and `reconcile`, even when the comparison base
still matches. Reassess the explanation and code, get the complete source, then
publish a new revision with `--parent CURRENT --revision NEW`; previous revisions
stay exact and inspectable. A move of unchanged bytes follows ordinary code
reference remapping. Only current active board revisions contribute living
documented-code coverage. A review's own deck retains independent coverage.
`whiteboard check` warns about repository links in notes without blocking or
rewriting them. Keep the opening view legible; readers can pan and focus detail.
