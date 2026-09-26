# Documentation coverage model {#documentation-coverage-model}

Coverage evaluates a concrete Git comparison. The diff is decomposed into stable line and file-event atoms, then exact code references are resolved against those atoms. The result separately reports covered, uncovered, overlapping, and stale evidence.

Coverage is evidence for review, not a product verdict. `status` reports the facts and exits successfully when it can produce a trustworthy report; `check` applies only the explicitly selected policy areas.
