import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga, expectNoSeriousAccessibilityViolations } from "../support/test.js";
import { runCLI, type SagaFixture } from "../support/fixture-builder.js";

function navigationBoard(saga: SagaFixture, withSection = false) {
  const run = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, result.stdout + result.stderr).toBe(0);
  };
  const file = (name: string, value: unknown) => {
    const path = join(saga.root, name + ".json");
    writeFileSync(path, JSON.stringify(value));
    return path;
  };
  const pin = (kind: string, id: string) => ({ target: `urn:change-saga:wave-one:${kind}:${id}`, revision: `urn:change-saga:wave-one:${kind}:${id}:revision:r1` });
  run("inventory", "adopt-format", "--format", "2", saga.sagaRoot);
  run("data-entity", "add", "--id", "record", "--from", file("record", { name: "Saved record", explanation: "A saved record.", intent: "proposed", baseline: "none", fields: [{ id: "id", name: "id", type: "uuid" }] }), "--repo", saga.sourceRepo, saga.sagaRoot);
  for (const id of ["local", "remote"]) {
    run("erd", "add", "--id", id, "--from", file(id, { name: id + " diagram", explanation: "An embedded source.", directory: [pin("data-entity", "record")], diagram: { version: 1, width: 600, height: 300, elements: [{ id: "record", kind: "node", shape: "entity", label: "Saved record", x: 30, y: 30, width: 400, height: 180, style: "normal", source: pin("data-entity", "record"), fields: [{ name: "id", type: "uuid" }] }] } }), "--repo", saga.sourceRepo, saga.sagaRoot);
  }
  // Deliberately author z before a: link resolution must use instance identity,
  // not incidental DOM order, while each repeat retains its own deep link.
  run("diagram", "publish", "--whiteboard", "erd", "--name", "Navigation board", "--revision", "r1", "--from", file("board", { version: 1, width: 2000, height: 1200, elements: [
    ...(withSection ? [{id:"model-section",kind:"group",shape:"section",label:"Local and remote",x:10,y:20,width:1800,height:1100,style:"normal"}] : []),
    { id: "z-copy", kind: "frame", label: "Second local", x: 1100, y: 50, width: 600, height: 300, style: "normal", source: pin("erd", "local") },
    { id: "a-first", kind: "frame", label: "First local", x: 50, y: 50, width: 600, height: 300, style: "normal", source: pin("erd", "local") },
    { id: "source-note", kind: "text", label: "Read local diagram", x: 50, y: 450, width: 700, height: 80, style: "normal", source: pin("erd", "local") },
    { id: "remote", kind: "frame", label: "Remote", x: 50, y: 700, width: 600, height: 300, style: "normal", source: pin("erd", "remote") }
  ] }), saga.sagaRoot);
  run("erd", "revise", "--id", "local", "--from", join(saga.root, "local.json"), "--revision", "r2", "--parent", pin("erd", "local").revision, "--repo", saga.sourceRepo, saga.sagaRoot);
}

test("ERD links focus exact instances without replacing the whiteboard and restore history", async ({ page, saga }) => {
  navigationBoard(saga);
  await page.goto(saga.baseURL + "/technical/erd");
  await waitForSettledSaga(page);
  const board = page.locator("[data-whiteboard-view]");
  await board.evaluate(node => node.setAttribute("data-navigation-sentinel", "kept"));
  const requests: string[] = [];
  page.on("request", request => { if (new URL(request.url()).pathname.startsWith("/technical/")) requests.push(request.url()); });
  // Imported instances remain addressable through provenance, while the
  // sidebar represents one document rather than source records.
  await expect(page.locator('.sidebar a[href^="/technical/erd/"]')).toHaveCount(0);
  await board.locator('[data-whiteboard-focus-frame="a-first"]').evaluate((node: HTMLButtonElement) => node.click());
  await expect(page).toHaveURL(/focus=a-first$/);
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("a-first");
  await board.locator("[data-whiteboard-focus]").selectOption("z-copy");
  await expect(page).toHaveURL(/focus=z-copy$/);
  await expect(board).toHaveAttribute("data-navigation-sentinel", "kept");
  await page.goBack();
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("a-first");
  await page.goBack();
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("");
  await page.goForward();
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("a-first");
  expect(requests).toEqual([]);
  await page.reload();
  await waitForSettledSaga(page);
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("a-first");
  await page.goto(saga.baseURL + "/technical/erd?focus=z-copy");
  await waitForSettledSaga(page);
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("z-copy");
  await page.goto(saga.baseURL + "/technical/erd?focus=removed-frame");
  await waitForSettledSaga(page);
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("");
});

