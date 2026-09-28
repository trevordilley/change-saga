# Theming the reviewer and its slides

A Saga may carry one theme: `theme.css` at its root, next to `saga.json`. It
recolours the reviewer and every slide it serves by overriding design tokens,
and nothing else.

```css
:root {
  --accent: #7c3aed;
  --ui: "Inter", system-ui, sans-serif;
}
:root[data-theme="dark"] {
  --accent: #a78bfa;
}
```

- Only two blocks: `:root { }` for light mode and `:root[data-theme="dark"] { }`
  for dark mode. A token that is the same in both modes (fonts, sizes,
  radius) takes its `:root` value in dark mode too.
- Only tokens from the contract: `change-saga spec --json` lists each under
  `theme` with its group, kind, and light and dark defaults.
- Values by kind: colours as hex, `rgb()`, `rgba()`, `hsl()`, `hsla()`, or a
  named colour; lengths as `0` or px, rem, or em; fonts as a family list;
  shadows as `[inset] x y [blur [spread]] colour`, comma-separated.
- Refused outright, with the line: other selectors, `url()`, `@import` and
  every other @-rule, backslash escapes, `!important`, and `<`. A theme can
  never restyle or hide review controls or load anything.

The reviewer serves the validated values re-serialized, never the file's
text, after its own defaults. Served SVG and HTML slides get every token
declared with the theme applied, so a hand-authored slide that paints with
`var(--token)` follows the theme; committed slide bytes, digests, and
approvals do not change. An invalid theme is not applied at all.

## Commands

1. `change-saga theme init change.saga` writes a starter `theme.css` listing
   every token with its light and dark defaults, grouped and commented out.
   It refuses to replace an existing theme; `--force` replaces it.
2. Uncomment and change only the tokens the user asked to change.
3. `change-saga theme check change.saga` validates the file, naming each
   problem's line, then measures WCAG AA contrast (4.5:1) in light and dark
   mode for ink and muted text on the background, the primary button, each
   diagram style's text on its fill, and each palette ink on its sticky and
   text on its accent. It
   exits 1 when the theme is invalid and 3 naming each failing pair. Fix a
   failing pair rather than accepting it.
4. `change-saga theme preview change.saga` opens the reviewer's `/theme`
   page: swatches, the reviewer's chrome, and a diagram in every style and
   palette colour, light and dark side by side. `--no-open` prints the URL.
5. `change-saga validate change.saga` also reports an invalid theme.
