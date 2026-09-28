# Integration, comparison, and recovery

Read this reference when reconciling a code comparison, preparing or updating
a pull-request review artifact, working with a companion Saga repository,
repinning landed evidence, or handing work to another author. Read
[query.md](query.md) first; add [diagrams.md](diagrams.md) only when the task
also creates or changes visual or narrative content.

## Resolve the comparison without guessing

For a pull-request number or URL, obtain number, URL, title, stated motivation,
base branch and OID, head branch and OID, commits, and changed files from the
hosting provider. Use an available provider integration or its supported CLI.
Cross-check the provider head and changed-file summary against the local
checkout before recording anything.

For a local change without provider metadata, confirm the intended merge base
with the user or repository workflow. Comparisons operate on commits;
uncommitted changes are outside the comparison. Omit pull-request identity
rather than associating the Saga with the wrong change.

When asked to draft or prepare a pull request, preserve the repository's
existing PR templates, issue context, authoring process, and checks. Express
the result in the Saga rather than replacing that workflow.

## Reconcile from structured evidence

When the Saga holds living documentation (or the user asks), run
`change-saga reconcile --against <base> --json <saga>` after implementing,
verifying, and authoring the PR review deck. A Saga that holds only reviews
has nothing to reconcile; its review's coverage is the whole answer.

Start from the signal. The text report (and `summary` in `--json`, alone with
`--json --summary`) leads with "Your change made N references stale": one line
per reference with owner, old range, and proposed range. `status --against`,
`review list`, and `review create` say the same. Stale means stale at HEAD
and either current at the merge-base or pinned during the change (deleted-side
evidence of removed lines written during the change is counted apart; living
documentation that already described lines the change deleted is listed for
repair); debt that was already stale is a
count, listed only with `--all`.

Repair each one cheaply but with judgment:

1. Read its proposal: the pinned start and end mapped through the diff hunks,
   widened over lines inserted inside, with the small diff inside the range
   and the nearest hunk just before and after it (`references --stale`, or
   `repin --accept-proposed --dry-run`). `widened` means it takes in lines
   the range did not have, such as a new sibling function git slid inside:
   read them. A neighbouring hunk that adds a helper can mean the code moved.
2. If the explanation still holds (lines shifted, a field or guard added),
   accept it: `change-saga repin --accept-proposed --record FILE --reference N
   <saga>`, or in bulk with `--target URN`, `--review ID`, or `--all`. It keeps
   the note, owner, record, and side, pins the proposal at a fresh digest, and
   updates an `apply-slide` slide through one complete-slide transaction.
3. If it no longer holds, revise the explanation and re-cover. Nothing is
   proposed when the file was deleted (content may have moved) or the range
   was rewritten together with code outside it; accept refuses those.

Accepting is explicit and never automatic: staleness still forces a re-read.
While a pull request iterates, `review refresh-coverage --review ID` re-pins
its deck's moved references and whole-file references whose file event
persists, and lists the rest for you: stale references with proposals, and
new lines, uncovered, with the one Item covering their file as the proposed
owner. It never extends an Item on its own; after reading the new lines,
`--accept-proposed [--path P] [--note TEXT]` gives them to that owner under a
neutral "added in <head>" note unless you write one. `repin --accept-proposed
--all` reaches every open review deck, other pull requests' too; a review
whose range cannot be read is skipped and reported.

The full queue separates HEAD code
reference currency from documentation diff coverage and each review's own
coverage. It retains baseline debt, identifies newly stale references, and
reports the reason and typed inspection/repair paths for each implicated
record. An affected record needs reassessment, not necessarily an edit.
Requirement-only changes prompt traceability inspection of declared
implementation and test paths even when no code changed.

Read query pages to completion. For transaction-backed slides, stale evidence
moves with `repin --accept-proposed`; for any other change start from
`apply-slide --print-current SLIDE`, which prints the complete current request
(snapshot, Items, evidence, criterion links) to edit and apply. Preserve every
authoring head. Other evidence that needs new selectors uses the returned
evidence file with `replace-coverage` or deliberate removal. Replacing a file replaces all its
references, so preserve still-valid ones in the complete request. Reassess
meaning before repinning relations or changing evidence.

