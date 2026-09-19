# The app Saga

Status: agreed goals and execution plan. Supersedes the single-change framing
in [requirements-design-quality-lifecycle.md](requirements-design-quality-lifecycle.md)
where the two disagree.

## Why

An application evolves through many bodies of work. What makes requirements
valuable is having *all* of them, current, as the logic changes: stories are
added, refined, moved, and retired as the code changes. A Saga per change can't
do that, because a story outlives the change that introduced it. A folder of
independent Sagas can't either: a story revised by a later change would have to
be copied or referenced from outside, and "the current version of this story"
would stop being one answer.

So a repository has **one Saga that documents the application**, organized into
durable **epics**, and every change is a comparison of that one Saga and its
code between two commits.

## Goals

### 1. One app Saga

A repository has one `app.saga`. It holds material about the whole application
plus its epics:

```text
app.saga/
  saga.json            # identity and repository only
  ___overview/         # the elevator pitch for the app
  ___personas/         # structured records, like stories
  ___designsystem/     # Figma links and references
  ___onboarding/       # a deck that gets people up to speed on the app
  ___featureflags/     # flags, and the epics and stories each one gates
  ___epics/<epic>.epic/
    <requirements, design, quality, and the implementation deck>
```

Other app-level material may join later. Candidates: architecture (system
diagram and data model, since every epic changes the same system), constraints
that apply everywhere (performance, security, accessibility), and a glossary.

### 2. Epics are durable product domains

An epic is an area of the product, not a change. It contains stories with
acceptance criteria, design, test cases, and its implementation deck.

- Revising a story from an older epic refines that domain in place. Nothing is
  renamed or copied.
- A story may move between epics. Story identity is independent of epic
  membership, so moving a story breaks no link; story IDs are unique across the
  app.
- A pull request is not an epic. It is a comparison that may touch any number
  of epics.

### 3. The traceability chain is persona → story → design → code

Every story applies to one or more personas. Only adjacent links are declared:
a story names its personas, a design addresses a story, an Item references code.
Every longer path — code to persona, persona to code — is **inferred**, never
authored.

Coverage is eventual and always visible: every persona is covered by a story,
every story by design, every design by code. Each link is present, missing, or
stale, and `status` reports which. Adding a persona creates a visible gap (no
story serves it). Removing one leaves the stories that served only that persona
as a single question: retire them, or reassign them.

### 4. Nodes reference code at a commit, not diffs

A reference says "this node explains these lines, as of this commit":

```text
repository, commit, path, line range, content digest
```

A diff is never stored. It is a way of viewing references against two commits.

- **Deletions** reference the base commit; renames and binary changes reference
  whole files.
- **Staleness** is a comparison of the pinned commit with the viewed one. If the
  lines only moved, the range is remapped automatically; if they changed, the
  reference is stale and the diff since the pin can be shown.
- **Surviving merges.** At merge, references are re-pinned to the commit that
  landed, so squash merges and deleted branches lose nothing. The content
  digest finds the same lines in any commit as a fallback.
- **Commits that only change the Saga** leave the code identical, so their
  remap is a no-op.

This replaces today's `saga-diff://` evidence, whose identity is a digest of an
entire diff and which therefore breaks wholesale on merge.

### 5. One Saga, two ways to open it

```sh
change-saga open app.saga                  # observe the app at HEAD
change-saga open app.saga --against main   # compare what this branch changes
```

**Observe** shows everything current: personas, epics, stories, designs, test
cases, and each node's code rendered as code. Stale references show as health
warnings.

**Compare** starts from the merge-base of the given commit and HEAD, the way a
pull request does, and highlights three layers:

1. **Changed:** Saga records added, revised, or retired, each with its before
   and after.
2. **Affected:** records the change did not edit but invalidated — anything
   pinned to a revision that changed, and any node whose referenced code
   changed. A code-only change still lights up the stories, designs, and
   personas it touches.
3. **Code:** diff hunks grouped under the nodes that reference them, plus every
   changed line no node references.

Everything else stays reachable but de-emphasized. `status` and `query` accept
the same `--against`, so an agent's work queue is scoped to one change.
`saga.json` holds no comparison; the comparison is how the Saga is opened.

