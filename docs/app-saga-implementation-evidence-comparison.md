# App Saga implementation evidence and comparison audit

Date: 2026-09-21

## Scope and method

This audit covers only `app.saga/___features/code-evidence.feature`, `app.saga/___features/comparison.feature`, and this file. No product source or other feature directory was edited.

The implementation map was built from:

- `docs/app-saga-code-test-map.md`
- `docs/app-saga-design-evidence-comparison.md`
- the accepted current stories and criteria returned by `change-saga query requirements`
- every fragment and landmark in the two owned features, read through the query API
- the named Go types/functions and focused tests identified by the code/test audit

A fresh repository-local CLI was built with:

```text
hivecontrol exec oneshot 2m -- go build -o /tmp/change-saga-implementation-evidence-comparison ./cmd/change-saga
```

All Saga reads and mutations in this work used that binary. DayLight Local lookup was attempted before authoring, but the DayLight desktop service was not running, so no earlier conversational context was available; this is not evidence that earlier work does not exist.

## Authored evidence model

| Feature | Deck work | New slides | New Items | Accepted criteria with design | Accepted criteria with review target | Accepted criteria with code |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| `code-evidence` | Revised the existing evidence deck | 4 | 18 | 32/32 | 32/32 | 32/32 |
| `comparison` | Added the comparison implementation deck | 3 | 14 | 17/17 | 17/17 | 17/17 |
| Total | 2 owned decks | 7 | 32 | 49/49 | 49/49 | 49/49 |

The work added 50 precise Item-to-criterion explanation relations. Existing good implementation material on the original reference-resolution slide was preserved and reused for changed-line, moved-line, and documentation-only behavior.

Code references were pinned in two complementary places:

- implementation Items contain the source and focused tests that substantiate each reviewer-facing claim;
- 35 of 37 owned design landmarks contain direct references to the named runtime seams they describe.

The two design landmarks without direct code are `portable-evidence` and `mode-indicator`; criterion traceability instead runs through their implementation Items. The original pass added 113 pinned references (67 on implementation Items and 46 on design landmarks). This closeout adds five focused references for `move-later`; after later quality and implementation work, the repository-wide total is 662 current references.

The references favor named seams such as `criterionInputs`, `reviewEvidenceIndex`, `AnalyzeGraph`, `EvaluateRelation`, `Resolve`, `ReplaceCoverage`, `AddClaim`, `VerifyClaim`, `VerifyRepository`, `Sync`, `Open`, `companionSides`, `ReadRange`, `codeLayer`, `pair`, `attachReasons`, `NodeHistory`, and `branchCommits`. Runtime and test evidence are separate records so a broad multi-file selector does not obscure ownership.

## Implementation boundaries

1. `companion-repositories / move-later` is implemented by location-independent code-reference pins, explicit-versus-implicit checkout selection, and same-checkout detection. The earlier audit incorrectly treated a dedicated move preflight as part of the accepted criterion. Five references now connect the criterion's Item to those seams and to a focused end-to-end relocation test; no separate preflight is claimed.
2. The comparison range and all three layers are implemented, but the browser does not yet provide the complete persistent current-versus-comparison mode label and deep-link restoration contract described by `mode-indicator`. The slide calls this out without denying the implemented range-selection behavior.
3. The repository-wide quality axis is 141/146. This closeout maps implementation and existing test code; it does not create a quality test-case/run record for `move-later`.
4. Before the required merge from `main`, one stale reference remained under `reviewer-app`, outside this scope: `two-sides-sidebar.json#1` for `internal/server/appnav.go` lines 101-109. It was preserved rather than silently repaired across feature ownership. The upstream merge repaired it; final repository-wide reference health is now fully current.

## Traceability inspection

For each owned story, `query traceability` reports:

| Story | Criteria | With design | With review target | With code evidence |
| --- | ---: | ---: | ---: | ---: |
| `evidence-traversal` | 10 | 10 | 10 | 10 |
| `evidence-repair` | 7 | 7 | 7 | 7 |
| `claims-verification` | 9 | 9 | 9 | 9 |
| `companion-repositories` | 6 | 6 | 6 | 6 |
| `observe-or-compare` | 10 | 10 | 10 | 10 |
| `why-things-changed` | 7 | 7 | 7 | 7 |

The explicit criterion-to-Item-to-code paths are truthful. Design landmarks were independently checked with `query fragment-diffs`; in observe mode their selectors resolve as `current` while changed-atom counts are correctly zero.

Final observe-mode status after merging `main`:

| Axis | Covered | Total | Notes |
| --- | ---: | ---: | --- |
| Stories | 145 | 145 | Every implementation code target reaches a story |
| Personas | 145 | 145 | Every implementation code target reaches a persona |
| Design | 18 | 18 | Baseline coverage preserved |
| Quality | 141 | 146 | Five known quality-growth gaps remain outside this implementation closeout |
| Health | 1002 | 1002 | All relation and reference health checks pass |
| Changed-line implementation | 0 | 0 | Correct for observe mode; no product diff is being claimed |

