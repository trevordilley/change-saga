// Raw SVG QA, not the production reviewer-surface visual-qa command.
import { chromium } from 'playwright';
import { readFile, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
const dir=resolve(process.argv[2]||'runs/first');
const browser=await chromium.launch({headless:true});
const report={browser:browser.version(),network:'all requests blocked',views:[]};
try {
 for(const name of ['before','moved-unrepaired','after','raw-final']){
  let svg=await readFile(join(dir,name+'.svg'),'utf8');
  if(name==='raw-final') svg=svg.replace('./go-regular.ttf','data:font/ttf;base64,'+(await readFile(join(dir,'go-regular.ttf'))).toString('base64'));
  for(const [width,height] of [[1280,720],[1024,576]]){
   const page=await browser.newPage({viewport:{width,height},deviceScaleFactor:1});
   await page.route('**/*',r=>r.abort());
   await page.setContent(`<style>html,body{margin:0;width:100%;height:100%}body>svg{display:block;width:100%;height:100%}</style>${svg}`);
   await page.evaluate(()=>document.fonts.ready);
   const metrics=await page.evaluate(()=>{
    const root=document.querySelector('body>svg');const ids=[...root.querySelectorAll('[id]')].map(n=>n.id);
    const findings=[];const boxes=[];
    for(const e of root.querySelectorAll('[data-item-id]')){
     const b=e.getBoundingClientRect();boxes.push({id:e.id,x:b.x,y:b.y,width:b.width,height:b.height});
     if(b.x < -1||b.y < -1||b.right>innerWidth+1||b.bottom>innerHeight+1)findings.push({id:e.id,kind:'clipped'});
     if(b.width===0||b.height===0){if(e.dataset.kind!=='edge'&&e.dataset.kind!=='graphic')findings.push({id:e.id,kind:'empty'});}
    }
    return {ids,duplicate_ids:ids.filter((v,i)=>ids.indexOf(v)!==i),findings,boxes,font_ready:document.fonts.check('22px DraftGo')};
   });
   await page.screenshot({path:join(dir,`${name}-${width}.png`)});
   report.views.push({name,width,height,...metrics});await page.close();
  }
 }
 report.raw_final_pixel_equal={};
 for(const width of [1280,1024]) report.raw_final_pixel_equal[width]=(await readFile(join(dir,`after-${width}.png`))).equals(await readFile(join(dir,`raw-final-${width}.png`)));
 if(report.views.some(v=>v.findings.length||v.duplicate_ids.length||!v.font_ready)||Object.values(report.raw_final_pixel_equal).some(v=>!v))throw new Error('Visual checks failed: '+JSON.stringify(report));
 await writeFile(join(dir,'visual-qa.json'),JSON.stringify(report,null,2)+'\n');
 console.log(JSON.stringify({views:report.views.length,findings:report.views.flatMap(v=>v.findings),browser:report.browser}));
} finally {await browser.close()}
