# Persona and term query contract

Status: shipped query contract. The guarded rename proposal in
[cli-ergonomics-plan.md](cli-ergonomics-plan.md) remains blocked as described
below.

These operations extend the existing `change-saga.ai/v1` envelope. They use
Saga's current records, canonical typed references, source/Saga snapshot, and
snapshot-bound cursor contract; they do not introduce a generic graph API or a
second index.

## Final command spellings

```text
change-saga query personas --saga PATH [--persona ID|URN] [--limit N] [--cursor TOKEN] [--repo PATH]
change-saga query persona-references --saga PATH --persona ID|URN [--limit N] [--cursor TOKEN] [--repo PATH]
change-saga query terms --saga PATH [--term ID|URN] [--story ID|URN] [--ref LOCATION] [--limit N] [--cursor TOKEN] [--repo PATH]
change-saga query term-references --saga PATH --term ID|URN [--limit N] [--cursor TOKEN] [--repo PATH]
```

Use `change-saga query schema OPERATION` for the machine-readable data paths,
usage, and pagination fields. `change-saga spec --json` publishes the same
query schema and operation registry under `query`.

`personas` returns stable identity, immutable creation time, complete current
name and description, lifecycle state, revision and lifecycle heads, current
records when uniquely resolvable, and explicit conflict booleans. `terms`
returns the corresponding term fields plus aliases, applicability, semantic
maturity/evidence axes, and code-reference health. Exact selectors accept only
a stable ID or canonical URN in the opened Saga. They never select by display
name. A missing identity is `not_found`; malformed and foreign-Saga selectors
are `invalid_argument`. Retired records remain readable. Competing heads remain
visible and no current definition is synthesized.

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
Conflicted owners that could contain a reference appear in
`unresolved_owners`, make `complete` false, and retain every competing head.
Distinct field occurrences remain distinct; only an identical canonical use
is deduplicated.

Cursors bind the operation, normalized filters, offset, and snapshot. A
tampered cursor or one used with another operation/filter is
`invalid_argument`. A cursor from a changed Saga/source snapshot is
`stale_snapshot` with `retryable: true`; restart traversal instead of mixing
pages.

## Guarded rename safety decision

Name-only persona and term edits are not shipped in this milestone. Existing
single-revision writes already reload under the cross-process Saga writer lock
and publish one exclusive, durable JSON file, so a real append can be atomic
and concurrent attempts can be serialized. Existing replay detection is tied
to the revision file and caller-supplied revision ID, however.

The proposed operation also requires all of these properties at once:

- internally allocated revision IDs;
- a stable request ID that survives an uncertain outcome;
- rejection when that request ID is reused with different payload; and
- a true equal-name no-op that creates no unnecessary semantic revision.

For a no-op, the current Git-native format has nowhere to durably reserve the
request ID. Writing a revision would violate no-op behavior; remembering it
only in memory would make retries unsafe; and adding operation-receipt records
would be a persisted format change requiring its own design, schema, merge,
retention, and compatibility review. The implementation therefore stops at
the query milestone rather than weakening idempotency or silently redesigning
the format. Existing explicit `persona revise` and `term revise` commands
remain available for deliberate authoring.

No query or proposed rename changes URNs, rewrites story/prose/SVG text,
creates aliases, repins relations, records approvals, or changes semantic
currency policy. Today a persona name/description or term definition change
creates a normal new record revision; relation currency continues to follow
the existing endpoint-specific comparison rules.

## Measured eight-story read

`TestRecordQueryWorkflowMeasurements` constructs eight deterministic story
references and records actual JSON bytes. In the 2026-09-23 implementation
run:

| Workflow | Commands | Response bytes | Complete for the requested read? |
| --- | ---: | ---: | --- |
| Legacy `status --json` | 1 | 143,932 | No; it omits the persona description |
| Exact `personas` read plus `persona-references --limit 3` traversal | 4 | 8,420 | Yes; one record page plus three reference pages |

These are response bytes, not token estimates or a performance benchmark. The
legacy row is deliberately marked incomplete, so it is not presented as an
equivalent before/after latency comparison. Rename command counts and bytes
cannot be measured because the guarded mutation is not shipped.
