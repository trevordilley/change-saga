---
name: change-saga
description: 'Author, update, inspect, validate, and open Change Saga product, design, quality, implementation, and pull-request documentation linked to exact code. Use for a requested slice of the lifecycle without expanding it into unrelated authoring; conduct review actions only when explicitly requested.'
---

# Change Saga

## Mandatory contract

A Change Saga is the Git-native documentation of an application. A repository
has one app Saga holding durable product domains and their requirements,
design, quality, work, and implementation explanation. A pull request compares the Saga and code
between commits; its review deck explains that transition. Author the thing
submitted for human review, not the review verdict.

These rules apply to every Change Saga task:

- Follow the user's requested scope. Do not turn one story, term, diagram, or
  lightweight review into a complete-lifecycle project. If the user requests
  the complete lifecycle, do not silently stop after its easiest part.
- Speak as the change author while authoring. Record approvals, change
  requests, withdrawals, or review comments only when the user explicitly
  asks for a review. Never act or decide on another person's behalf.
- The installed `change-saga` CLI is the source of truth. Start with
  `change-saga --help`; use command `-h`, `change-saga spec --json`, and
  `change-saga query schema <operation>` instead of guessing a command or
  response shape. If a reference disagrees with the CLI, follow the CLI and
  report the mismatch.
- Read real Saga metadata only through `change-saga query`. Never glob, grep,
  or open metadata files to infer Saga state. Page every result to completion,
  keep one snapshot across a multi-query read, and restart if it changes.
- Make Saga mutations only through public CLI authoring commands. Do not edit,
  invent, rename, or delete metadata files. Narrative and visual source files
  are installed or replaced through the CLI commands that own them.
- Preserve history and uncertainty. Revisions and lifecycle events are
  immutable; decisions, comments, claims, verification results, and merge
  evidence are append-only. Preserve competing heads and reported conflicts;
  never fabricate a winner. Reconcile only with explicit parents and the
  user's supported intent.
- Preserve provenance. Keep requirement citations, relation rationales and
  pins, review identity, source comparison identity, code-reference digests,
  and lifecycle reasons. Omit an identity you cannot verify rather than
  recording a guess.
- Keep evidence exact. Code references belong on the narrowest semantic Item,
  landmark, test case, or other supported target that explains them. Never
  widen a selector merely to reach complete coverage. A feature implementation
  deck and a pull-request review deck have different coverage roles; do not
  substitute one for the other.
- Treat coverage as an omission check, not proof. Do not invent personas,
  stories, criteria, definitions, designs, test results, or novelty to fill a
  gap. Report genuine uncertainty and preserve conflicts.

## Route the task

Read only the references needed for the requested work. Do not preload every
reference. When a task crosses rows, combine only those rows.

| Requested work | Read before acting |
| --- | --- |
| Inspect or navigate an existing Saga; load compact feature context; resolve current heads, conflicts, evidence, or history | [Reading through the query API](references/query.md) |
| Audit whether one feature has a current, exact implementation handoff | [Reading through the query API](references/query.md) |
| Author or revise personas, stories, acceptance criteria, citations, requirement relations, or lifecycle state | [Query](references/query.md), then [stories and provenance](references/stories.md) |
| Author or revise the overview, pitch, description, or project vocabulary | [Query](references/query.md), then [overview and terms](references/terms.md) |
| Author diagrams, implementation/review decks, narrative fragments, landmarks, exact code evidence, or claims | [Query](references/query.md), then [diagrams and evidence](references/diagrams.md) |
| Render slides and run mechanical visual QA | [Diagrams and evidence](references/diagrams.md) |
| Reconcile a comparison, work with a companion repository, repin landed evidence, recover, or hand off work without changing visuals | [Query](references/query.md), then [integration and recovery](references/integration.md) |
| Compare parallel proposal branches or deliberately withdraw/consolidate a duplicate proposal | [Query](references/query.md), [integration](references/integration.md), and [stories](references/stories.md); use only capabilities confirmed by the installed CLI |
| Prepare or update a pull-request review artifact, or change visuals while integrating | [Query](references/query.md), [integration](references/integration.md), and [diagrams](references/diagrams.md) |
| Define a CI acceptance rule | [CI rules](references/ci.md); add [query](references/query.md) only when inspecting real Saga state |
| Look up resource shapes, stable target identities, or command families | [Format quick reference](references/format.md), only when the CLI's `spec`, help, or query schema is insufficient |

The references describe current public contracts only. Do not infer a command
from a planned capability or another branch. Discover new CLI or query support
from the installed binary before using it.

## Work at the requested scope

For a new story, capture a persona-focused outcome and independent,
observable pass/fail criteria. A proposed story may remain criterion-free
while intent is uncertain; an accepted story needs at least one criterion. Add
only the narrowest obligation confirmed by the user.

For an existing code change, it is valid to begin with its implementation or
review deck and offer missing product context as optional follow-up. For new
work whose whole lifecycle is requested, begin with personas and stories,
develop relevant design and quality, and connect exact implementation evidence
as it is built. Never manufacture product intent to make coverage complete.

Features are durable product domains, not changes. A story may move between
features without changing its identity. Declare adjacent semantic links and
let queries infer longer paths: story to persona, design or test case to story
or criterion, and visual Item or supported evidence target to exact code.
Relations retain their rationale and the revision they were read against; a
stale relation is repinned only after reading the new wording.

The chain is persona -> story -> design or test -> exact code. The Saga is
documentation, so requirements, designs, tests, and decks describe current
intent rather than carrying review verdicts. This is not a waterfall:
discovery may revise earlier records, but it does so with new immutable
history rather than rewriting what was previously known.

## Common workflow

1. Select one invocation form for the task. Prefer an installed
   `change-saga`; in this source repository use `go run ./cmd/change-saga`
   when no installed executable is available.
2. Confirm the Saga path. Create one with `change-saga init` only when the
   repository has none and the requested work authorizes creation. Resolve the
   requested scope and, for comparisons, the verified base and head. Query
   current state through the API and retain its snapshot.
3. Use the routed reference and public commands to make the smallest complete
   change. Follow returned URNs and evidence record paths; do not reconstruct
   them from storage.
4. After implementing, verifying, and preparing a PR review deck, run
   `change-saga reconcile --against <base> --json <saga>`. Inspect the queue,
   reassess affected living documentation, make justified repairs through
   typed public paths, then reconcile again. Review coverage is independent
   of HEAD documentation currency; retain baseline debt and uncertainty.
   Run `change-saga validate --json <saga>` and the task-relevant bounded
   queries. Use `status --json` and its ordered `next_actions` as a work queue,
   not a verdict. Use
   `check --covers ...` only for the areas the user or team actually requires.
5. Stop when the requested outcome is complete. Offer unrequested growth as
   optional and never present a clean status as proof that the explanation is
   correct.

Opening a Saga does not authorize a review. When explicitly asked to review,
first inspect the code diff independently, then inspect the author's deck and
evidence, test claims independently, and finally record only your own review
seat's actions. The tool records per-slide decisions; it never declares that a
review is approved.
