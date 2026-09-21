# Bounded automation and parallel delivery {#bounded-coordination}

Automation extends an author's reach while keeping scope, causality, and
delivery legible. Reads are snapshot-bound; writes name their exact target;
parallel work is released by explicit dependencies rather than optimistic
assumptions.

## Safe delegation loop {#safe-delegation-loop}

An automated assistant first reads a bounded projection and its snapshot
identity, selects one documented next action, previews or validates the intended
mutation, writes with an idempotency key, and re-reads the affected projection.
If the snapshot changed, the assistant stops and replans. If any member of a
batch is invalid, no member is written.

The loop never executes authored content, invents a broader policy, or treats a
successful mutation as evidence that the intended outcome was delivered.

## Delivery waves {#delivery-waves}

A plan organizes mergeable work into ordered waves. Every work item has one
objective, explicit deliverables, owned paths or surfaces, and dependency
conditions. Workspace assignment names responsibility; progress reports expose
activity. Neither assignment nor progress satisfies an acceptance criterion.

A dependency releases work only when its stated contract is present in merged
history. Coordination messages may announce readiness or friction, but immutable
merge evidence records the delivered unit and outcome. Integrators validate the
combined graph after each wave so individually valid branches do not create a
stale or conflicting whole.

## Conflict and resumption behavior {#coordination-resumption}

On resumption, a contributor reads the current plan, merged evidence, dependency
state, and its assigned item before editing. Diverged snapshots, overlapping
ownership, stale requirement pins, and conflicting record heads are explicit
stop conditions. The contributor preserves valid partial work, reports the
smallest blocking fact, and resumes only from a refreshed plan.