### 6. The Saga is documentation; approval happens only in reviews

Stories, designs, test cases, and epic decks are documentation. They have no
approvals and no comments, whether the Saga is observed or compared. A review
may reference them, and that is where a change to them is discussed and
approved (goal 11).

### 7. Why things changed lives in Git, and compare mode surfaces it

The Saga records the current state, not the work of getting there. If a queue
moves from SQS to a Postgres table, the design and implementation slides now
show the table; the reason for the switch is not part of the state. It is
recovered from Git instead of being written into a new record type:

- **Replacements are paired.** When a slide, design, or test case drops out and
  another takes its place, compare mode shows them as a pair (the SQS diagram
  beside the Postgres table). The pairing is inferred from the graph, because
  both explain the same design or criterion. The author links them explicitly
  only when that inference is ambiguous.
- **Commit reasons sit beside what they changed.** The commit messages in the
  comparison are attached to the nodes whose records or referenced code each
  commit touched, not listed as a flat log. The authoring skill requires a
  commit that changes a design to state why.
- **Every node shows its history.** In observe mode a node shows when it was
  introduced and what it replaced, read from Git's log of its record files, with
  one step to open the comparison where it happened. A reader a year later can
  find the reason without knowing which range to compare.
- **Squash merges keep their reasons.** A squash merge collapses a branch's
  commits into one message. When references are re-pinned at merge, the
  branch's commit messages are recorded with the change so the finer reasoning
  survives.

Decision records (an ADR-like record type) are deferred. Phase 5 decides whether
they are needed: any reasoning in this repository's Sagas that has no home in
commits or pairing is the evidence for them.

### 8. Every changed line is accounted for, per change

In compare mode, every changed line must fall inside some reference's range.
This is computed from references, not stored. The app does not require every
line of the codebase to be owned: ownership accumulates as changes land.

### 9. Adoption is incremental

The first time someone tries Change Saga on a 30-file pull request, the first
thing that happens is not "define the personas of this app".

- **The one thing asked of a first change is that the implementation covers
  it.** Every changed line is referenced by the implementation deck. Personas,
  stories, design, and test cases are not asked for up front.
- **Everything else is growth, not debt.** Missing personas, stories, design, and
  quality are reported as opportunities. They never block a change, and
  readiness passing with nothing defined is correct: absence is not failure.
- **What exists must stay healthy.** Once a story is accepted or a design
  references code, a change that makes that link stale or leaves its code
  uncovered is flagged. Coverage only ratchets up.
- **The tool reports gaps; teams decide what to do about them.** `status` has no
  verdict. Implementation coverage is part of the report like everything else:
  for each area (changed lines covered by the implementation, the health of
  existing records, stories, persona coverage, design, quality) it reports how
  many are covered, with the lists of what is and is not. A gap is a finding,
  never a failure, and `status` exits zero. It exits non-zero only when it
  cannot produce a trustworthy report: a malformed Saga, duplicate IDs, or a
  checkout that does not match the declared repository. The report is readable
  as text and emitted as stable JSON; a team that wants a gap to fail its build
  writes that rule over the JSON in its own CI, and the docs teach common rules
  as recipes rather than flags. The JSON shape is therefore a contract. Counts
  and lists, never one blended score.
- **Asking a question is one command.** `change-saga check --against main
  --covers implementation,stories app.saga` answers whether the named areas are
  fully covered: exit zero if they are, non-zero with only those areas' gaps if
  not. Nothing is required unless someone asks. The areas follow the chain, so
  each is one more link:

  | Area | Covered when |
  | --- | --- |
  | `implementation` | every changed line is referenced by the deck |
  | `stories` | every changed line reaches a story through the chain |
  | `personas` | every changed line reaches a persona |
  | `design` | every story in scope has design |
  | `quality` | every acceptance criterion in scope has a test |
  | `health` | nothing that already existed went stale or broke |

  With `--against`, the scope is the change: what it changed and what it
  affected. Without it, the scope is the whole app. `--epic` narrows either.
- **Nothing is locked in.** The first change's deck goes into an epic the author
  names, defaulting to the pull request's title. Story, deck, and slide URNs
  carry no epic, so reorganizing later breaks nothing.
