# App Saga accepted-quality closeout audit

Date: 2026-09-21

## Scope and result

This closeout addresses the five accepted-criterion quality gaps reported at
the start of the work:

- `claims-verification / failed-visible`
- `companion-repositories / move-later`
- `observe-or-compare / same-against`
- `safe-local-review / bounded-requests`
- `safe-local-review / bounded-responses`

Four active automated Saga test cases now verify those criteria through five
current direct `verifies` relations. Each case has one current passed run and
two current evidence records: focused executable test code and the
implementation under test. The closeout tests are pinned to commit
`427ae01f1ccbaa355f23911582b3ed28fd61d7d5`; the canonical companion-relocation
proof and its successor run are pinned to `acc53ef`.

The final app-wide status is quality 146/146 and health 1011/1011. Validation
reports `valid: true` with zero issues. The current evidence graph has no stale
head; the append-only reference history still reports the superseded
`focused-test` selector after the canonical relocation test replaced it.

## Executable evidence

| Saga test case | Criteria | Executable proof |
| --- | --- | --- |
| `failed-claim-verification-history` | `claims-verification / failed-visible` | `TestFailedClaimVerificationRemainsVisibleInHistory` records failed and later verified results, then reads both through `query verifications`. |
| `move-companion-saga-in-repository` | `companion-repositories / move-later` | `TestCompanionSagaLinksSurviveMoveIntoSourceRepository` compares the same Item-to-code projection before and after moving the Saga into the source checkout, then confirms the moved Saga no longer needs a companion sync cursor. |
| `shared-comparison-selection` | `observe-or-compare / same-against` | `TestSelectedComparisonScopesStatusQueryAndReviewer` compares mode, base, head, and object IDs across status, query, and the reviewer change response. |
| `bounded-local-review-resources` | `safe-local-review / bounded-requests`, `safe-local-review / bounded-responses` | Existing `TestHTTPServerHasBoundedResourceSettings` checks finite server limits; existing `TestAsyncReviewSurfacesAreBoundedAndCursorPaginated` checks row caps, byte budgets, cursor progress, and complete exhaustion. |

The combined command used to independently recheck all four cases after the
canonical relocation proof merged was:

```text
hivecontrol exec oneshot 10m -- go test ./internal/cli ./internal/server -run '^(TestFailedClaimVerificationRemainsVisibleInHistory|TestCompanionSagaLinksSurviveMoveIntoSourceRepository|TestSelectedComparisonScopesStatusQueryAndReviewer|TestHTTPServerHasBoundedResourceSettings|TestAsyncReviewSurfacesAreBoundedAndCursorPaginated)$' -count=1
```

Observed result:

```text
ok  github.com/twentyideas/changesaga/internal/cli     1.996s
ok  github.com/twentyideas/changesaga/internal/server  4.642s
```

The move case's current successor run records the narrower command actually
executed against `acc53ef`:

```text
go test ./internal/cli -run TestCompanionSagaLinksSurviveMoveIntoSourceRepository -count=1
```

A broader regression sweep was also run:

```text
hivecontrol exec oneshot 15m -- go test ./...
```

It completed with every package passing except `internal/cli`. That package's
pre-existing `TestInstalledSkillDescribesOneSagaDrivenByStatus` assertion
failed because `skills/change-saga/SKILL.md` omits the expected sentence `The
one thing asked of a change is that its implementation covers it`.
`internal/cli` finished in 124.150s and `internal/server` passed in 425.955s.
The skill file has no diff from source commit
`aef2db858cb1e8c34b2c16d88940009bbb21f186`; this closeout did not edit it.
The focused commands above, which exercise all five closed criteria, remained
green.

## Navigability audit

`query relations --state current` returned all 525 current relations in one
complete page, including the five new links with no stale reasons. For each of
the five criteria, `query traceability --criterion ...` returned exactly the
intended quality path:

```text
criterion -> focused test case -> current passed run
```

This makes every formerly missing criterion navigable forward to its current
case and result. The reverse query from each exact `test_implementation` source
range returned no criterion, however. That is a query-surface limitation rather
than missing evidence: current traceability stops quality paths at the run and
does not traverse a test case's code-evidence records. The authoring receipts,
status projection, current-head health, and forward traceability agree on the
case, evidence, run, and relation state, but source-first quality navigation is
not yet exposed.

