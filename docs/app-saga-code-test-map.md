# App Saga code and test map

Status: repository audit for the canonical `app.saga`; research only. This
document maps the implementation and its executable tests to the product model
that already exists in `app.saga`. It does not add requirements, choose a
design, or claim that current behavior is necessarily the desired behavior.

Audit snapshot: source commit `8059a17f19c6e4462d9a23429de0f65da6883770`
on 2026-09-20. `change-saga validate --json app.saga` reported a valid Saga.
`go test ./...` passed at this snapshot.

## How to use this map

The requirements pass should decide which existing stories and acceptance
criteria remain canonical. Only after that decision should a design node,
quality test case, implementation Item, or code reference be authored in the
Saga. The targets below are candidates for that later work; they are not a
substitute for the requirements decision.

When a target is accepted, prefer a narrow named function, type, route
registration block, or named test over a whole file. Create the actual pin with
the CLI so the commit, inclusive line range, and digest are canonical. File and
test names in this document are navigation aids, not persisted code references.

## Product-domain map

The canonical Saga currently has seven feature domains, 18 stories, and 69
acceptance criteria.

| Saga feature | User-visible capability already present in code | Primary implementation seams | Strong existing tests | Authored Saga gap observed in this audit |
| --- | --- | --- | --- | --- |
| `format` — Saga format and app structure | One v5 app Saga; overview, personas, flags, feature roots; durable IDs; strict loading and validation; report/deck composition | [`internal/applayout/applayout.go`](../internal/applayout/applayout.go), [`internal/saga/load.go`](../internal/saga/load.go), [`internal/saga/app.go`](../internal/saga/app.go), [`internal/saga/validate.go`](../internal/saga/validate.go), [`internal/livingid/urn.go`](../internal/livingid/urn.go), [`internal/store/store.go`](../internal/store/store.go) | `TestFeaturesListValidatesAndResolves`, `TestFeatureRootsAtTheAppRootAreRefused`, Saga schema/loader/hardening/path tests, store atomicity and lock tests, `feature-list.spec.ts`, `section-directories.spec.ts` | No design chapter or implementation deck. Both stories (`one-app-saga`, `durable-features`) are among the design gaps. |
| `requirements` — Requirements and traceability | Personas, flags, stories, criteria, citations, pinned typed relations, prototypes, terms, traceability/readiness projections, quality-domain records | [`internal/requirements`](../internal/requirements), [`internal/prototypes`](../internal/prototypes), [`internal/quality`](../internal/quality), [`internal/livingapp`](../internal/livingapp), [`internal/readiness`](../internal/readiness), [`internal/coverage/axes.go`](../internal/coverage/axes.go), [`internal/vocabulary`](../internal/vocabulary) | Requirements schema/mutation/currency/repin tests; prototype and quality tests; `TestTraceability...` and status tests; `requirements.spec.ts`; `terms.spec.ts` | No design chapter or implementation deck. Both stories (`overview-and-terms`, `traceability-chain`) are design gaps; none of their criteria has a canonical quality relation. |
| `code-evidence` — Code references and coverage | Canonical repository/location parsing; line and whole-file digests; moved-line remapping and stale detection; diff atoms; mapping coverage; impact; cover/replace/remove/repin/sync workflows | [`internal/coderef`](../internal/coderef), [`internal/coderesolve/resolve.go`](../internal/coderesolve/resolve.go), [`internal/gitdiff`](../internal/gitdiff), [`internal/coverage/coverage.go`](../internal/coverage/coverage.go), [`internal/impact/impact.go`](../internal/impact/impact.go), [`internal/cli/cover.go`](../internal/cli/cover.go), [`internal/cli/references.go`](../internal/cli/references.go) | Code-reference and resolver tests; 17 coverage tests; 17 Git-diff tests; CLI cover/range/reference/repair tests; mapping tests; `cli.spec.ts`, `bounded-surfaces.spec.ts`, and the code/coverage flows in `navigation.spec.ts` | Has the only feature implementation deck, one design chapter, and the only quality test case. `changed-lines-accounted` and `companion-repositories` remain design gaps. Only 2 of this domain's criteria have canonical `verifies` relations. |
| `comparison` — Observe, compare, and history | Merge-base comparison; Changed/Affected/Code layers; record inventory and pairing; commit reasons; node history | [`internal/changeview`](../internal/changeview), [`internal/gitdiff/opening.go`](../internal/gitdiff/opening.go), [`internal/gitattribution`](../internal/gitattribution), [`internal/cli/query_layers.go`](../internal/cli/query_layers.go), [`internal/server/layers.go`](../internal/server/layers.go) | CLI layer/history tests, Git attribution tests, review-app/session tests, server comparison tests, `@critical compares a change...` in `cli.spec.ts` | No design chapter, implementation deck, or quality records. Both stories (`observe-or-compare`, `why-things-changed`) are design gaps. `internal/changeview` has no colocated unit-test file; its behavior is exercised through higher layers. |
| `agent-loop` — Status, check, and the agent loop | Six-area coverage report; no-verdict status; scoped `check`; deterministic next actions; command grammar; machine-readable query envelope; installable skill | [`internal/areas/areas.go`](../internal/areas/areas.go), [`internal/nextaction`](../internal/nextaction), [`internal/grammar`](../internal/grammar), [`internal/livingapp/status.go`](../internal/livingapp/status.go), [`internal/cli/check.go`](../internal/cli/check.go), [`internal/cli/status_grammar.go`](../internal/cli/status_grammar.go), [`internal/cli/query.go`](../internal/cli/query.go) | Area-evaluation tests; 23 next-action tests; grammar parity tests; status/check/query envelope and integration tests; command exit-code tests in [`cmd/change-saga/main_test.go`](../cmd/change-saga/main_test.go) | Has a design chapter but no implementation deck or quality records. `first-change` and `growth-not-debt` are design gaps. The chapter addresses `coverage-report` and `check-covers`. |
| `reviews` — Pull request reviews | One review per PR; review deck and range; slide decisions and comments; decision currency; review coverage; merge freeze/history | [`internal/saga/review.go`](../internal/saga/review.go), [`internal/reviewstore/reviewstore.go`](../internal/reviewstore/reviewstore.go), [`internal/reviewstate`](../internal/reviewstate), [`internal/cli/reviews.go`](../internal/cli/reviews.go), [`internal/server/reviews.go`](../internal/server/reviews.go), [`internal/server/reviewcode.go`](../internal/server/reviewcode.go) | Saga/review-store/CLI review tests; server review decision, currency, frozen-review, and coverage tests; `pull-request-review.spec.ts`; documentation/security negative paths | No design chapter, implementation deck, or quality records. Both stories (`review-is-pr-deck`, `documentation-not-approval`) are design gaps. `internal/reviewstate` has no colocated unit-test file; callers exercise it. |
| `reviewer-app` — The reviewer application | Local loopback server; Documentation/Review sides; feature, requirement, term, test, deck, review, code, coverage, and history surfaces; bounded lazy loading; accessible navigation | [`internal/reviewapp`](../internal/reviewapp), [`internal/server`](../internal/server), [`internal/snapshotcache`](../internal/snapshotcache), [`internal/cli/opening.go`](../internal/cli/opening.go), [`internal/cli/server_runtime.go`](../internal/cli/server_runtime.go) | 142 server tests/benchmarks; 26 review-app tests/benchmarks; 12 cache tests; 40 Playwright scenarios covering navigation, security, accessibility, performance, documentation, and reviews | Has the `two-sides` design chapter but no implementation deck or quality records. `open-observe-or-compare` and `sidebar` are design gaps. One of its three landmark code records is currently stale. |

