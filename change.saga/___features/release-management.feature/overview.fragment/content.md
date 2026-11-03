**Planned: none of this is built.** Every story here is proposed, and the commands they name are working names. A story marked `must` is what Trevor asked for; `should` and `could` are additions proposed for him to accept or strike.

## The problem {#the-problem}

Change Saga can say what a team means to build (Stories), how it will build it (Plans) and what it built (Reviews). It cannot say what reached users, or what a team must do to get it there. That knowledge lives in a changelog written by hand, a checklist in a document or someone's head, and the memory of whoever released last.

This repository shows the cost. Its changelog dated a 0.5.1 that was never tagged, then folded it into 0.6.0. The first release run for v0.6.0 failed after a test hung for thirty minutes on a race; a re-run of the same tag passed, and the fix landed afterwards. v0.7.0 removed the old work plan, which makes any Saga still holding a `___workplan` directory invalid; the changelog says what to do, but no step asked anyone to try it on a real Saga. Each of these is a thing someone had to know and remember.

## What a release is {#what-a-release-is}

**A release is a named version of the product and everything needed to get it to users.** Every release has:

- **Stories** it will deliver or affect, so it says what changes for people and not only which commits it contains. Every one of them has code references by the time it ships.
- **A version**, supplied by a person when the release is created. It is never computed.
- **Internal notes** for the team: "v0.8.1" and a bunch of Jira tickets. Each links to stories or is marked as a fix.
- **Public notes** for everyone else: "Unleash the Kraken!", closer to a blog post about why the release is great than a list of changes.
- **A checklist**, copied from the repository's template and then changed freely as the work goes on.

- **A range**: the Git tags or commits it runs between, so what changed comes from Git and not from memory.

A release is also small. Some teams release every sprint, every Thursday or every day, so creating one has to be cheap.

## The checklist {#the-checklist}

The checklist is the steps taken every release so the train does not derail. It is not a list of Tasks: a Task is well-understood work that leads to a story, while a step can be as simple as "tell Mike you're starting the release" or "use the bathroom before next step".

It exists so nothing gets lost: the one-off button press somewhere, the thing to toggle off before the release and back on after it. Each step is one specific thing someone does, which either **proves** something is true or **changes** the state of the world. Steps can carry excerpts that say what to know when doing them, and anyone can tick one in the browser or from the CLI. A tick is a report, and the tool verifies nothing.

## Which tests prove it {#which-tests-prove-it}

End-to-end tests are the ones that let a team sleep at night, and the flakiest and most expensive to run. So a release should run the ones that matter to it and not always the whole suite. With [a catalog of those tests](/features/end-to-end-test-catalog) linked to acceptance criteria, a release's range gives a chain:

1. [The range](/requirements/say-which-range-a-release-covers) gives the changed lines.
2. Changed lines give [the stories and criteria in play](/requirements/see-stories-from-the-range).
3. Those give [the end-to-end tests to run](/requirements/select-e2e-tests-for-a-release).
4. The CLI prints them [in a form a test runner accepts](/requirements/get-tests-in-a-runnable-form).

An agent running the release takes that list, runs it, and [ticks a step with a link to the run](/requirements/prove-a-release-with-its-tests). The results stay in the team's CI.

**This is only as good as the links.** On v0.6.0 to v0.7.0, this repository changed 17,802 lines, deleted lines included, and `status` connects 1,378 of them to a story through the Saga as it stands. A list of tests built on that would be short and reassuring, while most of the change had no say in it. So [every list also says what it could not reach](/requirements/see-what-selection-cannot-reach), and never that it is safe to skip the rest. A fix with no story is allowed, and it shows up there.

## An agent can run it {#an-agent-can-run-it}

**Change Saga is a tool for AI, so the release commands are how an agent conducts a release and how a person guides it.** A developer can [tell an agent to run the release](/requirements/have-an-agent-run-the-release). The agent reads the process and the versioning scheme, asks the CLI for the next step, does it with its own tools, records it, and carries on. A step it cannot do, it asks a person about, and it ticks that step only once they confirm. If the agent stops, a new one continues from the first unticked step.

Steering is ordinary editing: a person adds a step or an excerpt during the run, and the agent reads it next. The reader is for people to see where things stand, and nothing about running a release needs it.

## Where it lives {#where-it-lives}

Releases gets a header entry beside Documentation, Plans and Reviews, with three entries in its sidebar:

- **Release Process Overview:** a whiteboard of how code travels from a developer's machine to users, pinned to the workflows and infrastructure that do it, so it stays grounded in what actually happens.
- **Release Template:** the checklist every release copies first, and where the team writes down its versioning scheme, whether semantic, incremental or anything else.
- **Releases:** the list. A version is set when a release is created.

All of it can be created, read, updated and deleted, and it follows the standard structure: small records that merge cleanly across branches and read well in a pull request.

## What people can do {#what-people-can-do}

- An **architect** can [draw how code reaches users](/requirements/see-how-code-reaches-users) and see when that drawing falls behind the code.
- A **developer** can [create a release from the template](/requirements/create-a-release), [adapt its checklist](/requirements/adapt-a-release-checklist) as they learn, [tick off steps](/requirements/check-off-a-step), [keep internal notes](/requirements/write-release-notes), and [merge release work from several branches](/requirements/merge-release-work).
- A **product manager** can [link the stories](/requirements/link-stories-to-a-release), [write the public notes](/requirements/write-public-notes), [place a release within a milestone](/requirements/place-a-release-in-a-milestone), and [see what shipped and what is next](/requirements/see-what-shipped-and-whats-next).

