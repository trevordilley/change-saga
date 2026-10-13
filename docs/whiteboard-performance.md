# Whiteboard browser performance

The [diagnostic harness](../e2e/support/whiteboard-performance.mjs) measures an
already-running whiteboard at `/technical/erd`. The paired measurements below
use the earlier 21-appearance DayLight scene. The harness does not
create fixtures, publish records, change source content, or restart the preview.
It exercises the reader's pan, zoom and focus controls through Playwright.

## Repeat the measurement

Run from the repository root with the existing `e2e` dependencies installed:

```sh
WHITEBOARD_PERF_URL=http://127.0.0.1:63540/technical/erd \
WHITEBOARD_PERF_OUTPUT=/tmp/change-saga-whiteboard-qa/performance/before \
node e2e/support/whiteboard-performance.mjs
```

If dependencies are installed in a different checkout, set
`WHITEBOARD_PLAYWRIGHT_PACKAGE` to that checkout's absolute `e2e/package.json`
path. This selects its existing `@playwright/test`; it installs nothing. Change
only the output prefix to `after` when measuring the optimized implementation.
The default is three independent browser contexts; `WHITEBOARD_PERF_RUNS` can
set another count. `WHITEBOARD_PERF_FOCUS_PATHS` accepts a JSON array of native
section or imported instance paths for another authored scene. The default tour
is the original 21-appearance scene; override it for the native model. Readiness
uses the viewer initialization signal, not a fixed number of imports.
Keep the same harness, browser, machine, fixture and workload
for before/after comparisons. Avoid concurrent browser tests or build loads.

Every run uses headless Chromium, a 1440 × 1000 viewport, device scale factor 1,
no CPU throttling, normal motion, and Details initially off. After page load and
settling, the workload performs:

1. Two opposing drags of 240 × 100 screen pixels, each with 60 pointer moves.
2. Twelve Control+wheel events at −80 followed by twelve at +80, separated by
   16 ms. Control is necessary because ordinary wheel input does not zoom.
3. Six focus-picker selections: the cross-store model, capture, source lineage,
   corpus, authority, and the repeated cross-store appearance. Each selection
   allows 400 ms for the authored camera motion.

Each phase includes a final 250 ms settling interval. Inputs and initial camera
are identical across runs; mouse delivery and animation-frame counts naturally
vary with the browser's ability to process work. Elapsed action time includes
Playwright transport, explicit waits and browser work; it is not CPU time.

## Evidence and interpretation

The output prefix produces an aggregate JSON file, one raw Chrome trace per run
(`-runN.trace.json`), and a final screenshot per run. JSON records the browser and
Node versions, initial camera, frame/node counts, response and script hashes,
workload, harness digest, page errors and per-phase measurements.

- `Performance.getMetrics` deltas report JavaScript (`ScriptDuration`), layout,
  style recalculation and total task durations in **seconds**, plus layout and
  style counts. These are page metrics, not a backend profile.
- A continuously scheduled `requestAnimationFrame` records frame intervals in
  **milliseconds**, including median, p95, maximum and counts above 25/50 ms.
  These intervals indicate responsiveness; they do not prove that every frame
  was presented to a physical display. Idle settling frames are included.
- Chrome trace complete events report Paint, PrePaint, Layout, UpdateLayoutTree,
  FunctionCall, EventDispatch and animation-frame duration aggregates in
  **milliseconds**. Nested categories overlap and must not be added together.
  Paint is main-thread paint work, not end-to-end GPU/compositor latency.
- Prototype wrappers count selected geometry reads and attribute API calls.
  A board-scoped MutationObserver counts actual mutation records by attribute
  name/type, including repeated writes of the same value. This is broader than
  the wrapped APIs: direct dataset/style assignments also cause mutations.
  Geometry counters include Playwright's own element/actionability reads.

Instrumentation and tracing add overhead. They are identical on both sides;
use the paired measurements to identify changes, not as an uninstrumented
production benchmark. Absolute timings also depend on other machine activity.
The traces can be loaded into Chrome DevTools Performance or Perfetto for
inspection. Source hashes distinguish a changed response from an accidental
rerun against the same viewer implementation.

## Measurements

Baseline captured on 2026-10-13 against the live DayLight `r3` board, using
Chromium 151.0.7922.34 on macOS arm64. All three runs had the same response hash,
21 frames, 6,459 board DOM elements, the same starting camera and no page errors.
The baseline app asset was `a856d3c433cc10cd/app.js`.

Medians across three complete workloads on each implementation:

