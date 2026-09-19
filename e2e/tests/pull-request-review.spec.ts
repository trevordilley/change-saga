import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { codeLocation, git, readJSON, reviewFiles, runCLI, startSagaServer, stopSagaServer, type SagaRepositories } from "../support/fixture-builder.js";
import { expect, test } from "../support/test.js";

// A pull request's review is a slide deck and the one place approval happens.
// Each slide records every reviewer's decision at the head it was given, and a
// push that changes a slide's code puts that slide's decisions out of date.

function cli(repositories: SagaRepositories, ...args: string[]): string {
  const result = runCLI(repositories, args);
  if (result.status !== 0) throw new Error(`change-saga ${args.join(" ")} failed (${result.status})\n${result.stdout}\n${result.stderr}`);
  return result.stdout;
}

const slideSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720" role="img" aria-label="Review slide"><rect id="change" x="80" y="80" width="480" height="240" fill="#dce8ff"/><text id="why" x="640" y="200">Why it changed</text></svg>\n`;

function authorReview(repositories: SagaRepositories): void {
  const { sagaRoot, sourceRepo, identity } = repositories;
  const visual = join(repositories.root, "review-slide.svg");
  writeFileSync(visual, slideSVG);
  cli(repositories, "review", "create", "--id", "pr-1", "--base", "main", "--head", "feature/wave-one", "--pr", "1", "--url", "https://example.test/acme/change-saga-demo/pull/1", "--title", "Wave one review", sagaRoot);
  for (const slide of ["greeting", "theme"]) {
    cli(repositories, "add-slide", "--review", "pr-1", "--intent", "explain", "--layout", "diagram", "--title", slide === "greeting" ? "Greeting takes a name" : "Theme colour", "--source", visual, sagaRoot, slide);
    cli(repositories, "add-item", "--review", "pr-1", "--slide", slide, "--kind", "node", "--element-id", "change", "--label", "The change", "--description", "The code this slide explains", sagaRoot);
  }
  cli(repositories, "add-item", "--review", "pr-1", "--slide", "greeting", "--kind", "statement", "--element-id", "why", "--label", "Epic", "--description", "The epic this change revises", "--record", "urn:change-saga:wave-one:epic:wave-one", sagaRoot);
  cli(repositories, "cover", "--repo", sourceRepo, "--target", "urn:change-saga:wave-one:review:pr-1:slide:greeting:item:change", "--ref", codeLocation(identity.head, "src/app.go", 4), sagaRoot);
  cli(repositories, "cover", "--repo", sourceRepo, "--target", "urn:change-saga:wave-one:review:pr-1:slide:theme:item:change", "--ref", codeLocation(identity.head, "assets/ui/theme.css", 2), sagaRoot);
  git(repositories.sagaRepo, "add", ".");
  git(repositories.sagaRepo, "commit", "-m", "Review deck for pull request 1");
}

test("@critical approves slide by slide and marks a decision out of date when its code changes", async ({ page, sagaRepositories }) => {
  authorReview(sagaRepositories);
  const running = await startSagaServer(sagaRepositories);
  try {
    await page.goto(new URL("/reviews/pr-1", running.baseURL).toString());
    const greeting = page.locator('[data-review-slide="greeting"]');
    const theme = page.locator('[data-review-slide="theme"]');
    await expect(greeting.getByRole("heading", { name: "Greeting takes a name" })).toBeVisible();
    // The Item's code reference is shown as a diff against the review's base.
    await expect(greeting.locator(".review-line.add").filter({ hasText: `"hello, " + name` })).toHaveCount(1);
    await expect(greeting.locator(".review-line.del").filter({ hasText: `return "hello"` })).toHaveCount(1);
    await expect(greeting.locator('[data-review-record="urn:change-saga:wave-one:epic:wave-one"]')).toBeVisible();

    await greeting.locator("[data-review-approve]").click();
    await expect(greeting.locator('[data-decision-state="approved"]')).toHaveAttribute("data-currency", "current");
    await theme.locator("textarea[name=body]").first().fill("Name the colour token.");
    await theme.locator("[data-review-request-changes]").click();
    await expect(theme.locator('[data-decision-state="changes_requested"]')).toHaveAttribute("data-currency", "current");
    await theme.locator('[data-review-comment-form$=":item:change"] textarea').fill("Is this contrast checked?");
    await theme.locator('[data-review-comment-form$=":item:change"] button').click();
    await expect(theme.getByText("Is this contrast checked?")).toBeVisible();

    const approvals = reviewFiles(sagaRepositories, /___reviews\/pr-1\.review\/approvals\/.+\.json$/);
    expect(approvals).toHaveLength(2);
    for (const path of approvals) {
      const record = readJSON<{ commit: string; reviewer: { kind: string } }>(path);
      expect(record.commit).toBe(sagaRepositories.identity.head);
      expect(record.reviewer.kind).toBe("human");
    }

    // A push that changes the greeting's code puts only that slide's approval
    // out of date; the other slide keeps its decision and its currency.
    git(sagaRepositories.sourceRepo, "checkout", "feature/wave-one");
    writeFileSync(join(sagaRepositories.sourceRepo, "src", "app.go"), `package demo\n\nfunc Greeting(name string) string {\n\treturn "hi, " + name\n}\n\nfunc Ready() bool {\n\treturn true\n}\n`);
    git(sagaRepositories.sourceRepo, "commit", "-am", "shorter greeting");
    await page.reload();
    await expect(greeting.locator('[data-decision-state="approved"]')).toHaveAttribute("data-currency", "out_of_date");
    await expect(greeting.locator("[data-out-of-date]")).toHaveText("Out of date");
    await expect(theme.locator('[data-decision-state="changes_requested"]')).toHaveAttribute("data-currency", "current");

    // The review index and status report the same state with no verdict.
    await page.goto(new URL("/reviews", running.baseURL).toString());
    await expect(page.locator('[data-review-summary="pr-1"] [data-review-slide-state="greeting"] [data-currency="out_of_date"]')).toBeVisible();
    const status = runCLI(sagaRepositories, ["status", "--json", "--repo", sagaRepositories.sourceRepo, sagaRepositories.sagaRoot]);
    const report = JSON.parse(status.stdout) as { reviews: Array<{ id: string; slides: Array<{ id: string; decisions: Array<{ state: string; currency: string }> }> }> };
    const slides = Object.fromEntries(report.reviews[0].slides.map((slide) => [slide.id, slide.decisions.map((decision) => `${decision.state}:${decision.currency}`)]));
    expect(slides).toEqual({ greeting: ["approved:out_of_date"], theme: ["changes_requested:current"] });
  } finally {
    await stopSagaServer(running);
  }
});

test("shows the living layers read-only beside the review of the compared change", async ({ page, sagaRepositories }) => {
  authorReview(sagaRepositories);
  const running = await startSagaServer(sagaRepositories, "main");
  try {
    await page.goto(running.baseURL);
    await page.getByRole("tab", { name: "Change" }).click();
    const review = page.locator('[data-change-review] [data-review-summary="pr-1"]');
    await expect(review).toBeVisible();
    await expect(review.getByRole("link", { name: "Wave one review" })).toHaveAttribute("href", "/reviews/pr-1");
    await expect(page.locator('[data-change-layer="changed"]')).toBeVisible();
    await expect(page.locator("[data-review-decision],[data-review-comment],form[action*='/decision']")).toHaveCount(0);
  } finally {
    await stopSagaServer(running);
  }
});
