package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppJavaScriptSyntaxAndLanguageContract(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	source := strings.Replace(appJavaScript, "})();", "globalThis.changeSagaTest = {languageForPath, tokenClass, pageKey, sanitizeSVGMeasurementClone};})();", 1)
	prelude := `globalThis.document={querySelector:()=>null,querySelectorAll:()=>[],addEventListener:()=>{},body:{dataset:{}}};
globalThis.location={href:'http://127.0.0.1/?view=code',pathname:'/',search:'?view=code',hash:''};
globalThis.history={pushState:()=>{}};globalThis.addEventListener=()=>{};globalThis.innerWidth=1400;
`
	checks := `
const assert = require('node:assert/strict');
function element(attributes) {
  return {attributes:Object.entries(attributes).map(([name,value])=>({name,value})),
    removeAttribute(name){this.attributes=this.attributes.filter(attribute=>attribute.name!==name);}};
}
const root=element({onload:'alert(1)',ONCLICK:'alert(2)',href:'https://example.test',viewBox:'0 0 400 200',width:'400',id:'root'});
const child=element({'xlink:href':'javascript:alert(1)',onmouseover:'alert(1)',transform:'translate(10 20)'});
const local=element({href:'#shape',fill:'red'});
let removed=0;
root.querySelectorAll=selector=>selector==='*' ? [child,local] : [{remove(){removed++;}},{remove(){removed++;}}];
changeSagaTest.sanitizeSVGMeasurementClone(root);
const attrs=node=>Object.fromEntries(node.attributes.map(({name,value})=>[name,value]));
assert.deepEqual(attrs(root),{viewBox:'0 0 400 200',width:'400',id:'root'});
assert.deepEqual(attrs(child),{transform:'translate(10 20)'});
assert.deepEqual(attrs(local),{href:'#shape',fill:'red'});
assert.equal(removed,2);
changeSagaTest.sanitizeSVGMeasurementClone(root);
assert.deepEqual(attrs(root),{viewBox:'0 0 400 200',width:'400',id:'root'});
const named='/technical/whiteboards/architecture?revision=r1';
if(changeSagaTest.pageKey(named+'&focus=area')!==changeSagaTest.pageKey(named))throw new Error('focus must stay inside the named board');
if(changeSagaTest.pageKey(named)===changeSagaTest.pageKey('/technical/whiteboards/architecture?revision=r2'))throw new Error('board revisions must load independently');
if(changeSagaTest.pageKey(named)===changeSagaTest.pageKey('/technical/whiteboards/journey?revision=r1'))throw new Error('board IDs must load independently');
if(changeSagaTest.languageForPath('src/main.go')!=='go')throw new Error('Go language detection failed');
if(changeSagaTest.languageForPath('web/view.tsx')!=='javascript')throw new Error('TSX language detection failed');
for(const prose of ['README.md','docs/guide.mdx','notes.txt','LICENSE','skills/x/SKILL.md'])
  if(changeSagaTest.languageForPath(prose)!=='prose')throw new Error('prose file was treated as code: '+prose);
if(changeSagaTest.languageForPath('config/app.json')!=='json')throw new Error('JSON language detection failed');
if(changeSagaTest.tokenClass('func','go')!=='tok-keyword')throw new Error('Go keyword was not highlighted');
if(changeSagaTest.tokenClass('"x"','go')!=='tok-string')throw new Error('string literal was not highlighted');
`
	path := filepath.Join(t.TempDir(), "appjs-check.js")
	if err := os.WriteFile(path, []byte(prelude+source+checks), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(node, "--check", path).CombinedOutput(); err != nil {
		t.Fatalf("JavaScript syntax check failed: %v\n%s", err, output)
	}
	if output, err := exec.Command(node, path).CombinedOutput(); err != nil {
		t.Fatalf("JavaScript interaction contract failed: %v\n%s", err, output)
	}
}
