---
name: change-saga
description: 'Create a review for a pull request or branch ("create a review for this PR", "change-saga this PR"), meant for the big change a person reviews as a whole, such as an integration branch gathering many agents'' work: a slide deck that explains the change''s architecture, with every changed line linked to the slide Item that explains it, so a human reviews the architecture instead of every line. Also author, update, inspect, validate, and open Change Saga product, design, quality, implementation, and living documentation linked to exact code when asked. Use for a requested slice without expanding it into unrelated authoring; conduct review actions only when explicitly requested.'
---

# Change Saga

Change Saga makes large changes, often AI-written, easy to digest. The author,
or their coding agent, creates a review deck for the pull request: slides that
explain the change's architecture, so a person reviews the architecture
instead of every line. Coverage makes the review trustworthy: every changed
line is linked to the slide Item that explains it.

## Review the big change, not every branch

A review deck is for the change a person reviews as a whole: usually a large
one, the pull request that brings many commits, workspaces, or agents' work
together. In an orchestrated workflow, where one integration workspace or
branch gathers the work of many child workspaces or sub-agents, only the
integration branch's pull request into the main branch gets a review. The
child branches merging into the integration branch get none, and neither do
their commits: the integration review explains their combined change as one
architecture. Many one- or two-slide reviews fragment the story a deck exists
to tell. A small pull request a reviewer can read line by line does not need a
deck unless the team asks for one.

Deferring is not skipping. The Saga is still kept complete and current: when
the real review comes, its pull request gets the full deck with every changed
line covered, and the living documentation is reconciled and repinned for
everything the children changed. That hard work is done once, for the change
people actually review, rather than piecemeal in every child. As a child
workspace or sub-agent, create a review or do Saga upkeep only when the user
or the parent asks.

## When to do Saga work

Saga work belongs to the moment a pull request is created or updated, not to
every session or commit. Creating or refreshing the review deck, covering
lines, reconciling living documentation, and repinning all happen when the
user says to create or update the PR ("okay, create a PR", "let's update the
PR"), or when their process otherwise publishes changes for review. Then do
it all together, so the deck explains the change as reviewers will see it.
Otherwise, keep a coding session about the code: do not end it with Saga
upkeep nobody asked for. A deck that lags the branch between PR updates is
expected. When the user asks for Saga work directly, do it then.

## Create a review for this PR (the usual first task)

When asked to "create a review for this PR" (or to sagafy a pull request or
branch), do this. Read [diagrams and evidence](references/diagrams.md)
before drawing slides; nothing else is needed.

1. If the repository has no Saga, `change-saga init` creates `change.saga` at
   its root.
2. `change-saga review create change.saga` works out the pull request, base,
   and review id (through `gh` when installed, else origin's default branch
   and the branch name) and prints the next command. Pass `--pr`, `--base`,
   or `--id` when they are known.
3. Read the diff against the base and find its architecture: the few ideas a
   reviewer must understand, such as the components and how they relate, the
   data flow, and a state change. Then find its surprises: whatever breaks a
   reasonable maintainer's expectation (counterintuitive behavior, a
   convention it changes, hidden coupling, a displaced cost, an ordering or
   ownership constraint, a tradeoff or rejected alternative, failure and
   recovery behavior). Surprises are what a reviewer most needs to see; the
   rest of the deck is the model that makes them legible. Plan one slide per
   idea, usually two to six; never one slide per file.