Feature-scoped status is 48/48 story and persona code targets with 180/180 health for `code-evidence`, and 25/25 story and persona code targets with 108/108 health for `comparison`.

The paginated repository-wide `query traceability` result returned 100 and 46 criteria under one snapshot. All 146 accepted criteria have design, an implementation target, code evidence, and at least one navigable criterion -> implementation Item -> code path.

## Validation and tests

`change-saga validate --json app.saga` reports `valid: true` with zero issues.

`change-saga references --json app.saga` reports 662 total, 662 current, 10 remapped, and 0 stale. All five references introduced by this closeout are current.

`change-saga relation status --json app.saga` reports 539 relations: 520 current and 19 explicitly superseded, with zero stale, conflicted, or invalid relations. The old gap relation is superseded by the current portability explanation.

Focused package tests passed:

```text
hivecontrol exec oneshot 10m -- go test ./internal/coderef ./internal/coderesolve ./internal/coverage ./internal/requirements ./internal/impact ./internal/livingapp ./internal/reviewapp ./internal/gitdiff ./internal/changeview ./internal/gitattribution ./internal/cli ./internal/server
hivecontrol exec oneshot 10m -- go test ./internal/cli -run '^TestCompanionSagaLinksSurviveMoveIntoSourceRepository$' -count=1
```

`internal/changeview` has no colocated test files; the behavior is exercised through the focused CLI, server, git-diff, living-app, and review-app tests included in the command.

All eight slides in the revised/new owned decks (the preserved original plus seven new slides) were rendered through macOS Quick Look at a 1280-pixel preview. Visual checks covered hierarchy, legibility, reading order, selector-to-element identity, and narrative continuity. The companion-repository slide was re-rendered after replacing the obsolete gap callout with the evidenced portability behavior; it remains legible without overlap.

## Change Saga CLI dogfooding

What worked well:

- `cover --batch - --dry-run --json` resolved an entire JSONL batch before writing. A failing selector would leave the Saga untouched, and the successful run returned structured record/reference counts.
- `references --json` made current/remapped/stale accounting deterministic.
- `query traceability`, `query fragment-diffs`, and `query slide` exposed bounded structured views suitable for auditing without reading Saga metadata files directly.
- Mutation errors were structured when `--json` was supported and preserved the Saga on failure.

Every rough spot encountered is recorded below.

