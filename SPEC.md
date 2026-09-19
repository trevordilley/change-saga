# Change Saga format

A Change Saga is the durable, version-controlled documentation and history of
an application: its personas, its requirements, their design, how they are
verified, and how they are implemented, with every implementing line of code
referenced at the commit it was documented against. The application evolves
through many bodies of work; the Saga keeps all of its requirements current as
the code changes, and every change is a comparison of the Saga and its code
between two commits.

There is exactly one Saga format. It is identified by `version: 5` and the
schemas under [`schema/v5`](schema/v5). Readers reject any other manifest
version, a root `00-saga.json`, and a manifest `presentation` member. A Saga may
live in the code repository or in a companion repository; code references
resolve against the repository the manifest declares.

Git is the outer audit log. Saga records add the semantic history inside it:
immutable identities, append-only revisions and lifecycle events, relations
that pin the revisions they rely on, supersession, evidence, and review
decisions. Staleness is always derived from pins, never from Git history.

### Layout

A Saga documents one application. Material about the whole application sits at
the root; everything else belongs to an **epic**, a durable area of the
product. The manifest carries identity and source only: it adds no aggregate
quality, coverage, relation, or deck fields.

```text
<id>.saga/
  saga.json
  ___overview/                 # pitch, description, and terms and vocabulary
  ___designsystem/             # design-system references
  ___personas/<id>.persona/    # who the application serves
  ___featureflags/<id>.flag/   # flags and the stories or epics they gate
  ___onboarding/<id>.deck/     # a deck that explains the application
  ___epics/<id>.epic/
    epic.json
    <report content>           # chapters, sections, fragments
    ___requirements/
      prototypes/              # revisioned interactive prototypes
      stories/                 # stories and their acceptance criteria
      citations/
      relations/               # typed, pinned edges between resources
      coverage-exceptions/     # immutable per-criterion, per-axis decisions
    ___design/                 # technical design chapters
    ___slides/                 # the epic's implementation deck
    ___workplan/               # waves, work items, dependencies, contracts
    ___quality/
      policies/
      test-cases/<id>.test/
  ___claims/
  ___verifications/
  ___merges/                   # landed commits and their branch messages
  ___reviews/<id>.review/      # pull-request reviews: a deck, approvals, comments
```

Report content and epic roots are invalid at the application root. No URN names
an epic, so every resource ID is unique across the whole Saga: a story, deck,
slide, fragment, or any other resource can move between epics without breaking
a link to it.

Each record is interpreted by the schema its `$schema` names. Report content
records use the v2 component schemas, living requirement and work-plan records
the v3 schemas, and deck, slide, and item records the v4 deck schemas; those
component versions are part of this one format, not separate formats. The
records introduced with the format are listed below. Every listed object
boundary is closed; record files are bounded to one MiB, collection limits are
enforced at runtime, and every ID uses `[A-Za-z0-9][A-Za-z0-9._-]{0,127}`.

| Record | Schema | Required semantic fields | Runtime-only checks |
| --- | --- | --- | --- |
| Manifest | `v5/saga.schema.json` | identity, title, source, `version: 5` | canonical repository identity |
| Epic | `v5/epic.schema.json` | immutable ID, title, creation time | directory name equals ID |
| Persona identity, revision, event | `v5/persona*.schema.json` | name and description; lifecycle `active` or `retired` | one root, acyclic revision and event graphs |
| Term identity, revision, event | `v5/term*.schema.json` | name and definition; optional aliases, story and record links, and code references; lifecycle `active` or `retired` | linked stories and records exist; aliases distinct from the name |
| Flag identity, revision, event | `v5/flag*.schema.json` | description and at least one story or epic target; state `off`, `on`, or `retired` | targets exist; `retired` is terminal |
| Relation | `v5/relation.schema.json` | endpoints, type, scope, pins required by the matrix, rationale, state, time | same Saga, no self-edge, canonical conflict ordering, graph acyclicity/currentness |
| Coverage exception | `v5/coverage-exception.schema.json` | one of six axes, criterion/revision pin, rationale, citation, supersession set | current revision, resolved citations, one unsuperseded head per criterion/axis |
| Test-case identity | `v5/test-case.schema.json` | immutable ID and creation time | filename/package match and one identity per package |
| Test-case revision | `v5/test-case-revision.schema.json` | full definition, parents, kinds, automation, ordered steps | one root, reachable acyclic graph, unique/non-reused step IDs |
| Test lifecycle | `v5/test-case-event.schema.json` | parents and proposed/active/deprecated/retired state | one root, reachable acyclic graph, explicit multi-head conflicts |
| Quality evidence | `v5/quality-evidence.schema.json` | test revision, role, at least one locator, supersession set | current code references, resolved local URNs, acyclic supersession |
| Test run | `v5/test-run.schema.json` | test revision, parents, exact source identity, result, evidence, execution time | current source/revision/evidence, one root, acyclic graph, visible multi-head conflicts |
| Quality policy | `v5/quality-policy.schema.json` | criterion/revision, required kinds, allowed automation, rationale | resolved current criterion, acyclic supersession, one policy head |

Nothing in a Saga is required up front. A first change is asked only to have
its implementation explained; personas, stories, design, quality, and terms
grow from there. Coverage is reported, never enforced (section 11).
Coverage-exception records are read and evaluated; no command writes them
yet.

### Application records

**Epics** are durable areas of the product, not changes: revising a story that
belongs to an older epic refines that area in place. An epic's `epic.json` is
immutable and its URN is `urn:change-saga:<saga>:epic:<id>`. Moving a story
between epics moves its directory and changes nothing else.

