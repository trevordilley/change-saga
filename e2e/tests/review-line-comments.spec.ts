import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { codeLocation, git, readJSON, reviewFiles, runCLI, startSagaServer, stopSagaServer, type SagaRepositories } from "../support/fixture-builder.js";
import { expect, test } from "../support/test.js";

// A reviewer comments on a line of a review's diff as on a pull request: the
// "+" in a line's gutter opens a composer under it, and the thread stays under
// that line, with its replies, when the page is read again.

function cli(repositories: SagaRepositories, ...args: string[]): string {
  const result = runCLI(repositories, args);
  if (result.status !== 0) throw new Error(`change-saga ${args.join(" ")} failed (${result.status})\n${result.stdout}\n${result.stderr}`);
  return result.stdout;
}

const slideSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720" role="img" aria-label="Review slide"><rect id="change" x="80" y="80" width="480" height="240" fill="#dce8ff"/></svg>\n`;

function authorReview(repositories: SagaRepositories): void {
  const { sagaRoot, sourceRepo, identity } = repositories;
  const visual = join(repositories.root, "review-slide.svg");
  writeFileSync(visual, slideSVG);
  cli(repositories, "review", "create", "--id", "pr-1", "--base", "main", "--head", "feature/wave-one", "--pr", "1", "--title", "Wave one review", sagaRoot);
  cli(repositories, "add-slide", "--review", "pr-1", "--intent", "explain", "--layout", "diagram", "--title", "Greeting takes a name", "--source", visual, sagaRoot, "greeting");
  cli(repositories, "add-item", "--review", "pr-1", "--slide", "greeting", "--kind", "node", "--element-id", "change", "--label", "The change", "--description", "The code this slide explains", sagaRoot);
  cli(repositories, "cover", "--repo", sourceRepo, "--target", "urn:change-saga:wave-one:review:pr-1:slide:greeting:item:change", "--ref", codeLocation(identity.head, "src/app.go", 3, 4), sagaRoot);
  git(repositories.sagaRepo, "add", ".");
  git(repositories.sagaRepo, "commit", "-m", "Review deck for pull request 1");
}

test("a reviewer comments on a diff line in an Item's drawer, replies, resolves, and finds it after a reload", async ({ page, sagaRepositories, browserEvents }) => {
  authorReview(sagaRepositories);
  const running = await startSagaServer(sagaRepositories);
  try {
    await page.goto(new URL("/reviews/pr-1", running.baseURL).toString());
    const slide = page.locator("[data-deck-slide].active");
    const openItem = async () => {
      await slide.locator(".landmark-hotspot").getByRole("button", { name: /Open linked code .* for The change/ }).click();
      const drawer = page.locator("#review-drawer");
      await expect(drawer.locator(".review-item-panel h2")).toHaveText("The change");
      await expect(drawer.locator("figure.review-diff tr.review-line.add").first()).toBeVisible();
      return drawer;
    };
    let drawer = await openItem();

    // The "+" is in every line's gutter, reachable from the keyboard.
    const added = drawer.locator("figure.review-diff tr.review-line.add").filter({ hasText: 'return "hello, " + name' });
    const plus = added.getByRole("button", { name: "Comment on line 4 of src/app.go" });
    await expect(plus).toHaveCount(1);
    await plus.focus();
    await page.keyboard.press("Enter");
    const composer = drawer.getByRole("textbox", { name: "Comment on line 4 of src/app.go" });
    await expect(composer).toBeFocused();
    // Escape leaves the composer, not the drawer, and returns to the "+".
    await page.keyboard.press("Escape");
    await expect(composer).toHaveCount(0);
    await expect(plus).toBeFocused();
    await expect(drawer).toBeVisible();

    await added.hover();
    await expect(plus).toBeVisible();
    await plus.click();
    await composer.fill("Should this **trim** the name? <script>alert(1)</script>");
    await drawer.getByRole("button", { name: "Comment", exact: true }).click();

    const thread = drawer.locator("tr.review-line-thread-row [data-review-line-thread]");
    await expect(thread).toHaveCount(1);
    await expect(thread.locator("strong", { hasText: "trim" })).toBeVisible();
    await expect(thread.locator("script")).toHaveCount(0);
    await expect(thread).toHaveAttribute("data-thread-state", "open");
    // The thread sits directly under the line it was made on.
    await expect(added.locator("xpath=following-sibling::tr[1]")).toHaveClass(/review-line-thread-row/);
    await expect(page.locator('[data-review-line-count-for$=":item:change"] [data-review-line-count]').first()).toHaveText("1");

    await thread.getByRole("textbox", { name: "Reply on line 4" }).fill("Trimmed in the handler.");
    await thread.getByRole("button", { name: "Reply", exact: true }).click();
    await expect(thread.locator(".review-comment")).toHaveCount(2);
    await thread.getByRole("button", { name: "Resolve" }).click();
    await expect(thread).toHaveAttribute("data-thread-state", "resolved");
    await expect(thread.getByRole("button", { name: "Reopen" })).toBeVisible();

    // The records are the review's own append-only comments, anchored to
    // the exact head line.
    const records = reviewFiles(sagaRepositories, /comments\/.*\.json$/).map(path => readJSON<{ code_line?: { commit: string; path: string; side: string; start: number; end: number; digest: string }; reply_to?: string; state?: string }>(path));
    const root = records.find(record => record.code_line);
    expect(root?.code_line).toMatchObject({ commit: sagaRepositories.identity.head, path: "src/app.go", side: "new", start: 4, end: 4 });
    expect(records.filter(record => record.reply_to)).toHaveLength(2);

    await page.reload();
    drawer = await openItem();
    const again = drawer.locator("tr.review-line-thread-row [data-review-line-thread]");
    await expect(again).toHaveCount(1);
    await expect(again).toHaveAttribute("data-thread-state", "resolved");
    await expect(again.locator(".review-comment")).toHaveCount(3);
    await expect(again).toContainText("Trimmed in the handler.");
    await expect(drawer.locator("figure.review-diff tr.review-line.add").filter({ hasText: 'return "hello, " + name' }).locator("xpath=following-sibling::tr[1]")).toHaveClass(/review-line-thread-row/);
    await page.keyboard.press("Escape");

    // The Code Diff tab shows the same thread under the same line.
    await page.getByRole("tab", { name: "Code Diff", exact: true }).click();
    await page.locator('#view-code [data-tree-path="src/app.go"]').click();
    const file = page.locator('#view-code article.file-diff[data-file-path="src/app.go"]');
    await expect(file.locator("[data-file-diff-status]")).toHaveText("All changed hunks");
    await expect(file.locator(".diff-thread-row [data-review-line-thread]")).toHaveCount(1);
    await expect(file.locator(".diff-row.new").filter({ hasText: 'return "hello, " + name' }).getByRole("button", { name: "Comment on line 4 of src/app.go" })).toHaveCount(1);
    expect(browserEvents.filter(event => !event.includes("favicon"))).toEqual([]);
  } finally {
    await stopSagaServer(running);
  }
});
