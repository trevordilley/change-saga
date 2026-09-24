# PR review loading: measured baseline and proposed first slice

Measured 2026-09-24 at **2fdf26fec900b08f618f8f8aa5f7db2720ebd469**.
This is an evaluation, with isolated diagnostic harnesses. No production
optimization or review decision was made.

Opening a meaningful PR spends most of its time constructing **every Item's
code diff before returning HTML**. The recommended first slice is to resolve
the repository root once and reuse each file patch within that request. A
benchmark-only prototype produces exactly the same 140 reference views and
reduces that stage from **2,136 ms to 220 ms**. This is a measured component
improvement, **not an implemented end-to-end improvement**.

## Workload and environment

- Apple M3 Pro, 36 GiB RAM, macOS Darwin 24.6.0 arm64; Go 1.26.1,
  Node 22.22.3; headless Chromium 151.0.7922.34 at 1440×1000. Native Chrome
  was also used to inspect all three tabs, the drawer, and slide navigation.
- Local loopback, no network/CPU throttling. Shared development host, with
  visible scheduling noise; runs are diagnostic rather than CI budgets.
  Our CPU-heavy loops used bounded `hivecontrol exec oneshot` jobs, sequentially.
- The provided `/tmp/csrux-final-f9d80cf` fixture was copied, **including its Git
  repository and existing dirty/untracked records**, to `/tmp/cs-pr-perf.cOw7AY`.
  The Saga contains **1,656 files / 1,215,165 logical bytes**, excluding `.git`.
  Public `query overview` reports 13 living/onboarding decks with 71 slides
  and eight chapters. This is the application's realistic documentation,
  not a minimal test fixture.
- The selected `review-ux-improvements` PR has **four slides, 12 Items,
  140 exact code references, 19 changed files and 1,234 changed atoms**.
  Ten Items link to story records, two to features; seven Items own code.
  One existing open annotation thread and no slide decisions are present.
  The initial Item has 18 references. No annotation capability claim is
  inferred from authored fixture prose.
- Review range: merge-base **6d717b2c9c7489ecf693e1c1202cb317dc9b3a58** through
  **2fdf26fec900b08f618f8f8aa5f7db2720ebd469**; manifest follows `main` and
  `feature/review-ux-improvements`. Both refs were checked during measurement.
  Review coverage reports 1,234/1,234, zero stale/overlap. The separate living
  documentation overview reports 65 stale mappings; these were preserved.
- Services used `--repo` pointing at this source worktree. Serving against
  the fixture's Saga-only Git repository would give misleading deleted code.
  Owned ephemeral ports were 58051 and 58707. The parent's preview was untouched.
- `diff -qr` confirmed the copied Saga remained identical to the supplied
  fixture after measurement. Existing dirty records were neither repaired nor
  committed. `query slide` does not resolve these review-slide URNs in this
  build; review inventory came from public `review list --json` and rendered
  DOM, with ordinary server loaders exercised by the stage benchmarks.

## Definitions and samples

“Process cold” means a new server process and fresh application caches, with
OS filesystem/Git caches left alone. It does **not** mean a cold disk or boot.
One process startup/first HTTP request was measured. “Fresh app” benchmarks
reset application caches per operation but share the compiled template and
process. “Warm HTTP” reuses the server after its first PR request.

Go baseline: three runs of three operations per stage; table gives the median
of the three per-run means and their minimum–maximum. Setup, source loading
for stage selection, and prototype equality checks are outside timed loops.
Browser: five new contexts on a warm server, then five same-context reloads.
First usable means the active slide's SVG is loaded and visible and its Item
code control is visible. Action times include automation and two animation
frames after the observable result; they are not pure handler CPU durations.
An earlier run waiting only for the control was discarded for usability timing.
No concurrent profiling job from this workspace ran during a timed browser run.

Small samples do not support a meaningful p95/p99; report median and observed
range. [Machine-readable samples](../e2e/profiling/pr-loading-baseline.json)
retain the individual HTTP/browser timings. All URLs were local.

## End-to-end baseline

