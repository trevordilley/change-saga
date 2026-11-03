---
name: change-saga
description: 'Create a review for a pull request or branch ("create a review for this PR", "change-saga this PR"), meant for the big change a person reviews as a whole: a slide deck explaining its architecture, with every changed line linked to the slide Item that explains it. Also author, update, inspect, validate, and open Change Saga product, technical-design whiteboards, quality, technical decks, and living documentation linked to exact code when asked. Use for a requested slice without expanding it into unrelated authoring; conduct review actions only when explicitly requested.'
---

# Change Saga

Change Saga makes large, often AI-written changes easy to digest: a review deck of slides
explains a pull request's architecture, so a person reviews that instead of every line, with
every changed line linked to the slide Item that explains it. A Saga is Git-native, by idiom
one `change.saga` at the repository root, also for a monorepo, documenting each app through
its features. It holds review decks and, when a team grows it, living documentation of
features, requirements, design, quality, work, and implementation.

## Mandatory contract

These apply to every task ([full wording](references/contract.md)).

- **Scope.** Do not turn one story, term, diagram, or review into a whole-lifecycle project;
  if that is requested, do not stop after its easiest part. Offer growth as optional. A Saga
  of reviews alone is complete.
- **Your seat.** Author what is submitted for review, not its verdict. Record approvals, change
  requests, withdrawals, or comments only when asked to review. Never act for another person.
- **The CLI is the source of truth.** Discover with `--help`, `-h`, and `change-saga query
  schema <operation>`; never guess a command, response shape, or capability. If a reference
  disagrees with the installed CLI, follow the CLI and report the mismatch.
- **Reads.** Read metadata only through `change-saga query`, `change-saga diagram describe`,
  or `change-saga whiteboard describe --id ID`, never by globbing, grepping, or opening its
  files. Page to completion, keep one snapshot across a read, and restart if it changes.
- **Writes.** Mutate only through public CLI commands, which also own narrative and visual
  source files. Never edit, invent, rename, or delete metadata files. Reuse returned URNs.
- **Delete what should not exist;** Git keeps the history. Run `remove --dry-run`, show its
  summary, confirm a removal beyond what was asked, then fix or report each adjustment.
- **History.** Revisions and lifecycle events are immutable; decisions, comments, claims,
  verifications, and merge evidence are append-only. Preserve competing heads and conflicts;
  never fabricate a winner. Reconcile only with explicit parents and the user's intent.
- **Provenance.** Keep citations, relation pins and rationales, review and comparison identity,
  digests, and lifecycle reasons. Omit what you cannot verify.
- **Exact evidence.** Put code references on the narrowest Item, landmark, or test case that
  explains them. Never widen a selector to reach coverage. Implementation and review decks
  have different coverage roles; neither substitutes for the other.
- **Coverage is an omission check, not proof.** Never invent personas, stories, criteria,
  definitions, designs, test results, or novelty to fill a gap, or present a clean status as
  proof. Report uncertainty.
- **Timing.** Read Saga context whenever it can inform the work (`change-saga-context`). Author
  near the end: when the user says to create or update the PR, or asks for Saga work. Do not
  end a coding session with upkeep nobody asked for. A Plan is the exception, written before
  the code. Only the pull request reviewed as a whole gets a deck; as a child workspace or
  sub-agent, do Saga work only when the user or parent asks.
- **Small output.** Every printed byte is paid for on each later turn. Stay on compact defaults
  and expand one thing: `status --json` (`--action ID`, `--max N`; `--detail` is megabytes);
  `check --covers AREA --max 0`; `spec --json` (`--command NAME`, `--resource KIND`, `--section
  NAME`; `--all` is huge); `query requirements` (`--requirement ID`); `query context --feature
  ID` (`--expand STORY`); `reconcile --against BASE` as text or `--json --summary`.

## Route the task

Do not preload every reference; combine rows when a task crosses them.

| Requested work | Read before acting |
| --- | --- |
| Create a review for a PR or branch (the usual first task) | [review](references/review.md), [slides](references/slides.md), [diagram source](references/diagram-source.md) |
| Inspect a Saga; load compact feature context; resolve heads, conflicts, history; audit whether one feature has an exact handoff | [query](references/query.md) |
| Rename, move, reparent, reorder, remove, archive; assign a whiteboard to a Technical Design role | [query](references/query.md), [reorganize](references/reorganize.md) |
| Describe implemented features; Interview for a feature or draft candidate stories; author or revise personas, stories, criteria, citations, relations, lifecycle | [query](references/query.md), [stories](references/stories.md) |
| Roadmap features, priorities, labels | [query](references/query.md), [roadmap](references/roadmap.md) |
| Author or revise the overview, pitch, description, Essays, or vocabulary | [query](references/query.md), [terms](references/terms.md) |
| Technical design: approach essay and its whiteboards | [query](references/query.md), [whiteboards](references/whiteboards.md), [diagram source](references/diagram-source.md), [architecture](references/architecture.md) for that board |
| Components, Systems, data entities, ERD records, pinned Item documentation links | [query](references/query.md), [inventory](references/inventory.md) |
| Author diagrams and slides of decks, with exact code evidence | [query](references/query.md), [slides](references/slides.md), [diagram source](references/diagram-source.md), [evidence](references/evidence.md) |
| Deck Essays, fragments, landmarks; repair code evidence; claims | [query](references/query.md), [decks](references/decks.md), [evidence](references/evidence.md) |
| Render slides and run mechanical visual QA | [slides](references/slides.md) |
| Quality: test cases, policies | [query](references/query.md), [quality](references/quality.md) |
| Deprecations | [deprecations](references/deprecations.md) |
| Product themes | [themes](references/feature-themes.md) |
| Theme colours, fonts | [theming](references/theme.md) |
| Plan work before building it; Tasks | [plans](references/plans.md), [slides](references/slides.md) for its deck |
| Reconcile a comparison, companion repository, repin, recover, hand off | [query](references/query.md), [integration](references/integration.md) |
| Compare parallel proposal branches; consolidate a duplicate | [query](references/query.md), [integration](references/integration.md), [stories](references/stories.md) |
| Prepare or update a pull-request review artifact with living documentation | [query](references/query.md), [integration](references/integration.md), [slides](references/slides.md) |
| Conduct a review when explicitly asked | [review](references/review.md), [integration](references/integration.md) |
| Define a CI acceptance rule | [ci](references/ci.md); [query](references/query.md) for real state |
| Resource shapes, target identities | [format](references/format.md) |

## Common workflow

1. Prefer an installed `change-saga` (here, `go run ./cmd/change-saga`). Run `change-saga init`
   only when no Saga exists and the work authorizes it; extend an existing one.
2. Resolve the scope and any verified base and head. Make the smallest complete change.
3. Run `change-saga validate --json <saga>`. `status --json` and its `next_actions` are a work
   queue, not a verdict; use `check --covers` only for areas required.
4. Only when the Saga already holds living documentation, or the user asks, reconcile as part
   of creating or updating the PR ([integration](references/integration.md)).
5. Stop when the requested outcome is complete.
