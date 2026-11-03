# Inventory: Components, Systems, and data entities

Read this reference when working with existing inventory records, data entities,
ERD records, or an Item's pinned documentation link. Read [query.md](query.md)
first. Most new technical design needs none of it: see [whiteboards](whiteboards.md).

## Existing inventory definitions: Components and Systems

Components and Systems remain available for existing inventory and pinned deck
evidence. They are not required parts of a Technical design. For a new design,
write the approach essay and the ERD, Infrastructure, Data flows and Architecture whiteboards;
do not build a Component/System catalog as a prerequisite.

When the requested work involves those existing records, start with
`query inventory` and reuse their identities. Components identify meaningful
logic/transformation with exact code; Systems pin Components and explain their
directed interactions with scoped evidence. Discover the JSON contract through
`spec --json --section technical_inventory` and command help. Update definitions with `add|revise --from` and
lifecycle with `set-state`, preserving parent heads and provenance.

Use data entities for meaningful persisted/domain records and their evidenced
relationships; explain them together in one cohesive data-model whiteboard.
Existing ERD records can supply optional pinned imports, without requiring a
separate ERD document per section. A function, endpoint or table does not
automatically need a record.

For example, when an existing Component describes a job runner or service
gateway, slides may pin that definition while keeping their contextual
explanations and exact code.
Naming it in prose or drawing a matching box creates no declared link. Preserve
the simple Front summary; use Back, notes and linked definitions for depth.

Before handing off the requested documentation, inspect the selected inventory
records and `query inventory-uses --target URN`: do the Items and Systems that
rely on a shared definition actually reference it? Check `query slide` for
Item pins, exact evidence and criterion links. Report meaningful missing links
within the requested scope; do not fill an inventory quota, invent relationships,
or claim whole-codebase impact coverage from an unconnected set of decks.

An implementation Item may link a canonical definition with `--documentation`
and `--documentation-revision`, or the optional `documentation` field of an
`apply-slide` Item. Keep contextual labels and exact Item evidence: a System
link grants no coverage of that System's code. Read the saved revision before
explicitly repinning; never rewrite old review content or approve a slide as a
side effect. The drawer opens the explanation, its Components and exact code,
then returns to the same implementation slide. Use `query inventory` for
current heads, code and Component pin health, and selected-record history;
`query inventory-uses` for the Items and Systems that declare a definition; and
`query inventory-coverage` for code the inventory accounts for. `reconcile`
reports stale definition evidence, Item pins that are not current, and
definitions without implementation-deck use. Inventory is not part of generic
`references`/`repin`, feature audits or comparison layers; do not infer those
integrations.

Before inventory authoring, run the idempotent initialization command
`change-saga inventory adopt-format --format 2 <saga>`. Use the current schema
without a format-choice interview. Reads and ordinary slide authoring need no
inventory initialization.

Every new Component, System or data-entity revision states `intent`. Record a
design as `proposed` (with `baseline` naming the implemented revision it changes,
or `none`) before code
exists; record delivery with a new `implemented` revision and `--delivery` at
the commit whose code you verified, keeping the proposal in history. Give each
reference a stable evidence `id`. Leave a relationship or interaction
`proposed` until its own evidence exists; nothing is promoted by implication.
To show only the relevant code of a documented entity, add `--selections`
naming the path of saved pins, the evidence ID and the subset range. Pin an
older revision only with `--documentation-view` naming the saved Saga commit
that bound it. Intent and selections are never approval or proof of coverage.
