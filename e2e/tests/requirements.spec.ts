import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { runCLI } from "../support/fixture-builder.js";
import { expectNoSeriousAccessibilityViolations, expect, test, waitForSettledSaga } from "../support/test.js";

test("requirements remain canonical while stories and criteria get dedicated review targets", async ({ page, saga }) => {
  const statement = "As a reviewer, I can inspect the canonical requirement without duplicated report prose.";
  const persona = runCLI(saga, ["persona", "add", "--id", "reviewer", "--name", "Reviewer", "--description", "Reads the Saga to approve a change.", saga.sagaRoot]);
  expect(persona.status, persona.stderr).toBe(0);
  const result = runCLI(saga, [
    "story", "add",
    "--feature", "wave-one",
    "--persona", "urn:change-saga:wave-one:persona:reviewer",
    "--id", "canonical-review",
    "--revision", "r1",
    "--event", "proposed",
    "--title", "Review canonical requirements",
    "--statement", statement,
    "--priority", "must",
    "--criterion", "stable-targets=Every story and criterion keeps its canonical stable target.",
    saga.sagaRoot
  ]);
  expect(result.status, result.stderr).toBe(0);

  await page.reload();
  await waitForSettledSaga(page);
  await expect(page.getByRole("tabpanel", { name: "Saga" }).getByText(statement)).toHaveCount(0);

  const contents = page.getByRole("navigation", { name: "Contents" });
  // Product starts collapsed; Requirements sits inside it. The header itself
  // opens the feature's Product section, so the twisty is what expands it here.
  await contents.getByRole("button", { name: "Toggle Product" }).click();
  await contents.getByRole("link", { name: "Requirements", exact: true }).click();
  await expect(page).toHaveURL(/\/requirements$/);
  await expect(page.getByRole("heading", { name: "Requirements", exact: true })).toBeVisible();
  await expectNoSeriousAccessibilityViolations(page);

  const story = page.locator("article.requirements-story-card", { hasText: "Review canonical requirements" });
  await story.locator("details > summary").click();
  await expect(story.getByText("Every story and criterion keeps its canonical stable target.")).toBeVisible();
  await story.getByRole("link", { name: /Every story and criterion/ }).click();

  await expect(page).toHaveURL(/\/requirements\/canonical-review\/criteria\/stable-targets$/);
  // A criterion has its own traceability view: its statement, what links to
  // it and to its story, and a way back to the story.
  const criterionPage = page.locator("[data-criterion-page]");
  await expect(criterionPage.getByRole("heading", { name: "Every story and criterion keeps its canonical stable target.", level: 1 })).toBeVisible();
  await expect(criterionPage).toHaveAttribute("data-requirement-target", "urn:change-saga:wave-one:story:canonical-review:criterion:stable-targets");
  await expect(criterionPage.locator("[data-criterion-own-trace]")).toBeVisible();
  await expectNoSeriousAccessibilityViolations(page);
  await criterionPage.getByRole("link", { name: "Review canonical requirements" }).first().click();

  await expect(page).toHaveURL(/\/requirements\/canonical-review$/);
  await expect(page.getByRole("heading", { name: "Review canonical requirements", level: 1 })).toBeVisible();
  await expect(page.getByText(statement)).toBeVisible();
  await expect(page.locator("[data-story-context]")).toBeVisible();
  const storyDetails = page.locator("details.requirement-story-details");
  await expect(storyDetails).not.toHaveAttribute("open", "");
  await expect(storyDetails.getByText("Lifecycle", { exact: true })).not.toBeVisible();
  await expectNoSeriousAccessibilityViolations(page);
  const criterion = page.locator('[data-requirement-target="urn:change-saga:wave-one:story:canonical-review:criterion:stable-targets"]');
  await expect(criterion).toBeVisible();
  await storyDetails.getByText("Details", { exact: true }).click();
  await expect(storyDetails.getByText("Lifecycle", { exact: true })).toBeVisible();
});

