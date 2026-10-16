import { expectNoSeriousAccessibilityViolations, expect, test, waitForSettledSaga } from "../support/test.js";
import { runCLI, startSagaServer, stopSagaServer } from "../support/fixture-builder.js";

test("product context and test walkthroughs remain readable across themes and narrow screens", async ({ page, sagaRepositories: repo }, testInfo) => {
  const cli = (...args: string[]) => {
    const result = runCLI(repo, args);
    expect(result.status, `${result.stdout}\n${result.stderr}`).toBe(0);
  };
  cli("feature-theme", "add", "--id", "departure", "--title", "Confident departures", "--text", "## Why this matters\n\nCrews need **timely information** before leaving harbor.\n\n## Shared purpose\n\nBring tide conditions and observations together so a planner can understand uncertainty.", "--feature", "tide-charts", repo.sagaRoot);
  cli("quality", "test-case", "add", "--feature", "tide-charts", "--id", "cached-forecast", "--title", "Read the last known forecast", "--kind", "edge", "--automation", "manual", "--precondition", "A previous tide forecast has been saved and the latest update is unavailable.", "--step", JSON.stringify({ id: "open", action: "Open the tide chart for the departure harbor.", expected_result: "The last known forecast is shown with its original observation time." }), "--step", JSON.stringify({ id: "inspect", action: "Read the freshness explanation beside the forecast.", expected_result: "The explanation makes clear that conditions may have changed." }), "--expected-result", "The planner can understand how old the forecast is before choosing a departure time.", repo.sagaRoot);
  const server = await startSagaServer(repo);
  const routes = [
    ["theme", "/feature-themes/departure", "[data-feature-theme-page]"],
    ["persona", "/personas/skipper", "[data-persona-page]"],
    ["term", "/terms/greeting", ".term-page"],
    ["test", "/tests/cached-forecast", "[data-test-case-page]"]
  ];
  try {
    for (const [name, route, selector] of routes) {
      await page.goto(server.baseURL + route);
      await waitForSettledSaga(page);
      const surface = page.locator(selector);
      await expect(surface).toBeVisible();
      await expect(surface.locator(".reading-surface").first()).toBeVisible();
      if (name === "term") {
        const definition = await surface.locator(".term-definition").boundingBox();
        const metadata = await surface.locator("[data-term-semantics]").boundingBox();
        expect(definition!.y).toBeLessThan(metadata!.y);
      }
      if (name === "test") {
        await expect(surface.locator(".test-step-result")).toHaveCount(2);
        await expect(surface.locator(".test-outcome")).toContainText("how old the forecast is");
      }
      await expectNoSeriousAccessibilityViolations(page);
      await page.screenshot({ path: testInfo.outputPath(`${name}-desktop.png`), fullPage: true });
      await page.locator("[data-theme-toggle]").click();
      await page.screenshot({ path: testInfo.outputPath(`${name}-alternate-theme.png`), fullPage: true });
      await page.locator("[data-theme-toggle]").click();
      await page.setViewportSize({ width: 390, height: 844 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
      await page.screenshot({ path: testInfo.outputPath(`${name}-mobile.png`), fullPage: true });
      const disclosure = surface.locator("details.reading-details").first();
      if (await disclosure.count()) {
        await expect(disclosure).not.toHaveAttribute("open", "");
        await disclosure.locator("summary").focus();
        await page.keyboard.press("Enter");
        await expect(disclosure).toHaveAttribute("open", "");
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
      }
      await page.setViewportSize({ width: 1440, height: 1000 });
    }
    const noJS = await page.context().browser()!.newContext({ javaScriptEnabled: false });
    try {
      const native = await noJS.newPage();
      await native.goto(server.baseURL + "/tests/cached-forecast");
      await expect(native.locator(".test-outcome")).toBeVisible();
      await native.locator("[data-test-runs] > summary").click();
      await expect(native.locator("[data-test-runs] .term-empty")).toBeVisible();
    } finally { await noJS.close(); }
  } finally { await stopSagaServer(server); }
});
