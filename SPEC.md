# Change Saga format

A Change Saga is the durable, version-controlled record of one big change: the
kind that warrants product requirements, UX and UI design, technical design,
quality verification, and an implementation deck. The change itself is often
built quickly, in one large pull request; the Saga is what captures why it
exists, what it was meant to do, and how every changed line traces back to
that intent. A Saga starts with the big work and ends with the big work.

There is exactly one Saga format. It is identified by `version: 5` and the
schemas under [`schema/v5`](schema/v5). A Saga has one `saga.json`, one Saga
ID, report content, living requirements and work-plan records, quality
records, and the embedded implementation deck under `___slides/`. Readers
reject any other manifest version, a root `00-saga.json`, and a manifest
`presentation` member.

Git is the outer audit log. Saga records add the semantic history inside it:
immutable identities, append-only revisions and lifecycle events, relations
that pin the revisions they rely on, supersession, evidence, and review
decisions. Staleness is always derived from those pins, never from Git
history.

### Layout

The manifest carries identity and source only: it adds no aggregate quality,
coverage, relation, or deck fields.

```text
<id>.saga/
  saga.json
  <report content>             # overview, chapters, sections, fragments
  ___requirements/
    prototypes/                # revisioned interactive prototypes
    stories/                   # stories and their acceptance criteria
    citations/
    relations/                 # typed, pinned edges between resources
    coverage-exceptions/       # immutable per-criterion, per-axis decisions
  ___design/                   # technical design chapters
  ___slides/                   # the implementation deck
  ___workplan/                 # waves, work items, dependencies, contracts
  ___quality/
    policies/
    test-cases/<id>.test/
      test-case.json
      revisions/
      events/
      evidence/
      runs/
  ___claims/
  ___verifications/
  ___review/                   # review overlay
```

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
| Relation | `v5/relation.schema.json` | endpoints, type, scope, pins required by the matrix, rationale, state, time | same Saga, no self-edge, canonical conflict ordering, graph acyclicity/currentness |
| Coverage exception | `v5/coverage-exception.schema.json` | one of six axes, criterion/revision pin, rationale, citation, supersession set | current revision, resolved citations, one unsuperseded head per criterion/axis |
| Test-case identity | `v5/test-case.schema.json` | immutable ID and creation time | filename/package match and one identity per package |
| Test-case revision | `v5/test-case-revision.schema.json` | full definition, parents, kinds, automation, ordered steps | one root, reachable acyclic graph, unique/non-reused step IDs |
| Test lifecycle | `v5/test-case-event.schema.json` | parents and proposed/active/deprecated/retired state | one root, reachable acyclic graph, explicit multi-head conflicts |
| Quality evidence | `v5/quality-evidence.schema.json` | test revision, role, at least one locator, supersession set | canonical/current diff URIs, resolved local URNs, acyclic supersession |
| Test run | `v5/test-run.schema.json` | test revision, parents, exact source identity, result, evidence, execution time | current source/revision/evidence, one root, acyclic graph, visible multi-head conflicts |
| Quality policy | `v5/quality-policy.schema.json` | criterion/revision, required kinds, allowed automation, rationale | resolved current criterion, acyclic supersession, one policy head |

Every axis of every accepted criterion (prototype, UX, UI, technical, quality,
and implementation) is required. A Saga with no test cases is not exempt from
quality; each criterion's quality axis is a visible gap until a test case
verifies it or a current coverage exception excuses it. Coverage-exception
records are read and evaluated; no command writes them yet.

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
one or more exact line/event diff URIs. Execution artifacts forbid diff URIs
and require a verification or citation. Every evidence record contains the
four explicit arrays `diffs`, `verifications`, `citations`, and `supersedes`;
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
citation. Every axis is required; an exception is the only way
to declare an axis inapplicable, and there is one unsuperseded head per
criterion/axis. The deprecated pre-six-axis value `design` still loads and
expands to `ux`, `ui`, and `technical`; writers must not emit it. There is no
exception from exact changed-source accounting: documentation-only work still
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
and `conflicted`. Readiness reports independent `requirements_ready`,
`product_ready`, `design_ready`, `implementation_trace_ready`, `quality_ready`,
`ready_for_review`, and `review_complete` gates with their required facts,
explicitly not-inferred judgments, and blocker paths. `design_ready` is
per-axis: it requires current `ux`, `ui`, and `technical` coverage or a current
exception on each applicable axis. Every criterion/axis cell is exactly one of a
current link, an explicit exclusion, or a visible gap.

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
  overview.fragment/
  ___requirements/
  ___design/
  ___workplan/
  ___quality/
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
criterion through its linked review targets to Item-owned diff URIs. It can be
filtered in reverse with `--diff` or with `--commit`, where the commit is the
resolved source-head commit of the active committed comparison. Commit lookup
is unavailable for `WORKTREE` comparisons because their exact diff may include
uncommitted content. Its `unlinked_code_evidence` collection exposes Item
evidence that has no active, current path to an accepted story. Thus a caller
can traverse from a story to code, or from the current head commit or diff back
to the story, without duplicating story text inside slide records.

