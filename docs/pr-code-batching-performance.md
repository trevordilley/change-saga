# PR Code Diff: measured page batching

Measured 2026-09-24. Baseline application: `7f60cfc8b8351c1df1cdc3187bd3043fd7aae2a4`
(the diagnostic-only commit after `2fdf26f`). Implementation: `1c3cc6977fa5b23702fcbb84a89161efb25fd8dd`.
This is the smaller, reviewed alternative to a cross-request selected-file
cache. The PR code surface explicitly requests the existing **200-row** limit.
The endpoint's default **50** and maximum **200** are unchanged, as are other
comparison surfaces. Every request still loads and validates the Saga, resolves
the current review range, reads the catalog, and builds the selected-file model.
No Saga, feedback, currency, HTML, or selected-file generation is retained.

On the realistic fixture, **573 rows take three sequential requests instead of
12**. The full-traversal Go benchmark drops from 2.16 s to 0.62 s (71%), and
browser navigation to all hunks drops from 6.58 s to 4.66 s (29%). First-row
latency did not improve. The remaining initial PR HTML work dominates that
measurement; this change does not optimize it.

## Workload and method

- Apple M3 Pro, Darwin arm64; Go 1.26.1, Node 22.22.3,
  Chromium 151.0.7922.34 at 1440×1000, loopback without throttling.
- Same realistic workload as [the original evaluation](pr-review-loading-performance.md):
  `review-ux-improvements`, selected `internal/server/reviews.go`, 573 display
  rows. Source merge-base `6d717b2c9c7489ecf693e1c1202cb317dc9b3a58` through
  head `2fdf26fec900b08f618f8f8aa5f7db2720ebd469`.
- Copied `/tmp/csrux-final-f9d80cf`, including `.git` and dirty/untracked
  records, into the owned `/tmp/pr-code-page.GwmeAB`. Used the locally built
  public CLI for validation, overview, and review-list reads. The installed
  CLI has older review syntax. No records were authored or reviewed.
- Both owned servers used `--repo` pointing at the source worktree and
  `127.0.0.1:0` (ports 64083 and 64886). Both were stopped after measurement.
  Builds, benchmark loops, browser runs, and test suites used tracked
  `hivecontrol exec oneshot`; servers used `hivecontrol exec service`.
- Go: three runs of three operations, shared process/filesystem caches;
  setup outside timed loops. Each full-traversal operation follows all cursors.
- Browser: five new contexts on each server, using the existing read-only
  profiling harness. Every baseline sample fetched 12 pages; every modified
  sample fetched three, all with `limit=200`. Every sample finished with 573
  rows and no page errors. These are warm-host diagnostic samples, not cold
  disk measurements or percentile budgets. Shared-host scheduling creates
  substantial noise; other workspaces were active.

## Measurements

Go values are milliseconds per operation (three per-run means):

| Stage | Run 1 | Run 2 | Run 3 | Median |
| --- | ---: | ---: | ---: | ---: |
| Proposed cache-key probes | 12.54 | 12.48 | 8.07 | 12.48 |
| Uncached selected-file diff and model | 18.76 | 18.62 | 37.02 | 18.76 |
| All hunks, limit 50 | 1823.08 | 2160.81 | 2326.17 | 2160.81 |
| All hunks, limit 200 | 682.30 | 477.78 | 617.29 | 617.29 |

Median allocated bytes per traversal were approximately **248.5 MB** at 50
rows versus **63.6 MB** at 200 rows; these are allocations, not retained heap.
The rejected cache-key experiment probes canonical checkout/Git/object-store
paths, effective config, selected old/new path attributes, replace refs, and
environment/range/catalog/target identity. Even before publication rechecks and
retention management, it costs much of the file work it could save. The helper
is benchmark-only and is not a claim of production-safe cache invalidation.

Browser samples, milliseconds from navigation start:

| Sample | 50: first row | 50: all hunks | 200: first row | 200: all hunks |
| --- | ---: | ---: | ---: | ---: |
| 1 | 4273.5 | 6582.4 | 4074.1 | 4080.5 |
| 2 | 3717.2 | 5512.2 | 4736.4 | 4929.6 |
| 3 | 3991.8 | 5783.4 | 4471.6 | 4655.4 |
| 4 | 4075.0 | 8428.8 | 5379.8 | 5384.5 |
| 5 | 6145.5 | 8463.4 | 4372.4 | 4537.0 |
| Median | 4075.0 | 6582.4 | 4471.6 | 4655.4 |

