# Diagram authoring API exploration

Status: design exploration, 2026-09-24. Incorporates the author's requested
imperative drawing model and selection of Lucide as the default icon family.
Commands and record shapes below are proposals, not shipped CLI contracts.
No production format migration, editor replacement, or existing asset conversion
is proposed by this document. A runnable native prototype and measured evidence
now live in [experiments/diagram-api](../experiments/diagram-api/README.md).

## Direction

Provide a drawing toolkit with explicit geometry, reusable shapes and icons,
targeted edits, and a compact structural reading API. The AI authors the
composition and checks its appearance. There is no diagram layout engine.

Moving a node changes only its geometry. Edges retain their authored paths.
Moving an explicit group transforms its children together. Edge `from` and `to`
fields record connections; they impose no geometry constraints and create no
requirements relations. Text wrapping and alignment are explicit operations or
declared shape properties; no operation silently resizes neighboring content,
reroutes edges, or rearranges the canvas.

Keep three concerns separate:

1. **Authoring API:** compact commands and atomic batches over named objects.
2. **Authoritative drawing document:** complete state, including geometry,
   object identities, styles, shape definitions, and pinned asset inputs.
3. **Drawing backend:** a library that materializes those choices into SVG.

The document produces both a pretty SVG and a small `describe` response. The
description is deliberately incomplete and cannot reconstruct the drawing.
It is neither Graphviz syntax nor an alternative editable source.

## Lucide and the drawing language