test("latest ERD routes enter the board; exact historical routes and entity inspection survive", async ({ page, saga }) => {
  navigationBoard(saga);
  await page.goto(saga.baseURL + "/technical/erd/local");
  await waitForSettledSaga(page);
  await expect(page).toHaveURL(/\/technical\/erd\?focus=a-first$/);
  await expect(page.locator("[data-whiteboard-focus]")).toHaveValue("a-first");
  await page.locator('[data-instance-path="a-first/record"]').click();
  await expect(page.locator(".diff-drawer.open")).toBeVisible();
  await expect(page).toHaveURL(/focus=a-first$/);
  await page.goto(saga.baseURL + "/technical/erd/local?revision=r1");
  await waitForSettledSaga(page);
  await expect(page.locator('[data-erd-revision="r1"]')).toBeVisible();
  await expect(page.locator("[data-whiteboard-view]")).toHaveCount(0);
  await page.goto(saga.baseURL + "/technical");
  await waitForSettledSaga(page);
  await page.goto(saga.baseURL + "/technical/erd/remote");
  await expect(page).toHaveURL(/\/technical\/erd\?focus=remote$/);
  await expect(page.locator("[data-whiteboard-focus]")).toHaveValue("remote");
});


test("saved ERD links match exact pins and prefer their enclosing repeated instance", async ({ page, saga }) => {
  navigationBoard(saga);
  await page.goto(saga.baseURL + "/technical/erd");
  await waitForSettledSaga(page);
  const board = page.locator("[data-whiteboard-view]");
  await board.locator('[data-instance-path="source-note"]').click();
  await expect(page).toHaveURL(/focus=a-first$/);
  await expect(page.locator(".diff-drawer.open")).toHaveCount(0);
  await board.locator('[data-whiteboard-frame="z-copy"]').evaluate(row => {
    for (const revision of ["r1", "r2"]) {
      const link = document.createElement("a");
      link.id = "saved-" + revision;
      link.href = "/technical/erd/local?revision=" + revision;
      link.textContent = "Read saved " + revision;
      row.append(link);
    }
  });
  await page.locator("#saved-r1").evaluate((link: HTMLAnchorElement) => link.click());
  await expect(page).toHaveURL(/focus=z-copy$/);
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("z-copy");
  await expect(board.locator('[data-whiteboard-frame="z-copy"]')).toHaveAttribute("data-whiteboard-source-href", "/technical/erd/local?revision=r1");
  // r2 exists but is absent from this board: never silently focus its r1 copy.
  await page.locator("#saved-r2").evaluate((link: HTMLAnchorElement) => link.click());
  await expect(page).toHaveURL(/\/technical\/erd\/local\?revision=r2$/);
  await expect(page.locator('[data-erd-revision="r2"]')).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(/focus=z-copy$/);
  await expect(board.locator("[data-whiteboard-focus]")).toHaveValue("z-copy");
});

// One authored model with no ERD inventory records and no embedded frames.
function nativeModel(saga: SagaFixture) {
  const run = (...args: string[]) => {
    const result = runCLI(saga, args, saga.sagaRepo);
    expect(result.status, result.stdout + result.stderr).toBe(0);
  };
  const file = (name: string, value: unknown) => {
    const path = join(saga.root, name + ".json");
    writeFileSync(path, JSON.stringify(value));
    return path;
  };
  const pin = (id: string) => ({ target: `urn:change-saga:wave-one:data-entity:${id}`, revision: `urn:change-saga:wave-one:data-entity:${id}:revision:r1` });
  const code = (id: string, start: number, note: string) => ({ id, commit: "HEAD", path: "src/app.go", start, end: start, note });
  run("inventory", "adopt-format", "--format", "2", saga.sagaRoot);
  run("data-entity", "add", "--id", "edit", "--from", file("edit", {name:"Local edit", explanation:"Authored directly in the model.", intent:"implemented", code:[code("edit",3,"Local edit evidence.")], fields:[{id:"id",name:"id",type:"uuid"}]}), "--delivery", "HEAD", "--repo", saga.sourceRepo, saga.sagaRoot);
  run("data-entity", "add", "--id", "record", "--from", file("record", {name:"Saved record", explanation:"A shared record.", intent:"implemented", fields:[{id:"id",name:"id",type:"uuid"}], code:[code("saved",3,"Entity evidence at its saved commit.")], relationships:[{id:"sync",meaning:"production",destination:pin("edit"),label:"synchronized from",flow:"destination_to_owner",explanation:"A saved edit produces a shared record.",intent:"implemented",code:[code("sync-code",4,"Relationship evidence at its saved commit.")]}]}), "--delivery", "HEAD", "--repo", saga.sourceRepo, saga.sagaRoot);
  run("diagram", "publish", "--whiteboard", "erd", "--name", "One cohesive model", "--revision", "r1", "--from", file("native-board", {version:1,width:3200,height:1500,reveal:"fade",elements:[
    {id:"delivery",kind:"group",shape:"section",label:"Delivery",x:50,y:150,width:1000,height:850,style:"normal",color:"blue"},
    {id:"edit",parent:"delivery",kind:"node",shape:"entity",label:"Local edit",x:80,y:150,width:500,height:240,style:"normal",source:pin("edit"),fields:[{name:"id",type:"uuid"}]},
    {id:"shared",kind:"group",shape:"section",label:"Shared records",x:2100,y:150,width:1000,height:850,style:"normal",color:"green"},
    {id:"record",parent:"shared",kind:"node",shape:"entity",label:"Saved record",x:80,y:150,width:500,height:240,style:"normal",source:pin("record"),fields:[{name:"id",type:"uuid"}]},
    {id:"sync",kind:"connection",from:"delivery/edit",to:"shared/record",label:"synchronized from",head:"arrow",style:"primary",source:{...pin("record"),relationship:"sync"},label_box:{x:1100,y:330,width:900,height:100}},
    {id:"overview-note",kind:"text",visibility:"overview",label:"One model across local and shared data",x:50,y:1100,width:1600,height:100,style:"normal"},
    {id:"detail-note",parent:"shared",kind:"text",visibility:"detail",label:"Exact definitions retain their source evidence",x:80,y:500,width:700,height:180,style:"normal"}
  ]}), saga.sagaRoot);
}

