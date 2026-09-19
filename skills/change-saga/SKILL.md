---
name: change-saga
description: 'Author, update, validate, and open a Change Saga: the Git-native documentation of an application, organized into durable epics that carry prototypes, user stories and acceptance criteria, UX/UI and technical design, test cases, and implementation decks whose Items reference the exact code they explain, plus one review deck per pull request. Drive the work with status --json next actions. A first change is asked only for implementation coverage; everything else grows over time. The primary purpose is to author what is submitted for human review, not to perform the review; only conduct review actions when explicitly requested.'
---

# Change Saga

## Purpose and role boundary

A Change Saga is the documentation of an application, kept in Git beside its
code. A repository has one app Saga. A pull request is a comparison of that
Saga and its code between two commits, and its review is a slide deck that
explains what the change did and why. What you author is the thing to be
reviewed, not the review itself: the successor to a flat pull-request title
and description.

During authoring:

- speak as the change author and guide, not as an independent reviewer;
- do not record review decisions or comments: no approvals, change requests,
  withdrawals, review comments, or findings;
- document known risks, limitations, and tradeoffs as part of the proposal
  without turning them into review verdicts;
- optimize for a human reviewer to understand and inspect the change over time.

Only enter reviewer mode when the user explicitly asks you to conduct the
review of a pull request that has a review (see the last section).

Optimize for reviewer understanding and information gain, not exhaustive
retelling. Establish enough of the surrounding system that a reviewer can form
an accurate mental model, then spend the deck's scarce attention on what may
violate that model: counterintuitive behavior, hidden coupling, consequential
constraints, intentional deviations from repository conventions, rejected
alternatives, and tradeoffs whose costs land elsewhere. For each such surprise,
show the reasonable expectation, what the code actually does, why, and the
consequence, tied to exact evidence. Do not manufacture drama: if
investigation finds no meaningful deviation, say so and use the slides to
teach system shape, risk boundaries, and proof.

The installed `change-saga` CLI is the source of truth for commands, flags,
format validity, and coverage. Coverage is an omission check, not proof that
the explanation is good. Treat generated content as a first draft: once the
evidence and structure are right, make a separate editorial pass for concise,
direct, factual explanations with one coherent idea per slide.

## The app Saga

```text
app.saga/
  ___overview/  ___personas/  ___designsystem/  ___onboarding/  ___featureflags/
  ___epics/<epic>.epic/     # Product, Design, Quality, and Implementation
  ___reviews/<id>.review/   # one slide deck per pull request
```

- **App level.** The overview (the project's name, an elevator pitch, a
  description, and its terms and vocabulary), the personas the app serves, the
  design system, an onboarding deck, and feature flags.
- **Epics are durable product domains**, not changes. Each holds its own
  **Product** (prototypes, and user stories with acceptance criteria),
  **Design** (UX, UI, and technical design), **Quality** (test cases that
  verify the criteria), and **Implementation** (one living deck whose Items
  reference the code). Revising a story refines its domain in place; `story
  move` moves a story between epics without breaking a link. A pull request is
  not an epic: it may touch any number of them.
- **The chain is persona → story → design → code.** Declare only adjacent
  links: a story names its personas, a design or test case addresses or
  verifies a criterion, an Item references code. Longer paths are inferred, so
  do not author code-to-story links by hand when a design or test case can
  carry the path. Every relation pins the revision it relied on, so revising a
  story makes its dependents visibly stale.
- **The Saga is documentation.** Stories, designs, test cases, and decks carry
  no approvals and no comments. They describe the current state; the reason it
  changed lives in commit messages (a commit that changes a design says why)
  and in the pull request's review deck.
- **Code references are pinned at a commit**, with a digest of the referenced
  content; a diff is never stored. When later commits only move the lines, a
  reference is remapped automatically; when the lines change, it is stale.
  Commits that change only the Saga never move or stale a reference.
- **Observe or compare.** Without `--against`, a command observes the app at
  `--head` (default HEAD), with no changed lines to account for. With
  `--against REV` it compares the merge-base of REV and the head through the
  head, the way a pull request does. `saga.json` stores no comparison.

This is not a waterfall. Prototypes and stories may come in either order and
iterate together; design starts while they mature; a discovery during
implementation becomes an explicit new revision of the story it changes,
preserving history rather than rewriting it.

Parallel authoring is a core property of the format. Partition ownership by
epic, story, prototype, design fragment, test case, work item, and deck bundle
so agents can fan out and merge their Saga changes alongside the code, and
consolidate the lanes before the final status and validation passes. Avoid
aggregating unrelated work into shared files. This localizes Git conflicts; it
does not make parallel edits conflict-free.

