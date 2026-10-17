import { type Page, type Locator } from "@playwright/test";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { git, runCLI } from "../support/fixture-builder.js";
import { expectNoSeriousAccessibilityViolations, expect, test } from "../support/test.js";

// Details are requested from compact badges; the canvas never carries an open panel.
async function inspectDeprecation(page: Page, reason: string, scope?: Locator) {
  await (scope ?? page.locator("body")).locator('[data-deprecation-list]:visible').first().click();
  await page.locator('.diff-drawer').getByRole('button', { name: reason, exact: true }).click();
  await expect(page.locator('.diff-drawer [data-deprecations]')).toBeVisible();
}

test("deprecation preserves exact code scope, board reasons and withdrawal history", async ({ page, saga }, testInfo) => {
  const cli = (args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, result.stdout + result.stderr).toBe(0);
    return result.stdout;
  };
  const term = JSON.parse(cli(["query", "terms", "--saga", saga.sagaRoot, "--repo", saga.sourceRepo, "--term", "greeting"])).data.terms[0];
  const code = term.code[0].head;
  const ref = `${code.commit}:${code.path}#L${code.start}-L${code.end}`;
  const board = join(saga.root, "legacy-board.json");
  writeFileSync(board, JSON.stringify({ version: 1, width: 600, height: 400, elements: [
    { id: "legacy", kind: "node", shape: "rect", label: "Legacy greeting", x: 20, y: 20, width: 250, height: 120, style: "normal" }
  ] }));
  cli(["whiteboard", "publish", "--id", "legacy", "--revision", "b1", "--name", "Legacy greeting", "--from", board, saga.sagaRoot]);
  const target = "urn:change-saga:wave-one:whiteboard:legacy";
  cli(["deprecation", "add", "--id", "old-explanation", "--scope", "content", "--target", target, "--element", "legacy",
    "--reason", "This diagram describes the retired greeting flow.", "--repo", saga.sourceRepo, "--ref", ref, saga.sagaRoot]);
  await page.goto(saga.baseURL + "/technical/whiteboards/legacy");
  await inspectDeprecation(page, "This diagram describes the retired greeting flow.");
  await expect(page.locator("[data-deprecations]")).toContainText("This diagram describes the retired greeting flow.");
  await expect(page.locator('[data-deprecation-scope="content"]')).toContainText("appearance legacy");
  await expect(page.locator('[data-instance-path="legacy"]')).toHaveCount(1);
  await page.goto(saga.baseURL + "/terms/greeting");
  await page.locator("details.term-code-list > summary").click();
  await inspectDeprecation(page, "This diagram describes the retired greeting flow.");
  const panel = page.locator(".diff-drawer [data-deprecations]");
  await expect(panel).toContainText("Related code only");
  await expect(panel.locator('[data-code-role="deprecated_code"]')).toHaveCount(0);
  cli(["deprecation", "add", "--id", "old-code", "--scope", "code", "--target", target,
    "--reason", "Use the replacement greeting implementation.", "--repo", saga.sourceRepo, "--ref", ref, saga.sagaRoot]);
  await page.reload();
  await page.locator("details.term-code-list > summary").click();
  await inspectDeprecation(page, "Use the replacement greeting implementation.");
  await expect(panel.locator('[data-code-role="deprecated_code"]')).toContainText("Authored deprecated code");
  await expect(panel.locator('[data-code-role="deprecated_code"]')).toContainText("exact");
  await expectNoSeriousAccessibilityViolations(page);
  await page.screenshot({ path: testInfo.outputPath("deprecation-code.png"), fullPage: true });
  const dep = "urn:change-saga:wave-one:deprecation:old-code";
  cli(["deprecation", "set-state", "--deprecation", dep, "--event", "withdraw", "--parent", dep + ":event:active",
    "--state", "withdrawn", "--reason", "The implementation is still supported; only its explanation is obsolete.", saga.sagaRoot]);
  await page.reload();
  await page.locator("details.term-code-list > summary").click();
  await inspectDeprecation(page, "Use the replacement greeting implementation.");
  await expect(panel.locator('[data-deprecation-state="withdrawn"]')).toContainText("Deprecation withdrawn");
  await expect(panel.locator('[data-deprecation-state="withdrawn"]')).toContainText("still supported");
  // Code Diff and narrative fragments arrive through the browser's own lazy
  // loader, so their annotation placeholders must also be activated.
  await page.goto(saga.baseURL + "/reviews?view=code&file=src%2Fapp.go");
  await inspectDeprecation(page, "Use the replacement greeting implementation.", page.locator(".code-main"));
  await expect(panel.locator('[data-deprecation-state="withdrawn"]')).toContainText("still supported");
  cli(["deprecation", "add", "--id", "old-narrative", "--scope", "content", "--target", "urn:change-saga:wave-one:fragment:diagram",
    "--reason", "This explanatory fragment has been superseded.", saga.sagaRoot]);
  await page.goto(saga.featureURL);
  const fragment = page.locator('[data-target="urn:change-saga:wave-one:fragment:diagram"]').first();
  await fragment.scrollIntoViewIfNeeded();
  await expect(fragment.locator("[data-deprecations]")).toHaveCount(0);
  await inspectDeprecation(page, "This explanatory fragment has been superseded.", fragment);
  await expect(page.locator(".diff-drawer [data-deprecations]")).toContainText("This explanatory fragment has been superseded.");
  const records = JSON.parse(cli(["query", "deprecations", "--saga", saga.sagaRoot, "--repo", saga.sourceRepo, "--state", "active"]));
  expect(records.data.deprecations.map((row: { deprecation: string }) => row.deprecation)).toEqual(["urn:change-saga:wave-one:deprecation:old-explanation", "urn:change-saga:wave-one:deprecation:old-narrative"]);
});

