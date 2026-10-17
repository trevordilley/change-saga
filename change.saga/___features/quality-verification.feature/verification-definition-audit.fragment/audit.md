# Verification definitions and gaps {#verification-definitions-and-gaps}

This is an assessment of verification definitions, authored on 2026-10-16 against the repository source at `cc75d453cd7cd65aa1f675533c07fb34404b1e6c`. It records what the inspected code asserts and what still needs verification. It contains no test execution results or delivery verdicts. All cases remain proposed for review.

The assessment covers **63 current stories and 211 acceptance criteria**, expressed as **178 canonical cases** with **306 implementation evidence records**. Each criterion has a case-level revision-pinned verifies relation and an explicit step mapping. Cases reuse canonical requirement and evidence identities. Requirements-only cases are valid definitions and can gain code links later without changing identity.

## How to read this assessment {#how-to-read-this-assessment}

| Assessment | Meaning | Criteria |
| --- | --- | ---: |
| Evidence linked | Inspected assertions support the scoped criterion checks; this is not an execution result or a claim of exhaustive test coverage. | 63 |
| Partial evidence | Specific steps have inspected implementation links; remaining assertions or broader combinations are planned. | 107 |
| Planned | The verification approach is defined, but the inspected sources do not establish the required automated assertions. Some checks require a missing product capability. | 36 |
| Ambiguous | Recorded intent and current behavior, or the expected outcome itself, require clarification. The case preserves the recorded obligation. | 5 |

Automation describes the approach: 78 automated, 60 hybrid and 40 manual cases. A case-level automation label does not make every step automated. Test evidence is attached to the particular steps it implements; setup and planned assertions may have no code. Optional product-code links identify implementation under test and are not substitutes for test assertions.

Start at a story, open its acceptance criterion, follow its linked test case, then read the step and its evidence. The case reader supplies exact committed source selectors. The evidence notes below explain limitations that a code link alone cannot convey.

## Findings to resolve next {#findings-to-resolve-next}

Existing kind policies were preserved. Twenty-two criteria currently have only negative or edge cases; the default positive-kind policy can therefore still report a missing kind. Review the intended policy or add a meaningful positive scenario where the criterion calls for one. A mapped definition does not by itself satisfy every policy.

- **Public work authoring:** contract lifecycle transitions lack a public writer, and typed technical-entity assignments do not yet connect work-item scope, queries and reconciliation. Definitions preserve those desired capabilities as planned checks.
- **Product intent:** first-slide navigation may conflict with the newer deck overview entry point; feature-scoped coverage currently includes unowned lines despite the recorded only-named-feature wording; delegated AI principal attribution is Git-derived. Clarify these contracts before treating current behavior as acceptance.
- **Historical context:** complete public history field fidelity, omitted-parent rejection, design-to-requirement context, and visible stale/conflicted reverse paths need stronger assertions. A successful path or a filtered-out stale path does not establish those obligations.
- **Browser behavior:** freehand gestures, cross-participant annotation authorship, source visual byte preservation, presentation mode, reopen/ordered discussion history, and broader media/sandbox behavior have planned or partial verification.
- **Technical inventory:** relationship-level comparisons, full proposed-plan-to-review handoff, approval/integration isolation, all-kind status matrices and architectural explanation quality need broader verification. Existing legacy ERD tests do not establish every named-whiteboard behavior.
- **Quality itself:** definition-first planning, same-case enrichment and exact mappings have implementation evidence. Full outcome-independent health/readiness matrices and complete public provenance field fidelity still need explicit assertions; historical result fields remain inert compatibility data.

These findings distinguish missing assertions from demonstrated product limitations and unclear intent. The inspected test sample is not proof that no other test exists. No story was rewritten merely to match code.

## Scope and compatibility {#scope-and-compatibility}

Three existing Quality stories were revised in place to express the agreed definition-only model: assess defined verification coverage, understand verification boundaries, and define verification/link its implementation. Their prior revisions, identities and lifecycle history remain intact. Three criteria were added for planned definitions, step traceability and immutable enrichment. No case was accepted, activated or presented as executed.

The retired `criterion-evidence-confidence` story (three criteria) remains historical. The two deferred hosting stories, `host-saga-outside-target-repo` and `configure-self-hosted-saga-repositories` (five criteria), were not changed or given cases in this pass. Other proposals retain their existing lifecycle and feature planning placement.

Existing baseline documentation warnings remain separate from this assessment. Older Quality diagrams describe the former run-gated approach and need semantic reassessment; their old relation pins are not silently advanced to the revised stories. Historical runs and old definitions remain readable. No command stored in run metadata was executed.

## Criterion assessment {#criterion-assessment}

### ai-interface {#feature-ai-interface}

#### [Page through a large Saga query safely](/requirements/bounded-query-pagination) {#story-bounded-query-pagination}

As a developer using an AI assistant, I can retrieve a large machine-readable Saga result in bounded pages from one snapshot so my tool can complete the read without unbounded output, duplicates, or mixed-state data.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [page-metadata](/requirements/bounded-query-pagination/criteria/page-metadata) | evidence-linked | [Traverse two bounded child pages without duplicates](/tests/contracts-cursor-pages). Exact traversal assertion is for two children. CLI all-operation table additionally checks metadata coherence (query_integration_test.go:98-111); this does not exhaust every operation/collection size. null next_cursor is treated as absent continuation, matching JSON contract. |
| [complete-traversal](/requirements/bounded-query-pagination/criteria/complete-traversal) | evidence-linked | [Traverse two bounded child pages without duplicates](/tests/contracts-cursor-pages). Exact traversal assertion is for two children. CLI all-operation table additionally checks metadata coherence (query_integration_test.go:98-111); this does not exhaust every operation/collection size. null next_cursor is treated as absent continuation, matching JSON contract. |
| [bounded-limit](/requirements/bounded-query-pagination/criteria/bounded-limit) | partial-evidence | [Check cursor-page defaults and limit boundaries](/tests/contracts-page-limits). Missing actual 100-row default and 1000-row maximum acceptance assertions in inspected tests. Criterion says "a query" but fragment/slide limits are bytes and some operations are unpaginated; definition explicitly exercises cursor item pagination rather than inventing one bound for all query kinds. |
| [cursor-context](/requirements/bounded-query-pagination/criteria/cursor-context) | partial-evidence | [Reject malformed and cross-context continuation tokens](/tests/contracts-cursor-context-rejection). Tests assert codes but do not assert Retryable=false for these actual cursor errors or normalized-equivalent selectors. Generic CLI error serialization tests do preserve supplied false retryability (query_test.go:199-231). |
| [snapshot-retry](/requirements/bounded-query-pagination/criteria/snapshot-retry) | evidence-linked | [Reject a cursor after the Saga snapshot changes](/tests/contracts-cursor-stale-snapshot).  |

#### [Give AI a stable application contract](/requirements/structured-ai-contract) {#story-structured-ai-contract}

As a developer using an AI assistant, I can give it stable machine-readable operations for querying and authoring a Saga so it can work without scraping storage files or prose output.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [envelope](/requirements/structured-ai-contract/criteria/envelope) | evidence-linked | [Emit one versioned JSON envelope for query success and failure](/tests/contracts-query-versioned-envelope). Enumerated operations are representative; new query families should extend table coverage. No tests were executed during this authoring task. |
| [discovery](/requirements/structured-ai-contract/criteria/discovery) | partial-evidence | [Discover operation usage, pagination and stable errors](/tests/contracts-query-operation-discovery). Need an all-operation discovery assertion for purpose, complete input shapes and published stable errors. Error behavior tests demonstrate serialization but not discovery completeness. Do not interpret usage strings alone as a fully typed input schema. |

### comparison-history {#feature-comparison-history}

#### [Separate authored changes from downstream impact](/requirements/change-impact-layers) {#story-change-impact-layers}

As an architect assessing a change, I can distinguish Saga records the change edited from unedited records it may invalidate so I can review both direct and propagated consequences.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [changed-records](/requirements/change-impact-layers/criteria/changed-records) | evidence-linked | [Classify records introduced after the comparison base](/tests/review-added-records), [Separate a revised story from the relation pinned to its previous revision](/tests/review-revised-record-and-pin), [Retire a replaced slide while retaining its replacement context](/tests/review-retired-replacement-history). Separate actual fixtures assert added, revised with before/after, and retired removed-slide records. Lifecycle retirement without removal is not separately asserted here. |
| [affected-records](/requirements/change-impact-layers/criteria/affected-records) | evidence-linked | [Separate a revised story from the relation pinned to its previous revision](/tests/review-revised-record-and-pin), [Propagate code impact without claiming Saga records were edited](/tests/review-code-chain-impact). Code-only fixture distinguishes Changed from Affected and asserts code/chain causes; revision fixture asserts changed-pin cause. |

#### [Choose observation or comparison context](/requirements/comparison-context) {#story-comparison-context}

As a developer inspecting a Saga, I can open one committed revision or compare a committed change so evidence and change layers are evaluated against an explicit, reproducible range.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [observe-zero-diff](/requirements/comparison-context/criteria/observe-zero-diff) | partial-evidence | [Observe the selected committed head without comparison layers](/tests/review-observe). Default-head observe behavior has direct assertions; explicitly selected historical head remains planned. |
| [merge-base-range](/requirements/comparison-context/criteria/merge-base-range) | partial-evidence | [Resolve a diverged comparison to its actual merge base](/tests/review-merge-base). Actual diverged range is asserted at gitdiff layer; mode is asserted in other comparison fixtures, not the same opening fixture. |
| [committed-head-only](/requirements/comparison-context/criteria/committed-head-only) | partial-evidence | [Reject WORKTREE and explain how to compare committed work](/tests/review-worktree-refusal). Rejection asserted; guidance exists in implementation but its wording is not asserted. |

#### [Reopen the change that shaped a record](/requirements/record-change-history) {#story-record-change-history}

As an architect investigating why a Saga record exists, I can inspect its change history and reopen the comparison for each event so I can understand what introduced or replaced it.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [commit-context](/requirements/record-change-history/criteria/commit-context) | partial-evidence | [Read the introducing replacement event and reopen its comparison](/tests/review-history-replacement), [Distinguish committed event history from uncommitted record changes](/tests/review-history-multiple-and-dirty). Introducing reason is asserted; full multi-event reasons and clean-to-dirty flag transition lack inspected assertions. NodeHistory derives Uncommitted from Git status. |
| [reopen-comparison](/requirements/record-change-history/criteria/reopen-comparison) | partial-evidence | [Read the introducing replacement event and reopen its comparison](/tests/review-history-replacement), [Distinguish committed event history from uncommitted record changes](/tests/review-history-multiple-and-dirty). Introducing event command is asserted; every-event and companion cursor boundary coverage remains planned. |
| [replacement-context](/requirements/record-change-history/criteria/replacement-context) | evidence-linked | [Read the introducing replacement event and reopen its comparison](/tests/review-history-replacement). Query history assertion checks the exact replaced SQS slide identity. |

### documentation-coverage {#feature-documentation-coverage}

#### [Know whether changed code is explained](/requirements/changed-code-explained) {#story-changed-code-explained}

As a developer preparing a change for review, I can see every changed line that lacks an explanation so I know whether the review documentation accounts for the change.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [separate-states](/requirements/changed-code-explained/criteria/separate-states) | evidence-linked | [Distinguish covered lines, gaps, overlaps and stale references](/tests/contracts-coverage-states).  |
| [no-verdict](/requirements/changed-code-explained/criteria/no-verdict) | planned | [Keep factual coverage separate from review approval](/tests/contracts-coverage-no-verdict). Report struct in internal/coverage/coverage.go:71-85 contains factual coverage fields. Inspected no-score tests prohibit numeric aggregates, not approval wording or review-state mutation; a dedicated no-verdict assertion is missing. |

#### [See missing documentation by area](/requirements/documentation-gaps-visible) {#story-documentation-gaps-visible}

As a developer preparing a change for review, I can see each documentation coverage area and its exact gaps so I can complete the relevant documentation without reducing everything to one score.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [separate-areas](/requirements/documentation-gaps-visible/criteria/separate-areas) | partial-evidence | [Report documentation areas independently with covered and gap entries](/tests/contracts-documentation-named-areas). Evaluator JSON is covered. CLI status_grammar_test.go:140-159 checks coverage/axes exist but not every nested area/entry; retain an explicit public status assertion as a gap. |
| [criterion-axis-reasons](/requirements/documentation-gaps-visible/criteria/criterion-axis-reasons) | evidence-linked | [Resolve each required axis to linked, excluded or a reasoned gap](/tests/contracts-criterion-axis-reasons).  |

#### [Reconcile living documentation after a change](/requirements/reconcile-living-documentation) {#story-reconcile-living-documentation}

As a developer, I can identify and repair documentation drift after implementing and reviewing a change so the Saga remains an accurate explanation of the current application.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [separate-currency](/requirements/reconcile-living-documentation/criteria/separate-currency) | evidence-linked | [Distinguish living currency regression from base-side coverage](/tests/contracts-reconcile-currency-regression), [Keep existing documentation debt separate from a new change](/tests/contracts-reconcile-baseline-debt), [Keep complete review coverage separate from living documentation](/tests/contracts-reconcile-review-isolation), [Resolve living currency at the explicitly selected head](/tests/contracts-reconcile-selected-head).  |
| [explain-impact](/requirements/reconcile-living-documentation/criteria/explain-impact) | partial-evidence | [Find declared-link reassessment after code and requirement-only changes](/tests/contracts-reconcile-impacted-records). The inspected tests assert affected identities and route kinds, but not each reason/path and unrelated-record exclusion. Keep those checks unlinked to test evidence. |
| [repair-loop](/requirements/reconcile-living-documentation/criteria/repair-loop) | evidence-linked | [Recheck after pin repair without clearing semantic reassessment](/tests/contracts-reconcile-pin-recheck), [Repair a transaction-managed slide from its complete snapshot](/tests/contracts-reconcile-slide-snapshot).  |

#### [Check selected documentation coverage](/requirements/scoped-coverage-check) {#story-scoped-coverage-check}

As a developer configuring change automation, I can check only the documentation coverage areas and scope my team requires so unrelated gaps do not decide the result.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [selected-areas](/requirements/scoped-coverage-check/criteria/selected-areas) | partial-evidence | [Return only requested coverage areas in caller order](/tests/contracts-selected-coverage-areas). Direct CLI assertions cover implementation/stories order; no table over all six supported names located. |
| [selected-scope](/requirements/scoped-coverage-check/criteria/selected-scope) | ambiguous | [Compare app, Git and feature scopes explicitly](/tests/contracts-selected-coverage-scope). Criterion says "only the named feature", but implemented and tested behavior includes every unowned changed line in a feature scope. Clarify whether conservative unowned inclusion is intended. Exact no-flag app and --feature CLI scope assertions are also missing from inspected tests. |
| [result-codes](/requirements/scoped-coverage-check/criteria/result-codes) | evidence-linked | [Distinguish complete, gap and untrustworthy check results](/tests/contracts-coverage-result-codes).  |

### feature-organization {#feature-feature-organization}

#### [Assign shared milestones](/requirements/assign-shared-feature-milestones) {#story-assign-shared-feature-milestones}

As a Product Manager, I can assign features to a shared named milestone, so the team can see a common delivery target without duplicating its definition.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [optional-shared-target](/requirements/assign-shared-feature-milestones/criteria/optional-shared-target) | partial-evidence | [Keep milestone identity through sharing, renaming and clearing](/tests/core-shared-milestones). Exact test assertions map to: core-shared-milestones/check-optional-shared-target. Unmapped manual/planned assertions: core-shared-milestones/check-unassigned-target. |
| [rename-once](/requirements/assign-shared-feature-milestones/criteria/rename-once) | evidence-linked | [Keep milestone identity through sharing, renaming and clearing](/tests/core-shared-milestones). Exact test assertions map to: core-shared-milestones/check-rename-once. No additional unmapped assertion is claimed by this definition. |
| [valid-references](/requirements/assign-shared-feature-milestones/criteria/valid-references) | partial-evidence | [Reject invalid milestone assignments](/tests/core-milestone-invalid-references). Exact test assertions map to: core-milestone-invalid-references/check-valid-references. Unmapped manual/planned assertions: core-milestone-invalid-references/check-foreign-milestone. |
| [preserve-or-clear](/requirements/assign-shared-feature-milestones/criteria/preserve-or-clear) | evidence-linked | [Keep milestone identity through sharing, renaming and clearing](/tests/core-shared-milestones). Exact test assertions map to: core-shared-milestones/check-preserve-or-clear. No additional unmapped assertion is claimed by this definition. |

#### [Group Features and Roadmap independently](/requirements/group-feature-views-independently) {#story-group-feature-views-independently}

As a Product Manager, I can choose a separate grouping for Features and Roadmap, so I can review delivered capabilities and upcoming priorities from different perspectives.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [group-options](/requirements/group-feature-views-independently/criteria/group-options) | planned | [Keep Features and Roadmap grouping independent](/tests/core-feature-grouping). Unmapped manual/planned assertions: core-feature-grouping/check-group-options. |
| [independent-preferences](/requirements/group-feature-views-independently/criteria/independent-preferences) | partial-evidence | [Keep Features and Roadmap grouping independent](/tests/core-feature-grouping). Exact test assertions map to: core-feature-grouping/check-independent-preferences. Unmapped manual/planned assertions: core-feature-grouping/check-story-preferences. |
| [membership-without-copying](/requirements/group-feature-views-independently/criteria/membership-without-copying) | partial-evidence | [Keep Features and Roadmap grouping independent](/tests/core-feature-grouping). Exact test assertions map to: core-feature-grouping/check-membership-without-copying. Unmapped manual/planned assertions: core-feature-grouping/check-canonical-group-links. |
| [unassigned-visible](/requirements/group-feature-views-independently/criteria/unassigned-visible) | partial-evidence | [Keep Features and Roadmap grouping independent](/tests/core-feature-grouping). Exact test assertions map to: core-feature-grouping/check-unassigned-visible. Unmapped manual/planned assertions: core-feature-grouping/check-unassigned-membership. |
| [find-shared-names](/requirements/group-feature-views-independently/criteria/find-shared-names) | partial-evidence | [Keep Features and Roadmap grouping independent](/tests/core-feature-grouping). Exact test assertions map to: core-feature-grouping/check-find-shared-names. Unmapped manual/planned assertions: core-feature-grouping/check-both-directory-searches. |

#### [Plan and deliver the same feature](/requirements/plan-and-deliver-one-feature) {#story-plan-and-deliver-one-feature}

