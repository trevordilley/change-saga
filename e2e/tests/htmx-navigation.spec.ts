import { type Page } from "@playwright/test";
import { runCLI, type SagaFixture } from "../support/fixture-builder.js";
import { expect, test, waitForSettledSaga } from "../support/test.js";

/**
 * The reviewer loads its shell once: the stylesheet and scripts, the sidebar,
 * and the viewer of every deck. Following a link swaps in only what the new
 * page changes, so nothing already loaded is downloaded, parsed, or prepared
 * again, and every page is still a URL that renders whole on its own.
 */

function run(saga: SagaFixture, ...args: string[]): void {
  const result = runCLI(saga, args, saga.sagaRepo);
  expect(result.status, `${args[0]} failed\n${result.stdout}\n${result.stderr}`).toBe(0);
}

/** A deck with one slide, so the shell has a deck viewer to keep. */
async function withDeck(page: Page, saga: SagaFixture): Promise<void> {
  run(saga, "add-deck", "--feature", "wave-one", "--objective", "Explain the request flow.", saga.sagaRoot, "request-flow");
  run(saga, "add-slide", "--deck", "request-flow", "--intent", "explain", "--layout", "diagram", "--title", "Request enters",
    "--takeaway", "Requests enter at the edge.", saga.sagaRoot, "request-enters");
  await page.reload();
  await waitForSettledSaga(page);
  await expect(page.locator("#view-slides [data-deck-viewer]")).toHaveCount(1);
}

/** Marks the document and the kept deck viewer, so a reload or re-render shows. */
async function markShell(page: Page): Promise<void> {
  await page.evaluate(() => {
    (window as unknown as { keptDocument: boolean }).keptDocument = true;
    (document.querySelector("#view-slides [data-deck-viewer]") as unknown as { kept: boolean }).kept = true;
  });
}

async function shellKept(page: Page): Promise<{ document: boolean; decks: boolean }> {
  return page.evaluate(() => ({
    document: (window as unknown as { keptDocument?: boolean }).keptDocument === true,
    decks: (document.querySelector("#view-slides [data-deck-viewer]") as unknown as { kept?: boolean } | null)?.kept === true
  }));
}

/** What the page says about the sidebar: its current rows and open places. */
async function sidebarState(page: Page): Promise<{ current: string[]; open: string[] }> {
  return page.evaluate(() => ({
    current: [...document.querySelectorAll(".doc-tree .doc-row.current")].map((row) => (row as HTMLElement).dataset.navRow ?? ""),
    open: [...document.querySelectorAll(".doc-tree .doc-children:not([hidden])")].map((children) => children.id)
  }));
}

async function follow(page: Page, link: ReturnType<Page["locator"]>): Promise<void> {
  await link.click();
  await waitForSettledSaga(page);
}

const pageHeading = (page: Page) => page.locator("#page h1");

test("@critical following links swaps the page and keeps everything already loaded", async ({ page, saga }) => {
  await withDeck(page, saga);
  await markShell(page);
  const shellRequests: string[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (url.pathname.startsWith("/assets/") || url.pathname === "/decks" || request.isNavigationRequest() && request.frame() === page.mainFrame()) {
      shellRequests.push(url.pathname);
    }
  });
  const contents = page.getByRole("navigation", { name: "Contents" });

  // Another feature: its page, its row current and open, the first shut.
  await follow(page, contents.getByRole("link", { name: "Tide Charts", exact: true }));
  await expect(page).toHaveURL(`${saga.baseURL}/features/tide-charts`);
  await expect(pageHeading(page)).toHaveText("Tide Charts");
  await expect(page).toHaveTitle(/^Tide Charts · /);
  await expect(pageHeading(page)).toBeFocused();
  await expect(page.locator("#nav-features > .doc-node:has(> .doc-children:not([hidden])) > .doc-row > .doc-link")).toHaveText(["Tide Charts"]);
  await expect(contents.locator('.doc-row.current > a[aria-current="page"]')).toHaveText(["Tide Charts"]);

  // A story, from a link in the page's content.
  await follow(page, page.locator('#page a[href="/requirements/read-the-tide"]').first());
  await expect(page).toHaveURL(`${saga.baseURL}/requirements/read-the-tide`);
  await expect(pageHeading(page)).toBeFocused();

  // The personas, and one persona.
  await follow(page, contents.getByRole("link", { name: "Personas", exact: true }));
  await expect(pageHeading(page)).toHaveText("Personas");
  await follow(page, page.locator("#page").getByRole("link", { name: "Skipper", exact: true }));
  await expect(pageHeading(page)).toHaveText("Skipper");
  await expect(page).toHaveTitle(/^Skipper · /);

  // The overview, then its technical design.
  await follow(page, page.getByRole("link", { name: "Documentation", exact: true }));
  await expect(page).toHaveURL(`${saga.baseURL}/`);
  await follow(page, page.locator('#page a[href="/technical"]').first());
  await expect(page).toHaveURL(`${saga.baseURL}/technical`);

  // The other side of the header: its tab, its section, and its tabs.
  await follow(page, page.getByRole("link", { name: "Review", exact: true }));
  await expect(page).toHaveURL(`${saga.baseURL}/reviews`);
  await expect(pageHeading(page)).toHaveText("Reviews");
  await expect(page.getByRole("link", { name: "Review", exact: true })).toHaveAttribute("aria-current", "page");
  await expect(page.getByRole("tablist", { name: "Review" })).toBeVisible();
  await expect(contents.getByRole("link", { name: "Features", exact: true })).toBeHidden();

  // Nothing was loaded again: no new document, no script or stylesheet, and
  // the deck viewer is the one element it was.
  expect(await shellKept(page)).toEqual({ document: true, decks: true });
  expect(shellRequests).toEqual([]);
});

