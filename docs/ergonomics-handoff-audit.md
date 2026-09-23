# Feature handoff audit

`change-saga query audit` is a read-only completion audit for one whole feature. It is intentionally distinct from `change-saga validate`: validation checks whether persisted records satisfy their schemas and structural invariants, while the audit reports whether the current feature has enough exact, current handoff evidence for a reviewer to continue.

## CLI contract

```text
change-saga query audit --saga PATH --feature ID|URN [--repo PATH] [--head REV]
```

`--feature` accepts either a bare feature ID or its canonical `urn:change-saga:<saga>:feature:<feature>` URN. `--head` selects the one source revision being audited and defaults to `HEAD`. An audit never compares two revisions, so `--against` is rejected. `--repo` names the source checkout when it is separate from the Saga checkout.

The command emits exactly one `change-saga.ai/v1` JSON envelope. `change-saga query schema audit` and `change-saga query audit --help` provide deterministic discovery without opening a Saga. The report contains:

- `feature` and `feature_id`: the resolved exact feature identity.
- `status`: `pass`, `findings`, or `incomplete`.
- `complete`: false whenever competing heads prevent a single current view. It never becomes true merely because partial data was inspectable.
- `ready`: true only when the audit is complete and has no findings.
- `exit_code`: the process result represented in the report.
- `summary`: inspected story, criterion, Item, and relation counts plus error and warning counts.
- `findings`: exact offending IDs, stable codes, severity, reasons, related IDs, and feature IDs where relevant.
- `exceptions`: recorded coverage exceptions with their currency, rationale, citations, reasons, and competing heads.
- `intentional_risks`: Items or criteria whose otherwise-missing implementation handoff is covered by a current, cited implementation exception.
- `unresolved_conflicts`: exact resources and every competing head; the audit does not select a winner.

Exit `0` means `status=pass`. Exit `8` means the JSON report was produced successfully but contains findings or is incomplete. The envelope remains `ok=true`; callers must inspect the report and must not mistake an audit finding for an execution failure. Existing query execution exits remain unchanged, including `2` for invalid arguments, `3` for an invalid Saga, `5` for an unknown feature, and `7` when source data is unavailable.

At the transport-neutral application boundary, callers open `livingapp` with `OpenOptions{Audit: true}` and issue `Query{Operation: "audit", Filters: Filters{Feature: value}}`; `Result.Data` is an `AuditReport`. The explicit open option binds exception history to the same read snapshot and prevents ordinary living queries from acquiring a larger read set. The CLI adapter additionally resolves Item code selectors at the selected source head before it finalizes readiness and exit status.

## Findings

The audit currently reports these stable finding codes:

- `broad_visual_intent`: an active Deck- or Slide-level `addresses`/`explains` link targets either a story or a criterion. It remains visible even when legacy descendant behavior makes it traversable; it is not rewritten or presented as exact Item ownership.
- `item_intent_missing`: an implementation Item has no current exact `explains` relation to a story or criterion owned by the feature.
- `item_evidence_missing`: an implementation Item has no exact code-reference evidence and no applicable current cited implementation exception.
- `criterion_explanation_missing`: a current proposed or accepted criterion has no current exact Item-level explanation and no applicable current cited implementation exception.
- `stale_or_dangling_pin` and `conflicted_relation`: persisted relation pins do not resolve to one current endpoint. The reasons and current competing heads come from the existing relation-currency model.
- `stale_or_dangling_selector`: an Item code reference cannot be resolved at the audited source head. The report retains the exact Item, code location, evidence file, and resolver reason.
- `cross_feature_unassigned_link`: relation storage or endpoints span multiple feature owners without another persisted handoff assignment that resolves the boundary.
- `exception_not_current`: an unsuperseded recorded exception is stale, invalid, or conflicted and therefore cannot turn missing authoring into an intentional risk. Superseded exceptions remain visible as lifecycle history but are not themselves findings.

Warnings remain actionable and therefore keep `ready=false`. The audit does not infer semantic correctness from proximity, labels, slide order, matching words, code paths, or Git history. It reports persisted links and evidence exactly as authored, and never creates or repairs them.

## Scope and integration

The audit reads existing feature ownership from loaded Saga, requirements, work-plan, and quality models. It deliberately introduces no competing persisted schema. Current cited `implementation` coverage exceptions are the only records that distinguish an intentional implementation gap from missing authoring.

The generic canonical feature traversal remains owned by the feature-context work. This implementation keeps its ownership index local to the audit boundary so that the shared traversal can replace it without changing the CLI or JSON report contract. The AI-instructions integration must add `audit` to the generated query reference in `skills/change-saga/**`; this branch does not edit that sibling-owned area.