As a Product Manager, I can describe a planned feature and later mark it built without recreating its knowledge, so the team retains the original intent through delivery.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [separate-views](/requirements/plan-and-deliver-one-feature/criteria/separate-views) | evidence-linked | [Promote a planned feature without recreating its knowledge](/tests/core-feature-lifecycle). Exact test assertions map to: core-feature-lifecycle/check-separate-views. No additional unmapped assertion is claimed by this definition. |
| [stable-identity](/requirements/plan-and-deliver-one-feature/criteria/stable-identity) | partial-evidence | [Promote a planned feature without recreating its knowledge](/tests/core-feature-lifecycle). Exact test assertions map to: core-feature-lifecycle/check-stable-identity. Unmapped manual/planned assertions: core-feature-lifecycle/check-additional-preservation. |
| [planning-context](/requirements/plan-and-deliver-one-feature/criteria/planning-context) | partial-evidence | [Promote a planned feature without recreating its knowledge](/tests/core-feature-lifecycle). Exact test assertions map to: core-feature-lifecycle/check-planning-context. Unmapped manual/planned assertions: core-feature-lifecycle/check-planned-design-context, core-feature-lifecycle/check-built-design-context. |
| [priority-order](/requirements/plan-and-deliver-one-feature/criteria/priority-order) | partial-evidence | [Order ranked and unranked planned features](/tests/core-feature-priority). Exact test assertions map to: core-feature-priority/check-priority-order. Unmapped manual/planned assertions: core-feature-priority/check-equal-priority-order. |

#### [Reuse shared feature labels](/requirements/reuse-shared-feature-labels) {#story-reuse-shared-feature-labels}

As a Product Manager, I can define a label once and reference it from several features, so categories stay consistent as the product grows.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [shared-definition](/requirements/reuse-shared-feature-labels/criteria/shared-definition) | evidence-linked | [Share labels without copying names or losing memberships](/tests/core-shared-labels). Exact test assertions map to: core-shared-labels/check-shared-definition. No additional unmapped assertion is claimed by this definition. |
| [rename-once](/requirements/reuse-shared-feature-labels/criteria/rename-once) | evidence-linked | [Share labels without copying names or losing memberships](/tests/core-shared-labels). Exact test assertions map to: core-shared-labels/check-rename-once. No additional unmapped assertion is claimed by this definition. |
| [duplicate-name](/requirements/reuse-shared-feature-labels/criteria/duplicate-name) | partial-evidence | [Reject duplicate labels and invalid memberships](/tests/core-label-invalid-writes). Exact test assertions map to: core-label-invalid-writes/check-duplicate-name. Unmapped manual/planned assertions: core-label-invalid-writes/check-duplicate-diagnostic. |
| [valid-references](/requirements/reuse-shared-feature-labels/criteria/valid-references) | evidence-linked | [Reject duplicate labels and invalid memberships](/tests/core-label-invalid-writes). Exact test assertions map to: core-label-invalid-writes/check-valid-references. No additional unmapped assertion is claimed by this definition. |
| [preserve-or-clear](/requirements/reuse-shared-feature-labels/criteria/preserve-or-clear) | partial-evidence | [Share labels without copying names or losing memberships](/tests/core-shared-labels), [Replace a nonempty label set](/tests/core-label-replacement). Exact test assertions map to: core-shared-labels/check-preserve-or-clear. Unmapped manual/planned assertions: core-label-replacement/check-nonempty-replacement. |

### merge-friendly-format {#feature-merge-friendly-format}

#### [Inspect requirement evolution](/requirements/auditable-requirement-history) {#story-auditable-requirement-history}

As a product manager, I can inspect the complete evolution of a story's definition and lifecycle so I can understand decisions without losing earlier intent.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [revision-history](/requirements/auditable-requirement-history/criteria/revision-history) | planned | [Read complete story revisions and lifecycle ancestry](/tests/contracts-history-complete-snapshots). historyRows in internal/livingapp/compose.go:43-75 returns full revisions/events. Inspected CLI criterion-overview test asserts a prose substring in history and immutable r2 only; no exact full prior/new definition, lifecycle reason, and parent query assertions located. Do not equate this with a test execution failure. |
| [lifecycle-history](/requirements/auditable-requirement-history/criteria/lifecycle-history) | planned | [Read complete story revisions and lifecycle ancestry](/tests/contracts-history-complete-snapshots). historyRows in internal/livingapp/compose.go:43-75 returns full revisions/events. Inspected CLI criterion-overview test asserts a prose substring in history and immutable r2 only; no exact full prior/new definition, lifecycle reason, and parent query assertions located. Do not equate this with a test execution failure. |

#### [Merge independently authored knowledge](/requirements/parallel-authoring) {#story-parallel-authoring}

As a developer collaborating in parallel, I can merge independently authored Saga records without editing one monolithic document so concurrent work remains reviewable and recoverable.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [partitioned](/requirements/parallel-authoring/criteria/partitioned) | partial-evidence | [Preserve independent revision and lifecycle records after a merge](/tests/contracts-parallel-partitioned-conflicts). Conflict projections and separate branch fixture records are asserted; no exhaustive byte-equality assertion for both definition/lifecycle branches was located. Independent source edits were not performed during this evaluation. |
| [conflicts](/requirements/parallel-authoring/criteria/conflicts) | partial-evidence | [Preserve independent revision and lifecycle records after a merge](/tests/contracts-parallel-partitioned-conflicts). Conflict projections and separate branch fixture records are asserted; no exhaustive byte-equality assertion for both definition/lifecycle branches was located. Independent source edits were not performed during this evaluation. |
| [atomic](/requirements/parallel-authoring/criteria/atomic) | evidence-linked | [Leave the previous record intact after interrupted file publication](/tests/contracts-atomic-record-write), [Publish a complete new entity directory or none](/tests/contracts-atomic-entity-directory). Single-record publication guarantee only; do not generalize to a global multi-file crash transaction. |

#### [Reconcile competing story heads](/requirements/recover-competing-heads) {#story-recover-competing-heads}

As a developer resolving concurrent story edits, I can reconcile every competing head into one complete revision so the story becomes current without erasing either branch's history.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [all-heads-required](/requirements/recover-competing-heads/criteria/all-heads-required) | planned | [Reject incomplete parents before reconciling competing story definitions](/tests/contracts-reconcile-all-story-heads). The story DAG test does not test omission despite its title. Missing assertions: omitted parent rejection leaves no revision, expected-head error payload, and former-head history query retention. |
| [current-restored](/requirements/recover-competing-heads/criteria/current-restored) | partial-evidence | [Reject incomplete parents before reconciling competing story definitions](/tests/contracts-reconcile-all-story-heads). The story DAG test does not test omission despite its title. Missing assertions: omitted parent rejection leaves no revision, expected-head error payload, and former-head history query retention. |

#### [Author safely alongside concurrent writers](/requirements/safe-concurrent-authoring) {#story-safe-concurrent-authoring}

As a developer collaborating in parallel, I can submit a Saga mutation without overwriting another writer's newer work so concurrent edits remain recoverable.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [serialized-writes](/requirements/safe-concurrent-authoring/criteria/serialized-writes) | partial-evidence | [Serialize supported writers and return busy at timeout](/tests/contracts-serialized-lock-writers). Tests exercise lock callbacks, not two full persisted public mutations; no exact busy-code/message assertion on the public surface. Callback serialization is evidence for the mechanism, not complete persisted-mutation proof. |
| [stale-parent-rejected](/requirements/safe-concurrent-authoring/criteria/stale-parent-rejected) | partial-evidence | [Reject a stale story parent and identify the expected heads](/tests/contracts-stale-story-parent). Missing direct full-story stale-parent test with expected heads and absent revision assertions; wrapper-only assertion must not stand in for all these obligations. |

### product-knowledge {#feature-product-knowledge}

#### [Keep product knowledge areas isolated](/requirements/independent-product-knowledge) {#story-independent-product-knowledge}

As an architect maintaining product knowledge, I can change requirements, prototypes, or quality evidence independently and then inspect one validated view so a change in one area does not silently rewrite or hide another.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [independent-histories](/requirements/independent-product-knowledge/criteria/independent-histories) | planned | [Keep requirement, prototype and quality histories independent](/tests/core-domain-isolation). Unmapped manual/planned assertions: core-domain-isolation/check-independent-histories. |
| [validated-composition](/requirements/independent-product-knowledge/criteria/validated-composition) | planned | [Refuse composition with an invalid domain](/tests/core-invalid-domain-composition). Unmapped manual/planned assertions: core-invalid-domain-composition/check-validated-composition. |

#### [Mature product intent safely](/requirements/product-intent-lifecycle) {#story-product-intent-lifecycle}

As a product manager, I can keep a story proposed while its success conditions are still being shaped and accept it only after observable criteria exist so draft intent is not mistaken for a commitment.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [draft-without-criteria](/requirements/product-intent-lifecycle/criteria/draft-without-criteria) | evidence-linked | [Keep unfinished intent proposed until a criterion exists](/tests/core-story-acceptance). Exact test assertions map to: core-story-acceptance/check-draft-without-criteria. No additional unmapped assertion is claimed by this definition. |
| [acceptance-requires-criterion](/requirements/product-intent-lifecycle/criteria/acceptance-requires-criterion) | evidence-linked | [Refuse acceptance without criteria](/tests/core-story-acceptance-rejection). Exact test assertions map to: core-story-acceptance-rejection/check-acceptance-requires-criterion. No additional unmapped assertion is claimed by this definition. |

#### [Review a prototype against product intent](/requirements/prototype-feedback-context) {#story-prototype-feedback-context}

As a designer, I can connect feedback on a specific prototype element to the exact story revision it explores so later changes stay visible instead of silently moving the feedback.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [immutable-revision](/requirements/prototype-feedback-context/criteria/immutable-revision) | evidence-linked | [Retain immutable prototype source and detect package tampering](/tests/core-prototype-package). Exact test assertions map to: core-prototype-package/check-immutable-revision. No additional unmapped assertion is claimed by this definition. |
| [pinned-annotation](/requirements/prototype-feedback-context/criteria/pinned-annotation) | partial-evidence | [Resolve an annotation authored before its endpoints](/tests/core-prototype-unresolved-pins). Exact test assertions map to: core-prototype-unresolved-pins/check-pinned-annotation. Unmapped manual/planned assertions: core-prototype-unresolved-pins/check-annotation-pin-fields. |
| [visible-currency](/requirements/prototype-feedback-context/criteria/visible-currency) | partial-evidence | [Resolve an annotation authored before its endpoints](/tests/core-prototype-unresolved-pins), [Expose both stale annotation pins](/tests/core-prototype-stale-pins). Exact test assertions map to: core-prototype-unresolved-pins/check-pinned-annotation, core-prototype-stale-pins/check-visible-currency. Unmapped manual/planned assertions: core-prototype-unresolved-pins/check-annotation-pin-fields, core-prototype-stale-pins/check-retained-stale-pins. |
| [trusted-external-origin](/requirements/prototype-feedback-context/criteria/trusted-external-origin) | evidence-linked | [Require an exact allowlisted external embed origin](/tests/core-prototype-external-origins). Exact test assertions map to: core-prototype-external-origins/check-trusted-external-origin. No additional unmapped assertion is claimed by this definition. |

#### [Read the obligation and its explanation](/requirements/read-authored-criterion-context) {#story-read-authored-criterion-context}

As a product manager, I can read a concise criterion title, its full pass/fail statement and authored context so I can judge the intended outcome without reconstructing it from metadata.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [full-statement](/requirements/read-authored-criterion-context/criteria/full-statement) | evidence-linked | [Read complete criterion intent with safe authored context](/tests/core-criterion-reading). Exact test assertions map to: core-criterion-reading/check-full-statement. No additional unmapped assertion is claimed by this definition. |
| [safe-prose](/requirements/read-authored-criterion-context/criteria/safe-prose) | evidence-linked | [Read complete criterion intent with safe authored context](/tests/core-criterion-reading). Exact test assertions map to: core-criterion-reading/check-safe-prose. No additional unmapped assertion is claimed by this definition. |
| [preservation](/requirements/read-authored-criterion-context/criteria/preservation) | evidence-linked | [Preserve omitted criterion prose](/tests/core-criterion-prose-preservation). Exact test assertions map to: core-criterion-prose-preservation/check-preservation. No additional unmapped assertion is claimed by this definition. |
| [reassess](/requirements/read-authored-criterion-context/criteria/reassess) | evidence-linked | [Reassess changed criterion prose](/tests/core-criterion-prose-currency). Exact test assertions map to: core-criterion-prose-currency/check-reassess. No additional unmapped assertion is claimed by this definition. |

#### [Scope requirements by product area](/requirements/requirements-feature-scope) {#story-requirements-feature-scope}

As a product manager reviewing product knowledge, I can see the feature that owns each story in requirement query results so I can separate one product area from another.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [feature-in-query](/requirements/requirements-feature-scope/criteria/feature-in-query) | evidence-linked | [Identify the owner of feature-filtered requirements](/tests/core-requirement-ownership). Exact test assertions map to: core-requirement-ownership/check-feature-in-query. No additional unmapped assertion is claimed by this definition. |

#### [Keep product language anchored](/requirements/shared-product-language) {#story-shared-product-language}

As a developer joining or changing the product, I can look up a project term with its definition, related intent, and defining code so I use the team language and notice when it drifts.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [term-context](/requirements/shared-product-language/criteria/term-context) | evidence-linked | [Keep terms tied to definitions, intent and source](/tests/core-term-context). Exact test assertions map to: core-term-context/check-term-context. No additional unmapped assertion is claimed by this definition. |
| [code-currency](/requirements/shared-product-language/criteria/code-currency) | partial-evidence | [Keep terms tied to definitions, intent and source](/tests/core-term-context), [Identify the exact term revision affected by source changes](/tests/core-term-revision-health). Exact test assertions map to: core-term-context/check-code-currency. Unmapped manual/planned assertions: core-term-revision-health/check-affected-term-revision. |
| [optional-suggestions](/requirements/shared-product-language/criteria/optional-suggestions) | evidence-linked | [Keep terminology growth optional](/tests/core-term-optional-growth). Exact test assertions map to: core-term-optional-growth/check-optional-suggestions. No additional unmapped assertion is claimed by this definition. |

### quality-verification {#feature-quality-verification}

#### [Set criterion verification policy](/requirements/criterion-quality-policy) {#story-criterion-quality-policy}

As a product manager, I can specify the coverage kinds and automation levels required for an acceptance criterion so readiness reflects the assurance appropriate to that obligation.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [default-policy](/requirements/criterion-quality-policy/criteria/default-policy) | partial-evidence | [Apply criterion kinds and automation policy without executions](/tests/core-quality-policies). Exact test assertions map to: core-quality-policies/check-default-policy. Unmapped manual/planned assertions: core-quality-policies/check-default-automation-matrix. |
| [required-kinds](/requirements/criterion-quality-policy/criteria/required-kinds) | partial-evidence | [Apply criterion kinds and automation policy without executions](/tests/core-quality-policies). Exact test assertions map to: core-quality-policies/check-required-kinds. Unmapped manual/planned assertions: core-quality-policies/check-required-kind-subsets. |
| [allowed-automation](/requirements/criterion-quality-policy/criteria/allowed-automation) | planned | [Exclude test definitions outside allowed automation](/tests/core-policy-automation-restriction). Unmapped manual/planned assertions: core-policy-automation-restriction/check-allowed-automation. |
| [policy-currency](/requirements/criterion-quality-policy/criteria/policy-currency) | planned | [Keep stale policy obligations until deliberate supersession](/tests/core-policy-currency). Unmapped manual/planned assertions: core-policy-currency/check-policy-currency. |

#### [Assess defined verification coverage](/requirements/criterion-quality-readiness) {#story-criterion-quality-readiness}

As a product manager, I can see how each accepted criterion will be verified and which mappings need attention so I can inspect the verification approach without mistaking it for an execution result.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [required-proof](/requirements/criterion-quality-readiness/criteria/required-proof) | partial-evidence | [Assess verification definitions without requiring execution](/tests/core-definition-coverage), [Require criterion-specific assertion mappings](/tests/core-definition-step-mapping), [Keep unsupported definition states explicit](/tests/core-definition-remaining-states). Exact test assertions map to: core-definition-coverage/check-required-kinds, core-definition-coverage/check-planned-without-runs, core-definition-step-mapping/check-specific-mapping. Unmapped manual/planned assertions: core-definition-remaining-states/check-definition-state-matrix. |
| [visible-uncertainty](/requirements/criterion-quality-readiness/criteria/visible-uncertainty) | partial-evidence | [Assess verification definitions without requiring execution](/tests/core-definition-coverage), [Require criterion-specific assertion mappings](/tests/core-definition-step-mapping), [Expose missing stale and excluded definition states](/tests/core-definition-known-uncertainty), [Keep unsupported definition states explicit](/tests/core-definition-remaining-states). Exact test assertions map to: core-definition-coverage/check-required-kinds, core-definition-step-mapping/check-specific-mapping, core-definition-known-uncertainty/check-known-reasons. Unmapped manual/planned assertions: core-definition-remaining-states/check-definition-state-matrix. |

#### [Understand verification boundaries](/requirements/quality-result-trust) {#story-quality-result-trust}

As a developer reading Quality, I can distinguish a defined verification approach from its implementation links and historical execution metadata so CI results never become Saga document verdicts.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [execution-origin](/requirements/quality-result-trust/criteria/execution-origin) | partial-evidence | [Keep historical executions separate from definitions](/tests/core-quality-boundary), [Keep legacy runs in explicit history](/tests/core-quality-history-separation), [Keep all definition surfaces independent of legacy execution](/tests/core-quality-run-outcome-matrix), [Expose changed definitions and keep commands inert](/tests/core-quality-definition-staleness). Exact test assertions map to: core-quality-boundary/check-run-removal, core-quality-boundary/check-conflicting-run-coverage, core-quality-history-separation/check-history-boundary, core-quality-definition-staleness/check-inert-command. Unmapped manual/planned assertions: core-quality-run-outcome-matrix/check-all-surfaces. |
| [observed-source-freshness](/requirements/quality-result-trust/criteria/observed-source-freshness) | partial-evidence | [Expose stale code without a fresh run](/tests/core-quality-source-staleness), [Expose changed definitions and keep commands inert](/tests/core-quality-definition-staleness), [Retain evidence revision pins and enrichment history](/tests/core-evidence-enrichment-history). Exact test assertions map to: core-quality-source-staleness/check-code-staleness, core-quality-definition-staleness/check-inert-command, core-quality-definition-staleness/check-definition-staleness. Unmapped manual/planned assertions: core-evidence-enrichment-history/check-exposed-provenance-fields. |

#### [Define verification and link its implementation](/requirements/record-test-proof) {#story-record-test-proof}

