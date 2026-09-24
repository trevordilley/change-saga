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

Query the comparison's Changed, Affected, and Code layers. Changed records
carry before and after state. Affected records were not edited but became
stale or invalid through pinned revisions or code references. Code groups
changed atoms beneath their current owners and reports unreferenced lines.

Follow returned record URNs and evidence record paths. Do not compare prose,
SVG, HTML, or raw metadata bytes to infer impact. Query history for why a
record changed. Re-author stale evidence through supported replace/remove
commands; do not hand-edit it. Read current diff context before assigning a
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

Semantic pre-integration is an integration dependency. Use it only when the
installed CLI lists `preintegrate`; otherwise compare the available committed
states through current read-only queries and report that the dedicated check
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
