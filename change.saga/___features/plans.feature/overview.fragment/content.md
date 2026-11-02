## The problem {#the-problem}

Change Saga explains a change after it is built. Before that, how the work will be done lives in chat, in someone's head, or in a document nothing links to. Work handed to agents suffers most: if a piece of work is not well understood, nobody finds out until it comes back wrong. And once the work ships, nothing can say whether what shipped is what was intended.

## What a Plan is {#what-a-plan-is}

**A Plan describes a body of work and how it can be executed effectively in parallel.** It is its own kind of document, in its own section: Documentation is what is, Plans is what we will do, Reviews is what we did.

A Plan is where the technical design for a change is drafted, and its Tasks are how the work gets done. It is a slide deck, like a review, but organized by implementation: the deck is encouraged to run phase by phase. It can have prototypes showing what will be built. Underneath it is a set of Tasks. When a Plan changes it is the same Plan, committed again; Git keeps every earlier version. A different body of work gets its own Plan.

A **Task** is one well-understood piece of work. It has dependencies on other Tasks, so what can run in parallel is visible rather than guessed. It can have a small essay and references to the code that matters. The Plan's slides reference Tasks, and a slide shows which of its Tasks are done.

## Everything links to a story {#everything-links-to-a-story}

**Every Task must lead to a user story, with no exceptions for refactors or infrastructure.** A Task need not name a story itself. It can unblock a Task that does, or one that leads to one. But there has to be a story somewhere, and a Task that reaches none is refused. That also gives a clear chain of dependencies: a refactor with no story of its own can show exactly which story it exists to serve.

A Plan belongs to no feature: its stories come from its Tasks, and its features come from those stories. So a Plan can cross features, a feature or story can list the Plans touching it, and nobody keeps those lists by hand.

Referential linking is the point of Change Saga. Keeping records this thoroughly was hard for people, even before AI, and is realistic when an agent does it. The cost is deliberate: a team that wants a Plan must have stories first, including a team that has used only Reviews until then.

The sizing goal is that a team is confident each Task would have taken two days or less before AI. That is a test of understanding: a Task someone can honestly size that small is one they understand.

## What people can do {#what-people-can-do}

- An **architect** can lay out a body of work phase by phase, map its dependencies, and be guided through research and prototypes until they are confident of each Task.
- A **developer** can see what can start now, open a Task and find everything needed to begin it, and read how far along a Plan is from its slides.
- As work happens, agents mark Tasks done and update the Plan, so it stays a working document.
- A Plan and the Reviews that implement it reference each other, and share one Git history, so checking out a Review gives the Plan as it stood. Reviews come after the Plan's work is done. **Comparing them is the key value of keeping a Plan**: what we said we would do against what we did.
- When every Task is done and its Reviews have merged, with at least one linked, an author marks the Plan delivered with an explicit command. It is then hidden from the Plans section and from the text of `plan list` by default, as merged Reviews are, and still there when someone asks for it.

## What is true today {#what-is-true-today}

Plans and Tasks are built. These stories are proposed, because the people who asked for them have not yet accepted them. [Plans and Tasks: first version](/plans/plans-first-version) is the first real Plan: it holds the technical design for this feature and the Tasks that built it. The [prototype](/prototypes/plans) shows the direction explored beforehand, and its content and done states are illustrative.

A Plan's slides are shown as images, so element notes and the reader's theme toggle do not apply to them yet.

## One way to plan work {#one-way-to-plan-work}

Plans and Tasks replaced an earlier work plan that was built but never used. Git has it, and [the Plan](/plans/plans-first-version) says what its removal gave up. Each feature's `plan.json`, written by `feature set-plan`, is Roadmap status and is a separate thing, though the word means two things there.

## Using it {#using-it}

```sh
change-saga plan create --id ID --title TEXT
change-saga plan list [--feature ID] [--story URN] [--all]   # text hides delivered Plans without --all
change-saga plan guide [--plan ID]                           # the workflow, and what a Plan still needs
change-saga plan add-phase --plan ID --id ID --title TEXT
change-saga plan add-prototype --plan ID --prototype URN
change-saga plan link-review --plan ID --review ID
change-saga plan deliver --plan ID                           # refused until every Task is done and a linked Review, all merged

change-saga apply-slide --plan ID --from SLIDE.json          # slide.section names its phase
change-saga deck overview --plan ID --file OVERVIEW.json

change-saga task add --plan ID --phase ID --id ID --title TEXT (--story URN | --unblocks TASK)
change-saga task depend --plan ID --task ID --on ID
change-saga task set --plan ID --task ID --sizing within-2-days --sizing-reason TEXT --code PATH:START-END
change-saga task done --plan ID --task ID [--summary TEXT]
change-saga task list --plan ID [--ready]
```

## Open questions {#open-questions}

1. **Is a Plan discussed and approved, as a review is?** The original idea was yes, per slide. It is left out of this version, so no story covers it; the deck format would allow adding it later.
2. **What happens when a Review departs from its Plan?** The comparison shows it, but nothing here says what should happen next.
3. **How far can an agent update a Plan?** Marking Tasks done is clear. The commands also let it add Tasks or rewrite slides, and whether it should is untested.
4. **What happens to a Plan's technical design when it ships?** It describes intent, and Documentation describes what is. Does it stay with the delivered Plan while someone writes the living documentation, or does some of it move?