- **The Saga can live in the code repository or in a companion repository, and
  both are first-class.** A companion repository lets a team document a
  codebase, such as a client's, without landing a large documentation change in
  it first; the Saga can move into the code repository later without rewriting
  anything, because references name their repository.
- **The first run is: initialize, cover the change, done.** After that, the
  authoring agent offers, and never requires, to capture the stories the change
  implies.
- **Tools guide the Saga's growth and teach the practices.** Growth is a
  first-class part of the product, just never a gate. Guidance is contextual
  and arrives with the work, not as an up-front questionnaire: "this change
  touched checkout; capture the checkout story?", "these three stories serve
  someone you haven't named; define that persona?", "this design has no test
  case; add one?". Each suggestion explains the practice it teaches, why it
  pays off, and the one command that acts on it. Status reports growth
  opportunities separately from the one requirement, ordered by value to the
  current change, so an author can grow the Saga a step at a time or ignore it
  entirely.

### 10. Companion repositories

When the Saga lives in its own repository:

- Code references resolve against a checkout of the code repository passed with
  `--repo`, which is verified against the repository the Saga declares.
  Staleness and remapping use the code repository's history, never the Saga's.
- The Saga records a **sync cursor**: the code commit it currently documents.
  Every Saga commit that updates the documentation moves the cursor. In compare
  mode the code delta comes from the code repository, and the Saga delta from
  the Saga commit whose cursor matched the base. In the code repository the
  cursor is implicit, so both setups share one model.
- Documenting existing code needs no change at all: observe mode, with references
  pinned at the current code commit. Per-change coverage does not apply because
  there is no change; ownership of the existing code accumulates.
- Scale is the risk to measure. A whole-codebase Saga reached 230 MB and 17
  minutes under per-line diff evidence (see
  [large-saga-diagnosis.md](large-saga-diagnosis.md)). References are ranges,
  which should be far smaller, but a 400k-line codebase must be measured before
  it is promised.

### 11. A review is a pull request's slide deck

```text
app.saga/
  ___reviews/<id>.review/
    review.json      # the pull request, its base, and the head last reviewed
    <the review deck: slides, Items, and code references>
    <per-slide approvals and comments>
```

**A review is equivalent to a pull request.** Its base is the pull request's
base, and its head follows the pull request as commits are pushed. There is one
review per pull request.

**A review is always a slide deck.** It explains what the change did and why:
the transition and the reasoning (why the queue moved from SQS to a Postgres
table) that the current state no longer shows. It is the one part of a Saga that
speaks in diffs: its Items reference commits, and what a reviewer sees is a diff
because the review is viewed against its base. Its Items may also reference Saga
records, such as the story or epic slide a change revises, so the reviewer can
open the documentation beside the change.

**Approval is per slide, and only in reviews.** Each slide records its state
(approved, changes requested, or none) with the head commit it was given at, and
comments attach to review slides and Items. An approval is **out of date** when
the slide, or the code it references, changed between that commit and the pull
request's current head. That is how a review is known to be out of date for a
pull request: slide by slide, not as a whole.

**A review deck must account for its change.** Every changed line in the
review's range must be covered by the review deck's Items, exactly as Change
Saga has always required of the change it explains. Coverage is reported per
review (covered, uncovered, and stale lines, with the uncovered lines
listed) so a reviewer can be confident the deck explains every line, and the
reviewer shows uncovered lines beside the deck.

**The tool records; the team decides.** The tool never declares a review
approved. `status` and `check` report every slide's state and currency, and a
team decides what it requires (every slide approved, a human approval, no open
discussion) in its own process.

**After merge,** the review is history. A node's history links to the reviews
that changed it, so the reasoning behind today's state stays one step away.

This replaces the existing review overlay: `___approvals` on report targets and
`___review` threads on documentation move into reviews, and the reviewer drops
approval and comment controls from the documentation.

### 12. The overview, and the project's terminology

The overview is formal, not a single page. Its parts each appear in the
sidebar: the **project name**, the **elevator pitch**, a **description** (a
short essay), and **Terms and Vocabulary**.

