# App Saga quality evidence and comparison report

Date: 2026-09-20

## Scope and contract

This is the canonical quality report for only:

- `app.saga/___features/code-evidence.feature`
- `app.saga/___features/comparison.feature`
- this report

No product source or other feature directory was edited. The audit used the accepted current story revisions, both owned design and implementation decks, `docs/app-saga-code-test-map.md`, the two preceding evidence audits, and the v5 requirements/design/quality lifecycle contract.

A fresh repository-local CLI was built and used for every Saga mutation:

```text
hivecontrol exec oneshot 5m -- go build -o /tmp/change-saga-quality-evidence-comparison ./cmd/change-saga
/tmp/change-saga-quality-evidence-comparison version
# 0.2.0-dev
```

The quality model follows the v5 contract: an active test case directly `verifies` each criterion it proves; its current revision has separate exact `test_implementation` and `implementation_under_test` evidence; and a current passed run records the exact command that was actually executed. A broad design or implementation relation was not treated as test proof.

## Result

Seven active automated test cases cover 34 of 39 owned accepted criteria. All seven have one current passed run, current direct verifies relations, and current evidence. There are zero stale, failed, blocked, skipped, conflicted, orphaned, or not-run owned test cases.

| Feature | Accepted criteria | Directly covered | Honest gaps | Active cases | Current passed runs |
| --- | ---: | ---: | ---: | ---: | ---: |
| `code-evidence` | 22 | 20 | 2 | 4 | 4 |
| `comparison` | 17 | 14 | 3 | 3 | 3 |
| Total | 39 | 34 | 5 | 7 | 7 |

The pre-existing `resolve-remaps-and-stales` case was revised in place from r1 to r2. Its two existing verifies relations were repinned and its earlier run/evidence remain superseded history; no duplicate resolver case was created.

## Canonical test cases

Every revision contains ordered, one-line actions paired with one-line pass/fail expected results. Coverage kinds describe meaningful behavior, not test volume.

| Test case | Kinds | Directly verified criteria | Exact test implementation |
| --- | --- | --- | --- |
| `resolve-remaps-and-stales` | positive, negative, edge | `moved-lines`, `changed-lines`, `stale-change`, `documentation-only-change` | `internal/coderesolve/resolve_test.go#L42-L103` |
| `repair-evidence-safely` | positive, negative, edge | `weak-mappings`, `atomic-repair`, `post-merge-current` | focused mapping, cover replacement, and reference-repin tests in `internal/reviewapp/mappings_test.go` and `internal/cli/{cover,references}_test.go` |
| `preserve-claim-verification-history` | positive, negative | `falsifiable-claim`, `claim-target`, `claim-evidence`, `verification-result`, `verification-method`, `verification-summary`, `append-only`, `verification-history` | `internal/cli/claims_test.go#L14-L63` |
| `use-companion-source-repository` | positive, negative, edge | `verified-checkout`, `companion-source`, `sync-cursor`, `companion-compare`, `observe-existing-code` | focused repository, companion comparison, and external-ref tests in `internal/gitdiff` and `internal/cli` |
| `inspect-comparison-layers` | positive, negative, edge | `merge-base`, `changed-layer`, `affected-layer`, `code-layer`, `unexplained-code` | focused merge-base/layer Go tests plus `e2e/tests/cli.spec.ts#L197-L234` |
| `keep-documentation-and-related-reviews-reachable` | positive, negative | `documentation-context`, `related-reviews` | focused server tests plus `e2e/tests/documentation.spec.ts#L31-L72` |
| `explain-comparison-history` | positive, negative, edge | `open-history`, `node-history`, `replacement-history`, `paired-replacements`, `ambiguous-replacements`, `commit-reasons`, `squash-keeps-reasons` | `internal/cli/layers_test.go#L196-L311` |

The 14 current quality-evidence records contain 32 distinct implementation-under-test selectors. An exact commit/path/range/digest set comparison found that all 32 already exist on implementation Items somewhere in the Saga. This makes the Item and quality paths converge on the same runtime facts instead of creating a second implementation map. Test implementation evidence points only to the exact functions executed by the recorded commands.

## Honest gaps

The five uncovered criteria are deliberate. No test case or passed run was fabricated for them.

1. `claims-verification / failed-visible`: the focused claim test records verified and inconclusive history, but does not prove that a failed result remains visible.
2. `companion-repositories / move-later`: the implementation audit already identifies the missing move preflight/end-to-end relocation workflow. Current code and tests do not prove that moving a companion Saga later preserves links.
3. `observe-or-compare / same-against`: individual range and surface behavior is tested, but no single test proves that reviewer view, status, and structured queries all use exactly the same selected comparison.
4. `observe-or-compare / separate-modes`: the known persistent browser mode-label and deep-link restoration contract remains incomplete.
5. `observe-or-compare / derived-reviews-label`: related-review derivation is tested, but the exact reviewer-facing label that identifies the list as derived from code links is not asserted.

