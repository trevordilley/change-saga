# CLI ergonomics: record access and deliberate edits

Status: implementation plan, not a description of shipped commands. Proposed
syntax below must be reconciled with the existing query registry before release.

## Decision and product boundary

Keep Change Saga's native engine, storage format, and opinionated model. Do not
adopt Doorstop as a datastore or introduce a general-purpose knowledge graph.
Saga organizes requirements, technical designs, diagram elements, exact code
evidence, and collaborative review into versionable, merge-friendly documents.
The CLI should make that workflow easier, not become an arbitrary graph editor.

Authoritative records remain in the repository. Preserve immutable revisions,
stable identities, explicit relationships, conflict visibility, and Windows
support. Any derived index remains disposable rather than an authoritative
database that collaborators must merge.

The immediate priority is better querying. Reduce discovery and bookkeeping
without asking the tool to infer intent from names or prose.

## Motivating workflow

The Product Manager to Product Owner rename preserved the existing persona URN:

```text
urn:change-saga:change-saga:persona:product-manager
```

That was correct: a display-name change must not break stable identity. The
reported friction was obtaining the persona's current definition and revision
parents, finding its eight referencing stories, selecting revision IDs, and
checking downstream effects. The role name also appeared independently in story
statements, overview prose, onboarding metadata, and SVG text. One relationship
needed an explicit repin. A globally installed CLI could not read the Saga version.

Query improvements address discovery and safe editing. They do **not** make all
those independent text copies live bindings, nor prove that every matching
phrase should change. The new workflow must say what it knows and what it has
not inspected.

## Lessons from Doorstop, without adopting it

