import { readFileSync, writeFileSync } from "node:fs";
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
    slide: { id: "the-work", title: "The work", section: "First phase", rank: 10, intent: "explain", layout: "diagram", takeaway: "One task reads the tides.", front: ["Read the tide table."], reading_order: ["read-tides"] },
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
  await expect(page.locator("[data-plan-deck] .slide-thumbnail-list")).toContainText("First phase");

  await page.locator("[data-plan-deck] [data-slide-thumbnail]").click();
  await page.locator('[data-plan-drawer^="plan-tasks-"]').click();
  await expect(page.locator('.diff-drawer.open [data-plan-task-row="read-tides"]')).toContainText("Read the tide table");

  // Marking the Task done changes what the slide says, with nothing republished.
  await expect(page.locator("[data-plan-drawer^=\"plan-tasks-\"]")).toContainText("0 of 1 tasks done");
  run(saga, "task", "done", "--plan", "tides", "--task", "read-tides");
  await page.goto(`${saga.baseURL}/plans/tides`);
  await waitForSettledSaga(page);
  await expect(page.locator("[data-plan-drawer^=\"plan-tasks-\"]")).toContainText("1 of 1 tasks done");
});

test("a record link in an essay opens that record's page", async ({ page, saga }) => {
  const story = "urn:change-saga:wave-one:story:read-the-tide";
  run(saga, "feature", "add", "--id", "essay-links", "--title", "Essay links");
  run(saga, "feature", "set-overview", "--feature", "essay-links", "--text", `# Essay links\n\nSee [the tide story](${story}) and [nothing](urn:change-saga:wave-one:story:absent).\n`);

  await page.goto(`${saga.baseURL}/features/essay-links`);
  await waitForSettledSaga(page);
  await page.getByRole("link", { name: "the tide story" }).click();
  await expect(page).toHaveURL(/\/requirements\/read-the-tide$/);

  const missing = await page.request.get(`${saga.baseURL}/records/${encodeURIComponent("urn:change-saga:wave-one:story:absent")}`);
  expect(missing.status()).toBe(404);
});

/** A Plan with one done Task pinned to real code, and a slide whose two nodes name it and a story. */
function planWithInteractiveSlide(saga: SagaFixture): void {
  run(saga, "plan", "create", "--id", "tides", "--title", "Plan the tides");
  run(saga, "plan", "add-phase", "--plan", "tides", "--id", "first", "--title", "Read first");
  const essay = join(saga.tempDir, "essay.md");
  writeFileSync(essay, "Why the **tide table** comes first.\n");
  run(saga, "task", "add", "--plan", "tides", "--id", "read-tides", "--title", "Read the tide table", "--phase", "first",
    "--story", "urn:change-saga:wave-one:story:read-the-tide", "--essay-source", essay,
    "--repo", saga.sourceRepo, "--code", "src/app.go:3-5=Where the tide is read");
  run(saga, "task", "add", "--plan", "tides", "--id", "tide-source", "--title", "Find a tide source", "--unblocks", "read-tides");
  run(saga, "task", "done", "--plan", "tides", "--task", "read-tides", "--summary", "Reads the table.");
  const node = { kind: "node", shape: "service", style: "primary", width: 400, height: 120 };
  const slide = {
    version: 1, operation: "create", request_id: "tide-nodes", plan: "tides", expected_snapshot: "absent",
    slide: { id: "the-nodes", title: "The nodes", rank: 10, intent: "explain", layout: "diagram", takeaway: "Each node is work.", front: ["Select a node."], reading_order: ["read-tides", "the-story"] },
    diagram: {
      version: 1, width: 1280, height: 720,
      styles: { primary: { fill: "@diagram-blue-tint", stroke: "@diagram-blue-accent", ink: "@diagram-blue-ink", font_size: 18, stroke_width: 2 } },
      elements: [
        { ...node, id: "read-tides", label: "Read the tide table", x: 100, y: 100 },
        { ...node, id: "the-story", label: "Read the tide", x: 700, y: 100 },
      ],
    },
    items: [
      { id: "read-tides", rank: 10, kind: "node", label: "Read the tide table", description: "The work.", selector: { type: "element", element_id: "read-tides" }, record: "urn:change-saga:wave-one:plan:tides:task:read-tides" },
      { id: "the-story", rank: 20, kind: "node", label: "Read the tide", description: "The story.", selector: { type: "element", element_id: "the-story" }, record: "urn:change-saga:wave-one:story:read-the-tide" },
    ],
  };
  const request = join(saga.tempDir, "tide-nodes.json");
  writeFileSync(request, JSON.stringify(slide));
  run(saga, "apply-slide", "--from", request);
}

