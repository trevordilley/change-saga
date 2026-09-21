# App Saga requirements audit

Audited the product contract, CLI surface, reviewer behavior, tests, and the complete current requirement graph on 2026-09-20.

## Findings and changes

- The three active personas are valid human beneficiaries but were underspecified. `change-author`, `reviewer`, and `newcomer` now state the value each person needs; the already-retired non-human `coding-agent` remains historical.
- All 18 existing accepted stories preserved their stable IDs and were revised in place as explicit persona/value statements. Their acceptance criteria were split and tightened into concise, independently testable assertions.
- Five accepted stories now cover implemented product surfaces that were missing from the requirements: `prototype-feedback`, `quality-evidence`, `parallel-delivery-plan`, `bounded-automation`, and `controlled-release`.
- Four immutable provenance citations were added for the lifecycle, work-plan, automation-interface, and product-overview contracts.
- Eleven existing design or quality relations affected by criterion rewording were read and repinned. No story relation remains stale.

No story, criterion, or active persona was retired. Validation passes with 23 accepted persona-focused stories and 132 one-line acceptance criteria.

## Out-of-scope follow-up

The requirements now expose the expected downstream work rather than fabricating it: 19 stories lack design coverage and 130 criteria lack test-case coverage. One pre-existing stale code reference remains in the `two-sides` design landmark because its source lines changed; repairing design evidence is outside this audit's ownership.