### Implemented capabilities not cleanly owned by a current story

The repository also implements substantial surfaces that are only indirectly
mentioned, or not bounded by a current app.saga criterion. The requirements
pass should decide whether these belong in an existing domain, deserve a new
story, or are intentionally out of the canonical product model:

- work-plan waves, items, dependencies, contracts, progress, workspace
  assignments, and merge evidence (`internal/workplan`, `plan ...` commands,
  and the corresponding query operations);
- author claims and append-only verifications (`add-claim`, `verify-claim`,
  query `claims`/`verifications`);
- prototype mutation and coverage behavior beyond the high-level
  requirements-domain description;
- quality policies, evidence, runs, and coverage exceptions beyond the
  traceability story;
- release archive construction, installers, signing/notarization, and version
  metadata;
- cache behavior, performance budgets, accessibility, and HTTP hardening as
  explicit product-quality obligations.

This list is a discovery result, not a proposal to add those requirements.

## Package and boundary map

The most useful architecture split is domain logic, composition, and adapters.

| Layer | Packages | Responsibility and product mapping |
| --- | --- | --- |
| On-disk app/report model | `applayout`, `saga`, `store`, `livingid`, `qualityid`, `sagaref` | Physical layout, strict reads, validation, atomic writes, stable URNs, and portable Saga references. Primarily `format`; shared by every domain. |
| Product and quality records | `requirements`, `prototypes`, `quality`, `workplan`, `vocabulary` | Append-only definitions/events and their domain validation. Primarily `requirements`; work-plan ownership is unresolved in current app.saga. |
| Source truth and evidence | `coderef`, `coderesolve`, `gitdiff`, `coverage`, `impact`, `gitattribution` | Repository verification, canonical locations, remapping/staleness, diff atoms, code ownership, incoming impact, and attribution. Primarily `code-evidence` and `comparison`. |
| Derived status and guidance | `areas`, `readiness`, `livingapp`, `nextaction` | Cross-domain graph composition, independent coverage/readiness axes, status facts, and ordered agent actions. Primarily `agent-loop` and `requirements`. |
| Comparison and review application | `changeview`, `reviewapp`, `reviewstate`, `reviewstore` | Changed/Affected/Code projection; bounded read API; PR decision/comment state; review mutations. `comparison`, `reviews`, and `reviewer-app`. |
| User adapters | `cli`, `server`, `cmd/change-saga` | Flag/JSON/HTTP/browser contracts. Domain policy should remain in lower packages. All seven domains. |
| Scale and delivery support | `snapshotcache`, `testfixture`, `querytest`, `releasearchive`, `internal/cmd/releasearchive`, `skills` | Derived-state caching, deterministic large/real-Git fixtures, packaging, and embedded agent skill. Mostly cross-cutting; several are not directly owned by current stories. |