test("a Plan slide's node highlights, opens its Task, and its code opens in the drawer", async ({ page, saga }) => {
  planWithInteractiveSlide(saga);
  await page.goto(`${saga.baseURL}/plans/tides`);
  await waitForSettledSaga(page);
  await page.locator("[data-plan-deck] [data-slide-thumbnail]").click();

  const stage = page.locator(".review-deck-slide.active .fragment-stage");
  const taskNode = stage.locator('.landmark-hotspot[data-element-id="read-tides"]');
  await expect(taskNode).toHaveAttribute("data-plan-task-state", "done");
  await expect(taskNode).toHaveCSS("border-top-color", "rgba(0, 0, 0, 0)");
  await taskNode.hover();
  await expect(taskNode).toHaveCSS("border-top-color", "rgb(211, 148, 24)");
  await expect(taskNode.locator(".landmark-affordance")).toHaveCSS("opacity", "1");

  await taskNode.click({ position: { x: 40, y: 60 } });
  const drawer = page.locator(".diff-drawer.open");
  const detail = drawer.locator('[data-plan-node-detail="read-tides"]');
  await expect(detail.locator(".plan-task-title")).toHaveText("Read the tide table");
  await expect(detail.locator(".plan-state.done")).toHaveText("Done");
  await expect(detail.locator("[data-plan-task-phase]")).toContainText("Read first");
  await expect(detail.locator("[data-plan-chain]")).toContainText("Read the tide");
  await expect(detail.locator("[data-plan-task-essay] strong")).toHaveText("tide table");
  await expect(detail.getByRole("link", { name: "Open this task" })).toBeVisible();

  await detail.locator("[data-task-code-list] button").click();
  const code = drawer.locator("[data-task-code-view]");
  await expect(code.locator("figure tr.referenced").first()).toBeVisible();
  await expect(code).toContainText("Where the tide is read");
  await expect(drawer.locator(".drawer-head strong")).toContainText("Read the tide table");

  await drawer.getByRole("button", { name: "Back to task" }).click();
  await expect(detail).toBeVisible();
  await drawer.locator("[data-close-drawer]").click();
  await expect(drawer).toHaveCount(0);

  await stage.locator('.landmark-hotspot[data-element-id="the-story"]').click({ position: { x: 40, y: 60 } });
  await expect(drawer.locator("[data-plan-story-detail]")).toContainText("It takes these 2 tasks, 1 done.");
  await drawer.locator("[data-close-drawer]").click();

  // The slide's own list of Tasks reaches the same code.
  await page.locator('[data-plan-drawer^="plan-tasks-"]').click();
  await drawer.locator('[data-plan-slide-tasks] [data-task-code-list] button').click();
  await expect(drawer.locator("[data-task-code-view] figure")).toContainText("src/app.go");
});

test("a Task's references open from its page and the board, and an unreadable one says so", async ({ page, saga }) => {
  planWithInteractiveSlide(saga);
  await page.goto(`${saga.baseURL}/plans/tides/tasks`);
  await waitForSettledSaga(page);
  await page.locator('[data-plan-node="task:read-tides"]').click();
  const drawer = page.locator(".diff-drawer.open");
  await drawer.locator("[data-task-code-list] button").click();
  await expect(drawer.locator("[data-task-code-view] figure tr.referenced").first()).toBeVisible();
  // The code needs the room the node's narrow detail does not.
  await expect(drawer).not.toHaveClass(/plan-node-drawer/);
  await drawer.getByRole("button", { name: "Back to task" }).click();
  await expect(drawer).toHaveClass(/plan-node-drawer/);
  await drawer.getByRole("link", { name: "Open this task" }).click();

  const taskCode = page.locator("#page [data-task-code] [data-task-code-list] button");
  await taskCode.click();
  await expect(drawer.locator("[data-task-code-view]")).toContainText("Where the tide is read");
  await drawer.locator("[data-close-drawer]").click();
  await expect(taskCode).toBeVisible();

  const record = join(saga.sagaRoot, "___plans/tides.plan/tasks/read-tides.task/task.json");
  const task = JSON.parse(readFileSync(record, "utf8"));
  task.code[0].commit = "a".repeat(40);
  writeFileSync(record, JSON.stringify(task));
  await expect(async () => {
    await page.reload();
    await waitForSettledSaga(page);
    await taskCode.click();
    await expect(drawer.locator("[data-task-code-view]")).toContainText("could not be read from this checkout", { timeout: 2000 });
  }).toPass();
});
