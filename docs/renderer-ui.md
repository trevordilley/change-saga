# Renderer UI conventions

Status: contract, 2026-08-20
Scope: the renderer in `internal/server/`, including its template, navigation,
styles, and browser behavior.

`docs/ux-reframe.md` and `docs/review-experience-audit.md` record why the
reviewer was restructured. This file records the conventions the renderer holds
to now, so they are not undone by accident. The target is a quiet developer
tool: a reviewer should recognise it from their editor and their code host, not
from a marketing page or a report generator.

## Vocabulary boundary

Reviewer-facing chrome speaks the reviewer's language: chapters, files, changes,
comments, explanations. Storage and format vocabulary — fragment, section, media
type, schema version, diff URI, manifest, internal paths — belongs to the CLI,
the validator, and the spec.

Authored saga content is rendered verbatim and may discuss the format freely.
Never edit committed saga content to hide a word from the chrome.

## Typography and surface

- System UI at 13px for chrome; monospace only for code and code-shaped
  metadata: paths, line numbers, change counts, identifiers.
- Prose stays in the UI font even inside a code surface. The fragment excerpt in
  the explanations panel is prose; the chapter eyebrow above it is metadata.
- Hairline separators instead of cards. No decorative rounding, shadows, or
  oversized headings on ordinary content.
- Most controls stay invisible until the reviewer hovers or focuses the thing
  they belong to. Info buttons fade in on element hover or keyboard focus; surprise buttons
  stay visible. Touch readers see both controls without hover. Content is central and stays central. Slide Items remain reachable
  without hover through the marked-places menu, stable permalink, and keyboard
  focus; their on-slide affordances use the same quiet treatment in
  implementation and pull-request decks.

## Icons

`icons.go` ships every glyph as an inline SVG `<symbol>` sprite. The set is
original to this repository and inherits its MIT licence, so a committed saga
can be reviewed with no network access and no third-party icon licence.

Icons are always decorative (`aria-hidden`). The accessible name comes from the
owning control's `aria-label` or visible text, and every icon-only control also
carries a `title` so the pointer user gets the same word. Do not add a remote
font or icon CDN.

`fileIcon` maps a repository path to a file-type badge. Unknown types fall back
to a neutral document outline rather than guessing.

## Navigation

V4 is a native presentation. A thumbnail rail groups authored slides by deck;
Previous/Next and unmodified arrow or Page keys move through the sequence. The
current deck, slide title, and position stay visible so reviewers can orient and
resume. Fullscreen presentation hides application and review chrome without
changing the active slide.

Overview, Front, Back, slide position, and Viewed controls occupy a separate
bar below the slide canvas. The bar can wrap on narrow screens without
covering authored titles or diagrams; the visual retains its 16:9 aspect ratio.
Presentation mode hides the bar and gives its space back to the slide.

A pull-request review uses this same native presentation as its whole default
surface: thumbnail rail plus one maximally fitted 16:9 slide. It is not a
document page containing a slide, and it does not require a presentation-mode
action before the slide fills the available pane. Optional Present removes the
chrome; it does not switch the reviewer into the deck.

Previous/Next and unmodified arrow or Page keys follow authored order. The URL
hash owns the active slide or Item, including after a mutation redirect. With no
hash it starts at the authored first slide: a recorded decision is not treated
as completion or used to invent a resume verdict. Exact slide Items project
interactive regions over the visual; their ordinary landmark affordances open
linked diffs and affected living-Saga records in the existing side drawer. A
review Item differs from an implementation Item only in what linked code means:
it opens the exact base/head diff. Code Diff and Coverage remain secondary tabs
and never displace the deck on entry.

V2/v3 remain legacy reports with their documentation tree and collapsible
chapters. The renderer must never reinterpret their fragments as slides or
manufacture deck-break slides. A report becomes a presentation only through an
explicit semantic rewrite to v4.

The Code Diff sidebar is a different thing and should stay that way: a compact,
filterable changed-file tree with counts, status, and a selected file.

Both navigation surfaces are built from manifests rather than incidental
rendered state, so every destination remains addressable before its content is
active.

Technical-deck and review Items may also pin reusable Component or System
definitions. Their book control and linked node open the saved definition in
the same drawer, with directed Component interactions and exact code. Nested
definitions have a back control; Escape restores the original Item control and
keeps the slide hash/position. Display stale, retired, conflicted, or missing
pins honestly. Neither navigation nor a newer definition repins the Item or
changes a decision. Inventory and interaction code are fetched on demand, never
embedded as a graph in every review shell.

## Deck overview, Front, and Back

A deck has three explicit reading surfaces: Overview, Front, and Back. Back is
always the default and preserves the existing visual asset or entrypoint,
Items, and linked evidence. Front shows the slide's authored summary bullets;
older slides fall back to their takeaway, then Item labels. Changing faces
never changes a slide or Item identity, records a decision, or marks it Viewed.
Previous/Next still follows authored slide order. Overview is not a slide and
never participates in slide position, Viewed totals, or code coverage.

