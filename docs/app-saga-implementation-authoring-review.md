# App Saga implementation authoring review

Status: canonical implementation/code-evidence authoring audit for
`agent-loop`, `reviewer-app`, and `reviews`, performed against source commit
`58541107888381e0d7f0332bee4f1ad353b0399e` on 2026-09-20.

## Scope and result

This wave changed only the three owned feature directories and this audit. It
did not edit product source code or any other feature. Every Saga mutation was
made with a freshly built repository CLI, `/tmp/change-saga-authoring`, from
`go build ./cmd/change-saga`.

The authored implementation layer contains:

- three feature implementation decks, five slides, and twenty-two semantic
  Items;
- exact source and focused-test references on every Item;
- matching exact evidence on the precise design landmarks used by the accepted
  criteria, including all three visual design fragments from the preceding
  design wave;
- current `explains` relations from implementation Items to all 51 criteria in
  the six accepted owned stories;
- implementation relations for the four reviewer-navigation criteria in the
  accepted `observe-or-compare` story; and
- an atomic repair of the stale `two-sides-sidebar.json` reference.

`query traceability` reports design, implementation Item, and code evidence for
8/8 `bounded-automation`, 7/7 `coverage-report`, 5/5 `first-change`, 7/7
`parallel-delivery-plan`, 8/8 `safe-local-review`, and 16/16
`review-is-pr-deck` criteria. The four reviewer-navigation criteria also have
current implementation Items and code evidence. Direct code attached to a
design landmark is inspected with `query mappings`, because the current
traceability response does not append that code to its criterion-to-design
path.

The strongest pins are named Go functions and focused tests: `areas.Evaluate`,
`nextaction.AuthoringLoop` and `Derive`, `requirements.MoveStory`,
`workplan.DependencySatisfied` and the event mutators, the query envelope and
cursor implementation, `server.ListenManaged`/`secureHandler`,
`reviewstore.Create`/`Decide`/`Comment`/`Freeze`, and
`reviewstate.ResolveRange`/`Build`/`Evaluate`. Template-only sandbox behavior is
pinned to the one source line that owns the iframe contract plus its focused Go
test rather than to the whole template.

## Stale-reference repair

The old sidebar reference pinned `internal/server/appnav.go` lines 101-109 at
`b851addd...`. Those bytes no longer owned the Documentation/Review split. It
was not blindly repinned. `replace-coverage` atomically replaced it with:

- `internal/server/appnav.go` lines 109-128, where `makeAppNavTree` selects
  Features or Reviews; and
- `internal/server/appnav_test.go` lines 154-197, where
  `TestEachSideListsWhatItIsAbout` proves both projections.

The resulting reference is current, and `references --stale --json app.saga`
reports zero stale references.

## Design correction and honest gaps

The safe-rendering design previously said the parent application communicates
with authored iframes through a validated message contract. No such message
channel exists. The owned fragment now says that the current reviewer has no
message channel and makes the validation rules conditional on a future one.
The six affected `addresses` relations were reread and repinned after that
correction.

No evidence is claimed for behavior that is not present:

- the reviewer does not persist read/unread state or automatically resume at
  the earliest unfinished slide; stable review routes exist, but the fuller
  session-resumption design is not implemented;
- authored iframes cannot send application messages, so there is no runtime
  message validator to reference;
- current resource evidence covers HTTP timeouts/header bounds, bounded query
  pages and fragment chunks, contained file reads, and cursor integrity; it
  does not prove every broader archive, decode, diff-expansion, and concurrency
  limit named by the design prose;
- local serving, CSP `connect-src 'none'`, and no-CDN renderer tests support the
  offline guarantee, but there is no dedicated browser test that asserts zero
  DNS/outbound requests for an entire review session; and
- this wave referenced real executable tests but did not create canonical
  quality test cases or runs, so quality gaps remain visible rather than being
  reclassified as implementation evidence.

## Change Saga CLI dogfooding

Each rough spot below records the intended goal, exact command, observed
behavior, impact, workaround, and smallest product improvement.

### F1 — ID collision is not discoverable

- Intended goal: add the local-boundary implementation slide.
- Exact command: `/tmp/change-saga-authoring add-slide --feature reviewer-app
  --deck reviewer-app-implementation --id local-review-boundary --title 'Local
  review fails closed' --intent risk --layout diagram --source
  app.saga/___features/reviewer-app.feature/___design/local-review-boundary.chapter/local-trust-map.fragment/local-trust-map.svg
  app.saga local-review-boundary`
