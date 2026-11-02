import { runCLI, type SagaFixture } from "../support/fixture-builder.js";
import { expect, test, waitForSettledSaga } from "../support/test.js";

/**
 * A Plan's tabs are separate pages. The shell keeps a page's own views on the
 * client without asking the server, so these must be followed by a real click:
 * loading their URLs directly would pass even when no click can reach them.
 */

function run(saga: SagaFixture, ...args: string[]): void {
  const result = runCLI(saga, [...args, saga.sagaRoot], saga.sagaRepo);
  expect(result.status, `${args[0]} failed\n${result.stdout}\n${result.stderr}`).toBe(0);
}

test("a Plan's tabs can be followed, and back returns to the last one", async ({ page, saga }) => {
  run(saga, "plan", "create", "--id", "tides", "--title", "Plan the tides");
  run(saga, "task", "add", "--plan", "tides", "--id", "read-tides", "--title", "Read the tide table",
    "--story", "urn:change-saga:wave-one:story:read-the-tide");

  await page.goto(`${saga.baseURL}/plans/tides`);
  await waitForSettledSaga(page);
  const tabs = page.getByRole("navigation", { name: "Plan views" });
  await expect(page.locator("[data-plan-deck]")).toHaveCount(1);

  await tabs.getByRole("link", { name: "Dependencies" }).click();
  await expect(page.locator('[data-plan-graph-task="read-tides"]')).toHaveCount(1);
  await expect(tabs.getByRole("link", { name: "Dependencies" })).toHaveAttribute("aria-current", "page");
  await expect(page.locator("[data-plan-deck]")).toHaveCount(0);

  await tabs.getByRole("link", { name: "Tasks" }).click();
  await expect(page.locator("[data-plan-tasks]")).toHaveCount(1);

  await page.goBack();
  await expect(page.locator('[data-plan-graph-task="read-tides"]')).toHaveCount(1);

  await tabs.getByRole("link", { name: "Deck" }).click();
  await expect(page.locator("[data-plan-deck]")).toHaveCount(1);
});