## What this builds on {#what-this-builds-on}

A release is one new record. Everything else extends something that exists, so this does not repeat the old work plan's mistake of being a second system beside Plans.

| Idea | Uses | Decision or recommendation |
| --- | --- | --- |
| Process overview | Named whiteboards, navigation roles, pinned code evidence and staleness | Recommended: a new whiteboard role. No new document type. |
| Release's stories | Stories, and URNs that survive moves and `feature deliver` | Recommended: plain story URNs, as a Task's `story` is, for hand links. Stories found from the range are computed and not stored. |
| Checklist | Only the stance of Tasks: `done` is a report, and Git is the history | Decided: a step is not a Task, and release steps stay out of Plans. |
| Larger goals | Shared milestones | Decided: [a release sits within a milestone](/requirements/place-a-release-in-a-milestone), optionally. They have different scopes and timelines and need not share a screen. Milestones gain no version. |
| Code behind a story | `query traceability`, Tasks, test-case evidence | Decided: [every story in a release has code references](/requirements/every-released-story-has-code). |
| Which tests to run | `review test-plan`, which already walks changed code to stories to tests, and reports gaps | Recommended: run the same walk over a release's range and narrow it to the catalog, with no second selector. |
| The catalog | Test cases, `verifies` relations, test evidence | Recommended: [a view over them](/features/end-to-end-test-catalog), with no new record. |
| Merged work | Reviews, Plan links, `___merges`, Git detection of merged reviews | Recommended: derive it and store nothing new. |
| Enabled or not | Flags and `implemented_not_enabled` | Recommended: read them. A checklist step tells someone to flip one. |
| Breaking changes | The changelog's **Format** marker and migration steps | Recommended: a breaking note carrying what to do. Deprecation records stay about documentation and code evidence. |
| Changelog | `CHANGELOG.md`, GitHub's generated notes | Recommended: notes become the source, and a template step puts them in the changelog. |
| Readiness | `status` and `check`: no verdict, teams write their own rules | Recommended: the same stance: [facts, no verdict](/requirements/see-whats-still-open). |
| Pull request review | Saga-only classification, record comparison | Recommended: nothing new: [release changes are reviewed like any other record](/requirements/review-release-changes). |

## The principles it keeps {#the-principles-it-keeps}

- **Git is the history.** A release that should not exist is deleted. A released version's stories are read at its commit, not copied, and what a range reaches is computed from Git.
- **Links are required, where they mean something.** Every story in a release reaches code, and an internal note links a story or says it is a fix. An agent does the record keeping, so the rule is affordable.
- **The Saga records and teams decide.** Nothing here says a release is ready, and a ticked step proves nothing.
- **Nothing new blocks a Saga that does not adopt it.** [A Saga with no releases is unaffected](/requirements/releases-stay-optional).

## Limits {#limits}

The CLI does not tag, build, publish or deploy, does not run tests, does not toggle flags, and does not choose version numbers. An agent running a release does those things with its own tools, and the CLI records them. The Saga stores no test results, and no record of which tests are flaky.

"Internal" means for the team, not secret. A Saga lives in the repository, and this repository is public, so anyone could read its internal notes, Jira ticket numbers included. Notes that must stay confidential need to live somewhere else.

It also does not check whether a release breaks somebody's Saga. That is one tool's concern and not a release management feature: this repository's template should carry a proof step that runs the new binary's `validate` against a real Saga.

## Open questions {#open-questions}

In priority order. Each has a recommendation; none is decided.

1. **When can a team trust the test selection?** With 1,378 of 17,802 changed lines reaching a story here, selection alone would not earn anyone a good night's sleep yet. Recommendation: ship it reporting what it missed, leave the decision to run everything with the team, and treat the unreached share as the number to bring down. This repository runs its whole browser suite on every pull request and push to main, so it would not save anything here until that changes.
2. **Where do ticks get committed during a run?** Each tick is a write to the Saga, while a release often tags the default branch, which is usually protected; this repository requires a tag to point at a commit on it. Recommendation: run the release on a short-lived branch and merge it through a pull request when the checklist is done, so the ticks land after the tagged commit and the release records that commit rather than containing it. A browser tick is the same kind of write and needs someone to commit it.
3. **Does a release have a "released" mark?** "What shipped?" and the code reference rule both need a moment to apply to. Recommendation: [one explicit mark](/requirements/mark-a-release-released) with a commit. Unticked steps do not refuse it, because some steps only happen afterwards. A story with no code references does, because that is the one rule the release is held to. Records stay editable afterwards, since a wrong note should be fixed, unlike a delivered Plan.
4. **Which code references count?** Of this Saga's 24 accepted stories, 13 reach code through slide Items. The other 11 are the shipped Plans stories, and only test-case evidence reaches code for all of them. Recommendation: count Items, Tasks and test evidence, so the rule holds for work this repository has already shipped, and accept that a test reference shows where a story is tested, not where it is built.
5. **Do release notes replace `CHANGELOG.md`?** Recommendation: internal notes are close to what the changelog is now, and a template step can put them there; the tool never writes it. A team that keeps a handwritten changelog can.
6. **How do hotfixes and several release lines work?** Recommendation: add no "line" concept yet. A hotfix is a release with its own version and commit. What this leaves open is where its record is written when the fix ships from a branch other than the default, and how it reaches the default branch.
7. **What does a team that deploys on every merge do?** A daily release is an ordinary release. A release for each deploy would be noise. Recommendation: such a team groups work into releases on whatever rhythm it reports on, or uses only the overview and template. Worth asking a real team before designing further.
