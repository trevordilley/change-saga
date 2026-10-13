package server

// Inline diagrams reuse the slide note renderer without manufacturing slide
// Items or interpreting arbitrary artwork as entity/evidence metadata.
const diagramInteractionsJavaScript = `
  const diagramNoteTemplates = new WeakMap();
  function prepareDiagramInteractions(root, {interactionRoot = root, signal} = {}) {
    if (!root || signal?.aborted) return;
    const controls = new Set();
    const notes = [];
    const paused = () => interactionRoot.dataset.whiteboardInteracting === 'true';
    for (const node of qa('[tabindex]', root)) {
      const bound = node.hasAttribute('data-documentation-target') && node.hasAttribute('data-documentation-revision')
        && (node.dataset.binding === 'true' || node.hasAttribute('data-source-element') || node.dataset.diagramEntity === 'true');
      // Only the shared grammar promises that a direct desc is an authored
      // note. Opaque source SVG descriptions are not silently reclassified.
      const text = node.hasAttribute('data-diagram-kind') ? q(':scope > desc', node)?.textContent?.trim() : '';
      if (!bound && !text) continue;
      controls.add(node);
      node.classList.add('diagram-interaction');
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
    const reveal = node => {
      if (!node || paused() || !diagramNoteTemplates.has(node)) return;
      if (noteOwner && noteOwner !== node && notePopover?.classList.contains('pinned')) return;
      showNote(node);
    };
    const enter = event => {
      if (event.pointerType === 'touch' || paused()) return;
      const node = controlFor(event.target);
      if (node === hovered) return;
      const previous = hovered; hovered = node; paint(previous); paint(node); reveal(node);
    };
    const leave = event => {
      if (!hovered || hovered.contains(event.relatedTarget)) return;
      const previous = hovered; hovered = null; paint(previous);
      delete previous.dataset.noteDismissed;
      if (noteOwner === previous && focused !== previous && !notePopover?.contains(event.relatedTarget)) scheduleHideNote();
    };
    const focus = event => {
      const previous = focused; focused = controlFor(event.target); paint(previous); paint(focused); reveal(focused);
    };
    const blur = event => {
      if (!focused || focused.contains(event.relatedTarget)) return;
      const previous = focused; focused = null; paint(previous);
      delete previous.dataset.noteDismissed;
      if (noteOwner === previous && hovered !== previous && !notePopover?.contains(event.relatedTarget)) hideNote();
    };
    const clear = () => {
      const oldHovered = hovered, oldFocused = focused; hovered = focused = null;
      paint(oldHovered); paint(oldFocused);
      if (controls.has(noteOwner)) hideNote();
    };
    const listeners = [['pointerover', enter], ['pointerout', leave], ['focusin', focus], ['focusout', blur]];
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
`
