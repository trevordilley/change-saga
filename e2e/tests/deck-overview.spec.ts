import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "../support/test.js";
import { codeLocation, runCLI, startSagaServer, stopSagaServer, type SagaRepositories } from "../support/fixture-builder.js";

function cli(repo: SagaRepositories, ...args: string[]): void {
  const result = runCLI(repo, args);
  expect(result.status, `${args.join(" ")}\n${result.stdout}\n${result.stderr}`).toBe(0);
}

const visual = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720"><rect width="1280" height="720" fill="white"/><g id="handler"><rect x="100" y="180" width="500" height="280" fill="#ddf"/><text x="140" y="260">Request handler</text></g></svg>`;

test("CLI-authored overview renders report, table, visual and opens exact item evidence in one click", async ({ page, sagaRepositories: repo }) => {
  const asset = join(repo.root, "overview.svg");
  writeFileSync(asset, visual);
  cli(repo, "review", "create", "--id", "overview", "--base", "main", "--head", "feature/wave-one", "--title", "Request review", repo.sagaRoot);
  cli(repo, "add-slide", "--review", "overview", "--intent", "explain", "--layout", "diagram", "--title", "Request flow", "--source", asset, "--front", "Input is checked before dispatch.", "--front", "The response belongs to one handler.", repo.sagaRoot, "request-flow");
  cli(repo, "add-item", "--review", "overview", "--slide", "request-flow", "--id", "handler", "--kind", "node", "--element-id", "handler", "--label", "Request handler", "--description", "Checks the request.", "--record", "urn:change-saga:wave-one:feature:wave-one", repo.sagaRoot);
  cli(repo, "cover", "--repo", repo.sourceRepo, "--target", "urn:change-saga:wave-one:review:overview:slide:request-flow:item:handler", "--ref", codeLocation(repo.identity.head, "src/app.go", 4), repo.sagaRoot);
  cli(repo, "add-slide", "--review", "overview", "--intent", "explain", "--layout", "diagram", "--title", "Response flow", "--source", asset, "--takeaway", "The handler returns a response.", repo.sagaRoot, "response-flow");
  const report = join(repo.root, "overview.json");
  writeFileSync(report, JSON.stringify({
    body: "# Request report\n\nInput reaches the [handler](annotation:handler).\n\n| Stage | Outcome |\n| --- | --- |\n| Validation | Dispatch |\n\n![Request visual](slide:request-flow)",
    annotations: [{ id: "handler", label: "Handler evidence", slide: "request-flow", item: "handler" }]
  }));
  cli(repo, "deck", "overview", "--review", "overview", "--file", report, repo.sagaRoot);
  const running = await startSagaServer(repo);
  try {
    await page.goto(`${running.baseURL}/reviews/overview`);
    const viewer = page.locator("#page [data-deck-viewer]");
    const back = viewer.locator('[data-slide-title="Request flow"] .fragment');
    await expect(back).toBeVisible();
    await viewer.getByRole("button", { name: "Front", exact: true }).click();
    await expect(viewer.locator('[data-slide-title="Request flow"] [data-slide-front]')).toContainText("Input is checked before dispatch.");
    await expect(back).toBeHidden();
    await viewer.getByRole("button", { name: "Next slide", exact: true }).click();
    await expect(viewer.locator('[data-slide-title="Response flow"] [data-slide-front]')).toBeVisible();
    await expect(viewer.getByRole("button", { name: "Front", exact: true })).toHaveAttribute("aria-pressed", "true");
    await viewer.getByRole("button", { name: "Previous slide", exact: true }).click();
    await viewer.getByRole("button", { name: "Overview", exact: true }).click();
    const overview = viewer.locator("[data-deck-overview]");
    await expect(overview).toBeVisible();
    await expect(overview.getByRole("heading", { name: "Request report" })).toBeVisible();
    await expect(overview.getByRole("cell", { name: "Dispatch", exact: true })).toBeVisible();
    const image = overview.getByRole("img", { name: "Request visual" });
    await expect(image).toHaveAttribute("src", "/reviews/overview/visual/request-flow");
    await expect.poll(() => image.evaluate((node: HTMLImageElement) => node.complete && node.naturalWidth > 0)).toBe(true);
    const citation = overview.getByRole("link", { name: "handler", exact: true });
    await citation.focus();
    await expect(overview.getByRole("region", { name: "Evidence preview" })).toContainText("Request flow · Request handler");
    await expect(overview.getByRole("region", { name: "Evidence preview" })).toContainText("src/app.go");
    await page.keyboard.press("Escape");
    await expect(overview.getByRole("region", { name: "Evidence preview" })).toBeHidden();
    await citation.hover();
    await expect(overview.getByRole("region", { name: "Evidence preview" })).toBeVisible();
    const preview = overview.getByRole("region", { name: "Evidence preview" });
    await preview.getByRole("button", { name: "Documentation", exact: true }).hover();
    await expect(preview).toBeVisible();
    await preview.getByRole("button", { name: "Documentation", exact: true }).click();
    await expect(page.locator(".diff-drawer.open")).toContainText("Affected documentation");
    await page.keyboard.press("Escape");
    await expect(viewer.getByRole("button", { name: "Back", exact: true })).toBeFocused();
    await viewer.getByRole("button", { name: "Overview", exact: true }).click();
    await citation.click();
    await expect(back).toBeVisible();
    await expect(viewer.getByRole("button", { name: "Back", exact: true })).toHaveAttribute("aria-pressed", "true");
    await expect(page.locator(".diff-drawer.open [data-review-item-panel=handler]")).toContainText("Checks the request.");
    await expect(page.locator(".diff-drawer.open")).toContainText("src/app.go");
    const itemHash = new URL(page.url()).hash;
    expect(itemHash).toBe("#" + await viewer.locator("[data-review-item=handler]").getAttribute("id"));
    await page.locator("[data-close-drawer]").last().click();
    await viewer.getByRole("button", { name: "Front", exact: true }).click();
    await page.goto(`${running.baseURL}/reviews/overview${itemHash}`);
    await expect(back).toBeVisible();
    await expect(viewer.locator('[data-slide-title="Request flow"] [data-slide-front]')).toBeHidden();
    await page.screenshot({ path: test.info().outputPath("overview-back.png") });
    await viewer.getByRole("button", { name: "Overview", exact: true }).click();
    await page.reload();
    await expect(overview).toBeVisible();
    await page.screenshot({ path: test.info().outputPath("authored-overview.png") });
  } finally { await stopSagaServer(running); }
});

