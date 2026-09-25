# Technical inventory: Technical Design UI

Status: implemented in the reviewer (`internal/server`) on
`feature/inventory-technical-design-ui-claude`. This note records delivered
renderer behavior and its boundaries. The product design remains
[technical-inventory-design.md](technical-inventory-design.md); the canonical
records and their public authoring belong to the records slice, and the
reverse-use, selection and coverage projections to `internal/inventoryview`.

## What a reader gets

- **Overview → Technical design** (`/technical`). The overview directory and
  sidebar list it. The page is a short landing page naming three areas with
  what each holds; it renders no directory. See
  [Technical design as a tree of pages](#technical-design-as-a-tree-of-pages).
- **Three area pages.** `/technical/erd` draws the application ERD (the one
  named `application`, else the first) with a link to its revision history and
  uses, lists other ERDs and overlays, and holds the directory of every data
  entity. `/technical/systems` and `/technical/components` are those
  directories. Each area page builds only its own content, and its filter
  submits to its own page. Directories count; they never score.
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

## Technical design as a tree of pages

The sidebar and the pages share one tree:

```text
Technical design          /technical              landing page, three areas
  ERD                     /technical/erd          application ERD, other views, all data entities
    each data entity      /technical/data-entity/{id}
  Systems                 /technical/systems      directory of Systems
    each System           /technical/system/{id}
  Components              /technical/components   directory of Components
    each Component        /technical/component/{id}
```

- **Sidebar.** Technical design under the overview discloses ERD, Systems and
  Components through the existing `navSection` rows: each area row opens its
  page, and its twisty discloses one row per definition. The current page's
  row is marked `aria-current="page"` and every ancestor is expanded;
  unrelated areas stay shut. ERDs and overlays have no row of their own, so on
  their pages the ERD row is the current one. An empty area is omitted, like
  every empty section.
- **Addresses.** Definition pages keep `/technical/{kind}/{id}` and
  `?revision=` pins, because slides, drawers and reviews link to them; nothing
  redirects. Their breadcrumb now passes through their area. The landing
  page's area entries carry the old single page's section ids
  (`#technical-systems-section`, `#technical-components-section`,
  `#technical-data-model`), so a bookmarked fragment still lands on the
  matching area. Only `erd`, `systems` and `components` are areas; any other
  `/technical/{segment}` is a 404.
- **Design chapters.** No design chapter rendered on `/technical`: a
  feature's `___design` chapters render on that feature's page and in its
  sidebar places, and they stay there. The old page's in-page jump links
  (Systems, Components, Data model) are replaced by the landing page and the
  sidebar.
- **Cost.** The landing page counts records only. Area and definition pages
  build the shared reverse index and, when comparing, newness.

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
- The ERD page's directory is a table rather than an expandable tree. It is
  filterable, keyboard reachable, and linked row-for-element to the drawing.
  The expandable tree is the sidebar, where the ERD row discloses every data
  entity.
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
`technical_nav_test.go` checks which sidebar rows are open and current on
every Technical design page, the ERD page's other views, breadcrumbs through
each area, old fragments and unknown areas. The browser spec walks the landing
page into an area, opens areas by keyboard and by touch at 390px, and follows
old addresses; `directory-width.spec.ts` reads the Components directory on its
own page at phone width.

Fixed in passing: `/api/layers` cached a git cancellation (`signal: killed`)
as the comparison's error when a reader navigated away mid-derivation, so every
later request answered 500. Cancellations are no longer cached.

## Known gaps

- Every directory, the Technical design ones included, scrolls its table
  inside its own `.directory-scroll` region at phone width, so the page keeps
  the device's width.
- The sidebar lists every data entity beneath the ERD row. With many more
  entities that list grows long; the ERD page's filter is the faster path.
- The ERD view does not pan or zoom beyond browser scaling. On narrow screens
  the directory is the readable path.
- Newness is identity-level (new, revised, unchanged). Relationship-level
  additions within a revision are not yet distinguished in the UI.

## Saga documentation and reconciliation

The slice was first documented under the legacy inventory format; `app.saga`
has since adopted format 2.

- Components `technical-design-page`, `authored-erd-view` and
  `item-selection-drawer`, and System `technical-design-ui`, which pins them
  with the existing `technical-explanation-drawer` and has three
  evidence-bearing interactions. No existing shared record was revised.
- Four implementation slides in the `technical-inventory` deck, section
  "Technical Design UI", ranks 240–255 (`ti-ui-impl-overview`, `-erd`,
  `-trace`, `-verification`). Items pin the new Components and own every added
  line of this slice. 26 `ti-ui-impl-` relations read back active and not
  stale. Visual QA reports no findings at 1280x720 or 1024x576.
- Review `inventory-technical-design-ui` explains `172996c0..94be1293` in four
  slides. It covers 2644 of 2644 changed lines and file events, with no overlaps
  and no stale references. No review decision is recorded.

`reconcile --against 172996c0` after authoring reports:

- 16 documentation-gap entries remain: 9 file-addition events for new files
  and 7 groups of deleted lines. Transaction-managed Items refuse whole-file
  evidence, and living evidence at HEAD cannot cite deleted lines, so the
  review deck owns these.
- 19 evidence regressions are on other slides and the shared
  `technical-explanation-drawer` Component. They cite server code that this
  slice edited. They are handed to the coordinated repair with current ranges.
  The drawer's label changed from "Component and system explanation" to
  "Technical explanation", so explanations that quote the old label also need
  revision.
- The four new definitions show as "reassess" only because this range added
  their code.

### The area pages

Splitting `/technical` moved the code the Technical Design UI's records cite,
so their evidence went stale. Under inventory format 2 every new revision
states intent, and an implemented interaction must join implemented
endpoints:

- Component `technical-design-page` r2 ("Technical design pages") and System
  `technical-design-ui` r3 explain the page tree and pin current code, as
  implemented at the delivery commit. The System's legacy members
  `authored-erd-view` (r2), `item-selection-drawer` (r2) and
  `technical-explanation-drawer` (r3) are restated as implemented with
  unchanged explanations and their current code. System
  `technical-documentation` still pins the drawer's r2, whose content is the
  same; revising it would cascade through its legacy members into the
  application ERD, so it is left to that record's owner.
- New slide `ti-ui-impl-areas` (rank 242, "Technical Design UI") draws the
  landing page, area pages, sidebar tree, area route and unchanged addresses.
  Its six Items own the added code and the new navigation test. The overview,
  ERD, trace and verification slides are republished through `apply-slide`
  with their Items pinned to the current revisions and their evidence at the
  current lines. Visual QA reports no findings for the new slide.
- Review `technical-design-subpages` explains this branch's change over
  `83672282`, the source-branch head it merged, in three slides. It covers
  every changed line and file event in that range, with no overlaps and no
  stale references. No review decision is recorded.

`reconcile --against da5101a6` afterwards reports the same 12 evidence
regressions on the historical proposal slides and 23 pre-existing stale
references as before this change, and no stale code evidence or inventory
evidence introduced by it. The Items and records above were repinned only
after rereading their explanations against the new code.