- Observed behavior: the command returned `slide id "local-review-boundary" is
  invalid or already used`, without naming the colliding chapter target.
- Impact: a globally reserved ID looks like a syntax error and requires a
  separate repository/query search.
- Workaround: used the unique ID `safe-local-boundary-implementation`.
- Smallest improvement: distinguish invalid syntax from collision and return
  the existing target URN and path.

### F2 — A new slide cannot read its initial asset from stdin

- Intended goal: create a generated coverage-contract SVG without a temporary
  repository file.
- Exact command: `/tmp/change-saga-authoring add-slide --feature agent-loop
  --deck agent-loop-implementation --id coverage-and-policy --intent explain
  --layout diagram app.saga coverage-and-policy`, followed by
  `/tmp/change-saga-authoring set-slide-content --feature agent-loop --target
  coverage-and-policy --source - app.saga`.
- Observed behavior: `add-slide --source -` treats `-` as a filesystem path;
  stdin is supported only by `set-slide-content`.
- Impact: one conceptual create is two mutations and can leave a valid but
  placeholder slide if interrupted.
- Workaround: created the default SVG slide and immediately replaced its
  content from stdin.
- Smallest improvement: let `add-slide --source -` atomically read stdin.

### F3 — Implementation Items cannot structurally name their design landmark

- Intended goal: link an implementation Item directly to the precise design
  landmark it implements.
- Exact command: `/tmp/change-saga-authoring add-item --feature agent-loop
  --slide coverage-and-policy --id six-areas --kind region --element-id
  six-areas --label 'Six independent areas' --description '...' --record
  urn:change-saga:app:fragment:coverage-report-contract:landmark:human-reading-contract
  app.saga`.
- Observed behavior: the command rejected the mutation with `--record is for
  onboarding items; implementation items explain code`.
- Impact: the data model can express Item-to-criterion and
  design-to-criterion, but not Item-to-design; a single structured
  requirement -> design -> implementation path cannot be persisted.
- Workaround: the Item description names the landmark, both targets carry the
  same exact code/test evidence, and the Item has precise `explains` relations
  to criteria.
- Smallest improvement: add a validated `design` link on implementation Items,
  or permit a report-design target in `record` without treating it as product
  navigation.

### F4 — Revision commands have inconsistent feature disambiguation

- Intended goal: reorder two slides in the reviewer-app implementation deck.
- Exact command: `/tmp/change-saga-authoring revise-slide --feature
  reviewer-app --slide two-lasting-sides --rank 20 app.saga`.
- Observed behavior: `revise-slide` rejected `--feature` even though
  `add-slide`, `add-item`, and `set-slide-content` accept it.
- Impact: an author cannot use one consistent disambiguation rule across slide
  mutations.
- Workaround: retried with the globally unique slide ID and omitted
  `--feature`.
- Smallest improvement: accept `--feature` consistently on revise/remove slide
  and Item commands, or explicitly state that IDs are globally unique in their
  help.

### F5 — Stale-reference replacement expects an undocumented path base

- Intended goal: atomically replace the one known stale evidence file.
- Exact command: `/tmp/change-saga-authoring replace-coverage --record
  app.saga/___features/reviewer-app.feature/___design/two-sides-design.chapter/documentation-and-review.fragment/___landmarks/documentation-side.landmark/___code/two-sides-sidebar.json
  --target urn:change-saga:app:fragment:two-sides-overview:landmark:documentation-side
  --name two-sides-sidebar --ref
  58541107888381e0d7f0332bee4f1ad353b0399e:internal/server/appnav.go#L109-L128
  --dry-run --json app.saga`.
- Observed behavior: it said the record did not exist and suggested `query
  mappings`; the flag requires a path relative to the Saga root, unlike the
  ordinary shell path an author naturally has.
- Impact: repair discovery adds a query round trip and the error does not show
  the normalized candidate.
- Workaround: removed the leading `app.saga/`, dry-ran successfully, then ran
  the same replacement without `--dry-run`.
- Smallest improvement: accept both Saga-root-relative and passed-Saga-prefixed
  paths, or print the expected normalized relative path.

### F6 — Relations have no atomic batch interface