First-row observation includes browser automation/polling; several pages may
already have arrived when it observes a row. No first-row improvement is claimed.

A separate HTTP-only traversal control (five runs, milliseconds) gave:

| Limit | Samples | Median |
| --- | --- | ---: |
| 50 | 2000.96, 2044.98, 1853.04, 2065.02, 2136.96 | 2044.98 |
| 200 | 775.42, 451.82, 422.30, 500.47, 559.48 | 500.47 |

These HTTP controls exclude initial PR HTML, browser rendering, and code
highlighting. They are not the original 8.64-second browser baseline.

## Reproduction

Keep the followed source refs at the exact OIDs above for a like-for-like run.
Use an owned fixture copy and source worktree; do not serve against the
Saga-only fixture repository.

```sh
hivecontrol exec oneshot 3m -- go build -o /tmp/pr-code-batching ./cmd/change-saga
batch_fixture=$(mktemp -d /tmp/pr-code-batching.XXXXXX)
cp -R /tmp/csrux-final-f9d80cf/. "$batch_fixture/"
/tmp/pr-code-batching validate --json "$batch_fixture/app.saga"
/tmp/pr-code-batching query overview --saga "$batch_fixture/app.saga" --repo "$PWD"
export PR_PERF_SAGA="$batch_fixture/app.saga"
export PR_PERF_REPO="$PWD"
export PR_PERF_REVIEW=review-ux-improvements
hivecontrol exec oneshot 3m -- go test ./internal/server -run '^$' -bench '^BenchmarkPRCodePages$' -benchmem -benchtime=3x -count=3
hivecontrol exec service -- /tmp/pr-code-batching serve --addr 127.0.0.1:0 --repo "$PWD" "$batch_fixture/app.saga"
```

Use the printed owned port. The existing harness needs the locally installed
Playwright dependency and Chromium browser cache.

```sh
export PR_PERF_URL=http://127.0.0.1:PORT/reviews/review-ux-improvements
export PR_PERF_FILE=internal/server/reviews.go
export PR_PERF_OUTPUT=/tmp/pr-code-batching-browser.json
hivecontrol exec oneshot 2m -- node e2e/profiling/pr-loading.mjs
```

For the HTTP control, GET `/reviews/review-ux-improvements/file-diff` with
`file=internal/server/reviews.go` and explicit `limit=50` or `limit=200`.
Follow `X-Change-Saga-Next-Cursor` until absent, retaining the same file and
limit. Sum `X-Change-Saga-Returned` and verify it equals
`X-Change-Saga-Total`. Repeat five times. The checked-in benchmark also exercises
both complete HTTP handler traversals, without loopback transport overhead.

Raw local browser logs are `/tmp/pr-code-page-browser50.json` and
`/tmp/pr-code-page-browser200.json`; the HTTP-200 log is
`/tmp/pr-code-page-http200.json`. Their lifetime is temporary; the complete
latency samples are preserved above.

## Verification and limits

The focused regression checks the generated limit and encoded literal path,
byte-identical concatenated row/action markup across 50/200-row traversals,
573 rows across 12/3 pages, default 50/hard maximum 200, and unchanged ordinary
comparison URLs. A malformed Saga edit of unchanged size with restored mtime
fails validation on the next page, repairs retry immediately, and a changed
source head rejects the old cursor. No freshness check was bypassed.

The focused regression, `go vet ./...`, build, formatting, and diff checks pass.
Full Chromium: **44/45 pass**, including all PR, bounded-surface, performance,
and navigation tests. The unrelated failure is
`section-directories.spec.ts:32`: its terms table assertion expects five
headers, but the existing directory renders seven, including Definition
maturity and Implementation evidence. Both extra headers exist in the baseline
source; this change does not touch terms rendering or that expectation.

The full `go test -race ./...` run reached its tracked **eight-minute bound**
(exit 124) while the server package was still active. Packages reported through
`internal/semanticgraph`, including CLI, gitdiff, reviewapp, reviewstore, and
Saga, had passed; no race failure was reported before the timeout. The full
race result is **incomplete**, and was not repeated. No Firefox/WebKit run was
made. Cross-request reuse remains deferred: this change reduces how
many times existing work runs, without solving its per-page cost.
