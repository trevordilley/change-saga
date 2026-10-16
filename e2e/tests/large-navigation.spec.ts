import { writeFileSync } from "node:fs";
import { expect, test } from "../support/test.js";
import { runCLI, startSagaServer, stopSagaServer } from "../support/fixture-builder.js";

// No private Saga or installed binary is needed: CI authors its own fixture
// before starting the server, then exercises real HTMX clicks in Chromium.
test("@navigation-performance a 104-slide Saga keeps ordinary navigation bounded", async ({ page, sagaRepositories: saga }, testInfo) => {
  test.setTimeout(180_000);
  const cli = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, result.stdout + result.stderr).toBe(0);
  };
  cli("add-deck", "--feature", "wave-one", "--objective", "Exercise large deck navigation.", saga.sagaRoot, "large-navigation");
  for (let index = 0; index < 104; index++) {
    cli("add-slide", "--deck", "large-navigation", "--intent", "explain", "--layout", "diagram", "--title", `Navigation ${index}`, saga.sagaRoot, `navigation-${index}`);
  }
  cli("deprecation", "add", "--id", "last-slide", "--scope", "content", "--target", "urn:change-saga:wave-one:slide:navigation-103", "--reason", "The last slide is superseded.", saga.sagaRoot);

  const server = await startSagaServer(saga);
  let decks = 0;
  const annotations: string[] = [];
  const frames: string[] = [];
  const metrics: object[] = [];
  page.on("request", request => {
    const url = new URL(request.url());
    if (url.pathname === "/decks") decks++;
    if (url.pathname === "/api/deprecations") annotations.push(url.searchParams.get("target") || "code");
    if (request.isNavigationRequest() && request.frame() !== page.mainFrame()) frames.push(url.pathname);
  });
  try {
    for (const phase of ["cold", "warm"]) {
      const start = Date.now();
      const response = await page.goto(server.baseURL);
      await page.locator("body[data-shell-ready]").waitFor();
      const rootMs = Date.now() - start;
      const clicks: { path: string; milliseconds: number }[] = [];
      const measured = { phase, slides: 104, rootMs, documentBytes: (await response!.body()).length, clicks, elements: await page.locator("*").count() };
      metrics.push(measured);
      for (const path of ["/features", "/technical", "/features"]) {
        const start = Date.now();
        try {
          await page.locator(`a[href="${path}"]:visible`).first().click();
          await expect(page).toHaveURL(server.baseURL + path);
          await page.locator("body[data-shell-ready]").waitFor();
        } finally {
          clicks.push({ path, milliseconds: Date.now() - start });
        }
      }
      Object.assign(measured, { decks, frames: [...frames], annotations: [...annotations], elements: await page.locator("*").count() });
      expect(decks, "ordinary pages must not request the hidden viewer").toBe(0);
      expect(frames, "hidden slide assets must not compete with navigation").toEqual([]);
      expect(annotations.filter(target => target.includes(":slide:") || target.includes(":deck:"))).toEqual([]);
      await expect(page.locator("[data-deck-slide]")).toHaveCount(0);
      // Only an order-of-magnitude smoke ceiling: exact times are diagnostics.
      expect(Math.max(...clicks.map(click => click.milliseconds))).toBeLessThan(10_000);
    }

    await page.goto(server.baseURL + "/?view=slides");
    await page.locator("body[data-shell-ready]").waitFor();
    await expect(page.locator("[data-deck-slide]")).toHaveCount(104);
    await page.waitForLoadState("networkidle");
    expect(decks).toBe(1);
    expect(frames.length, "only the active slide and its immediate neighbour load").toBeLessThanOrEqual(2);
    expect(annotations.filter(target => target.includes(":slide:"))).toHaveLength(1);
    const last = page.locator('[data-deck-slide][data-slide-target="urn:change-saga:wave-one:slide:navigation-103"] .fragment');
    const anchor = await last.getAttribute("id");
    // A hash-only permalink must reveal annotations on the last slide without
    // depending on an earlier visit or on the explicit ?view=slides parameter.
    await page.goto(server.baseURL + "/#" + anchor);
    await page.locator("body[data-shell-ready]").waitFor();
    await expect(last).toBeVisible();
    await expect(last.locator("[data-deprecation-summary]")).toBeVisible();
    await last.getByRole("button", {name:"Deprecations on this content"}).click();
    await page.locator(".diff-drawer").getByRole("button", {name:"The last slide is superseded.", exact:true}).click();
    await expect(page.locator(".diff-drawer [data-deprecations]")).toContainText("The last slide is superseded.");
    metrics.push({ phase: "opened-and-deep-linked", decks, frames, annotations });
  } finally {
    metrics.push({ phase: "final-requests", decks, frames, annotations });
    const metricsPath = testInfo.outputPath("large-navigation-metrics.json");
    writeFileSync(metricsPath, JSON.stringify(metrics, null, 2) + "\n");
    await testInfo.attach("large-navigation-metrics.json", { path: metricsPath, contentType: "application/json" });
    await testInfo.attach("server.log", { body: server.serverLog.join(""), contentType: "text/plain" });
    await stopSagaServer(server);
  }
});
