import { expectNoSeriousAccessibilityViolations, expect, test, waitForSettledSaga } from "../support/test.js";
import { runCLI } from "../support/fixture-builder.js";

/**
 * Three sections, and every header opens a page. These tests hold the rule
 * the old sidebar broke: a section header was a row that only expanded, so
 * reading the vocabulary meant opening every term one at a time, and
 * /personas answered 404 while /personas/{id} answered.
 */

test("@critical every section header opens its page rather than only expanding", async ({ page, saga }) => {
  const contents = page.getByRole("navigation", { name: "Contents" });
  await page.goto(`${saga.baseURL}/`);
  await waitForSettledSaga(page);

  // Overview: prose, and a directory of the parts that describe the app.
  const directory = page.locator("[data-overview-directory]");
  for (const part of ["Personas", "Terms and vocabulary", "Design system", "Onboarding", "Feature flags"]) {
    await expect(directory.locator(`[data-overview-part="${part}"]`)).toBeVisible();
  }
  await expect(directory.locator('[data-overview-part="Terms and vocabulary"] .overview-part-count')).toHaveText("1 term");
  await expect(directory.locator('[data-overview-part="Personas"] .overview-part-count')).toHaveText("2 personas");
  // A part nothing fills states the growth and the command, never a failure.
  await expect(directory.locator('[data-overview-part="Onboarding"]')).toContainText("change-saga add-deck --role onboarding");
  await expectNoSeriousAccessibilityViolations(page);

  // Terms and vocabulary: the whole vocabulary at once, as a table.
  await contents.getByRole("link", { name: "Terms and vocabulary", exact: true }).click();
  await expect(page).toHaveURL(`${saga.baseURL}/terms`);
  await waitForSettledSaga(page);
  const terms = page.locator('[data-directory="terms"]');
  await expect(terms.getByRole("columnheader")).toHaveText(["Term", "Also", "Definition", "Defined in code", "Reference"]);
  const greeting = terms.getByRole("row", { name: /Greeting/ });
  await expect(greeting).toContainText("salutation");
  await expect(greeting).toContainText("The line a caller is welcomed with");
  await expect(greeting).toContainText("src/app.go:3-5");
  await expect(greeting).toContainText("current");
  await expectNoSeriousAccessibilityViolations(page);

  // Personas: the table the section never had, at the path that used to 404.
  await contents.getByRole("link", { name: "Personas", exact: true }).click();
  await expect(page).toHaveURL(`${saga.baseURL}/personas`);
  await waitForSettledSaga(page);
  const personas = page.locator('[data-directory="personas"]');
  await expect(personas.getByRole("columnheader")).toHaveText(["Persona", "Description", "State", "Stories served"]);
  await expect(personas.getByRole("row", { name: /Skipper/ })).toContainText("active");
  await expect(personas.getByRole("row", { name: /Skipper/ })).toContainText("none accepted yet");
  await expect(personas.getByRole("row", { name: /Harbour master/ })).toContainText("retired");
  await expectNoSeriousAccessibilityViolations(page);

  // Feature flags: what is gated, whether it is on, and what it gates.
  await contents.getByRole("link", { name: "Feature flags", exact: true }).click();
  await expect(page).toHaveURL(`${saga.baseURL}/flags`);
  await waitForSettledSaga(page);
  const flags = page.locator('[data-directory="flags"]');
  await expect(flags.getByRole("columnheader")).toHaveText(["Flag", "State", "Gates", "What it is for"]);
  await expect(flags.getByRole("row", { name: /tide-charts-beta/ })).toContainText("off");
  await expect(flags.getByRole("row", { name: /tide-charts-beta/ })).toContainText("Tide Charts");
  await expectNoSeriousAccessibilityViolations(page);

  // Features: counts of what each feature holds, and never a verdict on them.
  await contents.getByRole("link", { name: "Features", exact: true }).click();
  await expect(page).toHaveURL(`${saga.baseURL}/features`);
  await waitForSettledSaga(page);
  const features = page.locator('[data-directory="features"]');
  await expect(features.getByRole("columnheader")).toHaveText(["Feature", "Description", "Stories", "Accepted", "With design", "Test cases", "Slides"]);
  await expect(features.getByRole("row", { name: /Tide Charts/ })).toContainText("1");
  await expectNoSeriousAccessibilityViolations(page);

  // Reviews: the header's other side, with nothing to list yet.
  await page.getByRole("link", { name: "Review", exact: true }).click();
  await expect(page).toHaveURL(`${saga.baseURL}/reviews`);
  await waitForSettledSaga(page);
  await expect(page.locator('[data-directory="reviews"]')).toContainText("No reviews yet.");
  await expectNoSeriousAccessibilityViolations(page);
});