| Surface | Samples | Median ms | Observed range ms |
| --- | ---: | ---: | ---: |
| Process start to listener announcement | 1 | 57.8 | — |
| First PR HTTP on that process | 1 | 2,855.5 | — |
| Subsequent PR HTTP, with Git Trace2 enabled | 5 | 3,426.6 | 2,530.7–3,756.7 |
| Browser document TTFB | 5 | 2,730.2 | 2,384.7–3,206.8 |
| Browser first contentful paint | 5 | 2,796.0 | 2,452.0–3,272.0 |
| Browser DOM interactive | 5 | 2,806.6 | 2,463.9–3,280.1 |
| Browser first usable slide | 5 | 2,936.6 | 2,620.2–3,366.7 |
| Browser load event | 5 | 3,083.1 | 2,773.0–3,555.2 |
| Reload to usable slide | 5 | 2,910.3 | 2,650.6–3,863.0 |
| Next slide | 5 | 47.6 | 43.7–56.8 |
| Previous slide | 5 | 39.9 | 31.6–49.8 |
| First Item → code drawer | 5 | 71.8 | 66.6–76.9 |
| Code Diff tab, first selected file | 5 | 309.2 | 307.1–820.6 |
| Coverage tab | 5 | 307.4 | 306.8–316.0 |
| Return to Code Diff | 5 | 32.4 | 31.6–33.5 |
| Return to Deck | 5 | 41.9 | 33.1–42.9 |

The code-tab boundary is the catalog and first diff row for the default
`CHANGELOG.md`; it must not be presented as completion of a large file.
Slide navigation, Item drawer, repeated Code Diff, and return to Deck each
made **zero new requests**. First Code Diff made two requests; Coverage one.
The native-browser inspection confirmed the known Code Diff layout defect;
fixing that remains the parent's work.

A separate five-context **Code Diff deep-link** run selected
`internal/server/reviews.go`, a substantive file with **573 display rows**:

| Surface from navigation start | Median ms | Observed range ms |
| --- | ---: | ---: |
| First diff row | 5,250.1 | 3,962.1–5,820.6 |
| Every hunk loaded | 8,644.4 | 7,558.8–10,039.7 |

Every sample fetched **12 sequential file-diff pages**, at the default 50-row
limit. Those requests alone totaled 2,669–6,211 ms per sample. Deep navigation
also rebuilds the original PR HTML first (TTFB 2,990–4,438 ms in this run).
This larger file exposes a separate repeated-page cost; neither tab-switch
speed nor the small first file represents the whole Code Diff experience.

## Server stages and profiles

| Stage | Median ms/op | Run means min–max ms | Allocated MB/op |
| --- | ---: | ---: | ---: |
| PR page, fresh app | 2,738.5 | 2,680.1–2,822.4 | 217.91 |
| PR page, warm app | 2,658.8 | 2,552.4–2,681.9 | 206.65 |
| Saga load and validation | 52.1 | 48.1–57.4 | 19.12 |
| Review report: range, coverage, currency, attribution | 136.8 | 127.2–147.3 | 10.82 |
| All Item reference diffs | 2,136.0 | 2,086.2–2,363.6 | 50.18 |
| Warm shell projection | 258.6 | 254.7–276.7 | 75.40 |
| Generic shell-template control | 17.7 | 17.6–18.7 | 5.53 |
| Request-local patch reuse experiment | 219.5 | 217.3–226.7 | 25.52 |

MB means decimal allocated bytes, **not retained heap or RSS**. Stages are
independent measurements, so do not add their medians as an exact accounting.
The generic shell-template control omits the PR body; it is not a measurement
of complete PR rendering. The full-page benchmarks include actual PR rendering.

