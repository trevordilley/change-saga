
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

## Start with a review

After [installing](#install) the CLI, give your coding agent two prompts from
the repository with the change:

> Use the change-saga cli to install its skill for this coding agent

> Create a review for this PR

(For a branch without a pull request yet, ask for a review of the branch.)

What you get:

- `change.saga`, one folder at the repository's root, committed with the
  change. Reviewers can collapse it in GitHub's file tree.
- A review deck for the pull request. Each slide explains one architectural
  idea, such as components and how they relate, a data flow, or a state
  change.
- The surprises called out. Whatever would break a reviewer's reasonable
  expectation is a callout on the responsible part of the diagram: what you
  would expect, what the change does instead, why, and the consequence.
  Reviewers see them under each slide, and `change-saga review list` names
  them.
- Every changed line linked to the slide Item that explains it.
  `change-saga check --covers review` exits 0 only when nothing is left out.
  Coverage is an omission check, not proof: reviewers still judge each slide,
  and approve or discuss the deck slide by slide.

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
each selecting one element. `diagram edit` revises a slide in place and
`diagram describe` reads it back as text. The skill's
[diagrams reference](skills/change-saga/references/diagrams.md) has a complete
example.

Sagafying a pull request is the best way to learn the tool. Nothing else is
needed.

## Use it in CI

Once a team creates reviews, it can make them part of its pull request gate.
That is the team's policy, opted into in its own workflow; the tool never
requires it. This GitHub Actions workflow fails a pull request whose review
deck does not explain every changed line:

```yaml
name: Change Saga
on: pull_request
permissions:
  contents: read
jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          # Review ranges and merge-bases need the full history.
          fetch-depth: 0
          # Check the pull request's own head rather than GitHub's merge commit.
          ref: ${{ github.event.pull_request.head.sha }}
      - name: Install change-saga
        env:
          # Replace with the release tag to pin (see the note below).
          CHANGE_SAGA_VERSION: vX.Y.Z
        run: |
          curl -fsSL "https://raw.githubusercontent.com/twentyideas/changesaga/$CHANGE_SAGA_VERSION/scripts/install.sh" | sh -s -- --version "$CHANGE_SAGA_VERSION" --dir "$HOME/.local/bin"
          echo "$HOME/.local/bin" >> "$GITHUB_PATH"
      - name: The review deck explains every changed line
        run: change-saga check --covers review --against origin/${{ github.base_ref }} change.saga
      # Once the Saga holds living documentation, a team may also keep it
      # from going stale:
      # - name: Existing documentation stays healthy
      #   run: change-saga check --covers health change.saga
```

Use a release that includes `check --covers review` (v0.2.0-rc.6 or later)
for `CHANGE_SAGA_VERSION`.

`check` exits 0 when the named areas are covered, 3 when one has a gap (it
prints only those gaps), and 1 when its report cannot be trusted, such as a
malformed Saga. A team names only the areas it wants; the
[CI rules](skills/change-saga/references/ci.md) describe the others and how
to write rules over `status --json`.

## Grow gradually

A Saga of reviews alone is complete, and many teams never need more. If the
team wants it, the same Saga can grow a step at a time, when it helps and never
as a prerequisite: the personas the app serves, their stories and acceptance
criteria, features, design, quality, terms, and living implementation
documentation. Nothing prompts you to do it; `change-saga status` stays about
the review until the Saga holds more (`status --full` shows every area). For a
first pass, `change-saga setup-initial-saga` guides an interview:

> Run change-saga setup-initial-saga and follow its guided setup workflow

A grown Saga holds:

- **Overview**: the project's name, elevator pitch, a short description, and
  its **terms and vocabulary**: the words the team uses that a newcomer would
  not know, each linked to the stories and the exact code (usually an enum or a
  constant) that define it.
- **Personas**, a **design system**, an **onboarding deck**, and **feature
  flags**.
- **Features**, the durable areas of the product. Each has the same four parts,
  which never reorder as work progresses: **Product** (prototypes, and user
  stories with acceptance criteria), **Design** (UX flows, UI references, and
  technical design), **Quality** (test cases and their evidence), and
  **Implementation** (a slide deck whose visual elements reference the exact
  code they explain).

A monorepo of several apps keeps one `change.saga` at its root and documents
each app through its own features. One Saga per repository is a
recommendation, not a rule: any `<name>.saga` directory is valid, and nothing
refuses a second one.

## A connected body of knowledge that pays dividends

Over time the Saga becomes one connected body of knowledge: persona → story →
design or test → exact code. Code maps back to stories through designs and
test cases rather than hand-written code-to-story links. Every link pins what
it relied on. Evidence pins code at a commit and follows it as it moves;
records pin the revisions they depend on. When a story or the code changes,
whatever depended on the old version goes visibly stale, and the tool says
exactly what to revisit.

That is what makes growing past reviews worth it: your agents build on it.
They answer questions from it, plan new work against it, and create better
reviews with it, and each review strengthens it further.

Change Saga is experimental, and its format may change before 1.0.

## Install

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/twentyideas/changesaga/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/twentyideas/changesaga/main/scripts/install.ps1 | iex
```

Then check the installation:

```sh
change-saga version
change-saga help
```

## See a Saga

This repository documents itself with [`change.saga`](change.saga), the
canonical self-hosting example. It contains the product requirements, designs,
quality cases, implementation explanations, and exact code references for
Change Saga itself. After installing Change Saga, open it from a source checkout with:

```sh
change-saga open change.saga
```

Or download the same validated Saga without cloning the repository:

```sh
curl -fL https://github.com/twentyideas/changesaga/releases/latest/download/change-saga-example.saga.zip \
  -o change-saga-example.saga.zip
unzip change-saga-example.saga.zip
change-saga open change.saga
```

[Download the example Saga](https://github.com/twentyideas/changesaga/releases/latest/download/change-saga-example.saga.zip)
directly from the latest stable release. The archive is platform-neutral and
expands into one `change.saga` directory.

For one concrete path through it, inspect the reviewer story **Trace a
requirement to current evidence and back** and its pass/fail criterion **Each
criterion shows its current code path**:

```sh
change-saga query requirements --saga change.saga --requirement evidence-traversal
change-saga query traceability --saga change.saga \
  --criterion urn:change-saga:app:story:evidence-traversal:criterion:criterion-code
```

In the reviewer, open
`/requirements/evidence-traversal/criteria/criterion-code`. Follow **Current
criterion evidence paths** to its implementation Item, then select the Item to
open the exact source it references. That live chain is the example used
throughout this README: requirement → design → quality → implementation → code.

## Prompts

**To install the Change Saga skill:**

> Use the change-saga cli to install its skill for this coding agent

**To create a review for a PR (start here):**

> Create a review for this PR

**To review a PR that has a review deck:**

> Use the change-saga cli to open this PR's review deck

**To grow the Saga into living documentation, if the team wants it:**

> Run change-saga setup-initial-saga and follow its guided setup workflow

**Once the Saga has living documentation, to keep it current for a PR:**

> Create a review for this PR, then update the Saga's living documentation for the change

## Status and check

`change-saga status --against main` answers first with how completely the
review deck explains the change, and asks for a review when there is none.
While the Saga holds only reviews, everything else stays out of the way.

Once a team grows the Saga, `status` reports seven coverage areas (the review,
implementation, stories, personas, design, quality, and the health of what
already exists) with every gap listed, and offers growth suggestions tied to
the change: "this change touched checkout; no story says why; capture the
checkout story?" Each suggestion explains the practice it teaches and the one
command that acts on it; take it or ignore it. New product work may instead
begin with personas, valuable user stories, and pass/fail acceptance criteria.
Over time prototypes, stories, design, test cases, and terms fill in, and a
discovery during implementation becomes an explicit revision of the story it
changes.

`status` reports; it never passes a verdict, and it exits 0 whenever it can
produce a report. Your team decides what must be true before a merge, and can
ask directly:

```sh
change-saga check --covers review --against main change.saga
change-saga check --covers implementation,stories --against main change.saga
```

`check` exits 0 when every named area is covered and 3 when one has a gap,
printing only those gaps. Nothing is reduced to a score.

When the Saga holds living documentation, reconcile it after authoring a PR's
review deck:

```sh
change-saga reconcile --against main --json change.saga
change-saga validate --json change.saga
change-saga check --covers health change.saga
```

The [reconciliation workflow](docs/reconciliation.md) separates review coverage,
documentation diff coverage, and HEAD currency. It exposes baseline debt and
regressions with reasons and typed repair paths. Reassess before editing, then
check again; current pins cannot prove the explanation is correct.

Saga files are built for parallel work. Separate agents can own story
revisions, prototypes, design, test cases, and work items, then merge the Saga
alongside the implementation as the work fans out and converges.

## Expect to iterate

Change Saga works with the coding assistant you already use, but authoring a
substantial Saga asks that assistant to navigate a repository, use tools,
explain architecture, create diagrams, and connect exact Git evidence. Use a
capable agentic model when possible. Smaller, free, or preview models may still
complete the workflow, but they often need more guidance and revision.

Treat a generated Saga as a first draft. Even frontier models rarely produce a
clear, complete Saga in one pass. Review the prose and diagrams, then ask the
assistant to revise anything repetitive, vague, or difficult to follow. A
useful follow-up prompt is:

> Keep the explanations concise, direct, and factual. Remove repetition. Prefer
> a clear diagram or concrete example over another paragraph. Revise the Saga
> until each slide explains one coherent idea. Establish the system model, then
> foreground the consequential behavior, tradeoffs, and deviations that may
> surprise a reviewer. Show what they would reasonably expect, what actually
> happens, why, and the consequence—preferably as a callout on the responsible
> part of the visual.

Review the Saga yourself before asking peers to review the change. AI can do a
good job of connecting explanations to code, but complete coverage does not
prove that each claim has the right evidence or that every link belongs where
it was placed. Check those relationships, correct anything misleading, and
make sure the narrative is coherent. Preparing a Saga for peer review is not
automatic: Change Saga provides a robust surface for reviewing the work, but
the author is still responsible for making it a quality piece of work.

Change Saga provides the structure that links technical documentation to exact
Git evidence; it does not impose a writing personality. If your assistant tends
to over-explain or overbuild, optional agent guidance such as
[Ponytail](https://github.com/DietrichGebert/ponytail) may help, but it is not
required.

## What it does

A normal PR description sits above a flat file-by-file diff. With a big change,
the reviewer has to rebuild the product intent, the design, and the system
model while reading isolated files in an arbitrary order.

A Saga gives the reviewer that context first and keeps it attached to the code:

1. The review deck explains the change as a sequence of visual arguments.
   Diagrams, interactive HTML, screenshots, and examples show the architecture
   and the important flows, and expectation/actual callouts expose surprising
   behavior, tradeoffs, hidden coupling, and intentional deviations.
2. Semantic items inside each slide link to the exact diff ranges they explain.
3. `change-saga check --covers review` and `change-saga status` report what is
   unaccounted for.
4. In a Saga grown into living documentation, prototypes and stories also
   establish what the change is for and what done means, design shows the
   flows, interface, data models, and system structure, test cases state how
   each acceptance criterion is verified, and each feature's implementation
   deck explains the current code.

The tool does not review the code or generate a verdict. It helps the author
prepare the material that other people will review. AI is useful here because
it can build the first draft, create diagrams and examples, and iterate until
the whole change is represented. The reviewer still decides whether the change
is correct.

Slides use self-contained SVG, raster, or sandboxed HTML visual entrypoints.
Each slide makes one review argument, and its addressable items carry the exact
implementation evidence. Everything is ordinary files in a `.saga` directory,
kept in small independent records so separate agents or branches can work
without a shared presentation file. [SPEC.md](SPEC.md) defines the format.

Prose citations and visual nodes carry the same evidence requirement. A
Markdown footnote marker and definition are not a finished citation until the
definition is an exact-text landmark with focused diff evidence. Likewise, a
code-bearing diagram node is unfinished without its element landmark and
diffs. Requirements provenance created with `citation add` is different: it
records where a story or decision came from and does not substitute for
implementation evidence.

## Reviewing a saga

`change-saga open` starts a local review application. Opened on its own it
shows the documentation as of the current commit; opened with `--against main`
it compares the branch, showing what the change revised, what it affected, and
the code, grouped by the documentation that explains it.

The header divides the application in two: **Documentation** is what it is, and
**Review** is how it is changing.

Each side's sidebar lists what that side is about. The **Overview** is on both,
because it is what the application is: a reader reviewing a change needs the
same vocabulary and personas as one learning the app. What differs is the
second section — the features, or the reviews.

Documentation holds:

- **Overview** is the application's pitch and description, plus its personas,
  terms and vocabulary, design system, onboarding deck, and feature flags.
- **Features** lists every feature, each opening its own page. Inside a feature
  the sidebar is always Product, Design, Quality, and Implementation, in that
  order. Implementation is the deck itself, open to its slide thumbnails; the
  other sections stay collapsed until you need them, and empty places say what
  is missing.
- **Present** shows the implementation deck full screen, one slide at a time.
  Linked code opens without losing the active slide.

Review holds the reviews and everything that reads a comparison. Its sidebar is
the Overview again, then every review as a row in place of the features:

- **Reviews** lists each pull request's review: a slide deck that explains what
  the change did and why, with the diffs its slides reference. Reviewers approve
  or request changes slide by slide and discuss slides and Items. A decision
  records the commit it was given at and shows as out of date once that slide or
  its code changes.
- **Code Diff** provides a traditional changed-file tree and diff view, with
  links back to every relevant explanation.
- **Coverage** shows the mapping in both directions: code to explanations and
  explanations to code.

The two sides meet on the documentation pages. A feature, story, or acceptance
criterion lists the reviews that changed the code it explains. Nobody writes
that list: it is derived from the diffs themselves, so it is right by
construction and says on the page that it was derived.

The documentation itself carries no comments or approvals; review happens in
reviews. The tool records every decision and never declares a review approved:
what a pull request needs before merging is the team's decision.

Every newly initialized saga also carries a small root `README.md`. It tells a
human or AI assistant how to install and open the intended reviewer, and tells
assistants to ask before downloading or executing anything from PR content.

Review data is stored inside the saga. Each decision and comment gets its own
file, which keeps concurrent Git changes small and avoids shared arrays.
Attribution comes from the commit that adds the record.

## Maintain a codebase Saga

If the team wants it, a Saga can document a repository over its entire
lifetime, not only its pull requests.
Its authoring and maintenance workflow is designed for AI, not manual human
operation. Maintaining exact coverage, granular citations, diagrams, and
structured evidence by hand would be unreasonable. That exhaustive bookkeeping
is precisely the kind of tedious work AI is good at; humans can focus on
understanding and reviewing the result.

**To create a Saga for the whole codebase:**

> Use the change-saga cli to create a Saga for this codebase since inception

**To update the codebase Saga for a PR:**

> Use the change-saga cli to update this codebase's Saga for the changes in this PR

**To update it from an existing PR Saga:**

> Use the change-saga cli to compare this PR's Saga with the codebase Saga and update what changed

## Structured access for AI

Agents do not need to crawl the saga's files. The CLI exposes a bounded,
read-only JSON interface for the overview, hierarchy, content, reviews,
coverage gaps, diff ownership, mapping quality, author claims, and verification:

```sh
change-saga query overview --saga change.saga
change-saga query gaps --saga change.saga --kind uncovered
change-saga query mappings --saga change.saga --sort scrutiny
change-saga query claims --saga change.saga --status unverified
```

`mappings` ranks broad or thin evidence so an AI can start with the weakest
justification. Claims are falsifiable assertions tied to exact code;
verification results are append-only and attributed through Git. An AI review
can inspect the diff cold first, then reconcile its findings against this
structured author account.

Authoring is batchable in the same spirit. `change-saga cover --batch -` reads
newline-delimited JSON records from standard input, resolves the whole batch
before writing anything, and leaves the saga untouched if any record fails:

```sh
printf '%s\n' \
  '{"target":"api.chapter","path":"api.go","side":"new","lines":"18-24","note":"validates the request"}' \
  '{"target":"api.chapter/flow.fragment#submit-action","path":"ui.ts","side":"new","lines":"9","note":"wires the control"}' \
  | change-saga cover --batch - change.saga
```

See [the AI-facing interface](docs/ai-facing-interface.md) for the complete
contract.

For a focused file whose entire change belongs to one explanation,
`change-saga cover --path FILE --changed-lines` derives its exact changed-line
and file-event selectors and stores gapless lines as canonical dense ranges.
Generated evidence paths identify the selector set rather than the authoring
timestamp: unrelated selectors stay in unrelated files, while a different
explanation for the same selectors requires explicit reconciliation. Coverage
summaries can be bounded with `--json` or silenced with `--quiet`. Repair broad
mappings using the `evidence_file` from `query mappings`:
`replace-coverage --record PATH --batch -` atomically splits or retargets one,
while `remove-coverage --record PATH` deletes one.

Evidence references code as of a commit, so it follows the code as it evolves.
When later commits only move the referenced lines, the reference is remapped
automatically; when the lines change, it goes stale and says why. After a change
lands, re-pin references to the landed commit before the branch is deleted:

```sh
change-saga references --stale --diff --repo ../source change.saga
change-saga repin --onto <landed-commit> --branch <branch> --repo ../source change.saga
```

`repin` also records the branch's commit messages, so a squash merge keeps the
reasoning in its individual commits.

## Manual CLI workflow

Most people should let their coding agent manage these commands. If you want to
author a Saga directly, do it in your own repository rather than in this
repository's canonical `change.saga` example. Run with no path, `init` creates
the repository's one Saga, `change.saga`, named after the repository's origin
remote. Then create a review of the branch, explain its architecture on
slides, and cover every changed line:

```sh
change-saga init
change-saga review create change.saga
change-saga apply-slide --review pr-42 --from ./architecture.json --dry-run change.saga
change-saga apply-slide --review pr-42 --from ./architecture.json change.saga
change-saga cover --target urn:change-saga:shop:review:pr-42:slide:architecture:item:cache \
  --path path/to/cache.go --changed-lines change.saga
```

`review create` prints the review id it chose (here `pr-42`) and each command
prints the next. `architecture.json` is one complete slide: its diagram source
and an Item per element that explains part of the change (review Items carry
no code; `cover` adds it). A slide that needs hand-drawn SVG uses `add-slide
--review`, `set-slide-content --review`, and `add-item --review` instead. See what is covered, ask it as CI would, and open the
reviewer:

```sh
change-saga review list change.saga
change-saga check --covers review --against main change.saga
change-saga open --against main change.saga
```

Once the Saga documents the application, a feature's implementation deck
explains the current code in the same way (`add-deck`, `add-slide --deck`,
`add-item --slide`, `cover --against main`); the first command that needs a
feature creates one named after the branch.

`open` leaves the reviewer running in the background so it remains available
after the command returns. Manage it later with:

```sh
change-saga serve status change.saga
change-saga serve stop change.saga
```

Use `change-saga serve --open change.saga` when you deliberately want the
reviewer attached to the current terminal instead.

Run these commands from the changed repository on the branch containing the
work. If the saga lives in a separate repository, pass
`--repo /path/to/source-checkout` to commands that inspect the diff.

### Maintaining a codebase Saga

See what a PR's change does to an existing Saga:

```sh
change-saga status --against <pr-base> --head <pr-head> change.saga
change-saga query layers --saga change.saga \
  --against <pr-base> --head <pr-head> --layer affected
```

A comparison never compares prose, diagrams, or other Saga content. It follows
changed code through the references that explain it and reports the records the
change must update, the records it should prompt you to reconsider, and changes
that need new documentation. It is read-only; use `--json` for CI or an
agent-driven maintenance loop.

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
