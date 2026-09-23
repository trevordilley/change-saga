# Feature context query

`change-saga query context` gives an AI a bounded view of one durable feature without making it read every requirements, design, slide, and work-plan record independently.

## Interface

```text
change-saga query context --saga PATH --feature ID|URN \
  [--expand STORY-ID|URN] [--cursor TOKEN] [--limit N] \
  [--repo PATH] [--against REV [--head REV]]
```

`--feature` accepts an exact feature ID or the canonical feature URN for the opened Saga. Titles and prefixes are not selectors. This prevents duplicate human-readable titles from causing a guessed result. A malformed selector is `invalid_argument`; a well-formed ID that is absent is `not_found`.

The default projection is a compact, cursor-paged story index. It includes:

- the feature identity once;
- owned story identities, lifecycle state, current criterion identities, and every revision/lifecycle head;
- directly related requirements in other features, with each exact relation URN, type, and endpoints that cross the boundary;
- related terms by identity and name;
- related slide and Item identities, exact story/criterion links, relation URNs, and code-reference counts;
- known stale links, readiness gaps, requirement/work-plan conflicts, and explicit completeness bounds.

The compact projection omits story statements, criterion statements, term definitions, citations, full relation records, and code-reference bodies. Use `--expand STORY-ID|URN` to request those for one owned story. Expansion cannot be combined with `--cursor`; selecting a story outside the feature returns `not_found`.

Both projections use the normal `change-saga.ai/v1` envelope, the established read-session snapshot, and the canonical signed cursor contract. A cursor is bound to the operation, normalized filters, and snapshot. Tampering, changing the feature, or replaying it for another operation is `invalid_argument`; replay after a snapshot change is retryable `stale_snapshot`.

The transport-neutral API is `livingapp.Session.Query` with `livingapp.Query{Operation: "context", Filters: livingapp.Filters{Feature: feature, Expand: story}}`. `Result.Data` is a `livingapp.FeatureContextPage`; `Result.Page` carries the same total/returned/next-cursor semantics as the CLI envelope. The existing `requirements` operation uses `Filters.Feature` and returns `Requirement.Feature`.

The response's `data.completeness` is intentionally explicit: the graph is bounded to feature-owned stories and their unique current criteria, linked vocabulary and visuals, and one-hop relations. Revision/lifecycle history, unrelated global prose, and relations beyond one hop are excluded. Conflicted records retain all heads and never receive a fabricated current value.

## Requirement ownership

The existing requirements projection now returns `feature` on every row and accepts `--feature ID|URN`:

```text
change-saga query requirements --saga PATH --feature checkout
```

This filter is exact and snapshot-bound like the context query.

## Representative measurement

The deterministic fixture in `internal/livingapp/feature_context_test.go` contains two owned stories, one cross-feature neighbor, one linked term, one linked slide Item, three relations, and an exact two-line code reference. Its measured JSON data sizes are:

| Read | Calls | Bytes |
| --- | ---: | ---: |
| compact feature context | 1 | 3,624 |
| compact context + one story expansion | 2 | 9,835 |
| requirements + relations + traceability | 3 | 4,178 |

Run `go test -run TestFeatureContextRepresentativeResponseMeasurement -v ./internal/livingapp` to reproduce the measurement. The three-query comparison is a conservative lower bound: it still does not supply term definitions or slide metadata, which require additional existing queries. These are response bytes, not estimated tokens, and no token-savings claim is made.

## Current limits

- Neighboring intent is one relation hop only.
- Only a unique current story revision can contribute current criteria or prose; competing heads remain visible as conflicts.
- Visuals are included when an active relation links a Deck, Slide, or Item to a returned story or criterion. Stale relations remain visible as gaps but do not establish current Item links.
- Compact pagination scopes neighbors, story-linked terms, visuals, gaps, and conflicts to the stories returned on that page; terms that explicitly name the feature remain feature-wide. The response states this in `completeness.ancillary_scope`, so pages do not silently summarize stories they did not return.
- Full history remains available through `query requirement-history`; it is deliberately not duplicated in feature context.
