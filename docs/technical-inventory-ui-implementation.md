# Technical inventory: Technical Design UI

Status: implemented in the reviewer (`internal/server`) on
`feature/inventory-technical-design-ui-claude`. This note records delivered
renderer behavior and its boundaries. The product design remains
[technical-inventory-design.md](technical-inventory-design.md); the canonical
records and their public authoring belong to the records slice, and the
reverse-use, selection and coverage projections to `internal/inventoryview`.

## What a reader gets

- **Overview → Technical design** (`/technical`). The overview directory and
  sidebar list it with its Systems, Components, ERDs and data entities. The page
  holds a directory per kind, the application ERD and every data entity.
  Directories count; they never score.
- **One canonical page per definition** (`/technical/{kind}/{id}`). With no
  revision it renders the unique current revision. `?revision=` renders that
  exact saved revision and states whether it is stale, retired or conflicted;
  nothing is repinned. Competing heads are all named and none is chosen. A
  missing kind, identity or revision is a 404, never the latest definition.
- **Separate facts.** Intent is the revision's explicit intent, or
  `unspecified` for legacy revisions. A proposal names its retained implemented
  baseline or says it has none; an implemented revision names its delivery
  commit and says that implementation is not verification or approval.
  Lifecycle and code currency are reported separately. System interactions and
  data-entity relationships show their own intent; proposed interactions are
  drawn dashed.
- **Newness only against a named comparison.** When the reviewer compares a
  head against a base, each identity is `new`, `revised` or `unchanged` relative
  to the inventory at the comparison's merge-base, loaded through `savedview`.
  An unreadable base, including a Saga in a companion repository, is `unknown`
  with its reason. Observing one commit shows no newness.
- **Data entities.** Purpose, author-selected fields and keys (stated as not
  exhaustive), holding Components as separate resources, owned relationships,
  incoming relationships derived from other entities' current revisions, the
  ERDs that list the entity, and exact code. A code-free proposal remains
  visible and says it has no code yet.
- **Authored ERD view.** The ERD revision's own offline SVG is drawn as authored
  and never generated. Only bound elements become controls; each opens the same
  pinned drawer as its directory row, by pointer, touch or keyboard, and focus
  returns on Escape. The directory states which entities are drawn and which are
  only listed. Associations show crow's-foot endpoints with textual cardinality,
  including a declared `unknown`; productions are labelled directional flows and
  say they are not foreign keys. Overlays compose over their exact baseline ERD
  revision and mark added, replacing and removed rows without rewriting it.
- **Usages.** A definition's page and drawer list declared uses from the shared
  reverse index: slide Items, owners (System membership, data holders,
  relationship destinations, ERD directories and overlays) and Items that reach
  it through one owner, with the path. Each links the exact slide Item or owner
  revision. Counts are lower bounds, and say so, if the index stopped early.
- **Slide Item → definition → selected code → back.** An Item's control names
  the Item. Opened from it, the drawer shows the Item's selections before the
  definition's complete code: the declared path of pinned revisions, the
  evidence and containing reference, the selected lines at the observed head,
  containing-reference staleness as a separate fact, whether the saved digest
  still matches the selected lines at their commit, and any unresolved reason
  code. A saved-view pin says so. Escape returns focus to the control and the
  slide URL is unchanged.
- **Coverage parity.** The reviewer's comparison coverage includes lines Items
  inherit through eligible selections, as the CLI does, and labels those owners
  "selected via" the selection rather than as authored evidence.

## Safety of authored drawings

The loader validates ERD assets, but inlining the author's bytes would still
execute anything a validator missed, and the page allows inline script. The
drawing is therefore parsed as XML and re-serialized from its tokens with an
allowlist of static SVG elements and presentation attributes. Links must be
same-document fragments. Every id is namespaced to the view so it cannot shadow
page ids, and bindings use the namespaced ids. A drawing needing anything else
(animation, script, handlers, `style`/`class`, external links, directives) is
not shown in part: the page states that it is unavailable and why, and the
directory still lists every entity. A drawing whose bytes no longer match its
digest is refused the same way.

## Deliberate divergences from the proposal slides

- The authored-ERD slide draws holding resources as containers around entity
  cards. The renderer does not infer containment from a drawing; it lists
  holders as data, and an author may draw containers in the ERD asset.
- The directory is a table rather than an expandable tree. It is filterable,
  keyboard reachable, and linked row-for-element to the drawing.
- `class` and `style` attributes are refused in ERD drawings, so authors must
  use presentation attributes. This is stricter than the loader.

## Verification

Focused Go tests live in `internal/server/technical*_test.go`. They use the
shared `testfixture.WriteInventoryFixture`, plus extra records written through
`WriteTechnicalRevision`. The only hand-built record is a simulated merge
conflict, made by copying a revision file. The browser checks are
`e2e/tests/technical-inventory-ui.spec.ts`: desktop and 390px touch
navigation, keyboard and Escape focus, permalink reload, 404s, an ERD and Item
selection authored through the public CLI, and reviewer coverage attribution.

Fixed in passing: `/api/layers` cached a git cancellation (`signal: killed`)
as the comparison's error when a reader navigated away mid-derivation, so every
later request answered 500. Cancellations are no longer cached.

## Known gaps

- `/features` and `/personas` directory tables widen the 390px layout viewport.
  Technical design tables now scroll within their section. The shared
  directory layout is unchanged.
- The ERD view does not pan or zoom beyond browser scaling. On narrow screens
  the directory is the readable path.
- Newness is identity-level (new, revised, unchanged). Relationship-level
  additions within a revision are not yet distinguished in the UI.