test("one native model exposes sections, fields and exact entity/relationship evidence without child ERDs @critical", async ({ page, saga }) => {
  nativeModel(saga);
  await page.goto(saga.baseURL + "/technical/erd");
  await waitForSettledSaga(page);
  const board = page.locator("[data-whiteboard-view]");
  const select = board.locator("[data-whiteboard-focus]");
  const evidence = board.locator("[data-whiteboard-toolbar] [data-whiteboard-evidence]");
  const sidebar = page.locator(".sidebar");
  await expect(board.locator("[data-frame-path]")).toHaveCount(0);
  await expect(sidebar.locator('a[href="/technical/erd"]')).toHaveCount(1);
  await expect(sidebar.locator('a[href^="/technical/erd/"] , a[href^="/technical/data-entity/"]')).toHaveCount(0);
  await expect(select.locator("option")).toHaveText(["Whole model", "Delivery", "Shared records"]);
  await expect(board.locator('[data-instance-path="overview-note"]')).toBeVisible();
  await expect(board.locator('[data-instance-path="shared/detail-note"]')).toBeHidden();
  await expect(board.locator('[data-field]').first()).toBeHidden();
  await board.evaluate(node => node.setAttribute("data-navigation-sentinel", "kept"));
  const sharedLink = sidebar.locator('a[href="/technical/erd?focus=shared"]');
  await sharedLink.evaluate((node: HTMLAnchorElement) => node.click());
  await expect(select).toHaveValue("shared");
  await expect(sharedLink).toHaveAttribute("aria-current", "page");
  await expect(board).toHaveAttribute("data-whiteboard-moving", "false");
  await expect(board.locator('[data-instance-path="shared/detail-note"]')).toBeVisible();
  await expect(board.getByRole("button", {name:"Replay", exact:true})).toBeVisible();
  const replay = board.locator('[data-whiteboard-replay]');
  const replayPaths = () => replay.evaluate((button: HTMLButtonElement) => {
    button.click();
    return [...button.closest('[data-whiteboard-view]')!.querySelectorAll('[data-reveal-step]')]
      .filter(node => node.getAnimations().length > 0)
      .map(node => node.closest('[data-instance-path]')!.getAttribute('data-instance-path'));
  });
  const animated = await replayPaths();
  expect(animated.length).toBeGreaterThan(0);
  expect(animated.every(path => path?.startsWith('shared/'))).toBe(true);
  await page.emulateMedia({reducedMotion:'reduce'});
  expect(await replayPaths()).toEqual([]);
  await page.emulateMedia({reducedMotion:'no-preference'});
  await expect(board.locator('[data-instance-path="overview-note"]')).toBeHidden();
  await expect(board.locator('[data-instance-path="delivery"]')).toHaveAttribute("data-whiteboard-culled", "true");
  await expect(board.locator('[data-instance-path="delivery/edit"]')).toHaveAttribute("tabindex", "-1");
  await expect(evidence).toBeHidden();
  await expect(evidence).not.toHaveAttribute("data-documentation-target");
  const record = board.locator('[data-instance-path="shared/record"]');
  await record.focus(); await page.keyboard.press("Enter");
  await expect(page.locator("[data-documentation-view]")).toHaveAttribute("data-documentation-pin", /data-entity:record:revision:r1$/);
  await expect(page.locator('.drawer-body [data-file-path="src/app.go"]')).toContainText("Entity evidence at its saved commit.");
  await expect(page.locator('.drawer-body [data-evidence-integrity="verified"]')).toBeVisible();
  await page.keyboard.press("Escape");
  await board.locator("[data-whiteboard-fields]").click();
  await expect(record.locator("[data-field]")).toBeVisible();
  await board.locator("[data-whiteboard-fields]").click();
  await expect(record.locator("[data-field]")).toBeHidden();
  await select.selectOption("");
  await expect(board).toHaveAttribute("data-whiteboard-moving", "false");
  const relationship = board.locator('[data-instance-path="sync"]');
  await expect(relationship).toHaveAttribute("data-whiteboard-from", "delivery/edit");
  await expect(relationship).toHaveAttribute("data-whiteboard-to", "shared/record");
  await relationship.focus(); await page.keyboard.press("Enter");
  await expect(page.locator(".drawer-body")).toContainText("Exact code for relationship sync");
  await expect(page.locator('.drawer-body [data-file-path="src/app.go"]')).toContainText("Relationship evidence at its saved commit.");
  await expect(board.locator("[data-whiteboard-inspection]")).not.toContainText("does not create a recorded relationship");
  await page.keyboard.press("Escape");
  await board.getByRole('button', {name:'Focus Delivery', exact:true}).focus(); await page.keyboard.press("Enter");
  await expect(select).toHaveValue("delivery");
  await expect(board).toHaveAttribute("data-whiteboard-moving", "false");
  await expect(board.locator('[data-instance-path="delivery"]')).toHaveAttribute("data-whiteboard-culled", "false");
  await expect(board.locator('[data-instance-path="delivery/edit"]')).toHaveAttribute("tabindex", "0");
  await expect(page).toHaveURL(/focus=delivery$/);
  await select.selectOption("shared");
  await page.goBack(); await expect(select).toHaveValue("delivery");
  await page.goForward(); await expect(select).toHaveValue("shared");
  await expect(board).toHaveAttribute("data-navigation-sentinel", "kept");
  await page.reload(); await waitForSettledSaga(page);
  await expect(select).toHaveValue("shared");
  await expect(sharedLink).toHaveAttribute("aria-current", "page");
  await expect(board).toHaveAttribute("data-whiteboard-moving", "false");
  await expectNoSeriousAccessibilityViolations(page, "[data-whiteboard-view]");
  const reads = await board.locator('[data-whiteboard-viewport]').evaluate(async viewport => {
    let geometry = 0, queries = 0;
    const bbox = SVGGraphicsElement.prototype.getBBox;
    const query = Element.prototype.querySelectorAll;
    SVGGraphicsElement.prototype.getBBox = function() { geometry++; return bbox.call(this); };
    Element.prototype.querySelectorAll = function(selector: string) { queries++; return query.call(this, selector); };
    for (let i = 0; i < 60; i++) viewport.dispatchEvent(new WheelEvent('wheel', {bubbles:true, cancelable:true, deltaX:1}));
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    SVGGraphicsElement.prototype.getBBox = bbox; Element.prototype.querySelectorAll = query;
    return {geometry, queries};
  });
  expect(reads).toEqual({geometry:0, queries:0});
});


