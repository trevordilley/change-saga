import { expect, test, waitForSettledSaga } from "../support/test.js";

// A page that takes a moment to arrive says so: a spinner shows while the
// navigation waits, and goes once the page is swapped in.
test("a slow page shows a loading spinner until it arrives", async ({ page, saga }) => {
  await waitForSettledSaga(page);
  const indicator = page.locator("#page-loading");
  await expect(indicator).toBeHidden();

  // Hold the Reviews page back long enough that the spinner must appear.
  let release: () => void = () => {};
  const held = new Promise<void>((resolve) => { release = resolve; });
  await page.route((url) => url.pathname === "/reviews", async (route) => {
    await held;
    await route.continue();
  });
  await page.getByRole("link", { name: "Reviews", exact: true }).click();
  await expect(indicator).toBeVisible();
  await expect(indicator).toHaveText("Loading…");
  await expect(page.locator("#page")).toHaveAttribute("aria-busy", "true");

  release();
  await expect(page).toHaveURL(`${saga.baseURL}/reviews`);
  await expect(page.getByRole("heading", { name: "Reviews", level: 1 })).toBeVisible();
  await expect(indicator).toBeHidden();
  await expect(page.locator("#page")).not.toHaveAttribute("aria-busy", "true");
});
