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

### 6. Approval belongs to a change

Approval exists only in compare mode, on exactly the Changed and Affected layers.
An approval pins the revision it approved, so the next change to that record
makes it need approval again. Observe mode has no approve or reject controls; it
shows approvals as history. Comments are available in both modes.

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

- **The only requirement is that the implementation covers the change.** Every
  changed line is referenced by the implementation deck. Personas, stories,
  design, and test cases are not required up front.
- **Everything else is growth, not debt.** Missing personas, stories, design, and
  quality are reported as opportunities. They never block a change, and
  readiness passing with nothing defined is correct: absence is not failure.
- **What exists must stay healthy.** Once a story is accepted or a design
  references code, a change that makes that link stale or leaves its code
  uncovered is flagged. Coverage only ratchets up.
- **Nothing is locked in.** The first change's deck goes into an epic the author
  names, defaulting to the pull request's title. Story, deck, and slide URNs
  carry no epic, so reorganizing later breaks nothing.
- **The app Saga lives in the repository by default.** Compare mode relies on one
  Git comparison covering both the Saga and the code. A companion repository is
  possible but second-class, because its commits have to be correlated with the
  code's.
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
3. When a change breaks a link on an existing record (a stale story link, or
   uncovered code under an existing design), does that block the change or only
   warn? Absence of records never blocks (goal 9).
4. What is the app-level sidebar above the epics?
5. Onboarding deck Items point at records (personas, epics, stories) rather than
   code. Confirm that this is its only kind of evidence.

## Execution

Each phase leaves the repository valid and tested.

**Phase 1 — Code references.** The reference format; `cover` writing
references; coverage computed per comparison; remap and staleness; deletions and
whole-file references; re-pinning at merge. Remove `saga-diff://` evidence and
`rebase-evidence`.

**Phase 2 — Observe and compare.** `--against` on `open`, `status`, and `query`,
with merge-base semantics; `saga.json` drops its comparison; the Changed,
Affected, and Code layers; approvals only in compare mode, pinned to revisions;
replacement pairing, commit reasons attached to nodes, and node history.
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

**Phase 6 — Adoption.** Split readiness into the one requirement
(implementation covers the change) and growth; the ratchet on existing
records; `init` and first-run next actions that start with coverage; the
default epic; contextual growth suggestions that teach the practices; and the
skill, README, and help rewritten for incremental adoption. The first-run experience on a real 30-file pull request is its
acceptance test.
