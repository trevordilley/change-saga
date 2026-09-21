# App Saga quality authoring review

Status: canonical quality authoring audit for `agent-loop`, `reviewer-app`,
and `reviews`, authored on 2026-09-20 against source commit
`36b4a21b5d049f272a1f4a0cbbcfdf60ded126c5`.

## Scope and result

This wave changed only the three owned feature directories and this audit. It
did not edit product source or another feature directory. Every Saga mutation
was made with a freshly built repository CLI,
`/tmp/change-saga-quality-authoring`, built from `./cmd/change-saga`.

The authored quality layer contains:

- 19 active, persona-readable automated test cases;
- 53 current direct `verifies` relations to owned accepted criteria;
- 38 current evidence records, exactly one `test_implementation` and one
  `implementation_under_test` head per case;
- five superseded test-evidence records retained as immutable history after a
  dogfood failure exposed unsafe selector text on documentation pages; and
- 19 current passed run heads, all recording a command that actually completed
  successfully at the source commit above.

Owned quality coverage is 53 of 55 accepted criteria. The 53 covered criteria
all have current positive coverage and current passed runs. The two deliberate
gaps are `safe-local-review:bounded-requests` and
`safe-local-review:bounded-responses`; the current Item evidence proves HTTP
timeouts and header limits, bounded query pages and fragment chunks, path
containment, and cursor integrity, but not the complete archive, decode,
diff-expansion, and concurrency contract.

All 53 new relations are current. All authored test cases are active,
non-orphaned, and passed: 19 passed/current, 0 stale, 0 failed, and 0 not run.
The full Saga has zero stale relations and zero stale code references.

## Canonical cases

Each revision has ordered one-line actions, a one-line expected result for each
action, an explicit overall pass/fail condition, and meaningful declared
coverage kinds. The implementation-under-test selectors were compared as sets
against the corresponding current implementation Item evidence; parity is
exact for all 19 cases. The two check Items share the same runtime selector, so
one case truthfully covers both.

| Feature | Test case | Kinds | Direct criteria | Matching implementation Item |
| --- | --- | --- | ---: | --- |
| `agent-loop` | `coverage-areas-are-readable` | positive, edge | 2 | `six-areas` |
| `agent-loop` | `trustworthy-selected-coverage` | positive, negative, edge | 4 | `trustworthy-report`; identical runtime selector on `selected-check` |
| `agent-loop` | `bounded-structured-reads` | positive, negative, edge | 5 | `structured-reads` |
| `agent-loop` | `first-change-stays-focused` | positive, edge | 6 | `first-change-flow` |
| `agent-loop` | `bounded-mutations-rollback-replay` | positive, negative, edge | 2 | `bounded-loop` |
| `agent-loop` | `merged-evidence-releases-work` | positive, negative, edge | 5 | `merge-gate` |
| `agent-loop` | `story-moves-keep-links` | positive, edge | 1 | `reversible-structure` |
| `agent-loop` | `coordination-facts-are-not-proof` | positive, edge | 2 | `coordination-resumption` |
| `reviewer-app` | `documentation-and-review-stay-distinct` | positive, negative, edge | 1 | `documentation-side` |
| `reviewer-app` | `review-keeps-documentation-reachable` | positive, edge | 1 | `review-side` |
| `reviewer-app` | `related-reviews-are-derived` | positive, edge | 2 | `related-reviews` |
| `reviewer-app` | `authored-content-is-sandboxed` | positive, negative, edge | 2 | `untrusted-content` |
| `reviewer-app` | `local-review-rejects-foreign-requests` | positive, negative, edge | 3 | `local-server` |
| `reviewer-app` | `browser-errors-redact-local-paths` | positive, negative, edge | 1 | `host-boundary` |
| `reviews` | `one-review-deck-follows-pr` | positive, negative, edge | 5 | `deck-spine` |
| `reviews` | `review-decisions-go-stale` | positive, negative, edge | 2 | `currency` |
| `reviews` | `merged-review-freezes-history` | positive, negative, edge | 2 | `merged-history` |
| `reviews` | `slide-decisions-and-comments` | positive, negative, edge | 6 | `decisions-comments` |
| `reviews` | `review-coverage-names-omissions` | positive, negative, edge | 1 | `review-omissions` |

The feature totals are eight cases and 27 criteria for `agent-loop`, six cases
and 10 covered criteria for `reviewer-app`, and five cases and 16 criteria for
`reviews`. The reviewer-app ownership set also includes the two intentionally
uncovered resource-bound criteria, making the complete owned denominator 55.

## Executed evidence

The first attempt failed and was not recorded: the existing documentation
read-only dogfood test found literal review-control selector text inside newly
rendered test-source evidence. Five evidence heads were superseded with focused
tests that exercise the same behavior without placing those literal selectors
on Documentation pages. The following command then completed successfully:

```text
go test ./internal/areas ./internal/nextaction ./internal/cli ./internal/reviewapp ./internal/workplan ./internal/requirements ./internal/server ./internal/reviewstore -run '^(TestReportShapeIsStable|TestComparedStatusTextStatesOneCountAndListsRanges|TestCheckAnswersOnlyTheNamedAreas|TestStatusFailsOnlyWhenTheReportCannotBeTrusted|TestEveryQueryEnvelopeCarriesTheSchemaAndASnapshot|TestDocumentedReadBounds|TestSeparateRepositoriesLargeAndActiveContentAreBoundedAndInert|TestCursorsAreStableBoundAndIntegrityChecked|TestFirstChangeCoversOnlyImplementation|TestComparingPutsTheChangeBeforeTheOverview|TestActionsAreOrderedByCategoryThenAxisThenResource|TestDeterministicActionsCarryGrammarShapesAndQuestionsCarryOneQuestion|TestGrowthComesLastAndTeachesItsPractice|TestEmptyActionsAreTheFixedPoint|TestCoverBatchWritesNothingWhenAnyRecordFails|TestRequestReplayIsIdempotentAndPayloadReuseConflicts|TestWavesDoNotCreateImplicitBarriersAndDependenciesFormADAG|TestDependencyConditionsUseProgressMergeAndPinnedContractFacts|TestMovingAStoryKeepsEveryRelationCurrent|TestRevisionAndProgressGraphsRetainConcurrentHeadsUntilReconciled|TestWorkspaceAssignmentsPersistPortableIdentityAndUseParentURNs|TestEachSideListsWhatItIsAbout|TestDocumentationPagesHaveNoApprovalOrCommentControls|TestComparingShowsTheMatchingReviewBesideTheLayers|TestTheReviewSideStatesTheGapWhenThereAreNoReviews|TestRelatedReviewsAreDerivedFromTheChangedLines|TestInteractiveFragmentIsServedWithSandboxCSP|TestSecureHandlerRejectsCrossOriginFetchSiteAndHost|TestHTTPServerHasBoundedResourceSettings|TestListenRefusesNonLoopbackAddressBeforeServing|TestCleanDiagnosticPathRedactsPortableAbsolutePaths|TestMissingSagaErrorDoesNotExposeAbsoluteRoot|TestReviewItemsReferenceRecordsAndNeverCountAsCoverage|TestOnePullRequestHasOneReview|TestRepinFreezesTheLandedReviewAndHistoryLinksIt|TestReviewCoverageAccountsForItsOwnRange|TestReviewPageShowsDiffsDecisionsAndCurrency|TestReviewDecisionsGoOutOfDateSlideBySlide|TestFrozenReviewIsViewableButTakesNoDecisions|TestReviewRecordsRoundTripAndValidate|TestReviewCommentsThreadOnSlidesAndItemsOnly|TestReviewCoverageReportsTheChangesTheDeckDoesNotExplain)$' -count=1 && npm --prefix e2e test -- tests/security.spec.ts tests/accessibility.spec.ts tests/pull-request-review.spec.ts
```

The Go packages passed, and Chromium passed all 10 scenarios in the three
named specs. Each run record names both current evidence heads. No failed run
record was fabricated for the earlier dogfood failure.

## Requirement-to-quality-to-code inspection

`query traceability` shows a current test case and passed run for every one of
the 53 covered criteria: 8/8 `bounded-automation`, 7/7 `coverage-report`, 5/5
`first-change`, 7/7 `parallel-delivery-plan`, 6/8 `safe-local-review`, 16/16
`review-is-pr-deck`, and the four owned reviewer-navigation criteria in
`observe-or-compare`.

The current query ends the quality path at the run rather than continuing
through quality evidence to code. The complete convergence was therefore
checked in two parts: traceability established
criterion -> test case -> passed run, and an exact set comparison established
that each case's `implementation_under_test` selectors equal its Item-owned
runtime selectors. The result was `19/19` exact. The test-implementation
evidence points to exact named Go test functions and focused Playwright test
blocks, never whole test files.

## Honest incomplete behavior

The quality graph deliberately does not claim more than current code and tests
prove:

- read/unread persistence and automatic earliest-unfinished resume remain
  unimplemented and have no quality claim;
- authored iframes have no parent-message channel, so no message validator or
  parent-messaging case is claimed;
- `bounded-requests` and `bounded-responses` remain `missing_kind` because the
  broader archive, decode, diff-expansion, and concurrency limits are not all
  implemented and tested by the current Item evidence; and
- `sandbox-network` is limited to the authored-content sandbox and CSP contract.
  There is still no full-session browser proof that observes zero DNS or
  outbound requests, so no broader offline-session claim is made.

## Change Saga CLI dogfooding