- Small, recognizable operations and automatic item numbering reduce ordinary
  authoring ceremony. Borrow the principle, not its identity or storage scheme.
  See [creation commands](https://doorstop.readthedocs.io/en/latest/cli/creation.html).
- Changed evidence and broken references are different problems. Doorstop's
  suspect-link validation provides a useful precedent for asking someone to
  renew confidence rather than declaring every changed dependency invalid.
  See [validation](https://doorstop.readthedocs.io/en/latest/cli/validation.html).
- Review fingerprints distinguish selected meaningful fields from presentation
  fields. This is a lesson in explicit policy, not permission to guess semantic
  equivalence. See [item format](https://doorstop.readthedocs.io/en/latest/reference/item.html).
- Direct lookup and reverse-reference access are useful library primitives.
  Doorstop exposes item lookup and parent/child access in its
  [tree](https://github.com/doorstop-dev/doorstop/blob/develop/doorstop/core/tree.py)
  and [item](https://github.com/doorstop-dev/doorstop/blob/develop/doorstop/core/item.py)
  APIs; these should not be mistaken for a richer general query CLI.

Saga already has advantages for AI consumers: structured query results and
errors, schema discovery, snapshot-bound pagination, exact code traceability,
and guarded authoring transactions. Build on these rather than replacing them
with a human-oriented CLI. The comparison was source/documentation inspection,
not a runtime or performance benchmark; Doorstop's `develop` sources can change.

## Existing foundations and actual gaps

- `query requirements` supports exact selection, filtering, and pagination.
- `query context` provides bounded feature context and explicit expansion.
- `query terms` exists, but its advertised pagination contract is currently
  `none`. There is no corresponding direct persona query.
- Query responses use `change-saga.ai/v1`; errors already include machine-readable
  codes, details, and retryability. Extend this contract rather than inventing a
  parallel envelope.
- `persona revise` requires a complete definition, a manually supplied revision
  ID, and explicit parents. Similar bookkeeping deserves a focused term audit.
- Complete-slide authoring already uses expected snapshots, explicit heads, and
  idempotent request handling. Reuse those safety principles for small edits.
- Currency already distinguishes invalid, conflicted, stale, and current. Some
  story/criterion revisions carry forward when the compared obligation is
  unchanged. Do not claim that every new revision makes every link stale.

Starting points: `internal/cli/query.go`, `internal/livingapp/`,
`internal/cli/app_authoring.go`, `internal/requirements/app_mutation.go`,
`internal/requirements/currency.go`, and `internal/cli/slide_transaction.go`.

## Milestone 1: answer ordinary record questions directly

### Persona and term reads

Add a first-class persona query with exact ID or URN selection and a bounded list
mode. Ensure terms have equivalent usable detail and bounded enumeration.
Proposed shape:

```text
change-saga query personas --saga PATH [--persona ID|URN] [--limit N] [--cursor TOKEN]
change-saga query terms --saga PATH --term ID|URN
```

Requirements:

- A selected record exposes its stable identity, complete current name and
  description/definition, lifecycle state, revision and lifecycle heads, conflicts,
  and the snapshot needed for guarded editing. Include term-specific semantic
  fields rather than flattening a term into a generic named node.
- A conflict exposes competing heads; never choose a winner or synthesize a
  current definition. Distinguish absent records from conflicted records.
- Lists are deterministic and bounded. Separate compact summaries from full
  definitions when necessary, with discoverable exact expansion.
- Follow established snapshot/cursor binding and structured error semantics.
- Exact selectors must not fall back to fuzzy name matching. Duplicate display
  names remain valid and unambiguous when addressed by identity.
- Preserve existing `terms` consumers. Do not silently turn a previously complete
  response into a truncated response. Prefer an additive bounded mode or a
  separately documented operation if changing the existing default is breaking.

### Explicit reference inspection

Provide named persona/term reference queries rather than a graph query language.
Proposed shape:

```text
change-saga query persona-references --saga PATH --persona ID|URN [--limit N] [--cursor TOKEN]
change-saga query term-references --saga PATH --term ID|URN [--limit N] [--cursor TOKEN]
```

Use the canonical graph/projection and typed record fields, including links owned
by authoring transactions. Do not scan only standalone relation files and call
the result complete. Explicit story persona references and term applicability
must be considered alongside relation records.

Each result must make the source, target, direction, relationship kind or typed
field, owning record, and exact selector clear. Where a link has a revision pin,
return the pin, currency, and existing reason codes. Expose incoming and outgoing
references within a documented domain-specific scope; do not silently perform
unbounded transitive expansion.

Return pagination/count information with a precise definition of what is counted,
and explicit completeness metadata naming covered and excluded reference classes.
Deduplicate the same canonical link without collapsing genuinely distinct uses.

Free-form prose and embedded SVG text are not explicit references. The response
must state that they are excluded. A lexical occurrence is not proof of a semantic
dependency; heuristic mention search is deferred.

### Discovery and errors

Register every new operation in help, `spec --json`, query schema discovery,
transport-neutral APIs where applicable, and the shipped query reference.
Use the existing spelling `query schema OPERATION`. Examples should demonstrate
the complete read, inspect references, and guarded-edit workflow.

For malformed or unsupported operations, keep one structured JSON result on the
machine interface and provide an actionable correction where known. Do not emit
progress prose into JSON output. Keep existing commands and flags working;
command-family normalization is not a prerequisite for these improvements.

## Milestone 2: small edits without revision bookkeeping

Personas and terms are deliberately named domain records. Add a focused,
previewable name-only edit for these two types, sharing internal machinery where
appropriate without creating a public generic CRUD engine. Proposed shape:

```text
change-saga persona rename PATH --persona ID|URN --name "Product Owner" \
  --expected-snapshot SNAPSHOT --request-id REQUEST --dry-run --json
```

Provide the analogous term operation using its existing domain terminology.
Omitting `--dry-run` applies the same validated single-record change.

- Preserve canonical identity and all other fields. Do not change the URN's
  readable suffix, rewrite references, or create aliases implicitly.
- Resolve only a unique current revision and compatible lifecycle state. Allocate
  the revision ID internally using existing safe identity conventions. Preserve
  advanced explicit revision authoring for reconciliation and historical work.
- Require the documented expected snapshot guard, and validate it at the mutation
  boundary. A mismatch or conflict performs no writes and returns actionable
  structured details. Never silently rebase a proposed edit onto another head.
- Make the single-record operation atomic using existing store guarantees. Do not
  imply a cross-record transaction or invent a new transaction engine.
- A stable request ID must make a retry safe, including after an uncertain client
  outcome. Reusing it for a different payload must fail. Define and test retry
  ordering relative to snapshot validation using existing transaction precedent.
- Dry-run is read-only, identifies the target/current heads and exact field delta,
  describes preserved fields and excluded propagation, and identifies applicable
  dependency findings. It must not reserve IDs or mark anything reviewed.
- Return the resulting revision and snapshot plus useful next-query selectors.
  Define no-op behavior explicitly and avoid unnecessary revisions for equal names.
- The command changes a name, not eight independently authored story statements.
  Free-form text remains a separate, deliberate editing task.

If existing storage cannot safely support one of these guarantees, report the
specific constraint and finish Milestone 1 rather than weakening safety or
expanding the storage format without review.

## Currency: explain uncertainty, do not auto-confirm it

Surface existing distinctions clearly in reference results and previews:

| Situation | Meaning and expected action |
| --- | --- |
| Invalid endpoint/reference | The reference cannot resolve or is structurally invalid; repair it. |
| Conflicting heads | The system cannot select a single current meaning; reconcile explicitly. |
| Stale semantic pin | The dependency changed; inspect whether the link still expresses the intended obligation. |
| Current or carried forward | Existing comparison rules justify currency; this is not a new human approval. |

Preserve machine status values and reason codes. Never blanket-repin links,
autoapprove review, or call a changed-but-resolvable dependency broken.

Audit how persona/term name-only revisions currently affect currency and document
the result. Do not casually alter field significance: a story title can express
an obligation, and arbitrary text equivalence is not mechanically knowable. A new
display-versus-semantic field policy, if needed, is a separate explicit design
decision with compatibility tests, not a hidden consequence of adding rename.

## Compatibility and deferred work

Document how to invoke the repository-compatible CLI. Where the existing version
check can provide it, improve an unsupported-Saga error with detected/supported
versions and a concrete recovery direction. Never download, install, or execute
an alternative binary automatically. Packaging/launcher redesign is a follow-up.

Capture, but do not implement in this pass:

- Opaque canonical IDs, slug/alias migrations, or changes to existing identities.
- Structured story templates that dynamically render persona names.
- Record-bound text slots in diagrams or other rendering changes.
- Lexical mention search, automatic prose replacements, or semantic guessing.
- Multi-record rename transactions and automatic dependency confirmation.
- A new semantic fingerprint model or broad currency-policy changes.
- A generic query language, graph database, extensible ontology, or universal CRUD API.
- Wholesale command renaming, authoritative database files, or unrelated Saga edits.

## Acceptance and evidence

Use deterministic fixtures, not the live Product Owner workspace, to establish:

1. One exact persona read supplies its full definition and edit preconditions;
   one paged reference workflow finds all eight explicitly referencing stories
   without reconstructing history. Terms have equivalent domain-appropriate tests.
2. Equal names on distinct IDs do not cause ambiguous selection or rewrites.
   Missing, malformed, foreign-Saga, withdrawn, and conflicted selectors follow
   explicit documented behavior. No conflicted record fabricates a current value.
3. Reference coverage includes supported typed fields and transaction-owned links,
   returns exact provenance, avoids duplicates, and declares prose/SVG exclusions.
4. Small page limits produce deterministic bounded results, truthful counts, and
   complete traversal. Tampered/cross-operation cursors fail; changed snapshots
   cannot silently mix pages from different states.
5. Rename preserves identity, description/definition, lifecycle, and unrelated
   files. Dry-run and rejected operations leave the repository unchanged.
6. Unique-head rename allocates a revision safely. Stale snapshots, conflicting
   heads, duplicate retries, changed retry payloads, no-op edits, and concurrent
   attempts exercise the documented write guarantees.
7. Reference results distinguish invalid/conflicted/stale/current, preserve reason
   codes, and never create approvals or implicit repins.
8. Existing CLI commands and query consumers remain compatible. JSON output,
   registry/schema documentation, and Windows/path-with-spaces cases are tested.

Record actual command counts and response bytes for the motivating read/edit
workflow before and after. Do not equate bytes with measured tokens or invent a
performance improvement. Verify that bounded output does not merely hide work or
required context. Run focused tests, the appropriate wider Go suite, and relevant
documentation/schema checks through tracked execution.

## Workspace handoff

Use one Codex child workspace for this cohesive CLI/API change. Start with
Milestone 1, then implement the guarded name-only operations if existing storage
can support the specified guarantees. Keep query, mutation, and documentation
changes in reviewable commits. Update the shipped AI instructions alongside the
public contract; do not leave an implemented operation undiscoverable.

Commit in the child and report commit IDs, tests, representative output, remaining
limitations, and any overlaps before parent integration. Do not copy files into
the parent worktree, merge/push automatically, or modify the separate Product
Owner rename branch. Parent review must check semantic overlap as well as Git
conflicts. No new engine and no scope expansion hidden inside ergonomic helpers.
