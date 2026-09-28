import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { git, readJSON, runCLI, startSagaServer, stopSagaServer, type SagaRepositories } from "../support/fixture-builder.js";
import { expect, expectNoSeriousAccessibilityViolations, test, waitForSettledSaga } from "../support/test.js";

// A review whose change has merged is history. The Reviews page says so with
// a Merged badge and sets it aside: the open reviews come first, and a merged
// one appears when the reader types a filter that matches it or asks for the
// merged ones.

function cli(repositories: SagaRepositories, ...args: string[]): void {
  const result = runCLI(repositories, args);
  if (result.status !== 0) throw new Error(`change-saga ${args.join(" ")} failed (${result.status})\n${result.stdout}\n${result.stderr}`);
}

// This fixture's Saga lives in a companion repository, where Git cannot
// detect a landing, so the merged review is recorded the way repin --onto
// records it. The page treats a recorded and a detected merge alike.
function authorReviews(repositories: SagaRepositories): void {
  const { sagaRoot, identity } = repositories;
  cli(repositories, "review", "create", "--id", "pr-1", "--base", "main", "--head", "feature/wave-one", "--pr", "1", "--title", "Landed greeting review", sagaRoot);
  cli(repositories, "review", "create", "--id", "pr-2", "--base", "main", "--head", "feature/wave-one", "--pr", "2", "--title", "Open theme review", sagaRoot);
  const manifestPath = join(sagaRoot, "___reviews", "pr-1.review", "review.json");
  const manifest = readJSON<Record<string, unknown>>(manifestPath);
  manifest.merged = { base: identity.base, head: identity.head, landed: identity.head, merged_at: "2026-09-24T12:00:00Z" };
  writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
  git(repositories.sagaRepo, "add", ".");
  git(repositories.sagaRepo, "commit", "-m", "One merged review and one open");
}

test("merged reviews show Merged and stay hidden until searched for or shown", async ({ page, sagaRepositories }) => {
  authorReviews(sagaRepositories);
  // Observing, no review is the one being compared, so none is kept in view.
  const running = await startSagaServer(sagaRepositories, null);
  try {
    await page.goto(`${running.baseURL}/reviews`);
    await waitForSettledSaga(page);
    const directory = page.locator('[data-directory="reviews"]');
    const merged = directory.locator('[data-directory-row="pr-1"]');
    const open = directory.locator('[data-directory-row="pr-2"]');
    const mergedSummary = page.locator('[data-review-summary="pr-1"]');
    const caption = directory.locator("[data-directory-caption]");
    const showMerged = directory.getByLabel("Show 1 merged");

    // By default only the open review is in view, and the caption says what
    // is set aside rather than letting it silently disappear.
    await expect(open).toBeVisible();
    await expect(merged).toBeHidden();
    await expect(mergedSummary).toBeHidden();
    await expect(caption).toHaveText("1 review · 1 merged hidden");
    await expect(directory.locator("[data-directory-none]")).toBeHidden();
    await expectNoSeriousAccessibilityViolations(page);

    // The sidebar sets it aside the same way, counting it in one row that
    // opens this page with the merged reviews shown.
    const contents = page.getByRole("navigation", { name: "Contents" });
    await expect(contents.getByRole("link", { name: "Open theme review #2" })).toBeVisible();
    await expect(contents.getByRole("link", { name: /Landed greeting review/ })).toHaveCount(0);
    await expect(contents.getByRole("link", { name: "1 merged", exact: true })).toHaveAttribute("href", "/reviews?archived=show");

    // Typing a filter searches merged reviews too.
    const filter = directory.getByRole("searchbox", { name: "Filter reviews" });
    await filter.fill("greeting");
    await expect(merged).toBeVisible();
    await expect(merged).toContainText("merged");
    await expect(open).toBeHidden();
    await expect(mergedSummary).toBeVisible();
    await expect(mergedSummary.locator('[data-review-state-badge="merged"]')).toHaveText("Merged");
    await expect(caption).toHaveText("1 of 2 reviews");

    // Clearing the filter sets it aside again.
    await filter.fill("");
    await expect(merged).toBeHidden();
    await expect(caption).toHaveText("1 review · 1 merged hidden");

    // Asking for the merged reviews shows them after the open ones.
    await showMerged.check();
    await expect(merged).toBeVisible();
    await expect(mergedSummary).toBeVisible();
    await expect(caption).toHaveText("2 reviews");
    await expect(directory.locator("[data-directory-row]")).toHaveText([/Open theme review/, /Landed greeting review/]);
    await showMerged.uncheck();
    await expect(merged).toBeHidden();
  } finally {
    await stopSagaServer(running);
  }
});