An authored Overview is a Markdown report with the existing table and visual
support. Its inline `[label](annotation:ID)` citations resolve to named slides and
optional Items in that same deck. An accessible reference list exposes the same destinations.
Opening a citation uses the existing slide/Item route and evidence drawer;
it does not copy or invent stories, code, or diffs. An older deck with no
authored overview gets an explicitly labeled generated directory. Missing
authored overview and uncovered slides are compatibility warnings; broken or
cross-deck references are errors. Optional fields remain compatible with older
tolerant readers; readers that reject unknown manifest fields require an
upgrade before they can read decks with Front or Overview content.

Feature navigation calls the explanatory deck **Technical**. The older
**Design > Technical** chapter group is **Design > Architecture**, while the
application-wide **Technical design** inventory keeps its existing name.
These are chrome changes only: routes, anchors, navigation keys, stored roles,
and authored titles retain their stable identities.
Opening a Technical Overview expands its owning feature, Technical section,
and deck, and selects that Overview instead of the application Overview.
The URL's slide or Overview anchor refines the server's page-level selection;
reloads, sidebar links, face changes, and browser history keep them in sync.

Comparison state must never dim authored content. Quiet controls can reveal on
hover or keyboard focus, but an unchanged slide, diagram, or report remains
fully legible. Comparison indicators carry state without lowering the opacity
of the content they describe. The SVG diagram reveal animation and its iframe
hash are separate from comparison styling; animation completion must leave
the drawing at full opacity and reduced-motion readers see the full drawing.
Authored entrance reveal remains allowed; leaving the slide with the pointer
never fades its content.

## Explicit Viewed state

Viewed is a manual reading aid, separate from approvals, comments, and code
coverage. The checkbox works immediately for the labeled **Local reader**, a
stable browser-generated profile. The reviewer can optionally switch to a
named local profile or back to Local reader. Visiting, hovering, focusing,
flipping, opening evidence, navigating, or approving never marks a slide.
Only checking or unchecking Viewed writes a mark. The visible per-deck count
and thumbnail label reflect that selected profile and stay independent from
review decisions; the content itself does not fade.

Marks use localStorage keys containing the Saga ID, local profile/name, stable
deck target, and stable slide target. Titles, positions, routes, and current
approval state are not keys. Reload and navigation preserve marks; switching
profiles isolates them. Local names are case-sensitive labels, not authenticated
accounts. Persistence belongs to this browser profile and origin, including the
server port; changing browser, host, or port does not carry marks over. The
local-only help states this limitation. With storage blocked, the control works
in memory and visibly says its marks last only on the current page.

The review shell currently uses its existing server-side human/Git author for
review events and has no browser-selectable account identity. A local Viewed
profile therefore does not change the author of any approval or comment. Viewed
marks are not Saga review events, are not committed to Git, and are not shared
with other reviewers. They provide no completion verdict.

## The page is a shell

For v4, `GET /` renders the deck and slide manifests, thumbnail navigator, and
one active visual review surface. Deep links select the owning slide and Item;
the visual asset is served through its stable slide target. V2/v3 keep the
bounded legacy shell: chapter bodies arrive from `/api/section`, fragments from
`/api/fragment`, and `/api/locate` resolves anchors in unopened chapters.

Three rules follow from that, and breaking any of them is a regression:

- A descriptor is still a destination. It carries the explanation's id, title,
  and target, so permalinks and the review progress map work before the content
  arrives. Its compact decision controls stay on the explanation bar; the
  chapter review directory mirrors the same target as an overview, and both
  views synchronize after one persisted event.
- Content arriving never overwrites what the reviewer has already done. A
  decision made on a descriptor moves into the rendered explanation rather than
  being replaced by the state its snapshot was built from.
- Anything that needs content must ask for it first. Arming an annotation tool
  on an explanation that has not arrived fetches it and waits, rather than
  silently disarming.
- Content is filled into the article it was described by, never swapped for a
  new one. A reviewer can be part way through clicking a descriptor's controls
  when its content lands, and replacing the element under the pointer loses
  that click.

`<body data-shell-ready>` is set once the first fill-in has finished: every
explanation on screen has arrived and any anchor in the URL has been resolved.
It is the page saying it has settled, which is what a reviewer can see and what
the browser suite waits for instead of guessing.

## Review controls

In v5 the complete slide is the approval target. Items provide precise comments,
annotations, and evidence links without becoming a second approval checklist.
Deck and Saga status are derived rollups. V2/v3 retain section/fragment controls
and the chapter review directory in their legacy report reader.

