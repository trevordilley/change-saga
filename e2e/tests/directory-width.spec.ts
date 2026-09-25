import { writeFileSync } from "node:fs";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect, test, waitForSettledSaga } from "../support/test.js";
import { runCLI, type SagaFixture } from "../support/fixture-builder.js";

// A directory table can need more width than a phone has. A mobile browser
// widens its layout viewport to fit such a table, and then the whole page
// scrolls sideways. Every directory's table scrolls inside its own region
// instead, so the page keeps the device's width and every column stays
// reachable by touch and by keyboard.

const prefix = "urn:change-saga:wave-one:";

// Every directory gets at least one row, so each renders its table rather
// than only its growth state.
function authorEveryDirectory(saga: SagaFixture): void {
  const run = (...args: string[]): void => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, result.stdout + result.stderr).toBe(0);
  };
  run("review", "create", "--id", "pr-1", "--base", "main", "--head", "feature/wave-one", "--pr", "1", "--url", "https://example.test/acme/change-saga-demo/pull/1", "--title", "Wave one review", saga.sagaRoot);
  const code = [{ commit: "HEAD", path: "src/app.go", start: 3, end: 3, note: "Exact implementation line used by this disposable definition." }];
  const path = join(saga.root, "FlagClient.json");
  writeFileSync(path, JSON.stringify({ name: "FlagClient", explanation: "A shared implementation unit with exact code and a stable identity.", code }));
  run("component", "add", "--id", "FlagClient", "--from", path, "--repo", saga.sourceRepo, saga.sagaRoot);
}

const directories = [
  { path: "/features", id: "features", region: "Features" },
  { path: "/personas", id: "personas", region: "Personas" },
  { path: "/flags", id: "flags", region: "Feature flags" },
  { path: "/terms", id: "terms", region: "Terms and vocabulary" },
  { path: "/reviews", id: "reviews", region: "Reviews" },
  { path: "/technical/components", id: "technical-components", region: "Components" }
];

type Measure = { inner: number; page: number; region: number; table: number; overflowX: string };

function measure(page: Page, id: string): Promise<Measure> {
  return page.evaluate(id => {
    const region = document.querySelector<HTMLElement>(`[data-directory="${id}"] [data-directory-scroll]`)!;
    return {
      inner: window.innerWidth,
      page: document.documentElement.scrollWidth,
      region: region.clientWidth,
      table: region.scrollWidth,
      overflowX: getComputedStyle(region).overflowX
    };
  }, id);
}

test("every directory keeps a 390px touch screen at its width and scrolls its table in place", async ({ browser, browserName, saga }) => {
  authorEveryDirectory(saga);
  // Firefox has no mobile layout viewport to emulate; there the page's own
  // scroll width still shows whether a table pushed it wider.
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: browserName !== "firefox", hasTouch: true });
  const narrow = await context.newPage();
  const scrolled: string[] = [];
  for (const directory of directories) {
    await narrow.goto(saga.baseURL + directory.path);
    await waitForSettledSaga(narrow);
    const widths = await narrow.evaluate(() => [window.innerWidth, document.documentElement.scrollWidth]);
    expect(widths, `${directory.path} is wider than the device`).toEqual([390, 390]);
    const region = narrow.locator(`[data-directory="${directory.id}"]`).getByRole("region", { name: directory.region, exact: true });
    await expect(region, directory.path).toHaveAttribute("tabindex", "0");
    // The region holds the real table, with its caption and headers intact.
    await expect(region.getByRole("table"), directory.path).toHaveCount(1);
    await expect(region.getByRole("columnheader").first(), directory.path).toBeVisible();

    const before = await measure(narrow, directory.id);
    expect(before.overflowX, directory.path).toBe("auto");
    if (before.table <= before.region) continue;

    // Panning moves the table inside its region; the page has nowhere to go.
    // (Mobile WebKit has no wheel to emulate a swipe, so this sets the scroll
    // positions directly; the keyboard check below drives real input.)
    await region.evaluate(element => { element.scrollLeft = element.scrollWidth; });
    await narrow.evaluate(() => window.scrollBy(400, 0));
    expect(await region.evaluate(element => element.scrollLeft), directory.path).toBeGreaterThan(0);
    expect(await narrow.evaluate(() => [window.scrollX, window.visualViewport?.pageLeft ?? 0]), directory.path).toEqual([0, 0]);
    expect((await measure(narrow, directory.id)).page, directory.path).toBeLessThanOrEqual(390);
    scrolled.push(directory.path);
  }
  // The widest tables really do exceed a phone's width, so the check above is
  // not vacuous for them.
  expect(scrolled).toEqual(expect.arrayContaining(["/features", "/terms", "/reviews", "/technical/components"]));

  // A keyboard reader focuses the region and scrolls it with the arrow keys.
  await narrow.goto(saga.baseURL + "/features");
  await waitForSettledSaga(narrow);
  const features = narrow.getByRole("region", { name: "Features", exact: true });
  await features.focus();
  await expect(features).toBeFocused();
  for (let press = 0; press < 5; press++) await narrow.keyboard.press("ArrowRight");
  await expect.poll(() => features.evaluate(element => element.scrollLeft)).toBeGreaterThan(0);
  expect(await narrow.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await context.close();
});

test("directories at desktop width fill their column without scrolling", async ({ page, saga }) => {
  authorEveryDirectory(saga);
  await page.setViewportSize({ width: 1440, height: 1000 });
  for (const directory of directories) {
    await page.goto(saga.baseURL + directory.path);
    await waitForSettledSaga(page);
    const size = await measure(page, directory.id);
    expect(size.page, directory.path).toBeLessThanOrEqual(size.inner);
    expect(size.table, `${directory.path} scrolls at desktop width`).toBeLessThanOrEqual(size.region);
  }
});
