# Visual storytelling for slides and whiteboards

Read before storyboarding a deck or whiteboard, or improving an existing one.
Use [diagrams.md](diagrams.md) for the public authoring API and evidence rules.
This reference sets the editorial standard; it is not a slide template.

The reader should understand **where things fit, what happens, why it happens
that way, and what changes for them**. A valid drawing and complete code
coverage do not establish that understanding. A sparse diagram can omit the
story just as readily as a dense one can obscure it.

## Build the argument before the pictures

Start from the reader's task and a concrete situation. For a review, explain
the previous limitation, the change in behavior or responsibility, the mechanism
that makes it possible, and the consequences a maintainer needs to assess.
For onboarding, establish the real problems and beneficiaries before teaching
internal architecture. Living implementation decks explain how the system works
now; do not invent an old/new contrast for them.

Write a short storyboard: **reader question → answer/title → visual relationship
→ concrete example → evidence → next question**. Read just those titles in order:
they should make an argument, not enumerate modules. Choose enough slides to
answer the questions. A small change may need only a few; a broad change needs
chapters and focused detail rather than squeezing everything into a fixed count.

For example, a session-lifecycle review might lead from “Work continues after
the tab closes” to “The Workspace owns execution,” then “Reconnect restores
observation,” then “Stop ends execution; retained files have a separate life.”
Use these only when the code supports them. This sequence connects a user-visible
outcome to ownership, mechanics and consequences; “Architecture / Runtime /
Recovery / Tests” leaves the reader to reconstruct that argument.

## Sectionality is part of the explanation

Name and draw the boundaries that matter: desktop/cloud, browser/worker,
user-owned/managed, durable/ephemeral, trusted/untrusted, or pipeline stages.
Place elements **inside** their owner with real nested `group` elements. A row
of component boxes with different fills does not communicate containment.

Use tinted `section` groups with title tabs, nested boundaries where needed,
and whitespace between responsibilities. Put a short explanatory sentence beside
the boundary: what it owns, what survives there, or what may cross it. Route
connections across those boundaries deliberately and label the crossing with
the message, data, decision or permission involved. Keep an overview locator
on a detail slide when readers would otherwise lose their place in the system.

Color carries meaning across the whole deck or board. Pick a small palette of
named responsibilities, keep their colors stable, and distinguish a normal
path from a consequential exception with line style and text as well as color.
Use theme-aware tokens and labeled shapes; inspect light and dark mode. Reserve
strong emphasis for the changed mechanism or important consequence. Do not
assign a different color to every box or reuse a domain color to mean failure.

## Put the explanation on the canvas

The title states the answer. The Front gives a short, self-contained summary.
The Back teaches the mechanism with useful detail: actors, ownership, a concrete
input, transformations, output, and relevant recovery or rejection behavior.
Keep names and colors consistent between them. The deck Essay supplies the
connected technical account and links into the slides; it does not compensate
for an unexplained Back.

Use short paragraphs, captions, examples and annotations close to the thing
they explain. A name such as “SessionManager” is not an explanation of its
responsibility. Show the condition and consequence on the picture; keep long
code details, evidence and secondary caveats in inspectors. A reader should
not have to hover every node to learn the main path or a critical limitation.
Callouts retain their exact evidence even when their key point is also visible.

Use the available space to explain, not merely to enlarge four labels. Add a
representative identifier, request, state transition, or observable outcome
when it makes the mechanism tangible. Label illustrative values as examples;
do not invent measurements, guarantees, behavior or requirements. When detail
crowds a slide, split at a meaningful question and retain the explanatory text.

## Choose compositions that teach different relationships

| Reader question | Useful composition | Details that earn their space |
| --- | --- | --- |
| What changed for the user? | Matched before/after scenario | Same action and actors; changed outcome highlighted |
| Where does this responsibility live? | Nested ownership map | Process, storage and trust boundaries; what crosses them |
| What happens to one request? | Sequence or swimlane flow | Concrete input, labeled handoffs, output and important branch |
| What survives a failure? | State transitions or recovery loop | Trigger, retained state, retry/stop conditions and observable outcome |
| How do two stores relate? | Selected entity clusters plus data-flow bridge | Keys, ownership, cardinality, transformations; replication vs derivation |
| Why this design? | Aligned alternatives and consequence | Actual tradeoff, rejected alternative and evidence-backed constraint |