test("an active requirement opens its implementation Item and exact linked source", async ({ page, saga }) => {
  const run = (...args: string[]): void => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, `${args.join(" ")} failed\n${result.stdout}\n${result.stderr}`).toBe(0);
  };
  const story = "urn:change-saga:wave-one:story:trace-code";
  const criterion = `${story}:criterion:opens-source`;
  const item = "urn:change-saga:wave-one:slide:requirement-implementation:item:implementation-node";

  run("story", "add", "--feature", "wave-one", "--id", "trace-code", "--revision", "r1", "--event", "proposed", "--title", "Trace intent to exact code", "--statement", "As a reviewer, I can follow a requirement to the implementation that fulfills it.", "--criterion", "opens-source=The implementation Item opens the exact linked source.", saga.sagaRoot);
  run("story", "set-state", "--feature", "wave-one", "--story", story, "--event", "accepted", "--parent", `${story}:event:proposed`, "--state", "accepted", "--reason", "The reviewer path is implemented and testable.", saga.sagaRoot);
  run("add-deck", "--feature", "wave-one", "--objective", "Explain how the requirement reaches its implementation.", saga.sagaRoot, "requirement-path");
  run("add-slide", "--deck", "requirement-path", "--intent", "trace", "--layout", "diagram", "--title", "Requirement implementation", "--takeaway", "The requirement reaches one exact implementation Item.", saga.sagaRoot, "requirement-implementation");
  const visual = join(saga.root, "requirement-path.svg");
  writeFileSync(visual, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720"><rect width="1280" height="720" fill="#f7f7f4"/><g id="implementation-node"><rect x="260" y="180" width="760" height="330" rx="32" fill="#dce8ff" stroke="#3867a8" stroke-width="6"/><text x="640" y="360" text-anchor="middle" font-size="46">Implementation Item</text></g></svg>`);
  run("set-slide-content", "--target", "requirement-implementation", "--source", visual, saga.sagaRoot);
  run("add-item", "--slide", "requirement-implementation", "--kind", "node", "--id", "implementation-node", "--element-id", "implementation-node", "--label", "Request implementation", "--description", "The exact implementation that fulfills the acceptance criterion.", saga.sagaRoot);
  run("cover", "--repo", saga.sourceRepo, "--against", "main", "--target", item, "--path", "src/app.go", "--side", "new", "--lines", "3", "--name", "requirement-path-code", saga.sagaRoot);
  run("relation", "add", "--feature", "wave-one", "--id", "requirement-item-explains-source", "--type", "explains", "--from", item, "--to", criterion, "--rationale", "The Item identifies the implementation that fulfills this criterion.", saga.sagaRoot);

  await page.goto(`${saga.baseURL}/requirements/trace-code`);
  await waitForSettledSaga(page);
  await page.locator(`[data-requirement-target="${criterion}"]`).getByRole("link", { name: "AC 01" }).click();
  await expect(page).toHaveURL(/\/requirements\/trace-code\/criteria\/opens-source$/);
  const criterionPage = page.locator("[data-criterion-page]");
  const implementationLink = criterionPage.locator(`[data-trace-target="${item}"]`).getByRole("link", { name: "Request implementation" });
  await expect(implementationLink).toBeVisible();
  await implementationLink.click();

  await expect(page).toHaveURL(/\/\?view=slides#.+implementation-node/);
  const activeSlide = page.locator('[data-deck-slide][data-slide-title="Requirement implementation"]');
  await expect(activeSlide).toBeVisible();
  const implementationItem = activeSlide.locator('.landmark-hotspot[data-element-id="implementation-node"]');
  await expect(implementationItem).toHaveAttribute("data-landmark-has-diffs", "true");
  await implementationItem.click({ position: { x: 8, y: 8 } });
  const drawer = page.getByRole("complementary", { name: "Linked code" });
  await expect(drawer).toHaveAttribute("aria-hidden", "false");
  await expect(drawer.getByText("src/app.go", { exact: true })).toBeVisible();
});

test("retired stories and criteria distinguish explicit replacements from prose", async ({ page, saga }) => {
  const run = (...args: string[]): void => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, `${args.join(" ")} failed\n${result.stdout}\n${result.stderr}`).toBe(0);
  };
  const addAccepted = (id: string, title: string, criterion: string): string => {
    const urn = `urn:change-saga:wave-one:story:${id}`;
    run("story", "add", "--feature", "wave-one", "--id", id, "--revision", "r1", "--event", "proposed", "--title", title, "--statement", `As a reviewer, I can use ${title.toLowerCase()}.`, "--criterion", criterion, saga.sagaRoot);
    run("story", "set-state", "--feature", "wave-one", "--story", urn, "--event", "accepted", "--parent", `${urn}:event:proposed`, "--state", "accepted", "--reason", "The behavior is current.", saga.sagaRoot);
    return urn;
  };
  const current = addAccepted("current-route", "Current review route", "current-entry=The current route opens the review.");
  const legacy = addAccepted("legacy-route", "Legacy review route", "legacy-entry=The legacy route opens the review.");
  const proseOnly = addAccepted("prose-only-route", "Prose-only legacy route", "prose-entry=The prose-only route opens the review.");
  run("story", "set-state", "--feature", "wave-one", "--story", legacy, "--event", "retired", "--parent", `${legacy}:event:accepted`, "--state", "retired", "--reason", "Use current-route instead.", saga.sagaRoot);
  run("story", "set-state", "--feature", "wave-one", "--story", proseOnly, "--event", "retired", "--parent", `${proseOnly}:event:accepted`, "--state", "retired", "--reason", "This prose mentions current-route but no graph edge exists.", saga.sagaRoot);
  run("relation", "add", "--feature", "wave-one", "--id", "current-route-replaces-legacy", "--type", "supersedes", "--from", current, "--to", legacy, "--rationale", "The current route explicitly replaces the legacy route.", saga.sagaRoot);
  run("relation", "add", "--feature", "wave-one", "--id", "current-entry-replaces-legacy-entry", "--type", "supersedes", "--from", `${current}:criterion:current-entry`, "--to", `${legacy}:criterion:legacy-entry`, "--rationale", "The current entry criterion explicitly replaces the legacy one.", saga.sagaRoot);

  await page.goto(`${saga.baseURL}/requirements/legacy-route/criteria/legacy-entry`);
  await waitForSettledSaga(page);
  const historical = page.locator("[data-historical-requirement]");
  await expect(historical.getByRole("heading", { name: "Retired acceptance criterion" })).toBeVisible();
  await expect(historical).toContainText("not current product intent");
  await expect(historical.getByRole("link", { name: "The current route opens the review." })).toBeVisible();
  await expect(historical).toContainText("The current entry criterion explicitly replaces the legacy one.");
  await expectNoSeriousAccessibilityViolations(page);

  await page.goto(`${saga.baseURL}/requirements/prose-only-route`);
  await waitForSettledSaga(page);
  const noReplacement = page.locator("[data-historical-requirement]");
  await expect(noReplacement).toContainText("No current replacement is explicitly linked in the Saga.");
  await expect(noReplacement.getByRole("link", { name: "Current review route" })).toHaveCount(0);
});
