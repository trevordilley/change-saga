# Stories, provenance, and lifecycle

Read this reference for personas, stories, criteria, requirement citations,
relations, or lifecycle changes. Read [query.md](query.md) first for an
existing Saga. Discover exact mutation syntax from the installed CLI.

## Ground the interview before drafting

This applies to candidate stories in conversation, not only persisted records.
An interview builds on the Saga's structured knowledge rather than starting
with a blank template:

1. Before proposing an "As a ...", query the existing personas. If the ID is
   unknown, enumerate `query personas --saga PATH` and page to completion;
   otherwise select the known ID or URN. Read definitions, current revision
   heads, and lifecycle state. Match by responsibilities and needs, not just
   the name. Name the selected persona and give its stable ID or URN alongside
   a candidate story. Do not replace a defined persona with a generic role.
2. Query the relevant feature context and existing requirements before
   proposing a new story; use their terms, relations, and linked technical
   definitions when they bear on the requested outcome. Prefer refining an
   existing story when it already expresses the intent. Follow the query
   reference's snapshot and pagination rules; read exact definitions where a
   compact index leaves uncertainty.
3. Briefly explain the relevant existing context, then ask about the missing
   outcome, value, or observable behavior. Do not ask the user to restate facts
   already established by those records or the conversation. Separate the
   user's confirmed intent from candidate wording and open questions; do not
   turn a proposed UI or the current implementation into a requirement.
4. If several personas fit, present the actual candidates and ask which
   outcome belongs to whom. If none fits, explicitly propose a new persona for
   confirmation rather than silently inventing one. Retired or conflicted
   definitions are not an unambiguous current choice. If querying is blocked,
   report that limitation and leave the persona unresolved.

Continue to capture only confirmed obligations. Interviewing does not itself
accept a story, authorize a new persona, or authorize implementation. When
authoring is requested, use the queried identities and supported typed persona
links; never substitute prose mentions for those links.

## Conversational feature discovery

For a feature interview, use this sequence rather than immediately generating
a backlog. A focused request to author a specific, already-understood story
does not require restarting discovery.

1. After identifying and confirming the existing personas, invite the user to
   explain conversationally what they want to build. Build on what they have
   already said; do not demand story syntax or choose a UI for them.
2. As the conversation develops, query relevant existing stories, implementation
   explanations, personas, and Component/System definitions when available.
   Keep a clear distinction between the desired outcome, documented intent,
   implemented behavior, and missing knowledge. Do not invent absent inventory
   or assume that a documented design is implemented. Use this context to ask
   focused follow-ups about examples, boundaries, and conflicts.
3. When the goal is coherent, summarize your understanding for correction and
   ask the user for a few representative user stories in their own words.
   Then offer to draft the remaining stories and observable criteria for their
   review. Do not present an exhaustive invented backlog as confirmed intent.
4. Once the user has reviewed the drafts and agreed what to record, author the
   stories in the app Saga through the public CLI, using existing persona and
   feature identities and truthful lifecycle state. Re-query and validate the
   records. Commit coherent documentation changes when requested or covered by
   the working agreement; stage only the intended changes and report the commit.
   Agreement to record a proposal does not itself make its lifecycle accepted.
5. Adapt to the user's role and technical comfort: ask whether to discuss the
   technical implementation next. If they want to proceed, develop an
   implementation and architecture plan grounded in the stories and existing
   technical design. Author and commit it in the feature's documentation as
   the specification subsequent AI work will reference, with explicit links,
   decisions, constraints, and open questions. Read the applicable design and
   diagram authoring guidance before doing so. Planning is not authorization
   to implement, and proposed architecture is not current code evidence.

Use Change Saga to retrieve this context efficiently, not filesystem searches
of Saga metadata or conversational memory alone. Discover available operations
with CLI help and `query schema`. Start with `query context --feature ID|URN`
when the feature is known; otherwise use `query overview` and `query children`
to locate it. Follow returned identities into `query requirements`,
`query personas`, and `query terms`; use `query relations` and
`query traceability` for supported connections, `query slide` or
`query fragment` for implementation explanations, and `query inventory` for
Component/System definitions when supported. Expand only the records relevant
to the next interview question and page each selected query completely. State
missing links or unavailable query capabilities rather than assuming a complete
cross-domain search or inventing records.

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
