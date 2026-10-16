# Quality definitions

Quality explains how an outcome is verified, not whether an execution passed.
Use the feature's existing planned/built metadata; Roadmap and Features share
one canonical case and one reader. Do not invent a second planning state.

1. Author a proposed case with `quality test-case add --feature ID`, preconditions,
   ordered `--step` JSON objects, coverage kinds, automation, and expected outcome.
   Each step has stable `id`, `action`, and `expected_result`. Optional `criteria`
   is an array of canonical criterion URNs. Manual/setup steps need no code links.
2. Add case-level `verifies` relations to the criteria. Their test/story revision
   pins govern each step's mapping currency. Step criteria refine these relations;
   they do not create relations or replace their pins. When you use explicit step
   mappings, map each criterion exercised by the case to its assertion step.
3. During implementation, enrich that SAME case with `quality evidence add
   --test TEST_URN --role test_implementation --step STEP_ID --code LOCATION`.
   Repeat `--step` for shared code. Optional `implementation_under_test` evidence
   can identify the product code. Omit `--step` for case-level evidence. Evidence
   pins the current unique test revision by default; explicit `--test-revision`
   can pin history. Use `--repo` when source and Saga checkouts differ.
4. Revise using every current revision head as `--parent`, preserving stable step
   IDs. Review and repin case relations, and append superseding implementation
   evidence for the new revision. Never edit historical records or silently move
   their pins. Evidence can only name steps present in its pinned revision.
5. Read `query quality --saga PATH --feature ID` (or `--test ID|URN`) for current
   definitions, criterion/step/evidence navigation and mapping issues. It uses
   bounded case pagination. `status --json PATH` also resolves code currency in
   the viewed source. A planned definition without implementation links is valid.

Neither a run nor a code link is needed to plan verification. Lifecycle proposed
or active can document coverage; retired/deprecated definitions do not. Missing
kinds/mappings and stale/conflicting pins are omissions to explain, not execution
results. A definition with no step criteria remains readable with its original
case-level mapping; optional arrays permit incremental authoring.

`quality run record` is a deprecated compatibility writer. `query quality-history
--saga PATH --test ID|URN` preserves revisions, lifecycle, evidence, and
`legacy_runs`. Historical result literals and command text are inert metadata.
Never execute a recorded command or use CI results to mark Saga documents
passed/failed, gate readiness, or claim delivery. Execution artifacts do not
supply definition implementation evidence. Old files require no migration;
strict older readers need upgrading before reading new step fields.