test("a deck's header opens it at the first slide", async ({ page, saga }) => {
  const run = (...args: string[]): void => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, `${args[0]} failed\n${result.stdout}\n${result.stderr}`).toBe(0);
  };
  run("add-deck", "--feature", "wave-one", "--objective", "Explain how a request is served.", saga.sagaRoot, "request-flow");
  for (const [slide, title] of [["request-enters", "Request enters"], ["response-returns", "Response returns"]] as const) {
    run("add-slide", "--deck", "request-flow", "--intent", "explain", "--layout", "diagram", "--title", title, "--takeaway", `${title} is explicit.`, saga.sagaRoot, slide);
  }
  await page.reload();
  await waitForSettledSaga(page);

  const contents = page.getByRole("navigation", { name: "Contents" });
  // The header is a link to the deck's first slide, and the slides stay
  // beneath it so any one of them is still one click away.
  const implementation = contents.getByRole("link", { name: "Implementation", exact: true });
  await expect(implementation).toHaveAttribute("href", /\?view=slides#./);
  await expect(contents.locator("[data-slide-thumbnail]")).toHaveCount(2);

  await implementation.click();
  await expect(page).toHaveURL(/view=slides#/);
  await expect(page.locator("#view-slides")).toBeVisible();
  await expect(page.locator('#view-slides [data-deck-slide][data-slide-title="Request enters"]')).toBeVisible();
  await expect(page.locator("#view-slides [data-slide-position]")).toHaveText("1 / 2");
});

test("@critical a directory filters as it is typed into, and without JavaScript too", async ({ page, browser, saga }) => {
  await page.goto(`${saga.baseURL}/features`);
  await waitForSettledSaga(page);
  const features = page.locator('[data-directory="features"]');
  const rows = features.locator("[data-directory-row]");
  await expect(rows).toHaveCount(3);
  await expect(features.locator("caption")).toHaveText("3 features");
  // The submit button belongs to the plain path; typing has replaced it.
  await expect(features.locator("[data-directory-submit]")).toBeHidden();

  const filter = features.getByRole("searchbox", { name: "Filter features" });
  await filter.fill("tide");
  await expect(rows.filter({ visible: true })).toHaveCount(1);
  await expect(features.locator("caption")).toHaveText("1 of 3 features");
  await filter.fill("nothing here");
  await expect(features.locator("[data-directory-none]")).toBeVisible();
  // Widening the filter brings the rows back: nothing was thrown away.
  await filter.fill("");
  await expect(rows.filter({ visible: true })).toHaveCount(3);
  await expectNoSeriousAccessibilityViolations(page);

  // With no JavaScript the same field is a form, and the server filters.
  const context = await browser.newContext({ javaScriptEnabled: false });
  const plain = await context.newPage();
  try {
    await plain.goto(`${saga.baseURL}/features`);
    const plainFeatures = plain.locator('[data-directory="features"]');
    await plainFeatures.getByRole("searchbox", { name: "Filter features" }).fill("tide");
    await plainFeatures.getByRole("button", { name: "Filter" }).click();
    await expect(plain).toHaveURL(`${saga.baseURL}/features?q=tide`);
    await expect(plainFeatures.locator("[data-directory-row]").filter({ visible: true })).toHaveCount(1);
    await expect(plainFeatures.locator("caption")).toHaveText("1 of 3 features");
    // And a way back to all of them, which a plain browser also needs.
    await plainFeatures.getByRole("link", { name: "Clear" }).click();
    await expect(plain).toHaveURL(`${saga.baseURL}/features`);
    await expect(plain.locator("[data-directory-row]").filter({ visible: true })).toHaveCount(3);
  } finally {
    await context.close();
  }
});
