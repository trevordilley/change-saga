# Authoring decks and narrative content

## Resolve the comparison

For a PR number or URL, use repository-host metadata rather than guessing:

- PR number, URL, title, and stated motivation
- base branch/OID and head branch/OID
- commit and changed-file summary
- local checkout containing the head

When GitHub CLI is available, an appropriate discovery call is:

```sh
gh pr view <number> --json number,title,url,body,baseRefName,headRefName,headRefOid,files,commits
```

Use an available provider integration instead when it has better access. For a
local change without a PR, identify the intended merge base and commit the work
first: comparisons are between commits, and uncommitted changes are not part of
any comparison.

Cross-check the provider's head OID/branch, title, and changed files against the
local checkout before recording anything. If they do not describe the same
change, stop and resolve the mismatch. A review with no PR identity is better
than one linked to the wrong pull request.

## Decks and traceability

Every deck shares one spine: Deck → Slide → Item. A feature's implementation deck
explains the domain's current implementation and stays valid as the code
changes: its Items remap, or go stale where the code really changed. A pull
request's review deck explains the transition and its reasoning. The app's
onboarding deck gets people up to speed; its Items carry a persona, feature, or
story record instead of code. Never paginate narrative content or turn user
stories, prototypes, or design into slides.

Requirements are where traceability ends, and they grow over time. When a story
exists, an Item with code evidence reaches it through an active relation on the
Item or its containing slide or deck, or transitively through the design or
test case it implements. Pin the relation's target revision. Prefer a criterion
target when the visual explains one acceptance criterion; use a story target
only when it genuinely applies to every criterion in that revision. Do not
repeat story prose in slide metadata. Before handoff, look up evidence in
reverse and offer a story for what has none:

```sh
change-saga query traceability --saga app.saga --ref '<commit>:<path>#L<start>-L<end>'
change-saga query traceability --saga app.saga --commit '<commit>'
```

## Storyboard visual questions before creating slides

Do not start by choosing a reusable SVG template. First inspect the change and
write a private storyboard. For every proposed slide, name:

- the specific reviewer question it answers, not merely its topic;
- its rhetorical intent and one-sentence takeaway;
- whether it establishes the system model or resolves a specific reviewer
  surprise, including expectation, actuality, rationale, and consequence;
- the relationship the picture must make visible;
- the visual form that truthfully encodes that relationship; and
- the meaningful nodes, edges, states, regions, or callouts that will become
  evidence-bearing Items.

Choose the visual form from the explanation, not from styling convenience:

- a system-context diagram shows actors, external systems, trust or ownership
  boundaries, and the changed interface;
- an architecture/composition diagram shows containment, dependencies,
  responsibilities, and what moved or was introduced;
- a data-flow diagram shows direction, inputs, transformations, storage,
  consumers, trust boundaries, and failure or retry paths;
- a sequence diagram shows participants, time ordering, calls, responses, and
  exceptional returns;
- a state machine shows states, labeled events, guards, and terminal states;
- an entity-relationship diagram shows entities, keys, ownership, cardinality,
  and the old-to-new shape of a migration;
- a logic or decision flow shows predicates, branches, joins, loops, and
  outcomes;
- a before/after comparison uses matched axes and highlights the meaningful
  delta;
- a failure-path diagram traces trigger, propagation, containment, cleanup,
  recovery, and observable outcome; and
- an evidence view connects a concrete claim or risk to tests, measurements,
  or observable results.

`--intent` states the slide's job and `--layout` states its canvas
arrangement; neither is a substitute for the correct visual form. A grid or row
of labeled cards is valid only when category membership or matched comparison
is itself the relationship being explained. Do not use cards as a universal
container for architecture, flow, lifecycle, data, or failure semantics. Boxes
connected only by reading order are an outline, not a diagram.

