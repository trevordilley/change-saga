import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga } from "../support/test.js";
import { currentRecordPin, git, runCLI, startSagaServer, stopSagaServer } from "../support/fixture-builder.js";

test("whiteboard element pins open the shared drawer through clicked navigation in light and dark @critical", async ({ page, sagaRepositories: saga }, testInfo) => {
  const run = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, args.join(" ") + "\n" + result.stdout + result.stderr).toBe(0);
    return result.stdout;
  };
  run("criterion", "add", "--story", "urn:change-saga:wave-one:story:read-the-tide", "--id", "readable", "--statement", "The skipper can read the tide.", saga.sagaRoot);
  const definition=join(saga.root,"greeting-component.json");
  run("inventory","adopt-format","--format","2",saga.sagaRoot);
  const componentDefinition={name:"Greeting component",explanation:"The saved greeting implementation.",intent:"implemented",code:[{id:"greeting",commit:saga.identity.head,path:"src/app.go",start:3,end:5,note:"Defines the greeting."}]};
  writeFileSync(definition,JSON.stringify(componentDefinition));
  run("component","add","--id","greeting","--from",definition,"--delivery",saga.identity.head,"--repo",saga.sourceRepo,saga.sagaRoot);
  const component="urn:change-saga:wave-one:component:greeting";
  const originalComponentPin=currentRecordPin(saga,"component","greeting");
  const storyPin=currentRecordPin(saga,"story","read-the-tide").revision;
  const input = join(saga.root, "whiteboard.json");
  const board = { version: 1, width: 1100, height: 700, elements: [
    { id: "greeting", kind: "node", shape: "rect", label: "Greeting behavior", description: "A named greeting backed by saved code.", x: 250, y: 180, width: 420, height: 190, style: "primary", note: "The greeting serves the caller.",
      evidence: [{ commit: saga.identity.head, path: "src/app.go", start: 3, end: 5, note: "The saved greeting implementation." }],
      record: "urn:change-saga:wave-one:feature:wave-one",
      documentation:{...originalComponentPin},
      selections:[{id:"greeting-return",path:[{...originalComponentPin}],evidence:"greeting",code:{commit:saga.identity.head,path:"src/app.go",start:4,end:4,note:"The exact greeting returned."}}],
      criterion_links: [{ id: "board-greeting-readable", criterion: "urn:change-saga:wave-one:story:read-the-tide:criterion:readable", story_revision: storyPin, rationale: "Fixture criterion linked from this exact element." }] }
  ] };
  const publish = () => {
    writeFileSync(input, JSON.stringify(board));
    run("whiteboard", "publish", "--id", "architecture", "--name", "Pinned architecture", "--from", input, "--repo", saga.sourceRepo, saga.sagaRoot);
  };
  publish();
  const boardPin=()=>JSON.parse(run("whiteboard","list","--json",saga.sagaRoot)).whiteboards.find((board:{id:string})=>board.id==="architecture").content_digest as string;
  const firstBoardPin=boardPin();
  git(saga.sagaRepo,"add",".");git(saga.sagaRepo,"commit","-m","Document original board and exact evidence");
  writeFileSync(join(saga.sourceRepo, "src/app.go"), 'package demo\n\nfunc Greeting(name string) string {\n return "Reassessed " + name\n}\n');
  git(saga.sourceRepo, "add", "src/app.go");
  git(saga.sourceRepo, "commit", "-m", "Change the greeting implementation");
  const server = await startSagaServer(saga, null);
  try {
    for (const scheme of ["light", "dark"] as const) {
      await page.emulateMedia({ colorScheme: scheme });
      await page.goto(server.baseURL);
      await waitForSettledSaga(page);
      await expect(page.locator("[data-theme-toggle]")).toHaveAttribute("aria-pressed",String(scheme==="dark"));
      await page.locator(".sidebar").getByRole("link", { name: "Technical design", exact: true }).click();
      await page.locator(".sidebar").getByRole("link", { name: "Architecture", exact: true }).click();
      await expect(page.locator("[data-whiteboard-view]")).toHaveAttribute("data-whiteboard-content-digest", firstBoardPin);
      await expect(page.locator(".whiteboard-stale-elements")).toContainText("1 stale element");
      const element = page.locator('[data-whiteboard-element="greeting"]');
      await expect(element).toHaveAttribute("data-evidence-state", "stale");
      await element.click();
      await page.locator(".diagram-affordance [data-target-code-href]").click();
      await expect(page.locator("#review-drawer")).toBeVisible();
      await expect(page.locator(".drawer-body")).toContainText("stale");
      await expect(page.locator(".drawer-body")).toContainText(saga.identity.head + ":src/app.go");
      await expect(page.locator(".drawer-body")).not.toContainText("Reassessed");
      await page.screenshot({ path: testInfo.outputPath(`whiteboard-evidence-${scheme}.png`) });
      await page.locator("button[data-close-drawer]").click();
      await element.click();
      await page.locator(".diagram-affordance [data-open-stories]").click();
      await expect(page.locator(".drawer-body")).toContainText("The skipper can read the tide.");
      await expect(page.locator(".drawer-body [data-status]")).toHaveAttribute("data-status", "current");
      await page.locator("button[data-close-drawer]").click();
      await element.click();
      await page.locator(".diagram-affordance [data-documentation-target]").click();
      await expect(page.locator("[data-documentation-view]")).toHaveAttribute("data-documentation-pin",originalComponentPin.revision);
      await expect(page.locator('[data-selection="greeting-return"]')).toBeVisible();
      await page.locator("button[data-close-drawer]").click();
      await element.click();
      await page.locator(".diagram-affordance [data-item-record]").click();
      await expect(page).toHaveURL(/\/features\/wave-one$/);
    }
    const reassessedHead=git(saga.sourceRepo,"rev-parse","HEAD");
    componentDefinition.code[0]!.commit=reassessedHead;
    writeFileSync(definition,JSON.stringify(componentDefinition));
    run("component", "revise", "--id", "greeting", "--from", definition, "--delivery", reassessedHead, "--repo", saga.sourceRepo, saga.sagaRoot);
    board.elements[0]!.evidence[0]!.commit = reassessedHead;
    board.elements[0]!.documentation.revision=currentRecordPin(saga,"component","greeting").revision;
    board.elements[0]!.selections[0]!.path[0]!.revision=currentRecordPin(saga,"component","greeting").revision;
    board.elements[0]!.selections[0]!.code.commit=reassessedHead;
    publish();
    const updatedBoardPin=boardPin();
    expect(updatedBoardPin).not.toBe(firstBoardPin);
    await page.goto(server.baseURL);
    await page.locator(".sidebar").getByRole("link", { name: "Technical design", exact: true }).click();
    await page.locator(".sidebar").getByRole("link", { name: "Architecture", exact: true }).click();
    await expect(page.locator("[data-whiteboard-view]")).toHaveAttribute("data-whiteboard-content-digest", updatedBoardPin);
    await expect(page.locator(".whiteboard-stale-elements")).toHaveCount(0);
    await page.locator('[data-whiteboard-element="greeting"]').click();
    await page.locator(".diagram-affordance [data-target-code-href]").click();
    await expect(page.locator(".drawer-body")).toContainText("Reassessed");
    await page.locator("button[data-close-drawer]").click();
    await page.goto(server.baseURL+"/technical/whiteboards/architecture?"+new URLSearchParams({content_digest:firstBoardPin}));
    await waitForSettledSaga(page);
    await expect(page.locator("[data-whiteboard-view]")).toHaveAttribute("data-whiteboard-historical","true");
    await expect(page.locator("[data-whiteboard-view]")).toHaveAttribute("data-whiteboard-content-digest",firstBoardPin);
    await expect(page.locator("[data-whiteboard-view]")).toContainText("Saved whiteboard content from Git");
    await page.locator('[data-whiteboard-element="greeting"]').focus();
    await page.keyboard.press("Enter");
    await page.locator(".diagram-affordance [data-target-code-href]").click();
    await expect(page.locator(".drawer-body")).toContainText(saga.identity.head+":src/app.go");
    await expect(page.locator(".drawer-body")).not.toContainText("Reassessed");
  } finally {
    await stopSagaServer(server);
  }
});
