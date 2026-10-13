import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga, expectNoSeriousAccessibilityViolations } from "../support/test.js";
import { codeDigest, runCLI, type SagaFixture } from "../support/fixture-builder.js";

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
function nativeModel(saga: SagaFixture, stories = false) {
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
    {id:"edit",parent:"delivery",kind:"node",shape:"entity",label:"Local edit",summary:"Captures local work before delivery.",x:80,y:150,width:500,height:240,style:"normal",source:pin("edit"),fields:[{name:"id",type:"uuid"}]},
    {id:"shared",kind:"group",shape:"section",label:"Shared records",x:2100,y:150,width:1000,height:850,style:"normal",color:"green"},
    {id:"record",parent:"shared",kind:"node",shape:"entity",label:"Saved record",summary:"Preserves the accepted edit for shared access.",description:"The exact shared record definition retains source provenance.",note:"This authored record note belongs to this appearance. [Saved definition](https://example.test/record)",x:80,y:150,width:500,height:240,style:"normal",source:pin("record"),fields:[{name:"id",type:"uuid"},{name:"body",type:"text"}]},
    ...(stories ? [{id:"record-copy",parent:"shared",kind:"node",shape:"entity",label:"Record copy",note:"A **different note** for this repeated appearance.",x:620,y:150,width:300,height:240,style:"normal",source:pin("record"),fields:[{name:"id",type:"uuid"}]}] : []),
    {id:"sync",kind:"connection",from:"delivery/edit",to:"shared/record",label:"synchronized from",head:"arrow",style:"primary",source:{...pin("record"),relationship:"sync"},label_box:{x:1100,y:330,width:900,height:100}},
    {id:"overview-note",kind:"text",visibility:"overview",label:"One model across local and shared data",x:50,y:1100,width:1600,height:100,style:"normal"},
    {id:"detail-note",parent:"shared",kind:"text",visibility:"detail",label:"Exact definitions retain their source evidence",x:80,y:500,width:700,height:180,style:"normal"}
  ]}), saga.sagaRoot);
  if (stories) {
    run("add-deck", "--feature", "wave-one", "--id", "record-flow", "--title", "Record flow", "--objective", "Explain shared records.", saga.sagaRoot, "Record flow");
    const story = "urn:change-saga:wave-one:story:record-story";
    run("story", "add", "--feature", "wave-one", "--id", "record-story", "--revision", "r1", "--event", "proposed", "--title", "Read shared records", "--statement", "As a reader I can read shared records.", "--criterion", "saved=The accepted record is readable", saga.sagaRoot);
    run("apply-slide", "--from", file("record-slide", {
      version:1,operation:"create",request_id:"record-slide",deck:"record-flow",expected_snapshot:"absent",
      slide:{id:"record-slide",title:"Record flow",rank:10,intent:"explain",layout:"diagram",takeaway:"Saved records retain their evidence.",reading_order:["record"]},
      diagram:{version:1,width:800,height:400,elements:[{id:"record",kind:"node",shape:"rect",label:"Saved record",x:50,y:50,width:400,height:200,style:"normal"}]},
      items:[{id:"record",rank:10,kind:"node",label:"Recorded implementation element",description:"Reads this exact saved definition.",selector:{type:"element",element_id:"record"},documentation:pin("record"),evidence:[{version:2,references:[{commit:saga.identity.head,path:"src/app.go",start:3,end:3,digest:codeDigest(saga.sourceRepo,saga.identity.head,"src/app.go",3),note:"This real slide explains the shared record."}]}],criterion_links:[{id:"record-saved",criterion:story+":criterion:saved",story_revision:story+":revision:r1",rationale:"This implementation element explains reading a saved record."}]}]
    }), "--repo", saga.sourceRepo, saga.sagaRoot);
  }

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
  await expect(record).toHaveAccessibleName("Inspect Saved record");
  await expect(record.locator('[data-diagram-summary="entity"]')).toBeVisible();
  await expect(record.locator('[data-diagram-summary="entity"]')).toContainText("Preserves the accepted edit for shared access.");
  await expect(record.locator('[data-diagram-detail="entity"]')).toBeHidden();
  expect(await record.locator('[data-diagram-detail="entity"] > path').evaluate(node => Boolean(node.closest('[aria-hidden="true"]')))).toBe(true);
  await record.focus(); await page.keyboard.press("Enter");
  await expect(page.locator("[data-documentation-view]")).toHaveAttribute("data-documentation-pin", /data-entity:record:revision:r1$/);
  await expect(board.locator("[data-whiteboard-inspection]")).toBeHidden();
  await expect(page.locator('.drawer-body [data-file-path="src/app.go"]')).toContainText("Entity evidence at its saved commit.");
  await expect(page.locator('.drawer-body [data-evidence-integrity="verified"]')).toBeVisible();
  await page.keyboard.press("Escape");
  await board.locator("[data-whiteboard-fields]").click();
  await expect(record.locator("[data-field]").first()).toBeVisible();
  expect(await record.locator('[data-diagram-detail="entity"] > path').evaluate(node => Boolean(node.closest('[aria-hidden="true"]')))).toBe(false);
  await expect(record.locator('[data-diagram-detail="entity"]')).toBeVisible();
  await expect(record.locator('[data-diagram-summary="entity"]')).toBeHidden();
  await board.locator("[data-whiteboard-fields]").click();
  await expect(record.locator("[data-field]").first()).toBeHidden();
  expect(await record.locator('[data-diagram-detail="entity"] > path').evaluate(node => Boolean(node.closest('[aria-hidden="true"]')))).toBe(true);
  await expect(record.locator('[data-diagram-summary="entity"]')).toBeVisible();
  await expect(record.locator('[data-diagram-detail="entity"]')).toBeHidden();
  await select.selectOption("");
  await expect(record.locator('[data-diagram-summary="entity"]')).toBeVisible();
  await expect(board).toHaveAttribute("data-whiteboard-moving", "false");
  const relationship = board.locator('[data-instance-path="sync"]');
  await expect(relationship).toHaveAttribute("data-whiteboard-from", "delivery/edit");
  await expect(relationship).toHaveAttribute("data-whiteboard-to", "shared/record");
  await relationship.focus(); await page.keyboard.press("Enter");
  await expect(page.locator(".drawer-body")).toContainText("Exact code for relationship sync");
  await expect(page.locator('.drawer-body [data-file-path="src/app.go"]')).toContainText("Relationship evidence at its saved commit.");
  await expect(board.locator("[data-whiteboard-inspection]")).toHaveCount(0);
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


