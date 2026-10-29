# UI references (proposal)

Status: proposal, not implemented.

A feature now shows Stories, Prototypes, UX, UI, Technical, then Quality. Every
section except UI has a resource behind it. The `ui` coverage axis exists, but
the only way to resolve it today is a `ui` coverage exception. This proposal
adds the resource that fills the UI section and covers the axis.

## What a UI reference is

A UI reference is the visual spec for a screen or component state: what it
must look like, as opposed to how it behaves. Each kind of design artifact has
its own job:

| Section | Artifact | Answers |
| --- | --- | --- |
| Prototypes | interactive HTML or an allowed embed | Does the interaction work? |
| UX | `ux`-role deck | What is the flow, step by step? |
| UI | UI reference | What exactly should this screen look like? |
| Technical | implementation deck, design chapters, whiteboards | How is it built? |

A UI reference is not a prototype with a flag on it. It covers a different
axis, it is static, and a reviewer compares it pixel-for-pixel against the
built UI instead of clicking through it.

## Shape

It follows the prototype model, so authors, the loader and coverage reuse
patterns they already know:

```text
___features/<feature>.feature/___requirements/
  prototypes/
  ui/<id>.ui/
    ui.json                     # identity: id, created_at
    revisions/<revision>.json   # immutable snapshot with parents
    assets/<digest>.<ext>       # content-addressed images
    annotations/<id>.json       # pins a region to a story or criterion
```

URN: `urn:change-saga:<saga>:ui:<id>`, with `:revision:<id>` and
`:annotation:<id>` children.

A revision is a complete snapshot:

```json
{
  "$schema": "https://changesaga.dev/schema/v5/ui-revision.schema.json",
  "version": 5,
  "id": "r2",
  "ui": "checkout-summary",
  "parents": ["urn:change-saga:shop:ui:checkout-summary:revision:r1"],
  "title": "Checkout summary",
  "state": "active",
  "source": { "kind": "image" },
  "variants": [
    { "id": "desktop-light", "viewport": "desktop", "theme": "light", "asset": "assets/sha256-….png", "digest": "sha256:…", "alt": "Order summary with line items, totals and a Pay button" },
    { "id": "mobile-dark", "viewport": "mobile", "theme": "dark", "asset": "assets/sha256-….png", "digest": "sha256:…", "alt": "…" }
  ],
  "created_at": "2026-10-28T00:00:00Z"
}
```

- `source.kind` is `image` (assets stored in the Saga) or `external` (a design
  tool URL, such as Figma, with the same `ProviderAllowlist` embed rules as
  prototypes).
- An `external` revision still needs at least one snapshot variant. Embeds
  break and change underneath you, and agents cannot read them. The snapshot
  is what gets pinned, what offline reviewers see, and what an agent reads.
- `variants` cover the viewport and theme matrix (light and dark, mobile and
  desktop) without splitting one screen across several resources.
- `alt` is required. It is what makes a UI reference readable by an agent
  that cannot see the image.

An annotation pins one region of one variant to a story or criterion:

```json
{
  "id": "pay-disabled",
  "ui": "urn:change-saga:shop:ui:checkout-summary",
  "ui_revision": "urn:change-saga:shop:ui:checkout-summary:revision:r2",
  "variant": "desktop-light",
  "region": { "x": 0.62, "y": 0.81, "width": 0.3, "height": 0.08 },
  "target": "urn:change-saga:shop:story:checkout:criterion:pay-disabled-until-valid",
  "story_revision": "urn:change-saga:shop:story:checkout:revision:r3",
  "rationale": "The Pay button stays disabled and muted until the form is valid."
}
```

Regions are fractions of the image, so they survive re-exports at a different
resolution.

## Coverage

The `ui` axis becomes: *a current UI annotation pinned to the criterion
(direct) or its story (broad)*. That is the same rule the `prototype` axis
already uses. It is stale when the pinned UI revision or story revision is no
longer current. The gate stays `design_ready`, and `ui` coverage exceptions
keep working for criteria with no visual surface.

This uses annotations rather than `addresses` relations because the useful
claim is "this region of this screen satisfies this criterion". A relation
from the whole resource would be a broad link and would carry no selector.

Following the incremental-adoption rule, the new axis is report-only. A Saga
with no UI references sees the same `ui` gaps it sees today and nothing new
blocks.

## Commands

```text
change-saga ui add-image    --feature ID --id ID --revision ID --title TEXT --variant ID=PATH[,viewport=V][,theme=T] --alt ID=TEXT ... <saga>
change-saga ui add-external --feature ID --id ID --revision ID --title TEXT --url URL [--embed-url URL --provider ID --embed-origin ORIGIN] --variant ... --alt ... <saga>
change-saga ui revise       --ui URN --revision ID [--title] [--variant ...] [--state retired] <saga>
change-saga ui annotate     --ui URN --id ID --variant ID --region X,Y,W,H --target URN --rationale TEXT <saga>
change-saga query ui        [--feature ID] [--target URN] <saga>
```

## Reviewer

- The UI section lists a feature's references. Each opens a page showing the
  variants side by side, with annotation hotspots that link to their criteria.
- A criterion page lists the UI regions pinned to it next to its prototype
  annotations and UX slides.
- The reviewer picks the variant that matches the active app theme by default.

## Open questions

1. **Storage location.** `___requirements/ui/` mirrors prototypes. The other
   option is `___design/ui/`, but `___design` is read today as technical
   chapters joined into the report tree, so putting UI there would need loader
   changes. I recommend `___requirements/ui/`.
2. **Design tokens.** Should a revision optionally record the token theme it
   was drawn against (see the design-tokens work), so a token change can flag
   UI references as possibly stale? I recommend deferring this.
3. **Built-UI comparison.** Linking a variant to a screenshot test (the Quality
   side) would let a run attach "built vs. spec" evidence. That is a natural
   follow-up but is out of scope here.