Show by example: include concrete inputs, important intermediate state, output
or side effects, and at least one consequential failure or edge path. Prefer a
realistic payload, schema row, event, command, or UI action over an abstract
paragraph. Use SVG for architecture, boundaries, data models, and stable flows.
Use self-contained HTML and JavaScript when a reviewer benefits from switching
paths, stepping through states, changing an input, or comparing old and new
behavior; make the default state understandable without interaction and load
no network dependencies.

## Deck contract

A deck is a visual argument, not a report broken into pages. Each slide has one
intent, one takeaway of at most 180 characters, and one bounded layout. Use 1–7
semantic Items and put every non-decorative node, edge, region, transition,
example, risk, metric, statement, and callout in `reading_order`. Every Item
needs a concise label, a semantic description that stands without the picture,
and an element or normalized-region selector.

Only Items own code evidence; coverage on a deck or slide is refused. If a
changed line does not belong to an existing Item, improve the composition or
add a focused Item. A callout is an overlaid Item, not a different evidence
layer: it may name the Item it explains with `--about`, and its own references
may point at the exact code that substantiates it. Keep callout bodies at 240
characters or less.

Keep labels and callouts short: state the non-obvious invariant or tradeoff next
to the visual relationship it explains. If the visual still needs a paragraph
to make its point, split the argument or choose a better composition. Do not
hide prose in SVG or HTML to evade the rendered density and legibility checks.

At 1280×720 and 1024×576, verify that nothing clips or overlaps, labels remain
legible, the reading order matches the intended scan path, every Item can be
focused without obscuring another, and the slide still makes sense in a narrow
linear reading view. `--layout custom` requires a rationale and does not waive
evidence, contrast, focus, geometry, or text-equivalent checks.

### Reviewer surprises

First establish the smallest system model needed to predict the change. Then
foreground the places where that prediction breaks or where a reasonable
maintainer may hesitate:

- behavior that is counterintuitive from the public contract or nearby code;
- intentional deviations from repository conventions or existing architecture;
- hidden coupling, ownership boundaries, ordering constraints, or state that
  makes a local-looking change non-local;
- tradeoffs, rejected alternatives, compatibility choices, and costs displaced
  into operations, security, performance, migration, or future maintenance;
- failure, fallback, cleanup, and recovery behavior that differs from the happy
  path; and
- unchanged behavior whose preservation is important but easy to assume
  incorrectly.

For each meaningful surprise, make four things legible: the reasonable
reviewer expectation, the actual behavior or design, why the change chose it,
and the consequence for users, operators, reviewers, or future code. Link the
actual behavior and consequence to exact evidence. A surprise is especially
effective as a callout Item attached to the node, edge, state, or transition
that creates it; give the callout its own evidence when it makes a code-backed
claim. Ground the contrast in documentation, established patterns, historical
design, or a plausible reviewer mental model. Do not manufacture novelty: if
the investigation finds no material surprise, say so and use the deck to teach
the system, risk boundaries, and verification. Reveal the highest-consequence
surprise early enough to shape how the reviewer reads later slides.

### Visual audits

Before handoff, run four audits:

1. **Silhouette test:** mentally remove labels, prose, and color. The remaining
   topology should still communicate whether this is containment, flow,
   sequence, state, entity structure, branching, or comparison.
2. **Relationship test:** every relationship essential to the takeaway is
   visibly encoded with an edge, boundary, lane, nesting, cardinality, axis,
   or transition, not left to nearby prose.
3. **Surprise test:** after reading the deck, a reviewer can name the system
   model, the highest-consequence deviation from likely expectation, why it
   exists, and the tradeoff it creates. If not, the deck is complete as an
   inventory but incomplete as an explanation.
4. **Contact-sheet test:** inspect all slides together. Reuse a visual grammar
   only when the underlying relationship is genuinely the same. If unrelated
   slides reduce to the same number and arrangement of cards, rewrite them.

Perform these audits before chasing complete coverage. Coverage is the final
omission check; it must not rationalize a generic visual after the fact.