**Personas** are application-level living records with an immutable identity,
append-only revisions (`name`, `description`), and lifecycle events whose root
state is `active`; a persona can be retired and restored. URNs are
`urn:change-saga:<saga>:persona:<id>`, with `:revision:<id>` and `:event:<id>`.
A story revision may name the personas it serves in `personas`; the list is
optional, but any persona it names must exist. Persona coverage (which active
personas an accepted story serves, and which stories served only a retired
persona) is reported and never blocks.

**Feature flags** have an immutable identity, revisions that name at least one
story or epic target, and lifecycle events with states `off`, `on`, and
`retired`. A current `off` or conflicted flag gates its targets, and a story is
gated directly or through its epic; a retired flag gates nothing. Status
reports each story as `not_implemented`, `implemented_not_enabled`, or
`implemented_enabled`, where implemented means every current criterion's
implementation axis is covered.

The **overview** is formal. Its parts are the project **name** (the manifest's
`title`), the **elevator pitch** (`___overview/pitch.fragment`), the
**description**, a short essay (`___overview/description.fragment`), and **terms
and vocabulary** (`___overview/terms/<id>.term/`). Nothing else may live in
`___overview`. Every part is optional; a missing part is reported as a gap and
never blocks.

A **term** names a word the project uses that a newcomer would not know. Terms
are living records like personas: an immutable identity, append-only revisions
(`name`, `definition`, and optional `aliases`, `stories`, `records`, and `code`),
and lifecycle events whose root state is `active`. URNs are
`urn:change-saga:<saga>:term:<id>`, with `:revision:<id>` and `:event:<id>`.
`stories` and `records` link the term to the stories, personas, epics, flags, or
other terms it relates to, and must exist. `code` holds code references
(section 5) to the lines that define the term, usually an enum value or a
constant. The links run both ways: a term reaches its stories and code, and a
line of code or a story reports the terms that reference it.

Term code references never count toward coverage; they are watched instead. A
term's references are judged at the head, so renaming the enum or constant it
names makes the term stale, naming exactly which term to update. When a
comparison adds an enum value or a typed constant that no term references, and
no term's name or alias matches it, a growth suggestion proposes defining it;
it never blocks. Recognizing enum values and constants is a conservative
per-language heuristic.

The **onboarding deck** is a flat deck with role `onboarding`, one per
application. Its Items carry `record`, the URN of a persona, epic, or story they
explain, and never own code references. Implementation decks keep role
`change` and never carry `record`.

### V5 identities and quality records

Quality URNs are canonical and local to the manifest Saga ID:

```text
urn:change-saga:<saga>:test-case:<test>
urn:change-saga:<saga>:test-case:<test>:revision:<revision>
urn:change-saga:<saga>:test-case:<test>:step:<step>
urn:change-saga:<saga>:test-case:<test>:event:<event>
urn:change-saga:<saga>:test-case:<test>:evidence:<evidence>
urn:change-saga:<saga>:test-case:<test>:run:<run>
urn:change-saga:<saga>:quality-policy:<policy>
urn:change-saga:<saga>:coverage-exception:<exception>
```

A test identity is immutable. Revisions are append-only complete snapshots;
their `steps` array is execution order. `coverage_kinds` is a non-empty unique
subset of `positive`, `negative`, and `edge`; `automation` is `manual`,
`automated`, or `hybrid`. Lifecycle is a separate append-only event graph with
states `proposed`, `active`, `deprecated`, and `retired`. Definition state does
not imply a run result. An active test can satisfy coverage only when its
current revision contains at least one step and a current direct `verifies`
relation.

The unique lifecycle root MUST be `proposed`. For a single-parent event, the
allowed transitions are `proposed -> active|deprecated|retired`,
`active -> deprecated|retired`, and `deprecated -> active|retired`; `retired`
is terminal. A multi-parent event is an explicit head reconciliation and may
choose any lifecycle state. Duplicate or missing parents, multiple roots, and
cycles are invalid. Multiple heads remain a loadable, blocking conflict rather
than being ordered by timestamp.

Quality evidence roles are `test_implementation`,
`implementation_under_test`, and `execution_artifact`. The first two require
one or more code references. Execution artifacts forbid code references and
require a verification or citation. Every evidence record contains the four
explicit arrays `code`, `verifications`, `citations`, and `supersedes`;
projection uses the supersession graph, never timestamps, to choose current
heads.

Runs are immutable graph events with results `passed`, `failed`, `blocked`, or
`skipped`. A passing literal is current only when the run is the unique head,
pins the unique current test revision, repeats the Saga's current canonical
repository/base/product-head identity, and resolves all referenced current
evidence. A command is optional so manual tests can record an execution without
inventing a shell command. Reads MUST NOT execute it.

Policies are immutable per-criterion decisions. `required_kinds` and
`allowed_automation` are non-empty unique sets. In the absence of a policy the
quality projection requires `positive`; a policy can additionally require
`negative` and/or `edge`. Coverage exceptions use one of the six axes
`prototype`, `ux`, `ui`, `technical`, `quality`, or `implementation`, pin one
story revision, and require a nonblank rationale plus at least one resolved
citation. An exception declares an axis inapplicable to a criterion, and there is one unsuperseded head per
criterion/axis. There is no exception from exact changed-source accounting: documentation-only work still
ends at its documentation diff.

### V5 relation matrix

Every v5 relation carries `scope`. `self` applies only to the named source;
`descendants` is valid only for a Deck or Slide source on an `addresses` or
`explains` relation. An Item and every report-design source MUST use `self`.
Every digest is lowercase `sha256:` followed by 64 hexadecimal digits.

