import { runCLI } from "../support/fixture-builder.js";
import { expectNoSeriousAccessibilityViolations, expect, test } from "../support/test.js";

test("the overview expands to its parts, and a term links its code and stories both ways", async ({ page, saga }) => {
  const contents = page.getByRole("navigation", { name: "Contents" });
  for (const part of ["Name", "Elevator pitch", "Description", "Terms and vocabulary"]) {
    await expect(contents.getByRole("link", { name: part, exact: true })).toBeVisible();
  }

  // A story names the term, so the term and the story reach each other.
  const story = runCLI(saga, [
    "story", "add", "--epic", "wave-one", "--id", "greet-by-name", "--revision", "r1", "--event", "proposed",
    "--title", "Greet a caller by name", "--statement", "As a caller, I am greeted by my name.", "--priority", "must", saga.sagaRoot
  ]);
  expect(story.status, story.stderr).toBe(0);
  const head = runCLI(saga, ["query", "terms", "--saga", saga.sagaRoot, "--repo", saga.sourceRepo, "--term", "greeting"]);
  expect(head.status, head.stderr).toBe(0);
  const current = JSON.parse(head.stdout).data.terms[0];
  const code = current.code[0].head;
  const revise = runCLI(saga, [
    "term", "revise", "--repo", saga.sourceRepo, "--term", current.term, "--revision", "r2", "--parent", current.revision_heads[0],
    "--name", "Greeting", "--alias", "salutation", "--definition", current.definition, "--story", "greet-by-name",
    "--ref", `${code.commit}:${code.path}#L${code.start}-L${code.end}`, saga.sagaRoot
  ]);
  expect(revise.status, revise.stderr).toBe(0);
  await page.reload();

  await contents.getByRole("link", { name: "Terms and vocabulary", exact: true }).click();
  await expect(page).toHaveURL(/\/terms$/);
  await expect(page.getByRole("heading", { name: "Terms and vocabulary", level: 1 })).toBeVisible();
  await page.getByRole("main").getByRole("link", { name: "Greeting", exact: true }).click();

  await expect(page).toHaveURL(/\/terms\/greeting$/);
  await expect(page.getByRole("heading", { name: "Greeting", level: 1 })).toBeVisible();
  await expect(page.getByText("The line a caller is welcomed with; it now names the caller.")).toBeVisible();
  await expect(page.getByText("salutation", { exact: true })).toBeVisible();
  const rendered = page.locator("[data-term-code]");
  await expect(rendered).toHaveAttribute("data-file-path", "src/app.go");
  await expect(rendered.locator("tr.referenced")).toHaveCount(3);
  await expect(rendered.locator("tr.referenced").first()).toContainText("func Greeting(name string) string {");
  await expect(page.getByRole("button", { name: /approve/i })).toHaveCount(0);
  await expectNoSeriousAccessibilityViolations(page);

  await page.getByRole("link", { name: "Greet a caller by name" }).click();
  await expect(page).toHaveURL(/\/requirements\/greet-by-name$/);
  await page.locator("[data-requirement-terms]").getByRole("link", { name: "Greeting" }).click();
  await expect(page).toHaveURL(/\/terms\/greeting$/);
});
