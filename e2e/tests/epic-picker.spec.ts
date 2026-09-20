import { expectNoSeriousAccessibilityViolations, expect, test, waitForSettledSaga } from "../support/test.js";

/**
 * The sidebar shows one epic at a time. These tests hold the two halves of
 * that apart: the enhanced picker a reviewer with JavaScript uses, and the
 * plain disclosure the server renders for one without.
 */

test("@critical shows one epic and picks another by typing", async ({ page, saga }) => {
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

  // One epic, over its four places. The others are not in the tree at all.
  const epic = page.locator(".doc-epic-current");
  await expect(epic.locator(":scope > .doc-row > .doc-link")).toHaveText("Wave One");
  // Its own report outline, then the same four places, in that order.
  await expect(epic.locator(":scope > .doc-children > .doc-node > .doc-row > .doc-link")).toHaveText([
    "Overview", "Architecture Diagram", "Interactive Demo", "Raster Preview",
    "Architecture", "Product", "Design", "Quality", "Implementation"
  ]);
  // Another epic is nowhere in the tree; it is only in the picker and the list.
  await expect(contents.locator(".doc-epic-current")).toHaveCount(1);
  await expect(contents.locator(".doc-link", { hasText: "Tide Charts" })).toHaveCount(0);
  // Implementation opens to its slides; the other three stay collapsed.
  for (const place of ["Product", "Design", "Quality"]) {
    await expect(epic.getByRole("button", { name: `Toggle ${place}` })).toHaveAttribute("aria-expanded", "false");
  }
  await expect(epic.getByRole("button", { name: "Toggle Implementation" })).toHaveAttribute("aria-expanded", "true");

  // The picker: one control, labelled, that opens a searchable list.
  const picker = page.locator("[data-epic-picker]");
  const summary = picker.locator("summary");
  await expect(summary).toHaveAttribute("aria-label", "Choose epic, currently Wave One");
  await summary.click();
  const filter = picker.getByRole("combobox", { name: "Filter epics" });
  await expect(filter).toBeFocused();
  await expect(filter).toHaveAttribute("aria-expanded", "true");
  const options = picker.getByRole("option");
  await expect(options).toHaveText([/Wave One/, /Tide Charts/, /Harbor Lights/]);
  await expectNoSeriousAccessibilityViolations(page);

  // Typing filters by title and by id, and the empty case says so.
  await filter.fill("tide");
  await expect(options).toHaveCount(1);
  await expect(options.first()).toContainText("Tide Charts");
  await filter.fill("harbor-lights");
  await expect(options).toHaveCount(1);
  await filter.fill("nothing here");
  await expect(options).toHaveCount(0);
  await expect(picker.getByText("No epic matches this filter.")).toBeVisible();

  // Arrow keys move the active option and Enter opens it.
  await filter.fill("a");
  const active = async (): Promise<string | null> => filter.getAttribute("aria-activedescendant");
  const first = await active();
  await page.keyboard.press("ArrowDown");
  expect(await active()).not.toBe(first);
  await page.keyboard.press("ArrowUp");
  expect(await active()).toBe(first);
  await filter.fill("tide");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(`${saga.baseURL}/epics/tide-charts`);
  await waitForSettledSaga(page);
  await expect(page.locator(".doc-epic-current > .doc-row > .doc-link")).toHaveText("Tide Charts");
});

test("Escape closes the picker and hands focus back", async ({ page, saga }) => {
  expect(saga.baseURL).toBeTruthy();
  const picker = page.locator("[data-epic-picker]");
  const summary = picker.locator("summary");
  await summary.click();
  await expect(picker).toHaveAttribute("open", "");
  await page.keyboard.press("Escape");
  await expect(picker).not.toHaveAttribute("open", /.*/);
  await expect(summary).toBeFocused();

  // Clicking away closes it too, without choosing anything.
  await summary.click();
  await expect(picker).toHaveAttribute("open", "");
  await page.locator(".sidebar-title").click();
  await expect(picker).not.toHaveAttribute("open", /.*/);
});