## Change Saga CLI dogfooding

Every rough spot encountered during this closeout is recorded below with its
goal, exact command, observed behavior, impact, workaround, and smallest useful
improvement.

| Goal | Exact command | Observed behavior | Impact | Workaround | Smallest improvement |
| --- | --- | --- | --- | --- | --- |
| Read the v5 Saga with the installed executable | `change-saga status --json app.saga`; `change-saga query overview --saga app.saga --repo .`; `change-saga version` | Installed `0.1.1` first reported `Git revision cannot be empty`; the query then disclosed `unsupported Saga version 5; expected 2, 3, or 4`. | The first status error looked like corrupt repository state rather than a CLI/Saga version mismatch. | Built `./cmd/change-saga` to `/tmp/change-saga-quality-closeout`; repository CLI `0.2.0-dev` read and authored v5 correctly. | Validate the Saga format before opening its source range and, on mismatch, name the installed and required versions plus the repository-local build command. |
| Read only the five quality gaps and inspect their cases | `/tmp/change-saga-quality-closeout query -h`; `/tmp/change-saga-quality-closeout query children --saga app.saga --parent urn:change-saga:app:test-case:preserve-claim-verification-history`; `/tmp/change-saga-quality-closeout status --json app.saga` | The query registry has no quality-coverage or test-case operation, `children` returned `target was not found` for a test case, and status emitted roughly 90 KB while omitting case steps and evidence heads. | Routine bounded inspection was noisy; the first terminal projection was truncated, and the supported read API could not show one complete quality case. | Projected `.coverage.areas.quality`, selected cases, and health with `jq`; used per-criterion traceability plus mutation receipts and reference health for detail. | Add paginated `query quality-coverage` and `query test-case` operations, or `status --area quality --summary`, so quality auditing does not require a full app report. |
| Author four cases, activate them, link five criteria, and record four runs atomically | `quality test-case add -h`; `quality test-case set-state -h`; `relation add -h`; `quality run record -h` | None of these mutation families offers batch or dry-run authoring. | The graph was valid but incomplete between independent commands, and interruption recovery depended on orchestration. | Used stable IDs and request IDs, fail-fast command groups, then validated and queried all resulting heads and relations. | Add one atomic quality-plan batch with dry-run support for case definition, lifecycle, relations, and runs. |
| Add eight exact evidence records atomically | `/tmp/change-saga-quality-closeout quality evidence add --batch - --json app.saga` | The write succeeded, but each evidence pair appeared twice in `current_heads` even though `created` and the persisted graph contained eight unique records. | The receipt looks like duplicate evidence heads and cannot be trusted as a concise state projection. | Treated `created` as the mutation receipt and confirmed unique current cases/runs through status and relations/traceability queries. | Deduplicate `current_heads` in batch output and add a regression assertion for one entry per current evidence head. |
| Prove relocation without assuming snapshot identity | `query fragment-diffs --saga <companion> --repo <source> --target <fragment>` before the move, then `query fragment-diffs --saga <source>/app.saga --target <fragment>` | The selector data stayed byte-for-byte equivalent and current, but the query snapshot changed solely across the checkout-placement transition. | A relocation test cannot use snapshot equality to distinguish link preservation from content changes. | Compared the complete returned link projection and asserted every selector remained current. | Document whether companion/in-repository placement is part of snapshot identity, or expose a separate content/evidence snapshot stable across relocation. |
| Navigate from exact test code back to the verified criterion | `query traceability --saga app.saga --ref 427ae01f1ccbaa355f23911582b3ed28fd61d7d5:internal/cli/quality_closeout_test.go#L19-L60 --limit 100` and `query traceability --saga app.saga --ref acc53ef:internal/cli/companion_relocation_test.go#L16-L74 --limit 100` | Every reverse query returned an empty criterion list although forward criterion queries reached the corresponding case and passed run. | Reviewers can navigate criterion-to-test, but not source-test-to-criterion through the supported API. | Audited forward paths per criterion and the five current direct relations; retained exact source selectors in current quality evidence. | Extend traceability and reverse lookup through `test_implementation` and `implementation_under_test` evidence, returning the test case, criterion, and current run. |