As a developer preparing a feature, I can define how its acceptance criteria are verified before code exists and enrich that same case with implementation evidence so intent and verification remain traceable as the feature develops.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [criterion-pin](/requirements/record-test-proof/criteria/criterion-pin) | partial-evidence | [Keep verifies links pinned when criterion content changes](/tests/core-criterion-verifies-currency). Exact test assertions map to: core-criterion-verifies-currency/check-criterion-pin. Unmapped manual/planned assertions: core-criterion-verifies-currency/check-conscious-repin. |
| [planned-definition](/requirements/record-test-proof/criteria/planned-definition) | partial-evidence | [Enrich the same planned case with step implementation evidence](/tests/core-planned-case-enrichment). Exact test assertions map to: core-planned-case-enrichment/check-planned-definition, core-planned-case-enrichment/check-built-case-reader. Unmapped manual/planned assertions: core-planned-case-enrichment/check-reader-identity. |
| [step-traceability](/requirements/record-test-proof/criteria/step-traceability) | evidence-linked | [Enrich the same planned case with step implementation evidence](/tests/core-planned-case-enrichment). Exact test assertions map to: core-planned-case-enrichment/check-step-traceability. No additional unmapped assertion is claimed by this definition. |
| [evidence-provenance](/requirements/record-test-proof/criteria/evidence-provenance) | partial-evidence | [Retain evidence revision pins and enrichment history](/tests/core-evidence-enrichment-history). Exact test assertions map to: core-evidence-enrichment-history/check-evidence-provenance. Unmapped manual/planned assertions: core-evidence-enrichment-history/check-exposed-provenance-fields. |
| [immutable-enrichment](/requirements/record-test-proof/criteria/immutable-enrichment) | evidence-linked | [Enrich a case without rewriting its definition](/tests/core-case-immutable-enrichment), [Require explicit reconciliation of every definition head](/tests/core-case-conflicting-heads), [Refuse reuse of a removed step ID](/tests/core-case-removed-step-id). Exact test assertions map to: core-case-immutable-enrichment/check-immutable-enrichment, core-case-conflicting-heads/check-definition-head-guards, core-case-removed-step-id/check-step-id-reuse. No additional unmapped assertion is claimed by this definition. |
| [run-provenance](/requirements/record-test-proof/criteria/run-provenance) | partial-evidence | [Retain immutable legacy run metadata without reviving execution gates](/tests/core-legacy-history-compatibility). Exact test assertions map to: core-legacy-history-compatibility/check-run-provenance. Unmapped manual/planned assertions: core-legacy-history-compatibility/check-all-run-fields. |
| [guarded-run](/requirements/record-test-proof/criteria/guarded-run) | evidence-linked | [Guard legacy run appends and preserve history](/tests/core-legacy-run-head-guards). Exact test assertions map to: core-legacy-run-head-guards/check-guarded-run. No additional unmapped assertion is claimed by this definition. |
| [existing-run-inputs](/requirements/record-test-proof/criteria/existing-run-inputs) | planned | [Reject missing inputs to legacy run history](/tests/core-legacy-run-missing-inputs). Unmapped manual/planned assertions: core-legacy-run-missing-inputs/check-existing-run-inputs. |

### repository-native {#feature-repository-native}

#### [Open the intended source context](/requirements/source-checkout-context) {#story-source-checkout-context}

As a developer, I can open or inspect a Saga against the intended source checkout so code-backed evidence is never silently resolved from a companion repository.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [in-repository-checkout](/requirements/source-checkout-context/criteria/in-repository-checkout) | evidence-linked | [Resolve the same Item evidence from companion and in-repository layouts](/tests/review-source-relocation). Public source-backed query compares explicit companion checkout with implicit in-source checkout and asserts current app.go evidence. Browser opening itself is not exercised. |
| [companion-checkout](/requirements/source-checkout-context/criteria/companion-checkout) | evidence-linked | [Resolve the same Item evidence from companion and in-repository layouts](/tests/review-source-relocation), [Read source changes from the intended checkout while Saga lives in a separate repository](/tests/review-companion-source-delta). Separate source/documentation repository fixture asserts source code delta and reason through --repo; relocation fixture additionally compares exact current Item evidence before/after moving into source. |
| [mismatch-refused](/requirements/source-checkout-context/criteria/mismatch-refused) | evidence-linked | [Refuse a different source repository unless override is explicit](/tests/review-repository-mismatch). Actual source comparison refuses different canonical identity and accepts explicit override. |

#### [Keep documentation with its source identity](/requirements/source-or-companion) {#story-source-or-companion}

As a developer, I can keep a Saga in the source repository or a companion repository while retaining the canonical identity and exact version of the source it documents.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [repository-identity](/requirements/source-or-companion/criteria/repository-identity) | partial-evidence | [Initialize portable source identity without transport credentials](/tests/review-canonical-repository). Portable canonicalization has direct assertion; Init persistence uses that value but persisted canonical identity is not asserted in inspected tests. |
| [companion-cursor](/requirements/source-or-companion/criteria/companion-cursor) | evidence-linked | [Record the exact commit documented by a companion Saga](/tests/review-companion-cursor). Exact cursor commit and subsequent source-head agreement are asserted. |

### review-decisions {#feature-review-decisions}

#### [Understand review-action provenance](/requirements/delegated-ai-attribution) {#story-delegated-ai-attribution}

As a review participant, I can see whether an action came from a human or an AI acting on a human's behalf so its authority and provenance are clear.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [human](/requirements/delegated-ai-attribution/criteria/human) | partial-evidence | [Show the human actor on a human-authored action](/tests/review-human-provenance). A committed human decision Author is asserted; other action kinds and visible label are not directly asserted. |
| [delegated-ai](/requirements/delegated-ai-attribution/criteria/delegated-ai) | ambiguous | [Display both an AI reviewer and its human principal](/tests/review-delegated-provenance). Implementation renders AI name/agent/model for Git-derived decision.Author (reviews.go:1142-1146); Git committer is not independently verified delegation. Existing CLI test checks AI model only. Clarify principal semantics and breadth across comments versus decisions; no test proves both identities together. |

#### [Record a review decision](/requirements/review-decision) {#story-review-decision}

As a developer reviewing a change, I can approve it or request changes so my review decision is explicit.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [approve](/requirements/review-decision/criteria/approve) | evidence-linked | [Record approval and a request for changes from their slide controls](/tests/review-decision-ui). Browser assertion directly exercises Approve. |
| [request-changes](/requirements/review-decision/criteria/request-changes) | evidence-linked | [Record approval and a request for changes from their slide controls](/tests/review-decision-ui). Browser assertion directly exercises Request changes. |
| [identifies-author](/requirements/review-decision/criteria/identifies-author) | partial-evidence | [Keep a committed human decision attributed to its author](/tests/review-decision-author). Report author is asserted; visible human author presentation remains planned. |
| [preserves-history](/requirements/review-decision/criteria/preserves-history) | partial-evidence | [Replace an approval with changes requested without losing the approval](/tests/review-decision-replacement-history). Append-only storage preserves original approval in a longer sequence; direct approve-to-request-changes replacement and reader history are not jointly asserted. |

#### [Know whether a review decision still applies](/requirements/review-decision-freshness) {#story-review-decision-freshness}

As a developer reviewing an evolving change, I can see whether each decision still applies and why it does not so I know which feedback must be revisited.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [current](/requirements/review-decision-freshness/criteria/current) | evidence-linked | [Keep decisions current when their slide and referenced code are unchanged](/tests/review-decision-current). Tests assert current before edits and unchanged sibling remains current after an unrelated code edit. |
| [out-of-date](/requirements/review-decision-freshness/criteria/out-of-date) | evidence-linked | [Name the exact changed code that invalidates a decision](/tests/review-decision-code-stale), [Name a changed slide that invalidates a decision](/tests/review-decision-slide-stale). Separate source and visual changes assert out-of-date with path/line or slide-change reasons. |
| [unknown](/requirements/review-decision-freshness/criteria/unknown) | ambiguous | [Report unknown currency when the decision commit or head is unreadable](/tests/review-decision-unreadable). No inspected test asserts decision unknown currency. currency at reviewstate.go:382-398 returns out_of_date instead of unknown when a changed slide is already known even if head/commit is unreadable; criterion does not specify precedence. Planned case isolates unchanged slides. |

#### [Carry a review discussion to resolution](/requirements/review-thread-lifecycle) {#story-review-thread-lifecycle}

As a developer reviewing a change, I can reply to, resolve, and reopen an anchored discussion so its outcome and full context remain visible.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [reply-context](/requirements/review-thread-lifecycle/criteria/reply-context) | partial-evidence | [Reply to an Item thread and resolve it](/tests/review-thread-reply-resolve), [Reopen a resolved thread without losing ordered discussion](/tests/review-thread-reopen-history). CLI asserts same-slide comment counts and browser shows reply; exact inherited target and root-thread identity remain planned. |
| [resolve-reopen](/requirements/review-thread-lifecycle/criteria/resolve-reopen) | partial-evidence | [Reply to an Item thread and resolve it](/tests/review-thread-reply-resolve), [Reopen a resolved thread without losing ordered discussion](/tests/review-thread-reopen-history). Resolve updates open-thread count in CLI assertions; no inspected slide/Item reopen assertion. Threads implementation uses latest nonempty state. |
| [preserves-discussion](/requirements/review-thread-lifecycle/criteria/preserves-discussion) | planned | [Reopen a resolved thread without losing ordered discussion](/tests/review-thread-reopen-history). No inspected assertion verifies ordered prior comments after both resolve and reopen; planned manual case preserves this requirement. |

#### [Withdraw a review decision](/requirements/withdraw-review-decision) {#story-withdraw-review-decision}

As a developer reviewing a change, I can withdraw my decision on a slide so the current review no longer presents a position I have retracted.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [withdraw-current](/requirements/withdraw-review-decision/criteria/withdraw-current) | evidence-linked | [Withdraw the current seat decision and preserve its history](/tests/review-decision-withdrawal). Browser test checks no current decision and both earlier approval and withdrawal records. |

### roadmap-dependency-mapping {#feature-roadmap-dependency-mapping}

#### [Communicate prerequisites between planned features](/requirements/map-roadmap-prerequisites) {#story-map-roadmap-prerequisites}

As a product manager, I can map dependencies between Roadmap features so the team understands which capabilities are prerequisites for others.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [directed-prerequisite](/requirements/map-roadmap-prerequisites/criteria/directed-prerequisite) | planned | [Read prerequisite direction between planned features](/tests/core-roadmap-prerequisite). Unmapped manual/planned assertions: core-roadmap-prerequisite/check-directed-prerequisite. |

### technical-inventory {#feature-technical-inventory}

#### [Understand data-model implications while designing](/requirements/erd-design-implications) {#story-erd-design-implications}

As an Architect defining a new design, I can see its ERD implications in the technical overview alongside the existing application data model so I can reason about how the proposed work changes the application's data.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [technical-overview](/requirements/erd-design-implications/criteria/technical-overview) | partial-evidence | [Open the application ERD and compare a proposed overlay](/tests/inventory-erd-entry-overlay). Application Data model entry and baseline/overlay composition are asserted. Automatic selection of a design from the technical overview is not asserted; case includes explicit overlay navigation. Evidence uses legacy ERD mode; named whiteboards now provide the primary technical-design document surface, so equivalence is not inferred. |
| [logical-and-persisted](/requirements/erd-design-implications/criteria/logical-and-persisted) | partial-evidence | [Read logical payloads, persisted records and declared relationships](/tests/inventory-erd-entity-meaning). PDF job/report fixture explicitly declares JSON payload, record and separate holders. Report holder/structure and job drawer holder are asserted; full JSON-payload presentation is not directly asserted. |
| [relationship-meaning](/requirements/erd-design-implications/criteria/relationship-meaning) | partial-evidence | [Read logical payloads, persisted records and declared relationships](/tests/inventory-erd-entity-meaning). Production direction and association/unknown cardinality text are asserted. Full visual distinction of all semantic relationship meanings still needs visual inspection. |
| [design-context](/requirements/erd-design-implications/criteria/design-context) | partial-evidence | [Open the application ERD and compare a proposed overlay](/tests/inventory-erd-entry-overlay). Overlay additions and unchanged baseline are asserted. Originating design/feature navigation is not asserted by the inspected overlay test. |
| [authored-overview](/requirements/erd-design-implications/criteria/authored-overview) | ambiguous | [Review the ERD as an authored architectural explanation](/tests/inventory-erd-authored-composition). Human-consumable and overall shape are qualitative; no objective rubric defines architectural adequacy. Manual review is defined without inventing required entity counts or layout rules. |
| [selective-detail](/requirements/erd-design-implications/criteria/selective-detail) | partial-evidence | [Read logical payloads, persisted records and declared relationships](/tests/inventory-erd-entity-meaning), [Review the ERD as an authored architectural explanation](/tests/inventory-erd-authored-composition). Tests assert the authored-fields/non-exhaustive disclaimer and selected fields. They do not assert omission never creates a missing-implementation finding across reports. |

#### [Discover proposed ERD changes through AI queries](/requirements/erd-discover-proposed-changes) {#story-erd-discover-proposed-changes}

As a Developer consuming a proposed implementation, I can have my AI retrieve the relevant proposed ERD changes and their design context so I know which data-model work to implement without reconstructing it from prose.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [query-changes](/requirements/erd-discover-proposed-changes/criteria/query-changes) | planned | [Retrieve design-specific ERD changes and their implementation context](/tests/inventory-erd-query-handoff). Feature reachability and identity newness have automation; no inspected assertion proves public relationship-level added/changed diffs for a selected design. Planned manual query case records this gap. |
| [scoped-proposals](/requirements/erd-discover-proposed-changes/criteria/scoped-proposals) | partial-evidence | [Query explicit proposals independently from comparison newness](/tests/inventory-public-roadmap), [Do not infer newness or hide conflicting definitions](/tests/inventory-baseline-boundaries), [Retrieve design-specific ERD changes and their implementation context](/tests/inventory-erd-query-handoff). Feature scope follows holders, relationships and Item pins; query tests inspect membership paths. A design-specific proposed relationship filter and unrelated-proposal exclusion are not directly asserted. |
| [separate-newness](/requirements/erd-discover-proposed-changes/criteria/separate-newness) | partial-evidence | [Query explicit proposals independently from comparison newness](/tests/inventory-public-roadmap). Format-2 tests independently filter intent/newness including implemented identities. No explicit data-relationship comparison-newness assertion is present. |
| [handoff-context](/requirements/erd-discover-proposed-changes/criteria/handoff-context) | planned | [Retrieve design-specific ERD changes and their implementation context](/tests/inventory-erd-query-handoff). No inspected test traces an ERD query result through committed design context plus current evidence to required/existing/unresolved work; planned manual case supplies that sequence. |

#### [Distinguish existing and proposed data in the ERD](/requirements/erd-distinguish-proposals) {#story-erd-distinguish-proposals}

As an Architect or Developer, I can easily distinguish existing data entities and relationships from proposed additions or changes in the ERD so I can understand the intended model without mistaking it for implemented reality.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [consistent-state](/requirements/erd-distinguish-proposals/criteria/consistent-state) | planned | [Compare entity and relationship states across all reading surfaces](/tests/inventory-erd-cross-surface-status). Individual surfaces have intent assertions, but no inspected test compares the same entities AND relationships across all four surfaces. Manual comparison avoids claiming cross-surface equivalence. |
| [visible-without-code](/requirements/erd-distinguish-proposals/criteria/visible-without-code) | partial-evidence | [Read logical payloads, persisted records and declared relationships](/tests/inventory-erd-entity-meaning), [Open the same canonical entity from drawing and directory](/tests/inventory-erd-canonical-navigation). Code-free proposed job remains visible in directory and details; proposed relationship fixture is renderable. Full design usability of code-free relationships is not directly exercised. |
| [evidence-health](/requirements/erd-distinguish-proposals/criteria/evidence-health) | planned | [Compare entity and relationship states across all reading surfaces](/tests/inventory-erd-cross-surface-status). Separate code-health fields exist, but the requested missing/stale/unresolved × intent × newness matrix for entities and relationships is not asserted together. |
| [preserved-proposal](/requirements/erd-distinguish-proposals/criteria/preserved-proposal) | partial-evidence | [Record implementation while preserving the proposed definition](/tests/inventory-implementation-history), [Implementation and coverage do not grant approval](/tests/inventory-no-approval-side-effects). Component transition retains r1 and delivery/evidence r2. Data-entity/relationship transitions and absence of verification/approval side effects require manual extension. |

#### [Inspect a data entity in application context](/requirements/erd-entity-context) {#story-erd-entity-context}

As an Architect or Developer, I can easily find the specific details of a data entity in the ERD so I understand its structure, purpose, and role in the wider application.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [diagram-and-directory](/requirements/erd-entity-context/criteria/diagram-and-directory) | partial-evidence | [Open the same canonical entity from drawing and directory](/tests/inventory-erd-canonical-navigation). Browser automation asserts drawing/directory canonical pins on the legacy authored ERD surface. New named-whiteboard navigation deliberately exposes document/instance focus; equivalent expandable entity-directory behavior there is not established. Preserve both current stories rather than inferring that one supersedes the other. |
| [structure-and-resource](/requirements/erd-entity-context/criteria/structure-and-resource) | partial-evidence | [Read logical payloads, persisted records and declared relationships](/tests/inventory-erd-entity-meaning). Exact report fields/holder and job holder text are asserted; exhaustive payload-purpose rendering remains manual. |
| [relationship-navigation](/requirements/erd-entity-context/criteria/relationship-navigation) | partial-evidence | [Read logical payloads, persisted records and declared relationships](/tests/inventory-erd-entity-meaning). Details assert outgoing/incoming relationship IDs and ERD links; complete related-entity click-through is a manual step. |
| [code-and-usages](/requirements/erd-entity-context/criteria/code-and-usages) | planned | [Follow an ERD entity to its feature behavior and code](/tests/inventory-entity-progressive-detail). Generic System usage and code tests do not directly exercise full ERD data-entity code-currency plus feature/deck/slide/Item reverse navigation. |
| [progressive-detail](/requirements/erd-entity-context/criteria/progressive-detail) | planned | [Follow an ERD entity to its feature behavior and code](/tests/inventory-entity-progressive-detail). Manual entity→implementation Item→selected code scenario covers this architectural-to-feature reading path; inspected browser automation uses System, not ERD entity. |

#### [Inspect a design without losing its history](/requirements/inspect-named-whiteboard-history) {#story-inspect-named-whiteboard-history}

As a developer, I can focus a named whiteboard and reopen an exact revision so I can inspect a design in the context in which it was authored.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [named](/requirements/inspect-named-whiteboard-history/criteria/named) | evidence-linked | [Publish named whiteboards without changing their siblings](/tests/inventory-whiteboard-independent-history). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |
| [history](/requirements/inspect-named-whiteboard-history/criteria/history) | evidence-linked | [Reopen an exact historical board after newer publication](/tests/inventory-whiteboard-historical-pins). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |
| [parent](/requirements/inspect-named-whiteboard-history/criteria/parent) | evidence-linked | [Reject publication based on an outdated board parent](/tests/inventory-whiteboard-stale-parent). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |
| [focus](/requirements/inspect-named-whiteboard-history/criteria/focus) | partial-evidence | [Focus exact instances within a named whiteboard](/tests/inventory-whiteboard-instance-focus). Repeated imported instance focus and history are asserted in browser; named native-section routing is server-tested, but section camera focus is not directly asserted by these evidence links. |

#### [Inspect code coverage by the technical inventory](/requirements/inventory-code-coverage) {#story-inventory-code-coverage}