Each finding records the intended goal, exact command, observed behavior,
impact, workaround, and smallest improvement.

### F1 — Quality batching is uneven

- Intended goal: author one coherent quality graph without a partial-progress
  window.
- Exact commands: `/tmp/change-saga-quality-authoring quality test-case add
  --feature agent-loop --from - --json app.saga` repeated per case;
  `/tmp/change-saga-quality-authoring quality evidence add --batch - --json
  app.saga`; `/tmp/change-saga-quality-authoring relation add --feature
  agent-loop --id q-bounded-structured-reads-verifies-stable-envelope --type
  verifies --from urn:change-saga:app:test-case:bounded-structured-reads --to
  urn:change-saga:app:story:bounded-automation:criterion:stable-envelope
  --rationale 'The case verifies the stable bounded query envelope, page
  metadata, snapshot cursor rejection, and inert content reads.' --request-id
  q-bounded-structured-reads-verifies-stable-envelope --json app.saga`;
  `/tmp/change-saga-quality-authoring quality run record --from - --json
  app.saga` repeated per run.
- Observed behavior: evidence has an atomic batch, but test-case creation,
  multi-criterion linking, activation, and run recording do not. This wave
  required 19 creates, 53 relation mutations, 19 activations, and 19 run
  mutations.
- Impact: interruption can leave a valid but incomplete graph, and shell
  orchestration must implement recovery.
- Workaround: request IDs on every mutation, validation and coverage reads
  between phases, and the evidence batch for the largest exact-reference write.
- Smallest improvement: add one dry-run and atomic quality-plan JSON batch for
  cases, links, lifecycle, evidence, and runs, with per-entry diagnostics.

### F2 — Evidence-batch errors lose request identity

- Intended goal: atomically add 38 evidence records.
- Exact command: `/tmp/change-saga-quality-authoring quality evidence add
  --batch - --json app.saga` with the 38-record JSON array on stdin.
- Observed behavior: the first batch correctly wrote nothing, but reported
  only `invalid code 2: code location is not canonical; canonical form is
  ...:internal/server/template.go#L363` for the submitted `#L363-L363`.
- Impact: `code 2` did not identify the batch entry, test case, or evidence ID;
  a large batch requires searching the input.
- Workaround: located the one single-line selector, changed it to canonical
  `#L363`, and replayed the unchanged atomic batch successfully.
- Smallest improvement: prefix every batch diagnostic with array index,
  test-case URN, evidence ID, and field path.

### F3 — Multi-criterion links have no batch surface

- Intended goal: link each persona-readable case directly to every criterion
  it tests.
- Exact command pattern: `/tmp/change-saga-quality-authoring relation add
  --feature reviews --id q-slide-decisions-and-comments-verifies-tool-records
  --type verifies --from
  urn:change-saga:app:test-case:slide-decisions-and-comments --to
  urn:change-saga:app:story:review-is-pr-deck:criterion:tool-records --rationale
  'The case verifies slide-scoped append-only decisions and discussion without
  a product approval verdict.' --request-id
  q-slide-decisions-and-comments-verifies-tool-records --json app.saga`.
- Observed behavior: one case verifying six criteria required six independent
  mutations; there is no repeated `--to` or atomic relation array.
- Impact: criterion-precise authoring is much slower than broad linking and can
  be partially applied.
- Workaround: deterministic IDs and request IDs, followed by stale-relation and
  owned-criterion coverage queries.
- Smallest improvement: support an atomic relation batch and a test-case
  `--verifies` array that still writes one precise relation per criterion.

### F4 — Test evidence still requires manual line selection

- Intended goal: cite exact named Go and Playwright tests.
- Exact command shape: `/tmp/change-saga-quality-authoring quality evidence
  add --test urn:change-saga:app:test-case:review-decisions-go-stale --role
  test_implementation --code
  58541107888381e0d7f0332bee4f1ad353b0399e:internal/cli/reviews_test.go#L98-L157
  --json app.saga`.
- Observed behavior: the CLI resolves and digests canonical ranges but cannot
  select a Go test, TypeScript test block, or symbol by name.
- Impact: authors must calculate inclusive ranges and harmless surrounding
  edits can stale a broader-than-needed reference.
- Workaround: inspected function boundaries and pinned only the exact named
  function or focused Playwright block.
- Smallest improvement: add language-aware selectors such as `--go-test` and
  `--playwright-test`, resolved to ordinary immutable ranges at write time.

### F5 — Item/evidence parity is not queryable

- Intended goal: prove that quality implementation-under-test selectors
  converge exactly with current Item-owned runtime evidence.
- Exact command: `/tmp/change-saga-quality-authoring query mappings --saga
  app.saga --target urn:change-saga:app:test-case:bounded-structured-reads
  --limit 100`.
- Observed behavior: the query returned `target was not found`; mappings do not
  accept a test case, and traceability does not expose the exact-match join.