Notable test-boundary gaps are `internal/changeview` and
`internal/reviewstate`, which have no colocated `_test.go` files, and
`internal/cmd/releasearchive`, whose behavior is tested through the
`internal/releasearchive` package and scripts. This is not proof of missing
behavior coverage, but it makes failures harder to localize.

## CLI surface map

[`internal/grammar/commands.go`](../internal/grammar/commands.go) and
[`internal/grammar/commands_content.go`](../internal/grammar/commands_content.go)
are the best stable registry for command names, flags, mutation status, and
written resource kinds. [`cmd/change-saga/main.go`](../cmd/change-saga/main.go)
is the process dispatcher and exit-code boundary.

| Capability | Commands |
| --- | --- |
| Bootstrap and self-description | `init`, `install-skill`, `spec`, `version`, `help` |
| App identity and overview | `feature add`, `overview set-pitch`, `overview set-description` |
| Personas, terms, flags | `persona add/revise/set-state`, `term add/revise/set-state`, `flag add/revise/set-state` |
| Stories and provenance | `story add/revise/move/set-state`, `criterion add/revise/remove`, `citation add` |
| Typed traceability | `relation add/repin/supersede/status` |
| Prototypes | `prototype add-html/add-external/revise/annotate` |
| Design/report authoring | `design add-chapter/add-section/add-fragment/set-fragment-content`; top-level chapter/section/fragment commands remain report-authoring aliases |
| Deck authoring | `add-deck`, `add-slide`, `set-slide-content`, `add-item`, the corresponding `revise-*`/`remove-*`, and `add-landmark` |
| Code evidence | `cover`, `remove-coverage`, `replace-coverage`, `references`, `repin`, `sync` |
| Claims | `add-claim`, `verify-claim` |
| Quality | `quality test-case add/revise/set-state`, `quality policy set`, `quality evidence add`, `quality run record`, `coverage-exception add/supersede` |
| Work plan | `plan add-wave/revise-wave/add-item/revise-item/add-dependency/add-contract/assign/progress/record-merge` |
| Pull-request review | `review create/list/approve/request-changes/withdraw/comment` |
| Read and decision support | `validate`, `status`, `check`, `query`, `open`, `serve` |

