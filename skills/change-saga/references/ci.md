# Coverage rules in CI

`change-saga status` has no verdict. It reports every gap as a finding and
exits 0 whenever its report can be trusted; it exits non-zero only when it
cannot produce one (a malformed Saga such as a duplicate ID, unreadable
records, or a checkout that does not match the declared repository). A team
decides which gaps fail its build, and writes that rule in its own CI. These
are common rules; adapt them rather than adding flags.

## Ask a question: `check --covers`

`change-saga check --covers AREA[,AREA...]` exits 0 when every named area is
fully covered in scope, 3 when one has a gap (printing only the named areas'
gaps), and 1 when the report cannot be trusted. Nothing is required unless it
is named.

| Area | Covered when |
| --- | --- |
| `implementation` | every changed line is referenced by the implementation deck, or test code by its test case's evidence |
| `stories` | every changed line reaches a story through the chain |
| `personas` | every changed line reaches a persona |
| `design` | every story in scope has design |
| `quality` | every acceptance criterion in scope has a test |
| `health` | nothing that already existed went stale or broke |

With `--against`, the scope is the change: what it changed and what it
affected. Without it, the scope is the whole app. `--feature` narrows either.

```sh
# Account for changed lines in living documentation.
change-saga check --against origin/main --covers implementation app.saga

# Check current health independently from diff-side accounting.
change-saga check --covers health app.saga

# Explain baseline debt, regressions and affected records with repair paths.
change-saga reconcile --against origin/main --json app.saga

# A stricter rule for one mature feature only.
change-saga check --against origin/main --feature checkout --covers implementation,stories,design,quality app.saga
```

Comparison coverage may accept references valid at the base for deleted lines.
It must not be used as sole evidence of HEAD documentation currency. Review
deck coverage is separate again. `reconcile` reports these independent axes;
fresh pins and complete coverage do not prove semantic correctness.

## Write a rule over the JSON

`status --json` carries `.coverage`, the contract rules are written against:

```json
{
  "coverage": {
    "scope": { "kind": "change", "against": "origin/main", "head": "HEAD" },
    "areas": {
      "implementation": {
        "area": "implementation", "unit": "changed_line",
        "total": 412, "covered": 412, "uncovered": 0, "complete": true,
        "covered_entries": [ { "resource": "src/pay.go", "side": "new", "lines": "4-9,12", "count": 7, "feature": "checkout", "via": ["urn:..."] } ],
        "uncovered_entries": []
      },
      "stories": { "...": "the same shape" },
      "personas": {}, "design": {}, "quality": {}, "health": {}
    }
  }
}
```

Every area has `total`, `covered`, `uncovered`, `complete`, and the lists
`covered_entries` and `uncovered_entries`. The counts are the sums of the
entries' `count`. `unit` says what is counted: `changed_line`, `code_target`
(the line areas when observing, which has no change), `story`, `criterion`,
or `record`. An uncovered entry has a `reason`; a covered entry names what
covers it in `via`. There is never one blended score: a rule names the areas
and thresholds it cares about.

```sh
status=$(change-saga status --json --against origin/main app.saga) || exit 1

# Comparison implementation and comparison health must be complete.
# Also run the independent HEAD health/reconciliation checks above.
echo "$status" | jq -e '.coverage.areas.implementation.complete and .coverage.areas.health.complete'

# At least one story per change, without requiring every line to reach one.
echo "$status" | jq -e '.coverage.areas.stories.covered > 0'

# Every story this change touches has design, but tests are optional.
echo "$status" | jq -e '.coverage.areas.design.complete'

# No more than 20 changed lines may reach no story.
echo "$status" | jq -e '.coverage.areas.stories.uncovered <= 20'

# Name the gaps in the build log.
echo "$status" | jq -r '.coverage.areas.stories.uncovered_entries[] | "\(.resource):\(.lines // .event) \(.reason)"'
```

`status` still exits non-zero when it cannot be trusted, so `|| exit 1` keeps
a broken Saga from passing a rule silently.

## Reviews

Pull request reviews are reported slide by slide in `.reviews`, never in the
exit status: each slide lists every reviewer's latest decision with its
`state` and `currency` (`current` or `out_of_date`). A team that requires
every slide to carry a current approval writes that rule too:

```sh
echo "$status" | jq -e '[.reviews[] | select(.merged == null) | .slides[]
  | select([.decisions[] | select(.state == "approved" and .currency == "current")] | length == 0)] | length == 0'
```