Vary the composition because the questions differ. Reuse a layout when it
expresses the same relationship. Icons, gradients, animations and extra boxes
cannot substitute for an absent mechanism. Reveal order or an interactive
example is useful when it teaches time or causality; the default state must
still explain the subject.

## Worked composition: closing a tab while work continues

**Weak version:** four equal service boxes labeled Presentation, Session,
Reconnect and Stop, with the key guarantees hidden in notes.

**Stronger version:** a title stating the observed behavior, with a small
scenario (“A task is running when its tab closes”) above three named regions:

- **Presentation surface:** the tab and its attach/detach connection. Show the
  connection ending, rather than drawing the worker as stopped.
- **Workspace runtime:** the executing session and the state it actually owns.
  A reconnect path returns to the same identity only if the implementation does.
- **Explicit lifecycle:** the Stop action and its verified effects on execution
  and identity. Keep retained checkout/storage separate from running state.

Below the main path, show a compact failure branch explaining what survives a
transport disconnect and what is lost in a worker crash. Include the recovery
limit visibly; keep evidence and secondary detail in the appropriate Items.
If this becomes two separate arguments, split connection recovery from Stop.
This is an editorial sketch, not a claim that every application has these semantics.

## Worked composition: a delivery loop

DayLight's authored “A failed delivery does not discard chosen work” slide
illustrates the richer standard. It groups the durable local queue, device
shipping worker, local result recording and failure recovery into named sections.
Within them it explains readiness, exact destination identity, distinct outcomes
and retained delivery obligations. The return arrow completes the retry story.
This is a useful composition reference, not a guarantee to copy into another app.

```text
LOCAL QUEUE             DEVICE SHIPPING WORKER          LOCAL RESULT
Durable delivery        Bounded work for one            Acknowledged IDs and
obligation              exact destination               terminal drops differ
      ── ready + due ──→       ── validated result ──────→
      ←───────────────────────┘
        no acknowledgement: restore pending

RECOVERY: what survives, when retry happens, and what a crash leaves behind
```

For a similar mechanism, use a consistent color for each responsibility and a
separate labeled recovery band. Put the input identity and result distinction
on the canvas. Keep the main path legible across the top and the return path
clear below it. Compare this with “Queue → Worker → Database”: the latter names
parts but omits the conditions, outcomes and loop that explain the behavior.
If the actual mechanism drops work, has a retry cap, or lacks durable recovery,
show that fact instead. Preserve those differences rather than copying the example.

## Whiteboards: an overview, then questions worth exploring

The board is a spatial explanation, not an exhaustive table or file inventory.
Start with a readable system picture and a guide to the questions its sections
answer. Use nested regions for ownership and focused subsections for mechanisms.
Preserve important cross-section connections so the board remains one model.

A desktop/cloud example can start with people, locally owned work, explicit
sharing, and shared evidence. A focused journey then follows one selected source
through local storage, delivery and hosted indexing. Entity detail belongs near
that journey, with proper crow's feet for recorded relationships. Transport or
authorization arrows must be labeled distinctly; a conceptual flow is not a
foreign key and two stores are not necessarily synchronized mirrors.

Keep selected keys and fields that explain identity, scope or integrity. Put
remaining detail in focused regions or inspectors. Use question-led sections
such as “Who may read this source?” and “What survives a retry?” alongside clear
domain names. Inspect both the opening view and focused detail at actual reading
scale. Fit-to-board is a map, not the required reading view.

## Calibrate once, then inspect the whole story

Finish one representative, nontrivial visual before scaling out. Inspect it in
the viewer at normal size: does it teach ownership, flow and consequences without
opening every inspector? Use that as the reference for detail, typography and
semantic color, not as a grid to clone. When authoring is already delegated,
share this reference and its storyboard with the authors.

Inspect the contact sheet for narrative progression and appropriate visual
variety, then read each slide at normal size. A new reader should be able to
explain its behavior and consequence from the visible content and locate the
supporting evidence. Fix clipping, weak contrast and tiny text, but also missing
context, unconnected boxes, unlabeled crossings and important facts hidden in
hover notes. Mechanical QA and coverage cannot assess storytelling quality.