## Report content, evidence, and review

The report carries the Saga's authored narrative, and the same component model
carries diff evidence, claims, and the review overlay across every part of the
Saga. Report records use the v2 component schemas.

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
    "repository": "https://github.com/acme/payments.git",
    "base": "main",
    "head": "HEAD"
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
userinfo is invalid rather than silently stripped on read. An optional `pr`
records a positive `number`, an absolute `url`, or both.

`base` and `head` select the comparison to evaluate. Commit comparisons resolve
both revisions and record their actual Git merge base. `WORKTREE` is allowed as `head`; engines resolve the merge base of
the configured base and current `HEAD`, then compare that tree to the tracked
worktree. Untracked files are excluded.

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
│       └── ___diffs/
└── ___diffs/
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
does not itself create coverage; the landmark's independent `___diffs` records
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
page anchor and assign the landmark its own target URN. Each `___diffs/*.json`
inside the landmark package associates fully qualified diff atoms with that
exact narrative element; each association remains an independent file.
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

## 5. Absolute diff URIs

A diff link is a fully realized URI. It does not inherit repository or revision
state from the directory containing it.

Line range:

```text
saga-diff://v1/line
  ?repository=https%3A%2F%2Fgithub.com%2Facme%2Fpayments.git
  &base=6c5f...40-hex-oid
  &head=b127...40-hex-oid
  &path=internal%2Fcheckout%2Fhandler.go
  &side=new
  &start=18
  &end=42
```

File event:

```text
saga-diff://v1/event
  ?repository=https%3A%2F%2Fgithub.com%2Facme%2Fpayments.git
  &base=<identity>&head=<identity>
  &event=rename
  &old_path=internal%2Fold.go
  &new_path=internal%2Fnew.go
```

File review target:

```text
saga-diff://v1/file
  ?repository=https%3A%2F%2Fgithub.com%2Facme%2Fpayments.git
  &base=<identity>&head=<identity>
  &path=internal%2Fcheckout%2Fhandler.go
```

Required common parameters are `repository`, `base`, and `head`. Line URIs also
require `path`, `side` (`old` or `new`), `start`, and `end`. Event URIs use
`event=add|delete|type-change|rename|mode|binary|modify`. Rename carries exactly
`old_path` and `new_path`; every other event carries exactly `path`. `add` and
`delete` record file lifecycle independently of any changed-line atoms, so
empty-file changes remain coverable. `type-change` records transitions among
regular files, symlinks, and Gitlinks; `mode` records permission-only changes;
`binary` records content Git cannot express as lines. `modify` is the defensive
event for any other Git-reported file record that yields no more specific atom.
File URIs identify the complete changed file for review-progress events; they
are not valid coverage links.

The query parameter set is closed and every parameter occurs exactly once.
Canonical builders sort and escape the query and canonicalize the embedded
repository identity. Parsers reject duplicate or unknown parameters and reject
alternate encodings, ordering, userinfo, fragments, or other noncanonical
spellings rather than assigning them an ambiguous meaning.

The base identity is the comparison's resolved merge-base commit OID. The head identity is
`product-<sha256-of-binary-patch>` where the patch excludes paths beneath any
`.saga` directory. Product edits therefore make links stale, while committing
comments, replies, approvals, or other saga-only changes does not invalidate
otherwise identical evidence. `HEAD` and `WORKTREE` produce the same identity
when their tracked product changes are identical. Engines compare the complete
URI identity, preventing evidence from silently matching a similar path in a
different repository or product comparison.

