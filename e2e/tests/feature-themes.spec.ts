import { expect, test, waitForSettledSaga } from "../support/test.js";
import { runCLI, startSagaServer, stopSagaServer } from "../support/fixture-builder.js";

test("product themes preserve shared feature links, grouping preference and rationale navigation", async ({ page, sagaRepositories: repo }) => {
  const cli = (...args: string[]) => {
    const result = runCLI(repo, args);
    expect(result.status, `${result.stdout}\n${result.stderr}`).toBe(0);
  };
  cli("feature", "add", "--id", "settings", "--title", "Settings", repo.sagaRoot);
  cli("feature-theme", "add", "--id", "t1", "--title", "Reliable work", "--text", "# Why reliable work\n\n**Continuity** matters.", "--feature", "wave-one", "--feature", "tide-charts", repo.sagaRoot);
  cli("feature-theme", "add", "--id", "t2", "--title", "Trusted data", "--text", "Explain confidence.", "--feature", "tide-charts", repo.sagaRoot);
  cli("add-deck", "--feature-theme", "t1", "--id", "why", "--objective", "Explain continuity.", repo.sagaRoot, "why");
  cli("add-slide", "--deck", "why", "--id", "reason", "--title", "Why features belong", "--intent", "explain", "--layout", "hero", repo.sagaRoot, "reason");
  cli("add-item", "--slide", "reason", "--id", "point", "--kind", "node", "--label", "Tide charts", "--description", "Timely information supports continuous work.", "--element-id", "slide-title", "--record", "urn:change-saga:wave-one:feature:tide-charts", repo.sagaRoot);
  const server = await startSagaServer(repo);
  try {
    await page.goto(`${server.baseURL}/features`);
    await waitForSettledSaga(page);
    const sidebar = page.locator("#nav-features");
    const options = page.locator('[data-nav-row="nav-features"] [data-feature-group-control]');
    await expect(options).toBeVisible();
    await expect(page.locator('[data-project-label] + [data-feature-group-control]')).toHaveCount(0);
    await options.locator('summary').press('Enter');
    await expect(options.getByRole('link', { name: 'Theme', exact: true })).toHaveAttribute('aria-current', 'true');
    await options.getByRole('link', { name: 'Theme', exact: true }).press('Escape');
    await expect(options).not.toHaveAttribute('open', '');
    await expect(sidebar.getByRole("link", { name: "Tide Charts", exact: true })).toHaveCount(2);
    await expect(sidebar.getByRole("link", { name: "Harbor Lights", exact: true })).toBeVisible();
    await expect(sidebar.getByText("Ungrouped", { exact: true })).toBeVisible();
    expect(await page.locator("[id]").evaluateAll(nodes => {
      const ids = nodes.map(node => node.id);
      return ids.filter((id, index) => ids.indexOf(id) !== index);
    })).toEqual([]);
    // A category title opens its context; the adjacent arrow only folds its features.
    const category = sidebar.getByRole("link", { name: "Reliable work", exact: true });
    await category.click();
    await expect(page).toHaveURL(`${server.baseURL}/feature-themes/t1`);
    await expect(page.locator("[data-theme-overview] strong")).toHaveText("Continuity");
    await expect(category).toHaveAttribute("aria-current", "page");
    const categoryArrow = sidebar.getByRole("button", { name: "Toggle Reliable work", exact: true });
    await categoryArrow.click();
    await expect(categoryArrow).toHaveAttribute("aria-expanded", "false");
    await expect(page).toHaveURL(`${server.baseURL}/feature-themes/t1`);
    await categoryArrow.click();
    await expect(categoryArrow).toHaveAttribute("aria-expanded", "true");
    await page.getByRole("navigation", { name: "Feature category breadcrumb" }).getByRole("link", { name: "Features", exact: true }).click();
    await expect(page).toHaveURL(`${server.baseURL}/features`);
    await sidebar.getByRole("link", { name: "Tide Charts", exact: true }).first().click();
    await expect(page).toHaveURL(`${server.baseURL}/features/tide-charts`);
    await expect(sidebar.getByRole("link", { name: "Tide Charts", exact: true })).toHaveCount(2);
    await options.locator("summary").click();
    await options.getByRole("link", { name: "Persona", exact: true }).click();
    await page.goto(`${server.baseURL}/requirements/read-the-tide`);
    await waitForSettledSaga(page);
    await options.locator("summary").click();
    await expect(options.getByRole("link", { name: "Persona", exact: true })).toHaveAttribute("aria-current", "true");
    await options.getByRole("link", { name: "None", exact: true }).click();
    await expect(sidebar.getByRole("link", { name: "Tide Charts", exact: true })).toHaveCount(1);
    await page.locator("#sidebar-tabs").getByRole("link", { name: "Reviews", exact: true }).click();
    await expect(page.locator("[data-feature-group-control]")).toBeHidden();
    await page.locator("#sidebar-tabs").getByRole("link", { name: "Documentation", exact: true }).click();
    await expect(page.locator("[data-feature-group-control]")).toBeVisible();
    await page.goto(`${server.baseURL}/feature-themes/t1`);
    await expect(page.locator("[data-theme-overview] strong")).toHaveText("Continuity");
    await page.locator("[data-feature-theme-page] summary").getByText("Rationale deck", { exact: true }).click();
    await page.locator("[data-feature-theme-page]").getByRole("link", { name: "why", exact: true }).click();
    await expect(page.locator("[data-deck-overview]:visible")).toContainText("Explain continuity.");
    await page.getByRole("button", { name: "Back", exact: true }).click();
    const slide = page.locator('[data-deck-slide][data-deck-role="feature-theme"]:visible');
    await expect(slide).toHaveCount(1);
    await slide.getByRole("button", { name: "Jump to a marked place" }).click();
    await slide.getByRole("link", { name: "Open referenced feature tide-charts" }).first().click();
    await expect(page).toHaveURL(`${server.baseURL}/features/tide-charts`);
    await waitForSettledSaga(page);
    await expect(page.locator("[data-feature-page]")).toBeVisible();
    await expect(page.locator("[data-feature-page] h1")).toHaveText("Tide Charts");
    await expect(sidebar.getByRole("link", { name: "Tide Charts", exact: true })).toHaveAttribute("aria-current", "page");
  } finally {
    await stopSagaServer(server);
  }
});
