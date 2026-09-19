package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppJavaScriptSyntaxAndLanguageContract(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	source := strings.Replace(appJavaScript, "})();", "globalThis.changeSagaTest = {languageForPath, tokenClass};})();", 1)
	prelude := `globalThis.document={querySelector:()=>null,querySelectorAll:()=>[],addEventListener:()=>{},body:{dataset:{}}};
globalThis.location={href:'http://127.0.0.1/?view=code',pathname:'/',search:'?view=code',hash:''};
globalThis.history={pushState:()=>{}};globalThis.addEventListener=()=>{};globalThis.innerWidth=1400;
`
	checks := `
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
