# PR review performance implementation handoff

The [baseline evaluation](pr-review-loading-performance.md) identified repeated
Git patch reads and unopened Item HTML as the dominant initial-load work.
After the user authorized implementation, three child workspaces prepared
independent changes. This coordinator branch does not merge those branches.

## Ready changes

| Change | Implementation | Measurements and reproduction |
| --- | --- | --- |
| Reuse successful raw file patches within one request | `a39e147fa2b2fe7c8f068ae32acae417c61001fe` | `2cbe5d07b69f64e54a52df092e3685b800cfaec5`, `docs/pr-request-diff-reuse-performance.md` |
| Request the existing 200-row batch for PR Code Diff | `1c3cc6977fa5b23702fcbb84a89161efb25fd8dd` | `496f4e764ef2f1f1e419da7cd4fb68958d921dda`, `docs/pr-code-batching-performance.md` |
| Save review decisions, discussions, and annotations without navigation | `a1074512e50545972e8f73234872b39f278cb521` | `docs/pr-review-async.md`, committed browser harness and raw samples |

Request-local reuse reduced five warm HTTP requests' median from **3439 ms to
1073 ms**. The complete 3,533,094-byte response was identical after normalizing
only the mutation token. The isolated 140-reference stage fell from **2363 ms
to 252 ms** (three runs of three operations). Each reference still resolves and
filters separately; only 19 successful raw file patches and one successful
repository-root lookup are shared. Failed reads are retried, including recovery
after a failed repository-root lookup. Nothing survives the request.

Larger Code Diff batches reduced a 573-row file from **12 requests to three**.
Five browser contexts per version gave all-hunks medians of **6582 ms versus
4655 ms**. First-row medians were 4075 ms versus 4472 ms, so this change does
not establish a first-row improvement. The full-traversal Go benchmark fell
from 2161 ms/248.5 MB allocated to 617 ms/63.6 MB allocated. The endpoint still
defaults to 50 rows and caps requests at 200; only the PR stream asks for 200.

These are separate experiments on a shared M3 Pro host, using the realistic
review fixture and comparison `6d717b2c9c7489ecf693e1c1202cb317dc9b3a58` through
`2fdf26fec900b08f618f8f8aa5f7db2720ebd469`. Do not add the gains together or
compare different experiments' noisy baselines as if they were one run.
The child findings preserve sample tables, fixture details, and commands.

## Saves without navigation

Five decisions, five comments, and five annotations on a fresh realistic copy
gave these medians, measured from the UI action:

| Action | Persisted receipt | Visible feedback | Requests after submission |
| --- | ---: | ---: | --- |
| Decision | 310.7 ms | 311.9 ms | One POST |
| Comment | 279.7 ms | 282.5 ms | One POST |
| Annotation | 327.5 ms | 405.5 ms | One POST and one annotations GET |

All 15 actions made zero full-PR GETs, document navigations, and iframe
navigations. This is an absolute measurement of the new path; there is no
matched baseline mutation-latency comparison. Ordinary forms retain their
redirect fallback.

Scripted writes carry the snapshot of the displayed slide. A precondition
under the Saga writer lock checks that snapshot, exact source range, canonical
target, and frozen state before appending. Feedback refreshes decisions,
currency, thumbnails, and discussion without replacing the slide or refreshing
its viewed snapshot. A changed view requires inspection before another write.
A confirmed receipt remains saved even if its display refresh fails. A lost
receipt keeps the draft and blocks blind retries until the reviewer checks the
saved feedback and explicitly reloads; this is not cross-session idempotency.

The same commit includes the parent's delegated annotation fixes: rejected
create/edit saves retain a usable draft, delete events cannot carry an anchor,
and non-finite or negative stroke widths cannot enter persisted records.

## Lazy Item loading was not accepted

The separately prototyped lazy Item endpoint is not part of the production
changes. It reduced initial HTML from 3,538,001 bytes to 208,954 bytes, but
moved substantial latency into the evidence drawer. The largest realistic Item
has 46 references and 6,899 displayed rows. Even at 1,000 rows per response,
it required seven sequential requests with repeated validation and selected
reference work. Five paired samples took a median **5204.7 ms** to complete
that drawer, versus **288.5 ms** for the existing eager drawer. The first Item
(655 rows) increased from **79.3 ms to 1347.1 ms**, despite using one request.
Initial usable-slide medians fell from 5324.7 ms to 1086.3 ms in this particular
shared-host run. Displayed row hashes matched in every sample, but the
interaction regression was unacceptable for this slice.

