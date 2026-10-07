import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { git, reviewFiles, runCLI, startSagaServer, stopSagaServer, type SagaRepositories } from "../support/fixture-builder.js";
import { expect, test, waitForSettledSaga } from "../support/test.js";

// A Saga's theme.css recolours the reviewer and the slides it serves, in
// light and dark mode, while the committed slide bytes stay as they were.

function cli(repositories: SagaRepositories, ...args: string[]): string {
  const result = runCLI(repositories, args);
  if (result.status !== 0) throw new Error(`change-saga ${args.join(" ")} failed (${result.status})\n${result.stdout}\n${result.stderr}`);
  return result.stdout;
}

// A hand-authored slide that paints with tokens and declares none.
const tokenSlide = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720" role="img" aria-labelledby="t"><title id="t">Token slide</title>
<rect id="canvas" width="1280" height="720" style="fill:var(--diagram-canvas)"/>
<rect id="card" x="440" y="260" width="400" height="200" style="fill:var(--diagram-primary-fill);stroke:var(--diagram-primary-stroke)"/>
</svg>`;

const theme = `/* A brand theme. */
:root {
  --bg: #fdf6e3;
  --bg-subtle: #eee8d5;
  --ink: #073642;
  --muted: #586e75;
  --accent: #005f87;
  --sel: #d9e8dc;
  --line: #93a1a1;
  --ui: Georgia,serif;
  --diagram-canvas: #fdf6e3;
}
:root[data-theme="dark"] {
  --bg: #002b36;
  --bg-subtle: #073642;
  --ink: #eee8d5;
  --muted: #93a1a1;
  --accent: #7fc4d2;
  --sel: #164654;
  --line: #586e75;
  --diagram-canvas: #001f27;
}
`;

test("a theme file recolours the reviewer and a slide in light and dark mode", async ({ page, sagaRepositories }) => {
  const { sagaRoot, root } = sagaRepositories;
  cli(sagaRepositories, "review", "create", "--id", "pr-9", "--base", "main", "--head", "feature/wave-one", "--pr", "9", "--title", "Themed review", sagaRoot);
  const svgPath = join(root, "token-slide.svg");
  writeFileSync(svgPath, tokenSlide);
  const requestPath = join(root, "token-slide.json");
  writeFileSync(requestPath, JSON.stringify({
    version: 1, operation: "create", request_id: "token-slide", review: "pr-9", expected_snapshot: "absent",
    slide: { id: "token-slide", title: "Token slide", rank: 10, intent: "explain", layout: "diagram", media_type: "image/svg+xml", takeaway: "Paints with tokens.", reading_order: ["card"] },
    asset: { path: "token-slide.svg" },
    items: [{ id: "card", rank: 10, kind: "node", label: "Card", description: "A card painted with tokens.", selector: { type: "element", element_id: "card" } }],
  }));
  cli(sagaRepositories, "apply-slide", "--from", requestPath, sagaRoot);
  expect(cli(sagaRepositories, "theme", "init", sagaRoot)).toContain("theme.css");
  writeFileSync(join(sagaRoot, "theme.css"), theme);
  expect(cli(sagaRepositories, "theme", "check", sagaRoot)).toContain("Valid theme");
  git(sagaRepositories.sagaRepo, "add", ".");
  git(sagaRepositories.sagaRepo, "commit", "-m", "Themed review");

  const running = await startSagaServer(sagaRepositories);
  try {
    const background = () => page.evaluate(() => getComputedStyle(document.body).backgroundColor);
    await page.emulateMedia({ colorScheme: "light" });
    await page.goto(`${running.baseURL}/reviews/pr-9`);
    // A hand-authored SVG slide is served into a sandboxed frame.
    const slide = page.locator('[data-deck-slide][data-slide-target$=":slide:token-slide"] iframe.fragment-frame');
    await expect(slide).toBeVisible();
    const frameCanvas = () => slide.contentFrame().locator("#canvas").evaluate((rect) => getComputedStyle(rect).fill);
    await expect.poll(frameCanvas).toBe("rgb(253, 246, 227)");
    expect(await background()).toBe("rgb(253, 246, 227)");

    // The grouped outline uses the same tokens, including a custom UI font.
    const rail = page.locator(".review-deck-rail");
    const selectedSlide = rail.locator(".slide-thumbnail-card.active");
    await expect(rail).toHaveCSS("background-color", "rgb(238, 232, 213)");
    await expect(selectedSlide).toHaveCSS("background-color", "rgb(217, 232, 220)");
    await expect(selectedSlide).toHaveCSS("color", "rgb(0, 95, 135)");
    await expect(rail.locator(".side-tab.current")).toHaveCSS("border-bottom-color", "rgb(0, 95, 135)");
    await expect(rail.locator(".slide-thumbnail-title")).toHaveCSS("font-family", "Georgia, serif");
    await expect(page.locator(".topbar .side-tabs")).toHaveCount(0);

    const overviewLink = rail.getByRole("link", { name: "Overview", exact: true });
    const titleBox = await rail.locator(".sidebar-title").boundingBox();
    const tabsBox = await rail.locator(".side-tabs").boundingBox();
    expect(titleBox!.y + titleBox!.height).toBeLessThanOrEqual(tabsBox!.y);
    await overviewLink.click();
    await expect(overviewLink).toHaveAttribute("aria-current", "page");
    await expect(overviewLink).toHaveCSS("background-color", "rgb(217, 232, 220)");
    await expect(overviewLink).toHaveCSS("text-decoration-line", "none");
    await expect(rail.locator(".slide-thumbnail-card.active")).toHaveCount(0);
    await page.locator("[data-deck-viewer]").getByRole("button", { name: "Back", exact: true }).click();
    await expect(overviewLink).not.toHaveAttribute("aria-current", "page");
    await expect(selectedSlide).toBeVisible();

    // Dark by the OS preference, then pinned light and dark by the toggle.
    await page.emulateMedia({ colorScheme: "dark" });
    await expect.poll(background).toBe("rgb(0, 43, 54)");
    await expect.poll(frameCanvas).toBe("rgb(0, 31, 39)");
    await expect(rail).toHaveCSS("background-color", "rgb(7, 54, 66)");
    await expect(selectedSlide).toHaveCSS("background-color", "rgb(22, 70, 84)");
    await expect(selectedSlide).toHaveCSS("color", "rgb(127, 196, 210)");
    await page.locator("[data-theme-toggle]").click();
    await expect.poll(background).toBe("rgb(253, 246, 227)");
    await expect.poll(frameCanvas).toBe("rgb(253, 246, 227)");
    await page.emulateMedia({ colorScheme: "light" });
    await page.locator("[data-theme-toggle]").click();
    await expect.poll(background).toBe("rgb(0, 43, 54)");
    await expect.poll(frameCanvas).toBe("rgb(0, 31, 39)");

    // The served slide declares the tokens with the theme applied, for the
    // reader's scheme; its own committed bytes declare none.
    const visualURL = new URL((await slide.getAttribute("src"))!, running.baseURL);
    visualURL.searchParams.delete("saga_scheme");
    const canvas = () => page.evaluate(() => getComputedStyle(document.getElementById("canvas")!).fill);
    await page.emulateMedia({ colorScheme: "light" });
    await page.getByRole("link", { name: "Documentation", exact: true }).click();
    await waitForSettledSaga(page);
    const sidebar = page.locator("aside.sidebar");
    await expect(sidebar).toHaveCSS("background-color", "rgb(7, 54, 66)");
    await expect(sidebar.getByRole("link", { name: "Documentation", exact: true })).toHaveAttribute("aria-current", "page");
    await page.locator("[data-theme-toggle]").click();
    await expect(sidebar).toHaveCSS("background-color", "rgb(238, 232, 213)");
    await expect(sidebar.locator(".sidebar-title")).toHaveCSS("font-family", "Georgia, serif");
    await page.getByRole("tab", { name: "Documented code", exact: true }).click();
    await expect(sidebar.getByRole("link", { name: "Reviews", exact: true })).toBeVisible();
    // Secondary surfaces stay beside the sidebar, including the mode switcher.
    const surface = await page.locator(".manifest-view").boundingBox();
    const sidebarBox = await sidebar.boundingBox();
    expect(surface!.x).toBeGreaterThanOrEqual(sidebarBox!.x + sidebarBox!.width);
    await page.goto(visualURL.href);
    expect(await canvas()).toBe("rgb(253, 246, 227)");
    expect(await page.evaluate(() => getComputedStyle(document.getElementById("card")!).fill)).toBe("rgb(237, 245, 255)");
    await page.emulateMedia({ colorScheme: "dark" });
    await expect.poll(canvas).toBe("rgb(0, 31, 39)");
    const committed = reviewFiles(sagaRepositories, /pr-9\.review.*\.svg$/);
    expect(committed).toHaveLength(1);
    expect(readFileSync(committed[0], "utf8")).toBe(tokenSlide);

    // The preview shows both schemes side by side.
    await page.goto(`${running.baseURL}/theme`);
    await expect(page.locator('.theme-pane[data-scheme="light"]')).toHaveCSS("background-color", "rgb(253, 246, 227)");
    await expect(page.locator('.theme-pane[data-scheme="dark"]')).toHaveCSS("background-color", "rgb(0, 43, 54)");
    await expect(page.locator(".theme-pane img.theme-diagram").first()).toBeVisible();
  } finally {
    await stopSagaServer(running);
  }
});
