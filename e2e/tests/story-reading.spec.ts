import { expect, test, waitForSettledSaga } from "../support/test.js";
import { runCLI, startSagaServer, stopSagaServer } from "../support/fixture-builder.js";

test("authored feature and story overviews lead into accessible criterion disclosures", async ({ page, sagaRepositories: repo }, testInfo) => {
  const cli = (...args: string[]) => {
    const result = runCLI(repo, args);
    expect(result.status, `${result.stdout}\n${result.stderr}`).toBe(0);
  };
  cli("feature", "set-overview", "--feature", "tide-charts", "--text", "## Purpose\n\nHelp harbor crews **choose a safe departure** with the latest tide information.\n\n## Why this matters\n\nCrews need enough context to understand an uncertain forecast before committing to a journey. The forecast supports their judgment; it does not replace it.", repo.sagaRoot);
  const statement = "As a harbor planner I can choose a safe departure time";
  const criterion = "When the latest forecast is unavailable, the planner can still read the last known tide and its observation time, with a clear explanation that conditions may have changed.";
  cli("story", "add", "--feature", "tide-charts", "--id", "reading-story", "--revision", "r1", "--event", "proposed", "--title", "Choose a safe departure time", "--statement", statement, "--overview", "## The need\n\nA planner compares the departure window with local tide conditions. **Observation time** matters as much as the height.\n\n## Boundaries\n\n- Show the source and time.\n- Explain uncertainty without claiming safety.\n\n<script>window.overviewUnsafe = true</script>\n\n[Unsafe](javascript:alert(1))", "--criterion", `fresh=${criterion}`, "--criterion", "source=The planner can inspect the source of the forecast.", repo.sagaRoot);
  const server = await startSagaServer(repo);
  try {
    await page.goto(`${server.baseURL}/features/tide-charts`);
    await waitForSettledSaga(page);
    await expect(page.getByRole("heading", { name: "Overview", exact: true })).toHaveCount(0);
    await expect(page.getByRole("navigation", { name: "Contents" }).getByRole("link", { name: "Overview", exact: true })).toHaveCount(0);
    await expect(page.locator("[data-feature-overview] strong")).toHaveText("choose a safe departure");
    const summary = page.locator("details").filter({ has: page.getByText("Delivery and coverage", { exact: true }) });
    await expect(summary).not.toHaveAttribute("open", "");
    await page.locator("[data-feature-stories]").getByRole("link", { name: "Choose a safe departure time", exact: true }).click();
    await expect(page).toHaveURL(`${server.baseURL}/requirements/reading-story`);
    await waitForSettledSaga(page);
    await expect(page.locator(".requirement-need")).toHaveText(statement);
    await expect(page.locator("[data-story-overview] strong")).toHaveText("Observation time");
    await expect(page.locator("[data-story-overview] script")).toHaveCount(0);
    await expect(page.locator('[data-story-overview] a[href^="javascript:"]')).toHaveCount(0);
    await expect(page.locator("#nav-features").getByRole("link", { name: "Choose a safe departure time", exact: true })).toBeVisible();
    const first = page.locator("details.requirement-criterion").first();
    await expect(first.locator("summary")).toHaveText(criterion);
    await expect(first).not.toHaveAttribute("open", "");
    await first.locator("summary").focus();
    await page.keyboard.press("Enter");
    await expect(first).toHaveAttribute("open", "");
    await expect(first.getByRole("link", { name: "AC 01 · Open criterion" })).toBeVisible();
    await page.keyboard.press("Space");
    await expect(first).not.toHaveAttribute("open", "");
    const anchor = await first.getAttribute("id");
    await page.goto(`${server.baseURL}/requirements/reading-story#${anchor}`);
    await waitForSettledSaga(page);
    await expect(first).toHaveAttribute("open", "");
    await first.getByRole("link", { name: "AC 01 · Open criterion" }).click();
    await expect(page).toHaveURL(`${server.baseURL}/requirements/reading-story/criteria/fresh`);
    await expect(page.locator("[data-criterion-page] h1")).toHaveText(criterion);
    await page.goto(`${server.baseURL}/requirements/reading-story`);
    await waitForSettledSaga(page);
    await page.screenshot({ path: testInfo.outputPath("story-reading-desktop.png"), fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(first.locator("summary")).toHaveText(criterion);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath("story-reading-mobile.png"), fullPage: true });
    const noJS = await page.context().browser()!.newContext({ javaScriptEnabled: false });
    try {
      const native = await noJS.newPage();
      await native.goto(`${server.baseURL}/requirements/reading-story`);
      const disclosure = native.locator("details.requirement-criterion").first();
      await disclosure.locator("summary").click();
      await expect(disclosure).toHaveAttribute("open", "");
      await disclosure.getByRole("link", { name: "AC 01 · Open criterion" }).click();
      await expect(native.locator("[data-criterion-page] h1")).toHaveText(criterion);
    } finally { await noJS.close(); }
  } finally { await stopSagaServer(server); }
});

