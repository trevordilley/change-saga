import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { runCLI, startSagaServer, stopSagaServer } from "../support/fixture-builder.js";
import { expect, test } from "../support/test.js";

for (const context of ["review", "implementation"] as const) {
  for (const sectioned of [true, false]) {
    test(`${context} deck preserves navigation with ${sectioned ? "section boundaries" : "unsectioned slides"}`, async ({ page, sagaRepositories: repo }) => {
      test.setTimeout(120_000);
      const cli = (...args: string[]) => {
        const result = runCLI(repo, args);
        expect(result.status, `${args.join(" ")}\n${result.stderr}`).toBe(0);
      };
      const scope = context === "review" ? ["--review", "sections"] : ["--deck", "sections"];
      if (context === "review") {
        cli("review", "create", "--id", "sections", "--base", "main", "--head", "feature/wave-one", "--title", "Sections review", repo.sagaRoot);
      } else {
        cli("add-deck", "--feature", "wave-one", "--id", "sections", "--title", "Sectioned implementation", "--objective", "Follow the request.", repo.sagaRoot, "sections");
      }
      const visual = join(repo.root, "sections.svg");
      writeFileSync(visual, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720"><rect id="node" x="120" y="180" width="900" height="260" fill="#dce8ff"/><text x="200" y="325" font-size="44">Request processing</text></svg>`);
      const labels = sectioned ? ["", "Request", "Request", "Errors", "", "Errors", "Request", ""] : ["", "", ""];
      for (const [index, section] of labels.entries()) {
        cli("add-slide", ...scope, "--section", section, "--rank", String(index * 10), "--intent", "explain", "--layout", "diagram", "--title", `Step ${index + 1}`, "--source", visual, repo.sagaRoot, `step-${index + 1}`);
      }
      cli("add-item", ...(context === "review" ? scope : []), "--slide", "step-2", "--id", "node", "--kind", "node", "--element-id", "node", "--label", "Request node", "--description", "The request processing step.", repo.sagaRoot);
      const running = await startSagaServer(repo);
      try {
        const requests: string[] = [];
        page.on("request", request => requests.push(new URL(request.url()).pathname));
        await page.goto(`${running.baseURL}${context === "review" ? "/reviews/sections" : "/features/wave-one"}`);
        await page.locator("body[data-shell-ready]").waitFor();
        const rail = page.locator(context === "review" ? ".review-deck-rail" : ".sidebar");
        const buttons = rail.locator("[data-slide-thumbnail]");
        await expect(buttons).toHaveCount(labels.length);
        await expect(rail.locator("[data-slide-section]")).toHaveText(sectioned ? ["Request", "Errors", "Errors", "Request"] : []);
        await expect(rail.locator("[data-slide-thumbnail-card] [data-slide-section]")).toHaveCount(0);
        await expect(rail.locator("iframe, .slide-thumbnail-preview img")).toHaveCount(0);
        if (context === "implementation") {
          // Building the outline must not eagerly request the feature viewer.
          expect(requests.filter(path => path === "/decks")).toHaveLength(0);
        }
        const targets = await buttons.evaluateAll(nodes => nodes.map(node => node.getAttribute("data-slide-target")));
        expect(new Set(targets).size).toBe(labels.length);
        await buttons.nth(0).click();
        const viewer = page.locator(context === "review" ? "#page [data-deck-viewer]" : "#view-slides [data-deck-viewer]");
        await expect(viewer.locator("[data-deck-slide]")).toHaveCount(labels.length);
        const assertCurrent = async (index: number) => {
          await expect(viewer.locator("[data-deck-slide]:visible")).toHaveAttribute("data-slide-target", targets[index]!);
          await expect(buttons.nth(index)).toHaveAttribute("aria-current", "true");
          await expect(rail.locator('[data-slide-thumbnail][aria-current="true"]')).toHaveCount(1);
          await expect(viewer.locator("[data-slide-position]")).toHaveText(`${index + 1} / ${labels.length}`);
        };
        await assertCurrent(0);
        for (let i = 1; i < labels.length; i++) {
          await page.keyboard.press("ArrowRight");
          await assertCurrent(i);
        }
        for (let i = labels.length - 2; i >= 0; i--) {
          await page.keyboard.press("ArrowLeft");
          await assertCurrent(i);
        }
        await buttons.nth(1).click();
        const slideURL = page.url();
        const item = viewer.locator('[data-deck-slide]:visible [data-landmark-target]').first();
        const itemID = await item.getAttribute("id");
        expect(itemID).toBeTruthy();
        await viewer.getByRole("button", { name: "Essay", exact: true }).click();
        await expect(viewer.locator("[data-deck-overview]:visible")).toHaveCount(1);
        await expect(rail.locator('[data-slide-thumbnail][aria-current="true"]')).toHaveCount(0);
        const essayURL = page.url();
        await page.reload();
        await expect(viewer.locator("[data-deck-overview]:visible")).toHaveCount(1);
        expect(page.url()).toBe(essayURL);
        await page.goto(slideURL);
        await assertCurrent(1);
        await page.goto(`${slideURL.split("#")[0]}#${itemID}`);
        await assertCurrent(1);
        await expect(page.locator(`[id="${itemID}"]`)).toHaveCount(1);
        if (context === "review") {
          // Reloading a deep link can put the pointer over a hover inspector.
          await page.mouse.move(0, 0);
          await page.keyboard.press("Escape");
          const active = viewer.locator("[data-deck-slide]:visible");
          await active.locator("[data-review-approve]").click();
          await expect(buttons.nth(1).locator("..").locator("[data-review-state]")).toHaveAttribute("data-review-state", "approved");
          await expect(active.locator("[data-review-annotation-toggle]")).toBeVisible();
        }
        if (sectioned) {
          await expect(rail.locator("[data-slide-section]").first()).toHaveCSS("text-transform", "uppercase");
          await expect(rail.locator("[data-slide-section]").first()).toHaveCSS("font-size", "11px");
        }
        await page.screenshot({ path: test.info().outputPath(`${context}-${sectioned ? "sections" : "plain"}.png`) });
        await page.setViewportSize({ width: 390, height: 844 });
        await assertCurrent(1);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
        await page.screenshot({ path: test.info().outputPath(`${context}-narrow.png`) });
      } finally {
        await stopSagaServer(running);
      }
    });
  }
}