## Executed and recorded commands

Each command below was executed at the source HEAD recorded in its run record and passed before it was recorded. The two Playwright commands used the real Node/npm entry points because DevSwarm's bare `npm` invocation was substituted with Bun during installation.

```text
go test ./internal/coderesolve -run '^TestResolveRemapsMovedLinesAndReportsChangedLinesStale$' -count=1

go test ./internal/reviewapp ./internal/cli -run '^(TestMappingReasonsExposeBreadthWithoutCallingItCorrectness|TestFocusedMappingHasNoManufacturedWarning|TestReplaceCoverageFailurePreservesOriginalRecord|TestReplaceCoverageCanAtomicallyReuseTheRecordName|TestReferencesSurviveShiftsGoStaleOnEditsAndRepinAfterSquash)$' -count=1

go test ./internal/cli -run '^TestClaimsAndVerificationsAreIndependentAppendOnlyRecords$' -count=1

go test ./internal/gitdiff ./internal/cli -run '^(TestReadVerifiesCheckoutRepositoryWithExplicitOverride|TestReadRequiresOverrideWhenCheckoutOriginIsUnavailable|TestRepositoryCorrespondenceIgnoresTransportAndGitSuffix|TestCompanionSagaComparesThroughItsSyncCursor|TestCoverCommitAndRefPinOutsideTheComparison)$' -count=1

go test ./internal/cli -run '^(TestReplacedSlidePairsWithItsReplacementAndItsReason|TestAmbiguousReplacementNeedsAnExplicitLink|TestSquashMergeKeepsItsBranchReasons)$' -count=1

go test ./internal/gitdiff ./internal/cli -run '^(TestCommittedComparisonsUseActualMergeBase|TestCodeOnlyChangeLightsUpTheChain|TestStoryRevisionIsChangedWithBeforeAndAfter)$' -count=1 && /Users/20idemo/.nvm/versions/node/v22.22.3/bin/node /Users/20idemo/.nvm/versions/node/v22.22.3/lib/node_modules/npm/bin/npm-cli.js --prefix e2e test -- --grep 'compares a change'

go test ./internal/server -run '^(TestEachSideListsWhatItIsAbout|TestRelatedReviewsAreDerivedFromTheChangedLines)$' -count=1 && /Users/20idemo/.nvm/versions/node/v22.22.3/bin/node /Users/20idemo/.nvm/versions/node/v22.22.3/lib/node_modules/npm/bin/npm-cli.js --prefix e2e test -- --grep 'renders no approval, comment, or annotation control when comparing a change'
```

## Validation, currency, and transitive paths

At the audited pre-handoff tree:

- `change-saga validate --json app.saga` reports `valid: true`, zero errors, and four pre-existing visual-fragment warnings outside the owned features.
- `change-saga relation status --json app.saga` reports 411 current and 18 explicitly superseded relations, with zero stale relations.
- `change-saga references --json app.saga` reports 438 total/current references, six remapped, and zero stale.
- feature-scoped quality status reports 20/22 for `code-evidence` and 14/17 for `comparison`.
- detailed quality status reports all 34 linked required-kind states as `covered` and the five gaps as `missing_kind`.
- each covered criterion's traceability result contains the direct criterion -> test case -> current passed run path. The five uncovered criteria contain no quality path.
- all 32 current implementation-under-test selectors exactly match Item-owned runtime evidence, while test implementation selectors name only executed test functions.

The traceability queries used the full story URNs; shorthand such as `story://...` is not accepted as an identifier:

```text
/tmp/change-saga-quality-evidence-comparison query traceability --saga app.saga --requirement urn:change-saga:app:story:claims-verification --limit 100
/tmp/change-saga-quality-evidence-comparison query traceability --saga app.saga --requirement urn:change-saga:app:story:companion-repositories --limit 100
/tmp/change-saga-quality-evidence-comparison query traceability --saga app.saga --requirement urn:change-saga:app:story:evidence-repair --limit 100
/tmp/change-saga-quality-evidence-comparison query traceability --saga app.saga --requirement urn:change-saga:app:story:observe-or-compare --limit 100
/tmp/change-saga-quality-evidence-comparison query traceability --saga app.saga --requirement urn:change-saga:app:story:why-things-changed --limit 100
```

## Change Saga CLI dogfooding

What worked well: test-case definitions enforce ordered action/result pairs; evidence batching is atomic; status exposes criterion-kind reasons and current run state; traceability exposes the direct quality path; and reference/relation inspection made stale-state checks deterministic.

Every product-facing rough spot encountered is recorded here.

