# Technical inventory: view and implementation policy

Status: implementation contract for the first policy slice. These decisions
resolve the three behavioral gaps found by the
[cold handoff](technical-inventory-handoff-check.md). They do not claim that
the public CLI, persisted format or reviewer already implements them.
The [design deck](technical-inventory-design.md) remains the visual explanation.

## 1. Explicit historical views, never implicit repinning

Default new-link authoring retains today's rule: the exact revision must be the
unique current revision of an active, unconflicted entity. An omitted view is
not permission to select history.

A new historical link is permitted only with an explicitly selected immutable
design/review view. That view must identify its saved Saga snapshot or committed
artifact, source commit, and the exact target/revision binding. A caller cannot
manufacture an exception by supplying an arbitrary old revision plus a label.
Resolve and validate the view before invoking the policy:

- The view contains the exact requested binding and its referenced definition.
- The binding's definition and lifecycle were unambiguous and active in that
  saved view. Missing views, missing revisions, mismatched pins or unresolved
  view conflicts refuse new-link authoring, with a specific reason.
- Global retirement, a newer proposal or competing global heads do not rewrite
  a previously valid saved view. Return those global health facts separately;
  the historical exception never labels the link globally current.
- Merely reading an existing historical pin remains possible even when a new
  link would be refused. Readability, authoring admission and coverage are three
  separate questions.

Coverage consumers may use a view-admitted historical pin only in that same
explicit report view, after checking the selected implemented revision/path
and exact selected code against the report's named source side. A baseline pin
does not cover HEAD by virtue of being readable. A proposal can reuse an
implemented baseline pin; the proposed successor itself is not silently treated
as implemented. Report global pin health and view validity separately.

No pin admission function grants coverage. The later selection resolver must
check every path hop, subset, code digest and report scope. This preserves the
existing `DocumentationLink` rule that a link transfers no code ownership.

## 2. Implementation is asserted at a delivery commit

Every operation newly asserting implemented intent names a source repository
and a full immutable delivery commit OID. Resolve symbolic `HEAD` once at the
public command boundary and preserve that OID throughout validation/publication.
Require the declared repository to match the Saga's canonical source identity;
never interpret an arbitrary local checkout as the same source.

For the candidate entity's own evidence and every candidate relationship marked
implemented:

1. Require at least one narrow, meaningful code reference, within the existing
   64-reference limit per owner. Validate location syntax and digest shape.
2. Verify the original commit/range and supplied digest against source bytes.
   Finding identical content elsewhere is not a substitute for verifying the
   supplied original evidence during a new implementation assertion.
3. Resolve each verified reference against the named delivery OID. Pure line
   movement is valid; changed/deleted bytes or unavailable source refuse the
   transition. Old code that still resolves currently at delivery is valid;
   code need not have been introduced by this change.
4. If any required evidence fails, publish no revision or intent changes.
   Preserve the original reference identity/digest and record the delivery OID;
   do not automatically repin the evidence to conceal its history.

The immutable revision is the implementation assertion and records the delivery
OID, not a mutable global application source setting. Later source drift reports
unhealthy evidence without demoting that revision to proposed. No assertion
implies successful tests, merge/integration, review approval or resolved feedback.

The existing `Resolver.Author` supplies original-byte verification and
`Resolver.Resolve` supplies delivery-view currency. Neither alone establishes
the entire rule. A public author must also preserve the existing lock, all-head
parent check and idempotent revision-ID rules when it eventually publishes.

## 3. Entity and relationship intent do not cascade

The candidate entity and every outgoing relationship have their own explicit
`proposed` or `implemented` intent. Legacy omitted intent reads as `unspecified`
until a deliberate new revision supplies an assessment. It is not inferred from
code, active lifecycle, a name or the age of the identity.

- Implementing an entity validates its own evidence and every relationship
  explicitly marked implemented in the candidate revision. Retained implemented
  relationships are rechecked at that revision's delivery commit too.
- A relationship left proposed stays proposed, including when both endpoint
  entities exist. It may have no code and earns no implemented-relationship
  coverage. Return its identity in the remaining-work projection.
- An implemented relationship requires both endpoint revisions to be explicitly
  implemented, plus its own current evidence. For an outgoing self/source
  endpoint, evaluate the candidate owner's post-transition intent; resolve the
  destination's exact saved revision. Do not substitute a destination's newer
  revision or infer implemented intent for an unspecified legacy endpoint.
