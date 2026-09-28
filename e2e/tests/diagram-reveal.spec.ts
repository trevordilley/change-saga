import { writeFileSync } from "node:fs";
import { join } from "node:path";
import type { Frame, Locator } from "@playwright/test";
import { runCLI, startSagaServer, stopSagaServer, type SagaRepositories } from "../support/fixture-builder.js";
import { expect, test } from "../support/test.js";

// A diagram with a reveal fades in, step by step, each time its slide is
// shown. The animation is CSS in the SVG keyed to a fragment the viewer sets
// on the sandboxed frame, so an element landmark link, which sets its own
// fragment, shows the finished drawing, and so does reduced motion.

function cli(repositories: SagaRepositories, ...args: string[]): string {
  const result = runCLI(repositories, args);
  if (result.status !== 0) throw new Error(`change-saga ${args.join(" ")} failed (${result.status})\n${result.stdout}\n${result.stderr}`);
  return result.stdout;
}

const node = (id: string, label: string, x: number, step?: number) => ({ id, kind: "node", shape: "rect", label, x, y: 300, width: 160, height: 80, style: "normal", ...(step ? { step } : {}) });

const revealDiagram = {
  version: 1, width: 1280, height: 720, reveal: "fade",
  elements: [
    { id: "heading", kind: "text", label: "Unfolding", x: 60, y: 28, width: 900, height: 48, style: "title", decorative: true },
    node("one", "One", 40), node("two", "Two", 240), node("three", "Three", 440),
    node("four", "Four", 640), node("five", "Five", 840), node("six", "Six", 1040, 2),
  ],
};

function publish(repositories: SagaRepositories, id: string, title: string, rank: number, diagram: object): string {
  const path = join(repositories.root, `${id}.json`);
  writeFileSync(path, JSON.stringify({
    version: 1, operation: "create", request_id: id, review: "pr-3", expected_snapshot: "absent",
    slide: { id, title, rank, intent: "explain", layout: "diagram", takeaway: `${title} slide.`, reading_order: ["five"] },
    diagram,
    items: [{ id: "five", rank: 10, kind: "node", label: "Five", description: "The fifth node.", selector: { type: "element", element_id: "five" } }],
  }));
  return JSON.parse(cli(repositories, "apply-slide", "--from", path, "--json", repositories.sagaRoot)).target;
}

async function slideFrame(slide: Locator): Promise<Frame> {
  const frame = await (await slide.locator("iframe[data-fragment-frame]").elementHandle())?.contentFrame();
  if (!frame) throw new Error("the slide has no frame");
  return frame;
}

// running names the reveal animations the frame is playing now.
const running = (frame: Frame) => frame.evaluate(() => [...new Set(document.getAnimations().map(animation => (animation as CSSAnimation).animationName))]);
const opacity = (frame: Frame, id: string) => frame.evaluate(target => getComputedStyle(document.getElementById(target)!).opacity, id);

test("a diagram's reveal replays each time its slide is shown, and landmark links show the finished drawing", async ({ page, sagaRepositories }) => {
  const { sagaRoot } = sagaRepositories;
  cli(sagaRepositories, "review", "create", "--id", "pr-3", "--base", "main", "--head", "feature/wave-one", "--pr", "3", "--title", "Reveal review", sagaRoot);
  const target = publish(sagaRepositories, "unfolding", "Unfolding", 10, revealDiagram);
  publish(sagaRepositories, "plain", "Plain", 20, { ...revealDiagram, reveal: undefined, elements: revealDiagram.elements.map(element => ({ ...element, step: undefined })) });
  expect(cli(sagaRepositories, "diagram", "describe", "--slide", target, sagaRoot)).toContain("Reveal: fade in 5 steps");

  const server = await startSagaServer(sagaRepositories);
  try {
    await page.goto(`${server.baseURL}/reviews/pr-3`);
    const slide = page.locator(`[data-deck-slide][data-slide-target="${target}"]`);
    await expect(slide).toBeVisible();
    const frameElement = slide.locator("iframe[data-fragment-frame]");
    await expect(frameElement).toHaveAttribute("src", /\/visual\/unfolding\?saga_aspect=[^#]*#diagram-reveal$/);
    let frame = await slideFrame(slide);
    // The first steps fade in while later ones wait their turn.
    await expect.poll(() => running(frame), { intervals: [50] }).toEqual(["diagram-reveal-fade"]);
    await expect.poll(() => running(frame)).toEqual([]);
    expect(await opacity(frame, "four")).toBe("1");

    // Showing another slide and coming back replays it.
    await page.getByRole("button", { name: "Show slide: Plain" }).click();
    const plain = await slideFrame(page.locator('[data-deck-slide][data-slide-target$=":slide:plain"]'));
    await expect.poll(() => running(plain)).toEqual([]);
    await page.getByRole("button", { name: "Show slide: Unfolding" }).click();
    await expect(frameElement).toHaveAttribute("src", /#diagram-reveal-replay$/);
    frame = await slideFrame(slide);
    await expect.poll(() => running(frame), { intervals: [50] }).toEqual(["diagram-reveal-replay-fade"]);
    await expect.poll(() => running(frame)).toEqual([]);
    await page.getByRole("button", { name: "Show slide: Plain" }).click();
    await page.getByRole("button", { name: "Show slide: Unfolding" }).click();
    await expect(frameElement).toHaveAttribute("src", /#diagram-reveal$/);
    await expect.poll(async () => running(await slideFrame(slide)), { intervals: [50] }).toEqual(["diagram-reveal-fade"]);

    // An element landmark link retargets the frame at its element, which
    // stops the reveal and shows the finished drawing.
    const anchor = await slide.locator('[data-landmark-target][data-element-id="five"]').getAttribute("data-landmark-anchor");
    await page.evaluate(id => { location.hash = id; }, anchor!);
    await expect(frameElement).toHaveAttribute("src", /#five$/);
    frame = await slideFrame(slide);
    await expect.poll(() => running(frame)).toEqual([]);
    expect(await opacity(frame, "six")).toBe("1");
  } finally {
    await stopSagaServer(server);
  }
});

test("under reduced motion a revealed diagram shows everything immediately", async ({ browser, sagaRepositories }) => {
  const { sagaRoot } = sagaRepositories;
  cli(sagaRepositories, "review", "create", "--id", "pr-3", "--base", "main", "--head", "feature/wave-one", "--pr", "3", "--title", "Reveal review", sagaRoot);
  const target = publish(sagaRepositories, "unfolding", "Unfolding", 10, revealDiagram);
  const context = await browser.newContext({ reducedMotion: "reduce", viewport: { width: 1280, height: 900 } });
  const server = await startSagaServer(sagaRepositories);
  try {
    const page = await context.newPage();
    await page.goto(`${server.baseURL}/reviews/pr-3`);
    const slide = page.locator(`[data-deck-slide][data-slide-target="${target}"]`);
    await expect(slide.locator("iframe[data-fragment-frame]")).toHaveAttribute("src", /#diagram-reveal$/);
    const frame = await slideFrame(slide);
    await frame.waitForSelector("#six");
    expect(await frame.evaluate(() => location.hash)).toBe("#diagram-reveal");
    expect(await running(frame)).toEqual([]);
    for (const id of ["one", "four", "six"]) expect(await opacity(frame, id)).toBe("1");
  } finally {
    await context.close();
    await stopSagaServer(server);
  }
});
