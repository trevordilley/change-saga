# Create a review

Read this reference to create or update a pull request's review deck, or to conduct
a review when explicitly asked.

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

## When to do it

Creating or refreshing the review deck, covering lines, reconciling living
documentation, and repinning all happen when the user says to create or update the
PR ("okay, create a PR", "let's update the PR"), or when their process otherwise
publishes changes for review. Then do it all together, so the deck explains the
change as reviewers will see it. Otherwise, keep a coding session about the code: do
not end it with Saga upkeep nobody asked for. A deck that lags the branch between PR
updates is expected. When the user asks for Saga work directly, do it then.

For an existing code change that the user asks to review, begin with its review
deck and offer missing product context only as optional follow-up. Never manufacture
product intent to make coverage complete.

If a user asks to document features while iterating on a PR deck, follow the
feature authoring workflow at that scope in the same Saga ([stories](stories.md)).
Keep the PR deck focused on the change and connect it to the feature records where
relevant; do not require a repository-wide setup interview for one feature.

## Create a review for this PR (the usual first task)

When asked to "create a review for this PR" (or to sagafy a pull request or
branch), do this. Read [slides](slides.md) before drawing slides and
[the diagram source grammar](diagram-source.md) while writing them; nothing else
is needed.

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
   reviewer question, never one per file. Read
   [visual storytelling](visual-storytelling.md): connect the user
   outcome, ownership, mechanism and consequences into an argument. Use enough
   slides to explain the change; do not compress a broad review into a fixed count.
   Make named sections, semantic color and useful on-canvas explanations show
   where things fit. Finish and inspect one representative visual before scaling
   out; reuse its visual language rather than copying its layout onto every slide.
4. For each slide, write one request: a `"diagram"` source (explicitly
   positioned nodes, edges, and groups the CLI renders to SVG) and an Item for
   each element that explains part of the change. Publish it with `change-saga
   apply-slide --review ID --from SLIDE.json change.saga` (`--dry-run` first).
   The request's shape and an example are under "Review deck slides" in
   [slides](slides.md); revise a slide with
   `diagram edit`. Give an element a short Markdown `note` when a reader
   would want the why its label cannot hold (an edge's protocol or failure
   behavior, a node's responsibility); readers open it from an info control revealed by element hover or focus. Surprises
   still go in callout Items, and a note is never padding: omit it when there
   is nothing to add. Hand-write SVG only for what a diagram source cannot
   express, through `add-slide --review`, `set-slide-content --review`, and
   `add-item --review`.
5. Call out every surprise with a `"kind": "callout"` Item in that slide's
   request: `"about"` names the Item for the responsible element, and
   `"body"` (at most 240 characters) states what a reviewer would expect,
   what the change does instead, why, and the consequence. Cover, from the
   callout itself, the code that decides the behavior it states (where the
   outcome is determined, such as a validation that rejects the input
   first), not just nearby code. Reviewers see
   callouts through compact warning controls at the responsible element’s
   bottom-left, with details on hover, focus, or tap. Never manufacture one: if
   nothing surprises, say so in the concluding slide's takeaway.
6. Cover every changed line from the narrowest Item that explains it:
   `change-saga cover --target ITEM_URN --path PATH --changed-lines
   change.saga`, or `--lines RANGES --side new` for part of a file
   (`--side old` for deleted lines). On a review Item, cover compares the
   review's own range. `change-saga query children --saga change.saga
   --parent REVIEW_URN` lists the review's slides and Items.
7. Confirm: `change-saga review list --uncovered change.saga` names what no
   Item explains yet, and `change-saga review list --review ID change.saga`
   lists the surprises called out, marking any with no code evidence; `change-saga check --covers review --against BASE
   change.saga` exits 0 once every changed line is explained. Run
   `change-saga diagram check --review ID change.saga` and `change-saga
   validate change.saga`. Run `change-saga visual-qa --review ID change.saga`
   and inspect the rendered slides and contact sheet: readable text, meaningful
   section boundaries, explicit flows, visible consequences and a coherent story.
   Mechanical success does not establish that the slides explain the change.
8. Check every sentence; this is the one check the tool cannot do. Have a
   fresh-context reader, ideally a sub-agent that did not write the deck,
   read each slide's `diagram describe --review ID --slide SLIDE`: every
   takeaway, Item description, callout body, edge description, and note.
   For each sentence it opens the code the Item lists and the code the
   claim depends on (what runs before or after, validation, error paths),
   and flags any sentence that is false, overstated ("each artifact" when
   one is connected), or unverifiable. Revise with `diagram edit` or
   `apply-slide`. describe lists each Item's `path:start-end`, so this is
   cheap.
9. Commit the Saga with the change. Reviewers can collapse `change.saga` in
   the pull request's file tree and open the deck with `change-saga open
   change.saga`.
10. Each time the PR is updated (not after every commit), `change-saga review
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

To select automated tests for a review, use `change-saga review test-plan
--review ID --json change.saga` (with `--repo PATH` for a separate source
checkout). It follows changed code to affected stories and selects all their
linked automated tests, deduplicated across stories. Inspect `gaps` before
using `commands`; exit 3 means the plan has missing or uncertain mappings.
Commands are recorded invocations, not guessed or executed by the planner.

A Saga of reviews alone is complete; do not pitch more. If the user asks to
document the application itself (personas, stories, features, design, living
documentation), `change-saga setup-initial-saga` guides growing this Saga.

## Update living documentation with the PR

Only when the Saga already holds living documentation (its `status --json` reports
`documentation.living_documentation`), or the user asks: after the PR review deck, as
part of creating or updating the PR, run `change-saga reconcile --against BASE
change.saga`. It opens with "Your change made N references stale", each with its old
and proposed range; read the diff and accept a proposal that still means the same
with `change-saga repin --accept-proposed`, or revise the explanation. Debt that
predates the change is a count (`--all` lists it); leave it to a separate upkeep pass
unless asked. Inspect the queue, reassess affected living documentation, make
justified repairs through typed public paths, then reconcile again. Review coverage
is independent of HEAD documentation currency; retain baseline debt and uncertainty.
A Saga of reviews alone needs none of this. See [integration](integration.md).

## Conduct a review when asked

Opening a Saga does not authorize a review. When explicitly asked to review, first
inspect the code diff independently, then inspect the author's deck and evidence,
test claims independently, and finally record only your own review seat's actions.
The tool records per-slide decisions; it never declares that a review is approved.
See "Pull-request review artifacts" in [integration](integration.md).
