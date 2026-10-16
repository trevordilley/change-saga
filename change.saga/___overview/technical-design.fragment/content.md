# Technical design: a repository is the durable model {#technical-design-a-repository-is-the-durable-model}

Read the design in three views:

- [ERD](/technical/erd) explains the logical records, ownership and optionality. These are file-backed entities, not SQL tables.
- [Infrastructure](/technical/whiteboards/infrastructure) locates the binary, browser, source checkout, Saga directory and disposable caches on the developer workstation, with the build and release services outside that runtime boundary.
- [Data flows](/technical/whiteboards/data-flows) follows authoring, reading, code comparison and review actions through their validation and persistence boundaries.

## Separate intent, design and review {#separate-intent-design-and-review}

Product describes why an application exists: personas, durable features, stories and independent acceptance criteria. Features and stories carry authored essays. A criterion can have a short navigation title and a Markdown explanation while retaining its full pass/fail statement. Explanations can change an obligation, so criterion title or overview changes require evidence to be reassessed; a prettier reading page does not make an old pin current.

Technical design is this approach essay, optional supporting essays, and the three whiteboards. Implementation decks explain mechanisms at a narrower feature scope. A pull-request review deck explains a change between commits. Those artifacts share diagram grammar and semantic links, but they have different jobs: a whiteboard source pin does not own changed-line review coverage, and a review decision does not accept a product requirement. Existing Component and System records remain readable for historical pins; they are not the organizing structure of this design.

## Stable identities, explicit history {#stable-identities-explicit-history}

The Saga manifest establishes a stable namespace and source repository. Features own storage locations, while story identities remain stable across feature moves. Story definitions are complete immutable snapshots; lifecycle events form a separate history. Criteria are embedded in a story revision and inherit its revision pin. Exact relations connect intent to designs, slides and tests. Readers preserve competing heads instead of selecting a winner by timestamp.

Decks contain slides, and slides contain addressable Items. An Item names the visual element it explains and can own precise code evidence. A code reference pins a repository commit, path, selection and digest. Readers can remap moved bytes; changed bytes remain stale until an author reassesses the explanation. The [ERD](/technical/erd) distinguishes this stored evidence from the comparison and coverage projections derived from it.

Whiteboards publish complete immutable diagram documents under independent document IDs. ERD, Infrastructure and Data flows are conventions for the three design views, not different rendering engines. Publication names the observed parent revision; historical views keep their original source pins. Native groups and connections let readers focus on a section without splitting the design into disconnected files.

## Write through a checked boundary {#write-through-a-checked-boundary}

Public CLI commands parse a typed request, resolve the current record and explicit parents, validate content and source references, then publish through the storage layer. Locks serialize cooperating writers. Individual files and staged directories use filesystem publication primitives; multi-file batches attempt rollback but are not globally crash-atomic. A stale parent or invalid request is an error to resolve, not permission to overwrite concurrent work. See [Storage and evolution](/technical/pages/storage) for the boundary and recovery rules.

## Derive views instead of storing verdicts {#derive-views-instead-of-storing-verdicts}

A read combines the Saga with a chosen source revision or comparison. The query application projects a bounded answer with a snapshot and cursor. The browser renders HTML from related application services and loads expensive evidence on demand. Git supplies commits and diff atoms; reference resolution supplies current, moved, missing or stale evidence. Coverage and readiness are derived facts about those inputs, not a declaration that the software is correct.

The server keeps reusable generations outside the Saga. Cache keys bind the source context and Saga fingerprint; unpinnable worktree inputs do not become trusted persistent cache hits. Cache deletion changes latency, not the authored record. The [Data flows board](/technical/whiteboards/data-flows) shows where projections branch toward JSON, HTML and source inspection.

## Trust the boundary, keep uncertainty visible {#trust-the-boundary-keep-uncertainty-visible}

The reviewer listens only on loopback and rejects remote binding. It constrains request hosts, mutation access and asset paths; authored Markdown and embedded visuals pass their own rendering restrictions. Git actions run against the selected source checkout. No hosted MCP service, cloud database or remote application deployment is part of this main-branch runtime. [Runtime and trust](/technical/pages/runtime) explains that layout.

Deprecations are explicit annotations with a reason and either content scope or exact code scope. A replacement is a pointer for a reader, not a claim that all surrounding code is deprecated. Missing evidence, competing definitions and old review decisions stay visible. The design favors inspectable uncertainty over a convenient green summary.

## Use the model to make a change {#use-the-model-to-make-a-change}

Begin with the relevant [feature](/features) and its stories, then use the boards to locate the affected records and flow. Update the implementation and tests. Revise the affected explanation through the CLI, retaining stable identities and all required parents. Check the diagram, validate the Saga and inspect the browser rendering. Where a code reference became stale, read the changed code before accepting a remap or replacement. The [Authoring and review workflow](/technical/pages/workflow) connects these steps to the public commands.
