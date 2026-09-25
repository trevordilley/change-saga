import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga } from "../support/test.js";
import { runCLI, type SagaFixture } from "../support/fixture-builder.js";

// Technical design: the overview's directory of Systems and Components, the
// canonical page of one exact revision, and the round trip from a slide Item
// to that definition, its usages, and back to the same slide.

const prefix = "urn:change-saga:wave-one:";
const systemTarget = prefix + "system:FeatureFlag";

function author(saga: SagaFixture) {
  const run = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, result.stdout + result.stderr).toBe(0);
  };
  const code = [{ commit: "HEAD", path: "src/app.go", start: 3, end: 3, note: "Exact implementation line used by this disposable definition." }];
  const definition = (id: string, extra = {}) => {
    const path = join(saga.root, id + ".json");
    writeFileSync(path, JSON.stringify({ name: id, explanation: "A shared implementation unit with exact code and a stable identity.", code, ...extra }));
    return path;
  };
  for (const id of ["FlagClient", "FlagStore"]) run("component", "add", "--id", id, "--from", definition(id), "--repo", saga.sourceRepo, saga.sagaRoot);
  const pins = ["FlagClient", "FlagStore"].map(id => ({ target: prefix + "component:" + id, revision: prefix + "component:" + id + ":revision:r1" }));
  const system = definition("FeatureFlag", { components: pins, interactions: [{ id: "read", from: pins[0].target, to: pins[1].target, description: "Read a flag value from the shared store.", code }] });
  run("system", "add", "--id", "FeatureFlag", "--from", system, "--repo", saga.sourceRepo, saga.sagaRoot);
  run("add-deck", "--feature", "wave-one", "--id", "flag-contexts", "--objective", "A slide uses the shared flag System", saga.sagaRoot, "flags");
  run("add-slide", "--deck", "flag-contexts", "--id", "checkout-flag", "--title", "checkout-flag", "--intent", "explain", "--layout", "diagram", saga.sagaRoot, "checkout-flag");
  run("add-item", "--slide", "checkout-flag", "--id", "shared", "--kind", "node", "--element-id", "slide-title", "--description", "Checkout reads the shared flag decision.", "--documentation", systemTarget, "--documentation-revision", systemTarget + ":revision:r1", saga.sagaRoot);
  return { run, system };
}