`query` is a public product surface, not an internal debugging command. Its
single `change-saga.ai/v1` envelope currently exposes 25 operations:
`schema`, `overview`, `children`, `fragment`, `fragment-diffs`, `slide`,
`slide-diffs`, `diff-owners`, `gaps`, `mappings`, `claims`, `verifications`,
`requirements`, `requirement-history`, `citations`, `relations`, `waves`,
`work-items`, `work-events`, `work-conflicts`, `traceability`, `readiness`,
`layers`, `history`, and `terms`. The operation registry and purposes in
[`internal/cli/query.go`](../internal/cli/query.go) are better targets than
duplicated help prose.

## Server surface map

The route registration block is `newMux` in
[`internal/server/server.go`](../internal/server/server.go). It is the most
compact reference for the HTTP boundary; focused handler files are better
targets for behavioral claims.

| Surface | Routes | Product capability |
| --- | --- | --- |
| Documentation pages | `GET /`, `/features`, `/features/{feature}`, `/personas`, `/personas/{persona}`, `/terms`, `/terms/{term}`, `/requirements`, `/requirements/{story}`, `/requirements/{story}/criteria/{criterion}`, `/chapters/{chapter}`, `/tests/{test}`, `/flags`, `/design-system` | Browse the app, its feature domains, requirements, terms, personas, design, quality, and implementation. |
| Review pages | `GET /reviews`, `/reviews/{id}`, `/reviews/{id}/code`, `/reviews/{id}/file-diff`, `/reviews/{id}/coverage`, `/reviews/{id}/visual/{slide}` | Review-side index, deck, code, coverage, and slide assets. |
| Review mutations | `POST /reviews/{id}/decision`, `POST /reviews/{id}/comment` | The only browser write surface: review decisions and discussion. Documentation pages have no write controls or endpoints. |
| Bounded comparison APIs | `GET /api/code`, `/api/coverage`, `/api/totals`, `/api/reference-code`, `/api/layers`, `/api/change`, `/api/history`, `/api/coverage-file`, `/api/coverage-target`, `/api/file-diff`, `/api/target-code`, `/api/file-owners` | Lazy, cursor-bounded code, layer, history, and bidirectional coverage surfaces. |
| Bounded narrative APIs | `GET /api/section`, `/api/fragment`, `/api/locate` | Load one authored node or locate one anchor without building the complete comparison. |
| Runtime management | `GET /api/runtime`, `POST /api/runtime-stop` | Managed detached server health and authenticated shutdown. |
| Static/sandboxed content | `GET /app.js`, `/theme.js`, `/f/{id}/{path...}` | Browser behavior, theme bootstrap, and fragment assets constrained to the Saga root and sandbox policy. |

The server refuses non-loopback listening, rejects foreign `Host` and
cross-site requests, bounds HTTP resources and pagination, keeps filesystem
paths out of browser responses, and requires a token for managed shutdown.
These are implemented and tested behaviors; the requirements pass must decide
which become canonical product criteria.

## Schema and storage contracts

The current v5 Saga is a composite format. Component schema versions identify
record families; they are not separate running products.

| Schema generation | Persisted concepts | Main runtime owners |
| --- | --- | --- |
| v1 | Legacy Saga, coverage, comment, and review records | Compatibility/history only; not accepted as the root manifest by the current loader |
| v2 | Chapter, section, fragment, landmark, exact code evidence, claims, verifications, merge records, and the report-era manifest | `internal/saga`, `internal/coderef` |
| v3 | Stories/revisions/events, citations, prototypes/annotations, relations, and work-plan identities/revisions/events | `internal/requirements`, `internal/prototypes`, `internal/workplan`, `internal/livingid` |
| v4 | Flat deck, slide, and Item bundles | `internal/saga` deck/slide loaders and mutations |
| v5 | Root app manifest, feature/persona/flag/term records, relation extensions/repins, quality records, coverage exceptions, reviews/comments/approvals, sync cursor, and query-v2 schema | `applayout`, `requirements`, `quality`, `reviewstore`, `saga`, CLI query adapters |

Best schema pins are the schema file itself plus its runtime parity test, not an
example alone. Relevant contract tests include
[`internal/saga/schema_v5_contract_test.go`](../internal/saga/schema_v5_contract_test.go),
[`internal/requirements/app_schema_test.go`](../internal/requirements/app_schema_test.go),
[`internal/requirements/schema_test.go`](../internal/requirements/schema_test.go),
[`internal/prototypes/schema_test.go`](../internal/prototypes/schema_test.go), and
the quality schema tests in [`internal/quality/quality_test.go`](../internal/quality/quality_test.go).

