# Diagram authoring API exploration

Status: design exploration, 2026-09-24. Incorporates the author's requested
imperative drawing model and selection of Lucide as the default icon family.
Commands and record shapes below are proposals, not shipped CLI contracts.
No production format migration, editor replacement, or existing asset conversion
is proposed by this document. The prototype and measurements remain to be done.

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
| [Cogent SVG](https://pkg.go.dev/cogentcore.org/core/svg) | Editable SVG tree, XML I/O, geometry and transform APIs | First full-featured Go candidate to test; arbitrary SVG preservation and dependency footprint remain unverified |
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

## Next experiment and adoption decisions

Keep the next implementation isolated. On one explicitly authored technical
diagram, exercise icon placement/reuse, paths, a group, a free text label, custom
SVG, targeted reads, label/style edits, movement, removal, and an atomic batch.
Test that moving a node leaves the connected edge path unchanged and that a
group transform moves only its children. Verify exact Item/code/story selectors.

Test hash mismatch/rebuild, retry and publication failure behavior, missing
assets, repeated icon IDs, different-element text/semantic merges, and
same-element conflict detection. Visually inspect standard slide sizes. Test
Go candidates for custom SVG preservation and font behavior before choosing one.
Windows compatibility is a design requirement, not an executed validation claim.

Measure the entire comparable workflow against hand-authored SVG: requests,
responses, schema and asset discovery, follow-up edits, and rendering retries.
Report bytes and command counts; report tokens only with an identified tokenizer
and model. Asset paths should never enter routine AI responses. No efficiency,
visual-quality, merge-safety, or cross-platform result has yet been measured.

Production source schema, CLI registration, storage integration, selected backend,
asset pack size, and any editor integration remain separate adoption decisions.
The present change records the design and Lucide choice only.