As an Architect or Developer, I can report which code is accounted for by Systems, Components, and data entities and which is uncovered so I can identify gaps in the application's technical documentation.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [bounded-report](/requirements/inventory-code-coverage/criteria/bounded-report) | partial-evidence | [Measure bounded inventory coverage with exact owners](/tests/inventory-bounded-coverage). Scope counts and explicit-head measurement are asserted; CLI serialization names head/scope in implementation, but public head/scope values need the manual assertion. |
| [exact-owners](/requirements/inventory-code-coverage/criteria/exact-owners) | partial-evidence | [Measure bounded inventory coverage with exact owners](/tests/inventory-bounded-coverage). Overlap owner/range assertions and uncovered empty owners exist; every exact uncovered locator and data-entity ownership require wider manual inspection. |
| [separate-health](/requirements/inventory-code-coverage/criteria/separate-health) | evidence-linked | [Measure bounded inventory coverage with exact owners](/tests/inventory-bounded-coverage), [Exclude stale and conflicting evidence from current coverage](/tests/inventory-stale-coverage). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |
| [deduplicate](/requirements/inventory-code-coverage/criteria/deduplicate) | evidence-linked | [Measure bounded inventory coverage with exact owners](/tests/inventory-bounded-coverage), [Exclude stale and conflicting evidence from current coverage](/tests/inventory-stale-coverage). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |

#### [Compose designs from shared technical entities](/requirements/inventory-compose-designs) {#story-inventory-compose-designs}

As an Architect, I can reference existing Systems, Components, and data entities in implementation decks so my designs use consistent application concepts grounded in the application codebase.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [shared-identity](/requirements/inventory-compose-designs/criteria/shared-identity) | partial-evidence | [Reuse one canonical definition from separate design contexts](/tests/inventory-shared-contexts). Automation shares one System across two slides in one deck. Requirement says two decks and includes all three entity kinds; manual extension is explicit. |
| [code-grounding](/requirements/inventory-compose-designs/criteria/code-grounding) | partial-evidence | [Compare entity and relationship states across all reading surfaces](/tests/inventory-erd-cross-surface-status), [Read canonical definitions, exact code and stale link health](/tests/inventory-definition-query), [Reuse one canonical definition from separate design contexts](/tests/inventory-shared-contexts). Saved pin staleness, exact source evidence and changed-byte stale query are asserted in separate fixtures. Full drawer code-health matrix needs manual check. |
| [contextual-use](/requirements/inventory-compose-designs/criteria/contextual-use) | partial-evidence | [Reuse one canonical definition from separate design contexts](/tests/inventory-shared-contexts). Browser opens reused System in two contexts; distinct per-deck explanations retaining canonical links are not directly asserted. |

#### [Carry technical design into implementation review](/requirements/inventory-design-to-review) {#story-inventory-design-to-review}

As an Architect or Developer, I can reuse the same technical entities and scoped evidence in review decks and compare the committed proposed design with delivered implementation so additions and departures remain understandable.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [review-reuse](/requirements/inventory-design-to-review/criteria/review-reuse) | partial-evidence | [Carry canonical inventory pins and code selections into review](/tests/inventory-review-saved-view). Review Item stores exact System→Component selection; full same-identity handoff including data entity remains manual. |
| [compare-plan](/requirements/inventory-design-to-review/criteria/compare-plan) | partial-evidence | [Carry canonical inventory pins and code selections into review](/tests/inventory-review-saved-view). Saved-view commit binding is tested, but the fixture starts implemented. Proposed-plan-to-delivered comparison is explicitly manual. |
| [visible-evolution](/requirements/inventory-design-to-review/criteria/visible-evolution) | partial-evidence | [Carry canonical inventory pins and code selections into review](/tests/inventory-review-saved-view). Successor leaves old review pin/selection intact. New entity and changed-selection history comparison are not asserted end to end. |
| [no-implied-approval](/requirements/inventory-design-to-review/criteria/no-implied-approval) | planned | [Implementation and coverage do not grant approval](/tests/inventory-no-approval-side-effects). No linked assertion compares approval records before/after references and inherited coverage; manual isolation case includes unexplained changed code. |

#### [Understand a technical entity and its uses](/requirements/inventory-documentation-and-usages) {#story-inventory-documentation-and-usages}

As an Architect or Developer, I can open a System, Component, or data entity's detailed explanation, inspect its code references, and find the implementation decks and Items that use it so I understand both the shared entity and its feature contexts.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [explanation](/requirements/inventory-documentation-and-usages/criteria/explanation) | partial-evidence | [Compare entity and relationship states across all reading surfaces](/tests/inventory-erd-cross-surface-status), [Navigate entity documentation to the referencing slide and Item](/tests/inventory-usage-navigation), [Discover all inventory kinds and preserve their selected revision context](/tests/inventory-all-kind-context-matrix). Page explanation and saved-pin status are asserted. Full exact code-reference currency display is not asserted by the same browser flow. |
| [reverse-uses](/requirements/inventory-documentation-and-usages/criteria/reverse-uses) | partial-evidence | [Follow declared reverse usages to their implementation Items](/tests/inventory-query-usages), [Navigate entity documentation to the referencing slide and Item](/tests/inventory-usage-navigation), [Discover all inventory kinds and preserve their selected revision context](/tests/inventory-all-kind-context-matrix). Query checks feature/path/slide and UI checks Item; all feature/deck/slide/Item fields together need manual verification. |
| [navigate-usage](/requirements/inventory-documentation-and-usages/criteria/navigate-usage) | evidence-linked | [Navigate entity documentation to the referencing slide and Item](/tests/inventory-usage-navigation). Browser usage opens checkout-flag; Item resolves exact System pin and focus/URL round trip is asserted. |

#### [Distinguish proposed entities from existing implementation](/requirements/inventory-proposed-and-existing) {#story-inventory-proposed-and-existing}

As an Architect or Developer, I can distinguish explicitly proposed technical entities from existing implementation and separately identify what is new relative to a comparison baseline so people and AI know both what remains to build and what a change introduced.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [proposal-without-code](/requirements/inventory-proposed-and-existing/criteria/proposal-without-code) | partial-evidence | [Create and reference a proposal before implementation](/tests/inventory-code-free-proposal). Code-free Component creation/query is asserted and data-entity fixture exists. Full System/Component/data entity design-reference matrix remains manual. |
| [explicit-status](/requirements/inventory-proposed-and-existing/criteria/explicit-status) | planned | [Compare entity and relationship states across all reading surfaces](/tests/inventory-erd-cross-surface-status). Manual matrix checks selected revision intent across docs/decks/queries and independent newness; isolated status tests cannot establish all-surface consistency. |
| [evidence-distinction](/requirements/inventory-proposed-and-existing/criteria/evidence-distinction) | planned | [Compare entity and relationship states across all reading surfaces](/tests/inventory-erd-cross-surface-status). Manual missing/stale/unavailable evidence matrix must distinguish existing-without-evidence from proposed and unspecified; no inference of existence is permitted from absent code. |
| [history](/requirements/inventory-proposed-and-existing/criteria/history) | evidence-linked | [Record implementation while preserving the proposed definition](/tests/inventory-implementation-history). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |
| [comparison-relative-new](/requirements/inventory-proposed-and-existing/criteria/comparison-relative-new) | partial-evidence | [Query explicit proposals independently from comparison newness](/tests/inventory-public-roadmap), [Do not infer newness or hide conflicting definitions](/tests/inventory-baseline-boundaries), [Discover all inventory kinds and preserve their selected revision context](/tests/inventory-all-kind-context-matrix). Named-baseline behavior, absent/unreadable baselines, implemented-new counts and missing-baseline rejection are tested; exact selected head/baseline values in each result need manual inspection. |
| [evidence-backed-transition](/requirements/inventory-proposed-and-existing/criteria/evidence-backed-transition) | partial-evidence | [Record implementation while preserving the proposed definition](/tests/inventory-implementation-history), [Reject unsupported implementation assertions](/tests/inventory-reject-bad-delivery). Positive transition, missing delivery/current-parent, stale and forged evidence refusals are asserted. Explicit empty evidence and unresolvable commit/range variants remain manual. |
| [remaining-roadmap](/requirements/inventory-proposed-and-existing/criteria/remaining-roadmap) | partial-evidence | [Query explicit proposals independently from comparison newness](/tests/inventory-public-roadmap), [Do not infer newness or hide conflicting definitions](/tests/inventory-baseline-boundaries), [Keep a proposed successor discoverable after delivery](/tests/inventory-successor-roadmap). Current implemented store excluded from proposed filter, proposed job/queue included, and conflicts retained. Same-identity successor filter after delivery is not directly asserted. |
| [implementation-not-approval](/requirements/inventory-proposed-and-existing/criteria/implementation-not-approval) | planned | [Implementation and coverage do not grant approval](/tests/inventory-no-approval-side-effects). Implementation writer and explanatory UI separate intent from approval, but no inspected assertion checks absence of verification/integration/review events; manual before/after histories are defined. |
| [same-identity-successor](/requirements/inventory-proposed-and-existing/criteria/same-identity-successor) | partial-evidence | [Record implementation while preserving the proposed definition](/tests/inventory-implementation-history), [Discover all inventory kinds and preserve their selected revision context](/tests/inventory-all-kind-context-matrix). Component same-identity implemented baseline and current proposal slide pin are asserted; System and data-entity variants remain unasserted. |

#### [Discover technical context through the CLI](/requirements/inventory-query-technical-context) {#story-inventory-query-technical-context}

As an Architect or Developer directing AI work, I can query Systems, Components, data entities, their relationships, code, and usages so the AI can identify reusable entities and the relevant code subset before exploring raw source.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [discover-definitions](/requirements/inventory-query-technical-context/criteria/discover-definitions) | partial-evidence | [Read canonical definitions, exact code and stale link health](/tests/inventory-definition-query), [Discover all inventory kinds and preserve their selected revision context](/tests/inventory-all-kind-context-matrix). Exact evidence commit/digest, explicit intent, Component/System identities and data-entity filter fixtures exist; complete identity/description/health fields for all kinds are not asserted together. |
| [transitive-navigation](/requirements/inventory-query-technical-context/criteria/transitive-navigation) | partial-evidence | [Query explicit proposals independently from comparison newness](/tests/inventory-public-roadmap), [Read canonical definitions, exact code and stale link health](/tests/inventory-definition-query), [Discover all inventory kinds and preserve their selected revision context](/tests/inventory-all-kind-context-matrix). Declared membership, holder, relationship scope and exact code query are tested separately; complete end-to-end entry→selected code subset navigation is manual. |
| [usage-navigation](/requirements/inventory-query-technical-context/criteria/usage-navigation) | evidence-linked | [Follow declared reverse usages to their implementation Items](/tests/inventory-query-usages). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |
| [truthful-results](/requirements/inventory-query-technical-context/criteria/truthful-results) | partial-evidence | [Do not infer newness or hide conflicting definitions](/tests/inventory-baseline-boundaries), [Read canonical definitions, exact code and stale link health](/tests/inventory-definition-query), [Follow declared reverse usages to their implementation Items](/tests/inventory-query-usages), [Discover all inventory kinds and preserve their selected revision context](/tests/inventory-all-kind-context-matrix). Tests cover conflicting heads, unspecified legacy intent, stale links/code and complete confirmed zero uses. Missing-link versus confirmed absence and historical record classification need wider matrix. |
| [filter-proposed](/requirements/inventory-query-technical-context/criteria/filter-proposed) | partial-evidence | [Query explicit proposals independently from comparison newness](/tests/inventory-public-roadmap), [Discover all inventory kinds and preserve their selected revision context](/tests/inventory-all-kind-context-matrix). Component/data-entity intent filter assertions exist; explicit proposed System inclusion and returned selected-state values for every kind need manual extension. |
| [filter-new](/requirements/inventory-query-technical-context/criteria/filter-new) | evidence-linked | [Query explicit proposals independently from comparison newness](/tests/inventory-public-roadmap), [Do not infer newness or hide conflicting definitions](/tests/inventory-baseline-boundaries). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |
| [scoped-roadmap](/requirements/inventory-query-technical-context/criteria/scoped-roadmap) | partial-evidence | [Query explicit proposals independently from comparison newness](/tests/inventory-public-roadmap), [Do not infer newness or hide conflicting definitions](/tests/inventory-baseline-boundaries), [Retrieve design-specific ERD changes and their implementation context](/tests/inventory-erd-query-handoff), [Discover all inventory kinds and preserve their selected revision context](/tests/inventory-all-kind-context-matrix). Feature reachability and scope paths are asserted; combined proposed/new filters, selected design scope, and relationship changes are broader than inspected assertions. |

#### [Reconcile entities without implementation-deck references](/requirements/inventory-reconcile-unreferenced) {#story-inventory-reconcile-unreferenced}

As an Architect or Developer, I can identify Systems, Components, and data entities without implementation-deck references and decide how to reconcile them so the inventory remains connected to the application's feature implementations.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [explicit-proposal](/requirements/inventory-reconcile-unreferenced/criteria/explicit-proposal) | partial-evidence | [Ask for reconciliation of existing unreferenced entities](/tests/inventory-unreferenced-choice). Newness exemption, existing and unknown baseline choices are tested; explicit proposal exemption is implemented but not asserted by the linked reconciliation test. |
| [ask-user](/requirements/inventory-reconcile-unreferenced/criteria/ask-user) | evidence-linked | [Ask for reconciliation of existing unreferenced entities](/tests/inventory-unreferenced-choice). Reconciliation requires user choice for old unreferenced entity and asserts no Saga mutation, including unknown baseline. |
| [actionable-context](/requirements/inventory-reconcile-unreferenced/criteria/actionable-context) | partial-evidence | [Ask for reconciliation of existing unreferenced entities](/tests/inventory-unreferenced-choice). Task identity/guidance and revision repair are tested. Entity explanation, evidence state and complete absent uses together require the manual follow-through. |

#### [Reuse scoped inventory coverage in implementation decks](/requirements/inventory-scoped-deck-coverage) {#story-inventory-scoped-deck-coverage}

As an Architect or Developer, I can select the code relevant to an implementation deck through its referenced Systems, Components, and data entities so the deck reuses grounded evidence without claiming the entities' entire implementations.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [selected-subset](/requirements/inventory-scoped-deck-coverage/criteria/selected-subset) | evidence-linked | [Credit only selected code through declared inventory paths](/tests/inventory-inherited-subset), [Inspect selected code and return to the originating Item](/tests/inventory-selected-code-roundtrip). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |
| [inherited-evidence](/requirements/inventory-scoped-deck-coverage/criteria/inherited-evidence) | evidence-linked | [Credit only selected code through declared inventory paths](/tests/inventory-inherited-subset), [Expose inherited ownership and unresolved selection paths](/tests/inventory-inheritance-provenance). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |
| [explain-path](/requirements/inventory-scoped-deck-coverage/criteria/explain-path) | partial-evidence | [Expose inherited ownership and unresolved selection paths](/tests/inventory-inheritance-provenance), [Inspect selected code and return to the originating Item](/tests/inventory-selected-code-roundtrip). Inherited Item/selection/two-hop path and browser selected code are asserted; exact source location plus all System/Component/data-entity path variants in report need manual verification. |
| [exclude-unselected](/requirements/inventory-scoped-deck-coverage/criteria/exclude-unselected) | partial-evidence | [Credit only selected code through declared inventory paths](/tests/inventory-inherited-subset), [Expose inherited ownership and unresolved selection paths](/tests/inventory-inheritance-provenance), [Keep stale selections visible while excluding them from coverage](/tests/inventory-inheritance-health). Unselected, unresolved, stale selected bytes and noncurrent paths are asserted. Overlapping inherited selections are explicitly manual; inventory-wide overlap is not substituted as proof. |

#### [Trace a diagram entity to relevant code](/requirements/inventory-trace-slide-code) {#story-inventory-trace-slide-code}

As an Architect or Developer reviewing an implementation deck, I can follow a referenced System, Component, or data entity to the particular code relevant to the slide and return without losing my place.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [selected-code](/requirements/inventory-trace-slide-code/criteria/selected-code) | evidence-linked | [Inspect selected code and return to the originating Item](/tests/inventory-selected-code-roundtrip). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |
| [source-identity](/requirements/inventory-trace-slide-code/criteria/source-identity) | partial-evidence | [Inspect selected code and return to the originating Item](/tests/inventory-selected-code-roundtrip). Exact line and integrity are browser-tested and source caption is implemented. Full commit/path/range display plus stale/unavailable no-substitution variants remain manual. |
| [return-context](/requirements/inventory-trace-slide-code/criteria/return-context) | evidence-linked | [Inspect selected code and return to the originating Item](/tests/inventory-selected-code-roundtrip). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |

#### [Read an application through its technical design](/requirements/read-application-technical-design) {#story-read-application-technical-design}

As an architect, I can read one approach essay and inspect an ERD, Infrastructure and Data flows whiteboard so I can understand how the application works before changing it.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [approach](/requirements/read-application-technical-design/criteria/approach) | evidence-linked | [Read the approach essay and reach its supporting whiteboards](/tests/inventory-technical-approach). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |
| [relationships](/requirements/read-application-technical-design/criteria/relationships) | evidence-linked | [Preserve one, many and optional association endpoints](/tests/inventory-technical-cardinality). Exact test assertions exercise the criterion behavior; implementation was inspected. These are implementation links, not test-run outcomes. |
| [platforms](/requirements/read-application-technical-design/criteria/platforms) | planned | [Read the actual local runtime infrastructure](/tests/inventory-technical-runtime-infrastructure). Generic named-board rendering cannot establish truth of this application's current runtime/resource diagram; manual source-backed architecture check is defined. |
| [flows](/requirements/read-application-technical-design/criteria/flows) | planned | [Follow directed labeled application flows](/tests/inventory-technical-data-movement). Shared diagram supports directed edges, but no inspected assertion validates authored authoring/reading/review flow semantics and persistence/failure boundaries; manual case is defined. |

### traceability {#feature-traceability}

#### [Follow a design in both directions](/requirements/design-usage-path) {#story-design-usage-path}

As a designer, I can see which stories a design addresses and which implementation evidence uses it so I know whether the delivered work reflects the design.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [bidirectional-path](/requirements/design-usage-path/criteria/bidirectional-path) | planned | [Follow design links to requirements and delivered evidence](/tests/contracts-design-paths). Inspected compose.go:103-120,346-445,470-492 and relation model/tests. Tests demonstrate stale pins and story-level design reachability but do not assert the entire design→requirement and design→implementation traversal or both endpoint pins round-trip. Requirement-revision context for a non-requirement edge is potentially transitive; do not invent a revision field on every edge. |
| [pinned-context](/requirements/design-usage-path/criteria/pinned-context) | planned | [Follow design links to requirements and delivered evidence](/tests/contracts-design-paths). Inspected compose.go:103-120,346-445,470-492 and relation model/tests. Tests demonstrate stale pins and story-level design reachability but do not assert the entire design→requirement and design→implementation traversal or both endpoint pins round-trip. Requirement-revision context for a non-requirement edge is potentially transitive; do not invent a revision field on every edge. |

