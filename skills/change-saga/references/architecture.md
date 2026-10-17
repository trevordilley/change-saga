# Architecture: understand the codebase before a feature

Use the Architecture whiteboard to explain the codebase's organizing ideas. A
reader should be able to locate a feature's implementation in the wider system
and recognize the intended dependency boundaries before opening its deck.
Follow [visual storytelling](visual-storytelling.md) for sectionality, color,
visible explanations and visual inspection; use [diagrams](diagrams.md) for
publication commands. This is a subject of the shared whiteboard format, not a
new visualization type or a revived Systems/Components inventory.

## Show the interesting decisions

Start with a readable map of responsibility boundaries and labeled dependency
direction. Identify composition roots, interfaces or adapters where they actually
exist. Distinguish a code dependency from a runtime message or data movement.
Show what owns durable state, transient UI state and derived/cache state.

Then choose the questions that make this codebase distinctive:

- Where are concrete dependencies selected? If it uses dependency injection,
  show who constructs what, the interface boundary, and what can be replaced.
- Where does a factory, adapter or other indirection earn its cost? Follow one
  concrete caller through the seam and explain what remains coupled.
- Who owns frontend state? If Redux and TanStack Query coexist, show the split
  between application state and server-derived state, including invalidation
  and synchronization. Those tools are examples, not defaults to recommend.
- What makes parallel work mergeable? Show identities, immutable versus mutable
  records, conflict handling and the operations that still require coordination.
- What makes reading fast without returning the wrong content? Show cache keys,
  reuse boundaries, invalidation and expensive work deferred until needed.

Pick the mechanisms the repository actually has. Do not invent layers or label
ordinary constructors as a dependency-injection framework. A list of fashionable
patterns does not establish sound architecture. Show an important exception,
tradeoff or known coupling when the evidence warrants it; let the reader assess
the structure without an automated quality score.

## Keep the main surface visual

Use an opening codebase map and question-led focused sections. Each detail should
retain a small locator or explicit link back to its place in the map. Give
responsibilities stable colors, put components inside their owners, and explain
why a crossing exists beside its arrow. The main mechanism and consequence must
be visible without hovering.

One board can contain several focused diagrams. If mechanisms need independent
reading space or history, publish supporting named boards and link to them from
the approach essay and supporting architecture essays. Architecture still has one role
assignment; supporting boards retain their own identities. Discover roles first
with `whiteboard list --json`; reuse an existing board via `assign-role --role
architecture --expect ID|URN|none`. The conventional board ID is `architecture`.

## One authoritative technology-stack reference

Use `overview set-technical-design --page technology-stack --title "Technology
stack" --source FILE` for a shared Essay. List each meaningful technology once,
with its responsibility, why it was chosen, relevant constraints/tradeoffs, and
source evidence. Distinguish runtime choices from build/test tooling. Do not
duplicate the list in every feature. Link from the approach, Architecture, and
feature decks; use only the few technology labels needed to explain a diagram.

Separate a rationale supported by project decisions from an inference based on
code. Say when historical motivation is unknown. Package manifests can establish
use and version, but cannot establish why a team chose a dependency.

## Connect architecture to feature implementation

Architecture explains shared conventions and cross-cutting mechanisms. A feature
deck explains how one capability uses them: its behavior, local algorithm,
decisions, failure paths and exact code evidence. Link a feature to the relevant
Architecture section rather than repeating the codebase map. ERD explains data
relationships, Infrastructure explains deployed resources, and Data flows
explains movement; cross-link those views where their context is needed.

Inspect the actual implementation before claiming a boundary or guarantee. Keep
source links and supporting evidence in the shared Essay or supported visual
evidence surfaces. A whiteboard does not earn slide/review line coverage and
does not support arbitrary code pins through the ERD source adapter.
