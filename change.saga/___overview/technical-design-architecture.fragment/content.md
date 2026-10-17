The [Architecture whiteboard](/technical/whiteboards/architecture) starts with the codebase map and has focused sections for [mergeable history](/technical/whiteboards/architecture?focus=mergeability), [cache validity](/technical/whiteboards/architecture?focus=cache-validity), and [shared presentation](/technical/whiteboards/architecture?focus=presentation). The [technology stack](/technical/pages/technology-stack) is the shared reference for implementation choices. These views describe cross-cutting mechanics; a feature deck should explain how its capability uses them.

## Dependency boundaries are explicit Go calls {#dependency-boundaries}

CLI commands and HTTP handlers compose domain packages rather than asking a dependency-injection container to assemble the application. The CLI also starts the server. Requirements, Saga narrative, quality, semantic links, evidence resolution and diagram rendering remain separate packages. This is a pragmatic package boundary, not a promise that every package is independent: the server still composes many domains and its application object owns several caches.

One concrete seam is `coverage.Resolver`: coverage asks an interface to resolve a code reference against a view. The production implementation is `coderesolve.Resolver`. This separates coverage accounting from the mechanics of locating source bytes and gives tests a replaceable boundary. It does not imply a universal plugin system, factory pattern or runtime service container.

## Small records preserve parallel intent {#mergeable-records}

Stories have stable identities, immutable definition revisions and separate lifecycle events. Independent edits add files rather than rewriting a shared array. If two authors revise the same definition, both heads can survive a Git merge. Readers expose the disagreement; they do not choose a winner by timestamp. Reconciliation is an explicit authoring operation against the observed parents.

The publication layer serializes cooperating local writers with locks. Individual file and directory publication uses filesystem primitives; multi-file batches stage their writes and attempt rollback after a publication error. A batch is neither reader-atomic nor crash-atomic across files. Presentation metadata, including whiteboard role assignments, is mutable and still needs observed-owner checks. Mergeability reduces accidental contention; it does not eliminate semantic conflicts or all mutable files.

## Cache keys describe the question being answered {#cache-validity}

The server shares freshness checks across requests and derives separate keys for the navigation outline, documentation, all files and source-dependent projections. Documentation parts are loaded lazily once per fingerprint. Changing the underlying inputs replaces the generation rather than trusting a global time-to-live.

Review coverage has a narrower key: commit range, repository identity, Git attributes and the deck Items' code references. Failed or provisional evidence resolution is not retained as a stable coverage answer. Persistent snapshot generations use the Saga location, tree fingerprint and source identity; unpinnable WORKTREE inputs are not admitted to that cache. Staging then renaming publishes a complete generation. These derived files live outside the Saga and can be deleted without changing authored knowledge.

The browser defers another kind of work. Ordinary documentation navigation does not load the deck viewer. Opening a deck fetches its markup, and frame prefetch is bounded around the active slide. This keeps hidden viewers out of ordinary navigation, but the first viewer open still renders all embedded deck markup. That remaining cost matters for large Sagas.

## Reuse presentation without merging semantics {#shared-presentation}

Essays, slide decks and whiteboards are the three presentation families. Stories and acceptance criteria compose Essay framing while retaining requirement semantics. Essays share safe Markdown rendering. Structured slide visuals and whiteboards share the diagram grammar and deterministic SVG renderer; the surrounding reading experience differs.

Slides have stable Items that can own exact code evidence. Whiteboards have independent complete-document histories and focusable sections. Their source pins do not grant changed-line review coverage. Sharing drawing code must not collapse those ownership rules.

The browser uses server-rendered HTML with htmx page swaps inside a persistent shell. Plain JavaScript owns viewer interaction; URLs carry reading locations, and page-scoped abort/cleanup releases handlers when a page leaves. Session response caches are discarded when the incoming Saga state changes. There is no Redux or TanStack Query layer to duplicate this state model.

## Where feature explanations fit {#feature-context}

A storage feature can point to the mergeability section and then explain its own identity, parent and publication rules. A navigation feature can point to cache validity and then explain which request or projection it defers. A visual feature can point to shared presentation and then explain its own interaction and evidence ownership. Use the [ERD](/technical/erd) for record relationships, [Infrastructure](/technical/whiteboards/infrastructure) for runtime resources, and [Data flows](/technical/whiteboards/data-flows) for the end-to-end journeys.