## Existing executable test coverage

At the audited snapshot the repository contains:

- 138 Go `_test.go` files with 676 `Test...` functions, one fuzz target, and
  16 benchmarks;
- 40 Playwright scenarios across 13 spec files, 22 marked `@critical`;
- CI that runs `go vet` and `go test` on Linux, macOS, and Windows, with the
  race detector on Linux/macOS;
- browser CI across Chromium, Firefox, and WebKit, followed by three repeats of
  all critical flows;
- installer, release reproducibility, archive-content, workflow-policy, and
  signing-credential script checks.

The largest Go concentrations are `internal/cli` (195 test/benchmark
functions), `internal/server` (142), `internal/saga` (71),
`internal/requirements` (35), `internal/reviewapp` (26), `internal/quality`
(24), and `internal/nextaction` (23).

The browser suite uses the real binary, separate temporary source and Saga Git
repositories, an ephemeral loopback server, exact CLI-authored coverage, and
zero-side-effect snapshots for rejection paths. Its main coverage groups are:

- CLI validation, repository identity, mapping scrutiny, and comparison;
- documentation read-only behavior and canonical requirement targets;
- feature/directory navigation with and without JavaScript;
- linked code, Code Diff, coverage, pagination, cancellation, and repeated
  cursor defense;
- pull-request review decisions and out-of-date currency;
- malformed/foreign locations, cross-origin/foreign-host rejection,
  non-loopback refusal, and path redaction;
- keyboard/tab semantics, inert drawers, and axe scans;
- first-load, deferred-content, and on-demand diff budgets.

The Go suite passed locally during this audit. The browser suite was not rerun:
the workspace's `node` command is a DevSwarm Bun shim, and `npm ci` failed with
`npm error weird error BuildMessage {}` before dependencies were installed.
The checked-in CI definition remains the evidence for the intended
three-browser matrix, not evidence that it passed in this workspace.

## Coverage already recorded in app.saga

The authored Saga contains:

- 17 relation records: 11 active (`7 addresses`, `2 explains`, `2 verifies`)
  and 6 superseded `addresses` records;
- 10 exact code-evidence records with 17 references, covering only
  `internal/coderesolve/resolve.go`, `internal/server/server.go`,
  `internal/server/appnav.go`, and `internal/server/relatedreviews.go`;
- three design chapters (`coverage-report-design`, `code-reference-design`,
  and `two-sides-design`);
- one implementation deck, `code-evidence-implementation`, with one slide and
  seven Items;
- one quality test case, `resolve-remaps-and-stales`, with two quality-evidence
  records and one passing run record;
- one requirements citation and an onboarding deck.

The canonical status projection at the audit snapshot is:

| Area | Covered | Total | Interpretation |
| --- | ---: | ---: | --- |
| Implementation | 0 | 0 | Observe mode has no source change to account for; this is not whole-repository implementation coverage. |
| Stories | 10 | 10 | Every currently code-bearing target reaches a story. |
| Personas | 10 | 10 | Every currently code-bearing target reaches a persona. |
| Design | 4 | 18 | Four stories have an active design path; 14 do not. |
| Quality | 2 | 69 | Two criteria have an active canonical test-case verification; 67 do not. |
| Health | 80 | 81 | One exact code reference is stale. |

The stale reference is
`two-sides-sidebar.json#1`, pinned to lines 101–109 of
`internal/server/appnav.go` at `b851addd...`; those lines changed by the audited
head. This document deliberately does not repair it because the task is
research-only and the referenced design should be rechecked after requirements
are accepted.

## Best stable targets for later code references

These targets express policy or a public boundary and have focused tests. They
are preferable to incidental helpers or broad files.