| Measurement | Before | After | Reduction |
| --- | ---: | ---: | ---: |
| JavaScript CPU duration | 0.500 s | 0.108 s | 78% |
| Layout duration | 3.072 s | 2.852 s | 7% |
| Style recalculation duration | 1.956 s | 0.342 s | 83% |
| Total task duration | 13.477 s | 8.256 s | 39% |
| Layout count | 631 | 234 | 63% |
| Style recalculation count | 455 | 109 | 76% |
| Main-thread Paint trace duration | 677.7 ms | 113.6 ms | 83% |

Pan had particularly clear deterministic overhead: each run's 120 updates
made 55,080 `setAttribute` calls, 47,640 `tabindex` mutation records and 2,520
`getScreenCTM` reads. Pan alone caused a median 364 layouts and 245 style
recalculations. The source's `paint()` walks all frames and selectable controls,
rewrites their state and reads each frame's screen transform for every input.
The counters support removing redundant work from those paths. They do not
attribute all layout cost to any one API call.

Across baseline runs, pan p95 RAF intervals were 50.1, 25.0 and 50.4 ms, with
78, 94 and 58 intervals above 50 ms respectively. The varying idle/automation
intervals can dilute percentile results; report the slow-interval counts too.
Pan elapsed time varied from 9.7 to 26.1 seconds despite identical event/write
counts. CPU metrics, operation counts and all-run ranges are more informative
than a single wall-clock speedup.

The optimized app asset was `83414ce4b58e398f/app.js`. The comparison verifies
identical harness SHA-256, Chromium version, viewport, workload and initial
camera. Each side also had a stable response hash across its three runs. The
harness digest is
`9e16b4d8b4e85686f30bc766d821d0da21d8529374f438061d87bb3959451188`.

In every optimized pan run, the same 120 updates made only 160 `setAttribute`
calls, 12 `tabindex` mutations and **zero** `getScreenCTM` reads. Optimized zoom
and focus also made zero screen-transform reads. Caching geometry, batching
camera updates and avoiding unchanged DOM state substantially reduce redundant
work. The implementation also culls offscreen content and constructs entity
summaries lazily; these measurements assess that combined change, not isolated
contributions from each technique or the simultaneously integrated hover helper.

| Phase | Before p95 frame intervals (ms), runs 1/2/3 | After p95 (ms), runs 1/2/3 | Intervals >50 ms, before → after |
| --- | --- | --- | --- |
| Pan | 50.1 / 25.0 / 50.4 | 24.9 / 25.1 / 25.1 | 78/94/58 → 0/0/1 |
| Zoom | 41.6 / 57.4 / 41.1 | 25.9 / 25.8 / 25.1 | 2/12/1 → 0/0/0 |
| Focus | 59.2 / 50.7 / 42.6 | 33.2 / 25.3 / 34.0 | 15/13/7 → 0/0/2 |

Task-duration ranges were 11.345–15.502 seconds before and 7.626–8.879 seconds
after. Layout ranges overlap: 2.786–3.546 seconds before and 2.753–2.978 seconds
after. Layout remains a substantial cost, and these samples do not establish a
large layout-time improvement despite the lower layout count. No tested phase
now shows the baseline volume of repeated attributes or transform reads, but
the result is not a guarantee of 60 fps on other machines or larger boards.
The manager held other builds/browser tests during the optimized capture;
ambient machine load during the baseline was not independently controlled.

## Culling and artifact checks

The board retains all 21 frame instances and their exact source pins. Initial
board DOM count fell from 6,459 to 5,459 because summary construction is lazy.
That count is deliberately not held constant: deferred work and offscreen
culling are part of the measured implementation. A separate, untimed focus tour
checks that every workload target is restored after culling; those reads are
not included in the performance numbers. All seven visibility snapshots retained
the exact 21 instance/pin pairs from the authored board. Initial culling retained
two frame drawings in or near the opening viewport (system overview and capture).
Focusing overview, capture, lineage or corpus retained only that target frame.
Authority focus retained three nearby frames (authority, identity, whole local
model); the repeated system focus retained two (system return and OAuth).
Every selected target had a nonzero rendered rectangle, `display` other than
`none`, and `data-whiteboard-culled="false"` after settling. The 180-pixel
overscan intentionally retains neighbors before they enter the visible area.

Artifacts are under `/tmp/change-saga-whiteboard-qa/performance/`:
`before.json`, `after.json`, `comparison.json`, `before-summary.json`,
`before-run{1,2,3}.trace.json`, `after-run{1,2,3}.trace.json`, matching screenshots,
and `after-visibility.json`. Raw traces remain local rather than being committed.
The application regression suite is owned separately; this diagnostic checks
performance and focused frame restoration, not the entire interaction contract.

