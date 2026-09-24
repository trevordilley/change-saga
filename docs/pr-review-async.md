# Review saves without page navigation

Review decision and comment POST routes accept `Accept: application/json` for
in-place saves. A successful write returns `saved: true`, its append-only event
ID, canonical target and fresh controls/discussion for the owning slide. A
projection failure after persistence returns the same receipt with a warning;
it must never be treated as permission to repeat the write. Ordinary form
requests keep their 303 fallback.

The browser retains the document, active slide, visual iframe, Item drawer,
scroll position and other composers. Feedback updates decision lists, source
currency, thumbnails and discussion; it does not render code patches, coverage
or the application shell. Existing threads receive new comments by event ID,
leaving open reply drafts in place. Source snapshots are not refreshed by a
feedback response: a response describing changed content marks the shown slide
stale until the reviewer reloads and inspects it.

Each scripted write carries the shown slide's snapshot. The snapshot combines
resolved base/head/following/frozen identity, slide target, the existing slide
content digest (visual, Item records and exact evidence) and the served source
checkout/Saga paths. The store invokes a read-only precondition under its writer
lock after loading the current review and resolving a reply's canonical target.
A changed snapshot, head or frozen review refuses the append. A later Git push
cannot change the explicit head recorded by an already-accepted decision.
There is no persisted format change or second approval target for Items.

Forms keep drafts on refusal and disable duplicate submissions while pending.
Network failures, 5xx responses and missing receipts leave the outcome unknown:
the browser retains the draft and blocks further writes for that page. **Check
saved feedback** performs a read-only refresh; it never retries the POST. The
reviewer can inspect the saved discussion before explicitly reloading. This is
a conservative outcome-check contract, not an idempotency claim across server
restarts or concurrent browser tabs.

Annotation create and note-edit composers retain their text and mark preview
until confirmed persistence. Updates continue using append-only replies to the
creation root. Delete events carrying an anchor and non-finite or negative
stroke widths are refused before writing, so failed validation cannot poison
the next Saga load. Browser saves keep the existing human seat; CLI AI seats
and the report's exact currency semantics remain intact.

## Diagnostic measurement and validation

On 2026-09-24, `e2e/profiling/pr-async.mjs` drove five decisions, five comments
and five annotation creates against a fresh copy of `/tmp/csrux-final-f9d80cf`,
including `.git`. The owned port-zero server used this source worktree with
`--repo`; the review still compared `6d717b2c9c7489ecf693e1c1202cb317dc9b3a58`
to `2fdf26fec900b08f618f8f8aa5f7db2720ebd469`. Chromium ran at 1440×1000 over
loopback, without throttling, on the shared development host. These are small
warm-page diagnostics, not CI budgets or a matched baseline speedup claim.

| Action (5 samples) | Median receipt ms | Receipt range ms | Median projected ms | Requests/action | Receipt bytes |
| --- | ---: | ---: | ---: | ---: | ---: |
| Slide decision | 310.7 | 301.3–421.3 | 311.9 | 1 POST | 5,708 |
| Slide comment | 279.7 | 254.9–429.0 | 282.5 | 1 POST | 8,904–21,688 |
| Annotation create | 327.5 | 288.5–414.6 | 405.5 | 1 POST + 1 annotation GET | 24,894–37,720 |

Receipt time ends after reading the server-confirmed JSON; projected time ends
when the control/discussion or annotation composer reflects success. Locators
and browser automation are included. The annotation GET is accounted for
separately by projected time and request counts. The comment/annotation payload
grows with the fresh slide discussion. All 15 actions made **zero full PR GETs,
zero document navigations and zero iframe navigations**. The harness performs
writes and requires an explicit disposable-fixture flag. Never use it on real
review records. An initial diagnostic attempt clicked a hotspot while placing a
note and was discarded; the recorded run uses a fresh fixture and chooses an
unobscured annotation-layer point.

Validation: all seven PR Chromium scenarios passed, plus the additional
sticky-note edit refusal/retry scenario. Focused server/store snapshot,
feedback-race and validation tests passed with `-race`. Tests include normal
form fallback, token checks, append-only counts, human attribution, current
currency, source/head/visual/evidence changes, frozen state, refused-save retry,
lost confirmation without automatic retry, and preserved drawer/document/visual.
The full server/store/saga race run was stopped after the coordinator identified
that its known duration exceeded the bounded job; it did not complete. The full
repository and cross-browser suites were not repeated in this child workspace.
