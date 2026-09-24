# Stories, provenance, and lifecycle

Read this reference for personas, stories, criteria, requirement citations,
relations, or lifecycle changes. Read [query.md](query.md) first for an
existing Saga. Discover exact mutation syntax from the installed CLI.

## Model the product outcome

A persona is a person who gets value from the app: the "As a ..." in a user
story. A tool, agent, or system operating the app is not a persona. Write the
human or organizational beneficiary, not the mechanism performing work.

A story captures an outcome for that persona and why it matters. Criteria are
independent, observable pass/fail obligations. A proposed story may remain
criterion-free while its intent is uncertain. Before moving it to `accepted`,
every current revision head needs at least one criterion. When only one
obligation is confirmed, add exactly that narrow obligation; never invent
broader behavior to make acceptance valid.

Features are durable product domains, not pull requests. Revise a story in
place as understanding improves; use the supported move operation when its
domain changes so stable identities and links survive.

## Preserve revisions and conflicts

A revision is a complete immutable snapshot. A single-parent revision may
inherit fields that the command does not replace; a multi-parent conflict
reconciliation must state the complete intended definition. Use dedicated
criterion commands for criterion-only changes when offered by the CLI.

Lifecycle transitions are append-only events with explicit parents, state,
and reason. Never rewrite or delete a previous revision or event to simplify
history. Queries may report several current definition or lifecycle heads;
preserve them all and do not infer a winner. Reconcile competing heads only
when the user or authoritative source establishes the intended result, naming
every required parent through the public command.

After each mutation, query the requirement and its history. Confirm the new
head, all parents, lifecycle state, and any remaining conflicts rather than
trusting filenames or a mutation message alone.

## Withdrawing or consolidating duplicate proposals

Use these focused operations only when
`change-saga story -h` lists `withdraw` and `consolidate`; otherwise stop with
the current heads and candidate evidence instead of simulating either action
with metadata edits or a false lifecycle transition.

When available, `story withdraw` appends a reasoned `rejected` lifecycle event
to a uniquely current `proposed` or `deferred` story. It refuses accepted
intent. Supply the exact story URN, current lifecycle parent, new event ID, and
reason.

`story consolidate` is preview-first and requires the duplicate and canonical
story URNs, the duplicate's current lifecycle parent, a new event ID, a reason,
and an exhaustive one-to-one mapping from every current duplicate criterion to
a current canonical criterion. Add `--apply` only after reviewing the preview.
Applying rejects a proposed/deferred duplicate, or retires an accepted
duplicate only into an accepted canonical story. It replaces only affected
relations, preserves exact non-requirement endpoints and current canonical
pins, and retains old relations as superseded history. Conflicted heads,
missing or many-to-one mappings, terminal canonical stories, invalid resulting
relations, and replacement-ID collisions are refused before writing.

Apply reloads and revalidates current state; a preview does not reserve its
snapshot. Stale, conflicted, or unverifiable external pins are refused.
Transaction-owned criterion links must first be updated through `apply-slide`;
consolidation refuses to retire their story while leaving those links behind.
Multi-record publication has best-effort rollback, not reader or crash
atomicity. If rollback fails, preserve the reported recovery files and paths
and inspect current state before retrying.

## Keep provenance exact

Requirement citations are immutable provenance records describing where a
story, criterion, or decision came from. Prefer durable source identity and a
focused excerpt or locator. Never replace provenance with implementation
evidence, and never fabricate a citation to make a requirement appear sourced.

Relations declare adjacent semantic links. A story names personas; a design
or test case addresses or verifies a story or criterion; a specific visual
Item may explain one. Longer traceability paths are inferred. Prefer the
narrowest criterion endpoint supported by the actual relationship.

Every relation preserves its rationale and pins the target revision it was
read against. A later wording change can make it stale even if the stable URN
is unchanged. Read the new wording before using `relation repin`; preserve the
relation ID, rationale, prior pins, and provenance. Supersede a relationship
through its public append-only operation rather than deleting its record.

## Focused workflow

1. Query the current persona with `query personas --persona ID|URN`, inspect
   `query persona-references` when its uses matter, then query the current
   requirement, its history, relevant citations, and relations. Page fully,
   keep one snapshot, and honor each reference result's completeness metadata,
   including every independently paged unresolved owner.
2. Confirm the persona, outcome, value, and the narrow observable obligations
   in scope. Leave uncertainty proposed instead of filling it with guesses.
3. Use the smallest public authoring command: add, revise, criterion change,
   lifecycle transition, move, relation mutation, or citation addition.
4. Re-query current heads, history, conflicts, stale relations, and relevant
   traceability. Preserve every competing head until explicitly reconciled.
5. Run validation. Treat optional design, quality, and implementation growth
   as optional unless the user requested those lifecycle areas.

Do not modify product code merely because story authoring exposed a possible
implementation change unless the user asked for implementation too.
