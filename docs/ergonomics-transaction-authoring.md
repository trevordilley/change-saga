# Complete-slide transactions

`change-saga apply-slide` publishes one coherent implementation slide from a
versioned JSON request. The request is a complete desired state: visual asset,
slide metadata, ordered semantic Items and selectors, exact code evidence, and
Item-level criterion links.

```sh
change-saga apply-slide --from slide.json --repo . --dry-run --json app.saga
change-saga apply-slide --from slide.json --repo . --json app.saga
```

The Go entry point is `cli.ApplySlideTransaction(ctx, sagaRoot, requestBase,
repo, request, dryRun)`. `requestBase` resolves a request's relative asset path;
`repo` is the checkout used to verify every pinned code range and digest.

## Request contract

The request has `version: 1`, `operation: "create"` or `"update"`, a stable
`request_id`, a deck selector, and a complete slide value. Creation requires
`expected_snapshot: "absent"`. Update requires the exact `sha256:...` snapshot
returned by the preceding successful result or by `query slide` as
`authoring_snapshot` (including for a legacy slide's first migration).
A retry with the same request ID
and payload is a no-op; reusing the ID with different content is rejected.

Each Item must provide:

- one selector that resolves in the submitted asset;
- at least one line-range code reference with a note and digest that matches
  the named Git commit;
- at least one exact criterion URN, its current story-revision URN, and a
  rationale.

The command rejects whole-file evidence, broad Deck/Slide criterion sources,
stale criterion revisions, missing criteria, selector-breaking visual
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
same-directory atomic record create/replace publishes the complete revision.
A reader therefore resolves either the preceding complete revision or the new
complete revision; it cannot resolve a new visual with old selectors or vice
versa. A failure before publication removes a newly created unreferenced asset
and leaves the previous record current. A failure after publication may leave
the new value visible or its crash durability uncertain; the error distinguishes
that state and the referenced asset is retained. Query current state before
retrying, and preserve the same request ID for an identical replay.

The record keeps up to 256 immutable revisions, preserving prior asset digests,
Items, evidence, criterion pins, request IDs, and timestamps. Migrating a
legacy flat slide captures its previous complete state as the first revision;
the old flat files remain as non-authoritative history. Older partial slide,
Item, content, coverage, and embedded-link mutations refuse transaction-managed
targets and direct the caller to query the complete state and use `apply-slide`.

Each revision records its `parent_snapshots`. Queries expose all
`authoring_heads` and `authoring_conflict`; an ordinary update refuses divergent
heads. After explicitly resolving the desired content, use `operation:
"reconcile"` with `expected_snapshots` naming every divergent head and omit
`expected_snapshot`. All prior revisions remain. This handles histories already
combined in a record; it does not automatically resolve a Git text conflict.

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
      "story_revision": "urn:change-saga:shop:story:checkout:revision:r3",
      "rationale": "The Item shows the exact implementation of this obligation."
    }]
  }]
}
```