| Intended goal | Exact command | Observed behavior | Impact | Workaround | Smallest product improvement |
| --- | --- | --- | --- | --- | --- |
| Relate an implementation Item directly to the design landmark it realizes | `/tmp/change-saga-implementation-evidence-comparison relation add --feature code-evidence --id forward-trace-implements-evidence-path --type implements --from urn:change-saga:app:slide:evidence-paths:item:forward-trace --to urn:change-saga:app:fragment:evidence-traversal:landmark:evidence-path --rationale 'The implementation Item realizes the forward traversal design.' --json app.saga` | The command correctly rejected the relation: `implements relation requires a work-item source and report design or criterion target`. The endpoint matrix is not discoverable from `relation add --help`; it is learned only after attempting the mutation. | There is no supported single edge from a review Item to its design landmark, so one query cannot follow Item -> design. | Added supported Item-to-criterion `explains` relations and pinned the same focused runtime seam directly on the corresponding design landmark. | Print the allowed source/target kinds for every relation type in help/schema output, and add a first-class review-Item-to-design relation if that semantic link is intended. |
| Add many criterion relations atomically | `/tmp/change-saga-implementation-evidence-comparison relation add --help` | `relation add` accepts one relation per invocation and has no batch or dry-run mode. | Authoring 50 relations required 50 mutations; an interruption can leave a valid but partially mapped graph. | Used stable IDs, fail-fast shell execution, then re-ran validation, status, and per-story traceability. | Add JSONL `--batch`, `--dry-run`, and all-or-nothing planning to `relation add`, matching `cover`. |
| Consume structured results from every hierarchy mutation | `/tmp/change-saga-implementation-evidence-comparison add-slide --help` (the same applies to `add-deck` and `add-item`) | These hierarchy mutations do not expose `--json`, while relation, content, and coverage mutations do. | Automation must parse prose or issue follow-up queries to discover the created target/path. | Supplied stable IDs and confirmed the resulting URNs with `query children`/`query slide`. | Give all mutations the same JSON envelope containing operation, target, path, created resources, and replay status. |
| Select durable named Go seams instead of manually maintaining line ranges | `/tmp/change-saga-implementation-evidence-comparison cover --target urn:change-saga:app:fragment:evidence-traversal:landmark:evidence-path --path internal/livingapp/compose.go --commit HEAD --lines 350-453 --name criterion-inputs --note 'criterionInputs assembles accepted criteria with their design, test, and implementation evidence.' app.saga` | Coverage selection supports file/range/reference locations, but not Go symbols. The digest makes the pin trustworthy and remapping handles pure movement, yet the author must locate function bounds manually. | Selecting many focused functions is accurate but tedious, and later function growth may require a deliberate refresh. | Audited named declarations with `rg`, selected complete focused bodies, separated runtime/tests, and retained the function name in record name and note. | Add language-aware `--symbol` resolution that emits the canonical line range and stores a symbol hint alongside the digest. |
| Inspect requirement -> design -> code as one transitive path | `/tmp/change-saga-implementation-evidence-comparison query traceability --saga app.saga --requirement evidence-traversal` | The response includes criterion-to-design paths and criterion-to-review-Item-to-code paths, but design-owned code is not appended to the design paths. | A consumer cannot prove the design landmark's direct code evidence from this one response, even though the reference exists and is current. | Paired `query traceability` with `query fragment-diffs --target <design-landmark-urn>` for each owned design landmark. | Include current design-owned code selectors as criterion -> design -> code paths, with stale/remapped state, in traceability output. |
| Rank weak mappings while observing the current tree | `/tmp/change-saga-implementation-evidence-comparison query mappings --saga app.saga --sort scrutiny --limit 10` | Before the final merge, the returned note said that every score was zero because there was no change, while the first mapping correctly had score 50 for the then-stale out-of-scope selector. Breadth signals were zero in observe mode, but non-change-dependent signals were not. After the upstream repair merged, all observe-mode scores are zero and the note is accurate for the final tree. | During a stale state, the note contradicted the structured result and could mislead an auditor about stale or thin mappings. | Trusted each mapping's `scrutiny_score` and `reasons`, and separately inspected `references --stale --diff`. | Change the note to distinguish change-dependent breadth signals from always-on stale/note signals. |
| Use the installed CLI named by the skill | `change-saga --help` followed by `change-saga spec --json` | The installed binary described the obsolete v2 format and command surface while the repository and bundled skill use v5. It could not safely query or mutate this Saga. | A well-configured agent can still choose an incompatible executable and receive misleading format guidance before its first query. | Built the repository CLI once with `go build -o /tmp/change-saga-closeout ./cmd/change-saga` and used that binary consistently. | Have `change-saga` detect a newer checkout-local CLI/schema and print the exact compatible invocation, or make `--version`/`spec` expose an explicit compatibility error before Saga access. |
| Exercise relocation from a test with a long descriptive name | `go test ./internal/cli -run TestCompanionSagaLinksSurviveMoveIntoSourceRepository -count=1` | The first run failed while creating the fixture: the platform temporary prefix plus the test name made the embedded deck's absolute path exceed the portable 240-character budget. The Saga content itself was compact. | A platform-dependent fixture path prevented the end-to-end portability behavior from being tested and initially looked like a product failure. | Reused the package's `shortTempDir` fixture helper, which keeps the absolute test path below the portable budget. | When rejecting an overlong absolute path, report the measured length, budget, and the user-controlled suffix separately; provide a documented short-temp helper for integration tests. |
| Distinguish navigable code coverage from delivery readiness | `/tmp/change-saga-closeout query traceability --saga app.saga --criterion urn:change-saga:app:story:companion-repositories:criterion:move-later` | After the criterion had an active Item-to-code path, `code_evidence` and `paths` were populated but `blockers` still reported `immutable_evidence_missing` and `work_item_missing`, and `delivered` remained false. | A caller asking whether a requirement can be followed to code can mistake independent work-plan/readiness blockers for a broken traceability path. | Evaluated `paths` and `code_evidence` for navigation, then used status health separately; treated `delivered` as the stricter planning/readiness projection. | Group blockers by axis (`traceability`, `plan`, `delivery`) or add an explicit `navigable` boolean alongside the stricter `delivered` field. |
| Confirm an exact coverage deletion | `/tmp/change-saga-closeout remove-coverage --record ___features/code-evidence.feature/___slides/code-evidence-implementation.deck/40-e-1d7e04d0fb5d-261d92f42010.json --json app.saga` | The command removed the named evidence file and reported its path, but its structured summary said `records: 0`, `references: 0`, and `evidence_files: null`. The same zero counts appeared in dry-run output. | Automation cannot use the mutation receipt to confirm how many records or references were removed, and the successful result resembles a no-op. | Re-queried mappings for the Item and validated the Saga after deletion. | Return `records: 1`, the deleted selector count, and `evidence_files: [<path>]` for both dry-run and successful deletion. |
| Inspect a bounded status summary | `/tmp/change-saga-traceability-closeout status --json app.saga` | The complete response exceeded 100,000 tokens because it embedded every coverage entry and generated next action. | Ordinary terminal inspection truncated output and obscured the few counts needed for verification. | Redirected once to a temporary file and projected exact area counts and targeted actions with `jq`; used paginated `query traceability` for criterion detail. | Add `status --summary` or an area projection flag, leaving detailed lists to paginated query operations. |

## Handoff facts

- Companion-to-source relocation is covered by a focused end-to-end CLI test:
  the same Item resolves to the same current code link before and after the
  Saga moves, and the moved Saga uses its containing checkout without
  `--repo`.
- No complete browser mode-label/deep-link contract is claimed.
- No product source was changed.
- Final reference health is 662/662; relation health has zero stale, conflicted, or invalid entries; validation has zero issues.
