# Coverage report contract {#coverage-report-contract}

`change-saga status` has no verdict. It reports every gap as a finding and
exits 0 whenever its report can be trusted; it exits non-zero only when it
cannot produce one (a malformed Saga such as a duplicate ID, unreadable
records, or a checkout that does not match the declared repository). A team
decides which gaps fail its build, and writes that rule in its own CI.

## Areas {#areas}

| Area | Covered when |
| --- | --- |
| `implementation` | every changed line is referenced by the implementation deck |
| `stories` | every changed line reaches a story through the chain |
| `personas` | every changed line reaches a persona |
| `design` | every story in scope has design |
| `quality` | every acceptance criterion in scope has a test |
| `health` | nothing that already existed went stale or broke |

With `--against`, the scope is the change: what it changed and what it
affected. Without it, the scope is the whole app. `--feature` narrows either.

## JSON shape {#json-shape}

`status --json` carries `.coverage`, the contract rules are written against.
Every area has `total`, `covered`, `uncovered`, `complete`, and the lists
`covered_entries` and `uncovered_entries`; the counts are the sums of the
entries' `count`. `unit` says what is counted: `changed_line`, `code_target`
(the line areas when observing, which has no change), `story`, `criterion`,
or `record`. An uncovered entry has a `reason`; a covered entry names what
covers it in `via`. There is never one blended score.

## Asking a question {#check}

`change-saga check --covers AREA[,AREA...]` exits 0 when every named area is
fully covered in scope, 3 when one has a gap (printing only the named areas'
gaps), and 1 when the report cannot be trusted. Nothing is required unless it
is named.

## Next actions {#next-actions}

`next_actions` are ordered: health first (conflicts, invalid and stale
records, failed runs), then `changed_source`, then reviews, then `growth`.
Each action names the `area` it advances and is either a command shape or one
focused question; a growth action also carries the `practice` it teaches.

Source: the change-saga skill's references/ci.md and SKILL.md, and
docs/app-saga.md goal 9.
