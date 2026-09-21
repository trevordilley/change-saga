# Actionable coverage without a verdict {#coverage-report-contract}

Coverage helps an author decide what to improve next; it never decides whether
the change may ship. The primary view is a plain-language set of independent
areas, each showing what is covered, what is missing, and why the result can be
trusted. A gap is useful work to consider, not a failed product judgment.

## Human reading contract {#human-reading-contract}

The report leads with the current scope and six separately named areas:
implementation, stories, personas, design, quality, and health. Each area pairs
a count with concrete covered and uncovered entries. There is no blended score,
traffic-light verdict, celebratory completion state, or language that turns
optional product knowledge into mandatory debt.

Every uncovered entry identifies the affected thing, states the missing
connection in human terms, and offers either one safe next action or one focused
question. Stable ordering keeps the most consequential integrity failures first,
then unexplained changed source, unfinished review work, and optional growth.
An author can stop after understanding the report without mutating the Saga.

## Trust and policy boundary {#trust-and-policy-boundary}

Producing a trustworthy report is distinct from applying a team's policy. Gaps
are a successful report. Malformed records, ambiguous heads, or a checkout that
cannot be reconciled are report failures because the tool cannot make a reliable
claim. A separate, explicitly selected policy check answers only whether named
areas are complete and returns only those areas' gaps.

This separation keeps the product neutral while letting a team encode its own
standards. It also prevents an automated assistant from silently widening the
question it was asked.

## Deterministic automation contract {#deterministic-automation-contract}

Human-readable and structured projections describe the same scope and entries.
Structured reads use a stable envelope, bounded pages, a snapshot identity, and
cursors that fail when the underlying Saga changes. Content is data: reading a
fragment, slide, prototype, or other authored payload never executes it.

Mutations are explicit and bounded. A batch is all-or-nothing, repeated request
identities are idempotent, and errors identify the rejected operation without
leaving partial records. These mechanics exist so an author can delegate safely,
not as a substitute for the author's judgment.

## Protocol appendix {#protocol-appendix}

The supported command surface currently exposes the human report through
`status` and the selected policy question through `check --covers`. A
comparison scopes both to the same base and head; a feature selection narrows
that scope. The structured coverage shape carries, per area, totals, covered and
uncovered entries, completeness, units, reasons, and covering resources. Exact
command spelling and field names may evolve while the human, trust, and
determinism contracts above remain stable.