A subsequent one-run sanity check used the `r4` copy, which adds three labeled
section backgrounds while preserving all 21 original frame instances and pins.
Pan retained the same 160 attribute calls, 12 tabindex mutations and zero screen
transform reads. All focus targets restored, with no page errors. Paint was
112.8 ms and total task duration 7.121 s. These are sanity observations under
possible concurrent review/build load, not another paired comparison. The
`sections-r4*` artifacts sit beside the unchanged before/after measurements.

## Native document sanity check

After the one-document revision, a separate single run measured the DayLight
`r6` scene: 23 native entities with authored summaries, 279 fields, 12 authored
sections and 20 original source drawings. This is **not a paired comparison**
with the earlier scene. It uses the same pan/zoom workload and a new focus tour:

```sh
WHITEBOARD_PERF_RUNS=1 \
WHITEBOARD_PERF_FOCUS_PATHS='["delivery-area/capture","delivery-area/hosted","search-area/local-scope","authority-area/directory","cloud-identity","delivery-area/capture"]' \
WHITEBOARD_PERF_OUTPUT=/tmp/change-saga-whiteboard-qa/cohesive-v6/performance \
node e2e/support/whiteboard-performance.mjs
```

The recorded tour used headless Chromium 151.0.7922.34. JavaScript consumed
0.1045 s, style recalculation 0.2949 s, layout 2.8413 s and total tasks 7.6487 s.
Main-thread Paint events totaled 101.6 ms. The 120-move pan made 184 attribute
calls, 20 tabindex updates and **zero measured geometry reads**. Pan RAF intervals
had a 32.4 ms p95 and one interval above 50 ms; this is not a promise of
60 fps on every device. No JavaScript errors occurred. Layout remains the main
measured rendering cost.

The initial view culled 47 cached elements while retaining the complete source
metadata. Separate live checks verified native control restoration after culling,
summary/detail switching, exact entity and relationship code inspection, and
section/import Back/Forward transitions. The DOM is retained; this implementation
reduces offscreen rendering rather than claiming to eliminate initial parse or
memory costs.

Artifacts are `cohesive-v6/performance.json`, `performance-run1.trace.json` and
`performance-run1.png` under the local QA directory. Harness SHA-256:
`0b84e5af40ede8b42432881c46417aa8afdfb5378b4ab9fe884b86c6eaa9a99c`.

## Comprehensive native document sanity check

The final `r7` example has 76 native tables, 675 fields, 120 recorded
relationships and four narrative connections, with **zero imported frames**.
Its 43 reading groups use real parent membership; broader parent regions remain
atlas views. A separate isolated single run on 2026-10-13 used the same harness
and headless Chromium 151.0.7922.34, with no other builds or browser tests running.
This is a new scene and **not a paired comparison** with either earlier sample.

```sh
WHITEBOARD_PERF_RUNS=1 \
WHITEBOARD_PERF_FOCUS_PATHS='["delivery-area/capture/capture-reading-2","delivery-area/hosted/hosted-reading-1","search-area/retrieval/retrieval-reading-1","authority-area/directory/directory-reading-1","authority-area/invitations/invitations-reading-1","delivery-area/capture/capture-reading-2"]' \
WHITEBOARD_PERF_OUTPUT=/tmp/change-saga-whiteboard-qa/cohesive-v7/performance \
node e2e/support/whiteboard-performance.mjs
```

JavaScript consumed 0.1180 s, style recalculation 0.1729 s, layout 2.0073 s,
and total tasks 6.5802 s. Main-thread Paint events totaled 132.3 ms. The pan
phase made 172 attribute calls, 12 tabindex mutations and zero measured
geometry reads. Pan, zoom and focus p95 RAF intervals were 17.4, 9.2 and
16.6 ms respectively; none exceeded 50 ms. The focus phase includes 12
browser-automation rectangle reads. No JavaScript errors occurred. These
observations do not guarantee a particular frame rate on other hardware.

The initial scene retained 6,958 DOM nodes and culled 214 cached elements.
Culling suppresses offscreen rendering; it does not remove the document or
eliminate parse and memory costs. Shared annotation controls use one active
strip, cached exact-pin metadata and lazy code loading. A separate fresh-page
browser check observed zero annotation requests before interaction. All source
pins and hidden field rows remain available for inspection and focus restoration.

Artifacts are `cohesive-v7/performance.json`, `performance-run1.trace.json` and
`performance-run1.png`. The harness digest matches the native `r6` run above;
the report also records the exact HTML and application asset digests. An earlier
sample from before the final field-column and oversized-hover fixes is retained
in `cohesive-v7/pre-detail-fix-performance/`; the figures above use the finished
build.
