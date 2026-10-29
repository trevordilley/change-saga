
# Change Saga

[![CI](https://github.com/twentyideas/changesaga/actions/workflows/ci.yml/badge.svg)](https://github.com/twentyideas/changesaga/actions/workflows/ci.yml)
[![Browser E2E](https://github.com/twentyideas/changesaga/actions/workflows/e2e.yml/badge.svg)](https://github.com/twentyideas/changesaga/actions/workflows/e2e.yml)
![status: experimental](https://img.shields.io/badge/status-experimental-orange)
![license: MIT](https://img.shields.io/badge/license-MIT-blue)
[![Made with ❤️ using DevSwarm](https://img.shields.io/badge/Made%20with%20%E2%9D%A4%EF%B8%8F%20using-DevSwarm-5F2AFF?labelColor=0A022E&logo=data%3Aimage%2Fpng%3Bbase64%2CiVBORw0KGgoAAAANSUhEUgAAAEAAAAAiCAQAAABFXBcEAAACEElEQVR42s1Y63mDMAw8ugErsAIdwR2BjsAKrMAKWSEdgRXICGQEMsL1RwhIIIFpXhX%2F7E%2F2WY%2BTBKCEAFhxko7TemDPGOmZXzUAQumUt3VHCIKpUimGVRA8MlaONx2ApYKWcg0CAfAgFJrhEAAsuEeKQQsEG7F%2BWLEBQTBXx4Tx%2FSnbXQDa61sJzM%2FMXRss0QpDVtwrlXCeYdWbJNP1CVjgOO5c8JmcCSABM7RIx50TTo4RMwRHv0E27nwnP5wuVuHXyRfQfgFZiB39aUfVYkdn1jIUCYC19iFsH%2BoAUx%2FAoP09njGDNgvF4Zo%2BIopJsnQtAOhklVlUKhvkAoIXKPTSz6UTAmC2tBbdAJcsp5iM0%2Fu7eACDRq3OmgDoO8JwCl2ycNNvC4AO5lqcZlh5aeae2Yg5M9l%2FldFXz9M0Xw4A5uknENvsvwFgYdGjY9GO6VVhVv1oxQXja5qhG2DHVMVF1AYRte1fASxqZ%2BMyRaaviWP%2Frap%2BS8fOqQwK2gcuQjPFK0TfuOKC5mEuaNdc8O4gxDPSMI1NQw4KRt%2F2FCLKjIK3QcX13VRcbVCx2XLLUOz3AYATU5wX%2FICpGr65HI8NSbe85IENSWGPLv%2BjJdtsSuvoprSziH2tKfXb8jO%2BHtaW52iEvtWWv28wecVoFiJHs7cPp%2BZ4Xt49nhc7xnNYE703uqz9oAjiB4VBy1J%2BAQwDuoYAr7YrAAAAAElFTkSuQmCC)](https://devswarm.ai)

Change Saga makes large changes, often AI-written, easy to digest. Ask your
coding agent to **create a review for this PR** and it produces a review deck:
slides that explain the change's architecture, so you review the design instead
of every line. Coverage links every changed line to the slide that explains it,
so you can trust that nothing was left out.

Change Saga is experimental, and its format may change before 1.0.

- [Install](#install)
- [Start with a review](#start-with-a-review)
- [Reviewing a Saga](#reviewing-a-saga)
- [Use it in CI](#use-it-in-ci)
- [Grow into living documentation](#grow-into-living-documentation)
- [Working with your coding agent](#working-with-your-coding-agent)
- [See an example Saga](#see-an-example-saga)
- [Status, check and reconcile](#status-check-and-reconcile)
- [Structured access for AI](#structured-access-for-ai)
- [Authoring by hand](#authoring-by-hand)
- [CLI reference](#cli-reference)

## Why

A normal PR description sits above a flat file-by-file diff. With a big change,
the reviewer has to rebuild the product intent, the design, and the system
model while reading isolated files in an arbitrary order.

A Saga gives the reviewer that context first and keeps it attached to the code:

1. The review deck explains the change as a sequence of visual arguments.
   Diagrams, interactive HTML, screenshots, and examples show the architecture
   and the important flows. Expectation/actual callouts expose surprising
   behavior, tradeoffs, hidden coupling, and intentional deviations.
2. Items inside each slide link to the exact diff ranges they explain.
3. `change-saga check --covers review` and `change-saga status` report what is
   unaccounted for.

The tool does not review the code or pass a verdict. It helps the author
prepare the material other people review. AI is useful here because it can
build the first draft, create diagrams and examples, and iterate until the
whole change is represented. The reviewer still decides whether the change is
correct.

## Install

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/twentyideas/changesaga/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/twentyideas/changesaga/main/scripts/install.ps1 | iex
```

Or download an archive for your platform from the
[latest release](https://github.com/twentyideas/changesaga/releases/latest),
or [build from source](#building-from-source). Then check the installation:

```sh
change-saga version
change-saga help
```

## Start with a review

Give your coding agent two prompts from the repository with the change:

> Use the change-saga cli to install its skills for this coding agent

> Create a review for this PR

(For a branch without a pull request yet, ask for a review of the branch.)

The installation includes two skills. `change-saga-context` helps the agent
consult existing stories, architecture, and code links during everyday
planning, debugging, and implementation, even when you do not mention Saga.
`change-saga` handles authoring when you ask for it, normally while preparing
or updating the PR. Reading context does not trigger upkeep.

What you get:

- `change.saga`, one folder at the repository's root, committed with the
  change. Reviewers can collapse it in GitHub's file tree.
- A review deck for the pull request. Each slide explains one architectural
  idea, such as components and how they relate, a data flow, or a state
  change.
- The surprises called out. Whatever would break a reviewer's reasonable
  expectation is a callout on the responsible part of the diagram: what you
  would expect, what the change does instead, why, and the consequence.
  Reviewers see them in a Surprises panel on each slide, and
  `change-saga review list` names them.
- Every changed line linked to the slide Item that explains it.
  `change-saga check --covers review` exits 0 only when nothing is left out.
  Coverage is an omission check, not proof: reviewers still judge each slide.

The handful of commands the agent runs (each one prints the next):

```sh
change-saga init                                   # creates change.saga
change-saga review create change.saga              # works out the PR, base, and id
change-saga apply-slide --review pr-42 --from architecture.json change.saga   # a diagram and its Items
change-saga cover --target urn:change-saga:shop:review:pr-42:slide:architecture:item:cache \
  --path cache.go --changed-lines change.saga
change-saga check --covers review --against main change.saga
change-saga open --against main change.saga          # the reviewer
```

Each slide is one JSON request: a diagram source (explicitly positioned nodes,
edges, and groups the CLI renders to SVG) and the Items a reviewer can open,
each selecting one element. The skill's
[diagrams reference](skills/change-saga/references/diagrams.md) has a complete
example.

Sagafying a pull request is the best way to learn the tool. Nothing else is
needed.

## Reviewing a Saga

`change-saga open` starts a local review application in the background and
opens it in your browser. Opened on its own it shows the documentation as of
the current commit; opened with `--against main` it compares the branch,
showing what the change revised, what it affected, and the code, grouped by
the documentation that explains it. `change-saga serve status` and
`serve stop` manage the running reviewer; `serve --open` keeps it attached to
the terminal instead.

The header divides the application in two: **Documentation** is what the app
is, and **Review** is how it is changing. The **Overview** (pitch,
description, personas, terms, design system, onboarding deck, and feature
flags) appears on both sides.

- **Documentation** lists every feature. Inside a feature the sidebar is
  always Stories, Prototypes, UX, UI, Technical, and Quality, in that order,
  omitting any that are empty. Technical opens on arrival to its slides.
  **Present** shows a deck full screen, one slide at a time.
- **Review** lists each pull request's review deck with the diffs its slides
  reference. Reviewers approve or request changes slide by slide and discuss
  slides and Items. A decision records the commit it was given at and shows as
  out of date once that slide or its code changes. **Code Diff** is a
  traditional file tree and diff with links back to every explanation, and
  **Coverage** maps code to explanations and back.

A feature, story, or acceptance criterion lists the reviews that changed the
code it explains. Nobody writes that list: it is derived from the diffs.

The documentation itself carries no comments or approvals; review happens in
reviews. The tool records every decision and never declares a review
approved: what a pull request needs before merging is the team's decision.
Each decision and comment is its own file inside the Saga, attributed by the
commit that adds it.

## Use it in CI

By default, changes that only touch root `change.saga` should invoke no CI and
have no CODEOWNERS. Run `change-saga setup-ci` for guidance on excluding those
changes from existing automation and wildcard ownership rules. Saga checks and
product, architecture or quality review requirements are separate opt-ins.
The command prints instructions for you or your coding agent; it does not change
workflows or repository settings. See the [full setup guide](internal/cli/setup_ci_prompt.md).

A team can also make the Saga part of its pull request gate. That is the
team's policy, opted into in its own workflow; the tool never requires it. The
optional workflow below checks that the documentation stays current: nothing
the Saga already documents is left stale or broken. It gates pull requests
into `main`; child branches merging into an integration branch are left
alone, since the integration branch's pull request reconciles their work:

```yaml
name: Change Saga
on:
  pull_request:
    branches: [main]
permissions:
  contents: read
jobs:
  saga:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          # Evidence pins and review ranges need the full history.
          fetch-depth: 0
          # Check the pull request's own head rather than GitHub's merge commit.
          ref: ${{ github.event.pull_request.head.sha }}
      - name: Install change-saga
        env:
          # Replace with the release tag to pin (v0.2.0-rc.6 or later).
          CHANGE_SAGA_VERSION: vX.Y.Z
        run: |
          curl -fsSL "https://raw.githubusercontent.com/twentyideas/changesaga/$CHANGE_SAGA_VERSION/scripts/install.sh" | sh -s -- --version "$CHANGE_SAGA_VERSION" --dir "$HOME/.local/bin"
          echo "$HOME/.local/bin" >> "$GITHUB_PATH"
      - name: The documentation stays current
        run: change-saga check --covers health change.saga
      # A team that wants a review deck on every pull request into main
      # can also require it to explain every changed line:
      # - name: The review deck explains every changed line
      #   run: change-saga check --covers review --against origin/${{ github.base_ref }} change.saga
```

`check` exits 0 when the named areas are covered, 3 when one has a gap (it
prints only those gaps), and 1 when its report cannot be trusted, such as a
malformed Saga. The [CI rules](skills/change-saga/references/ci.md) describe
every area and how to write rules over `status --json`.

## Grow into living documentation

A Saga of reviews alone is complete, and many teams never need more. If the
team wants it, the same Saga can grow a step at a time into documentation of
the application itself, never as a prerequisite. `change-saga status` stays
about the review until the Saga holds more (`status --full` shows every area).
For a first pass, `setup-initial-saga` guides an interview:

> Run change-saga setup-initial-saga and follow its guided setup workflow

A grown Saga holds:

- **Overview**: the project's name, elevator pitch, description, and its
  **terms and vocabulary**: the words a newcomer would not know, each linked to
  the stories and the exact code (usually an enum or a constant) that define
  it.
- **Personas**, a **design system**, an **onboarding deck**, and **feature
  flags**.
- **Features**, the durable areas of the product. Each has the same sections,
  which never reorder as work progresses: **Stories** (user stories with
  acceptance criteria), **Prototypes** (interactive experiences), **UX** (flow
  decks), **UI** (visual references), **Technical** (a slide deck whose visual
  elements reference the exact code they explain, plus technical design), and
  **Quality** (test cases and their evidence).

It uses four presentation forms:

| Form | Purpose |
| --- | --- |
| Long-form essays | Product overviews, features, user stories and the technical-design approach: intent, context and decisions. |
| Slide decks | Technical implementation and code reviews: a guided explanation with exact evidence. Each deck has an [overview](docs/deck-overview.md) essay that cites its slides. |
| [Whiteboards](docs/whiteboards.md) | ERD, Infrastructure, Data flows and Architecture: connected diagrams readers explore by panning, zooming and inspecting evidence. |
| Prototypes | Interactive HTML apps that show an intended experience. Each opens on its own sandboxed page, or in its own tab, with annotations pinning it to criteria. |

A Technical design starts with one approach essay that links to the four
whiteboards: an ERD with crow's-foot notation, the infrastructure layout, data
flows through key services, and the codebase architecture.

Over time the Saga becomes one connected body of knowledge: persona → story →
design or test → exact code. Every link pins what it relied on. Evidence pins
code at a commit and follows it as it moves; records pin the revisions they
depend on. When a story or the code changes, whatever depended on the old
version goes visibly stale, and the tool says exactly what to revisit. That is
what makes growing past reviews worth it: your agents answer questions from
it, plan new work against it, and write better reviews with it.

A monorepo keeps one `change.saga` at its root and documents each app through
its own features. One Saga per repository is a recommendation, not a rule: any
`<name>.saga` directory is valid. Saga files are small independent records,
so separate agents can own stories, prototypes, design, test cases, and work
items in parallel and merge cleanly.

## Working with your coding agent

| To | Ask your agent |
| --- | --- |
| Install the skills | *Use the change-saga cli to install its skills for this coding agent* |
| Review a PR (start here) | *Create a review for this PR* |
| Read a PR's review deck | *Use the change-saga cli to open this PR's review deck* |
| Grow living documentation | *Run change-saga setup-initial-saga and follow its guided setup workflow* |
| Keep it current for a PR | *Create a review for this PR, then update the Saga's living documentation for the change* |
| Document a whole codebase | *Use the change-saga cli to create a Saga for this codebase since inception* |
| Fold a PR's Saga into the codebase Saga | *Use the change-saga cli to compare this PR's Saga with the codebase Saga and update what changed* |

**Do Saga work at PR time.** Ask for it when you ask your agent to open or
update the PR, not at the end of every coding session; a deck may lag the
branch in between.

**Review the big change, not every branch.** When one integration branch
gathers the work of many child workspaces, only its pull request gets a
review; the child branches merging into it get none. Many one- or two-slide
reviews fragment the story a deck exists to tell, and a small PR a reviewer
can read line by line doesn't need a deck. Deferring isn't skipping: the real
review gets the full deck with every changed line covered, and the living
documentation is reconciled for everything the children changed.

**Expect to iterate.** Authoring a substantial Saga asks your assistant to
navigate a repository, explain architecture, draw diagrams, and connect exact
Git evidence, so use a capable agentic model when you can. Treat what it
produces as a first draft. A useful follow-up prompt:

> Keep the explanations concise, direct, and factual. Remove repetition. Prefer
> a clear diagram or concrete example over another paragraph. Revise the Saga
> until each slide explains one coherent idea. Establish the system model, then
> foreground the consequential behavior, tradeoffs, and deviations that may
> surprise a reviewer. Show what they would reasonably expect, what actually
> happens, why, and the consequence—preferably as a callout on the responsible
> part of the visual.

Review the Saga yourself before asking peers to review the change. Complete
coverage does not prove that each claim has the right evidence or that every
link belongs where it was placed. Change Saga provides the surface; the author
is still responsible for the quality of what is on it.

## See an example Saga

This repository documents itself with [`change.saga`](change.saga): its
product requirements, designs, quality cases, implementation explanations, and
exact code references. Open it from a source checkout:

```sh
change-saga open change.saga
```

Or [download it](https://github.com/twentyideas/changesaga/releases/latest/download/change-saga-example.saga.zip)
from the latest release without cloning:

```sh
curl -fL https://github.com/twentyideas/changesaga/releases/latest/download/change-saga-example.saga.zip \
  -o change-saga-example.saga.zip
unzip change-saga-example.saga.zip
change-saga open change.saga
```

For one concrete path through it, follow the story **Trace a requirement to
current evidence and back** and its criterion **Each criterion shows its
current code path**:

```sh
change-saga query requirements --saga change.saga --requirement evidence-traversal
change-saga query traceability --saga change.saga \
  --criterion urn:change-saga:app:story:evidence-traversal:criterion:criterion-code
```

In the reviewer, open `/requirements/evidence-traversal/criteria/criterion-code`,
follow **Current criterion evidence paths** to its implementation Item, and
select the Item to open the exact source it references: requirement → design →
quality → implementation → code.

## Status, check and reconcile

`change-saga status --against main` answers first with how completely the
review deck explains the change, and asks for a review when there is none.
Once a team grows the Saga, it reports seven coverage areas (the review,
implementation, stories, personas, design, quality, and the health of what
already exists) with every gap listed, plus growth suggestions tied to the
change: "this change touched checkout; no story says why; capture the checkout
story?" Take them or ignore them.

`status` reports; it never passes a verdict, and it exits 0 whenever it can
produce a report. Your team decides what must be true before a merge, and can
ask directly with `check`, which exits 3 with only the gaps:

```sh
change-saga check --covers review --against main change.saga
change-saga check --covers implementation,stories --against main change.saga
```

When the Saga holds living documentation, reconcile it after authoring a PR's
review deck:

```sh
change-saga reconcile --against main --json change.saga
change-saga validate --json change.saga
change-saga check --covers health change.saga
```

The [reconciliation workflow](docs/reconciliation.md) separates review
coverage, documentation diff coverage, and HEAD currency, with reasons and
typed repair paths. Reassess before editing; current pins cannot prove the
explanation is correct.

## Structured access for AI

Agents do not need to crawl the Saga's files. `query` is a bounded, read-only
JSON interface over the overview, hierarchy, content, reviews, coverage gaps,
diff ownership, mapping quality, author claims, and verification:

```sh
change-saga query overview --saga change.saga
change-saga query gaps --saga change.saga --kind uncovered
change-saga query mappings --saga change.saga --sort scrutiny
change-saga query claims --saga change.saga --status unverified
```

`mappings` ranks broad or thin evidence so an AI can start with the weakest
justification. Claims are falsifiable assertions tied to exact code;
verification results are append-only. An AI reviewer can inspect the diff cold
first, then reconcile its findings against the author's account.

Authoring is batchable too. `cover --batch -` reads newline-delimited JSON,
resolves the whole batch before writing anything, and leaves the Saga
untouched if any record fails:

```sh
printf '%s\n' \
  '{"target":"api.chapter","path":"api.go","side":"new","lines":"18-24","note":"validates the request"}' \
  '{"target":"api.chapter/flow.fragment#submit-action","path":"ui.ts","side":"new","lines":"9","note":"wires the control"}' \
  | change-saga cover --batch - change.saga
```

Evidence references code as of a commit, so it follows the code as it
evolves. When later commits only move the referenced lines, the reference is
remapped automatically; when the lines change, it goes stale and says why.
After a change lands, re-pin references to the landed commit before the branch
is deleted; `repin` also records the branch's commit messages, so a squash
merge keeps their reasoning:

```sh
change-saga references --stale --diff change.saga
change-saga repin --onto <landed-commit> --branch <branch> change.saga
```

See [the AI-facing interface](docs/ai-facing-interface.md) for the complete
contract and [SPEC.md](SPEC.md) for the format.

## Authoring by hand

Most people should let their coding agent run these commands; exhaustive
coverage and pinned evidence are tedious bookkeeping that AI is good at. If you
want to author directly, do it in your own repository, from the branch with the
work:

```sh
change-saga init
change-saga review create change.saga
change-saga apply-slide --review pr-42 --from ./architecture.json --dry-run change.saga
change-saga apply-slide --review pr-42 --from ./architecture.json change.saga
change-saga cover --target urn:change-saga:shop:review:pr-42:slide:architecture:item:cache \
  --path path/to/cache.go --changed-lines change.saga
change-saga review list change.saga
change-saga check --covers review --against main change.saga
change-saga open --against main change.saga
```

`review create` prints the review id it chose (here `pr-42`) and each command
prints the next. A slide that needs hand-drawn SVG uses `add-slide --review`,
`set-slide-content --review`, and `add-item --review` instead. A feature's
Technical deck explains the current code the same way (`add-deck`,
`add-slide --deck`, `add-item --slide`, `cover --against main`). If the Saga
lives in a separate repository, pass `--repo /path/to/source-checkout` to
commands that read the diff.

To see what a PR does to an existing Saga, without comparing prose or
diagrams:

```sh
change-saga status --against <pr-base> --head <pr-head> change.saga
change-saga query layers --saga change.saga \
  --against <pr-base> --head <pr-head> --layer affected
```

## CLI reference

Every command takes the Saga directory (usually `change.saga`) as its last
argument; `query` takes it as `--saga PATH`. Run `change-saga <command> -h` for
a command's flags, and `change-saga spec --json` for the machine-readable
grammar. Many commands that write accept `--dry-run`, and most accept `--json`.
Every `remove` deletes the record and the links that only pointed at it, and
lists what else lost coverage; run it with `--dry-run` first to see the whole
effect.
Commands that read a diff accept `--repo PATH` when the Saga lives in a
separate repository from the code.

### Setup and help

| Command | What it does |
| --- | --- |
| `version` | Print the installed version. |
| `help` | Overview of the workflow and every command. |
| `install-skill` | Print the prompt that installs the authoring skills into your coding agent. |
| `init [name.saga]` | Create the repository's Saga, `change.saga` by default. |
| `setup-ci` | Print guidance for keeping Saga-only changes out of CI and CODEOWNERS, and for optional Saga checks. Changes nothing. |
| `setup-initial-saga` | Print the guided, one-time workflow for growing a Saga into living documentation. |
| `spec [--json]` | Publish the resources, relation matrix, command grammar and invariants. |

### Reviews

| Command | What it does |
| --- | --- |
| `review create` | Create the review of a pull request or branch. Works out the PR, base and id. |
| `review list` | Each review slide's decisions, whether they are out of date, and each deck's coverage. |
| `review follow` | Set the ref an open review follows. |
| `review refresh-coverage` | Re-pin a review deck's moved references and report new uncovered lines. |
| `review test-plan` | Select the automated tests reached by the review's changed code and stories. |
| `review approve` / `request-changes` / `withdraw` | Record or withdraw a decision on one review slide at the current head. |
| `review comment` | Comment on a slide, Item or diff lines, or reply to a comment. |

### Slides and decks

| Command | What it does |
| --- | --- |
| `add-deck` | Create a feature's implementation deck. `--role ux` creates its UX flows deck, `--role onboarding` the app's onboarding deck, and `--feature-theme ID` a theme's rationale deck. |
| `revise-deck` / `remove-deck` / `move-deck` | Correct a deck's title, objective or rank; delete it; or move it to another feature. |
| `deck overview` | Author a deck's overview essay with slide and Item citations, or check that it cites every slide. |
| `apply-slide` | Publish one complete slide from a JSON request: its diagram or visual and its Items. `--print-current` prints the request for an existing slide. |
| `add-slide` | Add one slide to a deck or review. |
| `set-slide-content` | Replace a slide's visual while keeping its target and Items. |
| `revise-slide` / `remove-slide` / `move-slide` | Correct a slide's title, intent, layout, takeaway or rank; delete it; or move it to another deck. |
| `add-item` | Add a semantic Item, including a callout, to a slide. |
| `revise-item` / `remove-item` | Correct an Item in place, or delete it with its evidence. |
| `diagram describe` / `get` | Read a diagram slide as text, or one element's full properties. |
| `diagram edit` | Apply explicit edits to a diagram slide and republish it. |
| `diagram icons` | List the bundled icons a diagram may use. |
| `diagram check` | Report diagram slides whose published SVG no longer matches their source. |
| `visual-qa` | Render slides and the reviewer at two viewports, with a contact sheet and findings. |

### Coverage and evidence

| Command | What it does |
| --- | --- |
| `cover` | Reference the code a target explains, pinned at a commit. `--changed-lines` covers a file's whole change; `--batch` reads many records. |
| `replace-coverage` / `remove-coverage` | Atomically replace or delete one coverage record. |
| `references` | List every code reference as current, remapped or stale, with the diff since its pin. |
| `repin` | After a change lands, re-pin evidence to the landed commit (`--onto`). `--accept-proposed` repairs stale references by accepting their proposed ranges. |
| `sync` | Move a companion Saga's sync cursor to the code commit it now documents. |
| `add-claim` / `verify-claim` | Record a falsifiable author assertion with its code, and append independent verification results. |

### Status, checks and upkeep

| Command | What it does |
| --- | --- |
| `status` | Report review coverage first, then coverage by area, with every gap. Never a verdict. |
| `check --covers AREA,...` | Exit 0 when the named areas are covered, 3 with their gaps, 1 when the report cannot be trusted. For CI. |
| `validate` | Check every structural invariant without writing. |
| `reconcile --against REV` | Read-only queue of what a change affected: stale references, baseline debt and repair paths. |
| `preintegrate --ref REF --ref REF` | Report semantic convergence risks between branches before merging them. |

### Viewing

| Command | What it does |
| --- | --- |
| `open` | Start the reviewer in the background and open it in a browser. `--against main` compares the branch. |
| `serve` | Serve the reviewer on loopback; `serve status` and `serve stop` manage a running one. |
| `query <operation>` | Run one read-only query and print JSON. `query schema OPERATION` describes each operation. |
| `theme init` / `check` / `preview` | Write a starter `theme.css`, validate it and its contrast, or preview every token. |

### Product: overview, features and people

| Command | What it does |
| --- | --- |
| `overview set-pitch` / `set-description` | Write the app's elevator pitch and description. |
| `overview set-data-model` / `set-technical-design` | Write the data-model overview and the technical-design essay. |
| `feature add` / `rename` / `remove` | Add, rename or remove a durable product area. `remove --with-content` deletes it with everything in it. |
| `feature archive` / `restore` | Hide a feature from the feature lists and coverage gaps while keeping its records, or bring it back. |
| `feature set-overview` / `set-plan` | Write a feature's description, or set its planning metadata. |
| `feature-theme add` / `rename` / `remove` / `set-overview` / `set-features` | Group features into optional product themes. |
| `label` / `milestone` `add` / `rename` / `remove` | Shared labels and milestones for features. |
| `persona add` / `revise` / `set-state` / `remove` | Who gets value from the app. |
| `term add` / `revise` / `set-state` / `remove` | The project's vocabulary, linked to stories and the code that defines each term. |
| `flag add` / `revise` / `set-state` / `remove` | Feature flags gating stories or features. |
| `deprecation add` / `revise` / `set-state` | Record that something is deprecated, without changing it. |

### Stories and prototypes

| Command | What it does |
| --- | --- |
| `story add` / `revise` | Create a story, or append a complete revision. |
| `story set-state` / `withdraw` | Move a story through proposed, accepted, deferred, rejected or retired. |
| `story move` / `consolidate` | Move a story to another feature, or merge a duplicate. |
| `story remove` | Delete a story and the links that only pointed at it; lists what else lost coverage. |
| `criterion add` / `revise` / `remove` | Change one acceptance criterion through a new story revision. |
| `citation add` / `remove` | Record a source that stories and evidence can cite, or delete one. |
| `relation add` / `repin` / `supersede` / `status` | Typed, pinned links between records, such as a slide that addresses a criterion. |
| `prototype add` | Add a prototype: an HTML file committed in the project. The Saga points at it rather than copying it. |
| `prototype update` | Record a new version after the prototype's file changed. |
| `prototype add-external` / `add-html` | Name a prototype hosted elsewhere, or copy one that doesn't live in the project into the Saga. |
| `prototype revise` / `annotate` | Append a prototype revision, or pin part of it to a story or criterion. |
| `prototype remove` | Delete a prototype and its annotations. |

### Technical design and whiteboards

| Command | What it does |
| --- | --- |
| `whiteboard publish` | Publish a complete whiteboard revision from a `board.json`. |
| `whiteboard get` / `describe` / `check` / `list` | Read, describe, validate or list whiteboards. `--focus` selects one element. |
| `whiteboard rename` / `assign-role` / `unassign-role` | Change a board's title or its ERD, Infrastructure, Data flows or Architecture role. |
| `whiteboard archive` / `restore` | Hide a board from discovery, or bring it back, without losing history. |
| `component` / `system` / `data-entity` `add` / `revise` / `set-state` | Stable technical definitions that documentation can pin. |
| `erd` / `erd-overlay` `add` / `revise` / `set-state` | ERD documents and their overlays. |
| `inventory adopt-format` | Opt in to inventory format 2. |
| `design add-chapter` / `add-section` / `add-fragment` / `set-fragment-content` | Author a feature's technical-design chapters. |

An ERD is [one whiteboard](docs/erd-whiteboard-guide.md) of connected sections,
entities, relationships, callouts and source evidence. Named boards open at
`/technical/whiteboards/<id>`, and the Data model page at `/technical/erd` uses
the `data-model` board by convention:

```sh
change-saga whiteboard publish --id data-model --revision r1 --from board.json change.saga
change-saga whiteboard describe --id data-model change.saga
change-saga whiteboard check --id data-model change.saga
```

Source references currently support exact ERD and data-entity pins and ERD
imports; component and system pins are not supported yet.

### Narrative content

| Command | What it does |
| --- | --- |
| `add-chapter` / `add-section` / `add-fragment` | Add report chapters, sections and fragments to a feature or the design system. |
| `set-fragment-content` | Replace a fragment's content. |
| `add-landmark` | Make a heading, text span, element or image region inside a fragment coverable. |
| `revise-chapter` / `revise-section` / `revise-fragment` | Correct a title or display order. |
| `remove-chapter` / `remove-section` / `remove-fragment` | Delete content; dependent relations are listed. |
| `move-chapter` / `move-section` / `move-fragment` | Move or rename content, keeping its identity. |

### Quality

| Command | What it does |
| --- | --- |
| `quality test-case add` / `revise` / `set-state` / `remove` | Test cases with ordered steps, linked to the criteria they verify. |
| `quality policy set` | Set the test kinds a criterion requires (default positive, negative and edge). |
| `quality evidence add` | Attach test code, the code under test, or run artifacts to a test case. |
| `quality run record` | Deprecated: record run history, with no coverage effect. |

### Work planning

| Command | What it does |
| --- | --- |
| `plan add-wave` / `revise-wave` | Delivery phases with entry and exit conditions. |
| `plan add-item` / `revise-item` | Independently assignable units of work. |
| `plan add-dependency` / `add-contract` | Prerequisites and interfaces between work items. |
| `plan assign` / `progress` / `record-merge` | Bind work to a workspace and branch, and record its progress and merges. |

## Security

The review application runs only on loopback and refuses remote bind
addresses. Mutations require a per-process token and same-origin requests.
Interactive fragments run in sandboxed frames with network access and parent
application access disabled.

A saga can still contain untrusted HTML, SVG, and JavaScript. Treat one from an
untrusted author with the same care as code from an untrusted branch. See
[SECURITY.md](SECURITY.md) for the threat model and private reporting process.

## Building from source

Change Saga requires Go 1.26.6:

```sh
git clone https://github.com/twentyideas/changesaga
cd change-saga
go build -o ./bin/change-saga ./cmd/change-saga
./bin/change-saga help
```

The release build is a single executable with no separately installed Go
runtime and no hosted service.

## Project links

- [Format specification](SPEC.md)
- [Documentation index](docs/README.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [Support](SUPPORT.md)
- [Governance](GOVERNANCE.md)
- [Changelog](CHANGELOG.md)
- [Release process](docs/releasing.md)

## License

MIT. See [LICENSE](LICENSE).
