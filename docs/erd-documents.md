# Existing ERD source documents

For new data-model authoring, use [one cohesive ERD whiteboard](erd-whiteboard-guide.md).
This guide covers independently revisioned ERD source records and the reading
experience when no `data-model` whiteboard is present. Existing records remain
useful as optional imports and evidence; readers need not navigate a collection of them
to understand the whiteboard.

Separate named ERD records can describe different concerns. A local SQLite database,
a cloud Postgres database, and their synchronization diagram can each have their
own name, explanation, SVG and entity directory. Shared Markdown introduces the
whole data model. These documents use the existing ERD identities and append-only
revision history; `application` is an ordinary ID.

## Authoring

1. Record each store as a Component, then record its data entities with exact
   `holders` pins to that Component. Data entities own fields and relationships.
2. Prepare one complete ERD definition JSON and offline SVG per document.
   `directory` lists exact data-entity revision pins. `bindings` attach SVG element
   IDs to entities or to relationships owned by a pinned entity. See the
   [format contract](../SPEC.md#technical-inventory-format-2) for field details.
3. Publish each diagram independently:

```sh
change-saga erd add --id sqlite --from sqlite.json --visual sqlite.svg change.saga
change-saga erd add --id postgres --from postgres.json --visual postgres.svg change.saga
change-saga erd add --id cross-store --from cross-store.json --visual cross-store.svg change.saga
change-saga overview set-data-model --source data-model.md change.saga
```

The cross-store document can pin entities already used by the other documents.
Declare synchronization as a `production` relationship with explicit direction,
label and explanation. A diagram spanning two stores does not imply a physical
foreign key between them. The shared overview might say:

```markdown
Local SQLite stores offline edits; cloud Postgres stores shared records.

[SQLite](/technical/erd/sqlite) and [Postgres](/technical/erd/postgres) explain
each store. [Data flow](/technical/erd/cross-store) explains synchronization.
```

Markdown links navigate; they are not inventory pins and do not confer evidence
or coverage. To link to an exact diagram revision, use
`/technical/erd/sqlite?revision=r1`. Structured entity pins in each ERD continue
to control what the viewer shows and validates. Raw HTML and dangerous URL
schemes use the existing safe Markdown renderer.

Repeat `overview set-data-model` to edit the shared prose. It preserves the
fragment identity and updates Markdown content; Git records its history. Use
`erd revise --id sqlite --revision r2 --parent <r1-URN> --from ...` to append an
ERD revision. Neither operation rewrites older ERD revisions or SVG assets.

## Reading

```sh
change-saga query inventory --saga change.saga --kind erd
change-saga query fragment --saga change.saga --target <data_model_overview.target>
change-saga query history --saga change.saga --node <data_model_overview.target>
change-saga query inventory --saga change.saga --target <erd-URN> --history
```

`data.data_model_overview` is the app-wide overview descriptor (`target`, package
`path`, `media_type`), or null when absent. It is independent of the inventory
record page and filters. Use the returned target rather than assuming a fragment
ID. The fragment query supplies bounded content using its existing pagination.

Without a `data-model` whiteboard, the Data model page at `/technical/erd` shows the overview,
equal document links, proposed overlays, and the entity directory. Documents come before entities in
the sidebar. A single unambiguous active ERD also renders inline; with multiple
ERDs no document is automatically elevated. Each document retains its canonical
route and historical `?revision=` links. Retired or conflicted records stay
visible with their status.

## Format and compatibility

The optional `___overview/data-model.fragment/` package contains `fragment.json`
and its Markdown entrypoint, normally `content.md`. It reuses the version-2
fragment contract, constrained to `text/markdown` by the
[data-model overview schema](../schema/v5/data-model-overview.schema.json).
Its content appears on Data model, without duplication on the application
Overview. Absence is valid and does not trigger a generated document.

This extends the allowed v5 layout without changing any ERD schema or inventory
format. New readers accept preexisting Sagas unchanged. Older readers reject an
adopted data-model overview slot as an unknown overview entry. Upgrade those
readers before authoring the new slot. No automatic migration or rewriting of
records is performed.

The [multi-document fixture](../internal/testfixture/erd_documents.go) is a
runnable example used by the CLI and viewer tests. It describes a proposed
SQLite/Postgres architecture, independently of this repository's own storage.
