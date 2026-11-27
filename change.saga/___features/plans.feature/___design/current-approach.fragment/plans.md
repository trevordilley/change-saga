# Plan ownership, execution context and review comparison {#plan-ownership}

A Plan owns intended work under one stable identity; a Review explains the source change that implements it. The Plan's essay and phase-organized deck can exist before implementation. A diagram Item may name a Task so the reader can open the work behind a node. Changes edit that Plan's current records, while Git retains previous intentions. The reader exposes Plans separately from Documentation and Reviews.[^reader]

## Tasks must lead to an outcome {#task-outcomes}

Each Task belongs to one Plan. It either names a canonical story or unblocks Tasks that eventually reach one. The validator walks that graph before publication, rejecting unknown dependencies, cycles and changes that strand an existing Task. This allows prerequisite work without inventing a user story for every internal activity. A concrete example is a storage Task that unblocks a story-bearing UI Task: the storage Task's page shows the chain to that UI story rather than pretending it has direct product intent.[^graph]

Dependencies, not completion percentages, determine which Tasks can start. Tasks assigned to a phase may run together; a dependency from a phased Task must point to an earlier phase. The reader projects both prerequisites and dependents and distinguishes ready work from work held by an unfinished prerequisite. One Plan may span several features because story identity does not include feature ownership.[^graph]

## Context travels with the Task {#task-context}

The Task page combines its authored essay, direct or transitive story path, dependencies and exact code references. Code opens in the selected pinned context; a link is evidence context, not a claim that the implementation is complete. The same graph derives the Plans shown from story and feature pages, including after a story moves. There is no separately maintained Plan-to-feature list to go out of sync.[^reader]

Task sizing records an author's confidence that the work is understandable and would take one developer at most two days before AI assistance. Missing or larger sizing remains visible. It is a judgment, not an automatic estimate. The guide instructs the author or agent to research existing stories and code, present findings and uncertainty, prototype an unclear experience, and revisit that research before recording confidence. The command supplies and checks a workflow; it is not an autonomous research service.[^guide]

## Keep the explanation and progress connected {#plan-progress}

Plan slides name their Tasks through semantic Items. The reader derives done state from the Tasks when displaying those slides, so marking work done does not require republishing a diagram. CLI updates pass through the same checked Plan/Task model: an agent can update content or mark a Task done, but it cannot bypass the story path and dependency rules merely by using machine-readable commands.[^reader][^graph]

Prototypes are references attached to that same Plan. Several can coexist to explore alternatives, and each opens its own saved experience. They support design confidence without becoming a second Task system or a test result. Planning uses `plan` and `task`; the current record model has no separate workplan directory.[^reader][^guide]

## Compare intention with implementation {#plan-review-comparison}

Explicit Review links connect a Plan to one or more implementations. The comparison places planned Tasks beside the Review slides that name them, reports planned work with no delivered explanation, and lists Review work outside the Plan. A person assesses the differences; the display does not decide that the change matches the intention. Checkpoints additionally preserve the designated passer and explicit expectations without merging or deploying code when a pass is recorded.[^reader][^delivery]

Delivery is explicit. Every Task must be done, every linked Review must have merged, and checkpoints must be passed before the writer records delivery. Delivered Plans are hidden by default but remain readable through the delivered filter and their stable links. Their history and comparisons remain available, while further edits are refused. Task completion alone is therefore neither Plan delivery nor proof that a story's implementation passed verification.[^delivery][^reader]

[^graph]: Plan Task graph validation and checked mutation preserve paths to stories.
[^reader]: Plan reader projections join Tasks, stories, prototypes, slides and Reviews.
[^guide]: The Plan guide makes research, prototypes and sizing an explicit author workflow.
[^delivery]: Plan delivery and edit guards retain explicit completion boundaries.