test("section and imported-source focus share history without carrying a source pin into the section", async ({ page, saga }) => {
  navigationBoard(saga, true);
  await page.goto(saga.baseURL + "/technical/erd");
  await waitForSettledSaga(page);
  const board = page.locator("[data-whiteboard-view]");
  const select = board.locator("[data-whiteboard-focus]");
  const evidence = board.locator("[data-whiteboard-toolbar] [data-whiteboard-evidence]");
  await expect(select.locator('optgroup[label="Imported sources"]')).toHaveAttribute("hidden", "");
  await board.locator('[data-whiteboard-focus-frame="a-first"]').evaluate((node: HTMLButtonElement) => node.click());
  await expect(select).toHaveValue("a-first");
  await expect(evidence).toHaveAttribute("data-documentation-revision", /erd:local:revision:r1$/);
  await select.selectOption("model-section");
  await expect(page).toHaveURL(/focus=model-section$/);
  await expect(evidence).toBeHidden();
  await expect(evidence).not.toHaveAttribute("data-documentation-target");
  await page.goBack();
  await expect(select).toHaveValue("a-first");
  await expect(evidence).toHaveAttribute("data-documentation-revision", /erd:local:revision:r1$/);
  await page.goForward();
  await expect(select).toHaveValue("model-section");
  await expect(evidence).not.toHaveAttribute("data-documentation-revision");
});
