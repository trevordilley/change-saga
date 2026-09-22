# App Saga traceability navigation closeout

Date: 2026-09-21

## Scope and outcome

This closeout covers the reviewer path from a story and acceptance criterion to
an implementation Item and that Item's exact source. It also covers historical
requirements: a retired story and its criteria must be unmistakably historical,
and the reviewer may be sent to a current replacement only when the Saga graph
explicitly records that replacement.

The reviewer UI now:

- labels retired story cards, sidebar rows, story pages, and criterion pages as
  historical;
- states that historical content is not current product intent;
- shows the recorded retirement reason without treating prose as a link;
- lists current replacements only from active, non-stale `supersedes`
  relations whose source still resolves to a non-retired story or criterion;
- says that no current replacement is explicitly linked when the graph has no
  qualifying edge; and
- preserves the existing criterion-to-implementation navigation, including
  opening an implementation Item's exact linked-source drawer from its visual.

No requirement, relation, or coverage record in `app.saga` was changed by this
work.

## Supported-query audit

A repository-local CLI was built and used for every Saga read:

```text
hivecontrol exec oneshot 2m -- go build -o /tmp/change-saga-nav-closeout ./cmd/change-saga
/tmp/change-saga-nav-closeout query requirements --saga app.saga --state retired --limit 50
/tmp/change-saga-nav-closeout query relations --saga app.saga --type supersedes --limit 100
/tmp/change-saga-nav-closeout query requirement-history --saga app.saga --requirement check-covers --limit 100
/tmp/change-saga-nav-closeout query traceability --saga app.saga --requirement evidence-traversal --criterion criterion-code --limit 100
```

The audit found 11 retired stories and no active `supersedes` relation in the
current app Saga. Several lifecycle reasons describe where an obligation moved,
but those sentences are historical explanations, not machine-verifiable graph
edges. The UI therefore shows a truthful no-successor state for those stories
and criteria rather than guessing a link from prose.

The active `evidence-traversal / criterion-code` criterion has two current
implementation Items and seven exact code-evidence locations. Its traceability
response contains explicit criterion → Item → code paths, so the navigation
test exercises the same supported graph shape as the repository's real Saga.

## Verification

Focused server tests cover both sides of the historical contract: explicit
story/criterion successors render as links, while a lifecycle reason that merely
names another story produces the no-successor state. The browser test authors a
current story, criterion, implementation slide and Item, relation, and code
reference through the CLI, then follows the links and opens the linked-source
drawer.

```text
hivecontrol exec oneshot 3m -- go test ./internal/server -run 'TestRetiredRequirement|TestRequirements'
hivecontrol exec oneshot 5m -- ./node_modules/.bin/playwright test tests/requirements.spec.ts --project=chromium
```

Both commands passed; the Playwright run passed all three requirements
scenarios.

## Change Saga and test-workflow rough spots

| Intended goal | Exact command | Observed behavior | Impact | Workaround | Smallest product improvement |
| --- | --- | --- | --- | --- | --- |
| Find the current successor of a retired requirement through the supported read API | `/tmp/change-saga-nav-closeout query requirement-history --saga app.saga --requirement check-covers --limit 100` | The response completely describes revision and lifecycle history, including the retirement reason, but does not include active inbound or outbound `supersedes` relations. | A consumer cannot distinguish an explicitly linked successor from a name mentioned only in prose with the history response alone. | Queried `relations --type supersedes` separately and joined on stable requirement URNs. | Add an optional relation projection to `requirement-history`, or return explicit predecessor/successor relation URNs in its envelope without interpreting lifecycle prose. |
| Audit all retired requirements and their explicit replacements in one bounded read | `/tmp/change-saga-nav-closeout query requirements --saga app.saga --state retired --limit 50` | The state filter and current heads are precise, but replacement edges require a second paginated relation query. | Automation must preserve and compare snapshots across two result sets before presenting historical navigation. | Read both queries at the same unchanged snapshot and let the server graph join exact endpoints. | Add `--include-relations supersedes` or a dedicated historical-requirements projection while keeping the base requirements response bounded. |
| Run the focused browser suite through the tracked process wrapper | `hivecontrol exec oneshot 5m -- npm test -- --project=chromium tests/requirements.spec.ts` | The npm launcher failed immediately with `npm error weird error BuildMessage {}`. | The normal package script could not start the test despite dependencies already being installed. | Invoked `./node_modules/.bin/playwright` through the same tracked one-shot wrapper; all tests passed. | Preserve the selected package-manager launcher in `hivecontrol exec`, or report when it substitutes another runtime and show the exact executable to call. |
| Run the full server package as a final regression check | `hivecontrol exec oneshot 5m -- go test ./internal/server` | The package emitted no result before the five-minute one-shot timeout; the wrapper stopped the process cleanly. | A package-wide pass cannot currently provide timely feedback for this small reviewer-UI change. | Ran the focused requirements/server tests and the complete requirements Playwright file, both of which passed. | Split or label the long-running server tests so ordinary package runs finish inside the advertised bound, or print which test is still running. |