A five-operation warm-page CPU/allocation profile (plus untimed setup/warmup)
covered 18.15 seconds and sampled 4.80 seconds of Go CPU. Child Git CPU is not
included. Thus CPU flame graphs alone would understate the main latency source.
`referenceDiff` had 0.66 s cumulative sampled CPU, while its measured wall stage
is over two seconds **per operation**. The shell had 1.36 s cumulative CPU;
`requirements.newCurrencyHeads` accounted for 0.85 s (17.7% of total CPU) and
23.0% of sampled cumulative allocation space. Template execution together
accounted for 0.29 s (6.0% of CPU). This supports investigating repeated currency
projection after the Git loop; it does not justify bypassing currency checks.

Git Trace2 recorded **299 Git processes for first PR HTTP, 298 on every warm
request**. Of the warm processes, 143 resolve the repository root; **140 run
file patches for only 19 distinct paths**. `reviews.go` alone is diffed 46
times, `saga/review.go` 15 times. The production `referenceDiff` resolves the
root and calls `gitdiff.FileDiff` once per reference. The report and Item
rendering also create separate code resolvers. Cat-file batch processes live
across other commands: summing all Git `t_abs` values double-counts overlapping
lifetimes and is deliberately **not** reported as Git wall-time share.
The stage benchmark, process counts, and identical-output reuse experiment
jointly locate the avoidable work.

Sequential HTTP endpoint controls (five requests each, Trace2 enabled):

| Endpoint | Median ms (range) | Body bytes | Git processes/request |
| --- | ---: | ---: | ---: |
| First slide visual | 52.9 (51.5–62.4) | 4,348 | 0 |
| PR code catalog | 217.3 (125.9–348.3) | 15,600 | 7 |
| PR coverage | 329.1 (214.4–424.7) | 641 | 13 |
| `reviews.go` first file-diff page | 153.0 (120.6–225.5) | 14,874 | 10 |

`reviewVisual` reloads and validates the entire Saga for each small asset.
The browser observed multiple stage/thumbnail requests per slide, including
reloads during setup, with concurrent visual request durations much greater
than the isolated 53 ms control. This is real secondary work, not a reason to
silently skip validation. `reviewFileDiffSurface` reloads the document, resolves
the range, reads the catalog, reads the entire selected file comparison and
builds its rows **before slicing each cursor page**. Pagination currently bounds
the response, not the repeated generation work.

## Payload and browser work

The PR response is **3,533,094 bytes**, without content compression. Item drawer
templates account for **3,198,677 bytes (90.5%) and 12,566 diff rows**. The live
DOM reports only 1,447 elements: template contents are separate document
fragments, so that number alone hides most parsed markup. Repeated references
can include the same hunk, and the existing changed-reference fallback can
include every change to a file. Preserve those semantics when reducing work.

`app.js` is 129,464 bytes; `theme.js` 722 bytes. The four distinct slide visual
responses are approximately 4.3–5.6 KB each. There were no external assets.
Median document transfer after TTFB was 11.4 ms, and response-end to
DOM-interactive was 61.9 ms. Chromium's median cumulative layout time was
38.5 ms, style recalculation 5.1 ms, script execution 7.9 ms; no main-frame
long tasks were observed in the corrected five-sample baseline. These CDP
metrics include automation-related work and do not isolate every iframe's CPU.
They support **server-first prioritization**, not a claim that browser work is
free. Loopback transfer also says little about slower machines or remote links.

## Ranked proposals and correctness boundaries

1. **First slice: request-local repository root and raw file-patch reuse.**
   Keep Saga load/validation, range resolution, code-reference resolution,
   exact per-reference hunk filtering, notes, ordering and review projection.
   Cache the raw patch only after resolving each reference's effective path.
   The benchmark prototype compares the complete list of diff views deeply
   against production before timing. On this fixture it saves approximately
   **1.92 s in the reference stage (9.7× faster)**, allocating about 25 MB less.
   An end-to-end reduction of roughly **1.5–2 s** is a reasonable hypothesis;
   subtracting stage medians predicts about 0.74 s server time, but no complete
   optimized endpoint has been measured. Host noise, remaining shell work,
   assets and full-file pagination limit confidence. Expected Git count is
   about 38 rather than 298 if only those redundant calls are removed.