test("native hover reuses slide actions and loads real pinned code and Item story panels lazily", async ({page,saga}, testInfo) => {
  nativeModel(saga,true);
  const requests:string[]=[];
  page.on("request",request=>{if(request.url().includes("/api/diagram-annotation")) requests.push(request.url());});
  await page.goto(saga.baseURL+"/technical/erd"); await waitForSettledSaga(page);
  const board=page.locator('[data-whiteboard-view]'), bar=page.locator('[data-diagram-affordance]');
  await board.locator('[data-whiteboard-focus]').selectOption('shared');
  await expect(board).toHaveAttribute('data-whiteboard-moving','false');
  expect(requests).toHaveLength(0);
  const record=board.locator('[data-instance-path="shared/record"]');
  await record.hover();
  await expect(bar.locator('.landmark-affordance [data-documentation-target]')).toBeVisible();
  await expect(bar.locator('[data-open-stories]')).toBeVisible();
  await expect(bar.locator('[data-target-code-href]')).toBeVisible();
  await expect(page.locator('#element-note-popover')).toBeHidden();
  expect(requests).toHaveLength(1);
  expect(requests[0]).not.toContain('panel=code');
  await page.screenshot({path:testInfo.outputPath('native-hover-actions.png')});
  await bar.locator('.element-note-trigger').click();
  await expect(page.locator('#element-note-popover')).toContainText('authored record note');
  await page.keyboard.press('Tab');
  await expect(page.locator('#element-note-popover').getByRole('link',{name:'Saved definition'})).toBeFocused();
  await page.keyboard.press('Shift+Tab');
  await expect(bar.locator('.element-note-trigger')).toBeFocused();
  await page.keyboard.press('Escape');
  await record.focus(); await page.keyboard.press('Tab');
  await expect(bar.locator('[data-documentation-target]')).toBeFocused();
  await bar.locator('[data-open-stories]').click();
  await expect(page.locator('.drawer-body')).toContainText('Read shared records');
  await expect(page.locator('.drawer-body [data-story-provenance]')).toContainText('pin this exact saved definition');
  await expect(page.locator('.drawer-body')).toContainText('Recorded implementation element');
  await page.locator('.diff-drawer').evaluate(async node=>{await Promise.all(node.getAnimations().map(animation=>animation.finished));});
  await page.screenshot({path:testInfo.outputPath('native-story-provenance.png')});
  await page.keyboard.press('Escape'); await expect(record).toBeFocused();
  await record.hover();
  await bar.locator('[data-target-code-href]').click();
  await expect(page.locator('.drawer-body [data-file-path="src/app.go"]')).toContainText('Entity evidence at its saved commit.');
  await expect(page.locator('.drawer-body [data-evidence-integrity="verified"]')).toBeVisible();
  expect(requests.filter(url=>url.includes('panel=code'))).toHaveLength(1);
  await page.locator('.diff-drawer').evaluate(async node=>{await Promise.all(node.getAnimations().map(animation=>animation.finished));});
  await page.screenshot({path:testInfo.outputPath('native-code-evidence.png')});
  await page.keyboard.press('Escape'); await expect(record).toBeFocused();
  await board.locator('[data-instance-path="shared/record-copy"]').hover();
  await bar.locator('.element-note-trigger').click();
  await expect(page.locator('#element-note-popover strong')).toHaveText('different note');
  await expect(page.locator('#element-note-popover')).not.toContainText('authored record note');
  expect(requests.filter(url=>!url.includes('panel=code'))).toHaveLength(1);
  await page.keyboard.press('Escape');

  await board.locator('[data-whiteboard-focus]').selectOption('');
  await expect(board).toHaveAttribute('data-whiteboard-moving','false');
  const relationship=board.locator('[data-instance-path="sync"]');
  await relationship.focus();
  await expect(bar.locator('[data-source-relationship="sync"]')).toBeVisible();
  await expect(bar.locator('[data-open-stories]')).toHaveCount(0);
  await bar.locator('[data-target-code-href]').click();
  await expect(page.locator('.drawer-body')).toContainText('Relationship evidence at its saved commit.');
  await expect(page.locator('.drawer-body')).not.toContainText('Entity evidence at its saved commit.');
  await page.keyboard.press('Escape');
  await board.locator('[data-whiteboard-focus]').selectOption('delivery');
  await expect(board).toHaveAttribute('data-whiteboard-moving','false');
  await expect(board).toHaveAttribute('data-whiteboard-interacting','false');
  await board.locator('[data-instance-path="delivery/edit"]').hover();
  await expect(bar.locator('[data-documentation-target]')).toBeVisible();
  await expect(bar.locator('[data-open-stories]')).toHaveCount(0);
  await expect(bar.locator('.element-note-trigger')).toHaveCount(0);
  await expectNoSeriousAccessibilityViolations(page,'[data-diagram-affordance]');
});

