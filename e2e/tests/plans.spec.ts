import { writeFileSync } from "node:fs";
import { join } from "node:path";
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

  run(saga, "task", "add", "--plan", "tides", "--id", "tide-source", "--title", "Find a tide source", "--unblocks", "read-tides");

  await page.goto(`${saga.baseURL}/plans/tides`);
  await waitForSettledSaga(page);
  const tabs = page.getByRole("navigation", { name: "Plan views" });

  await tabs.getByRole("link", { name: "Tasks" }).click();
  await expect(page.locator('[data-plan-node="task:tide-source"]')).toHaveCount(1);
  await expect(tabs.getByRole("link", { name: "Tasks" })).toHaveAttribute("aria-current", "page");
  await expect(tabs.getByRole("link")).toHaveCount(2);

  await page.goBack();
  await expect(page.locator('[data-plan-node="task:tide-source"]')).toHaveCount(0);
  await expect(page.locator("[data-plan-deck], [data-plan] [data-deck-viewer]").first()).toBeVisible();

  await page.goForward();
  await expect(page.locator('[data-plan-node="task:tide-source"]')).toHaveCount(1);
});

test("selecting a node on Tasks marks its path and shows its detail", async ({ page, saga }) => {
  run(saga, "plan", "create", "--id", "tides", "--title", "Plan the tides");
  run(saga, "task", "add", "--plan", "tides", "--id", "read-tides", "--title", "Read the tide table",
    "--story", "urn:change-saga:wave-one:story:read-the-tide");
  run(saga, "task", "add", "--plan", "tides", "--id", "tide-source", "--title", "Find a tide source", "--unblocks", "read-tides");

  await page.goto(`${saga.baseURL}/plans/tides/tasks`);
  await waitForSettledSaga(page);

  await page.locator('[data-plan-node="task:read-tides"]').click();
  await expect(page.locator('[data-plan-node="task:tide-source"]')).toHaveClass(/plan-path/);
  await expect(page.locator('[data-plan-node^="story:"]')).toHaveClass(/plan-onward/);
  await expect(page.locator('.diff-drawer.open [data-plan-node-detail="read-tides"]')).toBeVisible();

  // The next node can be selected with the detail still open.
  await page.locator('[data-plan-node^="story:"]').click();
  await expect(page.locator(".diff-drawer.open [data-plan-story-detail]")).toContainText("It takes these 2 tasks, 0 done.");
  await expect(page.locator('[data-plan-node="task:tide-source"]')).toHaveClass(/plan-path/);
});

test("a Task's page shows it selected on the board, within its chain", async ({ page, saga }) => {
  run(saga, "plan", "create", "--id", "tides", "--title", "Plan the tides");
  run(saga, "task", "add", "--plan", "tides", "--id", "read-tides", "--title", "Read the tide table",
    "--story", "urn:change-saga:wave-one:story:read-the-tide");
  run(saga, "task", "add", "--plan", "tides", "--id", "tide-source", "--title", "Find a tide source", "--unblocks", "read-tides");

  await page.goto(`${saga.baseURL}/plans/tides/tasks`);
  await waitForSettledSaga(page);
  await page.locator('[data-plan-node="task:tide-source"]').click();
  await page.locator(".diff-drawer.open").getByRole("link", { name: "Open this task" }).click();

  const board = page.locator("[data-plan-task-board]");
  await expect(board.locator('[data-plan-node="task:tide-source"]')).toHaveClass(/plan-selected/);
  await expect(board.locator('[data-plan-node="task:read-tides"]')).toHaveClass(/plan-onward/);
  await expect(board.locator('[data-plan-node^="story:"]')).toHaveClass(/plan-onward/);

  // The board still works: selecting another node moves the selection.
  await board.locator('[data-plan-node="task:read-tides"]').click();
  await expect(board.locator('[data-plan-node="task:read-tides"]')).toHaveClass(/plan-selected/);
  await expect(page.locator('.diff-drawer.open [data-plan-node-detail="read-tides"]')).toBeVisible();
});

test("a Plan opens on its essay, and a slide's Tasks appear in a slide-out", async ({ page, saga }) => {
  run(saga, "plan", "create", "--id", "tides", "--title", "Plan the tides");
  run(saga, "task", "add", "--plan", "tides", "--id", "read-tides", "--title", "Read the tide table",
    "--story", "urn:change-saga:wave-one:story:read-the-tide");
  const slide = {
    version: 1, operation: "create", request_id: "tide-slide", plan: "tides", expected_snapshot: "absent",
    slide: { id: "the-work", title: "The work", rank: 10, intent: "explain", layout: "diagram", takeaway: "One task reads the tides.", front: ["Read the tide table."], reading_order: ["read-tides"] },
    diagram: {
      version: 1, width: 1280, height: 720,
      styles: { primary: { fill: "@diagram-blue-tint", stroke: "@diagram-blue-accent", ink: "@diagram-blue-ink", font_size: 18, stroke_width: 2 } },
      elements: [{ id: "read-tides", kind: "node", shape: "service", style: "primary", label: "Read the tide table", x: 100, y: 100, width: 400, height: 120 }],
    },
    items: [{ id: "read-tides", rank: 10, kind: "node", label: "Read the tide table", description: "The work.", selector: { type: "element", element_id: "read-tides" }, record: "urn:change-saga:wave-one:plan:tides:task:read-tides" }],
  };
  const request = join(saga.tempDir, "tide-slide.json");
  writeFileSync(request, JSON.stringify(slide));
  run(saga, "apply-slide", "--from", request);

  await page.goto(`${saga.baseURL}/plans/tides`);
  await waitForSettledSaga(page);
  await expect(page.locator("[data-plan-deck] [data-deck-viewer]")).toHaveAttribute("data-deck-face", "overview");

  await page.locator("[data-plan-deck] [data-slide-thumbnail]").click();
  await page.locator('[data-plan-drawer^="plan-tasks-"]').click();
  await expect(page.locator('.diff-drawer.open [data-plan-task-row="read-tides"]')).toContainText("Read the tide table");
});
