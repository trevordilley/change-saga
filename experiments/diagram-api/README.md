# Diagram API experiment

An isolated Go CLI and package for explicitly authored diagrams. It is **not a
production Change Saga command or format**. See the [design and findings](../../docs/diagram-authoring-api-exploration.md).

## Run

From this directory, with the repository's Go 1.26 toolchain, Python 3, and Git:

```sh
hivecontrol exec oneshot 5m -- go build -o bin/diagram-spike ./cmd/diagram-spike
hivecontrol exec oneshot 5m -- go test -race ./...
hivecontrol exec oneshot 5m -- go vet ./...
hivecontrol exec oneshot 3m -- python3 examples/run.py runs/demo
hivecontrol exec oneshot 3m -- python3 examples/raw-baseline.py runs/demo
```

Use a fresh output directory for every run. The first script performs 22 CLI
calls, including failed edits, retries, merge conflict, and recovery. Failures
are intentional and asserted. It records actual command traffic and exports
before/moved/after SVG. Raw baseline starts from the same artwork and performs
literal SVG edits using Python's XML library.

Optional screenshot QA, kept outside the Go runtime and production dependencies:

```sh
hivecontrol exec oneshot 5m -- npm ci
hivecontrol exec oneshot 5m -- npx playwright install chromium
hivecontrol exec oneshot 3m -- node examples/render.mjs runs/demo
```

QA blocks network requests, waits for the embedded font, checks bounds and IDs,
and compares final raw/API screenshots byte-for-byte at 1280×720 and 1024×576.
This is standalone SVG QA, not Saga's reviewer-surface visual QA. See the committed
[evidence](evidence/visual-qa.json) and [preview](evidence/after-1280.png).

Authoring sessions for new diagrams (the sequence and composition trials) log
each CLI call and screenshot arbitrary SVGs:

```sh
python3 examples/logged.py runs/mine/session.jsonl -- describe --store runs/mine/diagram
hivecontrol exec oneshot 3m -- node examples/shoot.mjs runs/mine runs/mine/final.svg
python3 examples/summarize-session.py runs/mine/session.jsonl
```

See [sequence](evidence/sequence/) and [composition](evidence/composition/)
evidence and their findings in the design document.

Windows compilation check (from a POSIX shell):

```sh
hivecontrol exec oneshot 5m -- env GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o bin/diagram-spike.exe ./cmd/diagram-spike
```

Runtime and screenshot tests were executed on macOS 15.7.4 arm64, with Chromium
145.0.7632.6. Windows runtime and browser behavior have **not** been tested.
Downloads are needed initially for Go modules and optional browser QA; the built
CLI and its bundled assets require no runtime network.

## API

`bin/diagram-spike schema` lists commands and fields. `examples/scene.json` is
the complete, explicitly positioned example. The CLI uses JSON for element
creation and patches, reserving flags for common targeting/movement operations;
there is no sprawling production flag family yet.

```sh
bin/diagram-spike init --store runs/manual --id example --title Example --request init
bin/diagram-spike assets search database
bin/diagram-spike describe --store runs/manual
bin/diagram-spike describe --store runs/manual --format json
```

Every mutation requires the preceding response's exact snapshot and a unique
request ID. Example `apply` body (replace `SNAPSHOT`):

```json
{
  "version": 1,
  "request_id": "draw-1",
  "expected_snapshot": "SNAPSHOT",
  "operations": [
    {"op":"add","element":{"id":"api","kind":"node","shape":"service","x":80,"y":100,"width":260,"height":110,"label":"API","icon":"lucide:server","style":"normal"}},
    {"op":"add","element":{"id":"db","kind":"node","shape":"datastore","x":500,"y":100,"width":260,"height":130,"label":"Database","icon":"lucide:database","style":"normal"}},
    {"op":"add","element":{"id":"write","kind":"edge","from":"api","to":"db","points":[{"x":340,"y":155},{"x":500,"y":155}],"head":"arrow","head_size":16,"style":"emphasis"}}
  ]
}
```

Pass with `apply --store runs/manual --from request.json`, or stdin. `--dry-run`
validates and renders without publishing; first creation may leave an empty
directory. `move --id api --dx 20 --expected HASH --request move-1` changes only
that object's translation. Add `--store runs/manual`. Edges stay exactly where
authored. `update --id write --set '{"points":[...]}'` explicitly changes a path.
`remove` refuses incident edges and children without `--cascade`; evidence-linked
Items cannot be cascaded away. No operation changes Saga requirements or reviews.

