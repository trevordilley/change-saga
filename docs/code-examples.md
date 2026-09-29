# Code examples in slides

Use a `code` diagram element to show an API call, library usage, command, or
small implementation example. It can sit beside diagrams and callouts on the
same slide. The source is displayed as text; it is never executed.

```json
{
  "id": "invoke",
  "kind": "code",
  "label": "Create an order",
  "language": "TypeScript",
  "code": "const order = await client.orders.create({\n  customer: \"cus_123\",\n  items: [{ sku: \"book\", quantity: 1 }],\n});",
  "x": 80,
  "y": 140,
  "width": 1120,
  "height": 400,
  "style": "code",
  "line_numbers": true,
  "highlight_lines": [2, 3]
}
```

`code` contains the literal source. `language` is an optional label; it does not
select an interpreter or apply syntax highlighting. `label` is an optional
title. Line numbers start at 1, and `highlight_lines` selects whole lines.

The bundled monospace font preserves indentation and blank lines. Tabs use
four-column stops. The renderer refuses source that does not fit the explicit
box, so examples cannot silently wrap, shrink, or disappear beyond its edge.
Use shorter examples, a larger box, or a custom style with a smaller font size.
The default `code` style uses 18px text and follows the light or dark theme.
Each element accepts up to 16,000 characters and 100 lines; slide dimensions
usually call for much shorter examples.

## Publish a slide

[api-code-slide.json](examples/api-code-slide.json) is a complete `apply-slide`
request using an illustrative order API. From the repository containing your
change, create a review and publish the example:

```sh
change-saga init
change-saga review create --id api-usage --base main change.saga
change-saga apply-slide --from /path/to/api-code-slide.json change.saga
change-saga open --against main change.saga
```

Use your own review ID and API in the request. A code example is a semantic
visual element. Select it with an Item of kind `example`, then link that Item
to the implementation using the usual coverage commands. Showing source in a
slide does not automatically count as evidence or code coverage.

## Edit or inspect an example

The existing APIs work with code elements:

- `apply-slide` publishes the complete slide and its Items.
- `diagram edit` updates `code`, `language`, `line_numbers`, or `highlight_lines`
  through an `update` operation without changing the element's identity.
- `diagram describe` includes the original source in text and JSON output.
- `diagram check` checks that the SVG still matches its source.
- `change-saga spec --json` describes the code element's fields and limits.

Keep product stories in the feature's Product records. Use code examples to
explain how callers use the API and link to those stories where relevant.

The generated SVG remains a standard image, but older CLI versions may reject
the new element kind or fields when loading its structured source. Upgrade
readers and authoring tools before using code elements.
