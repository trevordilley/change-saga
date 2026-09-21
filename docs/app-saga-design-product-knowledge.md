# App Saga design: durable product knowledge

Status: canonical UX and technical design for the accepted requirement heads at
main commit `4b847a1`. This wave owns only the `format` and `requirements`
design roots and their source-owned relations. It deliberately adds no code
references, implementation decks, quality records, stories, personas, flags,
or prototypes.

## Design outcome

The app Saga is one living model of the application, not a sequence of change
documents. It has two complementary shapes:

- app-wide orientation holds the application name, pitch, description, people,
  vocabulary, onboarding, and feature flags;
- durable feature slices hold Product, Design, Quality, and Implementation for
  one coherent domain.

The default projection answers “what is true now?” Accepted, active knowledge
leads. Earlier revisions, competing heads, lifecycle events, and retired
records remain inspectable history. A pull request or comparison explains a
transition; it does not become a second copy of the current product model.

The `format` chapter, **The living application knowledge model**, makes that
topology and the feature-flag overlay visible. The `requirements` chapter,
**Product knowledge from intent to evidence**, carries the vocabulary model,
story and criterion lifecycle, prototype-to-design flow, evidence traversal,
and the durable operating decisions.

## Information architecture

The overview is the front door. It introduces purpose before structure through
the application name, pitch, and description, then teaches the words needed to
navigate the product. A term has stable identity around its name, definition,
and aliases. It can point to story intent and defining source locations; those
endpoints can list the term in return. A changed defining location makes the
link stale rather than allowing vocabulary to drift silently. Comparison may
suggest a new domain word, but recording it remains optional growth.

From the overview, each feature opens as a stable Product → Design → Quality →
Implementation slice. Product carries stories, criteria, citations, and
prototypes. Design explains intended experience and system behavior. Quality
records how criteria are tested. Implementation explains how current code
realizes that intent. Moving a story changes feature membership, not its
identity or existing links.

## Story and criterion lifecycle

Definition history and lifecycle state are separate append-only graphs.

1. Revising a story appends a complete definition and keeps every earlier
   revision.
2. Concurrent revisions remain separate heads. No reader invents a winner; an
   author reconciles them with a new revision that names both parents.
3. Lifecycle events make proposed, accepted, deferred, rejected, and retired
   explicit without rewriting the definition graph.
4. Retirement removes a story from the current set but preserves its history.
5. A criterion keeps its identity when wording changes but the obligation does
   not. A genuinely different obligation gets a different identity.
6. Citations preserve where a requirement came from; they do not substitute for
   design or implementation evidence.

This gives one Saga room to evolve without becoming either an ahistorical
snapshot or a changelog that forces readers to reconstruct current intent.

## Prototype and design-before-implementation workflow

Prototypes and requirements may lead each other. A self-contained interactive
prototype or supported external prototype can exist before a story is settled,
and an annotation links one identifiable state to a story or criterion
revision. Changing either endpoint makes that annotation stale. An unlinked
prototype is visible optional growth, not an approval gate.

Once intent is discussable, UX and technical design become reviewable before
implementation evidence exists. The design contract is:

```text
prototype state ⇄ story / criterion → UX + technical design → implementation
                                              ↓
                                      test case → run result
```

Design chapters establish a concern; fragments choose the visual or narrative
form that best explains it; landmarks make independently discussable nodes
addressable. A focused landmark addresses the smallest criterion it satisfies.
Chapter- or fragment-level story relations are reserved for genuinely
cross-cutting design. Requirement and design surfaces expose the same pinned
relation in both directions. When the requirement changes, the relation goes
stale until an author reads the new intent and deliberately re-pins or replaces
the design.

Implementation does not retroactively define intent. It provides evidence that
the reviewed design was realized. Likewise, work progress coordinates people
but cannot satisfy the separate quality-evidence axis.

## Flags and release visibility

