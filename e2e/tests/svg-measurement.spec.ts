import { expect, test } from "../support/test.js";

test("SVG measurement removes root handlers and links while preserving hotspot geometry", async ({ page, saga }) => {
  // Inspect the real clone before it enters its closed measurement shadow root.
  await page.addInitScript(() => {
    const importNode = Document.prototype.importNode;
    Document.prototype.importNode = function <T extends Node>(node: T, deep?: boolean): T {
      if (node instanceof SVGSVGElement) {
        document.documentElement.dataset.measuredSvgInput = node.getAttribute("onload") || "";
      }
      return importNode.call(this, node, deep) as T;
    };
    const append = ShadowRoot.prototype.append;
    ShadowRoot.prototype.append = function (...nodes: (Node | string)[]) {
      for (const node of nodes) {
        if (node instanceof SVGSVGElement) {
          document.documentElement.dataset.measuredSvg = JSON.stringify({
            onload: node.getAttribute("onload"), href: node.getAttribute("href"),
            viewBox: node.getAttribute("viewBox"),
            hasTarget: Boolean(node.querySelector("#render-boundary"))
          });
        }
      }
      return append.apply(this, nodes);
    };
  });
  await page.route("**/f/diagram/**", async route => {
    const response = await route.fetch();
    const body = (await response.text()).replace("<svg ", '<svg onload="void 0" href="https://example.invalid/measurement" ');
    await route.fulfill({ response, body });
  });
  await page.goto(saga.featureURL);
  const hotspot = page.locator('[data-fragment-title="Architecture Diagram"] [data-auto-landmark-hotspot="true"][data-element-id="render-boundary"]');
  await hotspot.scrollIntoViewIfNeeded();
  await expect(hotspot).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-measured-svg-input", "void 0");
  const measured = await page.evaluate(() => JSON.parse(document.documentElement.dataset.measuredSvg || "null"));
  expect(measured).toEqual({ onload: null, href: null, viewBox: "0 0 240 80", hasTarget: true });
});