| Intended goal | Exact command | Observed behavior | Impact | Workaround | Smallest improvement |
| --- | --- | --- | --- | --- | --- |
| Create a coherent quality suite atomically | `/tmp/change-saga-quality-evidence-comparison quality test-case add --help` | Test-case creation and lifecycle changes accept one case per invocation; there is no batch or dry-run mode. | Seven cases required sequential writes and an interruption could leave a partial suite. | Used stable IDs, fail-fast invocations, then validated the complete graph. | Add JSONL `--batch`, `--dry-run`, and all-or-nothing planning for test-case add/revise/set-state. |
| Link one test case to several criteria | `/tmp/change-saga-quality-evidence-comparison relation add --help` | One `--to` target is accepted and relation mutation has no batch mode. | The suite required 32 new relation writes plus two repins; multi-criterion intent is fragmented across commands. | Added one precisely named verifies relation per criterion, then checked per-story traceability and relation currency. | Support an atomic multi-target verifies operation, or general JSONL batching for relations. |
| Add all test and runtime evidence atomically | `/tmp/change-saga-quality-evidence-comparison quality evidence add --batch - --json app.saga` | The write succeeded atomically, but the response repeated each evidence pair in `current_heads`. | The mutation result looked as if heads were duplicated even though the stored graph was correct. | Confirmed head state with quality status and inspected the 14 evidence records. | Deduplicate `current_heads` in the batch response. |
| Select exact test functions durably | `... quality evidence add --test-case inspect-comparison-layers --role test_implementation --code HEAD:internal/gitdiff/gitdiff_test.go#L416-L445 ...` | Quality evidence accepts exact code locations but has no language-aware symbol selector. | Function bounds had to be found and maintained manually. | Located declarations with `rg`/numbered source, selected complete focused functions, and retained exact digests. | Add `--symbol` resolution that stores a symbol hint and canonical range. |
| Execute and record runs without drift | `/tmp/change-saga-quality-evidence-comparison quality run record --test-case resolve-remaps-and-stales --id current-head --result passed --command "go test ..." ...` | Run recording stores a claimed command/result but does not execute it, and has no batch mode. | Execution and recording are separate manual steps, so a caller could accidentally record an unexecuted or mismatched command. | Executed every exact command first, recorded only successful results, and checked current run heads afterward. | Add an opt-in `quality run execute` command that captures exit status, source commit, timestamps, and output digest; add batch import for externally executed runs. |
| Query the v5 quality-coverage projection directly | `/tmp/change-saga-quality-evidence-comparison query quality-coverage --saga app.saga --include tests,runs,evidence,paths` | The CLI reports `unknown query operation: quality-coverage`. | The contract's named projection cannot be requested as a bounded query. | Combined `status --json`'s quality area/detailed projections with per-story `query traceability`. | Implement the documented `quality-coverage` query with tests, runs, evidence, and path inclusion. |
| Request the documented readiness contract version | `/tmp/change-saga-quality-evidence-comparison query readiness --api-version 2 --saga app.saga` | The query rejects `--api-version` as an unknown flag. | A caller cannot explicitly negotiate the v2 readiness shape described by the contract. | Used validation, status, traceability, relations, and references as separate readiness checks. | Add and document `--api-version 2`, or remove the unsupported invocation from the contract. |
| Inspect only one feature's detailed quality records | `/tmp/change-saga-quality-evidence-comparison status --json --feature code-evidence app.saga` | The quality area totals scope correctly to 20/22, but detailed `.quality.criteria` and `.quality.test_cases` remain app-global. | Consumers can mistake app-global detail for feature-filtered detail. | Filtered detailed records by owned criterion/test-case URNs and trusted `.coverage.areas.quality` for feature totals. | Apply the feature filter consistently to detailed quality projections, or label their scope explicitly. |
| Obtain requirement -> quality -> exact test/runtime code in one response | `/tmp/change-saga-quality-evidence-comparison query traceability --saga app.saga --requirement urn:change-saga:app:story:claims-verification --limit 100` | The query emits criterion -> test case -> run paths, while exact quality-evidence selectors remain in their evidence records rather than being appended to those paths. | One response proves the current run link but not both code legs of the quality case. | Paired traceability with quality status/evidence inspection and mechanically compared implementation-under-test selectors with Item evidence. | Append current test-implementation and implementation-under-test selectors, including currency, to quality paths. |

Test-environment rough spot: `hivecontrol exec oneshot 10m -- npm ci` failed with `npm error weird error BuildMessage {}` because the process wrapper substituted Bun for the npm launcher. Running the same install through the explicit Node 22 binary and npm CLI succeeded. An initial Playwright grep with shell-style `^...$` anchors also matched no tests; the exact substring titles shown in the recorded commands were then executed and passed.

## Handoff facts

- Seven active cases, seven current passed runs, 34/39 criteria covered.
- Five criteria remain honestly uncovered; the companion relocation and persistent comparison-mode gaps are unchanged.
- All current quality evidence and direct verifies relations are current; there are no stale references or relations.
- Product code was not changed.
