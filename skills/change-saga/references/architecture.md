# Architecture: understand the codebase before a feature

An Architecture whiteboard explains **where code lives, what may depend on
what, and how the important mechanisms fit together**. A reader should locate a
feature in that structure before opening its implementation deck. Use the shared
whiteboard format, not a new visualization type or a Systems/Components inventory.
Read [visual storytelling](visual-storytelling.md) for visual language and
[diagrams](diagrams.md) for the publication contract and current CLI limitations.

## Gather evidence before drawing

Build a compact evidence ledger: claim or edge, exact file/function and revision,
observed behavior, intended rule, and uncertainty. Keep it with the generation
work so another reader can check the board. Inspect the actual implementation:

- **Dependencies:** read package manifests and actual imports in the authored
  scope, resolving workspace aliases and public entry points. Count import sites
  per package pair and retain their locations. Distinguish production, test-only,
  type-only and dynamic dependencies where relevant. A manifest declaration is
  not an observed import; a folder name proves neither a dependency nor a layer.
  State the scan scope and exclusions rather than implying whole-repo coverage.
- **Repository rules:** read AGENTS.md, lint configuration and boundary checkers.
  Inspect and run the relevant checker in its read-only mode; put its command,
  result and scope beside the map. If unavailable or not run, say so. A passing
  import checker does not establish runtime isolation or architectural quality.
- **Composition roots:** find each process entry point and follow what it
  constructs and passes to whom. Name the actual container, factories or plain
  constructors. Plain constructor wiring is a finding; do not promote it to a
  dependency-injection framework or invent ports from suggestive names.
- **Ports and gaps:** trace declarations to implementations, callers and real
  wiring. Look for unimplemented ports, adapters with no port, test-only wiring,
  and deprecated code that is still composed. Show gaps that affect the story.
- **Status versus intent:** compare code with project decisions and requirements.
  Mark differences explicitly. Planned, implemented, integrated, verified and
  deployed are separate claims; accepted stories do not prove implementation.

Reread the evidence when code changes. Tenancy, mergeability or cache correctness
are guarantees only to the extent the code and its callers enforce them. Do not
infer historical rationale from structure.

## Use a recognizable board shape

**Title and subtitle:** state what is being described and its status honestly.
For example, “Prototype architecture; production integration is planned” is useful
only if supported. Never copy another project's deployment status.

**Top legend:** color means responsibility consistently across sections. Pick
responsibilities the code has: browser, process/composition, use case or
transport, adapter, domain/contracts, infrastructure, engineering/lab tooling.
Use labels and line styles as well as color:

| Line | Meaning |
| --- | --- |
| Solid gray | Observed code import; arrow runs from importer to imported code |
| Dashed accent | Runtime message or data movement; label what crosses |
| Dashed amber | Permitted exceptional coupling or divergence from intent; explicitly say which |
| Red with a violation label | Observed dependency forbidden by a cited rule |

An allowed dependency is not automatically an observed dependency. Make that
distinction explicit; do not draw hypothetical violations as if they exist.

**Section A: codebase map.** Show deployable apps, reusable libraries and
engineering tools inside their actual nested boundaries. Put a rules panel
alongside it: what each layer may import, where the rule comes from, and the
checker result. Include a sentence explaining each major responsibility. This
opening map must read on its own, without requiring the detail sections.

**Sections B onward: questions the code can answer.** Give each a question title,
a short visible answer, a locator chip naming its place in the map, and a diagram
using concrete files, functions, interfaces or tables. End with an exceptions
callout: where the rule bends, what is missing, or the limits of the evidence.
Do not invent an exception to fill a box. Useful questions include:

- Where are concrete dependencies selected? Follow one real caller through the
  seam and show what remains coupled.
- What crosses process boundaries at runtime? Distinguish imports from network
  traffic, serialization and trust boundaries.
- Who owns durable, derived and transient state? If Redux and TanStack Query
  coexist, show their actual division and invalidation; those tools are examples.
- What keeps concurrent workers safe? Show identities, ordering, retries and
  coordination limits rather than just naming a locking pattern.
- Where does tenant identity come from? Trace its binding and enforcement, not
  merely the field that carries it.
- What makes parallel work mergeable? Show immutable versus mutable records,
  conflict handling and operations that still require coordination.
- What makes reads fast without returning stale content? Show cache keys, reuse,
  invalidation and deferred work, with the relevant exception paths.

For a substantial codebase, aim for five to seven evidenced questions. Drop
unsupported questions; a smaller system needs fewer. A board of roughly 250–300
elements over six to eight sections can work when focused sections are readable;
that is permission for depth, not a quota. Use short labels, useful icons and
nearby callouts instead of paragraphs in every node. Keep the main mechanism and
important limitations visible without hovering. Readers explore sections through
`?focus=SECTION_PATH`; get exact paths from `whiteboard describe`.

## Generate, validate and inspect

1. **Discover before publishing.** Read `whiteboard list --json` and the existing
   board through `whiteboard get` / `whiteboard describe`. Preserve its identity,
   authored source pins and observed revision. A conventional ID such as
   `architecture` may already own its role implicitly; reread the list after
   publishing and assign only if needed, using the observed owner as `--expect`.
   See [reorganize](reorganize.md) for reassignment.
