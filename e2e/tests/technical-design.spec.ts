import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga, expectNoSeriousAccessibilityViolations } from "../support/test.js";
import { runCLI } from "../support/fixture-builder.js";

test("technical design links its approach, supporting pages and three whiteboards", async ({page, saga}, testInfo) => {
  const run = (...args: string[]) => {
    const result = runCLI(saga, [...args, saga.sagaRoot], saga.sagaRepo);
    expect(result.status, result.stderr + result.stdout).toBe(0);
  };
  run("overview", "set-technical-design", "--subtext", "How requests become deliveries.", "--abstract", "A **queue** separates intake from delivery.", "--text", "Our application uses a queue to coordinate delivery. Read [Reliability](/technical/pages/reliability) and follow the [delivery flow](/technical/whiteboards/data-flows?focus=delivery).");
  run("overview", "set-technical-design", "--page", "reliability", "--title", "Reliability decisions", "--text", "Retries preserve the original request identity. [Return to the approach](/technical).");
  const source = join(saga.root, "design-board.json");
  writeFileSync(source, JSON.stringify({version: 1, width: 1000, height: 700, elements: [
    {id: "delivery", kind: "group", shape: "section", label: "Delivery", width: 900, height: 600, style: "normal"},
    {id: "service", parent: "delivery", kind: "node", shape: "rect", label: "Delivery service", x: 100, y: 100, width: 300, height: 140, style: "normal"}
  ]}));
  for (const [id, name] of [["data-model", "Application ERD"], ["infrastructure", "Deployment infrastructure"], ["data-flows", "Application data flows"]]) {
    run("whiteboard", "publish", "--id", id, "--revision", "r1", "--name", name, "--from", source);
  }
  await page.goto(saga.baseURL + "/technical");
  await waitForSettledSaga(page);
  await expect(page.locator("[data-technical-essay]")).toContainText("Our application uses a queue");
  await expect(page.locator("[data-essay-subtext]")).toHaveText("How requests become deliveries.");
  await expect(page.locator("[data-essay-abstract]")).toHaveText("A queue separates intake from delivery.");
  await expect(page.locator("[data-essay-abstract]")).toHaveCSS("border-left-width", "3px");
  await expect(page.locator("[data-technical-essay]")).toHaveCSS("border-left-width", "0px");
  await expect(page.locator("[data-technical-essay] .fragment")).toHaveCSS("border-left-width", "0px");
  const sidebar = page.locator(".sidebar");
  for (const label of ["ERD", "Infrastructure", "Data flows"]) await expect(sidebar.getByRole("link", {name: label, exact: true})).toBeVisible();
  for (const label of ["Systems", "Components"]) await expect(sidebar.getByRole("link", {name: label, exact: true})).toHaveCount(0);
  await expectNoSeriousAccessibilityViolations(page, "[data-technical-page]");
  await page.screenshot({path: testInfo.outputPath("technical-design.png")});
  await page.locator("[data-technical-essay]").getByRole("link", {name: "Reliability", exact: true}).click();
  await expect(page).toHaveURL(/\/technical\/pages\/reliability$/);
  await expect(page.locator("[data-technical-essay]")).toContainText("Retries preserve");
  await page.getByRole("link", {name: "Return to the approach", exact: true}).click();
  await expect(page).toHaveURL(/\/technical$/);
  await page.locator("[data-technical-essay]").getByRole("link", {name: "delivery flow", exact: true}).click();
  await expect(page.locator("[data-whiteboard-view]")).toHaveAttribute("data-whiteboard-id", "data-flows");
  await expect(sidebar.getByRole("link", {name:"Data flows", exact:true})).toHaveAttribute("aria-current", "page");
});