| Type | Source | Target | Required pins |
| --- | --- | --- | --- |
| `refines` | story or criterion | story or criterion | source and target story revisions |
| `addresses` | report design, Deck, Slide, or Item | story or criterion | source content digest and target story revision |
| `implements` | work item | report design or criterion | work-item revision; target design digest or target story revision |
| `explains` | Deck, Slide, or Item | story or criterion | target story revision; source digest is recommended |
| `verifies` | claim, verification, or test case | criterion | target story revision; test cases also pin their revision |
| `supersedes` | resource | same resource kind | pins applicable to each mutable endpoint |
| `conflicts_with` | story or criterion | story or criterion | both story revisions |

All endpoints MUST use the same Saga ID. Self-relations are invalid. Active
`refines` and `supersedes` graphs are acyclic. A `conflicts_with` edge is stored
once with endpoints in lexical order and is interpreted symmetrically. V3
relations remain valid history in a v5 container; their existing meaning and
bytes do not change. In particular, a v3 `explains` relation is a legacy review
explanation and never becomes design coverage.

### Canonical visual content digests

Visual digests use the following `visual-v1` algorithm. It is deliberately
independent of filesystem record names and excludes diffs, claims,
verifications, comments, approvals, and every other overlay.

1. Parse each v4 manifest with strict JSON (duplicate keys and trailing data
   are invalid), then serialize its semantic JSON value with RFC 8785 JSON
   Canonicalization Scheme (JCS). The original `$schema`, if present, is part
   of the value. Storage filenames and filesystem metadata are not.
2. A framed value is `uint64-big-endian(byte-length) || bytes`. Hash input is a
   UTF-8 domain string followed by NUL, then each stated value as one frame.
3. Item manifest digest is SHA-256 over domain
   `change-saga-visual-item-manifest-v1` and the canonical Item manifest.
4. Item content digest is SHA-256 over domain `change-saga-visual-item-v1`, the
   canonical Item manifest, and the raw bytes of its Slide's validated
   entrypoint asset.
5. Slide content digest is SHA-256 over domain `change-saga-visual-slide-v1`,
   the canonical Slide manifest, the raw entrypoint bytes, then every Item
   manifest digest ordered by `(rank, id)` as lowercase ASCII
   `sha256:<64-hex>`.
6. Deck content digest is SHA-256 over domain `change-saga-visual-deck-v1`, the
   canonical Deck manifest, then every Slide content digest ordered by
   `(rank, id)` in the same ASCII form.
7. The externally stored result is lowercase `sha256:<64-hex>`.

Thus a selector, Item, Slide asset, Slide manifest, or Deck manifest edit
invalidates the relevant relation; adding or changing an overlay does not.
Deck/Slide containment is traversed only after validating exact parent IDs and
only when the relation explicitly says `scope: descendants`.

### Query API v2 contract

The closed response contract is
[`schema/v5/query-v2.schema.json`](schema/v5/query-v2.schema.json). V2 uses the
existing query envelope with `schema: "change-saga.ai/v2"`, snapshot-bound
cursor pagination, deterministic ordering, and operation-specific closed
`data`. V1 responses remain byte-compatible.

Every transitive path is an ordered array of typed hops. Hop types are
`relation`, `contains`, `owns_diff`, `has_quality_evidence`, and
`matches_item_diff`; each carries `from`, `to`, and an independently reported
`current`, `stale`, `invalid`, or `conflicted` status. A path separately reports
`precision`, relation/evidence provenance, shared-diff state, and diagnostics.
No score or percentage substitutes for the per-axis facts.

Design coverage states are `covered_direct`, `covered_broad`, `excluded`,
`gap`, `stale`, `invalid`, and `conflicted`. Quality states are `covered`,
`missing_kind`, `not_run`, `failed`, `blocked`, `stale`, `excluded`, `invalid`,
and `conflicted`. Every criterion/axis cell is exactly one of a current link,
an explicit exclusion, or a visible gap.

Schema validation cannot establish same-Saga equality, graph rules, current
Git identity, exact selector resolution, or canonical URI equivalence. Runtime
validation MUST enforce those checks before returning a current path or a ready
gate. Reads do not write files, execute commands, fetch URLs, or resolve
external content.

## Implementation deck

The implementation deck is the core of a Change Saga: the visual explanation
of the change a reviewer came to read. It lives beneath
`___slides/<deck-id>.deck/`, and the reviewer presents it as the
Implementation section itself. A Saga normally has one deck; several decks are
allowed when the change has separable implementations. Stories, acceptance
criteria, technical design, work-plan history, prototypes, chapters, and
ordinary fragments are never converted into slides.

Each deck bundle is flat and independently mergeable. It contains exactly one
deck record plus its slide, Item, asset, and `40-e` evidence records, all
interpreted by the v4 deck schemas. The directory basename must equal the deck
ID and the deck uses `role: "change"`. Deck, slide, and Item URNs use the Saga
ID:

```text
urn:change-saga:<saga-id>:deck:<deck-id>
urn:change-saga:<saga-id>:slide:<slide-id>
urn:change-saga:<saga-id>:slide:<slide-id>:item:<item-id>
```

```text
checkout.saga/
  saga.json
  ___epics/checkout.epic/
    epic.json
    ___slides/
      validation-flow.deck/
        10-d-....json
        20-s-....json
        20-s-....svg
        30-i-....json
        40-e-....json
```