test("legacy implementation deck keeps Back and offers generated directory and takeaway Front", async ({ page, saga }) => {
  cli(saga, "add-deck", "--feature", "wave-one", "--objective", "Explain requests.", saga.sagaRoot, "legacy-overview");
  cli(saga, "add-slide", "--deck", "legacy-overview", "--intent", "explain", "--layout", "diagram", "--title", "Legacy request", "--takeaway", "One handler receives each request.", saga.sagaRoot, "legacy-request");
  await page.reload();
  await page.getByRole("button", { name: "Show slide: Legacy request" }).click();
  const viewer = page.locator("#view-slides [data-deck-viewer]");
  const slide = viewer.locator('[data-deck-slide][data-slide-title="Legacy request"]');
  await expect(slide.locator(".fragment")).toBeVisible();
  await slide.hover();
  await page.locator(".brand").hover();
  await expect.poll(() => slide.locator(".fragment").evaluate(node => {
    const opacities = [];
    for (let current: Element | null = node; current; current = current.parentElement) opacities.push(getComputedStyle(current).opacity);
    return opacities.every(value => value === "1");
  })).toBe(true);
  await viewer.getByRole("button", { name: "Front", exact: true }).click();
  await expect(slide.locator("[data-slide-front]")).toHaveText(/One handler receives each request/);
  await viewer.getByRole("button", { name: "Overview", exact: true }).click();
  const overview = viewer.locator("[data-deck-overview]:visible");
  await expect(overview.locator("[data-overview-generated]")).toContainText("Generated slide directory");
  await overview.getByRole("link", { name: "Legacy request", exact: true }).click();
  await expect(slide.locator(".fragment")).toBeVisible();
  await expect(viewer.locator("[data-slide-position]")).toHaveText("1 / 1");
});

