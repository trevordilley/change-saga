# Visualization architecture review

## Verdict

**The boundaries are broadly sound; tighten presentation adapters, not the domain
model.** There are **three presentation families: Essays, slide decks, and
whiteboards**. A story with acceptance criteria is an **Essay composed with structured
requirement content**, not a fourth visualization. It remains a first-class requirement
record: sharing its reading layout must not erase revision, acceptance, evidence, or
coverage semantics.

Assessed local `main` at `1aa3e709` (Roadmap), including `bb7db21a` (separate Essay
subtext/abstract) and the technical-design Essays. This workspace started at that same
commit. This report changes no application behavior or Saga records.

## Prioritized findings

### 1. P2 — Independent Essay fields are not consistently projected

Story subtext is assigned only inside `if Overview != ""` in
[requirements.go:234](../internal/server/requirements.go#L234), although the template
renders it independently at [template.go:305](../internal/server/template.go#L305). A
revision with `subtext: "Independent subtext"` and no overview silently loses its
subtext in the reader. Criterion projection does not have that dependency
([requirements.go:247](../internal/server/requirements.go#L247)). A temporary executable
probe confirmed this behavior.

This is a concrete consequence of assembling Essay framing separately in feature, theme,
technical, deck, story, and criterion views. Shared `essay-subtext` / `essay-abstract`
templates exist, but live inside the deck overview file
([deckoverview.go:211](../internal/server/deckoverview.go#L211)); typography is shared
at [styles.go:591](../internal/server/styles.go#L591). Project framing still composes
pitch/description fragments ([template.go:59](../internal/server/template.go#L59)).
Those storage differences are legitimate; omission behavior should not depend on them.

**Risk / action:** authored information disappears. First fix the story projection and
test subtext with absent, present, and explicitly cleared overview. Then extract a small
Essay presentation adapter and template module with owner identity, title, optional
subtext, abstract, and body slots. Allow a structured abstract: story statement and AC
obligation are escaped requirement text, not interchangeable free-form Markdown.
Preserve fragment history and evidence controls inside the body slot. Existing [story
reading tests:11](../internal/server/story_reading_test.go#L11) cover safe prose and
complete obligations but miss subtext without prose.

### 2. P2 — Markdown policy is shared incompletely; anchors already diverge

The ordinary renderer enables explicit heading attributes, namespaced footnotes, and the
shared safe heading/HTML renderer
([markdown.go:20](../internal/server/markdown.go#L20)). Deck Essays construct another
Goldmark engine without `parser.WithAttribute()`
([deckoverview.go:139](../internal/server/deckoverview.go#L139)). The same `## Heading
{#stable}` produces `probe--stable` in ordinary prose but `probe--heading-stable` and
visible `{#stable}` in a deck Essay. This was reproduced, not inferred.

Abstracts call the generic `markdown` template function, whose namespace is always
`heading` ([deckoverview.go:212](../internal/server/deckoverview.go#L212),
[server.go:2341](../internal/server/server.go#L2341)). Two rendered abstracts with the
same heading/footnote generate identical IDs; multiple deck Essays can coexist in one
viewer. Body rendering has owner namespaces, so the inconsistency is specific and
avoidable.

**Risk / action:** copied prose loses stable links; abstract links can target another
Essay. Share engine construction and require an owner-and-slot namespace, including
abstracts. Keep the deck-only AST transformation for `annotation:` links and `slide:`
images as an explicit extension, not a universal Essay requirement. Add a cross-surface
test matrix for explicit headings, repeated headings/footnotes, raw HTML, unsafe URLs,
and citations inside code spans. Existing
[deckoverview_test.go:22](../internal/server/deckoverview_test.go#L22) tests safe
rendering and exact evidence references, but not parser parity or multiple abstract
namespaces.

### 3. P2 — SVG policy has several implementations; preserve trust distinctions explicitly

Slide SVG/HTML uses sandboxed frames
([template.go:423](../internal/server/template.go#L423)) and authored-content CSP
([server.go:2181](../internal/server/server.go#L2181)). Whiteboards inline renderer
output through `inlineDiagramSVG`; authored ERD artwork uses `sanitizeSVG`. Both server
functions parse XML, allowlist drawing attributes, and rewrite IDs, but have different
policies ([whiteboard.go:148](../internal/server/whiteboard.go#L148),
[technical_svg.go:53](../internal/server/technical_svg.go#L53)). Generated output needs
renderer metadata, theme tokens, and isolated embedded SVG images; arbitrary artwork
should not inherit that trust.

A third path fetches slide SVG into a hidden shadow-root measurement clone, stripping
scripts, foreign objects, and descendant event/external-link attributes
([appjs.go:435](../internal/server/appjs.go#L435)). It is a denylist rather than either
server allowlist, and `querySelectorAll('*')` does not process the root SVG's
attributes. Inventory validation adds another deliberate boundary: digest checks and
recursively safe offline SVG
([inventory_validate.go:541](../internal/requirements/inventory_validate.go#L541)).

**Risk / action:** policy drift and rendering incompatibility; this assessment does
**not** establish an executable exploit. Start with a common adversarial fixture corpus
across authored SVG, generated diagrams, imported sources, and slide measurement,
including root attributes and external CSS/paint references. Then share XML
escaping/namespacing mechanics under explicit authored/generated policies. Audit
measurement separately; do not solve reuse by allowing raw artwork into the page.
Existing [whiteboard_test.go:67](../internal/server/whiteboard_test.go#L67),
[technical_erd_test.go:235](../internal/server/technical_erd_test.go#L235), and
[whiteboard_sources_test.go:293](../internal/server/whiteboard_sources_test.go#L293)
exercise important boundaries.

### 4. P3 — Named whiteboards expose an uneven loading/cache boundary

Shared `sagaFiles` caches narrative, requirements, inventory, projected links, and
embedded slide HTML per documentation fingerprint
([sagafiles.go:105](../internal/server/sagafiles.go#L105),
[sagafiles.go:195](../internal/server/sagafiles.go#L195)). Whiteboard discovery is
cached without rendering every canvas
([whiteboard_documents.go:44](../internal/server/whiteboard_documents.go#L44)), but the
presentation cache has one slot for the data-model board
([sagafiles.go:204](../internal/server/sagafiles.go#L204)). Named board routes load
inventory and rebuild their selected board
([technical.go:548](../internal/server/technical.go#L548)); resolving it loads inventory
again ([whiteboard.go:68](../internal/server/whiteboard.go#L68),
[whiteboard.go:307](../internal/requirements/whiteboard.go#L307)). Ordinary page
navigation can also build the data-model board for its sidebar
([server.go:1425](../internal/server/server.go#L1425)).

**Risk / action:** redundant resolution/rendering as boards grow, not demonstrated stale
output. Measure repeated named-board requests before changing caching. If justified,
cache resolved presentations by document ID and selected revision within the existing
file generation; accept an already-loaded inventory at the resolver boundary. Keep
discovery lightweight and source-asset invalidation intact. Extend
[whiteboard_navigation_test.go:150](../internal/server/whiteboard_navigation_test.go#L150)
to named boards, historical revisions, source edits, and render counts. Do not cache
mutable camera state or mix review coverage into a presentation cache.

## What is already shared, and what should stay separate

| Path | Actual reuse and boundary |
| --- | --- |
| Essay authoring → loading → rendering | Feature/project/technical prose uses `setOverviewPart`, stable fragment identity, and batched body/metadata writes ([overview.go:123](../internal/cli/overview.go#L123)). The loader enforces Markdown for technical Essays ([load.go:311](../internal/saga/load.go#L311)); `makeFragmentView` uses safe Markdown ([server.go:1984](../internal/server/server.go#L1984)). Deck Essays instead live on the deck manifest ([deck_overview.go:96](../internal/cli/deck_overview.go#L96)), with objective fallback and generated slide directory when absent ([deckoverview.go:56](../internal/server/deckoverview.go#L56)). Share presentation, not persistence. |
| Diagram authoring → layout → rendering | `apply-slide` calls `diagram.Render` and records source/rendered assets ([slide_transaction.go:871](../internal/cli/slide_transaction.go#L871)). Whiteboard publish validates a complete `diagram.Document`, resolves pinned sources, then calls `Composition.Render` ([whiteboard.go:231](../internal/requirements/whiteboard.go#L231)). Both use `renderTree`, fonts, shapes, styles, connectors, notes, and reveal machinery ([render.go:50](../internal/diagram/render.go#L50), [composition_render.go:12](../internal/diagram/composition_render.go#L12)). Authored coordinates and composition transforms are shared; there is no need for a second layout engine. Flat slide rendering intentionally rejects composition frames/connections. Whiteboards add nested instance paths and cross-section connections. Source adapters currently support ERD/data-entity pins, not arbitrary infrastructure inventory ([whiteboard_resolve.go:133](../internal/requirements/whiteboard_resolve.go#L133)). |
| Visual evidence and interactions | Deck → section, slide → fragment, Item → landmark projection reuses existing review/evidence plumbing ([slides.go:539](../internal/saga/slides.go#L539)). Whiteboards reuse annotation controls, note popovers, lazy code/story drawers, and page abort lifecycle ([diagram_interactions.go:92](../internal/server/diagram_interactions.go#L92), [diagram_annotations.go:228](../internal/server/diagram_annotations.go#L228)). They do not manufacture Items: story links retain the exact implementation Item whose documentation pin matches; relationship evidence does not inherit entity-wide stories ([diagram_annotations.go:174](../internal/server/diagram_annotations.go#L174)). Deck Essay citations reuse these controls, and whole-slide citations do not silently select an Item ([deckoverviewjs.go:83](../internal/server/deckoverviewjs.go#L83)). |
| Navigation and reading | Owning headers open Essays ([navigation.go:133](../internal/server/navigation.go#L133)); decks retain sequential slides and overview/front/back faces ([deckoverviewjs.go:43](../internal/server/deckoverviewjs.go#L43)). Stories compose full obligations, native AC disclosures, and secondary traceability ([template.go:300](../internal/server/template.go#L300)). Whiteboards retain camera/detail/fit controls and document+revision+instance focus, with browser history restoration ([whiteboard_navigation.go:56](../internal/server/whiteboard_navigation.go#L56)). These are different interaction contracts, not redundant viewers. |
| Queries and history | Fragment/slide queries share dispatch ([query.go:888](../internal/cli/query.go#L888)); Git-backed record history uses `NodeHistory` ([history.go:58](../internal/changeview/history.go#L58)). Requirement history exposes complete revisions and separate lifecycle events in graph order ([compose.go:45](../internal/livingapp/compose.go#L45)). Whiteboard `get/describe/check` reads the same resolved composition as rendering ([diagram_whiteboard.go:86](../internal/cli/diagram_whiteboard.go#L86)); named histories and exact revision URLs remain independent ([whiteboard_documents.go:70](../internal/server/whiteboard_documents.go#L70)). Complete-slide transactions also preserve snapshots and divergent heads ([slide_transaction.go:382](../internal/cli/slide_transaction.go#L382)). A universal mutable Essay record would break these contracts. |

Requirement semantics belong in `requirements`, not in Essay components. Single-parent
story edits inherit omitted prose; full/multi-parent snapshots do not choose an implicit
winner ([story_authoring.go:93](../internal/cli/story_authoring.go#L93)).
Story/criterion prose and subtext participate in obligation currency; unchanged sibling
criteria can carry forward
([currency.go:366](../internal/requirements/currency.go#L366),
[story_overview_test.go:5](../internal/requirements/story_overview_test.go#L5)).
Technical Essay changes likewise affect design digests
([design_digest.go:80](../internal/saga/design_digest.go#L80)).

The canonical Item-to-criterion projection feeds GUI/query/traceability/audit consumers
([slide_links.go:13](../internal/semanticgraph/slide_links.go#L13)). Presentation reuse
cannot grant acceptance or evidence coverage:
[deck_overview_test.go:13](../internal/coverage/deck_overview_test.go#L13) explicitly
checks that Essay references transfer no code coverage. Review coverage has its own
commit/range/Item/attribute cache
([reviewcache.go:17](../internal/server/reviewcache.go#L17)); disk snapshot caches
refuse unpinned source keys
([snapshotcache.go:87](../internal/snapshotcache/snapshotcache.go#L87)). Keep these
semantic caches separate from rendered pages.

## Features / Roadmap: a sound example of reuse

Roadmap is a filtered, priority-ordered collection of the **same feature pointers**, not
a second feature type ([roadmap.go:9](../internal/server/roadmap.go#L9)). Both
collections use the same directory builder, grouping, `/features/<id>` pages, Essays,
stories, ACs, design, quality, and decks
([server.go:1421](../internal/server/server.go#L1421),
[feature_themes.go:61](../internal/server/feature_themes.go#L61)). Theme membership can
span both collections; grouping filters members without changing canonical records.

Immutable identity remains in `feature.json`; mutable status/milestone/priority lives in
`plan.json`. Missing plans default to built, and promotion is an explicit product
decision independent of story acceptance
([feature_plan.go:13](../internal/applayout/feature_plan.go#L13)). Plan edits preserve
omitted fields ([feature_plan.go:43](../internal/cli/feature_plan.go#L43)). CLI Roadmap
repeats the small filter/sort projection but shares `Planned` and `PriorityBefore`; its
cursor hashes returned rows, so planning changes invalidate pagination
([feature_plan.go:89](../internal/cli/feature_plan.go#L89)). Feature context includes
that same plan
([feature_context.go:189](../internal/livingapp/feature_context.go#L189)). This modest
projection duplication does not justify a new model or framework.

Evidence: [feature_plan_test.go:16](../internal/cli/feature_plan_test.go#L16) verifies
identity/content preservation on promotion;
[feature_plan_test.go:100](../internal/cli/feature_plan_test.go#L100) checks ordering
and stale cursors; [roadmap_test.go:37](../internal/server/roadmap_test.go#L37) checks
outline invalidation; [roadmap.spec.ts:4](../e2e/tests/roadmap.spec.ts#L4) exercises
canonical URLs, shared reading surfaces, grouping, and promotion. Add a small server/CLI
parity matrix for tied/unranked priorities, mixed themes/personas, and legacy plans
before extracting any collection helper.

## Small delivery sequence

1. Fix independent story subtext projection and add its regression test. No storage
   migration.
2. Consolidate Markdown configuration and owner/slot namespaces, retaining deck-specific
   citation transforms; move Essay primitives into their own presentation module. Test
   absent fields, multiple co-rendered Essays, parser parity, and unchanged story/AC
   obligation behavior.
3. Establish the SVG policy fixture matrix, then deduplicate safe serialization
   mechanics only where policies match. Retain sandboxed slide content and explicit
   generated/authored distinctions.
4. Measure named-board loading; only then introduce a bounded per-document/revision
   presentation cache if warranted. Preserve history/source invalidation tests and
   separate camera and review state.

Keep the shared diagram renderer, feature collection model, semantic graph, immutable
revision stores, review overlays, and distinct reading/navigation modes. No universal
visualization schema, new product format, or broad server rewrite is warranted.

## Validation

- Passed complete package tests for `internal/diagram`, `internal/saga`,
  `internal/applayout`, `internal/requirements`, `internal/cli`,
  `internal/snapshotcache`, and `internal/changeview`.
- Passed focused server tests with `go test ./internal/server -run
  'Test(Roadmap|StoryReading|CriterionReading|TechnicalDesignEssay|DeckOverview|DiagramInteraction|DiagramAnnotation|Inline|SanitizeSVG|Whiteboard|NamedWhiteboard)'
  -count=1`.
- Passed targeted coverage tests for non-transferred Essay coverage and stale story
  exceptions, plus livingapp design-currency, feature-context, and traceability tests.
- Temporary `TestArchitectureReviewProbe` reproduced missing story subtext, differing
  explicit heading IDs, and identical IDs across two abstracts; removed after execution.
- The full server suite failed on repository-Saga requests returning HTTP 500, including
  `TestTheDecksAreBuiltOncePerStateOfTheSaga`. Read-only `go run ./cmd/change-saga
  validate change.saga` confirmed absolute deck paths exceed the 240-character limit in
  this managed workspace ([flat.go:93](../internal/saga/flat.go#L93)). This limits full
  integration validation here; it is not evidence that these presentation refactors are
  needed. No Saga repair or checkout relocation was performed.
- Browser test sources were inspected, including deck Essay navigation, story
  disclosures, whiteboard focus/history and lazy evidence, and Roadmap promotion. E2E
  dependencies are absent, so no browser run is claimed. All evidence links were
  checked; only this report is committed.