test("back and forward restore each page and its sidebar", async ({ page, saga }) => {
  await page.evaluate(() => { (window as unknown as { keptDocument: boolean }).keptDocument = true; });
  const contents = page.getByRole("navigation", { name: "Contents" });
  const waveOne = await sidebarState(page);
  await follow(page, contents.getByRole("link", { name: "Tide Charts", exact: true }));
  const tideCharts = await sidebarState(page);
  await follow(page, contents.getByRole("link", { name: "Features", exact: true }));
  await expect(pageHeading(page)).toHaveText("Features");

  await page.goBack();
  await waitForSettledSaga(page);
  await expect(page).toHaveURL(`${saga.baseURL}/features/tide-charts`);
  await expect(pageHeading(page)).toHaveText("Tide Charts");
  expect(await sidebarState(page)).toEqual(tideCharts);

  await page.goBack();
  await waitForSettledSaga(page);
  await expect(page).toHaveURL(saga.featureURL);
  await expect(pageHeading(page)).toHaveText("Wave One");
  expect(await sidebarState(page)).toEqual(waveOne);

  await page.goForward();
  await waitForSettledSaga(page);
  await expect(pageHeading(page)).toHaveText("Tide Charts");
  expect(await sidebarState(page)).toEqual(tideCharts);
  expect((await shellKept(page)).document).toBe(true);
});

test("a page reached by a link is the page a deep link or a refresh renders", async ({ page, saga }) => {
  const contents = page.getByRole("navigation", { name: "Contents" });
  for (const title of ["Tide Charts", "Personas", "Terms and vocabulary"]) {
    await follow(page, contents.getByRole("link", { name: title, exact: true }));
    const swapped = { url: page.url(), title: await page.title(), heading: await pageHeading(page).textContent(), sidebar: await sidebarState(page) };
    await page.reload();
    await waitForSettledSaga(page);
    const loaded = { url: page.url(), title: await page.title(), heading: await pageHeading(page).textContent(), sidebar: await sidebarState(page) };
    expect(loaded).toEqual(swapped);
  }
});

test("a Saga edited while it is read reaches the kept sidebar on the next page", async ({ page, saga }) => {
  await page.evaluate(() => { (window as unknown as { keptDocument: boolean }).keptDocument = true; });
  const contents = page.getByRole("navigation", { name: "Contents" });
  await expect(contents.locator('a[href="/personas/navigator"]')).toHaveCount(0);
  run(saga, "persona", "add", "--id", "navigator", "--name", "Navigator", "--description", "Plots the course.", saga.sagaRoot);

  await follow(page, contents.getByRole("link", { name: "Personas", exact: true }));
  // The page and the sidebar both show the Saga as it is now, without a
  // new document.
  await expect(page.locator("#page").getByRole("link", { name: "Navigator", exact: true })).toBeVisible();
  await expect(contents.locator('a[href="/personas/navigator"]')).toHaveCount(1);
  expect((await shellKept(page)).document).toBe(true);
});