| Capability | Recommended implementation target | Recommended executable evidence |
| --- | --- | --- |
| Feature layout and durable membership | `applayout.Features`, `InCreationOrder`, `Require`, and `FeatureOfPath` | [`internal/applayout/applayout_test.go`](../internal/applayout/applayout_test.go) |
| Strict app load and composed roots | `saga.ReadManifest`, `Load`, `LoadOutline`, `LoadNarrative`, and `loadAppContent` | Saga load, app, hardening, ordering, and schema-contract tests |
| Atomic/no-partial mutation | `store.WriteFile`, `CommitDir`, `WithSagaLock`, `EnsureDirWithin` | [`internal/store/store_test.go`](../internal/store/store_test.go) |
| Stable resource identity | `livingid.Build`/`Parse`, `qualityid.Build`/`Parse`, `sagaref.Build`/`Parse` | Their colocated URN/reference tests |
| Story/persona/flag/term lifecycle | Public add/revise/set-state functions in `internal/requirements` | App-record, mutation, terms, carry-forward, repin, and schema tests |
| Relation currency | `requirements.EvaluateRelation` and `EvaluateRelations` | [`internal/requirements/relation_v5_test.go`](../internal/requirements/relation_v5_test.go) and [`internal/requirements/repin_test.go`](../internal/requirements/repin_test.go) |
| Quality records and runs | `quality.Load`, `AddTestCase`, `AddEvidenceBatch`, `RecordRun` | Quality load/mutation/schema/features tests |
| Repository and location grammar | `coderef.CanonicalRepository`, `ParseLocation`, `Validate`, `DigestRange` | [`internal/coderef/coderef_test.go`](../internal/coderef/coderef_test.go) |
| Remap versus stale | `coderesolve.Resolver.Resolve` and `coderesolve.Remap` | `TestResolveRemapsMovedLinesAndReportsChangedLinesStale` and resolver negative tests |
| Source comparison | `gitdiff.ReadRange`, `ReadCatalogRange`, `ReadFile`, `VerifyRepository`, `Parse` | Git-diff parser, tree-change, rename/binary, and repository-identity tests |
| Exact mapping coverage | `coverage.Evaluate`, `EvaluateTargets`, `SelectTarget`, `Sides` | [`internal/coverage/coverage_test.go`](../internal/coverage/coverage_test.go) |
| Coverage axes | `coverage.ProjectAxes` | [`internal/coverage/axes_test.go`](../internal/coverage/axes_test.go) |
| Six-area status | `areas.Evaluate`, `livingapp.LoadStatus`, and `livingapp.Assemble` | Area tests, living-app status/traceability tests, CLI status text/grammar tests |
| Ordered agent actions | `nextaction.Derive` and `AuthoringLoop` | Next-action category, growth, review, term, and answer tests |
| Command contract | `grammar.Commands`, `Lookup`, and `Invoke` | Grammar parity tests and process exit/output tests |
| Query contract | `queryOperations`, `queryPurpose`, the `Query` dispatcher, and the two application-session adapters | CLI query envelope/integration tests and query fixture tests |
| Changed/Affected/Code model | `changeview.Open`/`Compute`, `Build`, `NodeHistory`, and `CursorHistory` | CLI layers/history tests plus review-app/server integration tests; add focused package tests if this becomes accepted quality evidence |
| Reviewer read boundary | `reviewapp.Open` and the `Session` interface methods | Session, mapping, selector-index, adversarial, and large-Saga tests |
| Pull-request review state | `reviewstate.ResolveRange`, `Build`, `LatestDecisions`, `Threads`, and `Evaluate` | CLI/server review tests; add focused package tests if canonicalized |
| Review writes | `reviewstore.Create`, `Decide`, `Comment`, and `Freeze` | [`internal/reviewstore/reviewstore_test.go`](../internal/reviewstore/reviewstore_test.go) |
| HTTP boundary | `server.newMux` for route presence; then the focused handler in `requirements.go`, `terms.go`, `reviews.go`, `incremental.go`, or `server.go` | Matching server test plus the narrowest Playwright scenario |
| Derived cache correctness | `snapshotcache.Store` build/read/prune operations | [`internal/snapshotcache/snapshotcache_test.go`](../internal/snapshotcache/snapshotcache_test.go) |

Avoid pinning a call site when a lower-level function owns the behavior. For
example, a remapping criterion should point to `coderesolve`, not to the CLI
that prints its result; an HTTP presentation criterion can additionally point
to the handler and browser test.

## Missing design and implementation nodes

The status projection reports these 14 stories with no active design path:

| Feature | Stories without design coverage |
| --- | --- |
| `agent-loop` | `first-change`, `growth-not-debt` |
| `code-evidence` | `changed-lines-accounted`, `companion-repositories` |
| `comparison` | `observe-or-compare`, `why-things-changed` |
| `format` | `one-app-saga`, `durable-features` |
| `requirements` | `overview-and-terms`, `traceability-chain` |
| `reviewer-app` | `open-observe-or-compare`, `sidebar` |
| `reviews` | `review-is-pr-deck`, `documentation-not-approval` |