Rerun `validate`, relevant implementation/tests, and `reconcile` after repairs.
The returned recheck commands pin the source OIDs; advance head deliberately
after new code commits. Use `check --covers health` without `--against` for
current health. Validation is structural; current pins do not prove prose,
diagrams, code, or tests semantically correct. Reconciliation is read-only and
records no review decisions or resolution acknowledgments.

Query the comparison's Changed, Affected, and Code layers. Changed records
carry before and after state. Affected records were not edited but are implicated through pinned
revisions, code references, or declared chains. They may remain valid after
reassessment. Code groups
changed atoms beneath their current owners and reports unreferenced lines.

Follow returned record URNs and evidence record paths. Do not compare prose,
SVG, HTML, or raw metadata bytes to infer impact. Query history for why a
record changed. Re-author stale evidence through `repin --accept-proposed` or
the supported replace/remove commands; do not hand-edit it. Read current diff context before assigning a
new owner, and update visual content when behavior changes even if an old code
reference remaps cleanly.

Preserve concurrent heads, stale relations, and explicit work conflicts. Do
not collapse them to one result during handoff. A recovery note should state
the verified snapshot/comparison, completed mutations, remaining heads or
conflicts, stale evidence, validation state, and the smallest safe next query
or command.

Parallel authoring is a core property: partition ownership by stable resource
boundaries and merge the authored records with the code. It reduces shared-file
conflicts but does not erase semantic conflicts; report both competing heads.

## Semantic pre-integration

Discover `preintegrate` in the installed CLI before use; otherwise compare the
available committed states through current read-only queries and report that the dedicated check
is unavailable.

When available, `preintegrate --ref REF --ref REF [--repo PATH] [--json]
<saga>` reads the Saga from two or more explicit committed Git refs. It reports
stable-ID collisions, different current heads, and deterministic text-overlap
candidates with exact ref, commit, and Saga-path provenance. It never reads an
uncommitted working-tree Saga, chooses semantic equivalence, updates refs,
checks out a branch, or merges Git. Treat the overlap score as a review prompt,
not a duplicate decision; use [stories.md](stories.md) for an explicitly
authorized withdrawal or consolidation.

## Pull-request review artifacts

A review is one pull request's slide deck, bound to its verified base and head.
It explains what changed and why; feature implementation decks still explain
the current code. Review Items may name the durable record they revised, but
review coverage never substitutes for documentation coverage.

Every changed line in the review's range belongs to the narrowest review Item
that explains it. Query review coverage and uncovered atoms rather than
inferring completeness from the deck. After the change lands, use the public
repin operation with the verified landed commit and branch so code evidence is
re-pinned and merge reasoning is retained before the branch disappears.
Without that step, a landed review still reads as merged: `review list` and
the Reviews page find it through Git without writing anything (`state_source`
`detected`) and hide it by default. Detection never suggests a late
`repin --onto`: chosen after the fact, it can re-pin the whole Saga to an old
commit.

Opening a Saga only presents it. It does not authorize review actions. When a
review is explicitly requested, inspect the diff independently before the
author's explanation, then inspect mappings and claims, test them, and record
only your own review seat. AI review identity includes a distinct reviewer
name, agent, and exact model. Never turn an AI result into a human decision or
act on a person's behalf. Decisions and comments are append-only and may go
out of date when slides or referenced code change. The tool records decisions;
the team decides its approval policy.

## Companion repositories

When the Saga is separate from the code checkout, pass the verified source
checkout with `--repo` to every command. Advance the sync cursor through the
public sync command in each Saga commit that updates documentation. Never store
a local checkout path as durable repository identity.

## Closeout

Close out Saga work when the pull request is created or updated, or when
handing back Saga work the user asked for. An ordinary coding session does not
end with it.

Before handoff:

1. page the relevant gap, mapping, conflict, relation, and history queries at
   one stable snapshot;
2. run validation and the requested coverage checks;
3. confirm exact Item-level evidence, current relation pins, append-only
   history, source comparison identity, and unresolved conflicts;
4. repin only after a verified landing when the workflow calls for it; and
5. report optional growth separately from unfinished requested work.

What exists must stay healthy: do not hand off a newly stale or broken record
merely because it falls outside a coverage percentage.

If a public command is unavailable, stop before inventing file edits. Report
the missing capability and a read-only recovery path using query schema,
bounded queries, validation, and the machine-readable error code.
