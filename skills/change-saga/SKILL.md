---
name: change-saga
description: 'Author, update, validate, and open a Change Saga: the Git-native documentation and history of an application, organized into epics that carry prototypes, user stories and acceptance criteria, UX/UI and technical design, test cases, and implementation decks whose Items reference the exact code they explain. Drive the work with status --json next actions. The primary purpose is to create the artifact submitted for human review, not to perform the review; only conduct review actions when explicitly requested.'
---

# Change Saga

## Purpose and role boundary

A saga is the authored change proposal: the next-generation PR body submitted
alongside the code. It explains and demonstrates what changed, why, how it
behaves, and where every part is implemented. It is the thing to be reviewed,
not the review itself.

During authoring:

- speak as the change author and guide, not as an independent reviewer;
- create no review comments, approvals, rejections, or findings;
- document known risks, limitations, and tradeoffs as part of the proposal
  without turning them into review verdicts;
- optimize for a human reviewer to understand and inspect the change over time.

Optimize for reviewer understanding and information gain, not exhaustive
retelling. Establish enough of the surrounding system that a reviewer can form
an accurate mental model, then spend the deck's scarce attention on what may
violate that model: counterintuitive behavior, hidden coupling, consequential
constraints, intentional deviations from repository conventions, rejected
alternatives, and tradeoffs whose costs land elsewhere. For each such surprise,
show the reasonable expectation, what the code actually does, why, and the
consequence. Tie the actual behavior and consequence to exact evidence.
Surprises are especially good callout Items: attach the callout to the visual
element or transition that creates the surprise so the contrast remains in
context. Do not manufacture drama. If investigation finds no meaningful
deviation, say so and use the slides to teach system shape, risk boundaries,
and proof instead.

Only enter reviewer mode when the user explicitly asks to conduct the review of
a pull request that has a review.

Use the `change-saga` CLI as the source of truth for format validity and diff coverage.
Treat completeness as an omission check, not proof that the authored proposal
is good. Prefer showing behavior and relationships over describing them in
dense prose.

Treat generated content as a first draft, even when using a frontier model.
After the evidence and structure are correct, perform a separate editorial
pass: make explanations concise, direct, and factual; remove repetition and
vague framing; and revise each slide until it communicates one coherent idea.
Expect iteration rather than assuming the first complete draft is ready for
review.

The structured directory format is intentionally friendly to parallel
development. When work is parallelized, partition ownership along independent
chapters and embedded deck bundles, and let each lane add its own slides, Items,
evidence, claims, verifications, and review records. Avoid aggregating unrelated
work into shared files; merge the lanes before the final coverage and validation
passes. This localizes Git conflicts but does not make parallel edits
conflict-free.

## One Saga, from the big work to the big work

A Change Saga is the record of one big change: the kind that warrants product
requirements, UX and UI design, technical design, quality verification, and an
implementation walkthrough. There is one kind of Saga. It always has the same
four parts, in the same order:

- **Product**: prototypes, and user stories with acceptance criteria;
- **Design**: UX flows, UI references, and technical design;
- **Quality**: test cases that verify the acceptance criteria;
- **Implementation**: the deck that explains the change, whose Items own the
  exact diffs.

The only hard requirement is that code maps back to user stories. Designs,
specifications, and test cases map to stories, so code reaches a story
transitively through them. Do not author code-to-story links by hand when a
design or test case can carry the path. Every relation pins the revision it
relied on, so a story revision makes its dependents visibly stale.

This is not a waterfall. Prototypes and stories may come in either order and
iterate together; design starts while they mature; a discovery during
implementation becomes an explicit new revision of the story it changes,
preserving history rather than rewriting it. When the implementation already
exists, build the Saga the same way: the product intent and design are what the
reviewer needs in order to judge the code, so recover them from the source
material and the user rather than skipping them.

Parallel authoring is a core property of the document. Partition ownership by
stable stories, prototypes, design fragments, test cases, work items, and deck
bundles so agents can fan out and merge their Saga changes alongside the code.
Consolidate the lanes before the final status and validation passes.

## Drive the work with status

`change-saga status --json <saga>` is the work queue. It reports the readiness
gates, each accepted criterion's coverage on the prototype, UX, UI, technical,
quality, and implementation axes, the stale set with pinned and current
revisions, changed-source accounting, and ordered `next_actions`. Loop:

1. run `change-saga spec --json` once to learn the resources, legal relations,
   and command shapes;
2. run `status --json` and take the first next action;
3. a `command` action carries a valid command shape: fill in its author inputs
   and run it; a `question` action needs product judgment, external access, or
   an explicit exclusion: ask the user its one question and run the command for
   their answer;
4. run `validate`, then repeat from step 2.

Stop when no required gap remains. A clean status proves nothing is missing or
stale; it never proves the Saga is good. Status never reduces coverage to a
score, and neither should you.

## Locate the CLI

Prefer an installed `change-saga` executable. In the Change Saga source repository, use
`go run ./cmd/change-saga` when the executable is unavailable. Keep one invocation form
for the whole task.

Read [references/format.md](references/format.md) before changing saga files.
When authoring a new saga, also read
[references/authoring.md](references/authoring.md). Run `change-saga spec` if the
installed CLI disagrees with the references; follow the CLI and report the
mismatch.

## Read a saga through the query API

Never glob, grep, or read saga metadata files to learn what a saga contains.
The on-disk layout is an implementation detail with no compatibility promise.
Use `change-saga query`, the versioned read API, for every read of an existing
saga during both authoring and review. It is deterministic and paginated, never
starts the server, and never mutates either repository.

The operations are `schema`, `overview`, `children`, `fragment`, `fragment-diffs`, `slide`, `slide-diffs`,
`diff-owners`, `gaps`, `mappings`, `claims`, `verifications`, `requirements`,
`relations`, `traceability`, `layers`, and `history`. Start at `query overview`, walk one level
at a time with `query children`, read narrative content through `query
fragment`, navigate evidence in both directions with `query fragment-diffs` and
`query diff-owners`, read a comparison's Changed, Affected, and Code layers with
`query layers --against REV`, trace a record's history with `query history
--node URN`, and page
completeness problems with `query gaps --kind uncovered|stale|overlap`.
Use `query mappings --sort scrutiny` to find coverage records whose breadth or
thin justification deserves the most skepticism. Use `query claims` and
`query verifications` to inspect falsifiable author assertions and their
append-only verification history.

Pass `--saga <path>` to every query, and `--repo <source-checkout>` when the
source repository is separate. The one exception is `change-saga query schema
<operation>`, which describes that operation's data paths and pagination
contract without opening a saga. Use it instead of probing or guessing response
shapes. Each invocation writes exactly one JSON envelope with `schema`, `ok`,
`snapshot`, `data`, and `page`; failures carry `error.code`. Branch on `ok` and
`error.code`; never parse message text. For cursor-paginated operations, the
current page length at `pagination.counted_path` must equal `page.returned`.
Follow `page.next_cursor` while `page.has_more` is true, and confirm the
aggregate count equals `page.total`. Do not raise `--limit` to silently swallow a partial
result. `query
children` on a fragment lists its landmarks with the target URNs to pass to
`change-saga cover --target`.
Hierarchy nodes report both direct and descendant diff counts. Treat
`diffs.current` and `diffs.stale` as inclusive totals; use `direct_current`,
`direct_stale`, `descendant_current`, and `descendant_stale` when deciding
whether evidence belongs to the node itself or to one of its landmarks or
children.

## Author a saga

Build the Product first, since everything else traces to it. Use `prototype
add-html` or `prototype add-external` for interactive prototypes and
`prototype annotate` to pin them to the stories and criteria they clarify.
Use `story add`, `story revise`, and the `criterion` commands for sourced user
stories with explicit acceptance criteria, and `citation add` to record where a
story or decision came from. Develop technical design with the `design`
commands, test cases with the `quality` commands, and connect resources with
`relation add`: a test case `verifies` a criterion, and a design or deck target
addresses or explains one. Pin the current revision on every relation.

The implementation deck is the core of the Saga. Its authoring spine is `Deck →
Slide → Item`: use `add-deck`, `add-slide`, `set-slide-content`, and `add-item`.
Never turn report fragments, stories, or design into slides; the deck explains
the implemented change. A slide is one 16:9 visual composition with one
takeaway. An Item is a meaningful node, edge, region, transition, statement,
risk, metric, example, or callout. A callout may point at another Item with
`--about`, and it may own diff evidence itself. Put every non-decorative Item
in `reading_order`; `add-item` does this automatically. Attach every exact diff
atom to the narrowest Item; slide evidence is owned only by Items. Use `query
slide` and `query slide-diffs` to read it back. The Saga is documentation: its
decks carry no approvals and no comments. Review happens only in a pull
request's review (see below).

## Author a pull request's review

A review is a pull request's slide deck: one review per pull request, viewed
from the merge-base of its base through its head, and following the head as
commits are pushed. Create it with `change-saga review create --id pr-<n> --pr
<n> --url <url> --base <branch it merges into> --head <its branch>`, and author
its deck with `add-slide --review`, `set-slide-content --review`, `add-item
--review`, and `cover --against <base> --target <review Item URN>`.

The review deck explains what the change did and why: the transition and its
reasoning (why the queue moved from SQS to a Postgres table), which the current
documentation no longer shows. Its Items reference the code the change touched,
shown as a diff against the review's base, and may reference the records it
revised with `--record` (a story, an epic slide) so a reviewer can open them
beside the change. Review decks never count toward coverage; each epic's
implementation deck still explains the current code. After the change lands,
`change-saga repin --onto <landed commit> --branch <branch>` freezes the review
at its exact base and head.

Before handoff, page `query traceability`; use `--diff` for exact reverse
lookup or `--commit` for the resolved head commit of a committed comparison,
and resolve every entry in `data.unlinked_code_evidence`.

Apply the density and composition checks in
[references/authoring.md](references/authoring.md). If the change cannot be
explained without a wall of text or more than seven semantic Items on one
standard slide, split the visual argument across slides rather than shrinking
it.

1. Resolve the request to an exact source comparison. For a PR number or URL,
   use the available hosting integration or CLI to obtain its title, URL, base,
   and head, and ensure the head is available locally. Verify the returned head
   branch/OID and changed-file summary describe the checkout you are about to
   explain. Never guess a PR number from nearby context, and omit PR identity
   rather than recording one that cannot be verified. Never infer the base from
   the default branch when PR metadata is available.
2. Inspect the PR description, commit/file summary, full diff, tests, and any
   existing `.saga`. Do not modify product code while authoring unless asked.
3. Initialize the saga when none exists:

   ```sh
   change-saga init --title "<title>" <name>.saga
   ```

   Comparisons are between commits, so commit in-progress work before covering
   it; uncommitted changes are not part of any comparison. Then build the
   Product, Design, and Quality parts, following status next actions, before
   storyboarding the deck.
4. Page `change-saga query gaps --kind uncovered --saga <name>.saga` as the
   coverage work queue. Query `gaps --kind stale` for reconciliation work and
   `gaps --kind overlap` for mappings that need justification. Preserve the
   returned snapshot across the loop and restart if it changes unexpectedly.
5. Read the relevant code and diff context. Storyboard the implementation deck as
   a sequence of slides grouped by reviewer intent—architecture, request
   flow, state transition, migration, operational risk, or proof—not by source
   directory. For every planned slide, write one intent and one takeaway before
   choosing its visual grammar. Build a surprise inventory first: note what a
   reasonable reviewer would expect from nearby code or documented behavior,
   where this change differs, why it differs, and what that choice costs or
   enables. Use system-model slides to make those deviations intelligible.
6. Create the deck with `add-deck` and slides with `add-slide`. Choose a purpose-fit
   system-context, architecture, data-flow, sequence, state, entity,
   decision-logic, comparison, failure, or evidence composition. Use
   `set-slide-content` to install a self-contained SVG, raster image, or
   sandboxed HTML entrypoint. Do not use prose as the slide or repeat one generic
   card-grid silhouette across unrelated reviewer questions.
7. Enumerate every meaningful visual node, edge, region, transition, statement,
   risk, metric, example, and callout with `add-item`. Keep 1–7 primary Items per
   slide, include every non-decorative Item in `reading_order`, and give each a
   semantic description that stands without the picture. Items are the
   precise evidence units.
8. Reference only the exact code each Item explains with `change-saga cover
   --target`. A reference pins lines at a commit with a digest of their
   content. Always provide a concise reviewer-facing note. `--side new --lines`
   pins added lines at the comparison's head, `--side old --lines` pins deleted
   lines at its merge-base, and `--file` references a whole file for renames,
   mode, and binary changes. Prefer the locations returned by `query gaps`.
   Batch authoring may reduce calls, but it never justifies widened references
   or slide-level ownership.
9. Run `query mappings --sort scrutiny` and use `replace-coverage` or
   `remove-coverage` to repair broad or misplaced ownership. References follow
   their code: when later commits only move the referenced lines, they are
   remapped automatically. When the lines themselves change, the reference is
   stale; `change-saga references --stale --diff` shows why, and
   `replace-coverage` re-authors it. After the change lands, `change-saga repin
   --onto <landed commit> --branch <branch>` re-pins references to the landed
   commit and records the branch's commit messages; run it before the branch is
   deleted.
10. Record falsifiable assertions with `add-claim` against the exact Item making
    them, and append reproducible results with `verify-claim`. Claims never
    contribute to coverage, and prose confidence is not verification.
11. Repeat all three gap views until no product atom is uncovered, no reference
    is stale, and every overlap has a defensible reviewer reason.
12. Run `validate --json` and `status --json`, then perform the visual,
    accessibility, relationship-silhouette, surprise, and contact-sheet audits in
    `references/authoring.md`. A structurally valid deck that still makes the
    reviewer read paragraphs or decode decorative diagrams is not ready.

Never make a reference wider merely to reach 100%. If an atom does not fit the
current story, improve the structure or call out the unexplained change.

## Reconcile an evolving change

Before editing an existing Saga for a newly merged or proposed change, derive
the maintenance work queue from source evidence rather than comparing authored
content:

```sh
change-saga status --json --repo <source-checkout> \
  --against <incoming-base> --head <incoming-head> <maintained.saga>