## Grow the Saga incrementally

The one thing asked of a change is that its implementation covers it: every
changed line is referenced by an Item in the implementation deck of the epic
the change belongs to. That is the whole first run: initialize, add the epic,
cover the change, done.

Everything else is growth, not debt. Personas, stories, design, test cases,
the overview, and terms are never demanded up front, and their absence is not
failure. After the change is covered, offer the growth that `status` reports,
one contextual step at a time ("this change touched checkout; capture the
checkout story?"), with the one command that acts on the answer. Recover
product intent from the source material and the user; never invent a story,
criterion, persona, or definition the user did not give you.

What exists must stay healthy. Once a story is accepted or a design references
code, a change that makes that link stale or leaves its code uncovered needs
reconciling in the same change. Coverage only ratchets up.

## Drive the work with status

`change-saga status --json <saga>` is the work queue. It reports the readiness
gates, each accepted criterion's coverage on the prototype, UX, UI, technical,
quality, and implementation axes, the stale set with pinned and current
revisions, changed-source accounting, overview gaps, reviews, and ordered
`next_actions`. With `--against` it is scoped to one change. Loop:

1. run `change-saga spec --json` once to learn the resources, legal relations,
   and command shapes;
2. run `status --json` and take the first next action;
3. a `command` action carries a valid command shape: fill in its author inputs
   and run it; a `question` action needs product judgment, external access, or
   an explicit exclusion: ask the user its one question and run the command for
   their answer. An action in the `growth` category is an offer the user may
   decline;
4. run `validate`, then repeat from step 2.

Stop when no required gap remains. A clean status proves nothing is missing or
stale; it never proves the Saga is good. Status never reduces coverage to a
score, and neither should you.

## Locate the CLI

Prefer an installed `change-saga` executable. In the Change Saga source
repository, use `go run ./cmd/change-saga` when the executable is unavailable.
Keep one invocation form for the whole task. Begin with `change-saga --help`,
and consult each command's `-h` for exact flags.

Read [references/format.md](references/format.md) before changing Saga files and
[references/query.md](references/query.md) before reading one. When authoring
a deck or narrative content, also read
[references/authoring.md](references/authoring.md). If the installed CLI
disagrees with these references, follow the CLI and report the mismatch.

## Read a Saga through the query API

Never glob, grep, or read Saga metadata files to learn what a Saga contains.
The on-disk layout is an implementation detail with no compatibility promise.
Use `change-saga query`, the versioned read API, for every read during both
authoring and review. It is deterministic and paginated, never starts the
server, and never mutates either repository. The envelope contract and every
operation are in [references/query.md](references/query.md).

## Author a change

1. **Resolve the comparison** from the hosting provider's metadata, never
   from a guess, as described in
   [references/authoring.md](references/authoring.md): never infer the base
   from the default branch when PR metadata is available, and omit PR
   identity rather than record one you cannot verify. When asked to draft or
   prepare a pull request, keep the repository's existing PR-authoring
   processes, templates, issue context, and checks, and express the result in
   the Saga.
2. **Inspect** the PR description, commit/file summary, full diff, tests, and
   the existing Saga. Do not modify product code while authoring unless asked.
3. **Open the app Saga,** creating it only when the repository has none:

   ```sh
   change-saga init --title "<app name>" app.saga
   change-saga epic add --id <epic> --title "<product domain>" app.saga
   ```

   Put the change in the epic it belongs to, adding one only for a new product
   domain. Comparisons are between commits, so commit in-progress work before
   covering it; uncommitted changes are not part of any comparison.
4. **Page the coverage work queue** with `change-saga query gaps --kind
   uncovered --against <base> --saga app.saga`. Use `--kind stale` for
   reconciliation and `--kind overlap` for mappings that need justification.
   Preserve the returned snapshot across the loop and restart if it changes
   unexpectedly.
5. **Storyboard before creating slides.** Group slides by reviewer intent
   (architecture, request flow, state transition, migration, operational risk,
   or proof), not by source directory. Build a surprise inventory first, then
   write one reviewer question, intent, and takeaway per slide and choose the
   visual form that truthfully encodes its relationship. Follow the storyboard
   and visual-form guidance in [references/authoring.md](references/authoring.md).
6. **Build the deck.** An epic has one living implementation deck: when it
   already has one, update the slides the change affects instead of adding a
   deck per change. Create a deck with `change-saga add-deck --epic <epic>`,
   slides with `change-saga add-slide --deck <deck>`, and install a
   self-contained SVG, raster image, or sandboxed HTML entrypoint with
   `change-saga set-slide-content`. A slide is one 16:9 visual composition with
   one takeaway. Never turn stories, design, or narrative fragments into slides,
   and do not use prose as the slide.
7. **Enumerate Items** with `change-saga add-item`: every meaningful node,
   edge, region, transition, statement, risk, metric, example, and callout, with
   1–7 primary Items per standard slide and a semantic description that stands
   without the picture. `add-item` appends each to `reading_order`. A callout
   may point at another Item with `--about` and may own evidence itself. If the
   change cannot be explained without a wall of text or more than seven Items,
   split the argument across slides rather than shrinking it.
8. **Reference exactly the code each Item explains** with `change-saga cover
   --target <Item>`, always with a concise reviewer-facing `--note` saying what
   changed and why this Item owns it. `--side new --lines` pins added lines at
   the comparison's head; `--side old --lines` pins deleted lines at its
   merge-base; `--file` references a whole file for renames, mode, and binary
   changes; `--ref <commit>:<path>#L<start>-L<end>` names a location directly.
   When every changed line of one file belongs to the same Item, `--path FILE
   --changed-lines` references exactly those lines. Pipe many records to
   `cover --batch -`; the batch is resolved before anything is written. Use
   `--dry-run` to see which records an invocation would write. Deck- and
   slide-level coverage is refused: attach every changed line to the narrowest
   Item. Batching never justifies a wider reference.
9. **Repair ownership.** Run `change-saga query mappings --sort scrutiny` and
   use each `evidence_file` with `change-saga replace-coverage --record PATH`
   to split, retarget, or rewrite a record, or `change-saga remove-coverage` to
   delete one. The score is a work queue, not a grade. `change-saga references
   --stale --diff` shows why a reference went stale.
10. **Record falsifiable assertions** with `change-saga add-claim` against the
    Item making them, and append reproducible results with `change-saga
    verify-claim` (`unverified` when not checked). Claims never contribute to
    coverage, and prose confidence is not verification.
11. **Close the loop.** Repeat the three gap views until no changed line is
    uncovered, no reference is stale, and every overlap has a defensible
    reviewer reason. Page `query traceability` and offer a story for each
    entry in `data.unlinked_code_evidence`. Run `change-saga validate --json`
    and `change-saga status --json --against <base>`, then perform the audits
    in [references/authoring.md](references/authoring.md). A structurally
    valid deck that still makes the reviewer read paragraphs or decode
    decorative diagrams is not ready.

Never make a reference wider merely to reach 100%. If a changed line does not
fit the current story, improve the structure or call out the unexplained
change. Never leave generated instructions, blank scaffold fragments, or
example content in the handed-off Saga, and treat every validation warning as
an authoring task unless it is explicitly justified.

Product, Design, and Quality grow with the same commands the next actions
name: `prototype add-html`, `prototype add-external`, and `prototype annotate`
for prototypes; `story add`, `story revise`, and the `criterion` commands for
stories with explicit acceptance criteria; `citation add` for where a story or
decision came from; the `design` and `quality` commands; and `relation add`
to connect them (a test case `verifies` a criterion; a design or deck target
addresses or explains one). Narrative content (chapters, sections, fragments,
landmarks, and cited prose) follows the contracts in
[references/authoring.md](references/authoring.md).

## Keep the overview and the project's vocabulary current

The overview has four parts: the project's name (the Saga's title), an
elevator pitch (`change-saga overview set-pitch`), a description, a short
essay (`change-saga overview set-description`), and its terms and vocabulary.
Status lists each absent part under `overview.gaps`, and none ever blocks.