- Intended goal: add criterion-precise Item relations for a complete feature
  without leaving a partial graph.
- Exact command pattern: `/tmp/change-saga-authoring relation add --feature
  reviews --id impl-currency-explains-out-of-date --type explains --from
  urn:change-saga:app:slide:review-lifecycle:item:currency --to
  urn:change-saga:app:story:review-is-pr-deck:criterion:out-of-date --rationale
  'The currency Item explains slide- and code-sensitive decision currency.'
  --request-id impl-currency-explains-out-of-date --json app.saga`.
- Observed behavior: every relation required an independent process and
  mutation. A mistyped cross-feature criterion (`derived-label` instead of
  `derived-reviews-label`) failed only after the preceding relations had been
  written.
- Impact: large authoring waves are slow and expose a partial-progress window;
  shell orchestration must implement its own recovery.
- Workaround: used request IDs, queried the created relations, corrected the
  one target, and revalidated the full graph.
- Smallest improvement: support dry-run and atomic JSONL/JSON-array batching
  for relation add and repin, with per-entry errors.

### F7 — Stable code selection is still line-manual

- Intended goal: pin durable named functions and focused tests while avoiding
  whole-file evidence.
- Exact command shape: `/tmp/change-saga-authoring cover --batch - --json
  app.saga` with entries such as `{"target":"...:item:currency","refs":[
  "585411...:internal/reviewstate/reviewstate.go#L270-L350",
  "585411...:internal/cli/reviews_test.go#L98-L157"]}`.
- Observed behavior: atomic batching and structured summaries worked well, but
  the author must calculate inclusive line endpoints manually; the CLI cannot
  select a Go declaration or test by name.
- Impact: the resulting digest makes drift visible, but authoring is slower and
  harmless edits inside a broad function range create avoidable repair work.
- Workaround: selected named declaration bodies plus the narrowest focused test
  and recorded why each range owns the behavior.
- Smallest improvement: add language-aware selectors such as `--go-symbol
  reviewstate.currency` and `--go-test TestReviewDecisionsGoOutOfDateSlideBySlide`,
  resolved to ordinary immutable line/digest references at write time.

### F8 — Scoped structured discovery is too large

- Intended goal: read only accepted current requirements and compact coverage
  for one feature.
- Exact commands: `/tmp/change-saga-authoring query requirements --saga
  app.saga --limit 200` and `/tmp/change-saga-authoring status --json --feature
  agent-loop app.saga`.
- Observed behavior: `query requirements` has no feature filter, while feature
  status embeds full next-action command recipes and produced thousands of
  lines even when only six area counts were needed.
- Impact: callers must download and post-filter a large payload, increasing
  token cost and obscuring the evidence needed for a bounded task.
- Workaround: stored the JSON outside the repository and projected only current
  accepted heads and area summaries with `jq`.
- Smallest improvement: add `query requirements --feature` and a compact
  `status --summary` projection that retains snapshot and area counts.

### F9 — Transitive traceability stops at the design target

- Intended goal: inspect one requirement -> design -> implementation/code path
  in a single structured query.
- Exact command: `/tmp/change-saga-authoring query traceability --saga
  app.saga --requirement bounded-automation --limit 100`.
- Observed behavior: the response includes separate
  criterion -> design and criterion -> Item -> code paths. Even when the design
  landmark has direct exact code evidence, its path ends at the landmark.
- Impact: the query cannot show the fully requested transitive design-to-code
  edge, and a consumer could mistake a design endpoint for an evidence gap.
- Workaround: paired traceability with `/tmp/change-saga-authoring query
  mappings --saga app.saga --target <design-landmark-URN> --limit 100` and
  verified the Item carries matching exact references.
- Smallest improvement: include derived `owns_code` hops for report-design
  targets in traceability paths and expose the evidence-file identity beside
  each code location.

## Verification

The final handoff verification is recorded from the committed tree in the
handoff message. The targeted test command for this wave is:

```text
go test ./internal/areas ./internal/nextaction ./internal/workplan \
  ./internal/requirements ./internal/cli ./internal/reviewapp \
  ./internal/reviewstate ./internal/reviewstore ./internal/server
```

The tests passed before final validation. `internal/reviewstate` has no
colocated test files; its behavior is exercised through the CLI, review-store,
and server integration tests referenced above.
