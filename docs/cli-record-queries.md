# Persona and term query contract

Status: shipped query contract, updated for [Git-owned history](git-owned-history.md).
Name-only rename commands remain unimplemented; `revise` edits current content.

These operations extend the existing `change-saga.ai/v1` envelope. They use
Saga's current records, canonical typed references, source/Saga snapshot, and
snapshot-bound cursor contract; they do not introduce a generic graph API or a
second index.

## Final command spellings

```text
change-saga query personas --saga PATH [--persona ID|URN] [--limit N] [--cursor TOKEN] [--repo PATH]
change-saga query persona-references --saga PATH --persona ID|URN [--limit N] [--cursor TOKEN] [--conflict-limit N] [--conflict-cursor TOKEN] [--repo PATH]
change-saga query terms --saga PATH [--term ID|URN] [--story ID|URN] [--ref LOCATION] [--limit N] [--cursor TOKEN] [--repo PATH]
change-saga query term-references --saga PATH --term ID|URN [--limit N] [--cursor TOKEN] [--conflict-limit N] [--conflict-cursor TOKEN] [--repo PATH]
```

Use `change-saga query schema OPERATION` for the machine-readable data paths,
usage, and pagination fields. `change-saga spec --json` publishes the same
query schema and operation registry under `query`.

`personas` returns stable identity, immutable creation time, complete current
name and description, current state and content digest, and explicit legacy
conflict diagnostics. Content revision and lifecycle head arrays are omitted. `terms`
returns the corresponding term fields plus aliases, applicability, semantic
maturity/evidence axes, and code-reference health. Exact selectors accept only
a stable ID or canonical URN in the opened Saga. They never select by display
name. A missing identity is `not_found`; malformed and foreign-Saga selectors
are `invalid_argument`. Retired records remain readable. Legacy competing heads remain
visible as unresolved diagnostics and no current definition is synthesized.

Persona enumeration and all reference queries are bounded (default 100,
maximum 1000). For compatibility, `query terms` without `--limit` or
`--cursor` still returns the complete legacy term collection and its legacy
singleton page envelope. Supplying `--limit` opts into bounded enumeration;
subsequent calls use `page.next_cursor` until `page.has_more` is false.

## Reference result semantics

`persona-references` and `term-references` report direct, explicit references
only. Each entry names `source`, `target`, direction relative to the selected
subject, relationship/field `kind`, the exact owning record, selector, and
provenance resource. The result-level counts cover the full filtered result at
the response snapshot, while `page.returned` counts only the current page.

Covered classes are:

- current story `personas[]` assignments;
- current term `stories[]`, `records[]`, and exact `code[]` references;
- onboarding and review Item `record` fields; and
- canonical living relations, including complete-slide transaction relations
  projected by the existing semantic graph.

Canonical relations return their stored pins and the existing currency status,
reason codes, and carry-forward reasons. The current v5 relation endpoint
matrix permits stories, criteria, and design/review targets—not personas or
terms—so a valid persona/term response presently has no relation-currency row.
The query still consumes the canonical projection rather than inspecting only
standalone relation files, preserving truthful behavior if the domain contract
later adds a supported endpoint.

Free-form prose, embedded SVG text, historical revisions, and transitive graph
expansion are explicitly excluded. Those exclusions appear in
`data.completeness`; they must not be treated as searched or rewritten.
Conflicted owners that could contain a reference make `complete` false. Their
details are independently bounded under `data.completeness.unresolved_owners`,
with totals and continuation in `unresolved_page`. Its page size defaults to
the primary `--limit` (or 100) and may be set separately with
`--conflict-limit`; continue it with `--conflict-cursor`. This channel remains
available when the primary reference total is zero, and retains every competing
head. `query schema persona-references` and `query schema term-references`
publish this as `additional_pagination`. Distinct field occurrences remain
distinct; only an identical canonical use is deduplicated.

Cursors bind the operation and pagination channel, normalized filters, offset,
and snapshot. A tampered cursor or one used with another operation/filter or
between the primary and conflict channels is
`invalid_argument`. A cursor from a changed Saga/source snapshot is
`stale_snapshot` with `retryable: true`; restart traversal instead of mixing
pages.

## Current editing and the earlier rename proposal

Use `persona revise` and `term revise` for deliberate current-content edits.
Writers reload under the Saga lock and atomically replace `current.json`.
Equal-content retries are no-ops; there is no revision-ID or request-receipt
ledger. Renaming does not change stable identities or rewrite story/prose/SVG
text, create aliases, repin relations, or record approvals. Relation currency
continues to follow endpoint-specific content comparisons.

The earlier guarded rename proposal and its persisted replay-receipt design
were not shipped. Their history machinery is superseded by Git-owned history;
this is not a format blocker for ordinary current-content editing.

## Measured eight-story read

`TestRecordQueryWorkflowMeasurements` constructs eight deterministic story
references and records actual JSON bytes. In the 2026-09-23 implementation
run:

| Workflow | Commands | Response bytes | Complete for the requested read? |
| --- | ---: | ---: | --- |
| Legacy `status --json` | 1 | 143,932 | No; it omits the persona description |
| Exact `personas` read plus `persona-references --limit 3` traversal | 4 | 8,657 | Yes; one record page plus three reference pages |

These are response bytes, not token estimates or a performance benchmark. The
legacy row is deliberately marked incomplete, so it is not presented as an
equivalent before/after latency comparison. Rename command counts and bytes
cannot be measured because the guarded mutation is not shipped.

The conflict-bound regression fixture injects 1,000 valid-shaped conflicted
owners with no resolved references. At `--limit 1`, both persona and term
responses return one unresolved owner, report `unresolved_page.total: 1000`,
and encode to 1,911 data bytes; cursor traversal recovers all 1,000 owners and
both heads for each. The originally reproduced unbounded persona data response
was 234,959 bytes.