A term (`change-saga term add`) is a word the team says every day that a
newcomer cannot decode without digging through the code: a name, a
definition, aliases, the stories it belongs to (`--story`), other records it
names (`--record`), and the exact code that defines it (`--ref
HEAD:path#L12`), most often an enum value or a constant. Its code references
never count toward changed-line coverage; they are watched instead. From a
line of code, `query terms --ref <commit>:<path>#L<n>` and `query diff-owners`
return the terms it defines; from a story, `query terms --story <id>`.

- A rename makes the term's code reference stale, and a stale action names
  exactly that term with a prefilled `term revise`; supply the new `--ref`.
- In a comparison, an added enum value or typed constant that no term names
  becomes a growth action: "this looks like new terminology; define it?".
  Offer it with what the value appears to mean; never invent a definition, and
  never treat the suggestion as required.

## Reconcile an evolving change

Before editing the Saga for a newly merged or proposed change, derive the work
queue from source evidence rather than comparing authored content:

```sh
change-saga status --json --against <base> --head <head> app.saga
change-saga query layers --saga app.saga --against <base> --head <head> --layer affected
```

The comparison has three layers. **Changed** lists the Saga records added,
revised, or retired, each with its before and after. **Affected** lists the
records the change did not edit but invalidated: pinned to a revision that
changed, or referencing code that changed, followed up the persona, story,
design, and code chain. **Code** groups the changed hunks under the records
that reference them, plus every changed line nothing references. Update the
affected records, then cover the unreferenced lines. Follow the returned
record URNs and evidence files; do not compare prose, SVG, HTML, or other
fragment bytes to infer impact. Use `query history --node <urn>` to see when a
record was introduced, what it replaced, and the comparisons that changed it.