test("review Items retain annotations and inspect code at the review commit", async ({ page, saga }, testInfo) => {
  const cli = (args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, result.stdout + result.stderr).toBe(0);
    return result.stdout;
  };
  cli(["review", "create", "--id", "old-review", "--base", "main", "--head", saga.identity.head, "--title", "Historical implementation", saga.sagaRoot]);
  const visual = join(saga.root, "historical.svg");
  writeFileSync(visual, '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720"><rect id="legacy" x="80" y="80" width="400" height="200"/><text x="80" y="330">Historical implementation</text></svg>');
  cli(["add-slide", "--review", "old-review", "--intent", "explain", "--layout", "diagram", "--title", "Historical implementation", "--source", visual, saga.sagaRoot, "old"]);
  cli(["add-item", "--review", "old-review", "--slide", "old", "--kind", "node", "--element-id", "legacy", "--label", "Legacy", "--description", "Historical implementation", saga.sagaRoot]);
  const target = "urn:change-saga:wave-one:review:old-review:slide:old:item:legacy";
  cli(["deprecation", "add", "--id", "review-code", "--target", target, "--scope", "code", "--ref", `${saga.identity.head}:src/app.go#L3-L5`, "--repo", saga.sourceRepo,
    "--reason", "The reviewed implementation is deprecated.", saga.sagaRoot]);
  writeFileSync(join(saga.sourceRepo, "src/app.go"), "package demo\nfunc Replacement() string { return \"current\" }\n");
  git(saga.sourceRepo, "commit", "-am", "Replace the current implementation after the review");
  await page.goto(saga.baseURL + "/reviews/old-review");
  await expect(page.locator(".diff-drawer [data-deprecations]")).toHaveCount(0);
  await inspectDeprecation(page, "The reviewed implementation is deprecated.", page.locator(".review-fragment"));
  await page.screenshot({ path: testInfo.outputPath("deprecation-review.png") });
  await expect(page.locator(".diff-drawer [data-deprecations]")).toContainText("The reviewed implementation is deprecated.");
  await expect(page.locator(".diff-drawer [data-deprecations]")).toContainText(target);
  await page.keyboard.press("Escape");
  await page.getByRole("tab", { name: "Code Diff", exact: true }).click();
  await page.locator('.file-tree a[data-tree-path="src/app.go"]').click();
  await inspectDeprecation(page, "The reviewed implementation is deprecated.", page.locator(".code-main"));
  const evidence = page.locator('.diff-drawer [data-code-role="deprecated_code"]');
  await expect(evidence).toContainText("current · exact");
  await expect(evidence).not.toContainText("uncertain_path");
});
