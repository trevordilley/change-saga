# Requirements review {#requirements-review}

This is the requirements checkpoint for the next Change Saga schema evolution. All seven stories are currently **proposed**. Approving this checkpoint will move the work to visual technical design; no design slides, implementation links, or quality evidence have been authored yet.

The IDs shown below are stable identities. Editing wording creates a new immutable story revision; it does not replace or reuse a story or acceptance-criterion ID.

## 1. Keep the whole change in one Saga {#story-single-saga-lifecycle}

**Story ID:** `single-saga-lifecycle`<br>
**Revision:** `r1`<br>
**Priority / lifecycle:** must / proposed

> As a product and engineering team, we can evolve one Change Saga from initial requirements through design, implementation, quality, and completed review so intent and evidence remain connected.

### Acceptance criteria {#criteria-single-saga-lifecycle}

1. **`one-identity`** — Requirements, report content, embedded design decks, implementation evidence, quality records, and review history derive from one Saga manifest and stable Saga ID.
2. **`separate-surfaces`** — Requirements and quality render in the report surface while visual technical design remains in focused embedded decks; neither surface is converted into the other.
3. **`history-preserved`** — Requirement, design-link, test, evidence, and review changes preserve prior history rather than rewriting or silently retargeting it.
4. **`incremental-adoption`** — The Saga can begin with requirements alone and add design, implementation evidence, and quality later without creating disconnected artifacts.

## 2. Author living stories and criteria safely {#story-requirements-authoring}

**Story ID:** `requirements-authoring`<br>
**Revision:** `r1`<br>
**Priority / lifecycle:** must / proposed

> As a product owner or AI collaborator, I can create and revise user stories and acceptance criteria through supported CLI workflows while preserving stable identity and explicit lifecycle history.

### Acceptance criteria {#criteria-requirements-authoring}

1. **`stable-identities`** — Callers choose stable story and criterion IDs; criterion URNs remain stable across wording revisions and removed criterion IDs are never reused.
2. **`immutable-revisions`** — Adding, revising, or removing one criterion creates a complete immutable story revision that names the exact current parent revision.
3. **`explicit-lifecycle`** — Story lifecycle is recorded through append-only proposed, accepted, deferred, rejected, and retired events; criteria inherit the current story lifecycle.
4. **`conflicts-visible`** — Concurrent revision or lifecycle heads remain explicit conflicts until a reconciliation record names every competing head.
5. **`human-ai-workflows`** — Human editor workflows and structured FILE-or-stdin AI workflows share the same validation, idempotency, atomic-write, and machine-readable result path.
6. **`no-partial-write`** — Invalid identity, parent, schema, lifecycle, or criterion history leaves the Saga unchanged and returns an actionable error.

## 3. Trace requirements into visual design {#story-visual-design-coverage}

**Story ID:** `visual-design-coverage`<br>
**Revision:** `r1`<br>
**Priority / lifecycle:** must / proposed

> As an architect or reviewer, I can see exactly which technical-design Decks, Slides, and Items address each accepted story or acceptance criterion.

### Acceptance criteria {#criteria-visual-design-coverage}

1. **`visual-targets`** — A current typed relation can link a report design target, Deck, Slide, or Item to a story or criterion in the same Saga.
2. **`pinned-links`** — Each design link pins the target story revision and the current canonical source-content digest so edits become stale instead of silently remaining current.
3. **`explicit-containment`** — A Deck or Slide reaches descendant Item evidence only when its relation explicitly declares descendant scope; Item links remain self-scoped.
4. **`coverage-states`** — Each accepted criterion reports direct, broad, excluded, gap, stale, invalid, or conflicted design coverage with concrete relations and diagnostics.
5. **`precision-visible`** — Story-level links remain visibly broad and criterion-level links visibly direct; full coverage and full criterion precision are reported separately.
6. **`no-score-only`** — The design view exposes targets, paths, exclusions, and stale reasons rather than reducing coverage to an opaque percentage.

## 4. Connect requirements and design to exact implementation {#story-implementation-traceability}

**Story ID:** `implementation-traceability`<br>
**Revision:** `r1`<br>
**Priority / lifecycle:** must / proposed

> As a reviewer, I can traverse from an accepted criterion through the visual design element that addresses it to the exact current implementation diff, and traverse the same graph in reverse.

### Acceptance criteria {#criteria-implementation-traceability}

