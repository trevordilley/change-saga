# Diagrams, narrative, and exact evidence

Read this reference when the task creates or changes a feature implementation
deck, pull-request review deck, narrative fragment, landmark, code evidence,
claim, or verification. Read [query.md](query.md) first when a Saga already
exists. Use `change-saga --help`, command `-h`, and `change-saga spec --json`
for current syntax.

## Choose the artifact

Every deck has a Deck -> Slide -> Item spine. A feature's single living
implementation deck explains the domain's current implementation. Update its
affected slides rather than creating one deck per change. A pull-request review
deck explains a transition and its reasoning. An onboarding deck teaches the
app without owning code. Requirements, prototypes, and design are not slides.

Chapters, sections, and fragments carry longer design notes, overviews, and
walkthroughs. Use Markdown to orient and connect visual artifacts, not as the
default container. A substantial chapter should lead with a diagram,
interactive walkthrough, or concrete before/after example and state its
boundary, invariants, decisions, risks, compatibility concerns, and proof.

## Storyboard before drawing

For each proposed slide, identify privately:

- the specific reviewer question, intent, and one-sentence takeaway;
- the minimum system model or meaningful surprise it explains;
- the relationship the picture must make visible;
- the visual form that truthfully encodes that relationship; and
- the nodes, edges, states, regions, transitions, examples, or callouts that
  will become evidence-bearing Items.

Choose the form from the relationship:

- system context for actors, external systems, and trust or ownership
  boundaries;
- architecture/composition for containment, dependencies, and responsibility;
- data flow for direction, transformations, storage, consumers, and retry or
  failure paths;
- sequence for participants, time, calls, responses, and exceptional returns;
- state machine for states, labeled events, guards, and terminal states;
- entity-relationship for entities, keys, ownership, and cardinality;
- decision flow for predicates, branches, joins, loops, and outcomes;
- matched before/after axes for a meaningful delta;
- failure path for trigger, propagation, containment, cleanup, recovery, and
  observable outcome; and
- evidence view for a concrete claim or risk and its tests or measurements.

`--intent` names the slide's rhetorical job and `--layout` its canvas
arrangement; neither supplies the diagram's meaning. A row of cards is valid
only when membership or matched comparison is the relationship. Boxes joined
only by reading order are an outline, not a diagram.

Use SVG for stable architecture, boundaries, data models, and flows. Use
self-contained HTML when switching paths, stepping through states, changing an
input, or comparing behavior materially helps. Load no network dependencies,
and make the default state understandable without interaction.

## Compose semantic, reviewable visuals

One slide carries one intent, one takeaway of at most 180 characters, and one
bounded 16:9 composition. Use 1-7 primary semantic Items on a standard slide.
Every non-decorative node, edge, region, transition, statement, risk, metric,
example, and callout belongs in `reading_order`, with a label, a semantic
description that stands without the picture, and an element or normalized
region selector.

Only Items own deck code evidence. Deck- and slide-level coverage is refused.
A callout may name another Item with `--about` and may own exact evidence for
its claim. Keep callout bodies at most 240 characters. If the slide needs a
paragraph or more than seven primary Items, split the argument or choose a
better composition; do not hide prose in SVG or HTML.

Show a concrete input, important intermediate state, output or side effect,
and a consequential edge or failure path where relevant. At 1280x720 and
1024x576, verify clipping, overlap, legibility, focus geometry, scan order,
and a useful narrow linear reading. A custom layout needs a rationale and does
not waive accessibility or evidence requirements.

### Reviewer surprises

After establishing the smallest useful system model, foreground behavior that
breaks a reasonable maintainer expectation: counterintuitive public behavior,
intentional convention changes, hidden coupling, displaced costs, ordering or
ownership constraints, tradeoffs, rejected alternatives, and important
failure or recovery behavior.