Six of seven features have no implementation deck at all. Only
`code-evidence` has one. A later requirements/design pass should not create one
deck per package mechanically: each accepted feature needs a reviewer-oriented
system model, and its Items should then point to the narrow implementation
seams above.

Other visibly absent app-level or feature-level authored nodes include a
design-system root, feature-local work plans, canonical quality records for 67
criteria, and implementation Items for the server, CLI grammar/query API,
requirements model, comparison layers, status/next-action engine, and PR review
state. Absence is a coverage observation, not evidence that every node is
required.

## High-value quality cases after requirements acceptance

The following candidate records are grounded in existing criteria and tests.
They should be linked only after the requirement wording is accepted and a
reviewer confirms that the named test actually proves the criterion. Negative
paths are included because many core promises are about refusal without side
effects.

| Candidate quality case | Existing criteria it may verify | Executable evidence and important negative path |
| --- | --- | --- |
| App layout accepts one canonical feature tree | `one-app-saga`: `single-saga`, `app-roots`; `durable-features`: `move-without-breaking` | `applayout_test.go`, `app_authoring_test.go`; reject root-level feature content, duplicate IDs, mismatched feature ownership, and moves that would break relations. |
| Strict load is fail-closed and side-effect free | `one-app-saga`: `manifest-identity-only`; reviewer open behavior if retained | Saga hardening/schema tests and `@critical refuses to serve a structurally invalid saga...`; malformed/unknown JSON, symlinks, reserved paths, and unsupported versions must not write. |
| Overview terms are bidirectional and become stale on code rename | `overview-and-terms`: `term-record`, `both-directions`, `stale-on-rename` | Requirements term tests, living-app term status, server term tests, `terms.spec.ts`; unknown story/record, foreign ref, ambiguous remap, and renamed definition are negative cases. |
| Persona-to-story-to-design-to-code chain reports missing/stale states | `traceability-chain`: `adjacent-links`, `inferred-paths`, `link-states` | Living-app traceability/status tests and CLI relation currency tests; reject non-adjacent/illegal endpoints and ensure stale pins never count as coverage. |
| Persona retirement produces one focused question | `traceability-chain`: `new-persona-gap`, `removed-persona-question` | `TestStatusReportsPersonaGapsAndOneQuestionPerRetirement`; concurrent heads, already reassigned stories, and several affected stories are key negative/edge cases. |
| Status reports but does not decide | `coverage-report`: all four criteria; `growth-not-debt`: `growth-separate` | Area/status/check/next-action and JSON-envelope tests; gaps must exit zero for `status`, malformed/untrusted input must fail, ordering must be deterministic, and unrelated areas must not leak into `check`. |
| First-change path requires implementation only | `first-change`: `implementation-only`, `first-run`, `not-locked-in` | `TestFirstChangeCoversOnlyImplementation`, `TestFirstChangeNeedsNoPersonas`, default-feature tests; empty personas/stories/design/quality must not become a blocker. |
| Code-location grammar and repository identity are canonical | `companion-repositories`: `verified-checkout`; `code-references` where applicable | Coderef/gitdiff/CLI tests plus `cli.spec.ts` and `security.spec.ts`; reject abbreviated/non-canonical ranges, traversal, foreign repository origin, invalid commits, and contradictory flags without writes. |
| References remap only when content is unchanged | `code-references`: `remap-or-stale`, `saga-only-commits` | Existing `resolve-remaps-and-stales` quality case and resolver tests; changed content, duplicate digest matches, tampered digest, deletion, and missing pins must become stale or fail deterministically. |
| Whole-file, deletion, rename, and binary evidence uses the correct side | `code-references`: `deletions-and-whole-files` | CLI cover-reference/range tests and Git-diff tree-change tests; reject a line range on an unavailable side and ensure rename paths are not conflated. |
| Coverage is computed, exact, and non-stealing | `changed-lines-accounted`: all three criteria | Coverage and CLI changed-range tests; overlaps, gaps, adjacent atoms, file events, side filters, batch rollback, and replace failure should preserve exact ownership and originals. |
| Companion sync and compare use the code repository, not Saga history | `companion-repositories`: `sync-cursor`, `companion-compare`, `observe-existing-code`, `move-later` | Sync/opening/integration fixtures with separate repositories; missing cursor, mismatched origin, cursor without matching Saga commit, and Saga-only commits are negative paths. |
| Comparison has complete Changed/Affected/Code layers | `observe-or-compare`: all criteria | CLI layers tests, `changeview` through review-app/server, and critical CLI comparison E2E; require `--against`, use merge-base, preserve code-only affected records, and list unowned lines. A focused `internal/changeview` test suite would make this evidence less indirect. |
| History preserves pairing and reasons | `why-things-changed`: all criteria | Changeview pairing/reason/history flows exercised through CLI/server; missing/ambiguous replacement, squash merge, deleted branch, and records never committed are high-value negative paths. |
| Documentation and Review remain distinct | `two-sides`: all criteria; `documentation-not-approval`: both criteria | Server navigation/related-review tests, documentation E2E, and pull-request review E2E; documentation routes must have no mutation controls/endpoints, and empty derived related-review lists must still state derivation. |
| Reviewer opening is lazy, bounded, and preserves deep links | `open-observe-or-compare` and `sidebar` if the performance behavior is accepted | Server incremental/budget tests plus bounded/performance/navigation E2E; repeated cursors, cancellation, cold-building snapshots, huge limits, disabled JavaScript, and collapsed content are key negative paths. |
| Review decisions are per-slide and become out of date correctly | `review-is-pr-deck`: `per-slide-approval`, `out-of-date`, `tool-records` | Reviewstate via CLI/server and `pull-request-review.spec.ts`; wrong token, frozen review, changed slide only, changed referenced code, withdrawal, independent reviewer seats, and duplicate replay need explicit assertions. |
| Review coverage is exact and frozen after merge | `review-is-pr-deck`: `review-coverage`, `history-after-merge`, `one-per-pr` | Review coverage/server/review-store tests; uncovered/stale/file-event changes, duplicate PR review, moved head, and post-freeze mutation are negative paths. |
| Server trust boundary fails before handlers run | Existing reviewer-app behavior; canonical criterion still needed | Server security tests and `security.spec.ts`; foreign `Host`, cross-site fetch metadata, non-loopback listen, path escape/symlink, source path leaks, oversized pages, and invalid shutdown token. |
| Cache changes only speed, never answers | Existing reviewer-app performance behavior; canonical criterion still needed | Snapshot-cache tests; interrupted/failed builds publish nothing, changed inputs miss, damaged generations rebuild, pruning one Saga does not affect another, and cache files never appear inside the Saga. |