1. **`item-owned-diffs`** — Exact embedded-deck diff evidence is owned only by Items and uses canonical current line or file-event diff URIs.
2. **`current-path`** — A complete path requires a current accepted criterion revision, current design relation and digest, validated containment where used, an existing Item, and exact evidence matching the active source comparison.
3. **`forward-reverse`** — Queries return criterion-to-diff paths and reverse lookup by exact diff URI or committed source-head identity.
4. **`many-to-many`** — One Item may address several criteria, one criterion may have several Items, and shared diff selectors remain explicit per path rather than being arbitrarily assigned.
5. **`ambiguity-visible`** — Multiple semantic heads, invalid selectors, missing endpoints, broad containment, and stale source identities are reported as distinct diagnostics.
6. **`correctness-limit`** — Trace completeness and global all-atoms-mapped coverage remain separate omission checks and never claim that the implementation is correct.

## 5. Define and verify quality in the same Saga {#story-quality-evidence}

**Story ID:** `quality-evidence`<br>
**Revision:** `r1`<br>
**Priority / lifecycle:** must / proposed

> As a quality engineer or reviewer, I can define test cases and ordered expected behavior, link them to requirements, and inspect current verification evidence without leaving the Saga.

### Acceptance criteria {#criteria-quality-evidence}

1. **`test-history`** — Each test case has stable identity, immutable definition revisions, append-only lifecycle history, and explicit conflict heads.
2. **`ordered-steps`** — A test revision records ordered steps with stable step IDs, actions, per-step expected results, and an overall expected result; removed step IDs are not reused for different behavior.
3. **`coverage-kinds`** — Tests explicitly declare positive, negative, and edge coverage kinds, while criterion policy identifies which kinds are required for readiness.
4. **`requirement-links`** — A current test-case relation links directly to one or more criteria and pins both the test revision and story revision.
5. **`typed-evidence`** — Quality evidence distinguishes test-implementation diffs, implementation-under-test references, and execution artifacts; implementation references exactly match current Item-owned selectors.
6. **`current-runs`** — A satisfying run has one semantic head, a passed result, the current test revision, the active source comparison identity, and resolved current evidence.
7. **`quality-traces`** — Queries derive Requirements to Quality to Diff paths and compare design, implementation, and quality coverage per criterion without collapsing them into one score.

## 6. Make readiness and review gaps explainable {#story-transparent-readiness-review}

**Story ID:** `transparent-readiness-review`<br>
**Revision:** `r1`<br>
**Priority / lifecycle:** must / proposed

> As a delivery owner and reviewer, I can tell what is ready, what is missing, and why before approving the change.

### Acceptance criteria {#criteria-transparent-readiness-review}

1. **`separate-gates`** — Requirements, design, implementation trace, quality, review readiness, and completed review are separate derived gates with complete blocker details.
2. **`explicit-exclusions`** — Intentional design or quality exclusions are revision-pinned, cited, and shown separately from covered and missing work; accepted delivery obligations cannot be silently excluded.
3. **`progress-not-proof`** — Work progress, a relation, a passing status, diff coverage, and reviewer approval cannot substitute for one another or for current immutable evidence.
4. **`report-presentation`** — The reviewer UI presents derived Requirements and Quality report sections, keeps the Decks surface separate, and deep-links from criterion paths to Items, tests, and exact code.
5. **`snapshot-consistency`** — Queries and UI projections are bound to one Saga/source snapshot and reject stale cursors rather than mixing revisions.
6. **`review-complete`** — Completed review requires the configured readiness gates plus current required report, slide, and test-case decisions; it does not imply external merge authorization.

## 7. Adopt the evolution without breaking existing Sagas {#story-compatible-schema-evolution}

**Story ID:** `compatible-schema-evolution`<br>
**Revision:** `r1`<br>
**Priority / lifecycle:** must / proposed

> As a Change Saga maintainer, I can adopt the new report-container capabilities explicitly while existing report and slide-native artifacts remain readable and predictable.

### Acceptance criteria {#criteria-compatible-schema-evolution}

1. **`v5-report-container`** — The next report container declares version 5 because version 4 remains the distinct slide-native format.
2. **`legacy-reading`** — New readers continue to read canonical v2 reports, v3 living and hybrid reports, and standalone v4 slide-native Sagas without changing their semantics.
3. **`explicit-upgrade`** — Upgrade to v5 is explicit, dry-runnable, atomic, and never invents design links, tests, exceptions, or evidence.
4. **`old-reader-failure`** — Older readers reject v5 with a clear unsupported-version error rather than partially interpreting new reserved roots.
5. **`query-compatibility`** — Existing query API v1 response shapes remain compatible while typed expanded paths and coverage states are exposed through API v2.
6. **`safe-downgrade`** — Downgrade is allowed only when no v5-only quality, exception, policy, or relation records would be discarded.

## Approval checkpoint {#approval-checkpoint}

Please review the story boundaries, wording, and all 41 criterion IDs. If this requirements set is approved, the next step is to author design slides in this same Saga and link each design node or Item to the criteria it addresses.