A feature flag is an availability overlay on the living product model. It names
the feature or story it gates and exposes `on`, `off`, or `retired`.

- `on` means the documented behavior is enabled.
- `off` leaves the behavior documented but visibly unavailable, preventing a
  newcomer from mistaking the state for missing implementation.
- `retired` removes the flag from the current release view while preserving its
  lifecycle history.

The overlay never forks product knowledge. Requirements and design remain in
their durable feature; the flag only changes the reader's release lens.

## Evidence traversal entry points

The persisted graph records adjacent, reviewable links. Longer paths are
derived so one relationship does not have to be copied across the Saga.

```text
persona → story → criterion → design → code
                         ↘ test ───────↗
```

Readers can enter from four useful places:

- a persona or story to understand who receives value and which obligations
  express it;
- a criterion to inspect current design, test, and code paths;
- a source location to find connected requirements and explanations;
- changed code to find requirements and people it may affect.

Every path reports `current`, `missing`, or `stale`. Missing is not fabricated,
and stale does not count as current coverage. An active persona with no accepted
story remains a visible gap. Reverse traversal is therefore both an inspection
tool and an impact-analysis boundary.

## Quality boundary

A test case names the criteria it verifies, presents ordered steps, and states
one expected result. A run identifies the test revision it executed. Revising a
test makes earlier passing results stale; failed results remain visible until a
newer result supersedes them. Policy can require test kinds and classify each as
`satisfied`, `excluded`, or `missing`; every exclusion needs both a rationale
and an authoritative source.

The design records this boundary because it prevents two category errors:
implementation progress is not delivery evidence, and a broad “covered” signal
is not a substitute for criterion-specific proof.

## Decisions and rejected alternatives

- **One living projection, retained history.** A flat changelog was rejected
  because it makes every reader reconstruct the present. Destructive updates
  were rejected because they erase decision context.
- **Stable identity, versioned meaning.** Paths and feature membership are not
  identity. This allows moves and reorganization without breaking the graph.
- **Adjacent links, derived traversal.** Hand-authored end-to-end links were
  rejected because they duplicate state and drift independently.
- **Criterion precision by default.** Story-level links are appropriate only
  when a design genuinely spans the full story revision.
- **Explicit stale states.** Silent carry-forward was rejected wherever an
  endpoint changed meaning. The repair step is intentional author judgment.
- **Flags as overlays.** Copying or hiding product knowledge by release state
  was rejected because it makes unavailable behavior look undocumented.
- **Quality separate from progress.** Work tracking can report coordination;
  only immutable delivery and test evidence can support readiness.

## Authored graph and traceability

This wave adds:

- 2 design chapters;
- 4 SVG architecture/lifecycle/flow fragments and 1 Markdown decision
  fragment;
- 40 semantic landmarks;
- 89 current pinned `addresses` relations: 67 criterion-level and 22
  story-level cross-cutting links.

All 47 meaningful owned design targets—chapters, fragments, and landmarks—have
at least one current pinned relation. The seven owned accepted stories have
complete design coverage, and all 56 of their criteria have at least one
criterion-specific design target:

| Feature | Story | Focused criteria |
| --- | --- | ---: |
| `format` | `controlled-release` | 4 / 4 |
| `requirements` | `overview-and-terms` | 11 / 11 |
| `requirements` | `evolve-product-knowledge` | 7 / 7 |
| `requirements` | `prototype-feedback` | 6 / 6 |
| `requirements` | `design-intent` | 7 / 7 |
| `requirements` | `evidence-traversal` | 10 / 10 |
| `requirements` | `quality-evidence` | 11 / 11 |

## Validation and intentional gaps

Final checks used the repository CLI built from main:

```text
/tmp/change-saga-product-knowledge validate --json app.saga
/tmp/change-saga-product-knowledge status --json app.saga
/tmp/change-saga-product-knowledge status --json --feature format app.saga
/tmp/change-saga-product-knowledge status --json --feature requirements app.saga
/tmp/change-saga-product-knowledge query traceability --saga app.saga --requirement <owned-story>
```