for (const exit of ['pan','leave','navigate']) test("late annotation responses cannot restore an overlay after "+exit, async ({page,saga}) => {
  nativeModel(saga);
  let release:()=>void=()=>{};
  const held=new Promise<void>(resolve=>{release=resolve;});
  await page.route('**/api/diagram-annotation?**',async route=>{await held;await route.continue();});
  await page.goto(saga.baseURL+'/technical/erd'); await waitForSettledSaga(page);
  const board=page.locator('[data-whiteboard-view]'),bar=page.locator('[data-diagram-affordance]');
  await board.locator('[data-whiteboard-focus]').selectOption('shared');
  await expect(board).toHaveAttribute('data-whiteboard-moving','false');
  await board.locator('[data-instance-path="shared/record"]').hover();
  await expect(bar).toHaveAttribute('aria-busy','true');
  if (exit === 'pan') await board.locator('[data-whiteboard-viewport]').dispatchEvent('wheel',{deltaX:40,bubbles:true,cancelable:true});
  else if (exit === 'leave') await page.mouse.move(10,10);
  else await page.goto(saga.baseURL+'/technical');
  await expect(bar).toBeHidden();
  if (exit === 'navigate') { release(); await waitForSettledSaga(page); await expect(page.locator('[data-diagram-affordance]')).toHaveCount(0); }
  else { const response=page.waitForResponse(response=>response.url().includes('/api/diagram-annotation')); release(); await response; await expect(bar).toBeHidden(); }
});

test("touch readers reach the shared native action strip before opening a panel", async ({browser,saga}) => {
  nativeModel(saga,true);
  const context=await browser.newContext({hasTouch:true,viewport:{width:1280,height:800}});
  const page=await context.newPage();
  try {
    await page.goto(saga.baseURL+'/technical/erd?focus=shared'); await waitForSettledSaga(page);
    const board=page.locator('[data-whiteboard-view]'),bar=page.locator('[data-diagram-affordance]');
    await expect(board).toHaveAttribute('data-whiteboard-interacting','false');
    await board.locator('[data-instance-path="shared/record"]').tap();
    await expect(bar.locator('[data-open-stories]')).toBeVisible();
    await expect(page.locator('.diff-drawer')).not.toHaveClass(/open/);
    await bar.locator('[data-open-stories]').tap();
    await expect(page.locator('.drawer-body')).toContainText('Read shared records');
  } finally { await context.close(); }
});