4. For each slide, write one request: a `"diagram"` source (explicitly
   positioned nodes, edges, and groups the CLI renders to SVG) and an Item for
   each element that explains part of the change. Publish it with `change-saga
   apply-slide --review ID --from SLIDE.json change.saga` (`--dry-run` first).
   The request's shape and an example are under "Review deck slides" in
   [diagrams and evidence](references/diagrams.md); revise a slide with
   `diagram edit`. Give an element a short Markdown `note` when a reader
   would want the why its label cannot hold (an edge's protocol or failure
   behavior, a node's responsibility); readers see it on hover. Surprises
   still go in callout Items, and a note is never padding: omit it when there
   is nothing to add. Hand-write SVG only for what a diagram source cannot
   express, through `add-slide --review`, `set-slide-content --review`, and
   `add-item --review`.
5. Call out every surprise with a `"kind": "callout"` Item in that slide's
   request: `"about"` names the Item for the responsible element, and
   `"body"` (at most 240 characters) states what a reviewer would expect,
   what the change does instead, why, and the consequence. Cover the code
   that shows the actual behavior from the callout itself. Reviewers see
   callouts in a Surprises panel on the slide. Never manufacture one: if
   nothing surprises, say so in the concluding slide's takeaway.
6. Cover every changed line from the narrowest Item that explains it:
   `change-saga cover --target ITEM_URN --path PATH --changed-lines
   change.saga`, or `--lines RANGES --side new` for part of a file
   (`--side old` for deleted lines). On a review Item, cover compares the
   review's own range. `change-saga query children --saga change.saga
   --parent REVIEW_URN` lists the review's slides and Items.
7. Confirm: `change-saga review list --uncovered change.saga` names what no
   Item explains yet, and `change-saga review list --review ID change.saga`
   lists the surprises called out; `change-saga check --covers review --against BASE
   change.saga` exits 0 once every changed line is explained. Run
   `change-saga diagram check --review ID change.saga` and `change-saga
   validate change.saga`.
8. Commit the Saga with the change. Reviewers can collapse `change.saga` in
   the pull request's file tree and open the deck with `change-saga open
   change.saga`.
9. Each time the PR is updated (not after every commit), `change-saga review
   refresh-coverage --review ID change.saga` re-pins what only moved. New lines are reported uncovered,
   with the one Item covering their file as the proposed owner: read them,
   and only if that Item's explanation covers them accept with the printed
   `review refresh-coverage --review ID --accept-proposed --path P
   [--note TEXT]`. It lists stale references (an edit landed inside) with a
   proposed range and diff: read each, then accept with the printed
   `change-saga repin --accept-proposed --record FILE --reference N
   change.saga`, or revise the explanation if it no longer holds.

Coverage is an omission check, not proof that a slide is right. Never widen a
selector just to finish coverage.

A Saga of reviews alone is complete; do not pitch more. If the user asks to
document the application itself (personas, stories, features, design, living
documentation), `change-saga setup-initial-saga` guides growing this Saga.

## Mandatory contract

A Change Saga is Git-native: the recommended idiom is one Saga per
repository, `change.saga` at its root. It always holds the pull requests'
review decks. When a team grows it, it also documents the application: durable
product domains (features) and their requirements, design, quality, work, and
implementation explanation. A monorepo of several apps also keeps one
`change.saga` at its root and documents each app through its own features. A
pull request compares the Saga and code between commits; its review deck
explains that transition. Author the thing submitted for human review, not the
review verdict.

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
- Read real Saga metadata only through `change-saga query`, or read one slide
  compactly with `change-saga diagram describe`. Never glob, grep, or open
  metadata files to infer Saga state. Page every result to completion,
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
| Create a review for a PR or branch (the usual first task) | The fast path above, then [diagrams and evidence](references/diagrams.md) |
| Inspect or navigate an existing Saga; load compact feature context; resolve current heads, conflicts, evidence, or history | [Reading through the query API](references/query.md) |
| Audit whether one feature has a current, exact implementation handoff | [Reading through the query API](references/query.md) |
| Interview for a feature or draft candidate stories; author or revise personas, stories, acceptance criteria, citations, requirement relations, or lifecycle state | [Query](references/query.md), then [stories and provenance](references/stories.md) |
| Author or revise the overview, pitch, description, or project vocabulary | [Query](references/query.md), then [overview and terms](references/terms.md) |
| Author Component/System definitions, their interactions, or pinned Item documentation links | [Query](references/query.md), then [diagrams and evidence](references/diagrams.md) |
| Author diagrams, implementation/review decks, narrative fragments, landmarks, exact code evidence, or claims | [Query](references/query.md), then [diagrams and evidence](references/diagrams.md) |
| Render slides and run mechanical visual QA | [Diagrams and evidence](references/diagrams.md) |
| Theme the reviewer and its slides: brand colours, fonts, dark mode | [Theming](references/theme.md) |
| Reconcile a comparison, work with a companion repository, repin landed evidence, recover, or hand off work without changing visuals | [Query](references/query.md), then [integration and recovery](references/integration.md) |
| Compare parallel proposal branches or deliberately withdraw/consolidate a duplicate proposal | [Query](references/query.md), [integration](references/integration.md), and [stories](references/stories.md); use only capabilities confirmed by the installed CLI |
| Prepare or update a pull-request review artifact alongside living documentation, or change visuals while integrating | [Query](references/query.md), [integration](references/integration.md), and [diagrams](references/diagrams.md) |
| Define a CI acceptance rule | [CI rules](references/ci.md); add [query](references/query.md) only when inspecting real Saga state |
| Look up resource shapes, stable target identities, or command families | [Format quick reference](references/format.md), only when the CLI's `spec`, help, or query schema is insufficient |

The references describe current public contracts only. Do not infer a command
from a planned capability or another branch. Discover new CLI or query support
from the installed binary before using it.

## Work at the requested scope

Before drafting even a conversational candidate story for an existing Saga,
query its personas and relevant product context. Use the actual persona name
and stable identity, not a generic role invented from the conversation. Follow
the grounded interview workflow in [stories and provenance](references/stories.md).

For a new story, capture a persona-focused outcome and independent,
observable pass/fail criteria. A proposed story may remain criterion-free
while intent is uncertain; an accepted story needs at least one criterion. Add
only the narrowest obligation confirmed by the user.

For an existing code change, begin with its review deck (the fast path above)
and offer missing product context only as optional follow-up. For new
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
2. Confirm the Saga path; it is usually `change.saga` at the repository root.
   Create one with `change-saga init` (which creates `change.saga` by default)
   only when the repository has none and the requested work authorizes
   creation. If the repository already has a Saga, recommend extending it
   rather than adding another; a second Saga is allowed but not the idiom. Resolve the
   requested scope and, for comparisons, the verified base and head. Query
   current state through the API and retain its snapshot.
3. Use the routed reference and public commands to make the smallest complete
   change. Follow returned URNs and evidence record paths; do not reconstruct
   them from storage.
4. Run `change-saga validate --json <saga>` and the task-relevant bounded
   queries. Use `status --json` and its ordered `next_actions` as a work
   queue, not a verdict; for a Saga that holds only reviews they follow the
   review. Use `check --covers ...` only for the areas the user or team
   actually requires.
   Only when the Saga already holds living documentation (its `status --json`
   reports `growth.living_documentation`), or the user asks, then after the
   PR review deck, as part of creating or updating the PR, run
   `change-saga reconcile --against <base> --json <saga>`.
   It opens with "Your change made N references stale", each with its old and
   proposed range; read the diff and accept a proposal that still means the
   same with `change-saga repin --accept-proposed`, or revise the explanation.
   Debt that predates the change is a count (`--all` lists it); leave it to a
   separate upkeep pass unless asked.
   Inspect the queue, reassess affected living documentation, make justified
   repairs through typed public paths, then reconcile again. Review coverage
   is independent of HEAD documentation currency; retain baseline debt and
   uncertainty. A Saga of reviews alone needs none of this.
5. Stop when the requested outcome is complete. Offer unrequested growth as
   optional and never present a clean status as proof that the explanation is
   correct.

Opening a Saga does not authorize a review. When explicitly asked to review,
first inspect the code diff independently, then inspect the author's deck and
evidence, test claims independently, and finally record only your own review
seat's actions. The tool records per-slide decisions; it never declares that a
review is approved.
