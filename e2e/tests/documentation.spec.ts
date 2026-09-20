import type { Page } from "@playwright/test";
import { git, serverRequest, startSagaServer, stopSagaServer, treeSnapshot } from "../support/fixture-builder.js";
import { expect, openReviewSide, test, waitForSettledSaga } from "../support/test.js";

// The Saga is documentation. Stories, designs, fragments, chapters, decks,
// slides, and Items carry no approval, no comment, and no annotation control in
// either way the reviewer can be opened; approvals belong to the pull request.
const reviewControls = [
  "[data-review-decision]",
  "[data-review-comment]",
  "[data-review-controls]",
  "[data-review-progress]",
  "[data-annotation-tools]",
  ".annotation-toolbox",
  "[data-sticky-note]",
  "[data-annotation-bubble]",
  "[data-open-activity]",
  "[data-line-select]",
  "[data-selection-toolbar]",
  "[data-diff-action]",
  "form.file-review",
  'form[action="/api/thread"]',
  'form[action="/api/review"]',
  'form[action="/api/diff-review"]'
].join(",");

async function expectNoReviewControls(page: Page, where: string): Promise<void> {
  await expect(page.locator(reviewControls), `review controls on ${where}`).toHaveCount(0);
}

async function expectDocumentationOnly(page: Page, mode: "observe" | "compare"): Promise<void> {
  await expect(page.locator("[data-opening]")).toHaveAttribute("data-opening", mode);
  await expect(page.getByText("Wave 1 connects the story")).toBeVisible();
  await expectNoReviewControls(page, `the ${mode} Saga`);

  // Documentation keeps the documented code: the same references resolved at
  // the head rather than a diff.
  await page.getByRole("tab", { name: "Documented code" }).click();
  await expect(page.getByRole("tabpanel", { name: "Documented code" })).toBeVisible();
  await expectNoReviewControls(page, `${mode} documented code`);
  await expect(page.getByRole("tab", { name: "Code Diff" })).toHaveCount(0);

  // Code Diff and coverage of a change are comparison views: they are on the
  // Review side, and observing one commit has no comparison to offer at all.
  await openReviewSide(page);
  if (mode === "compare") {
    await page.getByRole("tab", { name: "Code Diff" }).click();
    await expect(page.locator("article.file-diff").first()).toBeVisible();
    await expectNoReviewControls(page, `${mode} Code Diff`);
    await page.getByRole("tab", { name: "Coverage" }).click();
    await expect(page.getByRole("tabpanel", { name: "Coverage" })).toBeVisible();
    await expectNoReviewControls(page, `${mode} Coverage`);
  } else {
    await expect(page.getByRole("tab", { name: "Code Diff" })).toHaveCount(0);
  }

  await page.goto(new URL("/chapters/architecture", page.url()).toString());
  await waitForSettledSaga(page);
  await expect(page.getByRole("tabpanel", { name: "Saga" }).getByText("The renderer and persistence boundary stay independent.")).toBeVisible();
  await expectNoReviewControls(page, `a ${mode} chapter`);
}

test("@critical renders no approval, comment, or annotation control when comparing a change", async ({ page, saga }) => {
  await expectDocumentationOnly(page, "compare");
  // The layers still mark what the change edited and affected, but they no
  // longer gate an approval.
  await expect(page.locator("body")).toHaveAttribute("data-layers-ready", "true");
  const layers = JSON.parse((await serverRequest(saga.baseURL, "/api/layers")).body) as Record<string, unknown>;
  expect(layers.mode).toBe("compare");
  expect(layers).not.toHaveProperty("approvable");
  await expect(page.locator(".layer-changed,.layer-affected").first()).toBeAttached();
});

test("@critical renders no approval, comment, or annotation control when observing the head", async ({ page, saga }) => {
  const observing = await startSagaServer(saga, null);
  try {
    await page.goto(`${observing.baseURL}/features/wave-one`);
    await waitForSettledSaga(page);
    await expectDocumentationOnly(page, "observe");
  } finally {
    await stopSagaServer(observing);
  }
});

test("@critical reading the Saga and its history never writes to either repository", async ({ page, saga }) => {
  expect(saga.sagaRepo).not.toBe(saga.sourceRepo);
  const sagaTree = treeSnapshot(saga.sagaRoot);
  const sagaStatus = git(saga.sagaRepo, "status", "--short");
  const sourceHead = git(saga.sourceRepo, "rev-parse", "HEAD");
  const sourceStatus = git(saga.sourceRepo, "status", "--short");

  // A record's history opens in the drawer: when it was introduced and every
  // commit that changed it.
  const overview = page.locator('[data-fragment-title="Overview"]');
  const historyButton = overview.getByRole("button", { name: "History of Overview" });
  await overview.hover();
  await historyButton.click();
  const drawer = page.locator("#review-drawer");
  await expect(drawer).toHaveAttribute("aria-hidden", "false");
  await expect(drawer).toHaveAttribute("data-drawer-mode", "history");
  await expect(drawer.locator("[data-review-surface=\"history\"] .history-wrap")).toBeVisible();
  await expect(drawer.locator(".history-wrap h1")).toHaveText("Overview");
  await page.keyboard.press("Escape");
  await expect(drawer).toHaveAttribute("aria-hidden", "true");
  await expect(historyButton).toBeFocused();

  await openReviewSide(page);
  await page.getByRole("tab", { name: "Code Diff" }).click();
  await expect(page.locator("article.file-diff").first()).toBeVisible();
  await page.getByRole("tab", { name: "Coverage" }).click();
  await page.getByRole("tab", { name: "Reviews" }).click();
  await page.reload();
  await waitForSettledSaga(page);

  expect(treeSnapshot(saga.sagaRoot), "saga tree after reading").toBe(sagaTree);
  expect(git(saga.sagaRepo, "status", "--short")).toBe(sagaStatus);
  expect(git(saga.sourceRepo, "rev-parse", "HEAD")).toBe(sourceHead);
  expect(git(saga.sourceRepo, "status", "--short")).toBe(sourceStatus);
});
