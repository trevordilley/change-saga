import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { runCLI, startSagaServer, stopSagaServer } from "../support/fixture-builder.js";
import { expect, test } from "../support/test.js";

test("code examples preserve source, load monospace, and remain Item targets @critical", async ({ page, sagaRepositories }) => {
  const cli = (...args: string[]) => {
    const result = runCLI(sagaRepositories, args);
    expect(result.status, result.stderr).toBe(0);
    return result.stdout;
  };
  cli("review", "create", "--id", "api-usage", "--base", "main", "--head", "feature/wave-one", "--title", "API examples", sagaRepositories.sagaRoot);
  const request = JSON.parse(readFileSync(new URL("../../docs/examples/api-code-slide.json", import.meta.url), "utf8"));
  request.diagram.elements[0].code += '\n// <script>alert("example")</script>';
  const path = join(sagaRepositories.root, "api-code-slide.json");
  writeFileSync(path, JSON.stringify(request));
  const result = JSON.parse(cli("apply-slide", "--from", path, "--json", sagaRepositories.sagaRoot));
  expect(cli("diagram", "describe", "--slide", result.target, sagaRepositories.sagaRoot)).toContain("client.orders.create");
  const server = await startSagaServer(sagaRepositories);
  try {
    await page.goto(`${server.baseURL}/reviews/api-usage`);
    const back = page.getByRole("button", { name: "Back", exact: true });
    if (await back.count()) await back.click();
    const slide = page.locator(`[data-deck-slide][data-slide-target="${result.target}"]`);
    await expect(slide).toBeVisible();
    const frame = await (await slide.locator("iframe[data-fragment-frame]").elementHandle())?.contentFrame();
    if (!frame) throw new Error("Missing code example frame");
    await expect(frame.locator('#invoke[data-diagram-kind="code"]')).toBeVisible();
    await expect(frame.locator('[data-code-line="2"]')).toHaveText('  customer: "cus_123",', { useInnerText: false });
    expect(await frame.locator('[data-code-line="2"]').textContent()).toBe('  customer: "cus_123",');
    await expect(frame.locator('[data-code-highlight]')).toHaveCount(2);
    await expect(frame.locator('script')).toHaveCount(0);
    await expect(frame.locator('[data-code-line="7"]')).toHaveText('// <script>alert("example")</script>');
    const metrics = await frame.evaluate(async () => {
      await document.fonts.load('18px "Change Saga Code"');
      const text = document.querySelector<SVGTextElement>('text.diagram-code')!;
      return { font: document.fonts.check('18px "Change Saga Code"'), family: getComputedStyle(text).fontFamily, width: text.getBBox().width };
    });
    expect(metrics.font).toBe(true);
    expect(metrics.family).toContain("Change Saga Code");
    expect(metrics.width).toBeLessThan(1088);
    const anchor = await slide.locator('[data-landmark-target][data-element-id="invoke"]').getAttribute('data-landmark-anchor');
    expect(anchor).toBeTruthy();
    await page.evaluate(id => { location.hash = id!; }, anchor);
    await expect(slide.locator("iframe[data-fragment-frame]")).toHaveAttribute("src", /#invoke$/);
    await page.screenshot({ path: test.info().outputPath("code-example-light.png") });
    await page.emulateMedia({ colorScheme: "dark" });
    await page.screenshot({ path: test.info().outputPath("code-example-dark.png") });
  } finally {
    await stopSagaServer(server);
  }
});