## Files and ranges not to pin

Do not use these as durable implementation references:

- `bin/`, `dist/`, `e2e/.cache/`, `node_modules/`, Playwright
  `test-results/`/`playwright-report/`, temporary repositories, and snapshot
  cache generations;
- `e2e/package-lock.json` or other lockfiles for product behavior;
- generated large-Saga fixtures or benchmark output; pin the fixture generator
  and invariant test instead;
- hashed flat deck filenames such as `20-s-...`, `30-i-...`, and `40-e-...` as
  path identity; use the semantic deck/slide/Item URN and let the CLI resolve
  its current flat record;
- append-only `revisions/`, `events/`, `runs/`, review approvals/comments, or
  `___merges` files as implementation; they are product data and history;
- schema examples as the sole contract target; pair the schema with runtime
  parity tests;
- `internal/server/appjs.go`, `styles.go`, or large inline template constants
  as whole-file evidence. Pin a focused function/selector plus a browser test;
- source line numbers copied from this document. The persisted reference must
  be created at the accepted commit and include its digest;
- any file inside `app.saga` as evidence for source implementation. The Saga is
  documentation, and Saga-only changes are intentionally excluded from source
  comparison semantics.

Generated filenames are not universally disposable: the flat bundle and
coverage names are deterministic storage identities. The rule is narrower:
do not hand-pin their path when a semantic URN or generating contract is the
stable reference.

## Audit verification

Commands used for the final audit:

```text
go run ./cmd/change-saga validate --json app.saga
go run ./cmd/change-saga status --json app.saga
go test ./...
```

Results: validation passed; status produced the coverage facts recorded above;
all Go packages passed. No source or `app.saga` file was changed by this audit.
