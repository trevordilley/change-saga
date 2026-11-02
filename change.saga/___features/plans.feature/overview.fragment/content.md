## The problem {#the-problem}

Change Saga explains a change after it is built. Before that, how a feature will be built lives in chat, in someone's head, or in a document nothing links to. Work handed to agents suffers most: if a piece of work is not well understood, nobody finds out until it comes back wrong. And once the work ships, nothing can say whether what shipped is what was intended.

## What a Plan is {#what-a-plan-is}

**A Plan describes the work to be done on a feature and how it can be executed effectively in parallel.** It is its own kind of document, not a review. It goes with a feature, and a feature being updated, or one on the Roadmap, can have several.

A Plan is a slide deck, like a review or a technical design, but organized by implementation: the deck is encouraged to run phase by phase. It has prototypes showing what will be built. Underneath it is a set of Tasks.

A **Task** is one well-understood piece of work. It has dependencies on other Tasks, so what can run in parallel is visible rather than guessed. It relates to the user story it is part of. It can have a small essay and references to the code that matters. The Plan's slides reference Tasks, and a slide shows which of its Tasks are done.

The sizing goal is that a team is confident each Task would have taken two days or less before AI. That is a test of understanding: a Task someone can honestly size that small is one they understand.

## What people can do {#what-people-can-do}

- An **architect** can lay out a feature's work phase by phase, map its dependencies, and be guided through research and prototypes until they are confident of each Task.
- A **developer** can see what can start now, open a Task and find everything needed to begin it, and read how far along a Plan is from its slides.
- As work happens, agents mark Tasks done and update the Plan, so it stays a working document.
- When a Plan is implemented, a reader can **compare it with the Review or Reviews that implemented it**: what we said we would do against what we did. This is the key value of keeping a Plan.
- Once a Plan is implemented and merged, it is done: hidden in the reader and filtered out of the CLI by default, and still there when someone asks for it.

## What is true today {#what-is-true-today}

Nothing here is built. There is no Plan, no Task, and no command or page for either. These stories are proposed. The [prototype](/prototypes/plans) shows the direction using this feature as its own example; its content and done states are illustrative.

## The work plan already covers part of this {#work-plan-overlap}

[Dependency-aware work planning](/features/work-planning) is already built in `internal/workplan`, under `change-saga plan`, though this repository has no records of it and the reader shows none of it. It covers a good part of a Task and little of a Plan:

| A Plan needs | The work plan has | Missing |
| --- | --- | --- |
| Phases | Waves with an objective, order, and entry and exit conditions | Waves belong to a feature, not to a Plan |
| A unit of work | Work items with an objective, deliverables, and completion checks | The essay and the sizing judgment |
| Dependencies | Dependency records with a reason and a condition. Cycles are refused, and `query work-items` reports blockers | A graph anyone can read |
| Parallel execution | Contracts between parallel work, and workspace assignment | Nothing, apart from showing it |
| Done state | Append-only progress events, including `done` and `cancelled` | Slides reading that state |
| A link to the story | An `implements` relation to a criterion or a design | Pointing at a story |
| Code references | Expected touch areas, by selector | Pinned lines of code |
| A link to what shipped | Merge units and merge events, per branch | A link to a Review |
| Several Plans, a deck, prototypes, hiding done work, a wizard, any reader page | Nothing | All of it |

**Recommendation: Tasks are work items, extended. Do not build a second task system.** Dependencies, conflict-preserving progress, and contracts already exist, are tested, and are what a Task needs. A Plan is the new thing: a container that owns a deck, prototypes, and its waves and work items. Readers see Phase and Task. Replacing the domain would mean rewriting that machinery to arrive at much the same model.

**Naming follows from that.** `change-saga plan` is taken by the work plan, and the work plan's job is exactly what the new noun means, so the two should merge rather than compete: `plan` becomes the Plan's grammar, its verbs for phases and Tasks move under it and a new `task` noun, and "work plan" stops being a separate idea. Each feature's `plan.json`, written by `feature set-plan`, is Roadmap status and stays as it is, though the word now means two things there.

## Proposed grammar {#proposed-grammar}

This is a design sketch. None of the new commands exist.

```sh
change-saga plan create --feature FEATURE --id ID --title TEXT
change-saga plan list [--feature FEATURE] [--all]    # done Plans are hidden without --all
change-saga plan link-review --plan ID --review ID
change-saga plan add-prototype --plan ID --prototype URN
change-saga plan add-phase --plan ID --id ID --title TEXT    # today: plan add-wave
change-saga plan guide --feature FEATURE                     # the wizard

change-saga apply-slide --plan ID --from SLIDE.json          # slide.section names its phase
change-saga deck overview --plan ID --file OVERVIEW.json

change-saga task add --plan ID --phase ID --id ID --title TEXT --story URN    # today: plan add-item
change-saga task depend --task ID --on ID                    # today: plan add-dependency
change-saga task set --task ID --essay-source FILE --code PATH:START-END --sizing within-2-days
change-saga task done --task ID [--summary TEXT]             # today: plan progress --to done
change-saga task list --plan ID [--ready]                    # what can start now
```

## Open questions {#open-questions}

1. **Who can object to the personas?** These stories use the architect and developer personas. "Contributor" was asked for, but the Saga has no such persona, and developer is the closest.
2. **Is a Plan discussed and approved, as a review is?** The original idea was yes, per slide. The current direction does not say, so no story covers it.
3. **What exactly makes a Plan done?** "Implemented and merged" could mean every Task is done, every linked Review has merged, or both. What happens when a Review merges with Tasks still open?
4. **What does done on a Task claim?** Work-plan progress today says it "never proves correctness". Should a slide's done mark say only that an agent reported it?
5. **What is a sizing judgment, in the record?** A yes or no with a reason, or a number? Who may make it, and can an agent?
6. **What does the comparison compare?** Tasks against Reviews is simple to report. Slides against slides says more but relies on authors linking them.
7. **How far can an agent update a Plan?** Marking Tasks done is clear. Should it also be free to add Tasks or rewrite slides, or only to propose that?
8. **Where does the wizard live?** A skill, a CLI command, or both, and when has it done enough?
9. **What happens to a Plan when `feature deliver` runs?** A Roadmap feature's Plans presumably move to the recipient with its stories.
10. **Do any external Sagas use the work plan?** If so, moving its verbs and renaming its records needs a migration. Its URNs and schema URLs say `work-item` and `wave`, and the name could stay internal.
