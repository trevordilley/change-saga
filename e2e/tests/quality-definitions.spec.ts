import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga, expectNoSeriousAccessibilityViolations } from "../support/test.js";
import { runCLI, startSagaServer, stopSagaServer } from "../support/fixture-builder.js";

test("Roadmap verification gains step evidence on the same case without run outcomes", async ({ page, sagaRepositories: repo }, testInfo) => {
  const cli = (...args: string[]) => {
    const result = runCLI(repo, [...args, repo.sagaRoot]);
    expect(result.status, result.stdout + result.stderr).toBe(0);
    return result;
  };
  const testURN = "urn:change-saga:wave-one:test-case:planned-check";
  const criterion = "urn:change-saga:wave-one:story:planned-story:criterion:works";
  const query = (operation = "quality") => {
    const result = runCLI(repo, ["query", operation, "--saga", repo.sagaRoot, "--repo", repo.sourceRepo, "--test", testURN]);
    expect(result.status, result.stdout + result.stderr).toBe(0);
    return JSON.parse(result.stdout).data.test_cases[0];
  };
  cli("feature", "add", "--id", "planned-quality", "--title", "Planned Quality", "--status", "planned");
  cli("story", "add", "--feature", "planned-quality", "--id", "planned-story", "--revision", "r1", "--event", "proposed", "--title", "Inspect the greeting", "--statement", "As a reader I can inspect the greeting", "--criterion", "works=The greeting includes the supplied name.");
  const steps = [
    { id: "setup", action: "Choose a caller name.", expected_result: "A name is available." },
    { id: "assert", action: "Inspect the greeting.", expected_result: "The greeting includes the supplied name.", criteria: [criterion] }
  ];
  cli("quality", "test-case", "add", "--feature", "planned-quality", "--id", "planned-check", "--title", "Verify the named greeting", "--kind", "positive", "--automation", "hybrid", "--precondition", "A caller name is available.", ...steps.flatMap(step => ["--step", JSON.stringify(step)]), "--expected-result", "The name is visible.");
  cli("relation", "add", "--feature", "planned-quality", "--id", "planned-verifies", "--type", "verifies", "--from", testURN, "--to", criterion, "--rationale", "The assertion checks the named greeting.");
  const revisionPath = join(repo.sagaRoot, "___features/planned-quality.feature/___quality/test-cases/planned-check.test/revisions/r1.json");
  const originalRevision = readFileSync(revisionPath, "utf8");
  expect(query().definition).toBe("planned");
  const server = await startSagaServer(repo);
  try {
    await page.goto(server.baseURL + "/roadmap?group=all");
    await waitForSettledSaga(page);
    await page.locator('[data-directory="roadmap"]').getByRole("link", { name: "Planned Quality", exact: true }).click();
    await page.locator('[data-feature-quality] > summary').click();
    await page.locator('[data-feature-quality]').getByRole("link", { name: "Verify the named greeting", exact: true }).click();
    await expect(page).toHaveURL(server.baseURL + "/tests/planned-check");
    await expect(page.locator('[data-test-definition-stage]')).toContainText("planned");
    await expect(page.locator('[data-test-step="assert"]')).toContainText("The greeting includes the supplied name.");
    await expect(page.locator('[data-test-runs]')).toHaveCount(0);
    await page.locator('[data-test-step="assert"]').getByRole("link", { name: "The greeting includes the supplied name.", exact: true }).click();
    await expect(page).toHaveURL(/\/requirements\/planned-story/);
    await page.goto(server.baseURL + "/tests/planned-check");
    await expectNoSeriousAccessibilityViolations(page);
    await page.screenshot({ path: testInfo.outputPath("planned-definition.png"), fullPage: true });
    cli("quality", "evidence", "add", "--test", testURN, "--id", "assertion-code", "--role", "test_implementation", "--step", "assert", "--repo", repo.sourceRepo, "--code", `${repo.identity.head}:src/app.go#L3-L5`);
    expect(readFileSync(revisionPath, "utf8")).toBe(originalRevision);
    expect(query().definition).toBe("implementation_linked");
    expect(query().steps[1].evidence).toEqual([testURN + ":evidence:assertion-code"]);
    cli("quality", "run", "record", "--test", testURN, "--id", "legacy-ci", "--result", "failed", "--summary", "Historical CI result", "--command", "touch SHOULD_NEVER_EXECUTE", "--commit", repo.identity.head, "--evidence", testURN + ":evidence:assertion-code");
    expect(query().definition).toBe("implementation_linked");
    expect(query("quality-history").legacy_runs[0].result).toBe("failed");
    expect(existsSync(join(repo.sagaRepo, "SHOULD_NEVER_EXECUTE"))).toBe(false);
    cli("feature", "set-plan", "--feature", "planned-quality", "--status", "built");
    await page.goto(server.baseURL + "/features/planned-quality");
    await page.locator('[data-feature-quality] > summary').click();
    await page.locator('[data-feature-quality]').getByRole("link", { name: "Verify the named greeting", exact: true }).click();
    await expect(page.locator('[data-test-definition-stage]')).toContainText("implementation linked");
    await page.locator('[data-test-step="assert"]').getByRole("link", { name: "test implementation", exact: true }).click();
    await expect(page.locator('[data-test-evidence-record="assertion-code"]')).toBeVisible();
    await expect(page.locator('[data-test-step="setup"] a')).toHaveCount(0);
    await expect(page.locator('[data-test-case-page]')).not.toContainText("Historical CI result");
    await expect(page.locator('[data-test-case-page]')).not.toContainText("SHOULD_NEVER_EXECUTE");
    await page.screenshot({ path: testInfo.outputPath("implemented-definition.png"), fullPage: true });
    cli("quality", "test-case", "revise", "--test", testURN, "--parent", testURN + ":revision:r1", "--revision", "r2", "--title", "Verify the named greeting precisely");
    expect(query().definition).toBe("stale_evidence");
    expect(query().steps[1].issues.join(" ")).toContain("stale");
    await page.goto(server.baseURL + "/tests/planned-check");
    await page.locator('[data-test-evidence] > summary').click();
    await expect(page.locator('[data-test-evidence]')).toContainText("test revision changed");
    await expect(page.locator('[data-test-verifies]')).toContainText("Stale mapping");
    await page.setViewportSize({ width: 390, height: 844 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await expectNoSeriousAccessibilityViolations(page);
  } finally { await stopSagaServer(server); }
});
