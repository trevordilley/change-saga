import { type Page } from "@playwright/test";
import { expectNoSeriousAccessibilityViolations, expect, test, waitForSettledSaga } from "../support/test.js";

/**
 * The Features section is a plain list: every feature, one row each, in the order
 * they were introduced. The feature whose content is on screen opens over its
 * authored contents; every other feature stays shut, a single row. The sidebar
 * is loaded once and kept across pages, so a shut feature still carries its
 * contents; the page only says which feature is open. Nothing about which one
 * is open is stored, so the list only ever says what the page already says.
 */

/** Every feature's row, in order. */
const featureRows = (page: Page) => page.locator("#nav-features > .doc-node");

/** The title of each feature that is opened over its authored contents. */
const openFeatures = (page: Page) => page.locator("#nav-features > .doc-node:has(> .doc-children:not([hidden])) > .doc-row > .doc-link");

test("@critical lists every feature and opens only the one being read", async ({ page, saga }) => {
  const contents = page.getByRole("navigation", { name: "Contents" });

  // Documentation holds two sections, in order, and both open a page.
  // Reviews are the header's other side, never repeated here.
  const places = contents.locator(":scope > .doc-node:not([hidden]) > .doc-row > .doc-link");
  await expect(places).toHaveText(["Overview", "Features"]);
  await expect(places.nth(1)).toHaveAttribute("href", "/features");
  // What describes the whole app hangs off the overview, beneath its prose.
  const overview = contents.locator(":scope > .doc-node").first();
  await expect(overview.locator(":scope > .doc-children > .doc-node > .doc-row > .doc-link")).toHaveText([
    "Name", "Elevator pitch", "Description", "Terms and vocabulary", "Personas", "Feature flags"
  ]);
  // Terms stay shut until they are opened, and the header is still the way in.
  await expect(contents.getByRole("button", { name: "Toggle Terms and vocabulary" })).toHaveAttribute("aria-expanded", "false");
  await expect(contents.getByRole("link", { name: "Terms and vocabulary", exact: true })).toHaveAttribute("href", "/terms");

  // Every feature is a row of its own, in creation order, linking to its page.
  await expect(featureRows(page).locator(":scope > .doc-row > .doc-link")).toHaveText(["Wave One", "Tide Charts", "Harbor Lights"]);
  for (const [id, title] of [["wave-one", "Wave One"], ["tide-charts", "Tide Charts"], ["harbor-lights", "Harbor Lights"]]) {
    await expect(contents.getByRole("link", { name: title, exact: true })).toHaveAttribute("href", `/features/${id}`);
  }

  // Exactly one of them is opened: the feature this page belongs to.
  await expect(openFeatures(page)).toHaveText(["Wave One"]);
  const waveOne = featureRows(page).first();
  // Its own report outline is present; all four empty product places are not.
  await expect(waveOne.locator(":scope > .doc-children > .doc-node > .doc-row > .doc-link")).toHaveText([
    "Overview", "Architecture Diagram", "Interactive Demo", "Raster Preview",
    "Architecture"
  ]);
  // The closed features show nothing of their places: one row each.
  await expect(contents.getByRole("button", { name: /Toggle (Product|Design|Quality|Implementation)/ })).toHaveCount(0);
  // Every ordinary row gets a real icon; slide rows use thumbnails instead.
  const ordinaryRows = contents.locator(".doc-row > .doc-link");
  await expect(ordinaryRows.locator(":scope > svg.i")).toHaveCount(await ordinaryRows.count());
  await expectNoSeriousAccessibilityViolations(page);

  // Nothing that served the old picker is left: no dropdown, no filter, no
  // "Show all features" disclosure.
  for (const gone of ["[data-feature-picker]", "[data-feature-option]", "[data-feature-list]", "[data-feature-filter]"]) {
    await expect(page.locator(gone)).toHaveCount(0);
  }
  await expect(contents.getByText("Show all features")).toHaveCount(0);

  // Clicking another feature's row opens that feature, and closes the first.
  await contents.getByRole("link", { name: "Tide Charts", exact: true }).click();
  await expect(page).toHaveURL(`${saga.baseURL}/features/tide-charts`);
  await waitForSettledSaga(page);
  await expect(featureRows(page).locator(":scope > .doc-row > .doc-link")).toHaveText(["Wave One", "Tide Charts", "Harbor Lights"]);
  await expect(openFeatures(page)).toHaveText(["Tide Charts"]);
});

test("the open feature follows the page, and no reading preference is stored", async ({ page, context, saga }) => {
  // A story's page opens the story's feature, wherever it is reached from.
  await page.goto(`${saga.baseURL}/requirements/read-the-tide`);
  await waitForSettledSaga(page);
  await expect(openFeatures(page)).toHaveText(["Tide Charts"]);
  await expectNoSeriousAccessibilityViolations(page);

  // The app's own pages belong to no feature, so they open none: the list is
  // rows, and nothing remembers where the reader has been.
  for (const path of ["/", "/terms", "/features"]) {
    await page.goto(`${saga.baseURL}${path}`);
    await waitForSettledSaga(page);
    await expect(featureRows(page).locator(":scope > .doc-row > .doc-link")).toHaveText(["Wave One", "Tide Charts", "Harbor Lights"]);
    await expect(openFeatures(page)).toHaveCount(0);
  }
  expect(await context.cookies()).toEqual([]);
});

test("@critical the list is plain links, so it works with JavaScript disabled", async ({ browser, saga }) => {
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage();
  try {
    await page.goto(`${saga.baseURL}/features/wave-one`);
    const contents = page.getByRole("navigation", { name: "Contents" });
    for (const title of ["Wave One", "Tide Charts", "Harbor Lights"]) {
      await expect(contents.getByRole("link", { name: title, exact: true })).toBeVisible();
    }
    await expect(openFeatures(page)).toHaveText(["Wave One"]);
    await contents.getByRole("link", { name: "Harbor Lights", exact: true }).click();
    await expect(page).toHaveURL(`${saga.baseURL}/features/harbor-lights`);
    // Harbor Lights has no authored contents yet, so its page opens without
    // inventing an empty subtree in the sidebar.
    await expect(openFeatures(page)).toHaveCount(0);
    await expect(page.getByRole("heading", { name: "Harbor Lights", exact: true })).toBeVisible();
    for (const empty of ["[data-feature-stories]", "[data-feature-design]", "[data-feature-quality]", "[data-feature-implementation]"]) {
      await expect(page.locator(empty)).toHaveCount(0);
    }

    // The Features header still opens the table of all of them.
    await contents.getByRole("link", { name: "Features", exact: true }).click();
    await expect(page).toHaveURL(`${saga.baseURL}/features`);
    await expect(page.locator('[data-directory-page="features"]').getByRole("link", { name: "Harbor Lights", exact: true })).toBeVisible();
    // Reading it stored nothing.
    expect(await context.cookies()).toEqual([]);
  } finally {
    await context.close();
  }
});
