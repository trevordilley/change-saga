# Lazy Item drawer evaluation — not enabled

The standalone lazy prototype failed its drawer-latency gate. It removes 94.1%
of the initial HTML, but makes the first Item take about 1.35 seconds to open
and the largest Item take about 5.20 seconds to finish. The production changes
in this branch remain the async mutation implementation at
`a1074512e50545972e8f73234872b39f278cb521`; **no lazy endpoint, template or drawer
code from this experiment is enabled**.

`prototype.patch` preserves the complete implementation and tests against that
commit. `git apply --check experiments/pr-review-lazy/prototype.patch` verifies
its base. Apply it only in a disposable source checkout to reproduce the
experiment. The browser harness is `e2e/profiling/pr-item-loading.mjs`.

## Compared behavior

The initial page carries Item targets, reference counts and source/slide
snapshots, with empty diff templates. Opening one Item requests its exact diff
panel. Each response carries at most 1,000 rows or no-row diagnostics; the
existing Code Diff API's 200-row limit is unchanged. Continuation is automatic.
A cursor resumes at a reference index and line offset, so earlier references
are not regenerated. A partially displayed reference also pins its complete
diff projection digest to avoid splicing changed offsets. There is no retained
evidence cache. Each request validates current Saga/source identity before and
after generation.

The standalone patch calls the existing one-shot `a.referenceDiff`. It does
**not** contain the separately implemented request-local patch helper at
`a39e147fa2b2fe7c8f068ae32acae417c61001fe`. It must not be described as a
measurement of their combined implementation. Large references still pay for
repeated file patches; pagination also repeats Saga validation and range work.
Row count bounds the response, not the byte length of a pathological line.

## Workload and method

Measured 2026-09-24 on the shared macOS development host, Chromium at 1440×1000,
loopback with no throttling. `/tmp/csrux-final-f9d80cf` was copied, including
`.git`, into `/tmp/cs-pr-lazy.mZd7Oz`. All owned servers used `--repo` pointing
at this source worktree and port-zero listeners. No review writes were made to
this copy. The review stayed at base
`6d717b2c9c7489ecf693e1c1202cb317dc9b3a58` and head
`2fdf26fec900b08f618f8f8aa5f7db2720ebd469`.

Five fresh-page samples alternated the async commit's eager page and the lazy
prototype. Each sample opened the first Item and then **On-slide evidence
controls**, the largest Item: 46 references and 6,899 displayed rows. The first
Item has 18 references and 655 rows. Each pair compared the hash of every
rendered row's class and text, in order; all pairs matched exactly. An additional
five-page control used the helper-only binary from `a39e147` against the same
fixture and source refs, without integrating it into this branch.

Times include locator/automation overhead. Initial usable means the active
slide SVG and Item control are visible; drawer times start at its opening click.
They exclude user think time. Host contention was substantial (the eager
initial-usable range reached 4.8–7.1 seconds); these are diagnostic samples, not
CI budgets or proof of a universal speed ratio. Report initial readiness,
first-row latency and full drawer completion separately.

| Surface | Eager async commit | Helper-only eager control | Standalone lazy prototype |
| --- | ---: | ---: | ---: | ---: |
| Initial HTML bytes | 3,538,001 | 3,533,094 | 208,954 |
| Median initial usable, ms | 5,324.7 | 1,888.5 | 1,086.3 |
| First Item: first row, ms | 79.3 | 59.5 | 1,339.2 |
| First Item: every row, ms | 79.3 | 59.5 | 1,347.1 |
| Largest Item: first row, ms | 288.5 | 292.1 | 1,862.1 |
| Largest Item: every row, ms | 288.5 | 292.1 | 5,204.7 |
| First / largest Item requests | 0 / 0 | 0 / 0 | 1 / 7 |

The helper-only control ran after the paired run, so it is not an interleaved
latency comparison. Its initial usable range was 1,455.8–2,393.6 ms. Its exact
row hashes also match both paired implementations. The 4,907-byte difference
between eager controls comes from the async commit's added form/snapshot markup.

The largest lazy drawer's completion range was 4,167.9–5,793.2 ms. The reduction
in initial payload is real; it does not establish a useful improvement for a
reviewer opening exact evidence. In particular, comparison only with the old
uncached eager implementation would hide the faster helper-only alternative.

## Validation and follow-up

Focused endpoint, cursor, exact-row, freshness, frozen-review and JavaScript
checks passed with `-race` (6.210 seconds). Eight existing PR browser scenarios
passed with the prototype enabled. The additional lazy retry/late-response test
initially failed because its assertion used a multi-row locator as a single
locator; that assertion was corrected in the preserved patch but not rerun
before the performance gate stopped this experiment. The later cached-feedback
freshness adjustment in the patch was reviewed but likewise not revalidated in
the browser. The patch is experimental, not a tested production handoff.

A future attempt needs a deliberate selected-Item generation/reuse design and
measurement against the request-local helper baseline, including largest-Item
first-row and full completion. More small round trips, or merely enabling this
patch after a faster initial page, is not justified by these results. Preserve
exact refs, diagnostics, current discussion, source snapshots, late-response
guards and the separate slide-only approval target in any follow-up.

Reproduction requires two owned servers and a disposable source/Saga copy:

```sh
PR_ITEM_EAGER_URL=http://127.0.0.1:EAGER/reviews/review-ux-improvements \
PR_ITEM_LAZY_URL=http://127.0.0.1:LAZY/reviews/review-ux-improvements \
PR_ITEM_OUTPUT=/tmp/pr-item-loading.json \
hivecontrol exec oneshot 4m -- node e2e/profiling/pr-item-loading.mjs
```

For the separate helper-only control, set `PR_ITEM_ONLY_EAGER=yes`, point
`PR_ITEM_EAGER_URL` at that owned server, and omit `PR_ITEM_LAZY_URL`.