- A proposed entity cannot contain a relationship asserted implemented from
  that proposed owner. Existing relationships can instead be read through its
  separately retained implemented baseline, without copying approval/state.
- Promoting an entity or edge never promotes another entity/edge. All changes
  are explicit in the candidate revision, retain IDs and preserve prior history.

Example: `PDFReport:r3` can be implemented with valid delivery evidence while
its outgoing `produced-from-job` relationship remains proposed. That result says
“entity implemented; one proposed relationship,” not “entire design complete.”
The illustrative names are not records to insert into Change Saga's own inventory.

## Shared policy interface and first implementation assignment

Implement a small internal policy boundary first, without public CLI wiring or
new persisted fields. This lets the rules be exercised before a format decision
is deployed. The worker owns new policy source/tests only; no sibling changes
to model, public CLI/schema, resolver, coverage or renderer are authorized by
this slice. Maintain relevant `app.saga` explanation through public authoring
commands as implementation evidence becomes available.

Inputs and outputs (semantic contract, not a public JSON schema):

- **Pin request:** exact requested identity/revision, current global head/lifecycle
  facts and optional *already resolved* saved-view binding/facts. Return admitted
  or refused with a stable reason, admission basis (`current` or `saved_view`),
  and distinct global health. No automatic fallback and no coverage Boolean.
- **Implementation candidate:** explicit intent, delivery OID, own evidence,
  relationships with stable IDs, intent and resolved endpoint intents, plus an
  evidence resolver capable of original-byte verification and view resolution.
  Return deterministic owner-scoped diagnostics and proposed relationship IDs.
  Never modify the candidate, source references or any Saga files.
- Stable refusal reasons distinguish missing/mismatched view or binding,
  ambiguous/inactive view, noncurrent default pin, invalid/absent delivery,
  missing evidence, invalid original evidence, stale delivery evidence and
  non-implemented relationship endpoints. Unknown facts fail closed.

Use existing reference validation and resolution rather than copying Git/digest
logic. Bound relationships to 64 and references to 64 per entity/edge for this
slice; reject duplicates/empty IDs, whole-file ranges and blank evidence notes.
Preserve input order in diagnostics and proposed-edge output. The first slice
must not expose a new command, write a record, relax a current guard or claim
the full feature is implemented. A trusted-view projection is a precondition,
not a replacement for the later snapshot-bound public reader.

### Acceptance matrix

| Case | Required result |
| --- | --- |
| Current, active, unconflicted exact pin; no view | Admit as current |
| Old pin without view, or view names another pin | Refuse new link; existing history remains readable |
| Exact old pin in valid saved view; global newer/conflicted/retired | Admit as saved-view only; preserve global health |
| Missing view/definition, ambiguous or inactive view | Refuse; no fallback to a newer revision |
| Implemented candidate with original evidence current at delivery | Valid candidate; no writes or approval |
| Original reference invalid but delivery has matching bytes | Refuse original-evidence verification |
| Original valid; delivery changes or deletes selected bytes | Refuse implementation assertion |
| Pure line movement before delivery | Accept; keep original reference unchanged |
| Implemented owner with a proposed relationship lacking code | Accept owner if own evidence is valid; return proposed edge |
| Implemented edge missing evidence or pointing at proposed/unspecified endpoint | Refuse; no cascading promotion |
| Retained implemented edge stale at new delivery OID | Refuse new implemented revision; preserve old revision |
| Duplicate edge ID, unbounded inputs or missing intent | Refuse deterministically |

Test the policy matrix and an actual temporary Git repository through the
existing resolver, including original verification versus delivery drift and
pure movement. The test fixture must not mutate `app.saga`. Verify inputs remain
unchanged and failed results cause no writes. Persistence retry/parent races,
source-identity checking at a public command, saved-view loading and real
coverage integration remain separate consumer tests, not claimed by pure policy.

## Compatibility boundary after the policy slice

No root or record format change is needed to implement and test the internal
policy. Before a public writer can use it, a separate coordinated format slice
must deliver the exact record/selection/view encodings and read compatibility
together with SPEC, schemas, `spec --json`, public command tests and the Format
release note. It must preserve original v5 histories and older review records,
fail safely in unsupported older readers, and require deliberate opt-in before
writing new structures. No automatic migration of `app.saga` is authorized by
this policy implementation assignment.

That remaining work is encoding/integration, not permission for downstream
workers to reopen or reinterpret the three behavioral decisions above.
