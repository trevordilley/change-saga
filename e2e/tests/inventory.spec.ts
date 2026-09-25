import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga } from "../support/test.js";
import { runCLI } from "../support/fixture-builder.js";

test("shared System opens saved Components and code without losing a slide @critical", async ({ page, saga, browser }) => {
  const run = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, result.stdout + result.stderr).toBe(0);
  };
  const prefix = "urn:change-saga:wave-one:";
  const code = [{ commit: "HEAD", path: "src/app.go", start: 3, end: 3, note: "Exact implementation line used by this disposable definition." }];
  const definition = (id: string, extra = {}) => {
    const path = join(saga.root, id + ".json");
    writeFileSync(path, JSON.stringify({ name: id, explanation: "A shared implementation unit with exact code and a stable identity.", code, ...extra }));
    return path;
  };
  for (const id of ["FlagClient", "FlagStore"]) run("component", "add", "--id", id, "--from", definition(id), "--repo", saga.sourceRepo, saga.sagaRoot);
  const pins = ["FlagClient", "FlagStore"].map(id => ({ target: prefix + "component:" + id, revision: prefix + "component:" + id + ":revision:r1" }));
  const target = prefix + "system:FeatureFlag";
  const path = definition("FeatureFlag", { components: pins, interactions: [{ id: "read", from: pins[0].target, to: pins[1].target, description: "Read a flag value from the shared store.", code }] });
  run("system", "add", "--id", "FeatureFlag", "--from", path, "--repo", saga.sourceRepo, saga.sagaRoot);
  run("add-deck", "--feature", "wave-one", "--id", "flag-contexts", "--objective", "Two contexts share one flag System", saga.sagaRoot, "flags");
  for (const id of ["checkout-flag", "search-flag"]) {
    run("add-slide", "--deck", "flag-contexts", "--id", id, "--title", id, "--intent", "explain", "--layout", "diagram", saga.sagaRoot, id);
    run("add-item", "--slide", id, "--id", "shared", "--kind", "node", "--element-id", "slide-title", "--description", "This feature uses the shared flag decision.", "--documentation", target, "--documentation-revision", target + ":revision:r1", saga.sagaRoot);
  }
  const inventoryRequests: string[] = [];
  page.on("request", request => { if (request.url().includes("/api/documentation")) inventoryRequests.push(request.url()); });
  await page.reload(); await waitForSettledSaga(page);
  expect(inventoryRequests).toHaveLength(0);
  for (const id of ["checkout-flag", "search-flag"]) {
    await page.getByRole("button", { name: "Show slide: " + id, exact: true }).click();
    const slide = page.locator('[data-deck-slide]:visible');
    await slide.locator('.landmark-menu summary').click();
    const open = slide.locator('.landmark-menu [data-documentation-target]');
    await open.focus(); await page.keyboard.press("Enter");
    const hash = new URL(page.url()).hash;
    await expect(page.locator('[data-documentation-view]')).toHaveAttribute('data-documentation-view', target);
    await expect(page.locator('.drawer-body')).toContainText('Read a flag value');
    await page.getByRole('button', { name: 'FlagStore', exact: true }).click();
    await expect(page.locator('[data-documentation-view]')).toHaveAttribute('data-documentation-view', pins[1].target);
    await expect(page.locator('.drawer-body .term-code')).toBeVisible();
    await page.getByRole('button', { name: 'Back to previous explanation' }).click();
    await expect(page.locator('[data-documentation-view]')).toHaveAttribute('data-documentation-view', target);
    await page.keyboard.press('ArrowRight'); expect(new URL(page.url()).hash).toBe(hash);
    await page.keyboard.press('Escape');
    await expect(page.locator('.diff-drawer')).not.toHaveClass(/open/);
    await expect(open).toBeFocused();
    expect(new URL(page.url()).hash).toBe(hash);
  }
  // A revised definition warns on the old slide and still opens its saved pin.
  run("system", "revise", "--id", "FeatureFlag", "--revision", "r2", "--parent", target + ":revision:r1", "--from", path, "--repo", saga.sourceRepo, saga.sagaRoot);
  await page.locator('[data-deck-slide]:visible .landmark-menu [data-documentation-target]').click();
  await expect(page.locator('[data-documentation-status]')).toHaveText(/stale/);
  await expect(page.locator('[data-documentation-view]')).toHaveAttribute('data-documentation-pin', target + ':revision:r1');
  await page.keyboard.press('Escape');
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
  const narrow = await context.newPage();
  await narrow.goto(page.url()); await waitForSettledSaga(narrow);
  await narrow.locator('[data-deck-slide]:visible .landmark-menu summary').tap();
  await narrow.locator('[data-deck-slide]:visible .landmark-menu [data-documentation-target]').tap();
  await expect(narrow.locator('[data-documentation-view]')).toHaveAttribute('data-documentation-view', target);
  await narrow.getByRole('button', { name: 'FlagClient', exact: true }).tap();
  await expect(narrow.locator('.drawer-body .term-code')).toBeVisible();
  expect(await narrow.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await narrow.locator('button[data-close-drawer]').tap();
  await expect(narrow.locator('.diff-drawer')).not.toHaveClass(/open/);
  await context.close();
});