Use **Lucide** as the default icon family for authored diagrams. Its
[design guidance](https://lucide.dev/guide/) emphasizes a consistent visual
style, and its [static distribution](https://lucide.dev/guide/static) provides
individual SVG files without a JavaScript framework dependency. A Go backend
can consume those assets without integrating the Lucide JavaScript runtime.

Lucide supplies glyphs, not complete technical diagram shapes. Build a small
Saga shape library around them: service, datastore, queue, document, actor,
decision, and boundary, alongside ordinary rectangles, ellipses, paths, text,
images, and groups. A shape can compose a body, icon slot, and label slot with
explicit internal geometry. It must not reduce every diagram to cards.

The proposed visual language has four parts:

| Part | Responsibility |
| --- | --- |
| Lucide assets | Consistent icons selected by stable names |
| Saga shapes | Reusable compositions with documented size and label slots |
| Saga theme | Font inputs, palette, strokes, spacing, and emphasis presets |
| Explicit overrides | Exact geometry, colors, paths, and typography when needed |

Start with a curated technical subset, for example `server`, `database`,
`file`, `user`, `cloud`, `lock`, and `terminal`; verify names against the pinned
upstream version when importing. Asset names use a namespace such as
`lucide:database`. Shape names and icon names are separate: changing an icon
does not change a node's structural role or requirements meaning.

Icons preserve their source viewBox and aspect ratio. Size, color, and stroke
are explicit resolved properties. The initial theme should use a consistent
24-unit icon convention and normal outline stroke; exact values become versioned
theme data. Specify whether stroke widths scale with the icon or stay in canvas
units, and serialize that choice. Edge emphasis is a separate stroke/head preset,
not an accidental consequence of scaling an icon. Typography needs its own
pinned fonts; Lucide does not solve text measurement.

Icons inside a node are decorative children of that Item by default. A meaningful
standalone icon can have its own Item and accessible label. An icon never
substitutes for the node's readable label or explanation. Renderer chrome keeps
its existing original icon system; this proposal concerns authored content only.

### Asset packaging and reproducibility

Select an exact upstream release or commit at implementation time. Import a
curated set of static SVG assets and record the source revision, canonical name,
content digest, and license notices in an asset manifest. Do not resolve
`latest`, download icons at command execution time, or rely on a CDN.

The CLI can embed its default pack with Go's embedding facilities. Each saved
drawing must also retain the exact asset bytes it uses in its committed,
content-addressed inputs, or an equivalent committed immutable pack. A version
number pointing only to assets inside the installed binary is insufficient:
an older diagram must remain rebuildable after upgrading the CLI.

Persist shape/theme versions and the renderer identity as well. Library upgrades
are explicit edits; they must not silently replace assets in existing diagrams.
Missing or corrupted pinned inputs are errors, not invitations to substitute a
similar icon. Exported SVG is self-contained, with deterministic namespacing of
any internal asset IDs and references so repeated instances do not collide.

Retain the complete applicable upstream license notices when vendoring assets.
Lucide uses ISC and identifies inherited Feather icons under MIT, including
several likely technical defaults. Carry notices with reusable asset packs and
standalone exports so assets remain attributable outside the repository.
See the [official license](https://lucide.dev/license).

### Compact discovery and use

Illustrative proposed interface:

```sh
change-saga diagram assets search database --limit 5
change-saga diagram assets describe lucide:database

change-saga diagram node add cache --diagram request-path \
  --shape datastore --icon lucide:database \
  --label "Session cache" --at 480,180 --size 180,100

change-saga diagram edge add lookup --diagram request-path \
  --from api --to cache --start 320,230 --end 480,230 \
  --head arrow --weight emphasis

change-saga diagram describe request-path
```

Discovery returns a bounded page of names, concise meanings, and supported
parameters. It does not dump SVG paths or the whole catalog. Unknown names fail
with bounded suggestions. Changing a theme or icon is explicit; no semantic
guessing or automatic asset replacement occurs.

An example reading response could show:

```text
Diagram request-path [snapshot ...]
cache: datastore "Session cache" [icon lucide:database]
lookup: api -> cache
Reading view: geometry, detailed styling, and decorative children omitted.
```

Targeted queries expose full object properties when needed. JSON responses
should state the snapshot and omitted fields; graph-like text is a presentation
of that same projection. Structural reads use the authoritative document and do
not reconstruct connections by inspecting SVG paths.

## Authority, transactions, and escape hatches

Store current drawing state, not only a replay log. That state must contain all
inputs needed to rebuild the exact authored composition, including explicit path
coordinates, text, transforms, z-order, groups, custom fragments, and asset pins.
Operations can retain history without making replay mandatory for a read.

For a batch, check the expected snapshot, apply changes to a private copy,
validate dependencies, produce SVG, and calculate its byte hash. Stage immutable
inputs and SVG first, then publish one record that identifies the complete
drawing revision and expected SVG hash. Two independent file replacements do
not provide this atomic boundary. A reader must see a complete old or new revision.

Use the safety model of [complete-slide transactions](ergonomics-transaction-authoring.md):
snapshot checks, stable request IDs, identical retry detection, explicit
divergent heads, and a single publication point. Integrating the new drawing
source with that record would need a separate adoption decision; existing
`apply-slide` does not already define this new authoring source format.

A hash mismatch detects visual divergence. Explicit rebuild renders from the
authoritative document; preserve externally modified visual bytes before
replacement. Byte-identical regeneration requires the same renderer and pinned
inputs. Do not claim browser pixels are identical across platforms or fonts.

Support custom SVG fragments inside the complete source for unsupported
compositions, with declared identity and bounds where available. Plain custom
SVG remains an independent existing authoring mode. No automatic import or
bidirectional round-trip is promised, and no existing app assets are converted.

## Saga identity and concurrent work

Map each semantic drawing object to an existing stable Saga Item ID and emit
an exact SVG selector for it. Decorative subparts get namespaced internal IDs,
not extra evidence-bearing Items. Moves, label edits, and icon replacements
preserve Item identity and its exact code and criterion links. Validate selector
resolution before publication. Never infer intent changes, repin references,
or approve a review as a consequence of a visual edit.

Removal defaults to refusing dependencies and reports incident edges, children,
and Item links. Any cascade is explicit and cannot silently discard evidence.
Reviewer annotations remain separate from authored diagram objects.

Readable deterministic storage helps Git review but does not guarantee semantic
merges. Different-element edits may combine while introducing overlap, broken
references, or conflicting style changes. Test text merges and validate the
combined document; preserve competing same-element edits and snapshot heads.
Generated SVG should be regenerated from a resolved source, not independently
merged as a competing authority. Global asset/theme changes are dependencies
of every object that uses them.

## Prior art and backend candidates

| Candidate | Relevant finding | Current role |
| --- | --- | --- |
| [Cogent SVG](https://pkg.go.dev/cogentcore.org/core/svg) | Editable SVG tree, XML I/O, geometry and transform APIs | Tested v0.3.42: preserved object ID but rewrote metadata attributes into CSS and dropped a filter; not selected for this prototype |
| [etree](https://github.com/beevik/etree) | Pure Go XML read/query/edit/write with standard-library dependencies | Smaller alternative requiring Saga-specific drawing conveniences |
| [SVGo](https://github.com/ajstarks/svgo) | Imperative SVG output to a writer | Useful creation API, not a complete targeted-edit model |
| [Canvas](https://github.com/tdewolff/canvas) | Rich vector paths, typography and multiple output backends | Possible geometry/text support; evaluate identity preservation before adoption |
| [tldraw agent](https://tldraw.dev/starter-kits/agent) | Simplified overview records and focused detail for AI | Precedent for asymmetric compact reads and detailed authoring |
| [Excalidraw libraries](https://libraries.excalidraw.com/) | Reusable collections of drawing elements | Asset/composition prior art; conversion/backend experiment remains optional pending revised scope |

Lucide asset selection does not select the drawing backend. Excalidraw's
[simplified element API](https://docs.excalidraw.com/docs/@excalidraw/excalidraw/api/excalidraw-element-skeleton)
regenerates IDs by default, so any later adapter must explicitly preserve them
and verify exported selectors. D2/Graphviz-style layout generation is outside
the agreed design. Borrow readable graph descriptions without using them as
the authoritative drawing language.

Existing [visuallayout](../internal/visuallayout/layout.go) demonstrates stable
SVG Item IDs; its automatic arrangements are not the proposed authoring model.
Use [visual QA](ergonomics-visual-qa.md) to check rendered results. The compact
persona/term query contracts are also a useful local precedent: stable identities,
bounded responses, snapshots, and explicit completeness, without inferring
semantic references from diagram text.

## Executed native experiment

The [isolated Go CLI](../experiments/diagram-api/README.md) implements creation,
add/get/update/move/remove, named styles, explicit alignment/distribution,
compact description, source export, render, hash check/rebuild, conservative
three-way merge, and atomic batches with expected snapshots and replay IDs.
No production CLI, schema, existing asset, requirements relation, or review
record changed. The later author direction selects this native-first experiment;
the original Excalidraw runtime A/B comparison has **not** been executed.

One authored publication-flow diagram combines services, a datastore cylinder,
a warning branch, free text, a boundary group, paths and Lucide icons. The author
positions everything. Moving the validator leaves all three incident paths
unchanged; a separate batch explicitly adjusts their points and changes its
label/style. A temporary edge is added and removed. The final compact description
retains seven semantic elements and their explanatory text and connections.
See [before](../experiments/diagram-api/evidence/before-1280.png),
[moved without path edits](../experiments/diagram-api/evidence/moved-unrepaired-1280.png),
and [final](../experiments/diagram-api/evidence/after-1280.png).

The richer [Cogent trial](../experiments/diagram-api/trials/cogent/main.go)
round-tripped a small SVG: `id="api"` survived, `data-item-id` and `data-from`
became CSS declarations, and the filter disappeared. Its module graph also
includes GUI/media/platform packages; that graph is not a binary-size measurement.
The [captured result](../experiments/diagram-api/evidence/cogent-roundtrip.txt)
supports choosing etree for faithful, explicit SVG construction in this spike;
it does not establish that Cogent is unsuitable for other drawing tasks.

The native binary uses etree 1.6.0, x/image 0.44.0 and x/text 0.40.0, plus Saga's
existing store package. The heavier trial and browser QA are separate modules.
Lucide inputs are pinned at commit `66d8f9fc394b8530377e5f6112f0b8908ba01280`;
Go Regular is pinned with the renderer. All inputs and notices ship locally.
No runtime downloads, seeds, or automatic ID allocation are involved. Ordering
is explicit sibling `z`, then stable ID. Custom fragments are a restricted subset;
full SVG freedom remains available through Saga's existing custom SVG mode.

### What passed

Race-enabled Go tests and vet passed on macOS 15.7.4 arm64. Tests cover unchanged
connected-edge geometry after node movement, explicit group transforms, named
styles/wrapping, reused icon IDs, compact semantic reads, dependency/cascade
refusal, missing inputs, hash recovery preserving divergent bytes, injected
pre-publication failure, same-request retry, stale/colliding requests, and two
concurrent writers (one wins). Existing publication primitives communicate
post-publication durability failures; this spike does not fault-inject an OS crash.

A real temporary Saga fixture publishes the generated SVG with `apply-slide`,
then republishes the move/label/style revision. Exact Item ID, code-range digest,
criterion identity and story revision remain unchanged. Parsed XML selector
checks reject missing/duplicate IDs. A separate existing production issue was
reported to the parent: substring selector checks can mistake `data-item-id` for
an actual `id`; this experiment does not change production validation.

Two source branches editing different elements merge cleanly with Git's text
merge and the semantic merge. Same-element changes conflict. A branch removing a
node and another adding an incident edge is rejected semantically. This does not
prove arbitrary independent edits are visually compatible: validate, render and
inspect resolved source before publication. Merge the source export, regenerate
the visual, and republish; do not hand-merge `current.json` receipt/hash bookkeeping.

Chromium 145.0.7632.6 rendered eight views with networking blocked: before, moved,
final, and direct-SVG final at 1280×720 and 1024×576. Bounds, duplicate-ID and font
checks passed; the images were visually inspected. Direct-SVG and API final PNGs
were byte-identical at both sizes. This is standalone SVG QA, not production
review-surface QA. Windows amd64 cross-compilation passed without CGO; Windows
runtime, OS crash recovery and cross-browser font metrics remain untested.

### Measured workflow

[Traffic](../experiments/diagram-api/evidence/traffic.json) counts actual UTF-8
argv (space-separated), stdin, stdout and stderr bytes, including absolute paths.
These are CLI interaction measurements, not this conversation's tokens, hidden
reasoning, or build/download traffic. No tokenizer was run. The baseline uses
the same initial artwork and equivalent literal XML edits, not independently
prompted SVG authorship. It excludes shared local font and license payloads;
we do not pretend an AI must type base64 fonts. Both final images are identical.

| Measured sequence | CLI calls | Request bytes | Response bytes |
| --- | ---: | ---: | ---: |
| Comparable compact sequence, including schema/asset discovery and initialization | 10 | 6,261 | 5,804 |
| Direct SVG creation/read/move/targeted read/edit/remove/read | 7 | 10,155 | 14,901 |
| All compact operations/queries, including failure, retry, checks and recovery | 15 | 8,449 | 7,134 |
| Full source export and two merge calls | 3 | 1,262 | 39,178 |
| Four complete SVG transfers (before, moved, after, rebuilt) | 4 | 992 | 843,160 |
| Entire instrumented prototype sequence | 22 | 10,703 | 889,472 |

The comparable API sequence totals 12,065 bytes against 25,056 for the controlled
SVG baseline, with more commands. This is evidence of smaller routine authoring
and reading exchanges on this diagram, not a general token-efficiency claim.
The final semantic description is 1,107 bytes. Its standalone SVG is 210,806 bytes,
mostly the embedded font; that artifact transfer is retained in the full totals.
The [comparison record](../experiments/diagram-api/evidence/comparison.json)
identifies the exact included calls. Full source branches are mechanically
prepared by the harness; their file bytes are outputs of source export, not
newly authored request text. Building the tool, reading this documentation and
writing the harness are not included as per-diagram discovery costs.

The run includes one rejected overlong-label edit followed by a valid edit,
an identical successful-batch retry, and hash-divergence recovery followed by
byte-identical re-render. Eight browser render passes are separately recorded
in [visual QA](../experiments/diagram-api/evidence/visual-qa.json). No screenshot
revision retries were needed in the recorded run. The raw baseline lacks
transaction/merge/recovery equivalents, so it is not compared to the full safety
sequence as though they did the same work.

### Excalidraw alternative and next decisions

Current official docs were re-read during this experiment. The
[skeleton API](https://docs.excalidraw.com/docs/@excalidraw/excalidraw/api/excalidraw-element-skeleton)
supports styled shapes, labels, explicit coordinates and arrow bindings; conversion
regenerates IDs unless `{regenerateIds:false}` is supplied. Label sizing and bound
arrows can introduce geometry behavior that this authoring contract deliberately
keeps explicit. An adapter must separate connection metadata from such bindings.
The [export API](https://docs.excalidraw.com/docs/@excalidraw/excalidraw/api/utils/export)
produces SVG DOM output; semantic element IDs surviving conversion must not be
assumed to become exact SVG selectors. Test a mapping at export before adoption.

Its [JSON format](https://docs.excalidraw.com/docs/codebase/json-schema) and
[integration guide](https://docs.excalidraw.com/docs/@excalidraw/excalidraw/integration)
remain useful prior art for scene elements and browser integration. An adapter
would need separately pinned package/font/runtime versions, deterministic seeds
and ordering, local assets for offline use, verified exported selectors, license
notices, dependency-size measurements and Windows execution. Those results and
adapter byte counts remain **unmeasured**; this document makes no superiority
claim against an executed Excalidraw backend.

Recommend continuing the native API experiment with two more hand-composed
technical diagrams (sequence/swimlane and mixed custom graphics), then measuring
independent AI authoring sessions. Retain exact authored geometry and compact
semantic reads. Improve shape/text affordances from those examples before
standardizing CLI flags. Resolve production selector validation separately.

Production adoption still requires decisions about source storage and merge
workflow, receipt/asset lifecycle, the supported SVG escape surface, font/script
coverage, asset pack size, and integration with existing `apply-slide` snapshots.
A production schema, editor, format migration or Excalidraw adapter should be
reviewed as its own change. This branch only adds the experiment and evidence.