#### [Follow product intent into the application](/requirements/product-intent-path) {#story-product-intent-path}

As a product manager, I can follow an accepted story through design and implementation so I can verify that product intent reached the application.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [current-paths](/requirements/product-intent-path/criteria/current-paths) | partial-evidence | [Preserve the explicit story hop for broad design](/tests/contracts-intent-broad-design-path), [Expose integrated evidence on a declared implementation path](/tests/contracts-intent-delivery-path), [Enumerate every declared current design and implementation path](/tests/contracts-intent-alternate-paths). Evidence covers representative paths; the universal "every available path" obligation needs an alternate-route completeness assertion. Separate fixtures are named explicitly; test names alone do not prove end-to-end completeness. |
| [stale-paths](/requirements/product-intent-path/criteria/stale-paths) | evidence-linked | [Expose stale links when design content changes](/tests/contracts-intent-stale-design), [Expose stale links when criterion wording changes](/tests/contracts-intent-stale-criterion).  Criterion-content changes are exercised; unchanged criterion pins can carry forward despite other story fields changing, so the broad criterion wording should not be interpreted as any unrelated sibling edit staling every relation. |

#### [Discover intent affected by code](/requirements/reverse-impact) {#story-reverse-impact}

As an architect, I can begin with changed code and discover the affected requirements, designs, tests, and personas so I can understand the change's system-wide consequences.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [reverse-path](/requirements/reverse-impact/criteria/reverse-path) | partial-evidence | [Resolve current code locations back to persisted intent](/tests/contracts-reverse-current-location). Need public reverse lookup assertions for persisted requirement/evidence identities and completeness; helper matching alone cannot substantiate the end-to-end criterion. |
| [blockers](/requirements/reverse-impact/criteria/blockers) | partial-evidence | [Show stale and conflicted reverse links without current claims](/tests/contracts-reverse-stale-conflict-report). Potential product gap: compose.go filters stale relations (347-351), skips conflicted stories (368-369), and excludes ref rows with no current code match (415-416). These filters avoid false current paths but may hide the stale/conflicted reverse path rather than report its blocker. A public visibility assertion is required. |

### visual-implementation {#feature-visual-implementation}

#### [Bind a diagram element to product intent](/requirements/diagram-element-intent) {#story-diagram-element-intent}

As an architect, I can bind a specific diagram element to the story or criterion it explains and inspect that intent separately from its code evidence so reviewers do not mistake a whole slide for implementation proof.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [item-only](/requirements/diagram-element-intent/criteria/item-only) | partial-evidence | [Reject broad visual intent links and accept exact Item links](/tests/review-item-only-intent). Both relation types and broad endpoints are checked for rejection/no writes, then accepted from Item. Rejection and acceptance assertions use different targets; the same-target endpoint matrix remains planned. |
| [separate-controls](/requirements/diagram-element-intent/criteria/separate-controls) | partial-evidence | [Inspect intent and code independently from one Item](/tests/review-story-code-controls). Both controls and their opening drawers are asserted; exclusive content/other-drawer absence remains planned. |
| [story-details](/requirements/diagram-element-intent/criteria/story-details) | partial-evidence | [Inspect intent and code independently from one Item](/tests/review-story-code-controls), [Show current story and criterion in the Item story drawer](/tests/review-story-details-current), [Expose stale and invalid Item intent links](/tests/review-story-details-stale-invalid). Current story/criterion, rationale and stale/invalid projection have assertions; stale/invalid Item-drawer rendering remains planned. |
| [summary-scope](/requirements/diagram-element-intent/criteria/summary-scope) | evidence-linked | [Count distinct exact-Item stories and isolate legacy links](/tests/review-story-summary-scope). Exact fixture asserts one distinct story, exclusions and migration guidance; legacy-only zero count is asserted. |
| [summary-preview](/requirements/diagram-element-intent/criteria/summary-preview) | partial-evidence | [Preview only Items with story links on hover and keyboard focus](/tests/review-story-summary-preview). Hover, unhover and focus assertions exist; focus removal clearing lacks direct assertion. |

#### [Navigate and present an implementation deck](/requirements/visual-deck-navigation) {#story-visual-deck-navigation}

As a developer reviewing technical work, I can move through a feature implementation deck and present the active slide so I can follow the explanation without losing my place.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [opens-first-slide](/requirements/visual-deck-navigation/criteria/opens-first-slide) | ambiguous | [Open the first slide from a feature Implementation section](/tests/review-implementation-opening). Current e2e implementation-deck.spec.ts:54-57 expects deck header navigation to show deck overview, and deckoverviewjs.go:43-59 supports overview/front/back. Criterion first-slide behavior conflicts with newer overview-first flow; clarify precise Implementation entry point before changing intent. |
| [bounded-navigation](/requirements/visual-deck-navigation/criteria/bounded-navigation) | partial-evidence | [Navigate slides within deck boundaries and keep position and anchor in sync](/tests/review-bounded-deck-navigation). Thumbnail/Next and deck boundary disabling are asserted; exact anchors and Previous transition remain planned. |
| [presentation](/requirements/visual-deck-navigation/criteria/presentation) | planned | [Present and exit while retaining the active slide](/tests/review-presentation). slides_test.go:62-76 checks JS/CSS string contracts only. No inspected browser assertion enters/exits presentation or verifies hotspot/chrome visibility and same-slide return. |

#### [Explain technical work visually](/requirements/visual-technical-explanation) {#story-visual-technical-explanation}

As an architect, I can explain technical work through ordered visual slides whose semantic elements reference exact code so reviewers can understand the system without reconstructing it from a file diff.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [media](/requirements/visual-technical-explanation/criteria/media) | partial-evidence | [Accept SVG and HTML implementation slides](/tests/review-svg-html-authoring), [Accept raster slides and sandbox interactive HTML](/tests/review-raster-html-sandbox). SVG and HTML authoring are directly exercised. Raster deck acceptance and actual HTML sandbox behavior remain planned; fragment media tests do not establish deck behavior. |
| [evidence](/requirements/visual-technical-explanation/criteria/evidence) | partial-evidence | [Attach exact committed source evidence to an addressable Item](/tests/review-item-exact-code-pin). Exact committed location is supplied and Item cover accepted. No mapped assertion checks persisted commit/path/line fields; plan public-read verification. |

### visual-review {#feature-visual-review}

#### [Discuss a review element](/requirements/contextual-comments) {#story-contextual-comments}

As a developer reviewing a change, I can comment on a specific review element so discussion stays connected to its context.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [element-context](/requirements/contextual-comments/criteria/element-context) | partial-evidence | [Attach a comment to an exact review Item](/tests/review-item-comment), [Attach an ordinary discussion to a whole review slide](/tests/review-slide-comment). Exact Item target has a round-trip assertion; ordinary slide comment association is planned independently of annotation tests. |

#### [See what is deprecated and why](/requirements/inspect-explicit-deprecation-scope) {#story-inspect-explicit-deprecation-scope}

As a developer, I can inspect an explicit deprecation annotation and its exact scope so I can avoid the deprecated content or code without guessing that nearby code is also deprecated.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [scope](/requirements/inspect-explicit-deprecation-scope/criteria/scope) | evidence-linked | [Distinguish explicit deprecated code from related code and neighboring lines](/tests/review-exact-deprecation). Exact code versus content roles and adjacent-line exclusion are asserted. |
| [reason](/requirements/inspect-explicit-deprecation-scope/criteria/reason) | partial-evidence | [Show the authored reason and explicitly chosen replacement](/tests/review-deprecation-reason). Reason rendering is asserted; optional selected replacement rendering is implemented but no inspected assertion exercises it. |
| [withdrawal](/requirements/inspect-explicit-deprecation-scope/criteria/withdrawal) | evidence-linked | [Withdraw an assertion while preserving its original revision](/tests/review-deprecation-withdrawal). Public CLI test asserts withdrawn lifecycle history and unchanged original assertion. |

#### [Add text to a visual review](/requirements/text-annotation) {#story-text-annotation}

As a developer reviewing a change, I can place text on a specific part of a slide or diagram so I can express visual feedback that a mark or shape alone cannot convey.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [text-overlay](/requirements/text-annotation/criteria/text-overlay) | partial-evidence | [Place text at a selected visual location](/tests/review-sticky-text-location). Sticky-note creation at chosen pointer coordinates and text are exercised; final exact location is not asserted. Case uses existing sticky-note implementation without inventing a different text tool. |

#### [Annotate a visual review](/requirements/visual-annotation) {#story-visual-annotation}

As a developer reviewing a change, I can annotate a specific part of a slide or diagram so my visual feedback is unambiguous.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [freehand](/requirements/visual-annotation/criteria/freehand) | planned | [Draw a freehand mark on a review slide](/tests/review-freehand). Freehand path implementation exists in appjs.go:3037-3047 but no inspected gesture/persist/display assertion. |
| [shapes](/requirements/visual-annotation/criteria/shapes) | partial-evidence | [Place and resize a rectangular review annotation](/tests/review-rectangle-place-resize). Rectangle placement and resizing are directly asserted. Criterion says basic shapes without naming required set; no assertion for resizing other supported kinds. |
| [anchored](/requirements/visual-annotation/criteria/anchored) | partial-evidence | [Keep an annotation attached to its slide through resizing and navigation](/tests/review-annotation-anchor). Normalized viewport scaling asserted; navigation, reload and no leakage to another slide remain planned. |
| [visible-author](/requirements/visual-annotation/criteria/visible-author) | planned | [Show an annotation and its author to another participant](/tests/review-annotation-participant-author). No inspected test changes participants and asserts visible annotation author; manual definition retains requirement. |
| [preserves-source](/requirements/visual-annotation/criteria/preserves-source) | planned | [Leave original visual bytes unchanged after annotation edits](/tests/review-annotation-source-immutable). Annotation history test preserves original anchor, not original source visual. No inspected byte-level visual immutability assertion. |

### work-planning {#feature-work-planning}

#### [Gate dependent work on a versioned contract](/requirements/contract-gated-handoff) {#story-contract-gated-handoff}

As an architect coordinating parallel work, I can define a versioned provider-consumer contract and record its lifecycle so dependent work opens only against the agreed revision.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [versioned-contract](/requirements/contract-gated-handoff/criteria/versioned-contract) | partial-evidence | [Persist a versioned provider-consumer contract definition](/tests/contracts-contract-definition). Create success is tested; exact field round-trip through public queries is not. query work-events exposes lifecycle events, not a dedicated full contract-definition projection; inspect step may reveal a public read gap. |
| [exact-revision-gate](/requirements/contract-gated-handoff/criteria/exact-revision-gate) | partial-evidence | [Require fulfillment of the current pinned contract revision](/tests/contracts-contract-exact-revision), [Keep unfulfilled or conflicted contract heads blocking](/tests/contracts-contract-conflicted-head). Exact revision mismatch is tested. Separate planned case covers lifecycle conflict and nonfulfilled states; no direct assertions for those contract-gate branches located. Requirements-only definition: public contract-state writer is absent, so do not emulate reconciliation by metadata editing. Gate branch implementation was inspected but is not test evidence. |
| [public-contract-state](/requirements/contract-gated-handoff/criteria/public-contract-state) | planned | [Append and query contract states through the public plan API](/tests/contracts-public-contract-state). Confirmed product gap: installed plan --help and internal/cli/living_plan.go:13-46 expose add-contract only, no state writer. Domain RecordContractState and workEventRows exist, but do not satisfy the public-command criterion. Do not invent a command name or claim this case is automated. |

#### [Plan work with explicit dependencies](/requirements/dependency-aware-work-plan) {#story-dependency-aware-work-plan}

As a product manager, I can organize work into ordered waves with explicit dependency gates so contributors can see what can start and why blocked work must wait.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [ordered-waves](/requirements/dependency-aware-work-plan/criteria/ordered-waves) | planned | [Read ordered waves without creating implicit wave barriers](/tests/contracts-waves-and-readiness). Inspected waveRows order sort (compose.go:123-166) and projectWorkItem (192-212); no exact query order/assignment or no-wave-barrier assertion located. TestWavesDoNotCreateImplicitBarriersAndDependenciesFormADAG only asserts DAG creation/rejection and count despite its name. |
| [no-wave-barrier](/requirements/dependency-aware-work-plan/criteria/no-wave-barrier) | planned | [Read ordered waves without creating implicit wave barriers](/tests/contracts-waves-and-readiness). Inspected waveRows order sort (compose.go:123-166) and projectWorkItem (192-212); no exact query order/assignment or no-wave-barrier assertion located. TestWavesDoNotCreateImplicitBarriersAndDependenciesFormADAG only asserts DAG creation/rejection and count despite its name. |
| [fact-gates](/requirements/dependency-aware-work-plan/criteria/fact-gates) | partial-evidence | [Evaluate progress, merge and exact-contract dependency facts](/tests/contracts-dependency-fact-gates). Inspected assertions exercise valid three-gate forms and missing merge reason. Public per-dependency blocker identities, all conflict branches, and invalid mixed-gate rejection need explicit assertions. |
| [blocker-reasons](/requirements/dependency-aware-work-plan/criteria/blocker-reasons) | partial-evidence | [Evaluate progress, merge and exact-contract dependency facts](/tests/contracts-dependency-fact-gates). Inspected assertions exercise valid three-gate forms and missing merge reason. Public per-dependency blocker identities, all conflict branches, and invalid mixed-gate rejection need explicit assertions. |
| [acyclic](/requirements/dependency-aware-work-plan/criteria/acyclic) | evidence-linked | [Reject a dependency that closes a cycle](/tests/contracts-dependency-cycle-rejection).  |

#### [Track work from assignment through integration](/requirements/track-work-delivery) {#story-track-work-delivery}

As an Architect or Developer coordinating delivery, I can associate specific technical entity identities with a work item's workspace, progress, and merge evidence so contributors and their AI workers can discover their assigned scope and collaborators can track it through integration.

| Criterion | Assessment | Definitions and findings |
| --- | --- | --- |
| [workspace-ownership](/requirements/track-work-delivery/criteria/workspace-ownership) | partial-evidence | [Retain portable workspace assignment and release history](/tests/contracts-workspace-ownership-events). Missing exact query field round-trip and released-not-active assertions; current test validates persistence portability and parent linkage only. |
| [progress-history](/requirements/track-work-delivery/criteria/progress-history) | partial-evidence | [Preserve progress history and expose current state only for a unique head](/tests/contracts-progress-history-heads). Domain heads/reconciliation are covered, but public history complete contents and current-progress round trip need direct assertions. |
| [merge-evidence](/requirements/track-work-delivery/criteria/merge-evidence) | partial-evidence | [Record required merge lifecycle with exact head and merge commit](/tests/contracts-merge-delivery-evidence). No public query assertion for exact integrated OIDs, each required unit lifecycle and event ancestry located; mutation success alone is insufficient. |
| [done-is-not-delivery](/requirements/track-work-delivery/criteria/done-is-not-delivery) | partial-evidence | [Keep done work undelivered until immutable evidence exists](/tests/contracts-done-not-delivery). Traceability delivery is directly asserted. The criterion names peer-review readiness; add an exact readiness API assertion rather than silently substituting delivered for ready. |
| [technical-entity-scope](/requirements/track-work-delivery/criteria/technical-entity-scope) | planned | [Assign existing technical identities without duplicating inventory or personas](/tests/contracts-assigned-entity-scope). Product gap in inspected workplan model: WorkItemRevision has relations/touch areas but no typed System/Component/data-entity assignment, and WorkspaceEvent has workspace-only fields. No supported assignment/query bridge or automated assertions found; do not infer entity ownership from file touch areas. |
| [query-assigned-entities](/requirements/track-work-delivery/criteria/query-assigned-entities) | planned | [Retrieve only a worker’s assigned entity roadmap context](/tests/contracts-assigned-entity-query). Public inventory can report intent/code health, but no typed work-assignment filter/bridge is present in the inspected model/query work-item projection. This is not covered by ordinary inventory query tests. |
| [entity-reconciliation](/requirements/track-work-delivery/criteria/entity-reconciliation) | planned | [Update an assigned entity roadmap without erasing proposal or assignment history](/tests/contracts-assigned-entity-reconciliation). No assigned-entity reconciliation bridge or combined assignment/proposal/integration assertions located. Existing inventory lifecycle and generic done-not-delivery mechanisms do not establish this cross-domain criterion. |

## Evidence inspection notes {#evidence-inspection-notes}

These are source-inspection notes, not run summaries. Evidence IDs are local to the linked canonical case. Open that case for the complete committed selectors and criterion-specific steps. Planned steps intentionally have no implementation evidence.

### [Reject malformed and cross-context continuation tokens](/tests/contracts-cursor-context-rejection) {#case-contracts-cursor-context-rejection}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `context-errors` | test_implementation → `reject` | Directly asserts tampering, cross-operation and changed-filter invalid_argument. |
| `cursor-check` | implementation_under_test → `reject`, `retryability` | Validates operation/key integrity and returns non-retryable invalidCursor. |

### [Traverse two bounded child pages without duplicates](/tests/contracts-cursor-pages) {#case-contracts-cursor-pages}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `two-page-assertions` | test_implementation → `first`, `last` | Asserts both metadata states, exact counts, and distinct target identities for the two-record fixture. |
| `page-builder` | implementation_under_test → `first`, `last` | Review-query pager exercised by the linked two-page assertions. |

### [Reject a cursor after the Saga snapshot changes](/tests/contracts-cursor-stale-snapshot) {#case-contracts-cursor-stale-snapshot}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `stale-error` | test_implementation → `issue`, `advance` | Asserts old-token rejection and retryability against a changed snapshot. |
| `snapshot-guard` | implementation_under_test → `advance` | Rejects a snapshot mismatch before offset slicing. |

### [Check cursor-page defaults and limit boundaries](/tests/contracts-page-limits) {#case-contracts-page-limits}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `invalid-limit-table` | test_implementation → `invalid` | The table contains gaps limits 0,1001,NaN and asserts invalid_argument before opening the Saga. It does not cover the separate negative requirements step. |
| `documented-constants` | test_implementation → `constants` | Asserts constants 100/1000; does not exercise default row count in a 1001-record query. |
| `limit-validation` | implementation_under_test → `default`, `bounds`, `invalid` | Uses default and enforces range in the living collection pager. |

### [Discover operation usage, pagination and stable errors](/tests/contracts-query-operation-discovery) {#case-contracts-query-operation-discovery}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `pagination-schema` | test_implementation → `paths` | Asserts operation identity, expected response paths and pagination counted path; purpose/input completeness is not asserted. |
| `record-discovery` | test_implementation → `records` | Asserts usage/pagination and record operations in spec; not every operation/input/error field. |
| `description` | implementation_under_test → `schema` | Publishes purpose, usage, data paths and pagination. |

### [Emit one versioned JSON envelope for query success and failure](/tests/contracts-query-versioned-envelope) {#case-contracts-query-versioned-envelope}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `common-success` | test_implementation → `success` | Asserts version/snapshot and real-query data/page shape across enumerated operations. |
| `common-error` | test_implementation → `failure` | Uses decodeOneJSONValue and validates code/retryability. |
| `one-encoder` | implementation_under_test → `success`, `failure` | Shared versioned success/error envelope serialization. |

