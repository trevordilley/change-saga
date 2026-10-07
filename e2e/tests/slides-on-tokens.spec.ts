import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { git, runCLI, startSagaServer, stopSagaServer, type SagaRepositories } from "../support/fixture-builder.js";
import { expect, test } from "../support/test.js";

// A generated slide paints with the design tokens, so it follows the app's
// light and dark mode, including the reviewer's manual toggle. A hand-authored
// slide with fixed colours stays light and sits on a paper card in dark mode.

function cli(repositories: SagaRepositories, ...args: string[]): string {
  const result = runCLI(repositories, args);
  if (result.status !== 0) throw new Error(`change-saga ${args.join(" ")} failed (${result.status})\n${result.stdout}\n${result.stderr}`);
  return result.stdout;
}

const handSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720" role="img" aria-label="Hand drawn"><rect id="box" x="80" y="80" width="480" height="240" fill="#dce8ff"/></svg>\n`;

test("toggling dark mode repaints a generated slide's canvas and nodes, and keeps a hand-authored slide on paper", async ({ browser, sagaRepositories }) => {
  const { sagaRoot, root } = sagaRepositories;
  cli(sagaRepositories, "review", "create", "--id", "pr-4", "--base", "main", "--head", "feature/wave-one", "--pr", "4", "--title", "Themed review", sagaRoot);
  const requestPath = join(root, "themed.json");
  writeFileSync(requestPath, JSON.stringify({
    version: 1, operation: "create", request_id: "themed", review: "pr-4", expected_snapshot: "absent",
    slide: { id: "themed", title: "Themed flow", rank: 10, intent: "explain", layout: "diagram", takeaway: "The flow follows the theme.", reading_order: ["caller"] },
    diagram: { version: 1, width: 1280, height: 720, background: "#fafaf8", elements: [
      { id: "caller", kind: "node", shape: "rect", label: "Caller", x: 120, y: 280, width: 240, height: 110, style: "normal" },
      { id: "greeting", kind: "node", shape: "rect", label: "Greeting", x: 760, y: 280, width: 260, height: 110, style: "primary" },
    ] },
    items: [{ id: "caller", rank: 10, kind: "node", label: "Caller", description: "Who calls.", selector: { type: "element", element_id: "caller" } }],
  }));
  cli(sagaRepositories, "apply-slide", "--from", requestPath, sagaRoot);
  const visual = join(root, "hand.svg");
  writeFileSync(visual, handSVG);
  cli(sagaRepositories, "add-slide", "--review", "pr-4", "--intent", "explain", "--layout", "diagram", "--title", "Hand drawn", "--source", visual, sagaRoot, "hand");
  git(sagaRepositories.sagaRepo, "add", ".");
  git(sagaRepositories.sagaRepo, "commit", "-m", "Themed review");

  const running = await startSagaServer(sagaRepositories);
  // The OS asks for light, so the toggle is what turns the slide dark.
  const context = await browser.newContext({ colorScheme: "light" });
  const page = await context.newPage();
  try {
    await page.goto(`${running.baseURL}/reviews/pr-4`);
    const slide = page.locator('[data-deck-slide][data-slide-target$=":slide:themed"]');
    await expect(slide).toBeVisible();
    const frame = slide.locator("iframe.fragment-frame");
    await expect(frame).not.toHaveAttribute("data-slide-paper");
    const drawing = page.frameLocator('[data-deck-slide][data-slide-target$=":slide:themed"] iframe.fragment-frame');
    const canvas = drawing.locator("svg > rect").first();
    const node = drawing.locator("#caller rect");
    const fill = (locator: typeof canvas) => locator.evaluate(element => getComputedStyle(element).fill);

    await expect.poll(() => fill(canvas)).toBe("rgb(250, 250, 248)");
    await expect.poll(() => fill(node)).toBe("rgb(255, 255, 255)");
    await page.screenshot({ path: test.info().outputPath("slides-on-tokens-light.png") });

    // Target an element first: a scheme change keeps the frame's fragment,
    // so a landmark or reveal target survives the toggle.
    await slide.locator(".landmark-menu [data-landmark-menu-toggle]").click();
    await slide.locator(".landmark-list a", { hasText: "Caller" }).click();
    await expect(frame).toHaveAttribute("src", /#caller$/);

    await page.locator("[data-theme-toggle]").click();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
    await expect(frame).toHaveAttribute("src", /saga_scheme=dark#caller$/);
    await expect.poll(() => fill(canvas)).toBe("rgb(13, 17, 23)");
    await expect.poll(() => fill(node)).toBe("rgb(22, 27, 34)");
    await expect(frame).toHaveCSS("color-scheme", "dark");
    // The compact outline has no duplicate visual frames to repaint.
    await expect(page.locator('.review-deck-rail [data-slide-visual]')).toHaveCount(0);
    await page.screenshot({ path: test.info().outputPath("slides-on-tokens-dark.png") });

    // The hand-authored slide keeps its light frame on the paper card.
    await page.locator('[data-slide-thumbnail][aria-label="Show slide: Hand drawn"]').click();
    const hand = page.locator('[data-deck-slide][data-slide-target$=":slide:hand"] iframe.fragment-frame');
    await expect(hand).toBeVisible();
    await expect(hand).toHaveAttribute("data-slide-paper", "");
    await expect(hand).not.toHaveAttribute("src", /saga_scheme/);
    await expect(hand).toHaveCSS("color-scheme", "light");
    await expect(hand).toHaveCSS("background-color", "rgb(221, 225, 230)");
    await expect(hand).toHaveCSS("padding-top", "12px");
    await page.screenshot({ path: test.info().outputPath("slides-on-tokens-paper.png") });

    // Back to light: the pinned theme now matches the OS, so the URL names no
    // scheme and the slide follows the OS again.
    await page.locator('[data-slide-thumbnail][aria-label="Show slide: Themed flow"]').click();
    await page.locator("[data-theme-toggle]").click();
    await expect(frame).not.toHaveAttribute("src", /saga_scheme/);
    await expect.poll(() => fill(canvas)).toBe("rgb(250, 250, 248)");
    await expect(hand).toHaveCSS("padding-top", "0px");

    // A reload keeps the pinned theme and the slide's scheme.
    await page.locator("[data-theme-toggle]").click();
    await page.reload();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
    await expect.poll(() => fill(canvas)).toBe("rgb(13, 17, 23)");
  } finally {
    await context.close();
    await stopSagaServer(running);
  }
});