## Narrative content

Chapters, sections, and fragments carry a feature's narrative (design notes,
overviews, walkthroughs) and the app-level design system. Create them with
`change-saga add-chapter --feature <feature>`, `change-saga add-section`, and
`change-saga add-fragment` (Markdown, SVG, image, text, or a self-contained
HTML package). Write or replace an entrypoint only with `change-saga
set-fragment-content --target <fragment> --source FILE|-`; do not edit fragment
package files directly. Fix a title or order with `revise-chapter`,
`revise-section`, or `revise-fragment --target <target>`, and delete a record
with the matching `remove-*`. Pass the target URN each command prints (or that `query
children` reports) as `--target`.

Organize chapters by behavior, risk, architecture, or reviewer intent rather
than file type, and use Markdown to orient and connect visual artifacts rather
than as the default container. Lead with a picture: a substantial chapter
begins with an SVG diagram, a self-contained interactive HTML walkthrough, or a
concrete before/after example, and makes its workflows, data flows, data
models, boundaries, and failure paths explicit. A new chapter holds no fragment;
add its first with `add-fragment --section <chapter>`. That overview states its
purpose and boundary, its invariants, notable decisions and rejected
alternatives, risks and compatibility concerns, and how it is verified. Split a
chapter when it contains independently understandable behavior with a different
risk profile; do not create one per directory or language.

### Landmarks

`change-saga add-landmark` makes a Markdown heading, exact text span, HTML/SVG
element, or image region independently addressable:

```sh
change-saga add-landmark --target <fragment> --element-id submit-action --label "Submit action" \
  --description "The validated request crosses into the persistence boundary." app.saga
change-saga add-landmark --target <fragment> --heading-id request-validation --label "Request validation" app.saga
change-saga add-landmark --target <fragment> --id lease-renewal \
  --text "Renewal is triggered from the heartbeat path before the lease midpoint." --label "Lease renewal evidence" app.saga
```

End every Markdown heading with a stable, fragment-local anchor such as `##
Request validation {#request-validation}`: lowercase letters, digits, and
hyphens, beginning with a letter, unique within the fragment, and preserved
when the visible heading changes. `change-saga validate --fix` adds a missing
anchor to any heading and changes nothing else; it does not choose meaningful
anchors for you.

Create landmarks for independently discussable concepts, states, controls,
diagram nodes, and edges, not decorative shapes or every sentence. Give every
meaningful visual landmark a semantic `--description` that explains its role
without relying on geometry, color, or position; it is what AI and non-visual
clients receive. SVG element bounds become on-canvas links automatically; use
`--hotspot x,y,width,height` (normalized to the viewBox) only to override
awkward geometry such as a long diagonal edge, and for HTML elements, which
need one. Raster regions use `--region` with normalized image coordinates.

Reference a landmark's code with `change-saga cover --target
<fragment>#<landmark-id>` or the landmark URN from `query children`. Do not
duplicate the same lines at fragment scope merely to make them visible.

A landmark is a heading inside its fragment, not a separate piece of design, so
code attached to it already reaches whatever the fragment's design addresses;
moving references off fragment scope never costs coverage. Relate a landmark
itself only to name which criterion that one heading addresses, which is more
precise and takes precedence over what it would inherit.

### Cite prose claims

Every concrete prose claim about implementation, behavior, an invariant, or a
data transition carries a Markdown footnote citation or lives under a
deliberately evidence-bearing heading:

```markdown
The heartbeat renews the lease before half its TTL elapses.[^lease-renewal]

[^lease-renewal]: Renewal is triggered from the heartbeat path before the lease midpoint.
```

Make the plain-text footnote definition an exact-text landmark and reference
only the code that substantiates it. A footnote is not linked until its
definition owns evidence; validation warns for each one that does not. Keep the
definition unique within the fragment and free of inline Markdown so the exact
selector stays durable. One citation supports one focused idea; cite
behavioral claims, invariants, and non-obvious facts, not background.
Requirements provenance from `citation add` records where a requirement came
from and does not replace implementation evidence.

