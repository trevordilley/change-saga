# PR review loading evaluation {#pr-review-loading-evaluation}

This record explains the September 24, 2026 performance investigation. The coordinator branch adds diagnostic harnesses and measured findings; the production changes below are separate commits awaiting integration. No review decision or annotation was authored in the real Saga for these tests.

## Reproducible baseline {#pr-loading-baseline}

The disposable fixture contained 1,656 files, four review slides, 12 Items and 140 exact code references across 19 changed files. Its source range was `6d717b2c9c7489ecf693e1c1202cb317dc9b3a58` through `2fdf26fec900b08f618f8f8aa5f7db2720ebd469`. Warm PR requests returned 3,533,094 bytes and invoked 298 Git subprocesses. Five Chromium contexts measured a median 2,936.6 ms to the visible first slide and usable Item control. These are shared-host observations, not latency budgets or cold-disk guarantees.

## Measure the whole interaction {#pr-loading-method}

The read-only browser harness measures initial usability, reload, slide navigation, Item code, Code Diff and Coverage; its selected-file mode distinguishes the first row from all hunks. The opt-in Go benchmark separates validation, report construction, reference diffs and shell work. Use a copied Saga with its explicit source checkout, local assets and tracked bounded runs. Do not infer rendering cost from a Git subprocess wait profile alone.

## Implementation handoff and rejected work {#pr-loading-handoff}

Separate reviewed commits implement request-local patch reuse (`a39e147`), existing 200-row PR Code Diff batches (`1c3cc69`) and no-reload feedback (`a107451`). Their measured outcomes and test limits are recorded in `docs/pr-review-performance-implementation.md`; none is represented as already integrated into this coordinator branch.

The isolated lazy-Item prototype was rejected: its largest 6,899-row drawer took a median 5,204.7 ms after clicking, versus 288.5 ms eager, even though initial HTML fell from 3.54 MB to 209 KB. An optimized eager control completed that drawer in 292.1 ms. The prototype remains an experiment, not the current renderer. Smaller initial HTML alone does not establish a better review experience.

## Boundaries {#pr-loading-limits}

The source references, repository identity, validation diagnostics, currency and attributed append-only feedback remain correctness requirements. Slide approval remains distinct from Item evidence. Small samples on a shared M3 Pro host do not support tail-percentile claims. Full-suite timeouts and the existing terms-table browser assertion are limitations, not passing checks. No current story or criterion was invented or advanced by this evaluation.
