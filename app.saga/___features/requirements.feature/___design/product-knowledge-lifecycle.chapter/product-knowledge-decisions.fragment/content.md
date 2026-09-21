# Lifecycle decisions and boundaries {#lifecycle-decisions}

The Saga presents one current model of the application while preserving the revisions and lifecycle events needed to explain how that model changed. A change author revises a story instead of replacing its identity, reconciles concurrent heads explicitly, and moves a story between feature domains without severing its relations.

## Design precedes implementation evidence {#design-before-implementation}

A requirement may acquire reviewable UX and technical design as soon as its intent is stable enough to discuss. An addressable design element points to the smallest criterion it actually satisfies. This relation is useful before code exists, opens in both directions, and becomes stale when the linked requirement changes. Implementation later explains how code realizes that design; it does not retroactively define the intent.

## Prototypes are discovery surfaces {#prototype-discovery}

A self-contained interaction or supported external prototype may precede or follow a story. An annotation binds one identifiable prototype state to a story or criterion revision. Changing either endpoint makes that annotation stale. A prototype without a requirement link is optional growth, not a hidden approval gate.

## Current truth and retained history {#current-and-history}

The default application view favors active, accepted knowledge. Proposed, deferred, rejected, and retired records remain available through history rather than competing with the current explanation. The same rule applies to retired feature flags: they leave the release view but keep their lifecycle record.

## Quality is evidence, not progress {#quality-not-progress}

A test case names the criteria it verifies, ordered steps, and one expected result. A run names the test revision it exercised, so later test edits can make an earlier pass stale without deleting it. Policy distinguishes required kinds that are satisfied, excluded with authority and rationale, or missing. Work progress can coordinate contributors, but it cannot satisfy this evidence axis.

## Failure and recovery contract {#failure-and-recovery}

A stale relation remains visible and stops contributing current coverage. The author reads the new endpoint, then deliberately re-pins or replaces the relationship. Concurrent requirement heads remain visible until reconciliation. These failure states prefer an explicit gap over a guessed winner, silent carry-forward, or erased history.