- Impact: the v5 parity invariant cannot be audited through the supported
  structured query surface.
- Workaround: queried traceability for the criterion paths and compared the
  current evidence JSON selectors against the matching Item `40-e` selectors;
  all 19 sets matched exactly.
- Smallest improvement: let mappings accept test cases and expose
  `matches_item_diff` with the Item, evidence record, exact locations, and any
  unmatched or overlapping selector.

### F6 — The documented quality queries are absent

- Intended goal: query owned quality coverage and one criterion's
  design/implementation/quality comparison directly.
- Exact commands: `/tmp/change-saga-quality-authoring query quality-coverage
  --saga app.saga --limit 200` and `/tmp/change-saga-quality-authoring query
  coverage-comparison --saga app.saga --criterion
  urn:change-saga:app:story:bounded-automation:criterion:stable-envelope`.
- Observed behavior: both returned `unknown query operation`; they appear in
  the v5 quality contract but not the current query registry.
- Impact: quality coverage requires the much larger full `status --json`
  payload, and no single response shows the promised matrix.
- Workaround: projected owned criteria from `status --json` with `jq`, then ran
  per-story traceability and exact evidence parity checks separately.
- Smallest improvement: implement the two documented operations or remove them
  from the contract until shipped.

### F7 — Feature-scoped status leaves quality app-wide

- Intended goal: inspect only one owned feature's quality totals.
- Exact command: `/tmp/change-saga-quality-authoring status --json --feature
  agent-loop app.saga`.
- Observed behavior: the quality section still contained all app criteria and
  all 20 app test cases, including the pre-existing code-evidence case.
- Impact: a feature-bounded audit must manually filter URNs and feature fields,
  increasing payload size and the risk of counting unrelated quality.
- Workaround: filtered the full quality projection to the six owned stories
  plus the four owned `observe-or-compare` criteria.
- Smallest improvement: apply `--feature` consistently to quality criteria,
  tests, facts, and summary counts, while retaining an explicit app-wide mode.

### F8 — Batch output duplicates current heads

- Intended goal: read a concise receipt for the successful evidence batch.
- Exact command: `/tmp/change-saga-quality-authoring quality evidence add
  --batch - --json app.saga`.
- Observed behavior: `current_heads` repeated each test's two evidence heads
  multiple times even though `paths` and `created` were correct.
- Impact: the receipt is noisy and cannot be used directly as a unique head
  list.
- Workaround: treated `created` as the mutation receipt and reloaded status for
  authoritative heads.
- Smallest improvement: deduplicate and sort `current_heads` before emitting
  the batch result.

### F9 — Documentation control checks inspect displayed source text

- Intended goal: run every referenced test after adding exact test-source
  evidence.
- Exact command: the combined Go and Playwright command in **Executed
  evidence** above.
- Observed behavior: the first run failed
  `TestDocumentationPagesHaveNoApprovalOrCommentControls` because a quality
  page displayed a test source line containing `data-review-decision`; the
  test scans raw HTML substrings and treated inert code text as a live control.
- Impact: correct exact evidence can make a documentation safety test fail for
  content rather than behavior.
- Workaround: superseded five test-evidence heads with other focused tests that
  cover the same behavior without those literal selectors, then reran the full
  command successfully.
- Smallest improvement: make the dogfood assertion parse the DOM and reject
  actual form/control elements outside escaped code blocks rather than scanning
  all rendered text.

### F10 — Traceability stops before quality code

- Intended goal: inspect requirement -> quality -> test code and requirement
  -> quality -> implementation Item/code in one response.
- Exact command: `/tmp/change-saga-quality-authoring query traceability --saga
  app.saga --requirement bounded-automation --limit 200`.
- Observed behavior: each quality path ends at the passed run; evidence URNs,
  test-code references, and the exact Item-selector match are omitted.
- Impact: `delivered: true` is visible, but the evidence chain requested by the
  v5 contract cannot be independently inspected from that response.
- Workaround: paired traceability with status, current evidence records, and
  the 19-case exact parity comparison.
- Smallest improvement: append current quality evidence, test-code locations,
  and `matches_item_diff` hops to traceability paths.

## Final verification commands

The final handoff uses the same freshly built CLI:

```text
/tmp/change-saga-quality-authoring validate --json app.saga
/tmp/change-saga-quality-authoring status --json app.saga
/tmp/change-saga-quality-authoring query relations --saga app.saga --state stale --limit 500
/tmp/change-saga-quality-authoring references --stale --json app.saga
/tmp/change-saga-quality-authoring query traceability --saga app.saga --requirement <owned-story> --limit 200
```

Validation is successful. The four warnings are pre-existing visual fragments
in `format` and `requirements`, outside this wave's allowed edit scope. The
stale relation and stale reference queries both return zero.