## 6. Attaching diffs

The root, a section, or a fragment can contain `___diffs/*.json` conforming to
[`schema/v2/diff.schema.json`](schema/v2/diff.schema.json):

```json
{
  "version": 2,
  "diffs": [
    {
      "uri": "saga-diff://v1/line?repository=...&base=...&head=...&path=internal%2Fapi.go&side=new&start=18&end=42",
      "note": "The behavior demonstrated by this fragment"
    }
  ]
}
```

The containing object is the target of the evidence. One link may select a line
range; one evidence file may hold multiple links and must hold at least one. All
atoms are mapped when every changed line and file event is selected and every
committed link matches the current source comparison. This is an omission
invariant only; it does not establish explanation quality, claim truth, review
completion, or correctness. Overlap is reported but permitted.

CLI-generated evidence filenames are a deterministic function of the target's
canonical selector set and exclude reviewer-facing notes. Unrelated selector
sets therefore write unrelated files, while two branches that explain the same
selectors differently write the same path and require an explicit Git
reconciliation. A generated filename collision MUST NOT be resolved by adding a
timestamp or numeric suffix: preserving both records would turn an authored
disagreement into an accidental overlap. Explicit `--name` values remain stable
author-chosen repair handles.

Every Git-reported product file has at least one event or line atom, so an
unrepresentable file record cannot make a nonempty comparison appear complete.

Changes under a `.saga` path in the source repository are classified separately
as saga-only changes. When source and saga are different repositories, all saga
history is naturally outside the source comparison.

### 6.1 Diff-based maintenance impact

`change-saga compare` is a read-only evidence projection for maintaining a Saga
as its source evolves. It MUST NOT compare fragment bytes, rendered prose,
diagram geometry, review comments, or other authored content.

The first Saga is the maintained document. An incoming Git comparison may be
provided directly or through another Saga's `source` declaration. The engine
reconstructs the maintained Saga's comparison at the incoming comparison's
resolved base and evaluates its committed evidence there. It then classifies
incoming source atoms as:

- `conflicting_intersection` when a removed line or destructive file event
  intersects source evidence already owned by a Saga target;
- `additive_near_owned_code` when a new line belongs to the same replacement
  block or is immediately adjacent to owned baseline code; or
- `new_content_required` when no existing evidence owner can be derived.

The result identifies stable target URNs, target kinds, content locations,
evidence files, and exact incoming atoms. A direct intersection means the
target MUST be revisited. An adjacent addition is a prompt for consideration,
not proof that narrative content is stale. An ownerless change requires a new
or newly expanded narrative target. If the maintained Saga is incomplete or
stale at the incoming base, the result carries a `baseline_incomplete`
diagnostic and the command exits 3; an implementation MUST NOT claim exhaustive
impact in that state.

This projection does not rewrite evidence or advance the Saga's declared head.
Its purpose is to produce the formal maintenance work queue before content and
coverage are reconciled against the new source state.

### 6.2 Exact evidence rebase

`change-saga rebase-evidence` is the only supported bulk identity migration for
a moved declared base. It resolves the Saga's current source comparison and
MUST prove that every candidate selector already carries the same
base-independent product identity as that comparison. It MUST also rebuild each
candidate using only the new resolved base and prove the result still selects a
current atom. A different repository, product identity, selector shape, or
unmatched translated selector MUST refuse the entire operation without writes.
Multiple old base cohorts MUST be refused rather than partially migrated.

The operation preserves evidence record paths, targets, ordering, notes, paths,
sides, ranges, and events. Only the exact base field in each canonical URI may
change. `--dry-run` performs the same proof and reports the complete impact
without mutation. Application is serialized under the Saga authoring lock; a
failed multi-file write restores original evidence and removes newly appended
records.

Claims remain immutable. Every claim containing migrated evidence is copied to
a new claim ID with translated evidence, and a v3 `supersedes` relation points
from the replacement to the original. Verification is not inherited by default. With
`--carry-verifications`, the latest result is appended for the replacement as a
new `analysis` verification whose summary identifies the source verification
and unchanged product identity; this records logical carry-forward and MUST NOT
imply that a command or test was rerun.

## 7. Claims and verification

