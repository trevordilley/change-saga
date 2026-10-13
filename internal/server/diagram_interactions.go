package server

// Inline diagrams reuse the slide note renderer without manufacturing slide
// Items or interpreting arbitrary artwork as entity/evidence metadata.
const diagramInteractionsJavaScript = `
  const diagramNoteTemplates = new WeakMap();
  function prepareDiagramInteractions(root, {interactionRoot = root, signal, affordance, templates} = {}) {
    if (!root || signal?.aborted) return;
    const controls = new Set();
    const notes = [];
    const paused = () => interactionRoot.dataset.whiteboardInteracting === 'true';
    for (const node of qa('[tabindex]', root)) {
      const bound = node.hasAttribute('data-documentation-target') && node.hasAttribute('data-documentation-revision')
        && (node.dataset.binding === 'true' || node.hasAttribute('data-source-element') || node.dataset.diagramEntity === 'true' || (node.hasAttribute('data-diagram-kind') && !node.hasAttribute('data-frame-path')));
      // Only the shared grammar promises that a direct desc is an authored
      // note. Opaque source SVG descriptions are not silently reclassified.
      const authored = templates?.get(node.dataset.instancePath);
      const text = !templates && node.hasAttribute('data-diagram-kind') ? q(':scope > desc', node)?.textContent?.trim() : '';
      if (!bound && !text && !authored) continue;
      controls.add(node);
      node.classList.add('diagram-interaction');
      if (authored) { diagramNoteTemplates.set(node,authored); notes.push(node); continue; }
      if (!text) continue;
      const template = document.createElement('template');
      const body = document.createElement('div'); body.className = 'element-note';
      const label = document.createElement('div'); label.className = 'element-note-label';
      label.textContent = node.getAttribute('aria-label') || 'Element note';
      const content = document.createElement('div'); content.className = 'element-note-body diagram-note-text';
      content.textContent = text;
      body.append(label, content); template.content.append(body);
      diagramNoteTemplates.set(node, template); notes.push(node);
    }
    let hovered = null, focused = null;
    const controlFor = target => {
      const node = target?.closest?.('.diagram-interaction');
      return controls.has(node) && !node.closest('[aria-hidden="true"],[hidden],[inert]') ? node : null;
    };
    const paint = node => node?.classList.toggle('diagram-interaction-active', !paused() && (node === hovered || node === focused));
    const reveal = (node, pointer = null) => {
      if (!node || paused()) return;
      if (affordance) { affordance.show(node, pointer); return; }
      if (!diagramNoteTemplates.has(node)) return;
      if (noteOwner && noteOwner !== node && notePopover?.classList.contains('pinned')) return;
      showNote(node);
    };
    const enter = event => {
      if (event.pointerType === 'touch' || paused()) return;
      const node = controlFor(event.target);
      if (node === hovered) return;
      const previous = hovered; hovered = node; paint(previous); paint(node); reveal(node, event);
    };
    const leave = event => {
      if (!hovered || hovered.contains(event.relatedTarget)) return;
      const previous = hovered; hovered = null; paint(previous);
      affordance?.leave(event);
      delete previous.dataset.noteDismissed;
      if (noteOwner === previous && focused !== previous && !notePopover?.contains(event.relatedTarget)) scheduleHideNote();
    };
    const focus = event => {
      const previous = focused; focused = controlFor(event.target); paint(previous); paint(focused); reveal(focused);
    };
    const blur = event => {
      if (!focused || focused.contains(event.relatedTarget)) return;
      const previous = focused; focused = null; paint(previous);
      affordance?.leave(event);
      delete previous.dataset.noteDismissed;
      if (noteOwner === previous && hovered !== previous && !notePopover?.contains(event.relatedTarget)) hideNote();
    };
    const clear = () => {
      affordance?.clear();
      const oldHovered = hovered, oldFocused = focused; hovered = focused = null;
      paint(oldHovered); paint(oldFocused);
      if (controls.has(noteOwner)) hideNote();
    };
    const listeners = [['pointerover', enter], ['pointermove', enter], ['pointerout', leave], ['focusin', focus], ['focusout', blur]];
    for (const [type, handler] of listeners) root.addEventListener(type, handler);
    const observer = new MutationObserver(() => {
      if (paused()) clear();
      else { focused = controlFor(document.activeElement); paint(focused); }
    });
    observer.observe(interactionRoot, {attributes:true, attributeFilter:['data-whiteboard-interacting']});
    const cleanup = () => {
      clear(); observer.disconnect();
      for (const [type, handler] of listeners) root.removeEventListener(type, handler);
      for (const node of controls) node.classList.remove('diagram-interaction', 'diagram-interaction-active');
      for (const node of notes) { diagramNoteTemplates.delete(node); delete node.dataset.noteDismissed; }
      signal?.removeEventListener('abort', cleanup);
    };
    signal?.addEventListener('abort', cleanup, {once:true});
    return cleanup;
  }
  // One action strip per board; only the hovered pin is materialized. The
  // controls and panels are the same server templates and delegated actions
  // used by implementation slides. No camera loop calls this helper.
  const diagramAffordanceOwners = new WeakMap();
  const diagramAffordanceLifetimes = new WeakMap();
  function diagramAffordanceActionIsCurrent(node) {
    const bar = node?.closest?.('[data-diagram-affordance]');
    if (!bar) return () => true;
    const lifetime = diagramAffordanceLifetimes.get(bar);
    return () => bar.isConnected && lifetime && diagramAffordanceLifetimes.get(bar) === lifetime;
  }
  function diagramAffordanceOwner(node) {
    return diagramAffordanceOwners.get(node?.closest?.('[data-diagram-affordance]')) || node;
  }
  function diagramAffordanceNext(owner) {
    const board = owner?.closest?.('[data-whiteboard-view]');
    if (!board) return null;
    return qa(tabbableSelector,board).find(node => tabbable(node) && !owner.contains(node)
      && Boolean(owner.compareDocumentPosition(node)&Node.DOCUMENT_POSITION_FOLLOWING)) || q('[data-whiteboard-viewport]',board);
  }
  function prepareDiagramAffordance(root, {signal, position}) {
    const bar = document.createElement('div');
    bar.className = 'diagram-affordance'; bar.dataset.diagramAffordance = '';
    bar.setAttribute('role','group'); bar.hidden = true;
    document.body.append(bar);
    const cache = new Map(), inflight = new Map();
    let owner = null, hideTimer = 0, pending = 0, touch = false, quietFocus = false;
    const viewport = root.closest('[data-whiteboard-viewport]');
    const inside = target => target instanceof Node && (owner?.contains(target) || bar.contains(target) || notePopover?.contains(target));
    function clear(focusTarget = null) {
      clearTimeout(hideTimer); pending++;
      const ownsNote = noteOwner && bar.contains(noteOwner);
      const losesFocus = bar.contains(document.activeElement) || (ownsNote && notePopover?.contains(document.activeElement));
      if (ownsNote) hideNote();
      bar.hidden = true; bar.replaceChildren(); owner = null;
      diagramAffordanceOwners.delete(bar); diagramAffordanceLifetimes.delete(bar);
      const destination = focusTarget || (losesFocus ? viewport : null);
      if (destination?.isConnected && !signal.aborted) {
        // Returning to the SVG owner fires focusin synchronously. Dismissal
        // must not recreate the strip through that focus event.
        quietFocus = true;
        try { destination.focus({preventScroll:true}); } finally { quietFocus = false; }
      }
    }
    function leave(event) {
      if (inside(event.relatedTarget)) return;
      clearTimeout(hideTimer);
      hideTimer = setTimeout(() => {
        if (!inside(document.activeElement) && !bar.matches(':hover') && !owner?.matches(':hover') && !notePopover?.matches(':hover')) clear();
      },180);
    }
    function materialize(node) {
      if (!node.dataset.documentationTarget || !node.dataset.documentationRevision) return Promise.resolve('');
      const query = new URLSearchParams({target:node.dataset.documentationTarget,revision:node.dataset.documentationRevision});
      if (node.dataset.sourceRelationship) query.set('relationship',node.dataset.sourceRelationship);
      const key = query.toString();
      if (cache.has(key)) return Promise.resolve(cache.get(key));
      if (!inflight.has(key)) inflight.set(key,fetch('/api/diagram-annotation?'+key,{credentials:'same-origin',signal}).then(async response => {
        const html = await response.text(); if (!response.ok) throw new Error(html.trim()); cache.set(key,html); return html;
      }).finally(() => inflight.delete(key)));
      return inflight.get(key);
    }
    function addNote(node) {
      const template = diagramNoteTemplates.get(node);
      if (!template) return;
      const button = document.createElement('button'); button.type = 'button';
      button.className = 'element-note-trigger'; button.setAttribute('aria-expanded','false');
      button.setAttribute('aria-label','More information: '+(node.getAttribute('aria-label') || 'Element'));
      button.title = button.getAttribute('aria-label');
      button.innerHTML = '<svg class="i" aria-hidden="true" focusable="false"><use href="#i-info"></use></svg>';
      diagramNoteTemplates.set(button,template); bar.append(button);
    }
    function place(node, pointer) {
      const onConnection = node.dataset.diagramKind === 'connection' || node.hasAttribute('data-source-relationship');
      const atPointer = onConnection && Number.isFinite(pointer?.clientX) && Number.isFinite(pointer?.clientY);
      const box = atPointer ? null : position(node);
      if (!atPointer && !box) return false;
      // A long edge may have an offscreen label. Pointer actions belong beside
      // the visible segment, with a four-pixel crossing into the action strip.
      // Keyboard focus keeps the cached authored geometry; neither reads SVG.
      const flip = atPointer && pointer.clientX > innerWidth-180;
      bar.style.transform = flip ? 'translateX(-100%)' : '';
      bar.style.left = (atPointer ? pointer.clientX+(flip ? -4 : 4) : Math.max(8,Math.min(innerWidth-180,box.right-160)))+'px';
      bar.style.top = Math.max(8,Math.min(innerHeight-40,atPointer ? pointer.clientY-14 : box.top+3))+'px';
      return true;
    }
    async function show(node, pointer = null) {
      if (quietFocus) return;
      clearTimeout(hideTimer);
      if (noteOwner && bar.contains(noteOwner) && notePopover?.classList.contains('pinned')) return;
      if (owner === node && !bar.hidden) {
        if (pointer && !bar.contains(document.activeElement)) place(node, pointer);
        return;
      }
      if (!place(node, pointer)) return;
      clear(); owner = node; const request = ++pending;
      diagramAffordanceOwners.set(bar,node); diagramAffordanceLifetimes.set(bar,{});
      bar.setAttribute('aria-label','Actions for '+(node.getAttribute('aria-label') || 'element'));
      addNote(node);
      bar.hidden = false;
      if (node.dataset.documentationTarget) { bar.setAttribute('aria-busy','true'); }
      try {
        const html = await materialize(node);
        if (request !== pending || signal.aborted) return;
        const content = document.createElement('template'); content.innerHTML = html;
        bar.prepend(content.content.cloneNode(true));
        // The shared story control carries its inert, ID-addressable panel
        // template; it lives only as long as this active strip.
        bar.removeAttribute('aria-busy');
      } catch (error) {
        if (request !== pending || signal.aborted) return;
        bar.removeAttribute('aria-busy');
        const retry = document.createElement('button'); retry.type='button'; retry.className='icon-button';
        retry.textContent='Retry'; retry.title=error.message || 'Element actions could not be loaded';
        retry.addEventListener('click',()=>{clear(); void show(node);}); bar.prepend(retry);
      }
    }
    bar.addEventListener('pointerenter',()=>clearTimeout(hideTimer));
    bar.addEventListener('pointerleave',leave);
    bar.addEventListener('focusout',leave);
    const keydown = event => {
      if (event.defaultPrevented || !owner || bar.hidden) return;
      if (event.key === 'Escape' && !noteOwner) { clear(owner); event.preventDefault(); return; }
      if (event.key !== 'Tab') return;
      const buttons = qa('button,a[href]',bar);
      if (event.target === owner && !event.shiftKey && buttons.length) { event.preventDefault(); buttons[0].focus(); }
      else if (event.target === buttons[0] && event.shiftKey) { event.preventDefault(); owner.focus({preventScroll:true}); }
      else if (event.target === buttons[buttons.length-1] && !event.shiftKey) {
        event.preventDefault(); const next = diagramAffordanceNext(owner);
        clear(); next?.focus({preventScroll:true});
      }
    };
    root.addEventListener('keydown',keydown); bar.addEventListener('keydown',keydown);
    // Unbound notes expose the info action on touch and click. Bound elements
    // retain their direct inspection action as well as the hover strip.
    const click = event => {
      const node = event.target.closest?.('.diagram-interaction');
      const tapped = touch; touch = false;
      if (!node || !root.contains(node) || (!tapped && node.hasAttribute('data-documentation-target'))) return;
      event.preventDefault(); event.stopPropagation(); void show(node);
    };
    const pointerdown = event => {touch = event.pointerType === 'touch';};
    root.addEventListener('pointerdown',pointerdown);
    root.addEventListener('click',click,true);
    signal.addEventListener('abort',()=>{clear(); bar.remove(); cache.clear(); root.removeEventListener('keydown',keydown); root.removeEventListener('click',click,true); root.removeEventListener('pointerdown',pointerdown);},{once:true});
    return {show,leave,clear};
  }
`