- Re-author stale references with `replace-coverage` or remove them with
  `remove-coverage`; never delete metadata files directly.
- Place newly uncovered lines only after reading their current diff context.
- Update slide and fragment content when behavior changed, even if an old
  reference still happens to remap cleanly.
- After the change lands, `change-saga repin --onto <landed commit> --branch
  <branch>` re-pins references to the landed commit, records the branch's
  commit messages, and freezes the pull request's review. Run it before the
  branch is deleted.

When the Saga lives in a companion repository, pass the code checkout with
`--repo` on every command, and move the sync cursor with `change-saga sync
--repo <checkout>` in every Saga commit that updates the documentation.

## Author a pull request's review

A review is a pull request's slide deck: one review per pull request, viewed
from the merge-base of its base through its head, and following the head as
commits are pushed. Create it and author its deck:

```sh
change-saga review create --id pr-<n> --pr <n> --url <url> --base <branch it merges into> --head <its branch> app.saga
change-saga add-slide --review pr-<n> --intent explain --layout diagram --title "<title>" app.saga <slide>
change-saga set-slide-content --review pr-<n> --target <slide> --source slide.svg app.saga
change-saga add-item --review pr-<n> --slide <slide> --kind node --id <item> --element-id <item> --description "<meaning>" app.saga
change-saga cover --target <review Item URN> --path <file> --changed-lines --note "<what and why>" app.saga
```

The review deck explains what the change did and why: the transition and its
reasoning (why the queue moved from SQS to a Postgres table), which the current
documentation no longer shows. Its Items reference the code the change touched,
shown as a diff against the review's base, and may name a record it revised
with `add-item --record` (a story, an epic slide) so a reviewer can open it
beside the change.

A review deck must account for its change: every changed line of the review's
range is covered by a review Item. Without `--against`, `cover` on a review
Item compares the review's own range. `change-saga review list` reports each
review's coverage, and `review list --uncovered` lists only the gaps as
ready-to-use locations. Review decks never count toward the documentation's
coverage: each epic's implementation deck still explains the current code.

## Open the Saga for review

Run `change-saga open app.saga` when asked to present the Saga. Opening it does
not authorize you to review anything. Without `--against` it observes the app
at the head, with stale references shown as health warnings; with `--against
<base>` it shows the Changed, Affected, and Code layers read-only beside the
pull request's review. The documentation has no comment or approval controls in
either mode. `change-saga open` starts a managed background reviewer and prints
its PID and URL; inspect or stop it with `change-saga serve status` and
`change-saga serve stop`. Use `change-saga serve --open` only when the reviewer
should remain attached to the current terminal.

## Review a pull request, only when asked

When explicitly asked to review a pull request, first read the code diff
independently and record provisional findings; then inspect the review deck,
mappings, claims, verifications, and narrative intent; finally reconcile
contradictions and independently test author claims. Do not let the author's
explanation anchor the first correctness pass. Read the Saga through the query
API rather than its metadata files.

Decisions are per review slide: `change-saga review approve`, `change-saga
review request-changes` (say what should change), or `change-saga review
withdraw`, each with `--review` and `--slide`; discuss with `change-saga review
comment` on a slide or Item. Always declare the reviewer persona. Use
`--reviewer-kind human` only for a decision the human made directly. For your
own review, use `--reviewer-kind ai` together with an independent
`--reviewer-name`, `--agent`, and the exact `--model`; never turn an AI pass
into a human approval. Give simultaneous passes stable distinct names such as
`Claude 1` and `Claude 2` even when their model is identical. One persona's
later decision supersedes only that same persona's earlier one. A decision
records the pull request head it was given at and goes out of date when the
slide or the code it references changes; `review list` and `status` report
each one's currency. Never state that a review is approved: the tool records
decisions and the team decides what it requires. Never record a decision or
comment on a person's behalf.
