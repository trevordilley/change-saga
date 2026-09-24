# Repeatable visual render and QA

`change-saga visual-qa` is the supported, read-only way to render authored
slides outside an interactive review session. It captures both the raw slide
asset and the actual reviewer surface at 1280×720 and 1024×576, then writes a
contact sheet and a machine-readable report.

## Dependency readiness

The operation uses the repository's pinned Playwright stack; it does not call
an external screenshot service. Prepare it once:

```sh
cd e2e
npm ci
npx playwright install chromium
```

By default the CLI searches for `e2e/node_modules/playwright` from the current
directory and its parents. An installed binary can instead use
`--playwright-dir /path/to/change-saga/e2e` or the
`CHANGE_SAGA_PLAYWRIGHT_DIR` environment variable. Node.js must match the
version declared in `e2e/package.json`.

## CLI contract

```text
change-saga visual-qa [--feature ID] [--deck TARGET] [--slide TARGET]
  [--output DIR] [--repo PATH] [--playwright-dir DIR] [--json] <saga>
```

Selectors narrow cumulatively. IDs and full URNs are accepted for decks and
slides; `--feature onboarding` selects the app onboarding deck. With no
selectors, every implementation and onboarding slide is rendered.

The default output is
`./change-saga-visual-qa/<saga-id>`. A successful run creates:

```text
<output>/
  .change-saga-visual-qa
  visual-qa.json
  contact-sheet.png
  slides/<deck-id>/<slide-id>/
    raw-1280x720.png
    raw-1024x576.png
    reviewer-1280x720.png
    reviewer-1024x576.png
```

The hidden marker identifies a managed output directory. A repeat run replaces
only an existing directory with that marker. The CLI refuses filesystem roots,
the home directory, symlink output targets, an output inside the Saga, an
output that contains the Saga, and unmarked existing directories. Safety
resolution walks to the nearest existing ancestor before resolving symlinks,
so nested nonexistent directories beneath a symlink into the Saga are refused
without creating them.

The CLI stages a complete run beside the destination and publishes it only
after Playwright and the temporary loopback reviewer have both stopped. When a
managed report already exists, it is renamed to a recoverable sibling backup
until the new report is installed. A failed publish restores the prior report;
if restoration also fails, the error names the retained backup path. No Saga
file is written.

`--json` prints the same report stored in `visual-qa.json`. Exit status is 0
when no error-severity mechanical finding exists, 3 when rendering completed
with error findings, and 1 when the render could not be trusted or completed.

## Mechanical findings

The browser helper can report:

- `missing_selector` or `empty_selector` for an Item's element selector;
- `item_clipped` and `asset_clipped` for bounds outside a standard viewport;
- `text_overflow` when an element mechanically clips its text;
- `item_overlap` when two addressable, independently positioned HTML boxes or
  rectangular SVG elements overlap by at least 15% of the smaller bounds;
- reviewer-surface failures such as a missing or hidden selected slide and
  horizontal viewport overflow.

Overlap findings are warnings because overlap may be intentional. SVG groups
and paths are excluded because their bounding rectangles do not reliably mean
their painted shapes collide. Region selectors are rendered but do not create DOM-element overlap findings. Raster
content can be inspected for viewport fit, but arbitrary text inside pixels is
not inferred. The report always records `semantic_arrows: "not_evaluated"`:
rendering an authored edge cannot prove its direction or product meaning.

## Reusable layout helpers

`internal/visuallayout` provides deterministic 1280×720 SVG starters for
sequence, state, and ownership views. Every `Node`, `Link`, and `Lane` ID is
emitted unchanged as both `id` and `data-item-id`, so the corresponding Saga
Item can use an exact element selector. Duplicate, unstable, and unknown IDs
are rejected before SVG is produced.

The helpers arrange authored facts; they do not infer or validate semantic
relationships. Callers should create Item records for the stable IDs through
the public authoring CLI, and should review arrow meaning separately from the
mechanical visual-QA report.
