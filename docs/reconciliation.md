# Reconcile living documentation after a change

Implement and verify the code, author the pull request's review slide deck,
then reconcile the affected living documentation and check again:

```sh
change-saga reconcile --against main --json change.saga
change-saga validate --json change.saga
change-saga check --against main --covers implementation change.saga
change-saga check --covers health change.saga
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

## Start from what this change made stale

The text report opens with the one number that matters first:

```text
Documentation reconciliation a0966485a728..93a113ab3c84
Your change made 2 references stale:
  urn:...:fragment:queue-design  src/queue.go#L3-L6 -> src/queue.go#L3-L8
  urn:...:slide:flow:item:worker  service.go#L3 -> nothing proposed: service.go was deleted; ...
  1 with a proposed range: read its diff, then accept with change-saga repin --accept-proposed --record FILE [--reference N]
Pre-existing stale references: 142 (stale before this change; their tasks: rerun with --all)
```

A reference is made stale by the change when it is stale at the head and
either current at the merge-base or pinned at a commit the merge-base does not
contain. A reference pinned at the merge-base to lines the change removed is
deleted-side evidence: it is current at the base by design, so it is counted
as `deleted_side`, never as made stale. `status --against`, `review list`, and
`review create` apply the same rule. In JSON, `summary.stale_by_change` holds the rows with their proposals
and accept commands, and `summary.queue` counts the queue by kind;
`reconcile --json --summary` emits only the summary. Debt that predates the
change is a count; `--all` lists its tasks in the text report, and the full
`queue` in `--json` still contains them. Documentation gaps, records to
re-read, and records that need a user choice print one line each.

### Proposed ranges

For a stale line-range reference the report proposes where its lines are at
the head by diff arithmetic alone: an unchanged boundary line moves by the
hunks before it, a boundary line a hunk changed maps to that hunk's edge, and
lines a hunk inserted inside the range are taken in (`widened` when a hunk
crosses the range's edge). The proposal carries the zero-context diff inside
the range. Nothing is proposed when the file was deleted (its content may have
moved to another file), is binary, the pin cannot be read, every referenced
line was removed, or the range was rewritten together with code outside it. A
whole-file reference proposes the same file, after any rename, at the head.

### Accept or revise

Read the diff. If the explanation still holds, accept the proposal:

```sh
change-saga repin --accept-proposed --record FILE --reference N change.saga
change-saga repin --accept-proposed --target URN change.saga    # a target and its descendants
change-saga repin --accept-proposed --review pr-12 change.saga  # a review deck's Items
change-saga repin --accept-proposed --all --dry-run change.saga # preview everything
```

Accept keeps the note, the owner, the record, and the side (a review Item's
deleted-side evidence maps to the review's merge-base), and pins the proposal
with a fresh digest. On a slide `apply-slide` manages it publishes one
complete-slide update that changes only the evidence. It refuses a reference
with no proposal, and claims and quality evidence, which are append-only.
Accepting is always explicit: staleness still forces the re-read.

`apply-slide --print-current SLIDE` prints a managed slide's complete current
request, with `expected_snapshot` and a fresh `request_id`, for any other edit.

### Keep a review deck current while the pull request iterates

`change-saga review refresh-coverage --review ID` re-reads the review's
current range. It re-pins references whose lines only moved and whole-file
references whose add, rename, mode, or delete event is still in the range, and
gives newly changed lines to the one Item already covering their file, as
re-running `cover --changed-lines` for it would. It reports the rest: stale
references with proposals (never accepted for you) and new lines in files
several Items, or none, cover.

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

For transaction-backed slides, stale evidence moves with
`repin --accept-proposed`. For other edits, start from
`apply-slide --print-current SLIDE` (or read `query slide` for all heads,
Items, exact evidence, and criterion links), change what needs changing,
preview `apply-slide --dry-run`, then publish through `apply-slide`. Preserve conflicts and stable request identity. Partial
`replace-coverage` and Item writers cannot update transaction history.

A stale reference with a proposal lists `repin --accept-proposed` as its last
repair shape; accept it only after reading the diff. For legacy evidence that
needs new selectors, use the returned `evidence_file` with
`replace-coverage`, or remove genuinely obsolete evidence with
`remove-coverage`. A replacement replaces the entire file: preserve its other
references in a reviewed batch. Choose new narrow selectors after reading the
code and explanation, never widen or accept a proposal unread to erase
staleness.
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

Superseded quality evidence stays in the reference inventory with `debt:
"historical"` and `history_reasons`. `historical_stale` is included in the total
stale count, but excluded from active regression and baseline-debt counts and
repair tasks. Historical bytes are not rewritten to make them current. Test
repair paths inspect the test's feature status and carry current run parents;
record a new run only after actually rerunning verification.