Author claims are independent `___claims/<id>.json` records conforming to
[`schema/v2/claim.schema.json`](schema/v2/claim.schema.json). A claim contains a
falsifiable `statement`, a `kind`, one existing narrative `target`, at least one
exact line/event diff URI in `evidence`, and `created_at`. Claim evidence never
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

## 8. Review overlay

Review data lives separately from authored content:

```text
___review/threads/<thread-id>.thread/
├── thread.json
├── events/
│   └── <event-id>.json
└── messages/
    └── <message-id>.message/
        ├── message.json
        ├── body.fragment/
        │   ├── fragment.json
        │   └── content.md
        └── screenshot.fragment/
            ├── fragment.json
            └── screenshot.png
```

The overlay also contains append-only file-review events:

```text
___review/diffs/<event-id>-reviewed.json
```

Each event conforms to
[`schema/v2/diff-review.schema.json`](schema/v2/diff-review.schema.json), uses a
fully realized `/file` diff URI, and has state `reviewed` or `unreviewed`. The
latest event for the URI is current; when two events share a `created_at`, the
greater event `id` is the later one. Ordering never depends on file names, so
every engine resolves the same commit to the same state. A new comparison identity therefore starts
with no files implicitly reviewed.

A thread conforms to [`schema/v2/thread.schema.json`](schema/v2/thread.schema.json)
and targets a saga, chapter, section, or fragment URN. Its anchor is one of:

- `target`: the entire target.
- `region`: rectangles, ellipses, or lines in normalized coordinates.
- `drawing`: arbitrary paths represented by normalized points.
- `text`: an exact quote with optional prefix, suffix, and character positions.
- `note`: a sticky note carrying its own visible text and normalized placement.
- `diff`: one fully realized line or file-event diff URI.

Normalized coordinates are in `[0,1]` relative to the rendered fragment stage,
so drawings survive responsive resizing. Shapes support presentation hints such
as color and stroke width, and text selectors may carry a highlight color.
Engines may apply accessible defaults.

A `note` anchor is a first-class review entity rather than a new thread kind: it
is an ordinary `comment` thread whose anchor holds `text`, the normalized `x`/`y`
centre of the note on the fragment stage, and an optional `color`. Note text is
limited to 2000 characters and carries no markup; engines must render it as
plain text. Because the note lives in the anchor, moving, rewording, and
recoloring a committed note are `anchor` events rather than message rewrites,
and the note keeps the thread's replies, state, and permalink. Engines should
give each committed note its own document anchor so a sticky is directly
linkable.

Text selectors follow the resilient idea from Web Annotation selectors: `exact`
is authoritative, while positions and surrounding text help engines re-anchor
after modest content edits. An engine must report an unanchored selector rather
than attaching it to different text silently.

## 9. Thread messages and state

Each message has `message.json` and one or more fragment packages. Comments are
therefore not limited to plain text: a reply may contain Markdown, images, SVG,
or sandboxed interactive HTML using the same fragment model as the saga.

Thread lifecycle changes are append-only files in `events/` with state `open`,
`resolved`, or `withdrawn`. The latest event by `created_at` is current, with
ties broken by the greater event `id`. A
withdrawn thread is omitted from the active review surface but remains fully
auditable; a later `open` event restores it. An event may instead carry an
`anchor` replacement to move, recolor, reword, or otherwise edit committed
annotation geometry and note content. Transient undo and redo are engine-local
composition behavior and do not create files. Messages, thread roots, and thread events are history and
should not be rewritten or deleted during ordinary review.

### Append-only file granularity

Review mutations never append to a shared JSON array or rewrite a neighboring
reviewer's record:

- Each top-level comment creates its own `<id>.thread/` directory, whose name is
  exactly the `id` in its `thread.json`.
- The initial comment and every reply create separate `<id>.message/`
  directories, with independent `message.json` metadata and fragment files, and
  the directory name is exactly the `id` in its `message.json`. A record whose
  directory and identifier disagree is invalid: the two would otherwise address
  different records.
- Every resolve/reopen/remove/restore, anchor edit, approval/rejection, and
  reviewed/unreviewed change is a new event file with a unique time-plus-random
  identifier.
- Writers use exclusive creation and must fail rather than overwrite an
  existing record.

