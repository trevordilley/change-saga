**Planned: none of this is built.** Every story here is proposed, and the commands they name are working names. A story marked `must` is what Trevor asked for; `should` and `could` are additions proposed for him to accept or strike.

## The problem {#the-problem}

Change Saga can say what a team means to build (Stories), how it will build it (Plans) and what it built (Reviews). It cannot say what reached users, or what a team must do to get it there. That knowledge lives in a changelog written by hand, a checklist in a document or someone's head, and the memory of whoever released last.

This repository shows the cost. Its changelog dated a 0.5.1 that was never tagged, then folded it into 0.6.0. The first release run for v0.6.0 failed after a test hung for thirty minutes on a race; a re-run of the same tag passed, and the fix landed afterwards. v0.7.0 removed the old work plan, which makes any Saga still holding a `___workplan` directory invalid; the changelog says what to do, but no step asked anyone to try it on a real Saga. Each of these is a thing someone had to know and remember.

## What a release is {#what-a-release-is}

**A release is a named version of the product and everything needed to get it to users.** Every release has:

- **Stories** it will deliver or affect, so it says what changes for people and not only which commits it contains.
- **A version**, supplied by a person when the release is created. It is never computed.
- **Release notes**, public and private, each linked to one or more stories.
- **A checklist**, copied from the repository's template and then changed freely as the work goes on.

A checklist exists so nothing gets lost: the one-off button press somewhere, the thing to toggle off before the release and back on after it. Each step is a specific thing someone does, and it does one of two things: it **proves** something is true, or it **changes** the state of the world. Steps can carry excerpts that say what to know when doing them.

## Where it lives {#where-it-lives}

Releases gets a header entry beside Documentation, Plans and Reviews, with three entries in its sidebar:

- **Release Process Overview:** a whiteboard of how code travels from a developer's machine to users, pinned to the workflows and infrastructure that do it, so it stays grounded in what actually happens.
- **Release Template:** the checklist every release copies first, and where the team writes down its versioning scheme, whether semantic, incremental or anything else.
- **Releases:** the list. A version is set when a release is created.

All of it can be created, read, updated and deleted, and it follows the standard structure: small records that merge cleanly across branches and read well in a pull request.

## What people can do {#what-people-can-do}

- An **architect** can [draw how code reaches users](/requirements/see-how-code-reaches-users) and see when that drawing falls behind the code.
- A **developer** can [create a release from the template](/requirements/create-a-release), [adapt its checklist](/requirements/adapt-a-release-checklist) as they learn, [check off steps](/requirements/check-off-a-step), and [merge release work from several branches](/requirements/merge-release-work).
- A **product manager** can [link the stories](/requirements/link-stories-to-a-release), [write notes tied to them](/requirements/write-release-notes), and [see what shipped and what is next](/requirements/see-what-shipped-and-whats-next).

## What this builds on {#what-this-builds-on}

A release is one new record. Everything else extends something that exists, so this does not repeat the old work plan's mistake of being a second system beside Plans.

| Idea | Uses | Recommendation |
| --- | --- | --- |
| Process overview | Named whiteboards, navigation roles, pinned code evidence and staleness | A new whiteboard role. No new document type. |
| Release's stories and notes | Stories, and URNs that survive moves and `feature deliver` | Plain story URNs, as a Task's `story` is. Required on every note. |
| Checklist | The way Tasks work: `done` is a report, and Git is the history | Extend the idea, not the record. A step is not a Task: a Task must lead to a story and belongs to a Plan's graph, while a step may be "press this button". Do not put release steps in Plans. |
| "What is in this version?" | Milestones, which today name a shared target on a feature | Do not add version numbers to milestones. See the questions below. |
| Merged work | Reviews, Plan links, `___merges`, Git detection of merged reviews | Derive it. Store nothing new. |
| Enabled or not | Flags and `implemented_not_enabled` | Read them. A checklist step tells someone to flip one. |
| Breaking changes | The changelog's **Format** marker and migration steps | A breaking note carrying what to do. Deprecation records stay about documentation and code evidence, and do not take this on. |
| Changelog | `CHANGELOG.md`, GitHub's generated notes | Notes become the source, and a template step puts them in the changelog. |
| Readiness | `status` and `check`: no verdict, teams write their own rules | The same stance: [facts, no verdict](/requirements/see-whats-still-open). |
| Pull request review | Saga-only classification, record comparison | Nothing new: [release changes are reviewed like any other record](/requirements/review-release-changes). |

## The principles it keeps {#the-principles-it-keeps}

- **Git is the history.** A release that should not exist is deleted. A released version's stories are read at its commit, not copied.
- **Links are required.** A note with no story is refused, as a Task with no path to a story is. An agent does the record keeping, so the rule is affordable.
- **The Saga records and teams decide.** Nothing here says a release is ready, and a checked step proves nothing.
- **Nothing new blocks a Saga that does not adopt it.** [A Saga with no releases is unaffected](/requirements/releases-stay-optional).

## Limits {#limits}

Change Saga does not tag, build, publish or deploy, does not toggle flags, and does not choose version numbers. It records what a team's own tools did, and a checklist step is how those tools enter the process.

It also does not check whether a release breaks somebody's Saga. That is one tool's concern and not a release management feature: this repository's template should carry a proof step that runs the new binary's `validate` against a real Saga.

## Open questions {#open-questions}

In priority order. Each has a recommendation; none is decided.

1. **What does "private" mean?** A Saga lives in the repository, and this repository is public, so a private note here would be readable by anyone. Recommendation: private means "not for the public notes", the reader says so plainly, and `release guide` warns against writing secrets there. Truly confidential notes would need storage outside the repository, which this feature does not attempt.
2. **Does a release have a lifecycle?** Trevor's direction does not name one, but "what shipped?" needs one. Recommendation: [one explicit "released" mark](/requirements/mark-a-release-released) with a commit, not refused by unchecked steps, because some steps only happen afterwards. Records stay editable afterwards, since a wrong note should be fixed, unlike a delivered Plan.
3. **What about fixes with no story?** Of v0.7.0's four Fixed entries, a keyword search of current stories found an obvious home for only one, the whiteboard focus fix; that search is a rough test and not an audit. Recommendation: hold the rule. Write the story, or link the story the fix affects, and accept that this makes the record keeping heavier at first.
4. **Is checking a box also done in the browser?** "Checkbox" suggests clicking it. The reader already writes review decisions and comments, so there is precedent. Recommendation: yes for checking steps, and CLI only for everything else at first. A browser check writes to the working tree, so someone must still commit it.
5. **Do release notes replace `CHANGELOG.md`?** Recommendation: notes are the source and the changelog is their published form, placed there by a template step, never written by the tool. A team that keeps a handwritten changelog can.
6. **What becomes of milestones?** They are described as naming "a version or launch", which now overlaps releases. This Saga has none. Recommendation: keep them for targets that are not versions, stop suggesting a version as an example, and migrate nothing.
7. **How do hotfixes and several release lines work?** Recommendation: add no "line" concept yet. A hotfix is a release with its own version and commit. What this leaves open is where its record is written when the fix ships from a branch other than the default, and how it reaches the default branch.
8. **What about apps that deploy continuously?** They have no versions to supply, and a release per deploy would be noise. Recommendation: the overview and template are still useful, and such a team either groups work into dated releases or does not use the list. Worth asking a real team before designing further.
9. **Should the tool notice the outside world?** It could read a Git tag matching a version, as it detects merged reviews, without writing anything. Recommendation: leave this out of the first version, and let a proof step say so instead.
