# Component and System documentation

Components are meaningful, identifiable units of logic or transformation: a
Redux store, an application service, a flag evaluator. Systems explain how
those Components interact and how data flows through them. These are reusable
technical definitions, independent of vocabulary terms and feature ownership.
There is no required C4 hierarchy or separate architecture workflow.

Implementation decks remain the explanatory experience. Their contextual
nodes can link the same canonical definition, keeping their own labels and
exact evidence. A System reference never covers all of that System's code.
Review decks still explain transitions and only whole slides receive decisions.

## Authoring

Use the workspace binary's `component` and `system` commands. `add` and
`revise` accept `--id`, `--revision`, `--from FILE|-`, and `--repo`; `revise`
requires every observed revision head with repeatable `--parent`. The complete
JSON definition follows [technical-definition.schema.json](../schema/v5/technical-definition.schema.json).
For example:

```json
{
  "name": "FlagStore",
  "explanation": "Keeps flag values and returns false for an unknown flag.",
  "code": [{"commit":"HEAD", "path":"flags.go", "start":3, "end":8,
            "note":"Lookup and fallback are implemented here."}]
}
```

The CLI resolves the commit and computes the exact digest. If supplied, a
digest must match. References require inclusive line ranges and rationale;
whole-file evidence is refused. System definitions also require `components`
(their exact `{target, revision}` pins) and directed `interactions` with `id`,
`from`, `to`, `description`, and their own exact `code`. Endpoint URNs must be
listed Components. Component order determines the canonical diagram's node
order. The renderer draws directed edges from this data and provides a linear
interaction explanation alongside it.

`set-state --id ID --event ID --state active|retired --reason TEXT --parent URN`
appends lifecycle history. Definitions and lifecycle events can have competing
heads after a merge; reads show the conflict, and revision/reconciliation must
name every head. Unchanged existing pins may be retained when revising surrounding content;
their noncurrent state remains visible. An identical explicit revision/event retry is a no-op; changed
content under the same identity is refused.

`add-item` and `revise-item` accept `--documentation URN` and
`--documentation-revision URN`. Pass both; on revise, two empty values clear the
link. Transaction-managed slides use `apply-slide` with an optional
`documentation: {target, revision}` on each Item. `query slide` round-trips it.
Onboarding's `record` rules are unchanged. Documentation pins are supported on
implementation and review Items, separately from the review's affected `record`.

## Reads and currency

`query inventory --saga PATH [--kind component|system] [--target URN --history]
[--repo PATH] [--head REV] [--limit N] [--cursor TOKEN]` is a bounded,
snapshot-bound query. `query schema inventory` describes its paths. Each record
reports explicit heads, its unique current definition/lifecycle (or null),
exact code health, and System member pin health. Historical definitions remain
readable; a name is never an identity selector.

`validate` reports missing pins as errors and stale, retired, or conflicted
pins as warnings. Exact code health is reported by `query inventory`. A new
link must resolve to a current, active, unconflicted definition. Existing links
are never silently repinned. A changed definition does not rewrite a saved
slide or its review decisions. An explicit Item pin edit changes the Item/slide
content digest through the existing currency mechanism.

The Item's book control opens the saved definition in the existing drawer.
Opening a Component from a System retains a back path; Escape closes the drawer
and returns focus while leaving the slide hash and position intact. Definition
history and current-definition navigation only read other revisions. They do
not mutate the Item's pin or imply approval. Canonical code and interaction code
are shown with the existing local code renderer. No remote assets are used.

## Compatibility and migration

This opt-in v5 extension reserves `___inventory/components/<id>.component/`
and `___inventory/systems/<id>.system/`. Each package has its immutable
`component.json` or `system.json`, `revisions/<id>.json`, and `events/<id>.json`.
The optional `documentation` field extends v4 Item manifests and complete-slide
requests. Existing Saga readability is unchanged; no migration is needed for previously
valid files. Existing
terms, Items, approvals, and evidence are not converted.

Older binaries reject the new reserved root and/or unknown Item field rather
than interpreting it as coverage. Upgrade readers before adopting inventory;
there is no automatic downgrade or destructive migration. Revisions preserve
code commits, byte digests, explanatory notes, parent pins, and lifecycle reasons.

## First-pass boundaries

Inventory is loaded separately from the requirements graph, only for explicit
inventory reads, mutations, validation, and the documentation drawer. It is
never eagerly attached to every review shell. Selected-record reads currently
load the bounded inventory metadata graph, then resolve code only for the
requested page/record. A future storage index can make those reads cheaper.

Canonical System layout is generated from its Component/interaction graph;
custom canonical diagram assets and layout authoring are deferred. Contextual
implementation slides retain their existing SVG/HTML/image authoring.
Inventory-wide reverse Item lookup and integration with comparison layers,
`references`/`repin`, reconciliation, feature audits and next-actions remain
future integration work; use `query inventory`, `query slide`, and `validate`
for the explicit contracts in this pass. Inventory code never enters diff
coverage implicitly. Prompt autocomplete, ERDs, deployment views, subsystem and
container taxonomies remain out of scope.

## Disposable preview and verification

Build the CLI, then run the public-authoring fixture generator with a fresh
temporary directory (the script refuses an existing directory):

```sh
go build -o /tmp/change-saga-inventory ./cmd/change-saga
python3 e2e/support/inventory-preview.py /tmp/change-saga-inventory /tmp/feature-flag-preview
/tmp/change-saga-inventory serve --addr 127.0.0.1:50176 --repo /tmp/feature-flag-preview/code /tmp/feature-flag-preview/demo.saga
```

Use a free port; in DevSwarm, wrap builds in `hivecontrol exec oneshot` and the
server in `hivecontrol exec service`. The generator creates its own source Git
repository and authors the Saga exclusively through the CLI. It defines
FlagClient, FlagEvaluator, and FlagStore once, pins them in FeatureFlag, and
links that System from separate checkout and search implementation decks.
The checkout and search Items own only their respective feature branch line.
Open `/features/checkout` or `/features/search`, activate the FeatureFlag node,
open a Component or interaction's exact code, and return to the original slide.

The automated inventory tests cover public authoring/query/validation,
duplicate and missing targets, immutable retries, schema round trips,
conflicting heads, retirement, exact evidence and stale code, snapshot-bound
pagination, saved pins and explicit repins, and slide transaction history.
`e2e/tests/inventory.spec.ts` exercises keyboard activation, focus return,
unchanged slide position, nested explanations, saved stale references, lazy
loading, and a 390-pixel touch viewport. The first-pass disposable preview was
also inspected in Chromium at desktop and narrow sizes, including interaction
code. `visual-qa` passed both slides at 1280×720 and 1024×576; its semantic-arrow
check is not evaluated, so arrow directions were inspected manually.

The full Chromium regression run passed 45 of 46 tests. The remaining test,
`section-directories.spec.ts` (Terms columns), expects five columns while the
unchanged source baseline already renders seven. That separate Terms contract
was left unchanged. Firefox and WebKit were not run for this pass.

`go vet ./...` passed. The full `go test -race -timeout 20m ./...` run
passed every non-server package, but timed out in the existing server dogfood
test `TestStoriesAndCriteriaShowTheirTraceability` after 20 minutes. Its stack
was in `saga.loadDeckRecords` through `LoadNarrative`; this is an incomplete
full-suite result, not a passing server regression run.
The focused race tests for inventory authoring/model behavior passed, as did
the exact documentation endpoint/control, review rendering, frozen-review,
coverage, and drawer-accessibility tests (server: 5.464 seconds). The CLI build,
TypeScript typecheck, formatting, and repository documentation-link check passed.
