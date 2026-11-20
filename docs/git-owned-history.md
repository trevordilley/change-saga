# Git owns documentation history

Saga stores the editable documentation of the application as it stands now.
Git records its versions, attribution, branches, merges, and conflict resolution.
A documentation edit replaces the current value; it does not publish another
Saga revision or ask the author to name parents.

Stable record URNs, criterion IDs, test step IDs, diagram element IDs, and Item
IDs continue to identify the same things across edits. Actual review decisions,
comments, test evidence, recorded executions, quality policy decisions, and
coverage exceptions remain domain facts. Source repository identities, real
Git commit pins, and byte digests retain their evidence meaning.

## Current storage

Stories, personas, terms, flags, deprecations, technical inventory definitions,
prototypes, and test definitions have one `current.json` in their existing
package. Existing identity files retain stable identity metadata. A current
record follows [current-record.schema.json](../schema/v5/current-record.schema.json):

```json
{
  "$schema": "https://changesaga.dev/schema/v5/current-record.schema.json",
  "version": 1,
  "content": {
    "persona": "urn:change-saga:shop:persona:buyer",
    "name": "Buyer",
    "description": "The person buying the product."
  },
  "state": {
    "persona": "urn:change-saga:shop:persona:buyer",
    "state": "active"
  }
}
```

Whiteboards use `___whiteboards/<board>/current.json`, whose content is `name`
and `document`. Prototype content includes its state and source. Neither needs
an independent state object. Copied prototype HTML lives under `assets/<digest>/`
inside the existing prototype package. Asset digests describe bytes, not a
content revision graph.

Relations use `relation-current.schema.json` at their existing paths. Reassessment
edits the current pins and rationale. Legacy `relation-repins/` overlays are
folded in their existing `created_at`, then ID order, preserving the exact
legacy effective pins and original rationale. Publication precedes removal of
the obsolete overlays. This preserves existing stale evidence; it does not
select a confirmation because it matches current content.

A complete slide keeps its existing `25-t-*.json` publication path and has
`$schema`, `version: 2`, `deck`, `slide`, and one `content` value, following
[slide-current.schema.json](../schema/v5/slide-current.schema.json). Assets are
published before that record. There is no revision array, current pointer, or
parent-snapshot list. Removing a slide changes its current removal state;
Git retains its previous content.

## Authoring and pins

Use `add` for a new stable record and `revise`, `set-state`, or `whiteboard
publish` to edit its current data. Content revision IDs and lifecycle event IDs
are not authoring inputs. Saga does not reconcile content heads after Git
merges. Resolve the authored files with Git's normal tools.

A semantic reference consists of a stable target and an exact content digest.
Existing pin field names such as `revision`, `story_revision`, and `test_revision`
remain decoder compatibility names, but normalized values are `sha256:<hex>`,
not revision URNs. They do not select another stored Saga version. Exact source
commit and range digests remain separate code evidence. A saved source view
continues to use a full real Git commit.

Content digests hash canonical JSON of the authored fields, excluding only
outer storage metadata (`$schema`, `version`, revision `id`, `parents`,
`created_at`, and `request_id`). Nested stable IDs, ordinary authored content,
and source pins remain part of the digest. A changed source leaves an old pin
stale. Migration never replaces that pin with the current source's digest.

Use `query history --saga PATH --node URN` for Git-backed history and normal
Git commit views for historical documentation. Historical technical content and
whiteboards can be resolved by their exact digest from committed files. An
explicit saved whiteboard view is labeled as content from Git and opens its
original evidence. Actions from a current page that has since changed return a
conflict instead of opening replacement evidence. If the Git object is
unavailable, readers report an unresolved pin; they never substitute current
content. Semantic comparisons against an earlier story or criterion likewise
read its exact committed content; no comparison archive is retained. An
overwritten uncommitted value absent from Git remains unresolved. Historical lookup is bounded to 512 relevant commits and a 15-second query; an
unavailable result remains explicitly unresolved. Recorded test executions are available through `query quality-runs`.

Complete content updates are idempotent by value. Delta operations such as
`diagram edit` require the current content digest and reject an old base, so a
retried relative move cannot apply twice. Request labels do not create a replay
ledger; read current content before submitting another delta.

## Legacy normalization

Ordinary reads do not write files. They accept legacy data for compatibility.
Current relation publications also take precedence over legacy confirmation
remnants, so interrupted cleanup never reapplies old pins. Other current records
also take precedence over legacy remnants. Authors can inspect a checkout without
unexpectedly changing it.

A requirements, inventory, prototype, or test-definition write normalizes legacy
record packages encountered in its Saga under the existing writer lock. The
optional `change-saga normalize PATH` performs the same operation explicitly.
Normalization is deterministic and idempotent:

1. Inspect legacy parent relationships and require one unambiguous selected
   current content and state. No timestamp selects an authored value.
2. Resolve revision-URN evidence pins against the exact old content, including
   its nested pins, and convert those references to content digests.
3. Prepare selected prototype assets and atomically publish current records.
4. Remove obsolete `revisions/` and `events/` storage only after reference
   conversion and current publication have succeeded. Actual review/evidence
   records and ordinary assets are retained.

Every current record is a single atomic replacement. Reference conversions are
preflighted, then published atomically per file before any source is deleted.
The whole Saga is not one crash-atomic filesystem transaction: an interrupted
run may have some converted references and published current records. Legacy
sources remain until all current publications have succeeded. Retrying completes
publication and cleanup; no permanent migration journal or second history store
is created.

Competing legacy heads stop normalization and list the competing IDs and paths.
All independently authored files remain. Preserve them in Git, inspect and
combine their content explicitly. For a whiteboard, publish that complete
resolved document with ordinary `whiteboard publish --id ID --from FILE PATH`.
The writer verifies the competing legacy files are committed before replacing
them with one current record and converting exact pins. It does not choose a
head, require parent IDs, or keep a second history store. Automatic normalization
still stops on ambiguity; explicit authoring supplies the resolved content.
Missing evidence sources also stop conversion;
restore the exact source from Git or author a verified replacement pin.

The Saga manifest remains version 5; the current-record component schemas
identify the new layout. Older readers reject its unfamiliar files and slide
version rather than silently ignoring documentation. Upgrade readers before
writing normalized data. Normalize legacy documentation branches before merging
them into a current-record branch so Git compares the same current files.
To inspect the original layout, use its Git commit; there is no automatic downgrade.

Plan layout alone does not select a compatible reader: v0.7.0 supports phased
authoring and v0.8.0 adds Plan queries, but both reject current-record layouts.
Use [whole-Saga compatibility diagnostics](plan-cli-compatibility.md) before
selecting a pinned build. Native checkpoints are now integrated with this
current-record reader. Earlier builds, including `ce8d796d`, reject them.

## Offline examples and exports

An archive of a Saga is not an archive of its Git history. An exact pin to content
that is absent from the current files still requires the real Git objects;
missing content must fail explicitly. Neither export nor validation substitutes
the current definition or copies older values into a Saga history sidecar.

The downloadable example is deliberately authored in
[`examples/offline/change.saga`](../examples/offline/change.saga), separately from
the canonical repository Saga. It has current documentation and an unpassed Plan
checkpoint, with no historical evidence dependency. Its archive is tested after
extraction outside a checkout, including validation, Plan queries and reader
pages. To inspect the full repository Saga and its historical evidence, use its
corresponding Git checkout. An export promising historical views must provide
access to genuine Git provenance and objects.
