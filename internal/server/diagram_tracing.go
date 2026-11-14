package server

// Trace declared instance endpoints, never inferred names or source artwork.
// Hit regions and paint state belong to the reader, not the saved diagram.
const diagramTracingJavaScript = `
  const diagramTraceHits = new WeakMap();
  function prepareDiagramTracing(root, {interactionRoot = root, signal} = {}) {
    if (!root || signal?.aborted) return;
    const edges = qa('[data-diagram-kind="edge"][data-from][data-to], [data-diagram-kind="connection"][data-from][data-to]',root);
    if (!edges.length) return;
    const nodes = new Map(qa('[data-instance-path]',root)
      .filter(node => ['node','group','frame'].includes(node.dataset.diagramKind))
      .map(node => [node.dataset.instancePath,node]));
    const incidents = new Map(), edgeSet = new Set(edges), regions = new Map();
    const addedFocus = [], lines = [], hits = [];
    for (const edge of edges) {
      regions.set(edge,edge);
      for (const path of new Set([edge.dataset.from,edge.dataset.to])) {
        const parts = path.split('/');
        while (parts.length) {
          const node = nodes.get(parts.join('/'));
          if (node) {
            if (!incidents.has(node)) incidents.set(node,new Set());
            incidents.get(node).add(edge);
          }
          parts.pop();
        }
      }
      // A generous screen-space target makes thin and dashed wires reachable.
      // Keep the visual path first: existing camera routing still owns it.
      for (const line of qa(':scope > path[stroke], :scope > [data-connection-detail] > path[stroke], :scope > [data-connection-overview] > path[stroke]',edge)) {
        line.setAttribute('data-diagram-line','');
        line.style.setProperty('--diagram-edge-width',(Number(line.getAttribute('stroke-width')) || 2)+'px');
        const hit = document.createElementNS('http://www.w3.org/2000/svg','path');
        for (const [key,value] of Object.entries({d:line.getAttribute('d'),fill:'none',stroke:'transparent','stroke-width':'14',
          'stroke-linecap':'round','pointer-events':'stroke','vector-effect':'non-scaling-stroke','aria-hidden':'true','data-diagram-hit':''})) hit.setAttribute(key,value);
        if (line.hasAttribute('transform')) hit.setAttribute('transform',line.getAttribute('transform'));
        line.after(hit); lines.push(line); hits.push(hit); diagramTraceHits.set(line,hit);
      }
    }
    for (const node of nodes.values()) {
      regions.set(node,node);
      if (node.dataset.diagramKind === 'node' && !node.hasAttribute('tabindex') && node.getAttribute('aria-hidden') !== 'true') {
        node.setAttribute('tabindex','0'); addedFocus.push(node);
      }
    }
    let hovered = null, focused = null, dismissed = null;
    const paused = () => interactionRoot.dataset.whiteboardInteracting === 'true';
    const targetFor = target => {
      const node = target?.closest?.('[data-instance-path]');
      return regions.has(node) && !node.closest('[aria-hidden="true"],[hidden],[inert]') ? node : null;
    };
    function paint() {
      const owner = paused() ? null : hovered || focused;
      const active = owner && owner !== dismissed ? (edgeSet.has(owner) ? new Set([owner]) : incidents.get(owner) || new Set()) : null;
      for (const edge of edges) {
        const state = active ? (active.has(edge) ? 'active' : 'muted') : null;
        if (state === null) edge.removeAttribute('data-diagram-trace');
        else if (edge.dataset.diagramTrace !== state) edge.setAttribute('data-diagram-trace',state);
      }
    }
    const enter = event => {
      if (event.pointerType === 'touch' || paused()) return;
      const next = targetFor(event.target);
      if (next === hovered) return;
      hovered = next; dismissed = null; paint();
    };
    const leave = event => {
      if (hovered?.contains(event.relatedTarget)) return;
      hovered = targetFor(event.relatedTarget); paint();
    };
    const focus = event => { focused = targetFor(event.target); dismissed = null; paint(); };
    const blur = event => { focused = targetFor(event.relatedTarget); paint(); };
    const escape = event => {
      if (event.key !== 'Escape') return;
      dismissed = hovered || focused; hovered = null; paint();
    };
    const touch = event => {
      if (event.pointerType !== 'touch' || paused()) return;
      hovered = targetFor(event.target); dismissed = null; paint();
    };
    const listeners = [['pointerover',enter],['pointerout',leave],['focusin',focus],['focusout',blur],['keydown',escape],['pointerdown',touch]];
    for (const [type,handler] of listeners) root.addEventListener(type,handler);
    const observer = new MutationObserver(() => {
      hovered = null;
      focused = paused() ? null : targetFor(document.activeElement);
      paint();
    });
    observer.observe(interactionRoot,{attributes:true,attributeFilter:['data-whiteboard-interacting']});
    signal?.addEventListener('abort',() => {
      observer.disconnect(); hovered = focused = null; paint();
      for (const [type,handler] of listeners) root.removeEventListener(type,handler);
      for (const node of addedFocus) node.removeAttribute('tabindex');
      for (const hit of hits) hit.remove();
      for (const line of lines) { diagramTraceHits.delete(line); line.removeAttribute('data-diagram-line'); line.style.removeProperty('--diagram-edge-width'); }
    },{once:true});
  }
`