2. **Then bound first-page Item payload.** Fetch an Item's exact diff panel on
   demand, with explicit pending/error/retry UI and pinned source/Saga identity.
   This can remove much of the 3.20 MB hidden template payload and unnecessary
   first-load work. It moves work to drawer opening, so benchmark both first
   drawer and repeat access. Preserve all unresolved/stale records and exact
   line identities. Do not change deck navigation or slide-only approval.
3. **Share immutable selected-file work across Code Diff cursor pages.** The
   573-row example pays the full loader/catalog/file transformation 12 times.
   Reuse a bounded generation for one repository/range/path, with cursor
   identity and invalidation below. Keep the 50/200 response bounds and bound
   retained bytes/eviction as well. No speedup is claimed without an experiment.
4. **Investigate request-generation reuse for visual validation and shell
   relation currency.** About 52 ms per full load plus 259 ms warm shell work
   remains. Prefer one validated snapshot and current review overlay with a
   freshness check over TTLs. Profile graph/requirements stages separately
   before changing them. Avoid broad persistent HTML caching as a first step.
5. Compression or JS tuning is lower priority on measured loopback behavior.
   Measure slower-device/network conditions before adding complexity; keep
   dependencies local and offline.

| Changing input | Request-local first slice | Required for any later cross-request reuse |
| --- | --- | --- |
| HEAD/base/followed branches, frozen/landed fallback | Resolve once with existing report on every request; patches use exact OIDs | Resolve refs before reuse; include effective base/head OIDs and frozen/fallback identity; reject old cursors |
| Dirty/new/deleted Saga records and conflicts | Existing full load/validation remains; no prior-request cache | Content-sensitive validated generation including untracked files, conflict heads, selectors, relations and assets; atomic publication/recheck |
| Review comments, annotations, decisions, withdrawal, attribution | Re-read/project through existing path; never cached across requests | Separate fresh append-only feedback generation; immediate own-mutation invalidation plus external-write detection; keep human/AI identity and unknown currency |
| Repository/worktree identity | Request belongs to one source checkout/root | Key canonical checkout and Git object-store/worktree identity; never share by relative path or commit alone |
| Git config, attributes, environment and diff options | Reuse only within this request under identical canonical arguments | Include effective inputs or conservatively invalidate; preserve binary, rename and submodule behavior |
| Source file edits / WORKTREE | This review resolves immutable commit OIDs; no claim about WORKTREE | Content-aware freshness or no cache for WORKTREE; changed files must not leave stale exact refs |
| Missing objects, read errors, malformed records | Preserve current diagnostics; prototype deliberately fails on patch errors | Do not turn errors into permanent negative cache hits or suppress unresolved records; retries must be able to recover |

The prototype is intentionally not a production helper: its fatal-on-error
behavior needs replacement with the existing per-reference error view. It
proves output equality only for this fixture. Concurrent edits during a single
request still require an explicit consistency policy; request-local reuse does
not solve that pre-existing snapshot boundary.

## Regression strategy and reproduction

Before implementing slice 1, add deterministic assertions for **one root lookup
and one file patch per unique resolved path**, plus before/after full response
semantics. Exercise duplicate references with different exact ranges, stale
fallback, pure line movement, rename, deletion, whole-file/binary refs, missing
objects, literal pathspec characters, nested checkout roots, and multiple repos.
Test changed HEAD/base and external dirty Saga edits between successive requests;
new comments/annotations and human/AI slide decisions must appear immediately,
with current/out-of-date/unknown currency unchanged. Coverage is an omission
check; no result is an inferred approval.

For subsequent lazy/cached slices, add mutation/concurrent-reader invalidation,
failed-request retry, bounded retained heap, cursor mismatch, source/config/file
change and cache eviction checks. Re-run this realistic fixture, not just tiny
unit fixtures. Keep the existing parent-owned annotation validation/retry and
full Code Diff layout changes separate.

Run from the source worktree (replace absolute paths deliberately):