Validation is valid with zero errors. It reports four warnings because the four
SVG fragments have no directly linked code. Those warnings are intentional:
this design wave was explicitly prohibited from adding code references.

Feature-scoped status reports design coverage `1/1` for `format` and `6/6` for
`requirements`. App-wide design coverage is `11/18`. The seven remaining story
gaps belong to other explicitly out-of-scope design owners:
`claims-verification`, `companion-repositories`, `first-change`,
`parallel-delivery-plan`, `review-is-pr-deck`, `safe-local-review`, and
`why-things-changed`.

App health is `180/181`. The sole remaining health gap is the pre-existing
stale `two-sides-sidebar.json#1` code reference under `reviewer-app`; repairing
it would require editing another feature and adding code evidence, both outside
this wave. Quality remains `2/146`; this wave defines the quality model but does
not author the out-of-scope test cases and run evidence. No concrete feature
flag or prototype record was added because this wave owns design, not product
records.

## CLI rough spots

| Exact command | Goal | Observed behavior | Severity | Workaround used | Smallest improvement |
| --- | --- | --- | --- | --- | --- |
| `change-saga query requirements --saga app.saga` | Read accepted requirement heads through the supported query API. | The installed `0.2.0-dev` binary rejected format v5 as an unsupported Saga and reported v2/v3/v4 layout assumptions. | High | Built the repository CLI with `hivecontrol exec oneshot 2m -- go build -o /tmp/change-saga-product-knowledge ./cmd/change-saga` and used that binary consistently. | Make the installed CLI advertise its maximum Saga version in `version`, and on mismatch point directly to the repository-local build or update path. |
| `/tmp/change-saga-product-knowledge query fragment --saga app.saga --target urn:change-saga:app:fragment:knowledge-topology --limit 8000` | Read back the first completed fragment before authoring the others. | The whole read failed with `invalid_saga` because three sibling fragments still contained generated examples and one was empty. The requested target itself was valid. | Medium | Finished every new fragment, then retried the query. | Let a target-specific query return valid target content plus global health warnings, or support an atomic add-and-set fragment mutation. |
| `/tmp/change-saga-product-knowledge design set-fragment-content --feature requirements --target urn:change-saga:app:fragment:story-design-lifecycle --source - app.saga` | Fix text overflow found by the 1280×720 render audit. | Correctly made digest-pinned design relations stale, but the four visual edits invalidated 69 chapter-, fragment-, and landmark-level relations and required one repin command per relation. | High | Read every stale relation from `status`, reviewed the final renders, and looped `relation repin --relation <URN>` with an explicit rationale. | Add a reviewed bulk operation such as `relation repin --from <fragment> --descendants` that previews every pin it will advance and commits atomically. |
| `/tmp/change-saga-product-knowledge query relations --saga app.saga --state stale --limit 50` | Enumerate relations whose pins were stale after the visual edit. | The filter selected stale relations, but each returned record still displayed `state: "active"`; currency was only visible in the separate `stale: true` field. | Medium | Inspected `.stale` and `stale_reasons`, then used the health entries from `status` as the repin queue. | Rename the filter to `--currency stale`, or expose lifecycle and currency as clearly named adjacent fields in both schema and output. |
| `/tmp/change-saga-product-knowledge relation repin --relation urn:change-saga:app:relation:app-root-addresses-overview-parts --rationale "The rendered node remains the same design claim after legibility edits." --json app.saga` | Confirm that a relation still holds after a non-semantic visual edit. | Repin worked and inferred the owning feature, but the help presents `--feature` as required while the command succeeds without it. | Low | Omitted `--feature` and relied on globally unique relation identity; verified the emitted path was under the source design's feature. | Change the usage line to show `--feature` as optional when `--relation` uniquely resolves its owner. |
