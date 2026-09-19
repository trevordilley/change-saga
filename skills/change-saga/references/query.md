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
- `schema`: the response paths and pagination contract for a query operation; no saga is required.
  `change-saga query schema <operation>`
- `overview`: saga identity, source comparison, coverage summary, and the top of the hierarchy.
  `change-saga query overview --saga PATH [--repo PATH] [--against REV [--head REV]]`
- `children`: one level of children under a target; a fragment's children are its landmarks.
  `change-saga query children --saga PATH --parent TARGET [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]`
- `fragment`: bounded fragment content by byte range, without reading files directly.
  `change-saga query fragment --saga PATH --target FRAGMENT [--offset N] [--limit N] [--repo PATH] [--against REV [--head REV]]`
- `fragment-diffs`: the changed atoms a saga, chapter, section, fragment, or landmark references, and its stale references.
  `change-saga query fragment-diffs --saga PATH --target TARGET [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]`
- `slide`: bounded visual slide content and its ordered semantic Items.
  `change-saga query slide --saga PATH --target SLIDE [--offset N] [--limit N] [--repo PATH] [--against REV [--head REV]]`
- `slide-diffs`: the changed atoms a slide Item references.
  `change-saga query slide-diffs --saga PATH --target ITEM [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]`
- `diff-owners`: in a comparison (--against), the narrative targets whose code references hold the changed lines or file at a code location, and the terms whose code contains each line; for any line, use traceability --ref.
  `change-saga query diff-owners --saga PATH --ref LOCATION [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]`
- `gaps`: uncovered atoms, stale selectors, and overlapping coverage.
  `change-saga query gaps --saga PATH [--kind uncovered|stale|overlap] [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]`
- `mappings`: coverage records ranked by breadth and justification signals so scrutiny starts at the weakest mappings.
  `change-saga query mappings --saga PATH [--target TARGET] [--sort scrutiny|target|path] [--minimum-score N] [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]`
- `claims`: falsifiable author assertions, exact evidence, current mapping state, and latest verification result.
  `change-saga query claims --saga PATH [--target TARGET] [--status unverified|verified|failed|inconclusive] [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]`
- `verifications`: append-only verification history for author claims.
  `change-saga query verifications --saga PATH [--claim ID] [--status unverified|verified|failed|inconclusive] [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]`
- `requirements`: current requirement definitions and lifecycle heads without fabricating winners for conflicts.
  `change-saga query requirements --saga PATH [--requirement ID|URN] [--state STATE] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]`
- `requirement-history`: append-only revision and lifecycle history in deterministic graph order.
  `change-saga query requirement-history --saga PATH --requirement ID|URN [--cursor TOKEN] [--limit N] [--against REV [--head REV]]`
- `citations`: immutable requirement provenance records.
  `change-saga query citations --saga PATH [--citation ID|URN] [--requirement ID|URN] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]`
- `relations`: typed current, stale, and superseded living-Saga relations.
  `change-saga query relations --saga PATH [--relation ID|URN] [--type TYPE] [--from URN] [--to URN] [--state STATE] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]`
- `waves`: ordered work-plan coordination cohorts and derived item counts.
  `change-saga query waves --saga PATH [--wave ID|URN] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]`
- `work-items`: current work-item definitions, progress, explicit dependency blockers, workspaces, and merge evidence.
  `change-saga query work-items --saga PATH [--item ID|URN] [--wave ID|URN] [--status STATE] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]`
- `work-events`: normalized append-only progress, workspace, merge, and contract events.
  `change-saga query work-events --saga PATH [--item ID|URN] [--kind KIND] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]`
- `work-conflicts`: deterministically identified work-plan conflicts and competing heads.
  `change-saga query work-conflicts --saga PATH [--item ID|URN] [--wave ID|URN] [--kind KIND] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]`
- `traceability`: current story-to-design/work/review/code/test paths (design that addresses the whole story is also listed as broad), reverse lookup by a code location at any revision (remapped as staleness is) or by pinned commit, and transitive blockers.
  `change-saga query traceability --saga PATH [--requirement ID|URN] [--criterion ID|URN] [--ref LOCATION | --commit OID] [--cursor TOKEN] [--limit N] [--repo PATH] [--against REV [--head REV]]`
- `readiness`: independent requirement, plan, and delivery coverage axes; only immutable delivery evidence gates peer-review readiness.
  `change-saga query readiness --saga PATH [--requirement ID|URN] [--status ready|blocked] [--cursor TOKEN] [--limit N] [--against REV [--head REV]]`
- `layers`: one comparison's Changed records (each with before and after), Affected records (with why), and Code (hunks grouped under the records that reference them, plus unreferenced lines).
  `change-saga query layers --saga PATH --against REV [--head REV] [--layer changed|affected|code] [--repo PATH]`
- `history`: when a record was introduced, what it replaced, and every commit that changed it, each with the command that opens that comparison.
  `change-saga query history --saga PATH --node URN`
- `terms`: the project's vocabulary: each term's definition, aliases, stories, records, and code health at the head; filter by term, by story, or by a code location at any commit to find the terms a line of code defines.
  `change-saga query terms --saga PATH [--term ID|URN] [--story ID|URN] [--ref LOCATION] [--repo PATH] [--against REV [--head REV]]`
<!-- query-operations:end -->
