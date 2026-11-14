import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, waitForSettledSaga, expectNoSeriousAccessibilityViolations } from "../support/test.js";
import { runCLI, type SagaFixture } from "../support/fixture-builder.js";

function author(saga: SagaFixture, name: string, data: unknown, ...args: string[]) {
  const path = join(saga.root, name + ".json");
  writeFileSync(path, JSON.stringify(data));
  const result = runCLI(saga, [...args, "--from", path, "--repo", saga.sourceRepo, saga.sagaRoot]);
  expect(result.status, result.stdout + result.stderr).toBe(0);
}

const model = {
  version: 1, width: 1000, height: 900,
  elements: [
    {id:"a",kind:"node",shape:"entity",label:"Account",x:60,y:100,width:220,height:140,style:"normal",fields:[{name:"id",type:"uuid",key:"pk"}]},
    {id:"b",kind:"node",shape:"entity",label:"Order",x:680,y:100,width:220,height:140,style:"normal",fields:[{name:"account_id",type:"uuid",key:"fk"}]},
    {id:"c",kind:"node",shape:"rect",label:"Delivery",x:680,y:420,width:220,height:140,style:"normal"},
    {id:"d",kind:"node",shape:"rect",label:"Worker",x:60,y:420,width:220,height:140,style:"normal"},
    {id:"ab",kind:"edge",from:"a",to:"b",points:[{x:280,y:170},{x:680,y:170}],style:"normal",head:"zero-or-many",line:"dashed"},
    {id:"bc",kind:"edge",from:"b",to:"c",points:[{x:790,y:240},{x:790,y:420}],style:"normal",head:"arrow"},
    {id:"dc",kind:"edge",from:"d",to:"c",points:[{x:280,y:490},{x:680,y:490}],style:"normal",head:"arrow"}
  ]
};

test("hover and keyboard trace declared edges without hiding columns or changing the document @critical", async ({page,saga},testInfo) => {
  author(saga,"trace-board",model,"whiteboard","publish","--id","data-model","--name","Relationship tracing","--revision","r1");
  await page.goto(saga.baseURL+"/technical/erd"); await waitForSettledSaga(page);
  const board=page.locator('[data-whiteboard-view]'),viewport=board.locator('[data-whiteboard-viewport]');
  const edge=(id:string)=>board.locator(`[data-instance-path="${id}"]`);
  const ab=edge('ab'),bc=edge('bc'),dc=edge('dc'),b=edge('b');
  await expect(ab.locator('[data-diagram-hit]')).toHaveCount(1);
  await expect(ab.locator('[data-diagram-hit]')).toHaveAttribute('vector-effect','non-scaling-stroke');
  await expect(ab.locator('[data-diagram-line]')).toHaveAttribute('stroke-dasharray',/./);
  await expect(board.locator('[data-field="account_id"]')).toBeVisible();
  const requests:string[]=[];
  page.on('request',request=>{if(/\/api\/(diagram-annotation|whiteboard-element)/.test(request.url())) requests.push(request.url());});
  const baseWidth=await ab.locator('[data-diagram-line]').evaluate(node=>parseFloat(getComputedStyle(node).strokeWidth));
  // Aim just off the thin painted stroke: the transparent hit region must
  // reach dashed gaps and keep a usable width independently of camera scale.
  const point=await ab.locator('[data-diagram-line]').evaluate(node=>{
    const path=node as SVGPathElement,p=path.getPointAtLength(path.getTotalLength()*.47),m=path.getScreenCTM()!;
    return {x:m.a*p.x+m.c*p.y+m.e,y:m.b*p.x+m.d*p.y+m.f+4};
  });
  await page.mouse.move(point.x,point.y);
  await expect(ab).toHaveAttribute('data-diagram-trace','active');
  await expect(bc).toHaveAttribute('data-diagram-trace','muted');
  await expect(dc).toHaveCSS('opacity','0.07');
  expect(await ab.locator('[data-diagram-line]').evaluate(node=>parseFloat(getComputedStyle(node).strokeWidth))).toBeGreaterThan(baseWidth);
  await board.screenshot({path:testInfo.outputPath('edge-hover-tracing.png')});
  await b.locator('[data-field]').hover();
  await expect(ab).toHaveAttribute('data-diagram-trace','active');
  await expect(bc).toHaveAttribute('data-diagram-trace','active');
  await expect(dc).toHaveAttribute('data-diagram-trace','muted');
  await expect(board.locator('[data-field="account_id"]')).toBeVisible();
  await page.mouse.move(5,5);
  await expect(ab).not.toHaveAttribute('data-diagram-trace');
  await expect(dc).toHaveCSS('opacity','1');
  await b.focus();
  await expect(ab).toHaveAttribute('data-diagram-trace','active');
  await expect(bc).toHaveAttribute('data-diagram-trace','active');
  await expect(dc).toHaveAttribute('data-diagram-trace','muted');
  await page.keyboard.press('Escape');
  await expect(b).toBeFocused();
  await expect(ab).not.toHaveAttribute('data-diagram-trace');
  await viewport.focus(); await page.mouse.move(point.x,point.y);
  await expect(ab).toHaveAttribute('data-diagram-trace','active');
  await viewport.dispatchEvent('wheel',{deltaX:10,bubbles:true,cancelable:true});
  await expect(ab).not.toHaveAttribute('data-diagram-trace');
  await expect(board).toHaveAttribute('data-whiteboard-revision','r1');
  expect(requests).toEqual([]);
  await expectNoSeriousAccessibilityViolations(page,'[data-whiteboard-view]');
});