The prototype, harness, and final control measurements are retained in isolated
evaluation commit `de5d500` in the async workspace, rather than enabling that path.
`experiments/pr-review-lazy/prototype.patch` applies to `a107451`; production
code was restored to that commit after the experiment.
The additional five-sample helper-only browser control completed before its
queued cancellation was read: median initial usability was 1888.5 ms, first
Item completion 59.5 ms, and largest Item completion 292.1 ms. Thus the lazy
prototype also worsened drawer completion substantially compared with the
already-optimized eager version. Those displayed row hashes matched as well.
Any follow-up should compare against the improved request-local eager baseline,
measure both click-to-first-row and click-to-complete, and address repeated
selected-Item work before changing the default. A smaller initial response alone
does not establish a better meaningful review experience. Cross-request reuse
would still need explicit source, configuration, dirty-record, and feedback
invalidation analysis. No such cache is part of this implementation.

## Verification limits

The request-local change passed focused review/diff race tests, resolver and
Git diff tests, vet, build, and all five existing Chromium PR tests. Its broader
server-plus-related package run exceeded a tracked five-minute limit.

Batching passed exact row/action equality, literal-path encoding, response
bounds, ordinary-comparison compatibility, fresh validation after a same-size
edit with restored mtime, retry after repair, and stale-head cursor rejection.
Vet, build, formatting, and documentation links passed. Full Chromium passed
44/45 tests; the terms-directory test expects five headers while the baseline
already renders seven. All PR, bounded-surface, performance, and navigation
tests passed. The full Go race run exceeded its tracked eight-minute limit in
the server package. Neither timeout is a full-suite pass. Firefox and WebKit
were not run.

Async feedback passed all seven PR Chromium scenarios plus the sticky-note
edit failure/retry scenario, and focused server/store race checks. Snapshot
tests include source and Item evidence changes, canonical checkout/Saga
identity, concurrent changes before the store check, and changed content
between persistence and feedback. An overbroad package race run was stopped;
no complete repository or cross-browser pass is claimed.

## Integration boundaries

The request-local helper changes only the reference-diff extraction and its
review-page loop. Async feedback also touches `reviews.go`, so preserve
both behaviors when combining them. The batching template edit is one additive
`limit` query parameter; retain it when applying the parent's Code Diff layout
work. No production cross-request cache was introduced: its measured key cost
was too close to the selected-file work it could avoid.

All measurements used disposable Saga copies and owned tracked services. Test
review actions never touched the real Saga or parent preview. A subsequent user
correction requires real branch `app.saga` documentation to accompany the code;
the follow-up is authored through public CLI commands with partitioned target
ownership. It does not record approvals or change actual review feedback.
No branches or workspaces were pushed, merged, archived, or deleted by this
coordinator or its implementation children.

## Saga documentation handoff

The actual branch Sagas now accompany each implementation through public CLI
authoring. No real review decisions, comments, or annotations were recorded.

| Branch | Documentation commit | Scope |
| --- | --- | --- |
| `perf/pr-review-loading` | `f01a0c0129d85639f11bee6715d162d0e7b96c07` | Evaluation design fragment, four focused landmarks, exact baseline/method/handoff evidence and an artifact-inspection verification. |
| `perf/pr-request-diff-reuse` | `ad0b1311ac594499f27764010d35e8a94d8ea30e` | Six-Item `pr-request-diff-reuse` slide in the existing visual implementation deck; request lifetime, independent resolution/filtering, successful patch reuse, retry behavior and measured HTTP results. |
| `perf/pr-code-page-generation` | `81b890a07f42155a3ca88e211f789400603a1b85` | Five-Item `pr-code-batching` slide in the existing comparison history deck; matched request counts, fresh per-page work, bounds/cursors and measured browser results. |
| `perf/pr-review-async` | `91b9966268951d8ae0b149b69eff1b0edcb056a5` | Four existing decision/comment slides updated, preserving 23 Item identities/selectors and replacing 14 exact evidence records. |

All three implementation Sagas validate without issues. The new reuse and
batching slides have zero stale selectors across their scoped mapping queries;
their scrutiny scores are zero. Overlap between reuse and retry evidence is
intentional: the first explains successful sharing, the second its failure
boundary. Existing unrelated drift remains with the reconciliation owner.
All six authored/revised implementation slides passed raw and actual-reviewer
visual QA at 1280×720 and 1024×576, with manual relationship inspection.
Temporary renders are under `/tmp/pr-request-reuse-saga/qa`,
`/tmp/pr-code-batching-saga/qa`, and `/tmp/cs-pr-async-qa`.

These branches are ready for the parent's integration review. Combined-source
validation and browser regression checks still need to run after integration;
the isolated measurements do not establish the combined latency distribution.