### [Classify records introduced after the comparison base](/tests/review-added-records) {#case-review-added-records}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `added` | Asserts all records added for absent Saga base. |
| `evidence-2` | implementation_under_test → `added` | Missing base node becomes an added record. |

### [Propagate code impact without claiming Saga records were edited](/tests/review-code-chain-impact) {#case-review-code-chain-impact}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `code`, `chain` | Asserts no edited records, code cause, traceability cause/via, and code grouped under affected design. |
| `evidence-2` | implementation_under_test → `chain` | Traverses containment and semantic links to attribute chain impact. |

### [Distinguish committed event history from uncommitted record changes](/tests/review-history-multiple-and-dirty) {#case-review-history-multiple-and-dirty}

Requirements-only definition; no implementation evidence is claimed.

### [Read the introducing replacement event and reopen its comparison](/tests/review-history-replacement) {#case-review-history-replacement}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `history`, `open` | Checks introducing subject, replaced URN and command endpoints; does not enumerate multiple events or uncommitted flag. |
| `evidence-2` | implementation_under_test → `history`, `open` | Builds reasoned events, comparison commands and replacement context. |

### [Resolve a diverged comparison to its actual merge base](/tests/review-merge-base) {#case-review-merge-base}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `range` | Diverged topology asserts exact merge base/head and excludes advanced-base-only file. |
| `evidence-2` | implementation_under_test → `range` | Resolves both commits and calls merge-base. |
| `evidence-3` | implementation_under_test → `mode` | Range mode is compare when against is present. |

### [Observe the selected committed head without comparison layers](/tests/review-observe) {#case-review-observe}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `observe` | Asserts observe mode, absent comparison and zero total at default HEAD; does not assert an explicit older head. |
| `evidence-2` | implementation_under_test → `observe`, `selected-head` | Observe uses the selected head as both comparison endpoints. |

### [Retire a replaced slide while retaining its replacement context](/tests/review-retired-replacement-history) {#case-review-retired-replacement-history}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `replace` | Asserts retired/removed classification, replacement pair and commit subject/body. |
| `evidence-2` | implementation_under_test → `replace` | A removed record retains its before snapshot and retired classification. |

### [Separate a revised story from the relation pinned to its previous revision](/tests/review-revised-record-and-pin) {#case-review-revised-record-and-pin}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `revision` | Asserts revised classification and exact before/after text. |
| `evidence-2` | test_implementation → `pin` | Asserts CausePin for the relation. |
| `evidence-3` | implementation_under_test → `revision` | Compares revisions and records snapshots. |
| `evidence-4` | implementation_under_test → `pin` | Unchanged relations with changed endpoints receive pin causes. |

### [Reject WORKTREE and explain how to compare committed work](/tests/review-worktree-refusal) {#case-review-worktree-refusal}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `reject` | Asserts rejection and WORKTREE-is-not-supported text, not the commit guidance. |
| `evidence-2` | implementation_under_test → `reject`, `guidance` | Error contains the required commit-and-compare-HEAD guidance. |

### [Keep factual coverage separate from review approval](/tests/contracts-coverage-no-verdict) {#case-contracts-coverage-no-verdict}

Requirements-only definition; no implementation evidence is claimed.

### [Distinguish complete, gap and untrustworthy check results](/tests/contracts-coverage-result-codes) {#case-contracts-coverage-result-codes}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `complete-gap` | test_implementation → `complete`, `gap` | Asserts success vs checkExitUncovered. |
| `untrustworthy` | test_implementation → `untrusted` | Asserts invalid Saga returns error outside coverage StatusError path. |
| `exit-contract` | implementation_under_test → `gap`, `untrusted` | Defines gap exit 3 and returns trust errors before gap evaluation. |

### [Distinguish covered lines, gaps, overlaps and stale references](/tests/contracts-coverage-states) {#case-contracts-coverage-states}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `counts-and-details` | test_implementation → `evaluate` | Assertions establish exact independent counts and locations. |
| `coverage-fixture` | test_implementation → `evaluate` | Defines current, overlapping, uncovered and stale inputs. |
| `report-fields` | implementation_under_test → `evaluate` | Separate output collections and summary. |

### [Resolve each required axis to linked, excluded or a reasoned gap](/tests/contracts-criterion-axis-reasons) {#case-contracts-criterion-axis-reasons}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `six-axis-fixture` | test_implementation → `project` | Inputs isolate linked/excluded/stale and missing axes. |
| `three-way-resolution` | test_implementation → `project` | Asserts all axes, resolution exclusivity and exact counts. |
| `stale-reason` | test_implementation → `stale` | Asserts retained stale reason. |
| `classification` | implementation_under_test → `project`, `stale` | Resolves detailed states into the three-state model with gap reasons. |

### [Report documentation areas independently with covered and gap entries](/tests/contracts-documentation-named-areas) {#case-contracts-documentation-named-areas}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `area-counts` | test_implementation → `areas` | Asserts independent counts, entries and empty axes. |
| `serialized-areas` | test_implementation → `areas` | Asserts all named areas and absence of null collections. |
| `area-composition` | implementation_under_test → `areas`, `status` | Composes each area independently. |

### [Keep existing documentation debt separate from a new change](/tests/contracts-reconcile-baseline-debt) {#case-contracts-reconcile-baseline-debt}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `baseline` | test_implementation → `debt` | Creates pre-existing debt and asserts classification. |
| `report-boundary` | implementation_under_test → `debt` | Builds exact comparison and declares separate currency/semantic limits. |

### [Distinguish living currency regression from base-side coverage](/tests/contracts-reconcile-currency-regression) {#case-contracts-reconcile-currency-regression}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `regression` | test_implementation → `regression` | Separates changed-reference currency from base-side diff validity. |
| `report-boundary` | implementation_under_test → `regression` | Builds exact comparison and declares separate currency/semantic limits. |

### [Find declared-link reassessment after code and requirement-only changes](/tests/contracts-reconcile-impacted-records) {#case-contracts-reconcile-impacted-records}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `code-queue` | test_implementation → `code` | Asserts story reassessment exists alongside evidence repair. |
| `requirement-queue` | test_implementation → `requirement` | Asserts no fabricated source change and named affected record classes/routes. |
| `causes` | implementation_under_test → `reasons` | Copies declared affected causes and explicit requirement_change reasons. |

### [Recheck after pin repair without clearing semantic reassessment](/tests/contracts-reconcile-pin-recheck) {#case-contracts-reconcile-pin-recheck}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `typed-route` | test_implementation → `route` | Asserts precise repair kinds and exact repin target arguments. |
| `recheck` | test_implementation → `repair` | Asserts restored currency while semantic reassessment remains. |

### [Keep complete review coverage separate from living documentation](/tests/contracts-reconcile-review-isolation) {#case-contracts-reconcile-review-isolation}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `review-isolation` | test_implementation → `review` | Exact review/documentation/currency isolation assertions. |
| `report-boundary` | implementation_under_test → `review` | Builds exact comparison and declares separate currency/semantic limits. |

### [Resolve living currency at the explicitly selected head](/tests/contracts-reconcile-selected-head) {#case-contracts-reconcile-selected-head}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `selected-head` | test_implementation → `head` | Selects base despite newer checkout and asserts no stale currency. |
| `report-boundary` | implementation_under_test → `head` | Builds exact comparison and declares separate currency/semantic limits. |

### [Repair a transaction-managed slide from its complete snapshot](/tests/contracts-reconcile-slide-snapshot) {#case-contracts-reconcile-slide-snapshot}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `snapshot-route` | test_implementation → `inspect` | Asserts typed routes and exact snapshot exposure. |
| `snapshot-repair` | test_implementation → `update` | Uses expected snapshot and verifies currency after repair. |
| `managed-route` | implementation_under_test → `inspect` | Managed slides require all heads, Items, references and criterion links. |

### [Return only requested coverage areas in caller order](/tests/contracts-selected-coverage-areas) {#case-contracts-selected-coverage-areas}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `ordered-json` | test_implementation → `select` | Asserts exact two-area order and change scope. |
| `filtered-text` | test_implementation → `text` | Asserts selected story gap and omitted unrelated prose. |
| `requested-loop` | implementation_under_test → `select`, `all-names` | Appends only parsed names in order. |

### [Compare app, Git and feature scopes explicitly](/tests/contracts-selected-coverage-scope) {#case-contracts-selected-coverage-scope}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `app-evaluator` | test_implementation → `app` | App evaluator counts code targets without inventing a diff; does not prove CLI default selection. |
| `change-cli` | test_implementation → `change` | CLI assertion confirms change scope with --against. |

### [Keep Features and Roadmap grouping independent](/tests/core-feature-grouping) {#case-core-feature-grouping}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-independent-preferences-assertions` | test_implementation → `check-independent-preferences` | Distinct selections remain selected after feature navigation. |
| `check-membership-without-copying-assertions` | test_implementation → `check-membership-without-copying` | The multi-label feature link count is two. |
| `check-unassigned-visible-assertions` | test_implementation → `check-unassigned-visible` | The browser asserts each unassigned-group heading. |
| `check-find-shared-names-assertions` | test_implementation → `check-find-shared-names` | Renamed titles appear in feature-plan and navigation assertions. |

### [Promote a planned feature without recreating its knowledge](/tests/core-feature-lifecycle) {#case-core-feature-lifecycle}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-planning-context-assertions` | test_implementation → `check-planning-context` | The planned reader displays Essay/subtext/milestone and navigates to its story. |
| `check-separate-views-assertions` | test_implementation → `check-separate-views` | Both pre-promotion and post-promotion collection placement are asserted. |
| `check-stable-identity-assertions` | test_implementation → `check-stable-identity` | Identity bytes and retained knowledge are asserted after promotion. |

### [Order ranked and unranked planned features](/tests/core-feature-priority) {#case-core-feature-priority}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-priority-order-assertions` | test_implementation → `check-priority-order` | The roadmap row identities are asserted in ranked then unranked order. |

### [Reject duplicate labels and invalid memberships](/tests/core-label-invalid-writes) {#case-core-label-invalid-writes}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-duplicate-name-assertions` | test_implementation → `check-duplicate-name` | A differently capitalized duplicate title is rejected. |
| `check-valid-references-assertions` | test_implementation → `check-valid-references` | Missing and foreign labels are rejected with unchanged plan bytes. |

### [Replace a nonempty label set](/tests/core-label-replacement) {#case-core-label-replacement}

Requirements-only definition; no implementation evidence is claimed.

### [Reject invalid milestone assignments](/tests/core-milestone-invalid-references) {#case-core-milestone-invalid-references}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-valid-references-assertions` | test_implementation → `check-valid-references` | Missing milestone occurs in the rejected-write matrix and every attempt preserves plan bytes. |

### [Share labels without copying names or losing memberships](/tests/core-shared-labels) {#case-core-shared-labels}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `shared-definition-1` | test_implementation → `check-shared-definition` | CLI asserts shared references; browser fixture exercises multi-label assignment. |
| `shared-definition-2` | test_implementation → `check-shared-definition` | CLI asserts shared references; browser fixture exercises multi-label assignment. |
| `check-rename-once-assertions` | test_implementation → `check-rename-once` | The plan stays unchanged and current names appear in the reader/navigation. |
| `check-preserve-or-clear-assertions` | test_implementation → `check-preserve-or-clear` | These exact assertions cover omission preservation and explicit clearing. |

### [Keep milestone identity through sharing, renaming and clearing](/tests/core-shared-milestones) {#case-core-shared-milestones}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-optional-shared-target-assertions` | test_implementation → `check-optional-shared-target` | Both feature creations use the shared milestone; the plan excludes the display name. |
| `check-rename-once-assertions` | test_implementation → `check-rename-once` | Rename leaves saved plan bytes unchanged and both features resolve the new title. |
| `check-preserve-or-clear-assertions` | test_implementation → `check-preserve-or-clear` | Omitting classifications preserves references; explicit clear removes them. |

### [Publish a complete new entity directory or none](/tests/contracts-atomic-entity-directory) {#case-contracts-atomic-entity-directory}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `directory-failure` | test_implementation → `failure` | Asserts absent final and staging state after injected failure. |
| `directory-success` | test_implementation → `success` | Asserts complete destination is present after publication. |

### [Leave the previous record intact after interrupted file publication](/tests/contracts-atomic-record-write) {#case-contracts-atomic-record-write}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `write-fault` | test_implementation → `fault` | Fault injection plus original-byte and cleanup assertions. |
| `staged-file` | implementation_under_test → `fault` | Writes/syncs staging bytes before atomic link or rename. |

### [Read complete story revisions and lifecycle ancestry](/tests/contracts-history-complete-snapshots) {#case-contracts-history-complete-snapshots}

Requirements-only definition; no implementation evidence is claimed.

### [Preserve independent revision and lifecycle records after a merge](/tests/contracts-parallel-partitioned-conflicts) {#case-contracts-parallel-partitioned-conflicts}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `revision-conflict` | test_implementation → `definitions` | Creates distinct branch record and asserts two heads/no current revision. |
| `lifecycle-conflict` | test_implementation → `lifecycle` | Creates distinct lifecycle record and asserts explicit conflict/no winner. |
| `exclusive-revisions` | implementation_under_test → `records` | Writes a new exclusive revision path; implementation context, not a byte-preservation assertion. |

### [Reject incomplete parents before reconciling competing story definitions](/tests/contracts-reconcile-all-story-heads) {#case-contracts-reconcile-all-story-heads}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `all-head-success` | test_implementation → `merge` | Asserts successful all-head reconciliation to current r3 only; no omitted-parent or history query assertion. |
| `parent-set-guard` | implementation_under_test → `omit` | Rejects a differing parent set before write and reports expected heads. |

### [Serialize supported writers and return busy at timeout](/tests/contracts-serialized-lock-writers) {#case-contracts-serialized-lock-writers}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `timeout` | test_implementation → `timeout` | Asserts timeout duration and release completion, but only checks timed out substring. |
| `serialization` | test_implementation → `serialize` | Counts concurrent callbacks and asserts maximum one. |
| `locked-mutation` | implementation_under_test → `public` | Reloads and executes requirement mutation inside Saga lock. |
| `busy-error` | implementation_under_test → `timeout` | Returns busy after configured lock deadline. |

### [Reject a stale story parent and identify the expected heads](/tests/contracts-stale-story-parent) {#case-contracts-stale-story-parent}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `stale-wrapper` | test_implementation → `reject` | Asserts stale criterion-wrapper rejection, not full story writer no-publication/expected-head behavior. |
| `story-guard` | implementation_under_test → `full-revision` | Full story writer guards the exact set before exclusive publication. |

### [Reassess changed criterion prose](/tests/core-criterion-prose-currency) {#case-core-criterion-prose-currency}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `reassess-1` | test_implementation → `check-reassess` | The pin evaluator is tested separately for each authored field. |

### [Preserve omitted criterion prose](/tests/core-criterion-prose-preservation) {#case-core-criterion-prose-preservation}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `preservation-1` | test_implementation → `check-preservation` | The test checks inherited prose and sibling preservation. |

### [Read complete criterion intent with safe authored context](/tests/core-criterion-reading) {#case-core-criterion-reading}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `full-statement-1` | test_implementation → `check-full-statement` | The template test checks title and complete statement on both routes. |
| `safe-prose-1` | test_implementation → `check-safe-prose` | Rendered HTML has positive formatting and negative unsafe-content assertions. |

### [Keep requirement, prototype and quality histories independent](/tests/core-domain-isolation) {#case-core-domain-isolation}

Requirements-only definition; no implementation evidence is claimed.

### [Refuse composition with an invalid domain](/tests/core-invalid-domain-composition) {#case-core-invalid-domain-composition}

Requirements-only definition; no implementation evidence is claimed.

### [Require an exact allowlisted external embed origin](/tests/core-prototype-external-origins) {#case-core-prototype-external-origins}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-trusted-external-origin-assertions` | test_implementation → `check-trusted-external-origin` | Assertions cover the specified independent fixture and its exact resolution or rejection conditions. |

### [Retain immutable prototype source and detect package tampering](/tests/core-prototype-package) {#case-core-prototype-package}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-immutable-revision-assertions` | test_implementation → `check-immutable-revision` | Assertions cover the specified independent fixture and its exact resolution or rejection conditions. |

### [Expose both stale annotation pins](/tests/core-prototype-stale-pins) {#case-core-prototype-stale-pins}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-visible-currency-assertions` | test_implementation → `check-visible-currency` | Assertions cover the specified independent fixture and its exact resolution or rejection conditions. |

### [Resolve an annotation authored before its endpoints](/tests/core-prototype-unresolved-pins) {#case-core-prototype-unresolved-pins}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-pinned-annotation-assertions` | test_implementation → `check-pinned-annotation` | Assertions cover the specified independent fixture and its exact resolution or rejection conditions. |

### [Identify the owner of feature-filtered requirements](/tests/core-requirement-ownership) {#case-core-requirement-ownership}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `feature-in-query-1` | test_implementation → `check-feature-in-query` | The filtered result asserts both identity and ownership. |

### [Keep unfinished intent proposed until a criterion exists](/tests/core-story-acceptance) {#case-core-story-acceptance}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `draft-without-criteria-1` | test_implementation → `check-draft-without-criteria` | The test successfully authors the empty proposed story. |

### [Refuse acceptance without criteria](/tests/core-story-acceptance-rejection) {#case-core-story-acceptance-rejection}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `acceptance-requires-criterion-1` | test_implementation → `check-acceptance-requires-criterion` | The assertion checks the error and absent event file. |

### [Keep terms tied to definitions, intent and source](/tests/core-term-context) {#case-core-term-context}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-term-context-assertions` | test_implementation → `check-term-context` | Term fields, semantic context, reverse references and reader code links have assertions. |
| `check-code-currency-assertions` | test_implementation → `check-code-currency` | Changed code is stale under the term owner; semantic axes persist and a corrected term revision restores currency. |

### [Keep terminology growth optional](/tests/core-term-optional-growth) {#case-core-term-optional-growth}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-optional-suggestions-assertions` | test_implementation → `check-optional-suggestions` | The suggestion is optional growth, absent from required actions, and disappears after documentation. |

### [Identify the exact term revision affected by source changes](/tests/core-term-revision-health) {#case-core-term-revision-health}

Requirements-only definition; no implementation evidence is claimed.

### [Require explicit reconciliation of every definition head](/tests/core-case-conflicting-heads) {#case-core-case-conflicting-heads}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-definition-head-guards-assertions` | test_implementation → `check-definition-head-guards` | Exact parent guards, conflict projection and successful explicit reconciliation are asserted. |

### [Enrich a case without rewriting its definition](/tests/core-case-immutable-enrichment) {#case-core-case-immutable-enrichment}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-immutable-enrichment-assertions` | test_implementation → `check-immutable-enrichment` | Evidence enrichment preserves definition bytes and old records; changing the revision stales evidence rather than advancing its pin. |