test("one implementation citation offers both linked stories and exact code without another navigation click", async ({ page, saga }) => {
  const asset = join(saga.root, "technical-overview.svg");
  writeFileSync(asset, visual);
  cli(saga, "add-deck", "--feature", "wave-one", "--objective", "Trace request intent and implementation.", saga.sagaRoot, "request-overview");
  cli(saga, "add-slide", "--deck", "request-overview", "--source", asset, "--intent", "explain", "--layout", "diagram", "--title", "Technical request", saga.sagaRoot, "technical-request");
  cli(saga, "add-item", "--slide", "technical-request", "--id", "handler", "--kind", "node", "--element-id", "handler", "--label", "Request handler", "--description", "Checks input before dispatch.", saga.sagaRoot);
  const item = "urn:change-saga:wave-one:slide:technical-request:item:handler";
  const story = "urn:change-saga:wave-one:story:overview-request";
  cli(saga, "cover", "--repo", saga.sourceRepo, "--against", "main", "--target", item, "--path", "src/app.go", "--side", "new", "--lines", "3", "--name", "overview-handler", saga.sagaRoot);
  cli(saga, "story", "add", "--feature", "wave-one", "--id", "overview-request", "--revision", "r1", "--event", "proposed", "--title", "Understand request intent", "--statement", "A reviewer can trace input validation.", "--criterion", "visible=Input validation is visible", saga.sagaRoot);
  cli(saga, "relation", "add", "--feature", "wave-one", "--id", "overview-explains", "--type", "explains", "--from", item, "--to", story, "--rationale", "The handler checks input.", saga.sagaRoot);
  const report = join(saga.root, "technical-overview.json");
  writeFileSync(report, JSON.stringify({ body: "The [handler](annotation:handler) checks input.", annotations: [{ id: "handler", label: "Handler evidence", slide: "technical-request", item: "handler" }] }));
  cli(saga, "deck", "overview", "--deck", "request-overview", "--file", report, saga.sagaRoot);
  await page.reload();
  await page.getByRole("button", { name: "Show slide: Technical request" }).click();
  const viewer = page.locator("#view-slides [data-deck-viewer]");
  const frame = viewer.locator('[data-slide-title="Technical request"] iframe.fragment-frame');
  await frame.hover();
  await page.locator(".brand").hover();
  await expect.poll(() => frame.evaluate(node => {
    for (let current: Element | null = node; current; current = current.parentElement) {
      if (getComputedStyle(current).opacity !== "1") return false;
    }
    return true;
  })).toBe(true);
  await viewer.getByRole("button", { name: "Overview", exact: true }).click();
  const overview = viewer.locator("[data-deck-overview]:visible");
  await overview.getByRole("link", { name: "handler", exact: true }).focus();
  const preview = overview.getByRole("region", { name: "Evidence preview" });
  await expect(preview).toContainText("Understand request intent");
  await preview.getByRole("button", { name: "Stories", exact: true }).click();
  await expect(page.locator(".diff-drawer.open [data-story-link]")).toContainText("Understand request intent");
  await page.keyboard.press("Escape");
  await expect(viewer.getByRole("button", { name: "Back", exact: true })).toBeFocused();
  await viewer.getByRole("button", { name: "Overview", exact: true }).click();
  await overview.locator(".overview-references").getByRole("button", { name: "Code / diff", exact: true }).click();
  await expect(page.locator(".diff-drawer.open")).toContainText("src/app.go");
  await page.keyboard.press("Escape");
  await expect(viewer.getByRole("button", { name: "Back", exact: true })).toBeFocused();
});

test("an empty review still has an authored Overview and a stable overview link", async ({ page, sagaRepositories: repo }) => {
  cli(repo, "review", "create", "--id", "empty-overview", "--base", "main", "--head", "feature/wave-one", "--title", "Planned review", repo.sagaRoot);
  const report = join(repo.root, "empty-overview.json");
  writeFileSync(report, JSON.stringify({ body: "# Review plan\n\nExplain the request before publishing its slides." }));
  cli(repo, "deck", "overview", "--review", "empty-overview", "--file", report, repo.sagaRoot);
  const running = await startSagaServer(repo);
  try {
    await page.goto(`${running.baseURL}/reviews/empty-overview`);
    const viewer = page.locator("#page [data-deck-viewer]");
    await expect(viewer.getByRole("heading", { name: "Review plan" })).toBeVisible();
    await expect(viewer.getByRole("button", { name: "Front", exact: true })).toBeDisabled();
    await viewer.getByRole("button", { name: "Overview", exact: true }).click();
    expect(new URL(page.url()).hash).toContain("overview-");
    await page.reload();
    await expect(viewer.getByRole("heading", { name: "Review plan" })).toBeVisible();
    await expect(viewer.locator("[data-deck-slide]")).toHaveCount(0);
  } finally { await stopSagaServer(running); }
});
