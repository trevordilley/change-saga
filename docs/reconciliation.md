# Reconcile living documentation after a change

Implement and verify the code, author the pull request's review slide deck,
then reconcile the affected living documentation and check again:

```sh
change-saga reconcile --against main --json app.saga
change-saga validate --json app.saga
change-saga check --against main --covers implementation app.saga
change-saga check --covers health app.saga
```

Pass `--repo PATH` to source-dependent commands when the Saga and source have
separate checkouts. Comparisons use committed source, with the merge-base of
`--against` and `--head` (default `HEAD`). The report names exact source OIDs
and the Saga sides selected by the comparison, including companion sync
cursors. It uses the working Saga when that is the selected head, so authoring
repairs can be checked before committing them. It never edits the Saga. The `snapshot` identifies the opened tree and
comparison; a concurrent Saga edit rejects the report and requires a rerun.

`reconcile` exits zero when it produces the report, including when findings
remain. A structural or source error prevents a trustworthy report. Baseline
read failures are explicit diagnostics: debt becomes `baseline_unknown`,
never silently pre-existing or clean. Teams choose their own acceptance rules.

## Read the independent results

- `documentation_coverage` accounts for changed lines through the living
  implementation/narrative and declared story/design/test links. Deleted-line
  evidence may be valid at base. This is diff accounting, not proof that the
  documentation describes current behavior.
- `reviews` reports each open PR deck's own range, diff coverage, and per-slide
  review state. Review coverage cannot satisfy documentation coverage. Item
  evidence remains distinct from slide approval; reconciliation records neither.
- `currency` resolves every living code reference at HEAD independently of
  base coverage. Pure line movement can remain current and is reported as
  remapped. Changed or unverifiable bytes are stale. Claims and quality
  evidence retain their immutable history; historical stale evidence is not
  rewritten just to make a count disappear.
- `head_health` reports current record/link problems across the app, including
  stale requirement relations. These problems remain visible even when a
  coverage percentage is complete.
- `changed` and `queue` connect detection to action. Uncovered changed code
  includes inspection and an explicit author choice of exact reference/owner. Queue reasons identify
  HEAD currency, health, code impact, pinned endpoints, and declared chains.
  A changed requirement includes a traceability inspection even with no code
  diff. Missing declared paths are a visibility gap, not proof of no impact.

Currency debt compares identical reference records against the actual base
Saga and source: `pre_existing` was already stale there, `regression` was
current there, and `introduced` has no identical base reference (including
revised evidence). `base_stale` retains the baseline total; `references`
contains all current-Saga references, including healthy ones and their resolved
locations. An unavailable baseline is `baseline_unknown`. Other health debt
matches resource plus problem: unchanged problems are `pre_existing`; others
are `new_or_changed`. These are mechanical classifications, not severity or
semantic equivalence judgments.

## Act on the queue

Inspect each record's `because` and `inspect` commands. Query results may be
paged: follow cursors to completion and keep a stable snapshot. `repair`
contains typed command shapes with known values and explicit author inputs,
not commands to run blindly. An affected record may need no edit after review.
Current bytes can still support obsolete prose or an incorrect diagram.

For transaction-backed slides, read `query slide` for the authoring snapshot,
all heads, Items, exact evidence, and criterion links. Prepare the complete
replacement request and preview `apply-slide --dry-run`; then publish through
`apply-slide`. Preserve conflicts and stable request identity. Partial
`replace-coverage` and Item writers cannot update transaction history.

For legacy evidence, use the returned `evidence_file` with
`replace-coverage`, or remove genuinely obsolete evidence with
`remove-coverage`. A replacement replaces the entire file: preserve its other
references in a reviewed batch. Choose new narrow selectors after reading the
code and explanation, never widen or mechanically repin to erase staleness.
For requirement relations, read both endpoints before `relation repin` with an
updated rationale, or supersede a relationship that no longer holds. For test
evidence, rerun the relevant tests and append evidence and actual run results.

After repairing the explanation and evidence, rerun relevant implementation
checks, `validate`, and `reconcile`. The report's `recheck` commands retain the
exact compared OIDs; deliberately advance `--head` after new source commits.
For a historical committed Saga head, the queue provides comparison inspection
and withholds repair shapes: check out the intended Saga revision and rerun
before authoring, because content queries and writers address the working Saga.
Use `check --covers ...` only for the areas your team requires. For current
health, omit `--against`: comparison-side validity alone cannot establish HEAD
currency. Neither validation, pin freshness, nor coverage proves semantic
correctness. Manual reassessment can remain in the queue after a successful
repair because reconciliation does not store a new approval or waiver.
