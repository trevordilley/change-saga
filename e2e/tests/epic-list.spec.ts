import { type Page } from "@playwright/test";
import { expectNoSeriousAccessibilityViolations, expect, test, waitForSettledSaga } from "../support/test.js";

/**
 * The Epics section is a plain list: every epic, one row each, in the order
 * they were introduced. The epic whose content is on screen opens over its
 * four places; every other epic is the row alone. Nothing about which one is
 * open is stored, so the list only ever says what the page already says.
 */

/** Every epic's row, in order. */
const epicRows = (page: Page) => page.locator("#nav-epics > .doc-node");

/** The title of each epic that is opened over its four places. */
const openEpics = (page: Page) => page.locator("#nav-epics > .doc-node:has(> .doc-children) > .doc-row > .doc-link");

test("@critical lists every epic and opens only the one being read", async ({ page, saga }) => {
  const contents = page.getByRole("navigation", { name: "Contents" });

  // Three sections, in order, and every one of them opens a page.
  const places = contents.locator(":scope > .doc-node > .doc-row > .doc-link");
  await expect(places).toHaveText(["Overview", "Epics", "Reviews"]);
  await expect(places.nth(1)).toHaveAttribute("href", "/epics");
  await expect(places.nth(2)).toHaveAttribute("href", "/reviews");
  // What describes the whole app hangs off the overview, beneath its prose.
  const overview = contents.locator(":scope > .doc-node").first();
  await expect(overview.locator(":scope > .doc-children > .doc-node > .doc-row > .doc-link")).toHaveText([
    "Name", "Elevator pitch", "Description", "Terms and vocabulary", "Personas", "Design system", "Onboarding", "Feature flags"
  ]);
  // Terms stay shut until they are opened, and the header is still the way in.
  await expect(contents.getByRole("button", { name: "Toggle Terms and vocabulary" })).toHaveAttribute("aria-expanded", "false");
  await expect(contents.getByRole("link", { name: "Terms and vocabulary", exact: true })).toHaveAttribute("href", "/terms");

  // Every epic is a row of its own, in creation order, linking to its page.
  await expect(epicRows(page).locator(":scope > .doc-row > .doc-link")).toHaveText(["Wave One", "Tide Charts", "Harbor Lights"]);
  for (const [id, title] of [["wave-one", "Wave One"], ["tide-charts", "Tide Charts"], ["harbor-lights", "Harbor Lights"]]) {
    await expect(contents.getByRole("link", { name: title, exact: true })).toHaveAttribute("href", `/epics/${id}`);
  }

  // Exactly one of them is opened: the epic this page belongs to.
  await expect(openEpics(page)).toHaveText(["Wave One"]);
  const waveOne = epicRows(page).first();
  // Its own report outline, then the same four places, in that order.
  await expect(waveOne.locator(":scope > .doc-children > .doc-node > .doc-row > .doc-link")).toHaveText([
    "Overview", "Architecture Diagram", "Interactive Demo", "Raster Preview",
    "Architecture", "Product", "Design", "Quality", "Implementation"
  ]);
  // Implementation opens to its slides; the other three stay collapsed.
  for (const place of ["Product", "Design", "Quality"]) {
    await expect(waveOne.getByRole("button", { name: `Toggle ${place}` })).toHaveAttribute("aria-expanded", "false");
  }
  await expect(waveOne.getByRole("button", { name: "Toggle Implementation" })).toHaveAttribute("aria-expanded", "true");
  // The closed epics spend nothing: one row, no twisty, no places.
  await expect(contents.getByRole("button", { name: "Toggle Product" })).toHaveCount(1);
  await expectNoSeriousAccessibilityViolations(page);

  // Nothing that served the old picker is left: no dropdown, no filter, no
  // "Show all epics" disclosure.
  for (const gone of ["[data-epic-picker]", "[data-epic-option]", "[data-epic-list]", "[data-epic-filter]"]) {
    await expect(page.locator(gone)).toHaveCount(0);
  }
  await expect(contents.getByText("Show all epics")).toHaveCount(0);

  // Clicking another epic's row opens that epic, and closes the first.
  await contents.getByRole("link", { name: "Tide Charts", exact: true }).click();
  await expect(page).toHaveURL(`${saga.baseURL}/epics/tide-charts`);
  await waitForSettledSaga(page);
  await expect(epicRows(page).locator(":scope > .doc-row > .doc-link")).toHaveText(["Wave One", "Tide Charts", "Harbor Lights"]);
  await expect(openEpics(page)).toHaveText(["Tide Charts"]);
});

test("the open epic follows the page, and no reading preference is stored", async ({ page, context, saga }) => {
  // A story's page opens the story's epic, wherever it is reached from.
  await page.goto(`${saga.baseURL}/requirements/read-the-tide`);
  await waitForSettledSaga(page);
  await expect(openEpics(page)).toHaveText(["Tide Charts"]);
  await expectNoSeriousAccessibilityViolations(page);

  // The app's own pages belong to no epic, so they open none: the list is
  // rows, and nothing remembers where the reader has been.
  for (const path of ["/", "/terms", "/epics"]) {
    await page.goto(`${saga.baseURL}${path}`);
    await waitForSettledSaga(page);
    await expect(epicRows(page).locator(":scope > .doc-row > .doc-link")).toHaveText(["Wave One", "Tide Charts", "Harbor Lights"]);
    await expect(openEpics(page)).toHaveCount(0);
  }
  expect(await context.cookies()).toEqual([]);
});

test("@critical the list is plain links, so it works with JavaScript disabled", async ({ browser, saga }) => {
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage();
  try {
    await page.goto(`${saga.baseURL}/epics/wave-one`);
    const contents = page.getByRole("navigation", { name: "Contents" });
    for (const title of ["Wave One", "Tide Charts", "Harbor Lights"]) {
      await expect(contents.getByRole("link", { name: title, exact: true })).toBeVisible();
    }
    await expect(openEpics(page)).toHaveText(["Wave One"]);
    await contents.getByRole("link", { name: "Harbor Lights", exact: true }).click();
    await expect(page).toHaveURL(`${saga.baseURL}/epics/harbor-lights`);
    await expect(openEpics(page)).toHaveText(["Harbor Lights"]);

    // The Epics header still opens the table of all of them.
    await contents.getByRole("link", { name: "Epics", exact: true }).click();
    await expect(page).toHaveURL(`${saga.baseURL}/epics`);
    await expect(page.locator('[data-directory-page="epics"]').getByRole("link", { name: "Harbor Lights", exact: true })).toBeVisible();
    // Reading it stored nothing.
    expect(await context.cookies()).toEqual([]);
  } finally {
    await context.close();
  }
});