Default named styles: `normal`, `primary`, `emphasis`, `secondary`, `warning`,
`boundary`, `title`. A batch `style` operation defines/replaces a complete style:
`{"op":"style","id":"fat","style":{"fill":"none","stroke":"#2563eb","ink":"#153869","stroke_width":6,"font_size":18}}`.
Elements reference its ID. Icons use the style's ink; their original 24-unit
stroke scales with icon size. Arrow stroke and head size are independent.

`text`, `group`, and `graphic` support other compositions. Groups translate their
explicit children. `align` takes IDs, axis `x|y`, and a coordinate; `distribute`
uses the supplied order and an explicit gap. They require a common parent and
never route arrows. `wrap:true` wraps text inside its fixed box; overflow and
missing font glyphs are errors. These are authoring tools, not automatic layout.

`describe` returns IDs, labels/detail/description, connections, parent/icon and
link counts, pagination, snapshot, and explicit omissions. It cannot reconstruct
the SVG. Output is compact Graphviz-like text by default (see the
[example](evidence/describe-after.txt)); `--format json` returns the same
projection for tooling. The text is a reading view, not an editable syntax. `get --id ID` returns the complete element and exact selector. `source`
is a verbose export used for recovery/merging, not a routine AI read.

## Authority and safety

`current.json` contains the full source, source snapshot, expected SVG hash, and
retry receipts. Immutable local inputs and rendered SVG are staged first; one
atomic record publication makes them current together. The implementation reuses
Saga's lock, atomic-write and publication-error primitives. A failed publication
can leave unreferenced staged files but cannot publish half a batch. Receipt
replay returns the current snapshot and the original applied snapshot.

`check` detects source/visual/input divergence and reports authoring-rule
violations; loading checks integrity only, so an older record stays readable
and repairable after a rule tightens. `rebuild` retains divergent SVG
bytes under `recovered/` before regenerating the expected artifact. It refuses
missing inputs or changed renderer output. There is no SVG-to-source import:
editing the generated SVG is divergence, not a second supported authority.

`merge --base base.json --ours ours.json --theirs theirs.json` merges complete
source exports without publishing. Different-element edits can combine;
same-element edits conflict even if different fields changed. Global styles,
asset pins and document settings merge conservatively. Dependency validation
runs afterwards; apply/render and visual inspection are still needed. Do not
merge generated SVG or hash/receipt records independently; merge source, then
publish the resolved source with `apply`'s `source` field and the current snapshot.

## Boundaries

- SVG generation uses etree 1.6.0, Go font measurement and a curated ten-icon
  Lucide pack pinned by revision and content hashes. Licenses are bundled and
  embedded into standalone exports. No svg.js, React, or browser is needed by
  the CLI. The Go module and QA package are nested; root dependencies are unchanged.
- Rendering order is deterministic by sibling `z` then ID; there are no random
  seeds or generated semantic IDs. Byte reproduction requires the pinned renderer
  and input versions. Pixel equality across platforms is not promised.
- Go Regular supports a limited script repertoire; this is not a shaping engine.
  Shape label slots are deliberately basic. The custom SVG escape supports basic
  primitives/text/groups, rejects unsupported elements/attributes, and does not
  import arbitrary SVG, filters, images, scripts, or external resources. Existing
  custom SVG authoring remains available independently. Path syntax is passed to
  the browser, so malformed/empty custom paths need visual QA.
- Prototype bounds: 500 elements, 1,000 batch operations, 256 retained request
  receipts, 16 MiB stored files. Receipt exhaustion refuses publication. No asset
  garbage collection, format migration, arbitrary path editing, general automatic
  layout, or end-user graphical editor is implemented.
- Saga integration is a temporary test fixture using actual `apply-slide`, exact
  code evidence and story/criterion records. Production linking still belongs to
  Saga. No live `.saga` or review record is touched.
- `trials/cogent` is a separate runnable module testing the richer Go SVG library;
  it is not linked into this prototype. Run `hivecontrol exec oneshot 5m -- go run .`
  there. Excalidraw remains a documented alternative, not a bundled dependency or
  a measured adapter in this native-first experiment.
