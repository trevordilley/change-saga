# Reading a Saga through the query API

`change-saga query` is the versioned read API. Use it for every read of an
existing Saga; never glob, grep, or read its metadata files. It is
deterministic, paginated, and safe to call concurrently; it never starts a
server and never mutates either repository.

## The envelope

Pass `--saga <path>` to every query, and `--repo <source-checkout>` when the
source repository is separate. Pass `--against REV` (and optionally `--head
REV`) to read one comparison; without it a query observes the head. The one
exception is `change-saga query schema <operation>`, which describes that
operation's data paths and pagination contract without opening a Saga. Use it
instead of probing or guessing response shapes.

Every invocation writes exactly one JSON envelope carrying `schema`, `ok`,
`snapshot`, `data`, and `page`; failures carry `error.code`. Branch on `ok` and
`error.code`; never parse message text. Each cursor schema names the response
collection counted by `page.total` and `page.returned` as
`pagination.counted_path`: the current page length at that path must equal
`page.returned`. Follow `page.next_cursor` while `page.has_more` is true, and
confirm the aggregate count equals `page.total`. Do not raise `--limit` to
silently swallow a partial result. Compare `snapshot` across calls to detect a
Saga that changed underneath a multi-step read.

## Navigating

Start at `query overview` and walk one level at a time with `query children`.
Read slide content with `query slide` and Item evidence with `query
slide-diffs`; read narrative content through `query fragment`. A fragment's
children are its landmarks, and each one reports the target URN to pass to
`change-saga cover --target`. Navigate evidence in both directions with `query
fragment-diffs` and `query diff-owners`, and page completeness problems with
`query gaps --kind uncovered|stale|overlap --against <base>`.

Hierarchy nodes report inclusive `diffs.current` and `diffs.stale` totals plus
`direct_current`, `direct_stale`, `descendant_current`, and
`descendant_stale`, so evidence owned by a landmark or child is not mistaken
for a node with no explained code.

## Operations

<!-- query-operations:begin -->
<!-- query-operations:end -->