### [Refuse reuse of a removed step ID](/tests/core-case-removed-step-id) {#case-core-case-removed-step-id}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-step-id-reuse-assertions` | test_implementation → `check-step-id-reuse` | Removed-step reuse fails and the file-path snapshot is unchanged. |

### [Keep verifies links pinned when criterion content changes](/tests/core-criterion-verifies-currency) {#case-core-criterion-verifies-currency}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-criterion-pin-assertions` | test_implementation → `check-criterion-pin` | An actual criterion rewording produces stale revision pins and the criterion-changed reason. |

### [Assess verification definitions without requiring execution](/tests/core-definition-coverage) {#case-core-definition-coverage}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-required-kinds-assertions` | test_implementation → `check-required-kinds` | The status asserts covered positive default and independent missing-positive/covered-negative/edge policy kinds. |
| `check-planned-without-runs-assertions` | test_implementation → `check-planned-without-runs` | Full status equality after run removal and coverage for the proposed case without evidence are asserted. |

### [Expose missing stale and excluded definition states](/tests/core-definition-known-uncertainty) {#case-core-definition-known-uncertainty}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-known-reasons-assertions` | test_implementation → `check-known-reasons` | Assertions cover missing, stale evidence, exclusions and exact stale relation pins. |

### [Keep unsupported definition states explicit](/tests/core-definition-remaining-states) {#case-core-definition-remaining-states}

Requirements-only definition; no implementation evidence is claimed.

### [Require criterion-specific assertion mappings](/tests/core-definition-step-mapping) {#case-core-definition-step-mapping}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-specific-mapping-assertions` | test_implementation → `check-specific-mapping` | Criterion-specific mappings, setup without evidence and explained missing mapping are asserted. |

### [Retain evidence revision pins and enrichment history](/tests/core-evidence-enrichment-history) {#case-core-evidence-enrichment-history}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-evidence-provenance-assertions` | test_implementation → `check-evidence-provenance` | Assertions establish r1 pin, stages, record counts and immutable definition bytes; exact other provenance fields have a separate manual check. |

### [Retain immutable legacy run metadata without reviving execution gates](/tests/core-legacy-history-compatibility) {#case-core-legacy-history-compatibility}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-run-provenance-assertions` | test_implementation → `check-run-provenance` | The public history assertion checks presence and result literal only. |

### [Guard legacy run appends and preserve history](/tests/core-legacy-run-head-guards) {#case-core-legacy-run-head-guards}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-guarded-run-assertions` | test_implementation → `check-guarded-run` | Rejected missing-parent/overwrite operations, reuse of ci2 after rejection, conflict projection, four-record reconciliation and unchanged ci1 content are asserted. |

### [Reject missing inputs to legacy run history](/tests/core-legacy-run-missing-inputs) {#case-core-legacy-run-missing-inputs}

Requirements-only definition; no implementation evidence is claimed.

### [Enrich the same planned case with step implementation evidence](/tests/core-planned-case-enrichment) {#case-core-planned-case-enrichment}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-planned-definition-assertions` | test_implementation → `check-planned-definition` | No-evidence/no-run planned definitions and Roadmap reader navigation are asserted. |
| `check-step-traceability-assertions` | test_implementation → `check-step-traceability` | The step evidence canonical ID, visible evidence record and link-free setup are asserted. |
| `check-built-case-reader-assertions` | test_implementation → `check-built-case-reader` | The built feature opens its named case and shows implementation-linked stage/evidence. |

### [Exclude test definitions outside allowed automation](/tests/core-policy-automation-restriction) {#case-core-policy-automation-restriction}

Requirements-only definition; no implementation evidence is claimed.

### [Keep stale policy obligations until deliberate supersession](/tests/core-policy-currency) {#case-core-policy-currency}

Requirements-only definition; no implementation evidence is claimed.

### [Keep historical executions separate from definitions](/tests/core-quality-boundary) {#case-core-quality-boundary}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-run-removal-assertions` | test_implementation → `check-run-removal` | The test compares the whole assembled status before/after removing runs. |
| `check-conflicting-run-coverage-assertions` | test_implementation → `check-conflicting-run-coverage` | The comparison covers Quality.Criteria, not every status/readiness surface. |

### [Expose changed definitions and keep commands inert](/tests/core-quality-definition-staleness) {#case-core-quality-definition-staleness}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-inert-command-assertions` | test_implementation → `check-inert-command` | Definition stage, historical result and absence of command execution marker are asserted. |
| `check-definition-staleness-assertions` | test_implementation → `check-definition-staleness` | The query and reader expose stale evidence and mapping after a definition revision. |

### [Keep legacy runs in explicit history](/tests/core-quality-history-separation) {#case-core-quality-history-separation}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-history-boundary-assertions` | test_implementation → `check-history-boundary` | Query separation and omission of historical run text from the reader are asserted. |

### [Apply criterion kinds and automation policy without executions](/tests/core-quality-policies) {#case-core-quality-policies}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-default-policy-assertions` | test_implementation → `check-default-policy` | The fixture asserts positive default coverage; it does not enumerate every automation mode. |
| `check-required-kinds-assertions` | test_implementation → `check-required-kinds` | Policy normalization/supersession and independent reporting of required kinds have assertions. |

### [Keep all definition surfaces independent of legacy execution](/tests/core-quality-run-outcome-matrix) {#case-core-quality-run-outcome-matrix}

Requirements-only definition; no implementation evidence is claimed.

### [Expose stale code without a fresh run](/tests/core-quality-source-staleness) {#case-core-quality-source-staleness}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `check-code-staleness-assertions` | test_implementation → `check-code-staleness` | The test removes runs and code resolution, then asserts stale status and definition stage. |

### [Initialize portable source identity without transport credentials](/tests/review-canonical-repository) {#case-review-canonical-repository}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `canonical` | Direct canonicalization assertion does not assert persisted init identity. |
| `evidence-2` | implementation_under_test → `canonical`, `record` | Init places discovered canonical repository in the manifest. |

### [Record the exact commit documented by a companion Saga](/tests/review-companion-cursor) {#case-review-companion-cursor}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `sync` | Asserts exact documented commit in sync cursor. |
| `evidence-2` | test_implementation → `advance` | Asserts subsequent cursor equals source head. |
| `evidence-3` | implementation_under_test → `sync`, `advance` | Resolves requested source revision to full commit and writes cursor. |

### [Read source changes from the intended checkout while Saga lives in a separate repository](/tests/review-companion-source-delta) {#case-review-companion-source-delta}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `compare` | Separate source/docs fixture asserts companion opening and that the code delta/reason comes from source. |
| `evidence-2` | implementation_under_test → `compare` | Explicit source checkout supplies Git range and resolver. |

### [Refuse a different source repository unless override is explicit](/tests/review-repository-mismatch) {#case-review-repository-mismatch}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `refuse`, `override` | Asserts mismatch error and override producing atoms. |
| `evidence-2` | implementation_under_test → `refuse`, `override` | Canonical identity checked unless explicitly overridden. |

### [Resolve the same Item evidence from companion and in-repository layouts](/tests/review-source-relocation) {#case-review-source-relocation}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `companion`, `inside` | Executes explicit source query, relocation and implicit source query, asserting equal current app.go evidence. |
| `evidence-2` | implementation_under_test → `companion`, `inside` | Uses explicit SourceDir or Saga root for Git range and source resolver. |

### [Keep a committed human decision attributed to its author](/tests/review-decision-author) {#case-review-decision-author}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `other` | Tests separate reviewer seats and that the remaining decision Author contains Other; no browser author assertion. |
| `evidence-2` | implementation_under_test → `other` | Resolves committed author or local Git user. |
| `evidence-3` | implementation_under_test → `visible` | Human decision renderer returns decision.Author. |

### [Name the exact changed code that invalidates a decision](/tests/review-decision-code-stale) {#case-review-decision-code-stale}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `change-code` | Asserts code-based out-of-date reason names the precise path and lines. |

### [Keep decisions current when their slide and referenced code are unchanged](/tests/review-decision-current) {#case-review-decision-current}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `current` | Asserts unchanged queue approval is current. |
| `evidence-2` | test_implementation → `isolation` | Asserts unchanged table decision remains current after unrelated referenced code changes. |
| `evidence-3` | implementation_under_test → `current`, `isolation` | Compares slide digest, readable head/commit and referenced code. |

### [Replace an approval with changes requested without losing the approval](/tests/review-decision-replacement-history) {#case-review-decision-replacement-history}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `retention-fixture` | Append-only test asserts seven records and original approval identity/state after subsequent decisions and withdrawals; it does not directly assert two-action replacement history in the UI. |
| `evidence-2` | implementation_under_test → `replace`, `history` | Writer appends events; report selects latest seat decisions. |

### [Name a changed slide that invalidates a decision](/tests/review-decision-slide-stale) {#case-review-decision-slide-stale}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `change-slide` | Asserts slide digest change yields named out-of-date reason. |
| `evidence-2` | implementation_under_test → `change-slide` | Adds a reason when slide digest differs. |

### [Record approval and a request for changes from their slide controls](/tests/review-decision-ui) {#case-review-decision-ui}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `approve`, `changes` | Browser clicks both controls and asserts their respective persisted UI states. |
| `evidence-2` | implementation_under_test → `approve`, `changes` | Appends a record with state, reviewer, commit and slide digest. |

### [Report unknown currency when the decision commit or head is unreadable](/tests/review-decision-unreadable) {#case-review-decision-unreadable}

Requirements-only definition; no implementation evidence is claimed.

### [Withdraw the current seat decision and preserve its history](/tests/review-decision-withdrawal) {#case-review-decision-withdrawal}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `approve`, `withdraw` | Asserts approval, withdrawal UI state and preserved ordered state records. |
| `evidence-2` | implementation_under_test → `withdraw` | Latest none suppresses current decision; earlier events remain in store. |

### [Display both an AI reviewer and its human principal](/tests/review-delegated-provenance) {#case-review-delegated-provenance}

Requirements-only definition; no implementation evidence is claimed.

### [Show the human actor on a human-authored action](/tests/review-human-provenance) {#case-review-human-provenance}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `report` | Report assertion checks the surviving decision author contains Other. |
| `evidence-2` | implementation_under_test → `display` | Human label returns the Git-derived author. |

### [Reopen a resolved thread without losing ordered discussion](/tests/review-thread-reopen-history) {#case-review-thread-reopen-history}

Requirements-only definition; no implementation evidence is claimed.

### [Reply to an Item thread and resolve it](/tests/review-thread-reply-resolve) {#case-review-thread-reply-resolve}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `root`, `reply` | Asserts counts before/after resolve; does not directly assert reply target or reopen. |
| `evidence-2` | implementation_under_test → `context` | Reply inherits target of its parent comment. |

### [Read prerequisite direction between planned features](/tests/core-roadmap-prerequisite) {#case-core-roadmap-prerequisite}

Requirements-only definition; no implementation evidence is claimed.

### [Discover all inventory kinds and preserve their selected revision context](/tests/inventory-all-kind-context-matrix) {#case-inventory-all-kind-context-matrix}

Requirements-only definition; no implementation evidence is claimed.

### [Do not infer newness or hide conflicting definitions](/tests/inventory-baseline-boundaries) {#case-inventory-baseline-boundaries}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `conflicts` | Asserts resolvable identities, competing heads, unspecified legacy intent and unresolved retention under filtering. |
| `evidence-2` | test_implementation → `unknown-base` | Asserts unknown-baseline and missing-baseline boundaries. |
| `evidence-3` | test_implementation → `scope-path` | Asserts exact scoped identities and declared path; fixture is System/Component, not a full proposed ERD diff. |

### [Measure bounded inventory coverage with exact owners](/tests/inventory-bounded-coverage) {#case-inventory-bounded-coverage}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `counts`, `owners` | Asserts scoped counts, deduplication, overlap owner and health; does not assert every uncovered source locator or CLI head/scope serialization. |
| `evidence-2` | implementation_under_test → `public` | Serializes head, scope, range entries and explicit measurement limits; implementation evidence only. |

### [Create and reference a proposal before implementation](/tests/inventory-code-free-proposal) {#case-inventory-code-free-proposal}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `create` | Asserts code-free Component publication and queried explicit intent; System/data-entity reference matrix remains manual. |

### [Read canonical definitions, exact code and stale link health](/tests/inventory-definition-query) {#case-inventory-definition-query}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `code` | Asserts evidence commit/digest for legacy Component/System fixture; does not assert all description/health fields or data-entity discovery. |
| `evidence-2` | test_implementation → `link` | Asserts stale member link and refuses a new stale-member System. |
| `evidence-3` | test_implementation → `drift` | Asserts stale code-health resolution after actual selected-byte change. |
| `evidence-4` | implementation_under_test → `code`, `link`, `drift` | Returns exact code resolutions and declared link health, including relationships/holders. |

### [Follow an ERD entity to its feature behavior and code](/tests/inventory-entity-progressive-detail) {#case-inventory-entity-progressive-detail}

Requirements-only definition; no implementation evidence is claimed.

### [Review the ERD as an authored architectural explanation](/tests/inventory-erd-authored-composition) {#case-inventory-erd-authored-composition}

Requirements-only definition; no implementation evidence is claimed.

### [Open the same canonical entity from drawing and directory](/tests/inventory-erd-canonical-navigation) {#case-inventory-erd-canonical-navigation}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `directory` | Checks visible entity directory links. |
| `evidence-2` | test_implementation → `same-pin` | Checks identical report pin from drawing/directory, focus restoration and undrawn proposal visibility. |

### [Compare entity and relationship states across all reading surfaces](/tests/inventory-erd-cross-surface-status) {#case-inventory-erd-cross-surface-status}

Requirements-only definition; no implementation evidence is claimed.

### [Read logical payloads, persisted records and declared relationships](/tests/inventory-erd-entity-meaning) {#case-inventory-erd-entity-meaning}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `record` | Assertions check report structure, authored-fields disclaimer, holder pin, description and relationship IDs. |
| `evidence-2` | test_implementation → `payload` | Asserts code-free proposal and pinned drawer holder text; JSON description is fixture content, not an explicit assertion. |
| `evidence-4` | test_implementation → `relations` | Asserts production meaning and direction text. |
| `evidence-5` | test_implementation → `relations` | Asserts association, owner/destination cardinality, unknown disclaimer and pinned directory link; link traversal itself remains manual. |
| `evidence-6` | implementation_under_test → `record`, `payload`, `relations` | Projects declared meaning, pins, cardinality and flow. |
| `evidence-7` | implementation_under_test → `record`, `payload` | Projects only authored fields and explicit holder pins. |

### [Open the application ERD and compare a proposed overlay](/tests/inventory-erd-entry-overlay) {#case-inventory-erd-entry-overlay}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `open` | Asserts Technical design to Data model entry and its directory. |
| `evidence-2` | test_implementation → `overlay` | Asserts overlay additions, exact baseline link and unchanged baseline; does not assert originating feature/design navigation. |
| `evidence-3` | implementation_under_test → `overlay` | Builds the selected ERD view from its exact revision. |

### [Retrieve design-specific ERD changes and their implementation context](/tests/inventory-erd-query-handoff) {#case-inventory-erd-query-handoff}

Requirements-only definition; no implementation evidence is claimed.

### [Record implementation while preserving the proposed definition](/tests/inventory-implementation-history) {#case-inventory-implementation-history}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `implement` | Asserts delivery/evidence pins, replay and preserved proposed r1; no verification/approval state assertion. |
| `evidence-2` | test_implementation → `successor` | Asserts same-identity successor with implemented baseline only. |
| `evidence-3` | test_implementation → `pin` | Asserts refusal of stale pin without usable saved view and exact current proposal pin. |
| `evidence-4` | implementation_under_test → `implement` | Builds evidence/endpoint policy candidate for explicit implementation. |

### [Keep stale selections visible while excluding them from coverage](/tests/inventory-inheritance-health) {#case-inventory-inheritance-health}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `selected-drift` | Asserts selected-byte drift has zero inherited/current coverage and separate selection health. |
| `evidence-2` | test_implementation → `stale-pin` | Asserts noncurrent pin is unattached with explicit reason. Overlapping inherited selections require the manual extension; inventory-wide overlap tests do not prove it. |

### [Expose inherited ownership and unresolved selection paths](/tests/inventory-inheritance-provenance) {#case-inventory-inheritance-provenance}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `paths`, `owners` | Asserts attached/unresolved states, exact count, Item/selection/path provenance and no duplicate code records; full displayed source location still requires UI inspection. |
| `evidence-2` | implementation_under_test → `owners` | Copies exact selected code and the full declared path to inherited provenance. |

### [Credit only selected code through declared inventory paths](/tests/inventory-inherited-subset) {#case-inventory-inherited-subset}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `selection` | Asserts two attached eligible selections; fixture exact paths/subsets at lines 61-64. |
| `evidence-2` | test_implementation → `coverage` | Asserts selected/unselected line ownership, not entire containing evidence. |
| `evidence-3` | implementation_under_test → `selection`, `coverage` | Attaches eligible selections with Item, path, evidence and exact code; no document writes. |

### [Implementation and coverage do not grant approval](/tests/inventory-no-approval-side-effects) {#case-inventory-no-approval-side-effects}

Requirements-only definition; no implementation evidence is claimed.

### [Query explicit proposals independently from comparison newness](/tests/inventory-public-roadmap) {#case-inventory-public-roadmap}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `intent` | Asserts positive/negative intent filtering for Component and data entity; no proposed System is present. |
| `evidence-2` | test_implementation → `new` | Asserts comparison-relative identity counts, including implemented identities in this fixture. |
| `evidence-3` | test_implementation → `scope` | Asserts feature-reachable identities and relationship/overlay reverse roles; does not assert design-specific filter or relationship change diff. |
| `evidence-4` | implementation_under_test → `intent`, `new`, `scope` | Keeps selected intent and comparison newness separate; unresolved rows remain outside filters. |

### [Follow declared reverse usages to their implementation Items](/tests/inventory-query-usages) {#case-inventory-query-usages}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `direct`, `transitive`, `absence` | Asserts direct/transitive paging, role, feature/path/slide and confirmed unused existing record; exact deck and Item field values are not all asserted. |
| `evidence-2` | implementation_under_test → `transitive` | Constructs feature/deck/slide/Item usage context and exact navigation links. |

### [Reject unsupported implementation assertions](/tests/inventory-reject-bad-delivery) {#case-inventory-reject-bad-delivery}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `missing-delivery` | Asserts delivery and current-parent admission failures. |
| `evidence-2` | test_implementation → `stale-digest` | Asserts stale delivery refusal, unchanged file list and forged digest rejection; missing/unresolvable evidence variants are separate manual checks. |

### [Carry canonical inventory pins and code selections into review](/tests/inventory-review-saved-view) {#case-inventory-review-saved-view}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `select` | Asserts exact review selection. |
| `evidence-2` | test_implementation → `preserve` | Asserts saved pin/selection persistence after successor; does not inspect a changed-selection history. |
| `evidence-3` | test_implementation → `historical` | Asserts exact committed historical view admission and refusals; fixture starts implemented, not proposed. |
| `evidence-4` | implementation_under_test → `select` | Shows declared selection path and saved code without substitution. |