2. **Generate source with a small script.** Keep stable semantic IDs, palette and
   geometry in one place. Lay out in absolute coordinates if convenient, then
   convert nodes, edge points and label boxes to their parent-local coordinates.
   Nothing automatically lays out, resizes or reroutes. Use responsibility styles
   from `@diagram-<color>-tint`, `-accent`, `-ink`, plus `@accent` and `@amber`;
   discover supported tokens rather than hard-coding hex colors. Estimate text
   height and reserve lanes and space for titles, labels and callouts.
3. **Iterate on a scratch copy.** Copy the Saga intact to a temporary workspace;
   use only public CLI mutations there, with the original repository supplied to
   readers that need code. Preserve source identity and history. Do not copy
   generated metadata back into the real Saga. Publish generated diagram JSON to
   the scratch Saga. Read overflow errors and adjust the named element, its
   containing section and affected routes together. Failed validation is not a
   reason to shrink everything. Successful scratch updates need fresh revision
   IDs and the current scratch `--parent`. Bound automated repair retries; if
   fitting stalls, redesign the section rather than growing boxes indefinitely.
4. **Inspect the reader, every time.** Whiteboards currently have no `visual-qa`
   selector or publish dry-run. Open the scratch Saga with `change-saga open
   --repo SOURCE_REPO --open=false SCRATCH_SAGA` and use its returned URL. With
   Playwright or available browser tools, capture the opening/full board and each
   focused section in light and dark mode. Inspect at normal reading size for
   overlaps, edges crossing nodes, crowded lanes, labels on lines and unreadable
   text. Fit-to-board is orientation, not a substitute for focused reading.
   Re-render changed sections. Stop only this scratch reader with `change-saga
   serve stop SCRATCH_SAGA` when finished. If browser tooling is unavailable,
   report visual verification as incomplete rather than claiming it passed.
5. **Fact-check with a fresh reader before the real publication.** Where delegation
   is available and authorized, use a sub-agent that did not write the board.
   Give it the board's public description/source, the evidence ledger and the
   code, not just the author's conclusions. Check every label, edge, note,
   callout and citation against code and relevant callers. Scrutinize “all,”
   “every” and “one table”; caller behavior attributed to a callee; one package's
   module format generalized to the repo; and inferred rationale presented as
   fact. Return claim, evidence and correction. Fix the board and visually
   recheck changed sections. If no fresh reader is available, disclose that limit
   and perform a separate evidence pass yourself.
6. **Publish the stable result through the CLI.** Recheck the real board's current
   revision and role owner; reconcile concurrent changes rather than overwriting
   them. Publish the final source once with a new revision and that real
   `--parent` (omit it for a new board). Scratch parents are not real parents.
   Run `whiteboard check`, validate the Saga and verify final section links.

## Connect the board to the Saga

The approach Essay links to each section using
`/technical/whiteboards/BOARD_ID?focus=SECTION_PATH`. Supporting named boards can
hold mechanisms that need independent space or history; Architecture still has
one role assignment. Link those boards from the approach as well.

Maintain one authoritative technology-stack Essay with `overview
set-technical-design --page technology-stack --title "Technology stack"
--subtext "Runtime and engineering choices, with their evidence" --source FILE`.
Separate runtime technologies from build/test tooling. Give each meaningful
technology one entry with its responsibility, constraints, source and rationale:

| Rationale label | Required grounding |
| --- | --- |
| Decision | Cite the project decision or requirement that states why |
| Provenance | Cite its recorded origin or extraction history; origin is not a design rationale |
| Inference | Label the reasoning as yours and cite the facts it rests on |
| Unknown | State that the reason is not established |

Package manifests establish declared use/version, never why a team chose a tool.
Supporting pages are discoverable from the Technical Design essay and their
`/technical/pages/ID` URLs, not extra sidebar children. Supply their own subtext.

Use verified, revision-specific repository web links for readable source citations
when available. A relative filesystem link in an Essay is not a supported code
viewer link. For exact CLI-tracked evidence, use supported narrative landmarks or
slide Items; do not invent an Essay code-link scheme. Whiteboards do not earn
slide/review line coverage, and the ERD source adapter cannot pin arbitrary code.

Feature decks link to relevant Architecture sections rather than redrawing the
map. They explain the local behavior, algorithm, decisions and failure paths in
that context. ERD explains data relationships, Infrastructure deployed resources,
and Data flows movement; cross-link them where needed.

## Compact orchestration prompt

“Build the Architecture whiteboard using references/architecture.md. First gather
manifests/imports, boundary rules and checker results, composition roots, ports
and gaps, and divergence from intent. Use a status title, legend, codebase map
with rules panel, then evidenced question sections with locators and exceptions.
Generate against a scratch Saga, inspect the opening and every focused section
in light and dark mode, and have a fresh reader fact-check claims against code
before publishing. Add the shared technology-stack Essay and approach links.
Report verification limits; do not invent missing evidence.”