test("native annotation dismissal, note exit and camera changes preserve keyboard focus", async ({page,saga}) => {
  nativeModel(saga,true);
  await page.goto(saga.baseURL+'/technical/erd?focus=shared'); await waitForSettledSaga(page);
  const board=page.locator('[data-whiteboard-view]'),bar=page.locator('[data-diagram-affordance]');
  const viewport=board.locator('[data-whiteboard-viewport]');
  const record=board.locator('[data-instance-path="shared/record"]');
  const copy=board.locator('[data-instance-path="shared/record-copy"]');
  const settle=async()=>{await expect(board).toHaveAttribute('data-whiteboard-interacting','false');};
  const actions=async()=>{
    await settle(); await viewport.focus(); await record.focus();
    await expect(bar.locator('[data-documentation-target]')).toBeVisible();
    await page.keyboard.press('Tab'); await expect(bar.locator('[data-documentation-target]')).toBeFocused();
  };
  await actions();
  await page.keyboard.press('Escape');
  await expect(record).toBeFocused(); await expect(bar).toBeHidden();
  // Include queued focus work: dismissal must not synchronously recreate the strip.
  await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  await expect(bar).toBeHidden();
  await actions();
  await bar.locator('.element-note-trigger').focus();
  await page.keyboard.press('Tab');
  await expect(page.locator('#element-note-popover').getByRole('link',{name:'Saved definition'})).toBeFocused();
  await page.keyboard.press('Tab');
  await expect(copy).toBeFocused(); await expect(page.locator('#element-note-popover')).toBeHidden();
  await expect(bar.locator('.element-note-trigger')).toBeVisible();
  await bar.locator('.element-note-trigger').focus();
  await page.keyboard.press('Escape'); // note
  await page.keyboard.press('Escape'); // action strip
  await expect(copy).toBeFocused(); await expect(bar).toBeHidden();
  await actions();
  await viewport.dispatchEvent('wheel',{deltaX:10,bubbles:true,cancelable:true});
  await expect(bar).toBeHidden(); await expect(viewport).toBeFocused();
  await actions();
  await page.setViewportSize({width:1260,height:760});
  await expect(bar).toBeHidden(); await expect(viewport).toBeFocused();
});

for (const exit of ['pan','navigate','revisit']) test("late native code panels cannot reopen after "+exit, async ({page,saga}) => {
  nativeModel(saga,true);
  let release:()=>void=()=>{};
  const held=new Promise<void>(resolve=>{release=resolve;});
  await page.route('**/api/diagram-annotation?**',async route=>{
    if (new URL(route.request().url()).searchParams.get('panel') !== 'code') { await route.continue(); return; }
    await held; await route.continue();
  });
  await page.goto(saga.baseURL+'/technical/erd?focus=shared'); await waitForSettledSaga(page);
  const board=page.locator('[data-whiteboard-view]'),bar=page.locator('[data-diagram-affordance]');
  const record=board.locator('[data-instance-path="shared/record"]');
  await expect(board).toHaveAttribute('data-whiteboard-interacting','false');
  await record.focus();
  const code=bar.locator('[data-target-code-href]');
  await expect(code).toBeVisible();
  const requested=page.waitForRequest(request=>new URL(request.url()).searchParams.get('panel') === 'code');
  await code.focus(); await page.keyboard.press('Enter'); await requested;
  await expect(code).toHaveAttribute('aria-busy','true');
  if (exit === 'navigate') {
    // Use in-app navigation so the old promise survives in this document.
    await page.locator('html').evaluate(node=>node.setAttribute('data-code-race-document','same'));
    await page.locator('.sidebar-title[href="/"]').click();
    await expect(board).toHaveCount(0); await waitForSettledSaga(page);
    await expect(page.locator('html')).toHaveAttribute('data-code-race-document','same');
  } else {
    await board.locator('[data-whiteboard-viewport]').dispatchEvent('wheel',{deltaX:10,bubbles:true,cancelable:true});
    await expect(bar).toBeHidden();
    if (exit === 'revisit') {
      await expect(board).toHaveAttribute('data-whiteboard-interacting','false');
      await record.focus(); await expect(bar.locator('[data-target-code-href]')).toBeVisible();
    }
  }
  const response=page.waitForResponse(response=>new URL(response.url()).searchParams.get('panel') === 'code');
  release(); await (await response).finished();
  await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  await expect(page.locator('.diff-drawer')).not.toHaveClass(/open/);
  if (exit === 'revisit') {
    await bar.locator('[data-open-diffs]').click();
    await expect(page.locator('.drawer-body')).toContainText('Entity evidence at its saved commit.');
  }
});
