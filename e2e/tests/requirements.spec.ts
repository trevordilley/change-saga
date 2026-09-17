import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { runCLI } from "../support/fixture-builder.js";
import { expectNoSeriousAccessibilityViolations, expect, test, waitForSettledSaga } from "../support/test.js";

test("requirements remain canonical while stories and criteria get dedicated review targets", async ({ page, saga }) => {
  const manifestPath = join(saga.sagaRoot, "saga.json");
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8")) as { $schema?: string; version: number };
  manifest.$schema = "https://changesaga.dev/schema/v3/saga.schema.json";
  manifest.version = 3;
  writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  const statement = "As a reviewer, I can inspect the canonical requirement without duplicated report prose.";
  const result = runCLI(saga, [
    "story", "add",
    "--id", "canonical-review",
    "--revision", "r1",
    "--event", "proposed",
    "--title", "Review canonical requirements",
    "--statement", statement,
    "--priority", "must",
    "--criterion", "stable-targets=Every story and criterion keeps its canonical stable target.",
    saga.sagaRoot
  ]);
  expect(result.status, result.stderr).toBe(0);

  await page.reload();
  await waitForSettledSaga(page);
  await expect(page.getByRole("tabpanel", { name: "Saga" }).getByText(statement)).toHaveCount(0);

  const contents = page.getByRole("navigation", { name: "Contents" });
  await contents.getByRole("link", { name: "Requirements", exact: true }).click();
  await expect(page).toHaveURL(/\/requirements$/);
  await expect(page.getByRole("heading", { name: "Requirements", exact: true })).toBeVisible();
  await expectNoSeriousAccessibilityViolations(page);

  const story = page.locator("article.requirements-story-card", { hasText: "Review canonical requirements" });
  await story.locator("details > summary").click();
  await expect(story.getByText("Every story and criterion keeps its canonical stable target.")).toBeVisible();
  await story.getByRole("link", { name: /Every story and criterion/ }).click();

  await expect(page).toHaveURL(/\/requirements\/canonical-review\/criteria\/stable-targets$/);
  await expect(page.getByRole("heading", { name: "Review canonical requirements", level: 1 })).toBeVisible();
  await expect(page.getByText(statement)).toBeVisible();
  const storyDetails = page.locator("details.requirement-story-details");
  await expect(storyDetails).not.toHaveAttribute("open", "");
  await expect(storyDetails.getByText("Lifecycle", { exact: true })).not.toBeVisible();
  await expectNoSeriousAccessibilityViolations(page);
  const criterion = page.locator('[data-requirement-target="urn:change-saga:wave-one:story:canonical-review:criterion:stable-targets"]');
  await expect(criterion).toBeVisible();
  await expect(criterion).toHaveClass(/selected/);
  await storyDetails.getByText("Details", { exact: true }).click();
  await expect(storyDetails.getByText("Lifecycle", { exact: true })).toBeVisible();
});