For each material surprise, show expectation, actual behavior, rationale, and
consequence together. Attach a callout Item to the responsible visual element
and give the actual behavior and consequence exact evidence. Ground the
contrast in source material or a plausible reviewer mental model. Do not
manufacture novelty; when none exists, teach system shape, risk boundaries,
and verification instead.

### Four visual audits

1. **Silhouette:** without text or color, topology still communicates
   containment, flow, sequence, state, entity structure, branching, or
   comparison.
2. **Relationship:** every relationship essential to the takeaway is encoded
   as an edge, boundary, lane, nesting, cardinality, axis, or transition.
3. **Surprise:** a reviewer can name the system model, highest-consequence
   deviation, its reason, and its tradeoff.
4. **Contact sheet:** repeated visual grammar represents the same underlying
   relationship; unrelated slides have not collapsed into identical cards.

Run these before chasing coverage. Coverage cannot rescue a generic visual.

## Narrative fragments and landmarks

Create chapters, sections, and fragments with their public commands. Install
or replace a fragment entrypoint only with `set-fragment-content`; revise or
remove the owning record with the matching command. Do not edit fragment
package metadata directly.

Create landmarks for independently discussable headings, concepts, states,
controls, nodes, and edges, not decorative shapes or every sentence. Preserve
stable lowercase heading anchors. Give every meaningful visual landmark a
description that works without geometry, color, or position. SVG element
bounds become links automatically; use normalized hotspots or image regions
only where needed. Evidence attached to a landmark already reaches the owning
fragment's design relation; do not duplicate it at fragment scope.

Every concrete prose claim about implementation, behavior, an invariant, or a
data transition needs a focused footnote citation or deliberately
evidence-bearing heading. Make a footnote definition an exact-text landmark
and attach only substantiating code. Requirement provenance citations do not
replace implementation evidence.

## Evidence discipline

Code references are pinned to a commit and digest of the exact bytes. Pure
line movement may remap them; changed bytes make them stale. Repair or remove
stale evidence through its public command while retaining its history.

- Attach code to the narrowest Item or landmark that actually explains it.
- Each reference note says both what changed and why this target owns it.
- Keep one file and coherent reason per record; split different reasons.
- Preserve deletions, migrations, generated files, snapshots, lockfiles, and
  vendored changes as explicit evidence rather than hiding them in broad
  ranges.
- Use changed-lines coverage only when every changed line in that file belongs
  to the same focused target. `cover --batch -` may plan several focused
  records atomically, and `--dry-run` previews supported mutations. Never widen
  a selector merely because a batch makes broad coverage convenient.
- Treat overlap as intentional only for distinct reviewer journeys. Repair an
  existing record with `replace-coverage`, not a duplicate.
- Use `query mappings --sort scrutiny` after coverage. Never hand-edit a
  record to make stale evidence pass.

A landmark target may be passed back as its returned URN or, where the command
accepts it, as `<fragment>#<landmark-id>`. Use returned targets rather than
deriving storage paths.

An Item may relate to a story or criterion with a pinned, self-scoped relation.
Prefer a criterion when the visual explains one obligation; use the story only
when it genuinely applies to every criterion. A slide's summary is derived
from Item relations. Legacy deck or slide links remain readable but do not
apply automatically to every Item.

## Claims and verification

Use `add-claim` for a falsifiable assertion such as an invariant,
compatibility promise, measured performance result, security property, or
test outcome. Claim evidence is separate from coverage. Append a result with
`verify-claim` only after performing its named method; record a reproducible
command where possible. Use `unverified` when it was not checked. Claims and
results are append-only.

## Handoff checks

Read the deck in order without relying on author knowledge. Confirm its visual
forms and four audits, exact evidence, useful collapsed-file notes, current
code agreement, accessible semantics, and explicit uncertainty. Query until
no requested changed line is uncovered, no reference is stale, and each
overlap is defensible. Verify claims independently; complete coverage is not
verification. Remove scaffold content, run validation, and use the relevant
bounded queries before handoff.