Every project has its own language, and it is usually the least documented and
fastest to rot: a word the team says every day ("testtaker" for one sitting of
an assessment) is baffling to a newcomer until they dig through the code. A
**term** is a record with a name, a definition, and any aliases, and it
references what it names: the stories it belongs to, and the exact code that
defines it, most often an enum value or a constant. The links run both ways:
from a term to its code and stories, and from a line of code or a story to the
terms it defines.

Terms stay current through mechanisms that already exist. Renaming the enum
makes the term's code reference stale, which names exactly the term to update.
A comparison that adds an enum value or a constant no term references suggests,
as growth rather than a requirement, that the change may have introduced new
terminology to define. Recognizing enums and constants is a per-language
heuristic, and it only ever suggests.

### Kept from the current design

One format and no backwards compatibility. Staleness derived only from pins.
Readiness never reduced to a score. Ordered `next_actions`, each a command or
one focused question. The epic sidebar: Product, Design, Quality,
Implementation, with Implementation as the deck.

### Not goals

- A folder of independent Sagas collected at open time.
- Storing diffs as evidence.
- Approving documentation outside a change.
- Owning every line of the codebase at once.

## Decisions this settles

**One living deck per epic.** With references instead of diffs, an epic's
implementation deck explains the domain's *current* implementation and stays
valid as code changes: its Items remap, or go stale where the code really
changed. A pull request updates the slides it affects, and compare mode shows
which slides and Items changed and the diffs beneath them. A deck per change is
no longer needed to preserve history; the comparison between any two commits
reconstructs it.

**Personas belong to the app; stories belong to epics.**

## Open decisions

1. Do feature flags gate stories, epics, or both?
2. Are retired stories shown in the app view, or only in history?
3. What is the app-level sidebar above the epics? Partly settled: the overview
   expands to name, elevator pitch, description, and Terms and Vocabulary.
4. Onboarding deck Items point at records (personas, epics, stories) rather than
   code. Confirm that this is its only kind of evidence.

## Execution

Each phase leaves the repository valid and tested.

**Phase 1 — Code references.** The reference format; `cover` writing
references; coverage computed per comparison; remap and staleness; deletions and
whole-file references; re-pinning at merge. Remove `saga-diff://` evidence and
`rebase-evidence`.

**Phase 2 — Observe and compare.** `--against` on `open`, `status`, and `query`,
with merge-base semantics; `saga.json` drops its comparison; the Changed,
Affected, and Code layers; no approvals or comments on documentation;
replacement pairing, commit reasons attached to nodes, and node history;
reviews
as pull-request slide decks with per-slide approvals and out-of-date detection,
replacing the documentation review overlay (goal 11).
Depends on Phase 1.

**Phase 3 — App structure.** The app-level roots; epics containing today's
structure; app-unique story identity and moving stories between epics; personas
and story-to-persona links; the persona coverage gate; feature flags.
Independent of Phase 1 and can run beside it.

**Phase 4 — App view.** The reviewer's app-level view above the epics. Needs
open decision 4.

**Phase 5 — This repository.** Fold its Sagas into one `app.saga` as epics. If
the fold is awkward, the model is wrong, so this is the model's acceptance test.
It also decides whether decision records are needed.

Phase 2 also includes the sync cursor and compare mode for companion
repositories (goal 10).

**Review coverage.** Every changed line in a review's range is covered by its
deck, reported per review and shown in the reviewer (goal 11).

**Overview and terminology.** The overview's formal parts, term records with
bidirectional story and code references, stale terms on rename, and new-term
suggestions in comparisons (goal 12).

**Phase 6 — Adoption.** Turn readiness into a coverage report with no
verdict: `status` exits zero whenever it can report, and non-zero only when the
Saga is broken; the report covers implementation, the ratchet on existing
records, and every growth area as stable JSON teams can script their own rules
over, with documented CI recipes; `check --covers` for yes/no questions; `init` and first-run next actions that start with coverage; the
default epic; contextual growth suggestions that teach the practices; and the
skill, README, and help rewritten for incremental adoption. The first-run experience on a real 30-file pull request is its
acceptance test.