test("repeated native imports trace their own instance paths and cross-frame connections", async ({page,saga}) => {
  let result=runCLI(saga,['inventory','adopt-format','--format','2',saga.sagaRoot]);
  expect(result.status,result.stdout+result.stderr).toBe(0);
  author(saga,'record-definition',{name:'Record',explanation:'A reusable record.',intent:'proposed',baseline:'none',fields:[{id:'id',name:'id',type:'uuid'}]},'data-entity','add','--id','trace-record');
  const pin={target:'urn:change-saga:wave-one:data-entity:trace-record',revision:'urn:change-saga:wave-one:data-entity:trace-record:revision:r1'};
  const source={version:1,width:600,height:300,elements:[
    {id:'a',kind:'node',shape:'rect',label:'Source',x:20,y:80,width:180,height:120,style:'normal'},
    {id:'b',kind:'node',shape:'rect',label:'Destination',x:400,y:80,width:180,height:120,style:'normal'},
    {id:'link',kind:'edge',from:'a',to:'b',points:[{x:200,y:140},{x:400,y:140}],style:'normal',head:'arrow'}
  ]};
  author(saga,'trace-source',{name:'Shared model',explanation:'Repeated instances keep separate relationship paths.',directory:[pin],diagram:source},'erd','add','--id','trace-model');
  const erd={target:'urn:change-saga:wave-one:erd:trace-model',revision:'urn:change-saga:wave-one:erd:trace-model:revision:r1'};
  author(saga,'copies',{version:1,width:1800,height:1000,elements:[
    {id:'first',kind:'frame',label:'First model',x:50,y:100,width:700,height:350,style:'normal',source:erd},
    {id:'second',kind:'frame',label:'Second model',x:1000,y:100,width:700,height:350,style:'normal',source:erd},
    {id:'cross',kind:'connection',from:'first/a',to:'second/a',style:'normal',head:'arrow'}
  ]},'whiteboard','publish','--id','data-model','--name','Repeated relationship models','--revision','r1');
  await page.goto(saga.baseURL+'/technical/erd?focus=first'); await waitForSettledSaga(page);
  const board=page.locator('[data-whiteboard-view]');
  await expect(board).toHaveAttribute('data-whiteboard-moving','false');
  await board.locator('[data-instance-path="first/a"]').focus();
  await expect(board.locator('[data-instance-path="first/link"]')).toHaveAttribute('data-diagram-trace','active');
  await expect(board.locator('[data-instance-path="cross"]')).toHaveAttribute('data-diagram-trace','active');
  await expect(board.locator('[data-instance-path="second/link"]')).toHaveAttribute('data-diagram-trace','muted');
  await board.locator('[data-instance-path="first/link"]').focus();
  await expect(board.locator('[data-instance-path="first/link"]')).toHaveAttribute('data-diagram-trace','active');
  await expect(board.locator('[data-instance-path="second/link"]')).toHaveAttribute('data-diagram-trace','muted');
  await expect(board.locator('[data-instance-path="cross"]')).toHaveAttribute('data-diagram-trace','muted');
  await board.getByRole('button',{name:'Fit board',exact:true}).click();
  const cross=board.locator('[data-instance-path="cross"]');
  expect(await cross.locator('[data-connection-detail] > [data-diagram-hit]').getAttribute('d'))
    .toBe(await cross.locator('[data-connection-detail] > [data-diagram-line]').getAttribute('d'));
});