```sh
hivecontrol exec oneshot 3m -- go build -o /tmp/change-saga-pr-perf ./cmd/change-saga
perf_fixture=$(mktemp -d /tmp/change-saga-pr-perf.XXXXXX)
cp -R /tmp/csrux-final-f9d80cf/. "$perf_fixture/"
/tmp/change-saga-pr-perf query overview --saga "$perf_fixture/app.saga" --repo "$PWD"
/tmp/change-saga-pr-perf review list --json --review review-ux-improvements --repo "$PWD" "$perf_fixture/app.saga"
export PR_PERF_SAGA="$perf_fixture/app.saga"
export PR_PERF_REPO="$PWD"
export PR_PERF_REVIEW=review-ux-improvements
hivecontrol exec oneshot 5m -- go test ./internal/server -run '^$' -bench '^BenchmarkPRLoading$' -benchmem -benchtime=3x -count=3
hivecontrol exec oneshot 3m -- go test ./internal/server -run '^$' -bench '^BenchmarkPRReferenceMemoExperiment$' -benchmem -benchtime=3x -count=3
hivecontrol exec oneshot 2m -- go test ./internal/server -run '^$' -bench '^BenchmarkPRLoading/warm_page$' -benchtime=5x -count=1 -cpuprofile=/tmp/pr-cpu.out -memprofile=/tmp/pr-mem.out -o /tmp/pr-server.test
go tool pprof -top /tmp/pr-server.test /tmp/pr-cpu.out
go tool pprof -top -alloc_space /tmp/pr-server.test /tmp/pr-mem.out
hivecontrol exec service -- /tmp/change-saga-pr-perf serve --addr 127.0.0.1:0 --repo "$PWD" "$perf_fixture/app.saga"
```

Use the printed owned URL; port zero avoids claiming another service's port.
The fixture's branch refs must still resolve to the recorded OIDs for a
like-for-like comparison. A later parent commit changes this workload.

```sh
hivecontrol exec oneshot 3m -- npm ci --offline --prefix e2e
export PR_PERF_URL=http://127.0.0.1:PORT/reviews/review-ux-improvements
export PR_PERF_OUTPUT=/tmp/pr-browser.json
hivecontrol exec oneshot 3m -- node e2e/profiling/pr-loading.mjs
PR_PERF_FILE=internal/server/reviews.go PR_PERF_OUTPUT=/tmp/pr-file.json hivecontrol exec oneshot 3m -- node e2e/profiling/pr-loading.mjs
```

The installed browser cache supplied Chromium; no CDN/runtime dependency was
introduced. The harness needs the repository's existing Playwright dependency
and compatible installed Chromium. `PR_PERF_FILE` measures a complete selected
file separately from the usual deck/drawer/tab/reload journey.

For HTTP/Git counts, start a **separate owned** service with
`GIT_TRACE2_EVENT=/absolute/temp/git.jsonl` in its environment, then issue one
cold GET followed by five warm GETs and five requests per endpoint from the
table. Capture `curl -w '%{time_starttransfer} %{time_total} %{size_download}\n'`
with `-o /dev/null`. Count Trace2 `start` events between requests and group their
`argv`; do not sum long-lived cat-file process times. Measure startup with a
monotonic timer immediately before spawning the built binary and stop it when
its loopback listener URL is printed (exclude build and queue wait).
Inspect `hivecontrol process list` and stop **only your own service IDs**.

Local raw output/profiles for this run are in `/tmp/cs-pr-perf.cOw7AY`; their
lifetime is temporary. The checked-in JSON preserves the main distributions.
The current fixture does not stress many reviewers, competing heads, huge
binary files, or a 100-slide deck. Those are regression/scale scenarios to add,
not performance conclusions established here.

Validation of this evaluation: both diagnostic benchmark suites completed
(3×3 operations), the CPU/allocation profile completed (5 operations), the
corrected browser journey and full-file journey each completed five samples,
four focused Go review tests passed, and all five Chromium tests in
`e2e/tests/pull-request-review.spec.ts` passed. Node syntax, formatting,
`git diff --check`, and the documentation-link check passed. No production
files changed; the complete repository/race and cross-browser suites were not
rerun for these isolated diagnostics.
