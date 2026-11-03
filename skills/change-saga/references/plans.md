# Writing a Plan

Read this when asked to plan work before building it, to draft a technical
design for a change, or to break work into Tasks. Run `change-saga plan guide
change.saga` for the workflow and `change-saga plan guide --plan ID change.saga`
to see what is not yet ready.

A Plan describes a body of work and how it can be executed in parallel. It is
where the technical design for a change is drafted, and its Tasks are how the
work gets done. A review explains what a change did; a Plan says what it will do.
A Plan belongs to no feature. When it changes, it is the same record, committed
again. A different body of work gets its own Plan.

## The rules

- **Every Task leads to a user story.** A Task names one with `--story`, or
  unblocks a Task that leads to one, with `--unblocks`. Anything else is refused,
  refactors included. If there is no story to point at, write the story first.
- **Add Tasks top-down.** Add the Task that names the story, then add its
  prerequisites with `--unblocks`. A storyless Task cannot exist before what it
  unblocks does.
- **A phase is a set of Tasks that can run in parallel.** The next phase follows
  it, so a Task can only depend on Tasks of an earlier phase; anything else is
  refused. Keep dependencies inside one Plan. To reorder phases, add them again
  and `plan remove-phase` the old ones.
- **Size each Task honestly.** Each Task should be reasonably granular: a coherent
  piece of work that one human developer, coding it without AI help, would finish
  in two days or less, which implies it is well understood. Judge it as that
  developer, not by how long an agent would take. If you are not confident,
  research or prototype until you are, or split it, but not so finely that a Task
  stops being a meaningful unit of work. Record the judgment with `--sizing`, and
  never mark a Task `within-2-days` to get past this.
- **Done is a report.** `task done` says the work was finished; it never proves
  the work is correct. Mark Tasks done as you finish them, and update the Plan as
  you learn. A Plan is a working document until it is delivered.

## Authoring

```sh
change-saga plan create --id ID --title TEXT change.saga
change-saga plan add-phase --plan ID --id PHASE --title TEXT change.saga
change-saga task add --plan ID --id TASK --title TEXT --phase PHASE --story STORY_URN \
  --essay-source essay.md --code path/to/file.go:10-40=why --sizing within-2-days --sizing-reason TEXT change.saga
change-saga task add --plan ID --id PREREQUISITE --title TEXT --unblocks TASK change.saga
change-saga task list --plan ID --ready change.saga
```

The deck is authored as a review's is, with `apply-slide --plan ID --from
SLIDE.json` and `deck overview --plan ID --file OVERVIEW.json`; see
[slides](slides.md). Group its slides under `slide.section`,
usually by phase.
A Plan Item carries no evidence and no criterion links: its `record` names a Task
of the Plan (`urn:change-saga:SAGA:plan:ID:task:TASK`) or a story, and the slide
lists those Tasks and their done state in a slide-out. Put code references on Tasks. Read a
Plan's slides with `diagram describe --slide SLIDE_URN`.

Read Plans through `query`, not the rendered reader: `query plans` lists them,
`query plan --plan ID` returns a Plan's essay, phases, Tasks and slides, and
`query task --task ID` returns a Task's essay, story, code and dependencies.

Give each diagram node that represents work an Item that selects the node
(`selector: {type: element, element_id: NODE}`) and names its Task in `record`:

```json
{"id": "read-tides", "rank": 10, "kind": "node", "label": "Read the tide table",
 "description": "The work.", "selector": {"type": "element", "element_id": "read-tides"},
 "record": "urn:change-saga:SAGA:plan:ID:task:read-tides"}
```

That Item is what makes the node interactive. The node highlights on hover, shows a
check once the Task is done, and opens a slide-out of the Task: its state, phase,
story, what it needs and unblocks, its essay, and its code references, each opening
its pinned lines. A node with no such Item is only a drawing. An Item naming a story
opens that story the same way, and a callout stays in the list under the slide.

Attach prototypes with `plan add-prototype`.

## While the work happens

```sh
change-saga task set --plan ID --task TASK --state in_progress change.saga
change-saga task done --plan ID --task TASK --summary TEXT change.saga
```

A change that would leave a Task with no path to a story, or make a dependency
cycle, is refused and names the Tasks. Adjust the chain rather than working around it.

## Reviews and delivery

Link each pull request review that implements the Plan with `plan link-review`.
In that review's deck, an Item whose `record` is a Task's URN says the review
delivers it, and a callout whose `record` is a Task's URN says why the work
departed from it. The Plan's comparison reads those links. It only reports
differences; a person judges whether the work matches.

`plan deliver` is explicit, and is refused until every Task is done, at least
one Review is linked, and every linked Review has merged. A delivered Plan is hidden by default, as a merged
review is (`plan list --all` shows it), still reads, and no longer changes.
