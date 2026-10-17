# Architecture authoring: verified feedback

Synaptic's Architecture board motivated the updated
[authoring workflow](../skills/change-saga/references/architecture.md): its public
description contains 260 semantic elements and seven top-level sections, starting
with a codebase map and continuing through composition, runtime, operation sharing,
state, concurrency and tenancy. This is a useful scale example, not a required
element count or an independent certification of that board's claims.

The following feedback was checked against v0.5.0 and the current source. The
guidance changes address authoring and discovery; they do not add CLI capabilities.

| Report | Verified behavior and disposition |
| --- | --- |
| Publishing `architecture` makes `assign-role --expect none` fail | Confirmed when no explicit presentation metadata overrides the conventional role. Reread `whiteboard list --json`; skip assignment when the board already owns it, otherwise use the observed owner. Documented in the skill. |
| Custom styles require every field, including `dash` | Five fields are required: `fill`, `stroke`, `ink`, `font_size`, `stroke_width`. `dash` is optional and boolean. A scratch publication without `dash` succeeded. Added a complete example to the skill and specification. |
| Edges cannot end on sections/groups | True for ordinary `edge`, whose endpoints must be nodes. Composed `connection` uses instance paths and accepts non-decorative targets with bounds. Documented the distinction. |
| The font rejects check and triangle glyphs | Reproduced U+2713 and U+25B7 rejection through public publication. Documented words, Lucide icons and terminators as alternatives; an authoritative character-set discovery command remains absent. |
| Whiteboard visual QA and publish dry-run are missing | Confirmed in the command flags. The documented workaround is scratch publication plus browser inspection of the opening and all focused sections in both themes. |
| Supporting pages inherit the main Essay's subtext | Not reproduced: a public-CLI scratch fixture with a unique main subtext showed no inheritance. An empty supporting-page subtext gets a generic Technical Design fallback. Supply a page-specific subtext. |
| Supporting pages do not appear in the navigation tree | Intentional: the sidebar contains four whiteboard roles; supporting pages are discoverable from the Technical Design essay and direct URLs. Documented without changing the navigation model. |
| There is no reader stop command | `serve stop SAGA` stops the managed reader launched by `open`; verified by starting and stopping only a scratch reader. Documented alongside the open workflow. |
| Relative code links in Essays are dead | The shared Markdown renderer does not translate filesystem paths into source-viewer routes. Use verified repository web permalinks for readable citations and supported landmarks/Items for tracked evidence. A dedicated Essay source-citation syntax remains absent. |

## Remaining product work

- Add a whiteboard selector to visual QA, including focused sections and both
  themes. Until then, slide QA must not be reported as whiteboard verification.
- Add publication preflight/dry-run using the same validation and rendering path,
  with no revision writes. Keep immutable-publication and parent checks intact.
- Expose complete style fields and supported glyph discovery through machine-
  readable contract/help, with field-specific diagnostics for malformed styles.
- Consider an explicit source-citation mechanism for Essays that retains exact
  revision identity and works when code is in a separate checkout. Ordinary
  Markdown links must not silently claim code coverage.
- Improve supporting-page subtitle presentation when empty. This is a shared
  generic fallback, not metadata copied from the main Essay.

These are follow-ups, not capabilities shipped by the guidance update. Any
implementation should reuse the existing renderer, font and evidence contracts.

## Implementation references

- [Whiteboard CLI](../internal/cli/diagram_whiteboard.go) and
  [role defaults and metadata](../internal/requirements/whiteboard_metadata.go).
- [Diagram schema](../schema/v5/diagram.schema.json),
  [node/edge validation](../internal/diagram/model.go),
  [composition endpoints](../internal/diagram/composition.go), and
  [font measurement](../internal/diagram/render.go).
- [Visual QA selectors](../internal/cli/visual_qa.go),
  [reader dispatch](../internal/cli/cli.go), and
  [managed runtime](../internal/cli/server_runtime.go).
- [Essay authoring](../internal/cli/overview.go),
  [Technical Design presentation](../internal/server/technical.go), and
  [shared Markdown](../internal/server/markdown.go).
