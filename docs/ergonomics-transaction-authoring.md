# Complete-slide transactions

`change-saga apply-slide` publishes one coherent implementation or pull request
review slide from a versioned JSON request. The request is a complete desired
state: visual asset or diagram source, slide metadata, ordered semantic Items
and selectors, and, for an implementation slide, exact code evidence and
Item-level criterion links.

```sh
change-saga apply-slide --from slide.json --repo . --dry-run --json change.saga
change-saga apply-slide --from slide.json --repo . --json change.saga
```

The Go entry point is `cli.ApplySlideTransaction(ctx, sagaRoot, requestBase,
repo, request, dryRun)`. `requestBase` resolves a request's relative asset path;
`repo` is the checkout used to verify every pinned code range and digest.

## Request contract

The request has `version: 1`, `operation: "create"` or `"update"`, an optional diagnostic
`request_id`, a deck selector, and a complete slide value. Creation accepts
`expected_snapshot: "absent"`. Update optionally checks the exact `sha256:...` snapshot
returned by the preceding successful result or by `query slide` as
`authoring_snapshot` (including for a legacy slide's first migration).
A retry of the complete current value is a no-op. Request labels are not a version or replay ledger; they are not stored.

Each Item must provide:

- one selector that resolves in the submitted asset;
- at least one line-range code reference with a note and digest that matches
  the named Git commit;
- at least one exact criterion URN, its exact current story content digest, and a
  rationale.

A review slide (`"review": ID`, or `apply-slide --review ID`) is the exception:
its Items carry only a selector (and optionally a `record` to open beside the
change), and `cover --changed-lines` references their code after publishing.

The command rejects whole-file evidence, broad Deck/Slide criterion sources,
stale criterion content pins, missing criteria, selector-breaking visual
replacements, duplicate IDs, unknown JSON fields, traversal, absolute asset
paths, and asset paths containing symlinks. `asset.content_base64` is available
when the caller does not want filesystem path resolution.

The JSON result contains the previous and next snapshots, a concise sorted
`changed_ids` list, and a semantic diff with asset, Item, and selector changes.
Dry-run performs the same repository and semantic checks in an external staging
directory and writes nothing into the Saga.

## Atomicity boundary

The atomic boundary is exactly one complete slide transaction record inside an
existing deck bundle. Visual bytes are written first to an immutable,
content-addressed regular file. They are not visible as a slide until one
same-directory atomic record create/replace publishes the complete value.
A reader therefore resolves either the preceding complete value or the new
complete value; it cannot resolve a new visual with old selectors or vice
versa. A failure before publication removes a newly created unreferenced asset
and leaves the previous record current. A failure after publication may leave
the new value visible or its crash durability uncertain; the error distinguishes
that state and the referenced asset is retained. Query current state before
retrying, and preserve the same complete desired value for an identical retry.

The version-2 stored record contains one current content value, including a
current deletion marker when removed. It has no revision list, parent snapshots,
head pointer, authoring timestamps, or request journal. Git owns prior slide
content and its attribution. Existing visual assets remain unchanged.

Legacy input is normalized only when a writer encounters it. A unique current
value is published before obsolete history metadata is discarded. Competing
legacy heads remain intact and require explicit selection of complete current
content; there is no Saga reconciliation operation. Git resolves file conflicts.
Older partial Item, content, coverage, and embedded-link mutations refuse
transaction-managed targets and direct callers to `apply-slide`.

This is not a generic filesystem transaction and does not claim atomicity with
story edits, standalone relation records, another slide, a Git commit, or any
external system. Criterion links are transaction-owned exact pins projected into
the canonical in-memory requirements relation graph. Context, audit,
traceability, currency checks, and reviewer story hovers consume that graph;
no duplicate standalone relation file is written. Story consolidation refuses
to retire intent while a current transaction still links to it; first repoint
those links through `apply-slide` and then preview consolidation again.

## Example skeleton

```json
{
  "version": 1,
  "operation": "create",
  "request_id": "checkout-flow-v1",
  "deck": "implementation",
  "expected_snapshot": "absent",
  "slide": {
    "id": "checkout-flow",
    "title": "Checkout flow",
    "rank": 10,
    "intent": "explain",
    "layout": "diagram",
    "media_type": "image/svg+xml",
    "takeaway": "Validation completes before persistence.",
    "reading_order": ["validator"]
  },
  "asset": {"path": "checkout-flow.svg"},
  "items": [{
    "id": "validator",
    "rank": 10,
    "kind": "node",
    "label": "Validator",
    "description": "The boundary that rejects malformed input.",
    "selector": {"type": "element", "element_id": "validator"},
    "evidence": [{
      "version": 2,
      "references": [{
        "commit": "0123456789abcdef0123456789abcdef01234567",
        "path": "internal/checkout/validate.go",
        "start": 20,
        "end": 34,
        "digest": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
        "note": "This range enforces the validation boundary."
      }]
    }],
    "criterion_links": [{
      "id": "validator-explains-valid-input",
      "criterion": "urn:change-saga:shop:story:checkout:criterion:valid-input",
      "story_revision": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
      "rationale": "The Item shows the exact implementation of this obligation."
    }]
  }]
}
```