In a pull-request review, slide decisions and slide discussion live in a quiet
overlay; Item discussion lives with that Item's evidence drawer. New comment,
reply, and decision composers are not permanently open. Replies use the
existing append-only thread target; revealing or navigating to a slide never
records a decision. Escape closes an open composer without submitting it and
restores its summary control. The annotation button opens the compact released
palette beside the active slide's action strip: Select, Comment, Highlight,
Rectangle, Freehand, Sticky note, color, undo, redo, and selection deletion.
The palette starts closed and belongs to the slide, not to an Item. Item
hotspots continue to open their exact diff and affected record independently.

In the legacy chapter directory, each row mirrors the decision control on its target's own bar and projects
append-only approval events into exactly `Unreviewed`, `Approved`, or `Changes requested`.
Its discussion count is a separate signal: it combines non-withdrawn comment
and annotation threads, including threads on an explanation's landmarks, but
never changes the approval state. Row links use the ordinary anchor resolver,
so they fetch the chapter or explanation if needed before moving to the exact
destination.

Chapter-level approval records remain valid and appear on the chapter bar, but
container decisions still create no directory row, progress segment, or
completion credit. No data migration is required. Existing `open` and `closed`
events on approval-bearing descendants project to
`Unreviewed`; new UI decisions continue to append the storage-compatible
`approved`, `rejected`, and undo (`open`) events.

## Comments and their marks

A review comment may carry an anchor. When that anchor is a mark drawn on the content —
a rectangle, a freehand drawing, a highlight, or a sticky note — the comment
belongs to the mark and renders as a compact bubble pinned to it, revealed on
hover or focus of either the mark or the bubble. Every other anchor — a whole
fragment, a section, a chapter, a diff line — keeps its comment in the list
below the content. `ReviewComment.Anchor` stores normalized slide geometry;
`AnnotationAction` records create, update, and delete events. The root comment
creates the annotation. Every move, color change, undo, redo, or delete is a
reply event, so the original anchor, author, and discussion are never rewritten.
Update and delete events reply directly to that creation root; accepting them
on an ordinary comment would create valid-looking history that the annotation
projection cannot show. A merged review renders the projected marks read-only
and never offers its editing toolbar.

`GET /reviews/{id}/annotations` projects the latest visible anchor from that
append-only thread. `prepareReviewAnnotations` in `appjs.go` mounts it over the
active slide and derives both the mark and bubble location from the same
normalized geometry. Highlights store their region just like other shapes,
which keeps hit testing and bubble placement stable across deck fitting and
narrow-screen layout changes.

A revealed comment never buries the mark it describes, and arming a drawing tool
closes every open bubble so the content keeps the pointer.

## Diagnostics, not congratulation

Complete coverage is an invariant enforced by the validator, not an achievement.
The renderer shows no score, percentage, progress bar, or success banner. Only
two things are worth a reviewer's attention: changes that are still unexplained,
and committed references that have gone stale.

## Syntax highlighting

The highlighter in `appjs.go` is deliberately small and applies to code only.
Markdown, plain text, and licence files resolve to the `prose` language and are
left untouched — colouring capitalised words inside a sentence is noise, not
information. Add new code languages to `languageKeywords` and `languageForPath`
together; add new prose extensions to the `prose` branch.

## Dependencies

No frontend framework, no bundler, no CDN. All CSS, JavaScript, and assets are
served from the binary so that reviewing a committed saga works offline.

## Comments on review code

Within a review, hover or keyboard-focus a diff line's gutter and use **+**
to comment. This works in an Item's linked-code drawer and in **Code Diff**.
Shift-click another line on the same side to select a range. Deleted lines
retain their old-side identity. Escape closes a new composer and returns focus
to its gutter; Ctrl/Cmd+Enter submits it.

Threads appear below their line and support replies, Resolve, and Reopen.
They persist in the review's append-only records. Changed code is marked
outdated and includes its original snippet; comments outside the visible lines
remain listed below the diff. Merged reviews remain read-only.

The composer carries the exact base and head used to render its diff. If the
branch changes before controls load or before submission, the browser asks for
a reload and retains any draft. An uncertain save retains the draft and blocks
that composer's resubmission so a lost receipt cannot cause a duplicate click.

## Element detail and surprises

Moving the pointer across a slide element must not open a popover. Explicit
notes use a small info button at the element's bottom-left. It fades in when
the element is hovered or keyboard-focused and stays visible while its detail
is open. Touch readers see it without hover; reduced motion disables the fade. Notes add grounded detail beyond the visible text;
ordinary Item labels and descriptions alone create no info control. Keep the
system explanation visible on detailed slides, with optional depth in notes.

Surprises use always-visible compact warning buttons in the same corner. They reveal the
callout's explanation and access to its linked code on demand. Multiple controls
on one element sit alongside each other. Do not cover the diagram with an
expanded Surprises panel. Escape dismisses the popover; linked code retains
its existing drawer behavior.
