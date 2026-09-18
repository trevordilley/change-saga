package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppJavaScriptSyntaxAndRangeSelectionContract(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	source := strings.Replace(appJavaScript, "})();", "globalThis.changeSagaTest = {languageForPath, parseCodeLocation, selectedRangeRef, normalizedAnnotationColor, colorWithAlpha, shortcutDirection, annotationDeleteShortcut, translateShape, stepShapeDraftHistory, clampNormalized, stickyNoteAnchor, translateNote, annotationLabel};})();", 1)
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
const commit='b'.repeat(40);
const rows=[{dataset:{line:'11',diffRef:commit+':web/app.go#L11'}},{dataset:{line:'13'}}];
if(changeSagaTest.selectedRangeRef(rows)!==commit+':web/app.go#L11-L13')throw new Error('code range selection failed');
if(changeSagaTest.selectedRangeRef(rows.slice(0,1))!==commit+':web/app.go#L11')throw new Error('single-line selection must use the canonical #L<n> form');
const line=changeSagaTest.parseCodeLocation(commit+':web/app.go#L4-L9');
if(!line||line.commit!==commit||line.path!=='web/app.go'||line.start!==4||line.end!==9)throw new Error('code location parse failed');
const whole=changeSagaTest.parseCodeLocation(commit+':web/app.go');
if(!whole||whole.path!=='web/app.go'||'start' in whole||'end' in whole)throw new Error('whole-file location parse failed');
if(changeSagaTest.parseCodeLocation('saga-diff://v1/line?path=app.go')!==null||changeSagaTest.parseCodeLocation('abc:app.go')!==null)throw new Error('non-canonical locations must not parse');
if(changeSagaTest.normalizedAnnotationColor('#A1b2C3')!=='#a1b2c3')throw new Error('annotation color normalization failed');
if(changeSagaTest.normalizedAnnotationColor('red')!=='#d04832')throw new Error('unsafe annotation color fallback failed');
if(changeSagaTest.colorWithAlpha('#112233')!=='#11223355')throw new Error('highlight alpha failed');
if(changeSagaTest.shortcutDirection({key:'z',metaKey:true})!=='undo')throw new Error('Command+Z undo failed');
if(changeSagaTest.shortcutDirection({key:'Z',ctrlKey:true,shiftKey:true})!=='redo')throw new Error('Ctrl+Shift+Z redo failed');
if(changeSagaTest.shortcutDirection({key:'y',ctrlKey:true})!=='redo')throw new Error('Ctrl+Y redo failed');
if(changeSagaTest.shortcutDirection({key:'z',ctrlKey:true,altKey:true})!=='')throw new Error('modified shortcut should be ignored');
const canvasTarget={matches:()=>false};const textTarget={matches:()=>true};
if(!changeSagaTest.annotationDeleteShortcut({key:'Delete',target:canvasTarget}))throw new Error('Delete annotation shortcut failed');
if(!changeSagaTest.annotationDeleteShortcut({key:'Backspace',target:canvasTarget}))throw new Error('Backspace annotation shortcut failed');
if(changeSagaTest.annotationDeleteShortcut({key:'Backspace',target:textTarget})||changeSagaTest.annotationDeleteShortcut({key:'Delete',metaKey:true,target:canvasTarget}))throw new Error('annotation delete shortcut escaped its scope');
const movedRect=changeSagaTest.translateShape({type:'rect',x:.8,y:.8,width:.2,height:.2},.4,.4);
if(movedRect.x!==.8||movedRect.y!==.8)throw new Error('rectangle movement must remain normalized');
const movedPath=changeSagaTest.translateShape({type:'path',points:[{x:.1,y:.2},{x:.3,y:.4}]},.2,-.1);
if(Math.abs(movedPath.points[0].x-.3)>.0001||Math.abs(movedPath.points[0].y-.1)>.0001)throw new Error('freehand movement failed');
const empty={type:'region',coordinate_space:'normalized',shapes:[]};
const rectangle={type:'region',coordinate_space:'normalized',shapes:[{type:'rect',x:.1,y:.1,width:.2,height:.2}]};
const freehand={type:'drawing',coordinate_space:'normalized',shapes:[...rectangle.shapes,{type:'path',points:[{x:.2,y:.2},{x:.4,y:.4}]}]};
const draft={anchor:freehand,undo:[empty,rectangle],redo:[]};
if(!changeSagaTest.stepShapeDraftHistory(draft,'undo')||draft.anchor.shapes.length!==1||draft.redo.length!==1)throw new Error('gesture undo failed');
if(!changeSagaTest.stepShapeDraftHistory(draft,'redo')||draft.anchor.shapes.length!==2||draft.undo.length!==2)throw new Error('gesture redo failed');
if(changeSagaTest.clampNormalized(1.4)!==1||changeSagaTest.clampNormalized(-.3)!==0||changeSagaTest.clampNormalized('x')!==0)throw new Error('normalized placement clamp failed');
if(changeSagaTest.annotationLabel({type:'note'})!=='sticky note')throw new Error('sticky note history label failed');
if(changeSagaTest.normalizedAnnotationColor('not-a-color','#f2bd4b')!=='#f2bd4b')throw new Error('sticky note color fallback failed');
const sticky=changeSagaTest.stickyNoteAnchor('Ship it',1.4,-.2,'oops');
if(sticky.type!=='note'||sticky.coordinate_space!=='normalized')throw new Error('sticky anchor shape failed');
if(sticky.note.text!=='Ship it'||sticky.note.x!==1||sticky.note.y!==0||sticky.note.color!=='#f2bd4b')throw new Error('sticky anchor normalization failed');
const moved=changeSagaTest.translateNote(sticky.note,-.25,.5);
if(Math.abs(moved.x-.75)>.0001||moved.y!==.5||moved.text!=='Ship it')throw new Error('sticky note movement failed');
if(changeSagaTest.translateNote(moved,.5,-.9).x!==1||changeSagaTest.translateNote(moved,.5,-.9).y!==0)throw new Error('sticky note movement must stay normalized');
const noteDraft={noteDraft:true,anchor:changeSagaTest.stickyNoteAnchor('Ship it',.4,.4),undo:[changeSagaTest.stickyNoteAnchor('Ship it',.1,.1)],redo:[]};
if(!changeSagaTest.stepShapeDraftHistory(noteDraft,'undo')||noteDraft.anchor.note.x!==.1||noteDraft.redo.length!==1)throw new Error('sticky note undo failed');
if(!changeSagaTest.stepShapeDraftHistory(noteDraft,'redo')||noteDraft.anchor.note.x!==.4)throw new Error('sticky note redo failed');
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