test("criteria have readable titles and explanations while retaining full obligations", async ({ page, sagaRepositories: repo }, testInfo) => {
  const cli = (...args: string[]) => {
    const result = runCLI(repo, args);
    expect(result.status, `${result.stdout}\n${result.stderr}`).toBe(0);
  };
  const urn = "urn:change-saga:wave-one:story:criterion-reading";
  const title = "Know how fresh the forecast is";
  const statement = "When the latest forecast is unavailable, the planner can still read the last known tide and its observation time, with a clear explanation that conditions may have changed.";
  cli("story", "add", "--feature", "tide-charts", "--id", "criterion-reading", "--revision", "r1", "--event", "proposed", "--title", "Choose a departure time", "--statement", "As a harbor planner I can choose a departure time", "--criterion", `fresh=${statement}`, "--criterion", "source=The forecast names its source.", repo.sagaRoot);
  cli("criterion", "revise", "--story", urn, "--criterion", `${urn}:criterion:fresh`, "--parent", `${urn}:revision:r1`, "--revision", "r2", "--title", title, "--overview", "## Why this matters\n\nA cached forecast remains useful only when its **observation time** is clear.\n\n## Example\n\nIf the latest update fails, show the previous observation with its original timestamp.\n\n<script>window.criterionUnsafe = true</script>\n\n[Unsafe](javascript:alert(1))", repo.sagaRoot);
  const server = await startSagaServer(repo);
  try {
    await page.goto(`${server.baseURL}/requirements/criterion-reading`);
    await waitForSettledSaga(page);
    const criterion = page.locator("details.requirement-criterion").first();
    await expect(criterion.locator(".criterion-title")).toHaveText(title);
    await expect(criterion.locator(".criterion-statement")).toHaveText(statement);
    await expect(criterion.locator(".criterion-statement")).toBeVisible();
    await expect(criterion.locator("[data-criterion-overview]")).toBeHidden();
    await criterion.locator("summary").focus();
    await page.keyboard.press("Enter");
    await expect(criterion.locator("[data-criterion-overview] strong")).toHaveText("observation time");
    await expect(criterion.locator("[data-criterion-overview] script")).toHaveCount(0);
    await expect(criterion.locator('[data-criterion-overview] a[href^="javascript:"]')).toHaveCount(0);
    await expect(page.locator("#nav-features").getByRole("link", { name: `AC 01 · ${title}`, exact: true })).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath("criterion-story-desktop.png"), fullPage: true });
    await criterion.getByRole("link", { name: "AC 01 · Open criterion" }).click();
    await expect(page.locator("[data-criterion-page] h1")).toHaveText(title);
    await expect(page.locator("[data-criterion-page] .requirement-need")).toHaveText(statement);
    await expect(page.locator("[data-criterion-overview]")).toBeVisible();
    await expect(page.locator("[data-criterion-own-trace]")).not.toHaveAttribute("open", "");
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({ path: testInfo.outputPath("criterion-desktop.png"), fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({ path: testInfo.outputPath("criterion-mobile.png"), fullPage: true });
    const noJS = await page.context().browser()!.newContext({ javaScriptEnabled: false });
    try {
      const native = await noJS.newPage();
      await native.goto(`${server.baseURL}/requirements/criterion-reading`);
      const details = native.locator("details.requirement-criterion").first();
      await details.locator("summary").click();
      await expect(details.locator("[data-criterion-overview]")).toBeVisible();
      await details.getByRole("link", { name: "AC 01 · Open criterion" }).click();
      await expect(native.locator("[data-criterion-page] h1")).toHaveText(title);
    } finally { await noJS.close(); }
  } finally { await stopSagaServer(server); }
});