`add-deck`, `add-slide`, `set-slide-content`, and `add-item` author the deck.
Exact diff evidence on a slide is owned only by Items, so every changed line a
reviewer sees in the deck is attached to the specific visual element that
explains it.

Stories and their acceptance criteria are the traceability backbone. A deck,
slide, or Item relates to a story or criterion through an active relation that
pins the exact story revision it relied on. Linking a story applies to every
acceptance criterion in that pinned revision; linking a criterion is the
narrower form. A relation becomes stale when its pinned revision is no longer
current or its visual source disappears, and a stale relation is never
coverage. Relations never move exact diff ownership away from Items.

`query traceability` returns the complete current paths from each accepted
criterion through its linked review targets to Item-owned code references. It can be
filtered in reverse with `--ref <location>`, or with `--commit` to select
evidence pinned at that commit. Its `unlinked_code_evidence` collection exposes Item
evidence that has no active, current path to an accepted story. Thus a caller
can traverse from a story to code, or from the current head commit or diff back
to the story, without duplicating story text inside slide records.

## Report content, evidence, and review

The report carries the Saga's authored narrative, and the same component model
carries code evidence and claims across every part of the Saga. Report records use the v2 component schemas.

## 1. Model

A Change Saga is a Git-native review document with six layers:

1. **Overview** explains the change as a whole.
2. **Chapters** divide the change into independently reviewable units—roughly
   the PRs that might have existed if the work had been split.
3. **Sections and fragments** recursively organize each chapter; fragments are
   the smallest units of authored content.
4. **Diff links** connect a saga, chapter, section, or fragment to immutable source
   changes, including changes in another repository.
5. **Claims and verification** make falsifiable author assertions and their
   independently recorded verification state machine-readable.
6. **Review overlays** add anchored discussions without modifying the authored
   fragments.

Every persistent object is an ordinary file. Content and review data can be
branched, merged, audited, and committed independently.

## 2. Root and source

A saga root ends in `.saga` and contains `saga.json` conforming to
[`schema/v5/saga.schema.json`](schema/v5/saga.schema.json):

```json
{
  "$schema": "https://changesaga.dev/schema/v5/saga.schema.json",
  "version": 5,
  "id": "checkout-rewrite",
  "title": "Checkout rewrite",
  "source": {
    "repository": "https://github.com/acme/payments.git"
  }
}
```

New sagas also contain a root `README.md` written by `change-saga init`. It is
non-normative reviewer bootstrap material: it identifies the directory as a
Change Saga, explains how to install and open the local reviewer, and directs
AI assistants to the structured `change-saga query` interface. Engines ignore
the file when loading content and calculating diff coverage. Because a saga may arrive in an untrusted pull request,
the bootstrap tells assistants to obtain user permission before downloading or
executing software.

`source.repository` is a canonical absolute URI and identifies the source
repository, not necessarily the repository containing the saga. Repository
identities never contain URL userinfo; credentials in a remote URL are removed
before the identity is persisted, and a persisted identity that still carries
userinfo is invalid rather than silently stripped on read.

The manifest holds no comparison. A comparison is how a Saga is opened: commands
that read one take `--against REV` and an optional `--head REV` (default
`HEAD`), and compare the merge-base of the two through the head, exactly as a
pull request does. Without `--against`, a command observes the Saga at its head
with no changed-line accounting. Comparisons are always between commits: code
references pin commits, so uncommitted working-tree changes are not a
comparison. A pull request belongs to its review (section 8), not to the
manifest.

A Saga that lives in a companion repository, separate from its code, records a
sync cursor in `sync.json`, conforming to
[`schema/v5/sync.schema.json`](schema/v5/sync.schema.json): the code commit the
Saga currently documents. Every Saga commit that updates the documentation moves
the cursor, with `change-saga sync` or `repin`. When such a Saga is compared, the
code delta comes from the code repository and the Saga delta from the Saga
commit whose cursor matched the base. A Saga in its code repository has no
cursor: it documents the commit it is read at.

`change-saga init` uses the canonical portable `origin` identity when available.
Without an origin, or when origin is itself a local path, the author must provide
a portable `--repository` URI or explicitly opt in to a local `file://` identity
with `--allow-local-repository`; a local home-directory path is never selected
silently. When a saga is stored
separately, readers provide a local source checkout with `change-saga status
--repo PATH` or `change-saga open --repo PATH`. Local checkout paths are runtime
configuration and are never committed into the portable format. Readers verify
a declared file identity or checkout origin against the declared repository; a
checkout without origin is unverifiable and fails closed. CLI authoring and
status commands expose
`--allow-repository-mismatch` for the exceptional case where a known-equivalent
checkout cannot be identified from its origin.

## 3. Chapters, sections, and fragments

The root overview is followed by direct child directories ending in `.chapter`.
Each contains `chapter.json` conforming to
[`schema/v2/chapter.schema.json`](schema/v2/chapter.schema.json):

```text
backend.chapter/
├── chapter.json
├── overview.fragment/
└── request-flow/
    ├── section.json
    └── interactive-flow.fragment/
```

```json
{ "version": 2, "id": "backend", "title": "Backend behavior", "order": 20 }
```

A chapter is a review boundary: it may own diffs, threads, and approvals, and
can be reviewed independently by a different person. Chapters are direct
children of the saga root and cannot nest. Ordinary directories inside them are
recursive sections containing `section.json`:

```json
{ "version": 2, "id": "request-flow", "title": "Request flow", "order": 20 }
```

A directory ending in `.fragment` is a fragment package rather than a section.
It contains `fragment.json` plus an entrypoint and any supporting files:

```text
interactive-flow.fragment/
├── fragment.json
├── index.html
├── app.js
├── styles.css
├── sample-data.json
├── ___landmarks/
│   └── submit-action.landmark/
│       ├── landmark.json
│       └── ___code/
└── ___code/
```

```json
{
  "version": 2,
  "id": "interactive-request-flow",
  "title": "Try the new request flow",
  "media_type": "text/html",
  "entrypoint": "index.html",
  "order": 30
}
```

The reference engine supports `text/markdown`, `text/plain`, `text/html`,
`image/svg+xml`, and raster `image/*` fragments. A fragment is directory-backed
so HTML and SVG can use JavaScript, CSS, images, data, and other relative assets.

`entrypoint` is a normalized package-relative slash path. It is never absolute,
never begins with a drive letter, never contains `\` or a control character,
never traverses with `.` or `..`, and never addresses `fragment.json` or a
reserved `___` path. Backslash is excluded because it is an ordinary filename
byte on Unix and a separator on Windows, so one saga would otherwise address two
different files. Entrypoints cannot escape their package. Engines should warn
about component names that cannot be checked out on every supported platform,
such as reserved Windows device names.

Markdown headings should declare a durable fragment-local anchor after the
visible heading text:

```markdown
## Request validation {#request-validation}
```

The anchor begins with a lowercase letter, contains only lowercase letters,
digits, and hyphens, is at most 64 characters, and is unique within its
fragment. Engines namespace it with the fragment target when producing a page
anchor. The reference validator warns about headings without explicit anchors
and rejects invalid or duplicate explicit anchors. Engines may derive fallback
anchors for older content, but authored anchors are the stable sharing contract.

Markdown footnotes are the standard prose citation form. Authors place the
citation after a focused implementation claim and make the reference definition
an exact-text landmark. When that landmark owns diff evidence, reference
renderers should let both the inline marker and the definition open the linked
code. Citation definition text should remain plain, unique within its fragment,
and focused on one claim so its exact-text selector is durable. Footnote syntax
does not itself create coverage; the landmark's independent `___code` records
remain the authoritative association.

A prose diff citation and a code-bearing visual landmark have identical
completion criteria: a stable addressable landmark plus focused diff evidence.
A footnote marker or definition without that evidence is an incomplete citation,
just as an SVG node without linked diffs is incomplete. Validators SHOULD warn
for each Markdown footnote definition that lacks a matching exact-text landmark
or whose landmark owns no diff evidence. Living-Saga provenance citations are a
separate requirements surface and do not satisfy this implementation-evidence
contract.

Addressable subparts use one `<id>.landmark` package under `___landmarks/`.
Its `landmark.json` conforms to
[`schema/v2/landmark.schema.json`](schema/v2/landmark.schema.json) and contains a
stable fragment-local ID, a reviewer-facing label, and one selector:

- `heading` enriches an explicit Markdown heading anchor;
- `element` identifies an `id` in an HTML or SVG entrypoint;
- `text` identifies an exact quote, with optional prefix and suffix, in Markdown
  or plain text;
- `region` identifies a normalized rectangle in an image.

```json
{
  "version": 2,
  "id": "submit-action",
  "label": "Submit action",
  "description": "The validated request crosses into persistence.",
  "selector": { "type": "element", "element_id": "submit-action" }
}
```

Landmark IDs follow the Markdown-anchor grammar and are unique within the
fragment. A `heading` package intentionally shares its ID with the heading it
enriches. Engines combine the fragment target and landmark ID into a portable
page anchor and assign the landmark its own target URN. Each `___code/*.json`
inside the landmark package associates code references with that exact
narrative element; each association remains an independent file.
Meaningful visual landmarks should carry a concise semantic `description` that
explains their role without relying on geometry, color, or position. Query
clients return this description so non-visual consumers do not need to infer
meaning from SVG or HTML source.
Every code-bearing SVG or HTML node or edge should prefer its own stable element
ID and element landmark over evidence attached only to the enclosing fragment.

SVG element landmarks use the rendered element bounds as their on-canvas
interaction area by default, including groups, nodes, lines, paths, and graph
edges. Static SVG, HTML, and image landmarks may include a normalized `hotspot`
rectangle to override or supply that geometry. The renderer uses an external
overlay to reveal permalink and related-code controls directly on hover without
trusting or modifying the fragment document. A `region` selector is itself a
hotspot. The fragment target remains the portable fallback for media without a
native selector; authors may use an HTML wrapper with element landmarks when
inner addressability is important.

HTML and SVG fragments execute in an iframe with `sandbox="allow-scripts"` and a
network-denying Content Security Policy. They can execute bundled JavaScript but
cannot access the review application, navigate its parent, submit forms, open
popups, or make network connections. Engines must provide equivalent isolation;
they must not insert fragment HTML directly into trusted application markup.

## 4. Stable target URNs

Review and engine data use stable identifiers rather than filesystem paths:

```text
urn:change-saga:<saga-id>:saga
urn:change-saga:<saga-id>:chapter:<chapter-id>
urn:change-saga:<saga-id>:section:<section-id>
urn:change-saga:<saga-id>:fragment:<fragment-id>
urn:change-saga:<saga-id>:fragment:<fragment-id>:landmark:<landmark-id>
```

IDs are 1–128 characters from `A-Z`, `a-z`, `0-9`, `.`, `_`, and `-`, beginning
with an alphanumeric character. Chapter, section, and fragment IDs are unique
across the saga. Renaming or moving a directory does not change its target URN.

## 5. Code references

Evidence never stores a diff. A code reference says "this node explains these
lines, as of this commit", and a diff is a way of viewing references against
two commits. Evidence records conform to
[`schema/v2/code.schema.json`](schema/v2/code.schema.json):

```json
{
  "commit": "b127...40-hex-commit",
  "path": "internal/checkout/handler.go",
  "start": 18,
  "end": 42,
  "digest": "sha256:<hex of the exact referenced bytes>",
  "note": "The behavior demonstrated by this fragment"
}
```

`commit` is a full commit object name, never a symbolic ref. `start` and `end`
select a 1-based inclusive line range; both are absent for a whole-file
reference. `digest` is `sha256:` followed by the hex digest of the exact
referenced bytes: the selected lines including their newlines, or the whole file
blob. References do not repeat the repository; it is the Saga's declared
`source.repository`.

The compact location form, used by CLI flags, query arguments, URLs, and
traceability results, omits the digest:

```text
<commit>:<path>[#L<start>[-L<end>]]
```

**Deletions** reference the base side of a comparison, since removed lines exist
only there. Renames, mode and type changes, binary changes, and file additions
or deletions are referenced by whole-file references.

### 5.1 Viewing a reference at another commit

A reference pinned at commit P is viewed at commit V by diffing P against V
(renames followed, `.saga` paths excluded):

- The pinned digest is verified first. A mismatch makes the reference stale.
- Insertions and deletions entirely before the range shift it; the reference is
  **remapped** and stays current.
- Any change touching the range, including an insertion inside it, makes the
  reference **stale**, with the reason and the diff since the pin available.
- A whole-file reference remaps only on a pure rename and is stale on any
  content, mode, or binary change.
- Commits that change only `.saga` paths leave the code identical, so they
  never move or stale a reference.
- If the pinned commit is no longer available, the reference is resolved by
  searching for its digest in the same path at V; only a unique match counts.

A reference that is current but covers no changed line is not stale: it
explains unchanged code.

## 6. Attaching code

A report target (a section, fragment, or landmark) holds evidence in
`___code/*.json`, and a deck Item holds it in its `40-e-*.json` record, both as
`{"version": 2, "references": [ ... ]}`. The containing object is the target of
the evidence. An evidence file holds at least one reference.

Coverage is computed for a comparison, never stored. For the merge-base M and
head H (excluding `.saga` paths), every reference is viewed at both M and H.
Added and modified lines are covered by references current at H, and deleted
lines by references current at M. File events are covered only by whole-file
references on the side where the file exists (M for a deletion). A whole-file
reference does not cover the file's individual lines. `cover --changed-lines`
therefore writes a whole-file reference for each file event plus line ranges
for the changed lines. All atoms are mapped when every changed line and file
event is covered and no reference in the Saga is stale. This is an omission
invariant only; it does not establish explanation quality, claim truth, review
completion, or correctness. Overlap is reported but permitted.

CLI-generated evidence filenames are a deterministic function of the target's
canonical reference set and exclude reviewer-facing notes. Unrelated reference
sets therefore write unrelated files, while two branches that explain the same
code differently write the same path and require an explicit Git
reconciliation. A generated filename collision MUST NOT be resolved by adding a
timestamp or numeric suffix: preserving both records would turn an authored
disagreement into an accidental overlap. Explicit `--name` values remain stable
author-chosen repair handles.

Every Git-reported product file has at least one event or line atom, so an
unrepresentable file record cannot make a nonempty comparison appear complete.
Comparisons are between commits; uncommitted working-tree changes are not a
comparison.

Changes under a `.saga` path in the source repository are classified separately
as saga-only changes. When source and saga are different repositories, all saga
history is naturally outside the source comparison.

### 6.1 Observing and comparing

Opened without a comparison, a Saga is **observed**: everything is shown as of
its head, and each code reference's health is judged against that commit.

Opened with `--against`, a Saga is **compared**, and the comparison is reported
in three layers (`status --json` under `comparison`, `query layers`, and the
reviewer's Change view):

1. **Changed:** Saga records added, revised, or retired between the Saga at the
   merge-base and the Saga at the head, each with its before and after.
2. **Affected:** records the change did not edit but invalidated: records whose
   referenced code the change touched, records pinned to a revision the change
   revised, and records reached through the persona → story → design → code
   chain, so a change to code alone still reaches the stories and personas it
   touches. A removed line or destructive file event that intersects code a
   record references means that record must be revisited; a new line adjacent to
   referenced code is a prompt for consideration, not proof that the record is
   stale; a change no record can own requires new or expanded documentation.
3. **Code:** the diff, grouped under the records whose references it touches,
   plus every changed line no record references.

A comparison never compares authored prose, rendered content, or diagram
geometry, and it never rewrites evidence.

**Why things changed.** When a slide, design, or test case drops out and
another takes its place, the comparison pairs them: the pairing is inferred from
a shared story or criterion, an explicit `supersedes` relation wins, and an
ambiguous case lists its candidates. The commit messages in the comparison are
attached to the records whose files or referenced code each commit touched,
including the messages a squash merge preserved in `___merges`. `query history
--node URN` reports when a record was introduced, what it replaced, the reviews
that changed it, and the command that opens each of those comparisons.

### 6.2 Re-pinning at merge

When a change lands, `change-saga repin --onto REV [--branch REV]` re-pins
coverage references from branch commits to the landed commit, following moved
lines exactly as viewing does. A squash merge or a deleted branch therefore
loses nothing. References to deleted code, which exist only at the base, stay
pinned and are reported. Claims, quality evidence, threads, and file reviews are
immutable records; they keep their pins and resolve through the same viewing
rules and digest fallback.

`repin` also writes `___merges/<landed-commit>.json`, conforming to
[`schema/v2/merge.schema.json`](schema/v2/merge.schema.json). It records the
landed commit, the merge-base it was compared from (so `--against <base> --head
<commit>` reproduces the change), the review it froze, and the branch's commit
messages, ancestors first, so the
reasoning in individual commits survives a squash merge. `--branch` must be
given while the branch still exists. `--dry-run` reports the complete impact
without writing.

`change-saga references [--stale] [--diff]` reports every reference's health:
current, remapped, or stale with its reason and the diff since the pin.

## 7. Claims and verification

Author claims are independent `___claims/<id>.json` records conforming to
[`schema/v2/claim.schema.json`](schema/v2/claim.schema.json). A claim contains a
falsifiable `statement`, a `kind`, one existing narrative `target`, at least one
code reference in `evidence`, and `created_at`. Claim evidence never
contributes to coverage; readers independently report whether it is current and
whether every matching atom is already mapped to the claim's target.

Verification results are independent append-only
`___verifications/<id>.json` records conforming to
[`schema/v2/verification.schema.json`](schema/v2/verification.schema.json).
Each references a claim and records `unverified`, `verified`, `failed`, or
`inconclusive`, a human-readable summary, and—except for an unverified result—a
method: `test`, `command`, `measurement`, `inspection`, or `analysis`. An
optional command makes a check reproducible. The latest result is convenient
navigation state; every earlier result remains part of the audit history.

Claims and results never accept an author name. Readers derive attribution from
the Git commit that first introduced each file.

## 8. Reviews

The Saga is documentation: stories, designs, test cases, epic decks, and report
content carry no approvals and no comments. Review happens in **reviews**. A
review is equivalent to a pull request, and it is always a slide deck:

```text
___reviews/<id>.review/
├── review.json
├── deck/                      # one flat deck bundle with role "review"
│   ├── 10-d-....json
│   ├── 20-s-....json
│   ├── 30-i-....json
│   └── 40-e-....json
├── approvals/<event-id>.json
└── comments/<event-id>.json
```

`review.json` conforms to
[`schema/v5/review.schema.json`](schema/v5/review.schema.json). It names the pull
request (`number`, `url`), the `base` it merges into, the `head` reference it
follows (omitted means the checkout's `HEAD`), and its creation time. A review's
range is the merge-base of base and head through head, exactly as a pull
request's is, and the head follows the pull request as commits are pushed. There
is one review per pull request number.

The review deck explains what the change did and why, including the transition
and reasoning that the current documentation no longer shows. Its URNs are
scoped to the review, `urn:change-saga:<saga>:review:<id>[:deck:<d>|:slide:<s>[:item:<i>]]`,
so slide IDs never collide across reviews. An Item may reference code, which a
reviewer sees as a diff against the review's base, and may carry a `record`: the
URN of a persona, epic, story, test case, deck, slide, chapter, section, or
fragment in the documentation. Review decks never join the documentation's
structure, so they have no effect on coverage, readiness, or comparison layers.

**A review deck must account for its change.** Review coverage is computed,
never stored, over the review's own range with the same rules as documentation
coverage (section 6): every added or modified line must be covered by a review
Item reference current at the head, every deleted line by one current at the
base, and every file event by a whole-file reference. It is reported per review
(`review list`, `status` under `reviews[].coverage`, and the review page) as
covered and total counts, the uncovered lines with ready-to-use locations,
stale references, and overlap. It is a report, not a verdict; `review list
--uncovered` lists only reviews with gaps. Review decks never count toward the
documentation's own coverage. `cover` on a review Item defaults to the review's
range, so it needs no `--against`.

**Approvals are per slide.** Each decision is an append-only record in
`approvals/`, conforming to
[`schema/v5/review-approval.schema.json`](schema/v5/review-approval.schema.json):
the slide, a state of `approved`, `changes_requested`, or `none` (which
withdraws), the reviewer, the full pull-request head commit at the time of the
decision, a `slide_digest` over the slide's manifest, visual, Items, and their
evidence, an optional body (required when requesting changes), and the creation
time. The reviewer is `human` or `ai`; an AI reviewer also names a distinct
reviewer seat, the agent, and the exact model, so `Claude 1` and `Claude 2`
remain independent even on the same model. Git supplies the authoritative author
identity. The latest decision for each Git author and reviewer seat on a slide
is that reviewer's current decision.

**A decision is out of date** when the slide's digest differs from the one it
recorded, or when the code referenced by any of the slide's Items changed
between the decision's commit and the pull request's current head. Currency is
`current`, `out_of_date` with its reasons, or `unknown` when the decision's
commit is not available. This is how a review is known to be out of date for a
pull request: slide by slide.

**The format records; teams decide.** There is no aggregate state for a slide
or a review. Every current decision is reported individually, and an AI
approval is never presented as a human one. Repository policy, not this
format, decides which combination of decisions permits merging.

**Comments** are append-only records in `comments/`, conforming to
[`schema/v5/review-comment.schema.json`](schema/v5/review-comment.schema.json).
A comment targets a review slide or Item, may reply to another comment, carries
the reviewer and the head commit, and may resolve or reopen its thread.

Every decision and comment is its own file with a unique time-plus-random
identifier, written with exclusive creation, so two reviewers acting on the same
slide add disjoint files and do not create a Git conflict.

**After merge**, `repin` freezes the review: `review.json` gains `merged` with
the exact base and head commits and the landed commit, and the merge record in
`___merges/` names the review. A frozen review stays viewable against its exact
range and refuses new decisions and deck edits. A node's history lists the
reviews that changed it.

## 10. Reserved names

`___code` is reserved on saga/chapter/section/fragment targets.
`___landmarks` is reserved inside fragments.
`___overview`, `___designsystem`, `___personas`, `___featureflags`,
`___onboarding`, `___epics`, `___reviews`, `___claims`, `___verifications`, and
`___merges` are reserved at the saga root. `___requirements`, `___design`,
`___workplan`, `___slides`, and `___quality` are reserved at an epic root;
`___slides` contains only real `<deck-id>.deck` directories.
Reserved metadata directories must be
real directories, not symlinks. So must every entity package: a `.chapter`,
`.fragment`, or `.landmark` entry that is a symlink or a
regular file is invalid rather than ignored, because silently skipping it would
hide authored content behind a valid-looking saga. Other names beginning with
`___` are invalid.

## 11. CLI behavior

- `change-saga init SAGA` creates a Saga in the one format, ready for
  prototypes and stories. It never creates any other kind of Saga.
- `change-saga epic add`, `persona add|revise|set-state`, `flag
  add|revise|set-state`, and `story move --story URN --epic ID` author the
  application's structure. Commands that create a record inside an epic
  require `--epic`; commands that revise one accept it as an assertion.
- `change-saga term add|revise|set-state` authors terms, with `--ref
  <rev>:<path>#L<start>[-L<end>]` for their code, and `change-saga overview
  set-pitch|set-description` writes the overview's text. `query terms` finds
  terms by term, story, or code location.
- `change-saga references [--stale] [--diff]` reports every code reference's
  health, and `change-saga repin --onto REV [--branch REV]` re-pins references
  when a change lands (see section 6.2).
- `change-saga install-skill` prints an agent-agnostic prompt for installing the
  project-local Change Saga authoring skill. It MUST NOT mutate the repository
  or assume an agent-specific skill path.
- `change-saga add-fragment` creates Markdown, HTML, SVG, text, or image fragments.
- `change-saga add-landmark` creates and validates a separately addressable
  Markdown heading, exact text span, HTML/SVG element, or normalized image
  region. It prints the target accepted by `change-saga cover`.
- `change-saga add-claim` creates one falsifiable assertion file without
  changing coverage. `change-saga verify-claim` appends one independent result.
- `change-saga add-chapter` creates a top-level independently reviewable chapter and
  its overview fragment.
- `change-saga cover --target ...` attaches code references to a target.
- `change-saga status --json` (`status_schema` `change-saga.status/v3`) reports
  a **coverage report** under `coverage`: a scope (the change with `--against`,
  otherwise the whole application; `--epic` narrows either) and six areas.
  `implementation` counts changed lines referenced by an implementation deck;
  `stories` and `personas` count changed lines that reach a story or persona
  through the chain; `design` counts stories in scope that have design;
  `quality` counts acceptance criteria in scope that have a test; `health`
  counts existing records that went stale or broke. Each area gives its unit,
  total, covered, uncovered, and `complete`, with the covered and uncovered
  entries listed. It also reports the pin-derived stale set, the comparison
  layers, reviews, and ordered `next_actions`: each is a command shape from the
  grammar `spec --json` publishes, or one focused question. Growth suggestions
  (a story, persona, design, test, or term that would help) explain the practice
  they teach and are never demanded. Nothing is reduced to a score or
  percentage, and the JSON shape is a contract teams may script against.
- `change-saga check --covers AREA[,AREA...]` answers whether the named areas are
  fully covered in scope, printing only their gaps; it takes the same
  `--against`, `--head`, and `--epic` as `status`.
- Commands that need an epic default to the only epic when there is exactly
  one. With none, the first such command creates one named after the branch
  (or, on a default branch, after the application).
- `--against REV [--head REV]` opens `status`, `references`, `cover`,
  `replace-coverage`, every `query` operation, `serve`, and `open` in compare
  mode; without it they observe. `query layers` returns the three comparison
  layers and `query history --node URN` a record's history.
- `change-saga sync --repo PATH [--commit REV]` moves a companion Saga's sync
  cursor.
- `change-saga query mappings --sort scrutiny` ranks broad or thin evidence
  records without claiming that a low score proves correctness. `query claims`
  and `query verifications` expose assertions, exact evidence, attribution, and
  result history.
- `change-saga review create|list|approve|request-changes|withdraw|comment`
  manages a pull request's review: its deck is authored with `add-slide`,
  `set-slide-content`, and `add-item` using `--review`, and decisions and
  comments apply to review slides and Items only.
- `change-saga open` serves a Saga view with attached-diff drawers, a Code Diff view
  with a changed-file tree, and a bidirectional Coverage Manifest. The Manifest
  must derive both code-to-narrative and narrative-to-code projections from the
  same atom assignments. Attached code is grouped by collapsed source file, and
  evidence `note` values provide the reviewer-facing what-and-why summary before
  linked ranges are expanded. The documentation has no comment or approval
  controls; review happens in reviews (section 8).
- `change-saga validate` checks structure, identifiers, URIs, anchors, entrypoints, and
  review history independently from coverage completeness.

`status` carries no verdict: it exits 0 whenever it can produce a trustworthy
report and 1 when it cannot (a structural error, records that fail to load such
as duplicate IDs, or a repository mismatch). Every gap, including uncovered
changed lines, is a finding. `check` exits 0 when every named area is covered,
3 when one has a gap, and 1 when the report cannot be trusted. `validate` exits
1 for structural errors. Unknown v2 JSON fields are rejected.