Before attaching broad coverage, inventory addressability: give every concrete
claim in each Markdown fragment a citation or heading landmark, give every
code-bearing SVG or HTML node, edge, arrow, transition, state, and control a
stable element ID and landmark, then use `query children` and `query
fragment-diffs` to confirm these focused targets own their evidence. A
citation-free implementation narrative, or a visual with no landmarks, is
unfinished even when coverage is complete.

## Evidence discipline

- Attach a changed line to the most focused Item or landmark that actually
  explains it.
- Give every reference a concise `--note` that answers both "what changed in
  this file?" and "why does this target own it?" Write for the collapsed file
  row a reviewer sees before opening code, for example "Parses and validates
  code references so evidence stays attached to the exact lines it explains."
  Do not use path-only or generic notes such as "implementation" or "tests."
- Keep one file and one coherent reason per record. When separate ranges in one
  file serve different ideas, attach them to their own targets with distinct
  notes.
- Read generated, vendored, lockfile, migration, and snapshot changes; group
  them explicitly rather than hiding them in a broad range.
- Keep deletions visible and explain behavior that disappeared.
- Treat overlaps as intentional only when the same code is necessary in two
  distinct reviewer journeys.
- Never widen a selector solely to make every line covered. This applies
  unchanged to `cover --batch`, whose records carry `target`, `path`, `side`,
  `lines`, `changed_lines`, `file`, `commit`, `refs`, `note`, and `name`.
- Use `--changed-lines` only when every changed line of the file belongs to the
  same focused target; it also records whole-file events such as `add`. A
  second explanation for the same lines requires `replace-coverage` rather than
  a duplicate record.
- Run `query mappings --sort scrutiny` after coverage and address its warnings
  rather than merely accepting them.
- Never hand-edit a reference to make stale evidence pass; re-author it.

## Claims and verification

Record falsifiable assertions, not design opinions, with `change-saga
add-claim --target <Item or landmark> --kind <kind> --statement <text> --ref
<location>`. Examples include behavioral invariants, compatibility promises,
measured performance changes, security properties, and test outcomes; prefer
"at most one sampler may be active for a process" over "the design is clean."
Claim evidence is separate from coverage and never makes an uncovered line
covered.

Append a result with `change-saga verify-claim`. Use `verified`, `failed`, or
`inconclusive` only after performing the named test, command, measurement,
inspection, or analysis, and record the reproducible `--method` and
`--command`. Use `unverified` when the claim has not been checked. Each claim
and each result is its own append-only file.

## Reviewer-readiness check

Before handing off:

- Read the deck in order without relying on prior author knowledge. Its first
  slides answer: what problem is solved, what behavior changes and what
  deliberately does not, the end-to-end flow, why it is shaped this way, and
  the major risks and verification signals.
- Every slide leads with a purpose-fit visual or worked example, and the four
  visual audits pass.
- Interactive content teaches through its default state and controls, is
  self-contained, and is not static prose placed in HTML.
- Diagrams and examples agree with the current code.
- Every changed line is covered, no reference is stale, and every overlap is
  defensible. Review decks cover their own range (`review list --uncovered` is
  empty).
- `query mappings --sort scrutiny` shows no unjustifiably broad records.
- Narrative claims have focused citations, and every code-bearing node, edge,
  and control opens its exact code.
- Important assertions are structured claims with an explicit verification
  state and reproducible commands. "Every line covered" is not verification.
- Every collapsed file shows a useful what-and-why note before its ranges are
  expanded.
- Tests, migrations, generated artifacts, and removed behavior are not silently
  omitted, and validation reports no untouched scaffold.
- Genuine uncertainty is reported in the Saga instead of invented intent.