test("Technical design lists shared definitions and traces a slide Item to its pinned page and back @critical", async ({ page, saga }) => {
  const { run, system } = author(saga);
  await page.goto(saga.baseURL + "/");
  await waitForSettledSaga(page);
  const part = page.locator('[data-overview-part="Technical design"]');
  await expect(part).toContainText("1 System · 2 Components");
  await part.getByRole("link", { name: "Technical design" }).click();
  await expect(page).toHaveURL(/\/technical$/);
  const systems = page.locator('[data-technical-kind="system"]');
  const row = systems.locator('[data-directory-row="FeatureFlag"]');
  await expect(row).toContainText("unspecified");
  await expect(row).toContainText("active");
  await expect(row.locator("td").last()).toHaveText("1");
  await expect(page.locator("[data-technical-data-model]")).toContainText("No data entities");

  // Keyboard: the definition link is reachable and opens its canonical page.
  const link = row.getByRole("link", { name: "FeatureFlag" });
  await link.focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/technical\/system\/FeatureFlag$/);
  await expect(page.locator("[data-technical-entity]")).toHaveAttribute("data-technical-revision", "r1");
  await expect(page.locator(".documentation-explanation")).toContainText("Read a flag value");
  const usage = page.locator("[data-technical-usages] [data-usage-item]");
  await expect(usage).toHaveText("shared");

  // The usage opens the exact slide; its Item opens the same pinned definition.
  await usage.click();
  await waitForSettledSaga(page);
  const slide = page.locator("[data-deck-slide]:visible");
  await expect(slide).toHaveAttribute("data-slide-title", "checkout-flag");
  const slideURL = page.url();
  await slide.locator(".landmark-menu summary").click();
  const open = slide.locator(".landmark-menu [data-documentation-target]");
  await open.click();
  await expect(page.locator("[data-documentation-view]")).toHaveAttribute("data-documentation-pin", systemTarget + ":revision:r1");
  const usedBy = page.locator(".drawer-body details.documentation-usages");
  await usedBy.locator("summary").click();
  await expect(usedBy.locator("[data-usage-item]")).toHaveText("shared");
  await expect(page.locator(".drawer-body [data-documentation-page]")).toHaveAttribute("href", "/technical/system/FeatureFlag?revision=r1");
  await page.keyboard.press("Escape");
  await expect(page.locator(".diff-drawer")).not.toHaveClass(/open/);
  await expect(open).toBeFocused();
  expect(page.url()).toBe(slideURL);

  // A revised definition leaves the saved pin readable and says so.
  run("system", "revise", "--id", "FeatureFlag", "--revision", "r2", "--parent", systemTarget + ":revision:r1", "--from", system, "--repo", saga.sourceRepo, saga.sagaRoot);
  await page.goto(saga.baseURL + "/technical/system/FeatureFlag?revision=r1");
  await expect(page.locator("[data-technical-pin-status]")).toHaveAttribute("data-technical-pin-status", "stale");
  await expect(page.locator("[data-technical-usage]")).toHaveAttribute("data-usage-status", "stale");
  await page.getByRole("link", { name: "Read the current definition" }).click();
  await expect(page.locator("[data-technical-entity]")).toHaveAttribute("data-technical-revision", "r2");
  await page.reload();
  await expect(page.locator("[data-technical-entity]")).toHaveAttribute("data-technical-revision", "r2");
  await page.goto(saga.baseURL + "/technical");
  await expect(page.locator('[data-directory-row="FeatureFlag"]')).toContainText("1 not current");

  // Members open their own pinned pages.
  await page.goto(saga.baseURL + "/technical/system/FeatureFlag?revision=r1");
  await page.locator(".documentation-members").getByRole("link", { name: "FlagStore" }).click();
  await expect(page).toHaveURL(/\/technical\/component\/FlagStore\?revision=r1$/);
});

test("Technical design stays readable on a 390px touch screen", async ({ browser, saga }) => {
  author(saga);
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
  const narrow = await context.newPage();
  await narrow.goto(saga.baseURL + "/technical");
  await waitForSettledSaga(narrow);
  // A mobile browser widens its layout viewport to fit wide content, so
  // compare against the device width rather than innerWidth.
  expect(await narrow.evaluate(() => [window.innerWidth, document.documentElement.scrollWidth])).toEqual([390, 390]);
  // The directory's header is sticky; bring the row to the middle of the
  // screen the way a reader scrolling to it would, rather than under it.
  const row = narrow.locator('[data-directory-row="FeatureFlag"]').getByRole("link", { name: "FeatureFlag" });
  await row.evaluate(element => element.scrollIntoView({ block: "center" }));
  await row.tap();
  await expect(narrow.locator("[data-technical-entity]")).toHaveAttribute("data-technical-revision", "r1");
  expect(await narrow.evaluate(() => [window.innerWidth, document.documentElement.scrollWidth])).toEqual([390, 390]);
  await narrow.locator("[data-technical-usages] [data-usage-item]").tap();
  await waitForSettledSaga(narrow);
  await expect(narrow.locator("[data-deck-slide]:visible")).toHaveAttribute("data-slide-title", "checkout-flag");
  await context.close();
});

test("a missing definition or revision is a 404, never the latest definition", async ({ page, saga }) => {
  author(saga);
  for (const path of ["/technical/system/FeatureFlag?revision=r9", "/technical/system/Absent", "/technical/nonsense/FeatureFlag"]) {
    const response = await page.goto(saga.baseURL + path);
    expect(response?.status(), path).toBe(404);
  }
});
