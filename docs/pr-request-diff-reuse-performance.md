# Request-local PR reference patch reuse

Measured 2026-09-24; implementation commit `a39e147fa2b2fe7c8f068ae32acae417c61001fe`.
The PR page now resolves the source checkout root once and reads one successful
raw patch per effective file path, within one request and resolved review range.
Each reference still runs through the resolver, its own hunk filter, and the
existing diagnostic projection. Empty patches are reusable; failed root lookups
and patch reads retry. No data survives on the application across requests.

## Workload and semantic checks

The supplied `/tmp/csrux-final-f9d80cf` fixture was copied, including `.git`, to
`/tmp/pr-diff-reuse.umga05`. Its `app.saga` remained byte-identical to the source
fixture (`diff -qr`), including existing dirty records. No review actions were
performed on this fixture or real records. The ordinary Go/browser test fixtures
remain isolated and exercise their existing review mutation flows.

Both binaries served the same copied Saga with an explicit `--repo` pointing at
the coordinator's read-only pinned source checkout,
`/tmp/cs-perf-implementation.6NuG4x/source`. Local `main` remained
`6d717b2c9c7489ecf693e1c1202cb317dc9b3a58`; the PR head remained
`2fdf26fec900b08f618f8f8aa5f7db2720ebd469`. The selected
`review-ux-improvements` review contains 140 references across 19 files.

The paired benchmark deeply compares all 140 complete reference views against
a copy of the original uncached implementation before timing. Instrumented calls
verify one successful root lookup, 19 successful patch reads, and 140 resolver
calls. Unit tests additionally cover distinct exact ranges, stale digest fallback,
line movement, rename, deletion, whole-file and binary references, missing
objects, literal pathspec characters, nested checkouts, new ranges/repositories,
independent output slices, successful empty patches, and recovery after failed
root lookups or patch reads.

## Reference-stage results

Apple M3 Pro, macOS arm64, Go 1.26.1; shared development host. Three runs of
three operations each, with resolver creation included and fixture loading and
semantic comparison outside timing. These are diagnostic samples, not CI budgets.
Other workspaces were active; no percentile claim is supported.

| Implementation | Run means, ms/op | Median ms/op | Allocated MB/op (median) |
| --- | --- | ---: | ---: |
| Original uncached path | 2363.169, 2384.565, 2280.989 | 2363.169 | 50.196 |
| Production request helper | 252.147, 231.871, 272.834 | 252.147 | 25.532 |

The measured stage is **9.37× faster**, saving approximately 2.11 seconds per
operation. Allocations are decimal bytes per operation, not retained heap.
The separate full-page stage harness now constructs one helper per operation;
its standalone one-reference compatibility wrapper must not be used to measure
request reuse.

## Full HTTP response results

The original and optimized binaries each served one first request, followed by
five warm requests, alternating before/after sequentially. No other one-shot
from this workspace ran during measurement; other workspaces remained active.
Process cold means a fresh server process with warm OS/Git caches, not a cold
machine. This measures server HTTP completion, not browser first-usable time.

Every response was **3,533,094 bytes**. The entire HTML was byte-identical after
replacing only each process's random mutation token. The normalized response
SHA-256 was `2f284287d338b82b18ac0c40245e594887012f146a30f4af7bfded5b93eadbed`.
This includes all diff rows, exact locations, notes, ordering, reports, unresolved
records, and existing review discussion; no HTML sections were omitted from the
comparison.

| Request | Before total ms | After total ms | Before TTFB ms | After TTFB ms |
| --- | ---: | ---: | ---: | ---: |
| First | 4329.587 | 910.037 | 4328.276 | 908.861 |
| Warm 1 | 3539.187 | 1203.433 | 3537.500 | 1201.775 |
| Warm 2 | 3373.677 | 1072.808 | 3371.087 | 1071.425 |
| Warm 3 | 3805.934 | 848.806 | 3804.481 | 845.802 |
| Warm 4 | 3299.214 | 1303.979 | 3298.002 | 1300.410 |
| Warm 5 | 3439.007 | 828.760 | 3437.549 | 827.857 |

Warm total HTTP median fell from **3,439.007 ms to 1,072.808 ms** (3.21× faster,
68.8% reduction). Observed ranges were 3,299.214–3,805.934 ms before and
828.760–1,303.979 ms after. First requests were 4,329.587 ms and 910.037 ms;
one sample per process is insufficient for a cold-start distribution. The
large HTML payload is deliberately unchanged by this optimization.

## Validation and limitations

- `go test -race ./internal/server -run '^TestReview' -count=1`: passed, 8.205s.
- Final `go test -race ./internal/server -run '^TestReviewDiffs' -count=1`:
  passed, 4.557s, including root and patch recovery.
- `go vet ./internal/server ./internal/coderesolve ./internal/gitdiff ./internal/reviewstate`:
  passed.
- Offline `npm ci --offline --prefix e2e`, followed by
  `npm test --prefix e2e -- --workers=2 tests/pull-request-review.spec.ts`:
  all five Chromium PR-review tests passed, 22.9s.
- Separate `go test ./internal/coderesolve ./internal/gitdiff ./internal/reviewstate -count=1`:
  passed (0.939s and 3.414s; `reviewstate` has no test files).
- Production binary build, formatting, `git diff --check`, and documentation
  link check: passed.
- `hivecontrol exec oneshot 5m -- go test ./internal/server ./internal/coderesolve ./internal/gitdiff ./internal/reviewstate -count=1`
  **timed out at 300 seconds**, exit 124, before any package completion output.
  The full suite is incomplete, not passed. The tracked Go process and its
  `server.test` child were confirmed gone after timeout. No unrelated code was
  changed to accommodate this limit.

The complete repository/race suite and the Firefox/WebKit/browser-wide suites
were not completed. Successful request reuse does not provide cross-request
snapshot consistency or reduce the existing hidden Item markup payload. Exact
source/currency semantics, unresolved records, append-only attributed review
history, and the distinction between slide approval and Item evidence are
unchanged.

## Reproduction

Build the before binary from `7f60cfc8b8351c1df1cdc3187bd3043fd7aae2a4` and the
after binary from the implementation commit above, each via a bounded
`hivecontrol exec oneshot 3m -- go build ... ./cmd/change-saga`. Copy the supplied
fixture including its `.git`, and use a source checkout with the recorded refs.
Never serve with the Saga-only fixture as the source checkout.

```sh
PR_PERF_SAGA=/absolute/disposable/app.saga \
PR_PERF_REPO=/absolute/pinned/source \
PR_PERF_REVIEW=review-ux-improvements \
hivecontrol exec oneshot 3m -- go test ./internal/server -run '^$' \
  -bench '^BenchmarkReviewDiffs$' -benchmem -benchtime=3x -count=3

hivecontrol exec service -- /absolute/before-binary serve \
  --addr 127.0.0.1:0 --repo /absolute/pinned/source /absolute/disposable/app.saga
hivecontrol exec service -- /absolute/after-binary serve \
  --addr 127.0.0.1:0 --repo /absolute/pinned/source /absolute/disposable/app.saga
```

Use only the printed owned URLs, alternate before/after HTTP GETs, and compare
whole response bytes after replacing each response's `data-review-token` value
with the same placeholder. Stop only the service IDs created for this run.
Raw local samples, response bodies, measurement script, and the exact bounded
suite failure are retained temporarily under `/tmp/pr-diff-reuse.umga05`.