Consequently, two people commenting on the same fragment or replying to the
same thread normally add disjoint files and do not create a Git content conflict.

A thread has kind `comment` or `suggestion`. Suggestions must use a `diff`
anchor and include explicit replacement text; they remain review proposals and
are never applied to source code automatically.

Saga-, chapter-, section-, and fragment-level decision records remain separate
from discussion threads. Chapter decision records are accepted, but the
reviewer UI treats a chapter as
a container and derives its progress from the independently reviewed sections
and fragments inside it.
Append-only approval events live in the target's `___approvals/` directory and
use state `approved`, `rejected`, `closed`, or `open` as defined by
[`schema/v2/review.schema.json`](schema/v2/review.schema.json). New events carry
a `reviewer` persona: `kind` is `human` or `ai`; AI personas also require a
distinct reviewer name, agent kind, and model. Git still supplies the
authoritative author identity. The name identifies one independent review seat
(for example `Claude 1` and `Claude 2`) even when both seats use the same model.
Legacy events without a persona remain valid and are displayed as unspecified.

The latest event for each distinct Git author and reviewer persona is that
reviewer's current decision, resolving `created_at` ties by the greater `id`.
An `open` or `closed` event retracts only that persona's active decision. The
displayed aggregate is rejected when any current reviewer rejects, approved
when at least one current reviewer approves and none reject, and otherwise
unreviewed. Every current decision remains visible; an AI approval is never
presented as a human approval. Repository policy, not this format, decides
which combination of approvals permits merging.

## 10. Reserved names

`___diffs` and `___approvals` are reserved on saga/chapter/section/fragment targets.
`___landmarks` is reserved inside fragments.
`___review`, `___claims`, `___verifications`, `___requirements`, `___design`,
`___workplan`, `___slides`, and `___quality` are reserved at the saga root;
`___slides` contains only real `<deck-id>.deck` directories.
Reserved metadata directories must be
real directories, not symlinks. So must every entity package: a `.chapter`,
`.fragment`, `.landmark`, `.thread`, or `.message` entry that is a symlink or a
regular file is invalid rather than ignored, because silently skipping it would
hide authored content behind a valid-looking saga. Other names beginning with
`___` are invalid.

## 11. CLI behavior

- `change-saga init SAGA` creates a Saga in the one format, ready for
  prototypes and stories. It never creates any other kind of Saga.
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
- `change-saga cover --target ...` attaches an absolute diff URI to any target.
- `change-saga status --json` reports the readiness gates, every accepted
  criterion's coverage on each of the six axes, the pin-derived stale set with
  pinned and current revisions, changed-source accounting (uncovered atoms with
  ready-to-use absolute URIs, orphans, and overlap), and ordered `next_actions`.
  Each action is either a command shape from the same grammar `spec --json`
  publishes, or one focused question for the author. Nothing is reduced to a
  score or percentage.
- `change-saga compare` projects a direct Git range or another Saga's source
  comparison onto a maintained Saga's evidence owners. It compares source
  diffs only and emits stable update locations plus ownerless changes.
- `change-saga query mappings --sort scrutiny` ranks broad or thin evidence
  records without claiming that a low score proves correctness. `query claims`
  and `query verifications` expose assertions, exact evidence, attribution, and
  result history.
- `change-saga thread` and `change-saga reply` edit the review overlay without modifying
  authored fragment content.
- `change-saga review` appends a saga-, chapter-, section-, or fragment-level decision.
- `change-saga open` serves a Saga view with attached-diff drawers, a Code Diff view
  with a changed-file tree, and a bidirectional Coverage Manifest. The Manifest
  must derive both code-to-narrative and narrative-to-code projections from the
  same atom assignments. Attached code is grouped by collapsed source file, and
  evidence `note` values provide the reviewer-facing what-and-why summary before
  linked ranges are expanded. Review surfaces support diff comments and
  suggestions; the full diff view also records reviewed/unreviewed file events.
- `change-saga validate` checks structure, identifiers, URIs, anchors, entrypoints, and
  review history independently from coverage completeness.

`status` exits 0 when every atom is mapped with no stale selector and 3 when
mapping gaps remain. The status is not a correctness verdict. `validate` exits
1 for structural errors. Unknown v2 JSON fields are rejected.
