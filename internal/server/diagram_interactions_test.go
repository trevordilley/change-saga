package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDiagramInteractionLifecycle(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	// Exercise event transitions and the metadata boundary independently of
	// camera geometry. The helper must never read layout during hover/focus.
	prelude := `
const assert = require('node:assert/strict');
class Node {
  constructor(attributes = {}, text = '') {
    this.attributes = attributes; this.dataset = {}; this.textContent = text; this.children = [];
    this.classes = new Set(); this.listeners = new Map();
    this.classList = {add:(...names)=>names.forEach(n=>this.classes.add(n)), remove:(...names)=>names.forEach(n=>this.classes.delete(n)),
      toggle:(name,on)=>on ? this.classes.add(name) : this.classes.delete(name), contains:n=>this.classes.has(n)};
  }
  hasAttribute(name) { return Object.hasOwn(this.attributes,name); }
  getAttribute(name) { return this.attributes[name] ?? null; }
  append(...children) { this.children.push(...children); }
  closest(selector) { return selector === '.diagram-interaction' ? (this.classes.has('diagram-interaction') ? this : this.owner || null) : this.hidden ? this : null; }
  contains(node) { return node === this || this.children.includes(node); }
  addEventListener(type, handler) { this.listeners.set(type,handler); }
  removeEventListener(type) { this.listeners.delete(type); }
  fire(type, target, relatedTarget = null) { this.listeners.get(type)?.({target,relatedTarget,pointerType:'mouse'}); }
  getBoundingClientRect() { throw Error('Hover must not measure diagram geometry'); }
}
const document = {activeElement:null, createElement:tag=>{const n=new Node(); if(tag==='template') n.content=new Node(); return n;}};
const qa = (selector,root) => root.controls;
const q = (selector,node) => node.desc;
let observer;
class MutationObserver { constructor(callback) { this.callback=callback; observer=this; } observe() {} disconnect() { this.disconnected=true; } }
let noteOwner=null, notePopover=null, shown=0, hidden=0, scheduled=0;
const showNote=node=>{noteOwner=node; shown++;};
const hideNote=()=>{noteOwner=null; hidden++;};
const scheduleHideNote=()=>{scheduled++;};
`
	checks := `
const root=new Node(), board=new Node(), controller=new AbortController();
const entity=new Node({'tabindex':'0','data-documentation-target':'entity','data-documentation-revision':'r1'}); entity.dataset.binding='true';
const unbound=new Node({'tabindex':'0','data-documentation-target':'frame','data-documentation-revision':'r1'});
const note=new Node({'tabindex':'0','data-diagram-kind':'node','aria-label':'Authored note'}); note.desc=new Node({},'<script>literal note</script>');
const opaque=new Node({'tabindex':'0'}); opaque.desc=new Node({},'An SVG description is not a grammar note');
root.controls=[entity,unbound,note,opaque];
prepareDiagramInteractions(root,{interactionRoot:board,signal:controller.signal});
assert(entity.classes.has('diagram-interaction')); assert(note.classes.has('diagram-interaction'));
assert(!unbound.classes.has('diagram-interaction')); assert(!opaque.classes.has('diagram-interaction'));
assert.equal(diagramNoteTemplates.get(note).content.children[0].children[1].textContent,'<script>literal note</script>');
root.fire('pointerover',entity); assert(entity.classes.has('diagram-interaction-active')); assert.equal(shown,0);
root.fire('focusin',entity); root.fire('pointerout',entity); assert(entity.classes.has('diagram-interaction-active'));
root.fire('focusout',entity); assert(!entity.classes.has('diagram-interaction-active'));
root.fire('pointerover',note); assert.equal(noteOwner,note); assert.equal(shown,1);
root.fire('pointerout',note); assert.equal(scheduled,1);
root.fire('focusin',note); assert.equal(shown,2);
board.dataset.whiteboardInteracting='true'; observer.callback();
assert(!note.classes.has('diagram-interaction-active')); assert.equal(noteOwner,null);
root.fire('pointerover',note); root.fire('focusin',note); assert.equal(shown,2);
document.activeElement=entity; board.dataset.whiteboardInteracting='false'; observer.callback();
assert(entity.classes.has('diagram-interaction-active'));
entity.hidden=true; root.fire('focusin',entity); assert(!entity.classes.has('diagram-interaction-active'));
entity.hidden=false; root.fire('pointerover',note); assert.equal(noteOwner,note);
controller.abort(); assert(observer.disconnected); assert.equal(root.listeners.size,0); assert.equal(noteOwner,null);
assert(!diagramNoteTemplates.has(note)); assert(!entity.classes.has('diagram-interaction')); assert(!note.classes.has('diagram-interaction-active'));
const aborted=new AbortController(); aborted.abort();
prepareDiagramInteractions(root,{signal:aborted.signal}); assert.equal(root.listeners.size,0);
`
	path := filepath.Join(t.TempDir(), "diagram-interactions.js")
	if err := os.WriteFile(path, []byte(prelude+diagramInteractionsJavaScript+checks), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(node, path).CombinedOutput(); err != nil {
		t.Fatalf("diagram interaction lifecycle: %v\n%s", err, output)
	}
}