change-saga query layers --saga <maintained.saga> --repo <source-checkout> \
  --against <incoming-base> --head <incoming-head> --layer affected
```

The Affected layer lists the records the change invalidated: a removed or
destructively changed line under a record's references means that record must
be revisited; a new line beside referenced code is a prompt to reconsider it;
changed code no record can own requires new or expanded documentation. Follow
the returned record URNs and evidence files; do not compare prose, SVG, HTML, or
other fragment bytes to infer impact.

Then handle both sides of drift:

- Re-author stale references with `replace-coverage` or remove them with
  `remove-coverage`; never delete metadata files directly.
- Place newly uncovered atoms only after reading their current diff context.
- Update fragment content when behavior changed, even if an old reference
  still happens to remap cleanly.
- Commits that change only the Saga never move or stale a reference; code
  changes do, and require reconciliation.

## Open the authored saga for review

Run `change-saga open <name>.saga` when asked to present the Saga. Opening it
does not authorize the AI to review anything. Without `--against` it shows the
documentation as of the head; with `--against <base>` it shows the Changed,
Affected, and Code layers read-only beside the pull request's review. The
documentation has no comment or approval controls in either mode.
`change-saga open` starts a managed background reviewer and prints its PID and
URL. Discover it later with `change-saga serve status [SAGA]` and stop it with
`change-saga serve stop [SAGA]`. Use `change-saga serve --open` only when the
reviewer should remain attached to the current terminal.

When explicitly asked to review a pull request, first read the code diff
independently and record provisional findings; then inspect the review deck,
mappings, claims, verifications, and narrative intent; finally reconcile
contradictions and independently test author claims. Do not let the author's
explanation anchor the first correctness pass. Read the Saga through the query
API rather than its metadata files.

Decisions are per review slide: `change-saga review approve`, `review
request-changes` (say what should change), or `review withdraw`, each with
`--review` and `--slide`; discuss with `review comment` on a slide or Item.
Always declare the reviewer persona. Use `--reviewer-kind human` only for a
decision the human made directly. For your own AI review, use `--reviewer-kind
ai` together with an independent `--reviewer-name`, `--agent`, and the exact
`--model`; never turn an AI pass into a human approval. Give simultaneous passes
stable distinct names such as `Claude 1` and `Claude 2` even when their model
is identical. A decision records the pull request head it was given at and goes
out of date when the slide or the code it references changes; `change-saga
review list` and `status` report each one's currency. Never state that a review
is approved: the tool records decisions and the team decides what it requires.
Do not record decisions or comments on a person's behalf without an explicit
request to conduct the review.