### [Inspect selected code and return to the originating Item](/tests/inventory-selected-code-roundtrip) {#case-inventory-selected-code-roundtrip}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `selected`, `return` | Asserts selected line, path, integrity, complete-definition separation and round-trip context; does not assert complete source-location caption or failure variants. |
| `evidence-2` | implementation_under_test → `selected` | Builds declared hops, resolution problems and saved selected bytes. |

### [Reuse one canonical definition from separate design contexts](/tests/inventory-shared-contexts) {#case-inventory-shared-contexts}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `shared` | Exercises two slides in ONE deck, not two independent decks; checks canonical identity/member/code and return focus. |
| `evidence-2` | test_implementation → `saved` | Asserts stale saved definition and exact r1 pin, not full code-health matrix. |
| `evidence-3` | implementation_under_test → `shared`, `saved` | Reads explicit pin, intent and immutable revision history. |

### [Exclude stale and conflicting evidence from current coverage](/tests/inventory-stale-coverage) {#case-inventory-stale-coverage}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `initial` | Asserts counts, no owners for uncovered ranges, and conflicted reader excluded. |
| `evidence-2` | test_implementation → `edit` | Asserts stale references contribute no current coverage. |

### [Keep a proposed successor discoverable after delivery](/tests/inventory-successor-roadmap) {#case-inventory-successor-roadmap}

Requirements-only definition; no implementation evidence is claimed.

### [Read the approach essay and reach its supporting whiteboards](/tests/inventory-technical-approach) {#case-inventory-technical-approach}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `approach` | Asserts authored essay and three board links. |
| `evidence-2` | test_implementation → `support` | Asserts supporting-page navigation and data-flows identity. |
| `evidence-3` | implementation_under_test → `approach` | Projects authored approach/supporting fragments into technical pages. |

### [Preserve one, many and optional association endpoints](/tests/inventory-technical-cardinality) {#case-inventory-technical-cardinality}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `render`, `focus` | Asserts composed SVG markers and retained semantic endpoints/multiplicity/source; pixel meaning of glyphs is supported by renderer fixture separately. |
| `evidence-2` | test_implementation → `unknown` | Asserts unknown endpoint text and disclaimer in the ERD. |
| `evidence-3` | implementation_under_test → `unknown` | Renders one/optional/many and question-mark unknown glyph from declared cardinality. |

### [Follow directed labeled application flows](/tests/inventory-technical-data-movement) {#case-inventory-technical-data-movement}

Requirements-only definition; no implementation evidence is claimed.

### [Read the actual local runtime infrastructure](/tests/inventory-technical-runtime-infrastructure) {#case-inventory-technical-runtime-infrastructure}

Requirements-only definition; no implementation evidence is claimed.

### [Ask for reconciliation of existing unreferenced entities](/tests/inventory-unreferenced-choice) {#case-inventory-unreferenced-choice}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `new-exemption`, `existing-choice`, `unknown` | Asserts comparison-based exemption, explicit user-choice guidance, unknown baseline and no mutation; does not assert proposal exemption or complete actionable context fields. |
| `evidence-2` | implementation_under_test → `existing-choice`, `context` | Complete zero-use records branch on explicit proposal/newness and otherwise request user choice. |

### [Navigate entity documentation to the referencing slide and Item](/tests/inventory-usage-navigation) {#case-inventory-usage-navigation}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `definition`, `navigate` | Asserts canonical page, explanation, usage, exact slide, Item pin and round-trip focus. |
| `evidence-2` | test_implementation → `currency` | Asserts saved pin and usage status; code-reference currency itself needs an additional health check. |
| `evidence-3` | implementation_under_test → `navigate` | Links usages to originating feature/deck/slide/Item. |

### [Reopen an exact historical board after newer publication](/tests/inventory-whiteboard-historical-pins) {#case-inventory-whiteboard-historical-pins}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `child` | Asserts child revisions do not implicitly alter saved board. |
| `evidence-2` | test_implementation → `history` | Asserts historical SVG equality and no mutation of source ERD history. |
| `evidence-3` | implementation_under_test → `history` | Selects an explicit existing revision and refuses missing revisions. |

### [Publish named whiteboards without changing their siblings](/tests/inventory-whiteboard-independent-history) {#case-inventory-whiteboard-independent-history}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `publish` | Asserts independent current revisions after one named publication. |
| `evidence-2` | test_implementation → `sibling` | Asserts original sibling bytes unchanged. |
| `evidence-3` | implementation_under_test → `publish` | Publishes only the selected board revision path. |

### [Focus exact instances within a named whiteboard](/tests/inventory-whiteboard-instance-focus) {#case-inventory-whiteboard-instance-focus}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `instances` | Asserts distinct repeated-instance focus, history and reload. |
| `evidence-2` | test_implementation → `section` | Asserts named/historical board selection, section rendering and revision-preserving sidebar focus; does not directly assert camera position for a native section. |

### [Reject publication based on an outdated board parent](/tests/inventory-whiteboard-stale-parent) {#case-inventory-whiteboard-stale-parent}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `reject` | Asserts stale and sibling-parent publication rejection. |
| `evidence-2` | implementation_under_test → `reject` | Checks the observed head under Saga lock before publication. |

### [Follow design links to requirements and delivered evidence](/tests/contracts-design-paths) {#case-contracts-design-paths}

Requirements-only definition; no implementation evidence is claimed.

### [Enumerate every declared current design and implementation path](/tests/contracts-intent-alternate-paths) {#case-contracts-intent-alternate-paths}

Requirements-only definition; no implementation evidence is claimed.

### [Preserve the explicit story hop for broad design](/tests/contracts-intent-broad-design-path) {#case-contracts-intent-broad-design-path}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `broad-design` | test_implementation → `broad` | Asserts broad design identities and the explicit criterion→story→design path. |
| `path-builders` | implementation_under_test → `broad` | Builds paths for declared current relations and integrated evidence. |

### [Expose integrated evidence on a declared implementation path](/tests/contracts-intent-delivery-path) {#case-contracts-intent-delivery-path}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `delivery-evidence` | test_implementation → `delivered` | Asserts delivered and exact count/type of merge-derived evidence; does not enumerate all paths. |
| `path-builders` | implementation_under_test → `delivered` | Builds paths for declared current relations and integrated evidence. |

### [Expose stale links when criterion wording changes](/tests/contracts-intent-stale-criterion) {#case-contracts-intent-stale-criterion}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `criterion-staleness` | test_implementation → `stale` | Asserts stale criterion relation after wording change. |

### [Expose stale links when design content changes](/tests/contracts-intent-stale-design) {#case-contracts-intent-stale-design}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `digest-staleness` | test_implementation → `stale` | Exact query assertions for two stale relations and their digest-change reasons. |

### [Resolve current code locations back to persisted intent](/tests/contracts-reverse-current-location) {#case-contracts-reverse-current-location}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `remapped-match` | test_implementation → `match` | Tests matching helper only, with a supplied locator; does not assert returned public paths. |
| `location-filter` | implementation_under_test → `match` | Uses current remapped location and overlap matching. |
| `reverse-path-build` | implementation_under_test → `query` | Matches explains evidence and builds criterion-to-evidence paths before filtering. |

### [Show stale and conflicted reverse links without current claims](/tests/contracts-reverse-stale-conflict-report) {#case-contracts-reverse-stale-conflict-report}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `stale-not-current` | test_implementation → `stale-filter` | Asserts stale evidence cannot match current location; does not assert blocker visibility. |
| `stale-filter-product` | implementation_under_test → `stale-filter` | Drops stale locator results. |

### [Navigate slides within deck boundaries and keep position and anchor in sync](/tests/review-bounded-deck-navigation) {#case-review-bounded-deck-navigation}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `thumbnail` | Asserts visible selected slide and 1 / 2. |
| `evidence-2` | test_implementation → `next`, `other-deck` | Asserts Next transition and per-deck bounds; only view query parameter, not exact anchor, is asserted. |
| `evidence-3` | implementation_under_test → `next`, `previous-anchor` | Computes deck-local bounds and replaces hash with active fragment. |

### [Open the first slide from a feature Implementation section](/tests/review-implementation-opening) {#case-review-implementation-opening}

Requirements-only definition; no implementation evidence is claimed.

### [Attach exact committed source evidence to an addressable Item](/tests/review-item-exact-code-pin) {#case-review-item-exact-code-pin}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `cover` | Uses exact full commit URI and asserts Item acceptance/slide rejection; does not assert stored reference fields. |
| `evidence-2` | implementation_under_test → `cover` | SVG/HTML Items are addressable by element ID; raster uses regions. |

### [Reject broad visual intent links and accept exact Item links](/tests/review-item-only-intent) {#case-review-item-only-intent}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `reject` | Both relation kinds and broad endpoints are refused, and document has zero relations. |
| `evidence-2` | test_implementation → `accept` | Both relation kinds succeed from exact Item. |
| `evidence-3` | implementation_under_test → `reject` | Broad source endpoint rejected before relation creation. |

### [Present and exit while retaining the active slide](/tests/review-presentation) {#case-review-presentation}

Requirements-only definition; no implementation evidence is claimed.

### [Accept raster slides and sandbox interactive HTML](/tests/review-raster-html-sandbox) {#case-review-raster-html-sandbox}

Requirements-only definition; no implementation evidence is claimed.

### [Inspect intent and code independently from one Item](/tests/review-story-code-controls) {#case-review-story-code-controls}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `story`, `code` | Both controls visible; story count/rationale asserted and code drawer opens. Explicit absence of cross-drawer content is not asserted. |
| `evidence-2` | implementation_under_test → `story` | Projects story/criterion/rationale and currency separately from code evidence. |

### [Show current story and criterion in the Item story drawer](/tests/review-story-details-current) {#case-review-story-details-current}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `drawer` | Item drawer opens with two links. |
| `evidence-2` | test_implementation → `details` | Asserts story name, criterion destination and current statuses. |
| `evidence-3` | implementation_under_test → `drawer`, `details` | Builds exact criterion display and relation currency. |

### [Expose stale and invalid Item intent links](/tests/review-story-details-stale-invalid) {#case-review-story-details-stale-invalid}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `stale` | Asserts projection statuses; browser rendering of these currencies is unasserted. |
| `evidence-2` | implementation_under_test → `stale` | Missing targets feed relation evaluator, which supplies status. |

### [Preview only Items with story links on hover and keyboard focus](/tests/review-story-summary-preview) {#case-review-story-summary-preview}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `hover`, `unhover`, `focus` | Asserts correct hover target, clearing on unhover, and highlighting on focus; no assertion after blur. |
| `evidence-2` | implementation_under_test → `hover` | Only exact owner Item gets story links for affordance projection. |

### [Count distinct exact-Item stories and isolate legacy links](/tests/review-story-summary-scope) {#case-review-story-summary-scope}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `summary` | Fixture distinguishes active exact, broad, unrelated and superseded relations and asserts counts and migration label. |
| `evidence-2` | test_implementation → `legacy-only` | Broad-only links do not contribute to count. |
| `evidence-3` | implementation_under_test → `summary`, `legacy-only` | Deduplicates by story and splits exact owner versus legacy links. |

### [Accept SVG and HTML implementation slides](/tests/review-svg-html-authoring) {#case-review-svg-html-authoring}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `svg` | Adds SVG slide and Item successfully. |
| `evidence-2` | test_implementation → `html` | Adds HTML, Item, then asserts valid load and asset presence; no sandbox behavior assertion. |
| `evidence-3` | implementation_under_test → `svg`, `html` | Allows explicit visual media types and requires source for non-SVG media. |

### [Keep an annotation attached to its slide through resizing and navigation](/tests/review-annotation-anchor) {#case-review-annotation-anchor}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `resize-window` | Asserts relative scale across viewport; cross-slide exclusion/reload target remains planned. |
| `evidence-2` | implementation_under_test → `navigate` | Annotation retains its explicit comment target. |

### [Show an annotation and its author to another participant](/tests/review-annotation-participant-author) {#case-review-annotation-participant-author}

Requirements-only definition; no implementation evidence is claimed.

### [Leave original visual bytes unchanged after annotation edits](/tests/review-annotation-source-immutable) {#case-review-annotation-source-immutable}

Requirements-only definition; no implementation evidence is claimed.

### [Show the authored reason and explicitly chosen replacement](/tests/review-deprecation-reason) {#case-review-deprecation-reason}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `reason` | Asserts escaped reason text; fixture has no replacement. |
| `evidence-2` | implementation_under_test → `replacement` | Template displays reason and optional replacement with target health. |

### [Withdraw an assertion while preserving its original revision](/tests/review-deprecation-withdrawal) {#case-review-deprecation-withdrawal}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `withdraw`, `immutable` | Checks event/revision counts and byte-identical original assertion after withdrawal. |

### [Distinguish explicit deprecated code from related code and neighboring lines](/tests/review-exact-deprecation) {#case-review-exact-deprecation}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `scope`, `neighbor` | Asserts exact commit/match and role distinction, plus no adjacent-line match. |
| `evidence-2` | implementation_under_test → `scope` | Explicit scope, references and replacement are separate fields and presentation exposes their distinct roles. |

### [Draw a freehand mark on a review slide](/tests/review-freehand) {#case-review-freehand}

Requirements-only definition; no implementation evidence is claimed.

### [Attach a comment to an exact review Item](/tests/review-item-comment) {#case-review-item-comment}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `comment` | Exact stored Item target asserted after load. |
| `evidence-2` | implementation_under_test → `comment` | Resolves supplied target and records it with the comment. |

### [Place and resize a rectangular review annotation](/tests/review-rectangle-place-resize) {#case-review-rectangle-place-resize}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `draw` | Asserts rectangle draft/display and stored normalized geometry. |
| `evidence-2` | test_implementation → `resize` | Asserts both dimensions increase after resize. |
| `evidence-3` | implementation_under_test → `draw` | Rectangle anchor generated from pointer bounds and saved against slide. |

### [Attach an ordinary discussion to a whole review slide](/tests/review-slide-comment) {#case-review-slide-comment}

Requirements-only definition; no implementation evidence is claimed.

### [Place text at a selected visual location](/tests/review-sticky-text-location) {#case-review-sticky-text-location}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `evidence-1` | test_implementation → `place` | Clicks chosen coordinates and asserts displayed note text; resulting coordinates are not asserted. |
| `evidence-2` | implementation_under_test → `place`, `location` | Records note text with normalized pointer location and slide target. |

### [Retrieve only a worker’s assigned entity roadmap context](/tests/contracts-assigned-entity-query) {#case-contracts-assigned-entity-query}

Requirements-only definition; no implementation evidence is claimed.

### [Update an assigned entity roadmap without erasing proposal or assignment history](/tests/contracts-assigned-entity-reconciliation) {#case-contracts-assigned-entity-reconciliation}

Requirements-only definition; no implementation evidence is claimed.

### [Assign existing technical identities without duplicating inventory or personas](/tests/contracts-assigned-entity-scope) {#case-contracts-assigned-entity-scope}

Requirements-only definition; no implementation evidence is claimed.

### [Keep unfulfilled or conflicted contract heads blocking](/tests/contracts-contract-conflicted-head) {#case-contracts-contract-conflicted-head}

Requirements-only definition; no implementation evidence is claimed.

### [Persist a versioned provider-consumer contract definition](/tests/contracts-contract-definition) {#case-contracts-contract-definition}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `cli-create` | test_implementation → `create` | Asserts successful public creation and initial event count, not round-trip field equality. |
| `contract-fields` | implementation_under_test → `create` | Requires contract input and forwards all definition fields to domain writer. |

### [Require fulfillment of the current pinned contract revision](/tests/contracts-contract-exact-revision) {#case-contracts-contract-exact-revision}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `fulfilled-and-stale` | test_implementation → `fulfill`, `revise` | Asserts fulfillment at v1 and rejection after v2 becomes current. |
| `gate` | implementation_under_test → `fulfill`, `revise` | Requires unique matching revision head and unique fulfilled event head. |

### [Reject a dependency that closes a cycle](/tests/contracts-dependency-cycle-rejection) {#case-contracts-dependency-cycle-rejection}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `cycle-no-publication` | test_implementation → `cycle`, `unchanged` | Asserts cycle error, absent destination and surviving dependency count. |

### [Evaluate progress, merge and exact-contract dependency facts](/tests/contracts-dependency-fact-gates) {#case-contracts-dependency-fact-gates}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `progress-merge` | test_implementation → `done`, `integrate` | Exercises progress and merge fact evaluation with nonempty missing-merge reason. |
| `contract` | test_implementation → `contract` | Exercises contract fulfillment using exact revision. |
| `blocker-projection` | implementation_under_test → `public-blockers` | Public projection associates each failed dependency with its identity, reason and path; no assertion evidence for exhaustive table. |
| `condition-validation` | implementation_under_test → `reject-condition` | Validates endpoint identity and the discriminated gate condition. |

### [Keep done work undelivered until immutable evidence exists](/tests/contracts-done-not-delivery) {#case-contracts-done-not-delivery}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `no-delivery` | test_implementation → `done` | Asserts done/planned path is not delivered and names immutable evidence missing. |
| `delivery` | test_implementation → `evidence` | Asserts integrated evidence changes delivery projection. |
| `missing-evidence` | implementation_under_test → `done` | Adds explicit blocker when evidence is absent. |

### [Record required merge lifecycle with exact head and merge commit](/tests/contracts-merge-delivery-evidence) {#case-contracts-merge-delivery-evidence}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `merge-chain` | test_implementation → `record` | Exercises all three public mutations with exact OIDs; asserts mutation success/event count, not full query payload. |
| `merge-event-query` | implementation_under_test → `query` | Public work events carry full merge lifecycle payload. |

### [Preserve progress history and expose current state only for a unique head](/tests/contracts-progress-history-heads) {#case-contracts-progress-history-heads}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `progress-conflict` | test_implementation → `conflict` | Creates both branch events and asserts no current progress. |
| `progress-reconcile` | test_implementation → `reconcile` | Asserts a single new event and one remaining head. |
| `progress-projection` | implementation_under_test → `history` | Public projection includes full progress events with parents. |

### [Append and query contract states through the public plan API](/tests/contracts-public-contract-state) {#case-contracts-public-contract-state}

Requirements-only definition; no implementation evidence is claimed.

### [Read ordered waves without creating implicit wave barriers](/tests/contracts-waves-and-readiness) {#case-contracts-waves-and-readiness}

Requirements-only definition; no implementation evidence is claimed.

### [Retain portable workspace assignment and release history](/tests/contracts-workspace-ownership-events) {#case-contracts-workspace-ownership-events}

| Evidence | Role and steps | Inspected scope |
| --- | --- | --- |
| `workspace-parent` | test_implementation → `assign-release` | Asserts separate event, parent reference and absence of transient identity; does not assert all field equality/public projection. |
| `workspace-query` | implementation_under_test → `query` | Public event output retains structured WorkspaceEvent fields. |