test("the chosen epic follows the reader onto the app's own pages", async ({ page, saga }) => {
  await page.goto(`${saga.baseURL}/epics/harbor-lights`);
  await waitForSettledSaga(page);
  await expect(page.locator(".doc-epic-current > .doc-row > .doc-link")).toHaveText("Harbor Lights");

  // The overview belongs to no epic, so it keeps showing the last one chosen.
  await page.goto(`${saga.baseURL}/`);
  await waitForSettledSaga(page);
  await expect(page.locator(".doc-epic-current > .doc-row > .doc-link")).toHaveText("Harbor Lights");

  // A story's own epic wins over that memory, whichever page it is reached from.
  await page.goto(`${saga.baseURL}/requirements/read-the-tide`);
  await waitForSettledSaga(page);
  await expect(page.locator(".doc-epic-current > .doc-row > .doc-link")).toHaveText("Tide Charts");
  await expectNoSeriousAccessibilityViolations(page);
});

test("the full list of epics expands for browsing and collapses again", async ({ page, saga }) => {
  const list = page.locator("[data-epic-list]");
  const toggle = list.locator("summary");
  await expect(toggle).toContainText("Show all epics");
  await expect(list.getByRole("link", { name: "Harbor Lights" })).toBeHidden();
  await toggle.click();
  for (const title of ["Wave One", "Tide Charts", "Harbor Lights"]) {
    await expect(list.getByRole("link", { name: title, exact: true })).toBeVisible();
  }
  await expectNoSeriousAccessibilityViolations(page);
  // Browsing the list never repeats an epic's four places.
  await expect(list.getByRole("button", { name: /Toggle Product/ })).toHaveCount(0);
  await toggle.click();
  await expect(list.getByRole("link", { name: "Harbor Lights" })).toBeHidden();

  // The index stays reachable as a page of its own.
  await toggle.click();
  await list.getByRole("link", { name: "Open the epics index" }).click();
  await expect(page).toHaveURL(`${saga.baseURL}/epics`);
  await waitForSettledSaga(page);
  const index = page.locator('[data-directory-page="epics"]');
  await expect(index.getByRole("heading", { name: "Epics", exact: true })).toBeVisible();
  for (const title of ["Wave One", "Tide Charts", "Harbor Lights"]) {
    await expect(index.getByRole("link", { name: title, exact: true })).toBeVisible();
  }
  await expectNoSeriousAccessibilityViolations(page);
});

test("@critical the picker still works with JavaScript disabled", async ({ browser, saga }) => {
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage();
  try {
    await page.goto(`${saga.baseURL}/epics/wave-one`);
    const picker = page.locator("[data-epic-picker]");
    // The filter field belongs to the enhanced path and stays out of the way.
    await expect(picker.locator("[data-epic-picker-search]")).toBeHidden();
    await expect(picker.locator("[data-epic-option]").first()).toBeHidden();

    // The summary is a real disclosure: it opens a list of links.
    await picker.locator("summary").click();
    const option = picker.getByRole("link", { name: /Tide Charts/ });
    await expect(option).toBeVisible();
    await option.click();
    await expect(page).toHaveURL(`${saga.baseURL}/epics/tide-charts`);
    await expect(page.locator(".doc-epic-current > .doc-row > .doc-link")).toHaveText("Tide Charts");

    // And the full list and the index work the same way.
    await page.locator("[data-epic-list] summary").click();
    await expect(page.locator("[data-epic-list]").getByRole("link", { name: "Harbor Lights", exact: true })).toBeVisible();
    await page.locator("[data-epic-list]").getByRole("link", { name: "Open the epics index" }).click();
    await expect(page).toHaveURL(`${saga.baseURL}/epics`);
    await expect(page.locator('[data-directory-page="epics"]').getByRole("link", { name: "Harbor Lights", exact: true })).toBeVisible();
  } finally {
    await context.close();
  }
});
