# App Saga implementation evidence and comparison audit

Date: 2026-09-20

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
| `code-evidence` | Revised the existing evidence deck | 4 | 18 | 32/32 | 32/32 | 31/32 |
| `comparison` | Added the comparison implementation deck | 3 | 14 | 17/17 | 17/17 | 17/17 |
| Total | 2 owned decks | 7 | 32 | 49/49 | 49/49 | 48/49 |

The work added 50 precise Item-to-criterion explanation relations. Existing good implementation material on the original reference-resolution slide was preserved and reused for changed-line, moved-line, and documentation-only behavior.

Code references were pinned in two complementary places:

- implementation Items contain the source and focused tests that substantiate each reviewer-facing claim;
- 35 of 37 owned design landmarks contain direct references to the named runtime seams they describe.

The two design landmarks without direct code are explicit gaps, not accidental omissions: `portable-evidence` and `mode-indicator`. In total, 113 new pinned references were added (67 on implementation Items and 46 on design landmarks), taking repository-wide reference health from 56 references to 169.

The references favor named seams such as `criterionInputs`, `reviewEvidenceIndex`, `AnalyzeGraph`, `EvaluateRelation`, `Resolve`, `ReplaceCoverage`, `AddClaim`, `VerifyClaim`, `VerifyRepository`, `Sync`, `Open`, `companionSides`, `ReadRange`, `codeLayer`, `pair`, `attachReasons`, `NodeHistory`, and `branchCommits`. Runtime and test evidence are separate records so a broad multi-file selector does not obscure ownership.

## Honest implementation gaps

1. `companion-repositories / move-later` has design and an explicit implementation Item, but no code reference. Portable URNs, digests, repository identity, and sync cursors exist; there is no dedicated move preflight or end-to-end relocation test for moving a Saga and later reconnecting it to source. This is the sole owned criterion without code evidence.
2. The comparison range and all three layers are implemented, but the browser does not yet provide the complete persistent current-versus-comparison mode label and deep-link restoration contract described by `mode-indicator`. The slide calls this out without denying the implemented range-selection behavior.
3. The repository-wide quality axis remains 2/146. This change maps implementation and tests; it does not fabricate quality test-case/run records.
4. One stale reference remains under `reviewer-app`, outside this scope: `two-sides-sidebar.json#1` for `internal/server/appnav.go` lines 101-109. It was preserved rather than silently repaired across feature ownership.

## Traceability inspection

For each owned story, `query traceability` reports:

| Story | Criteria | With design | With review target | With code evidence |
| --- | ---: | ---: | ---: | ---: |
| `evidence-traversal` | 10 | 10 | 10 | 10 |
| `evidence-repair` | 7 | 7 | 7 | 7 |
| `claims-verification` | 9 | 9 | 9 | 9 |
| `companion-repositories` | 6 | 6 | 6 | 5 |
| `observe-or-compare` | 10 | 10 | 10 | 10 |
| `why-things-changed` | 7 | 7 | 7 | 7 |

The explicit criterion-to-Item-to-code paths are truthful. Design landmarks were independently checked with `query fragment-diffs`; in observe mode their selectors resolve as `current` while changed-atom counts are correctly zero.

Final observe-mode status before merge:

| Axis | Covered | Total | Notes |
| --- | ---: | ---: | --- |
| Stories | 75 | 75 | Every implementation code target reaches a story |
| Personas | 75 | 75 | Every implementation code target reaches a persona |
| Design | 18 | 18 | Baseline coverage preserved |
| Quality | 2 | 146 | Baseline unchanged |
| Health | 459 | 460 | Sole miss is the out-of-scope stale reviewer-app reference |
| Changed-line implementation | 0 | 0 | Correct for observe mode; no product diff is being claimed |

Feature-scoped status is 47/47 story and persona code targets with 154/154 health for `code-evidence`, and 25/25 story and persona code targets with 82/83 health for `comparison`. The comparison health miss is the same cross-feature stale reviewer-app reference.

## Validation and tests

`change-saga validate --json app.saga` reports `valid: true`, zero errors, and seven warnings. All seven warnings are pre-existing visual fragments outside the two owned features; no new validation warning was introduced.

`change-saga references --json app.saga` reports 169 total, 168 current, 6 remapped, and 1 stale. All 113 references introduced here resolve current at the pinned baseline.

Focused package tests passed:

```text
hivecontrol exec oneshot 10m -- go test ./internal/coderef ./internal/coderesolve ./internal/coverage ./internal/requirements ./internal/impact ./internal/livingapp ./internal/reviewapp ./internal/gitdiff ./internal/changeview ./internal/gitattribution ./internal/cli ./internal/server
```

`internal/changeview` has no colocated test files; the behavior is exercised through the focused CLI, server, git-diff, living-app, and review-app tests included in the command.

All eight slides in the revised/new owned decks (the preserved original plus seven new slides) were rendered through macOS Quick Look at a 1280-pixel preview. Visual checks covered hierarchy, legibility, reading order, selector-to-element identity, narrative continuity, and explicit gap presentation. One overlap on the companion-repository slide was found, revised through `set-slide-content`, and re-rendered successfully.

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
| Rank weak mappings while observing the current tree | `/tmp/change-saga-implementation-evidence-comparison query mappings --saga app.saga --sort scrutiny --limit 10` | The returned note says that every score is zero because there is no change, while the first mapping correctly has score 50 for a stale selector. Breadth signals are zero in observe mode, but non-change-dependent signals are not. | The note contradicts the structured result and can mislead an auditor about stale or thin mappings. | Trusted each mapping's `scrutiny_score` and `reasons`, and separately inspected `references --stale --diff`. | Change the note to distinguish change-dependent breadth signals from always-on stale/note signals. |

## Handoff facts

- No implementation is claimed for the companion move-later workflow.
- No complete browser mode-label/deep-link contract is claimed.
- No product source was changed.
- The only stale reference and all seven validation warnings remain outside the owned features.
