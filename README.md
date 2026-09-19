
# Change Saga

[![CI](https://github.com/twentyideas/changesaga/actions/workflows/ci.yml/badge.svg)](https://github.com/twentyideas/changesaga/actions/workflows/ci.yml)
[![Browser E2E](https://github.com/twentyideas/changesaga/actions/workflows/e2e.yml/badge.svg)](https://github.com/twentyideas/changesaga/actions/workflows/e2e.yml)
![status: experimental](https://img.shields.io/badge/status-experimental-orange)
![license: MIT](https://img.shields.io/badge/license-MIT-blue)
[![Made with ❤️ using DevSwarm](https://img.shields.io/badge/Made%20with%20%E2%9D%A4%EF%B8%8F%20using-DevSwarm-5F2AFF?labelColor=0A022E&logo=data%3Aimage%2Fpng%3Bbase64%2CiVBORw0KGgoAAAANSUhEUgAAAEAAAAAiCAQAAABFXBcEAAACEElEQVR42s1Y63mDMAw8ugErsAIdwR2BjsAKrMAKWSEdgRXICGQEMsL1RwhIIIFpXhX%2F7E%2F2WY%2BTBKCEAFhxko7TemDPGOmZXzUAQumUt3VHCIKpUimGVRA8MlaONx2ApYKWcg0CAfAgFJrhEAAsuEeKQQsEG7F%2BWLEBQTBXx4Tx%2FSnbXQDa61sJzM%2FMXRss0QpDVtwrlXCeYdWbJNP1CVjgOO5c8JmcCSABM7RIx50TTo4RMwRHv0E27nwnP5wuVuHXyRfQfgFZiB39aUfVYkdn1jIUCYC19iFsH%2BoAUx%2FAoP09njGDNgvF4Zo%2BIopJsnQtAOhklVlUKhvkAoIXKPTSz6UTAmC2tBbdAJcsp5iM0%2Fu7eACDRq3OmgDoO8JwCl2ycNNvC4AO5lqcZlh5aeae2Yg5M9l%2FldFXz9M0Xw4A5uknENvsvwFgYdGjY9GO6VVhVv1oxQXja5qhG2DHVMVF1AYRte1fASxqZ%2BMyRaaviWP%2Frap%2BS8fOqQwK2gcuQjPFK0TfuOKC5mEuaNdc8O4gxDPSMI1NQw4KRt%2F2FCLKjIK3QcX13VRcbVCx2XLLUOz3AYATU5wX%2FICpGr65HI8NSbe85IENSWGPLv%2BjJdtsSuvoprSziH2tKfXb8jO%2BHtaW52iEvtWWv28wecVoFiJHs7cPp%2BZ4Xt49nhc7xnNYE703uqz9oAjiB4VBy1J%2BAQwDuoYAr7YrAAAAAElFTkSuQmCC)](https://devswarm.ai)

Change Saga is living documentation for an application, kept honest by the
code. It records what the application is for, who it serves, what it must do,
how it is designed and verified, and exactly which code implements it, and it
stays current as the application changes. With AI, a big change is often
fastest to build in one large pull request; that speed is no reason to lose
what the change was meant to do.

A Saga documents one application:

- **Overview**: the project's name, elevator pitch, a short description, and
  its **terms and vocabulary**: the words the team uses that a newcomer would
  not know, each linked to the stories and the exact code (usually an enum or a
  constant) that define it.
- **Personas**, a **design system**, an **onboarding deck**, and **feature
  flags**.
- **Epics**, the durable areas of the product. Each has the same four parts,
  which never reorder as work progresses: **Product** (prototypes, and user
  stories with acceptance criteria), **Design** (UX flows, UI references, and
  technical design), **Quality** (test cases and their evidence), and
  **Implementation** (a slide deck whose visual elements reference the exact
  code they explain).

Every pull request gets a **review**: a slide deck explaining what the change
did and why, which must account for every changed line, and whose slides
reviewers approve one by one.

Code maps back to user stories through designs, specifications, and test
cases rather than hand-written code-to-story links. Every link pins what it
relied on: evidence pins code at a commit and follows it as it moves, and
records pin the revisions they depend on. When a story or the code changes,
whatever depended on the old version becomes visibly stale, and the tool says
exactly what to revisit. Nothing is demanded up front: a first change needs only
its implementation explained, and the rest of the Saga grows from there.
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

This repository keeps its own Saga for the change that defines this format.
After installing Change Saga, open it from a source checkout with:

```sh
change-saga open requirements-design-quality-lifecycle.saga
```

## Quick start

Change Saga is designed to work with the coding agent you already use. Give it
these prompts from the repository containing your change:

**To install the Change Saga skill:**

> Use the change-saga cli to install its skill for this coding agent

**To author a PR's saga:**

> Use the change-saga cli to create a Saga for this PR

**To review a PR's saga:**

> Use the change-saga cli to open this PR's Saga

### How a Saga grows

Start with the change in front of you. The first time you use Change Saga on a
pull request, the only thing it asks is that the implementation is explained:
`init`, author a deck, and reference every changed line from the Item that
explains it. Nothing else is demanded up front: no personas, no stories, no
design.

From there the Saga grows a step at a time. `change-saga status` reports a
coverage report for six areas (implementation, stories, personas, design,
quality, and the health of what already exists) with every gap listed, and
offers growth suggestions tied to the change: "this change touched checkout; no
story says why; capture the checkout story?" Each suggestion explains the
practice it teaches and the one command that acts on it; take it or ignore it.
Over time prototypes, stories, design, test cases, and terms fill in, and a
discovery during implementation becomes an explicit revision of the story it
changes.

`status` reports; it never passes a verdict, and it exits 0 whenever it can
produce a report. Your team decides what must be true before a merge, and can
ask directly:

```sh
change-saga check --covers implementation --against main app.saga
change-saga check --covers implementation,stories --against main app.saga
```

`check` exits 0 when every named area is covered and 3 when one has a gap,
printing only those gaps. Nothing is reduced to a score.

Saga files are built for parallel work. Separate agents can own story
revisions, prototypes, design, test cases, and work items, then merge the Saga
alongside the implementation as the work fans out and converges.

### Expect to iterate

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

1. Prototypes and stories establish what the change is for and what done means.
2. Design shows the flows, interface, data models, and system structure.
3. Test cases state how each acceptance criterion is verified.
4. The implementation deck explains the change as a sequence of visual
   arguments. Diagrams, interactive HTML, screenshots, and examples show the
   important flows, and expectation/actual callouts expose surprising
   behavior, tradeoffs, hidden coupling, and intentional deviations.
5. Semantic items inside each slide link to the exact diff ranges they explain.
6. `change-saga status` reports what is missing, stale, or unaccounted for.

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

- **Saga** presents the whole change. Its sidebar is always Product, Design,
  Quality, and Implementation, in that order. Implementation is the deck
  itself, open to its slide thumbnails; the other sections stay collapsed
  until you need them, and empty places say what is missing.
- **Present** shows the implementation deck full screen, one slide at a time.
  Linked code opens without losing the active slide.
- **Code Diff** provides a traditional changed-file tree and diff view, with
  links back to every relevant explanation.
- **Coverage** shows the mapping in both directions: code to explanations and
  explanations to code.

- **Reviews** lists each pull request's review: a slide deck that explains what
  the change did and why, with the diffs its slides reference. Reviewers approve
  or request changes slide by slide and discuss slides and Items. A decision
  records the commit it was given at and shows as out of date once that slide or
  its code changes.

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

A Saga can document a repository over its entire lifetime, not only one PR.
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
change-saga query overview --saga checkout.saga
change-saga query gaps --saga checkout.saga --kind uncovered
change-saga query mappings --saga checkout.saga --sort scrutiny
change-saga query claims --saga checkout.saga --status unverified
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
  | change-saga cover --batch - checkout.saga
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
change-saga references --stale --diff --repo ../source checkout.saga
change-saga repin --onto <landed-commit> --branch <branch> --repo ../source checkout.saga
```

`repin` also records the branch's commit messages, so a squash merge keeps the
reasoning in its individual commits.

## Manual CLI workflow

Most people should let their coding agent manage these commands. If you want to
author a saga directly, the basic workflow is:

```sh
change-saga init --base main --head HEAD --title "Checkout rewrite" checkout.saga
change-saga add-chapter --title "Backend" checkout.saga backend
change-saga add-fragment --section backend.chapter --type markdown \
  --id request-flow --title "Request flow" checkout.saga
change-saga set-fragment-content --target request-flow --source ./request-flow.md \
  checkout.saga
change-saga add-landmark --target backend.chapter/request-flow.fragment \
  --heading-id request-validation --label "Request validation" checkout.saga
```

Check for unexplained changes, then open the review UI:

```sh
change-saga status checkout.saga
change-saga open checkout.saga
```

`open` leaves the reviewer running in the background so it remains available
after the command returns. Manage it later with:

```sh
change-saga serve status checkout.saga
change-saga serve stop checkout.saga
```

Use `change-saga serve --open checkout.saga` when you deliberately want the
reviewer attached to the current terminal instead.

Run these commands from the changed repository on the branch containing the
work. If the saga lives in a separate repository, pass
`--repo /path/to/source-checkout` to commands that inspect the diff.

### Maintaining a codebase Saga

See what a PR's change does to an existing Saga:

```sh
change-saga status --repo /path/to/checkout --against <pr-base> --head <pr-head> codebase.saga
change-saga query layers --saga codebase.saga --repo /path/to/checkout \
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

Change Saga requires Go 1.26.1:

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
