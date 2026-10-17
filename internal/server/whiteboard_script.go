package server

// This runs inside the application shell and shares its page lifecycle and
// pinned-definition drawer. It never persists a pan, focus, or selection.
const whiteboardJavaScript = `
  function prepareWhiteboards(root) {
    for (const board of within(root, '[data-whiteboard-view]')) {
      const viewport = q('[data-whiteboard-viewport]', board), svg = viewport && q('svg', viewport);
      if (!svg || board.dataset.whiteboardReady) continue;
      board.dataset.whiteboardReady = 'true';
      [svg, ...qa('svg', svg)].forEach(drawing => drawing.setAttribute('role', 'group'));
      qa('image', svg).forEach(drawing => drawing.setAttribute('role', 'img'));
      const original = svg.viewBox.baseVal;
      const bounds = {x: original.x, y: original.y, width: original.width, height: original.height};
      const openingJSON = q('[data-whiteboard-metadata]', board).dataset.whiteboardOpening;
      const opening = openingJSON ? JSON.parse(openingJSON) : bounds;
      let view = {...bounds}, focused = '', fitted = false, atStart = true, motion = 0, render = 0, interactionTimer = 0;
      let viewportRect = viewport.getBoundingClientRect();
      const frameRows = qa('[data-whiteboard-frame]', board);
      const geometry = new Map(JSON.parse(q('[data-whiteboard-metadata]', board).dataset.whiteboardMetadata).map(item => [item.path,item]));
      const frames = qa('[data-frame-path]', svg);
      const sections = new Set(qa('[data-whiteboard-section]', board).map(row => row.dataset.whiteboardSection));
      // Fields on root-authored entities share the same on-demand control.
      for (const field of qa('[data-field]', svg)) {
        field.style.display = 'none'; field.setAttribute('aria-hidden','true');
      }
      for (const node of qa('[data-diagram-detail="entity"]', svg)) {
        node.style.display = 'none'; node.setAttribute('aria-hidden','true');
      }
      for (const node of qa('[data-diagram-summary="entity"]', svg)) {
        node.style.display = ''; node.setAttribute('aria-hidden','false');
      }
      const select = q('[data-whiteboard-focus]', board);
      const level = q('[data-whiteboard-level]', board);
      const evidence = q('[data-whiteboard-toolbar] [data-whiteboard-evidence]', board);
      const rows = new Map(frameRows.map(row => [row.dataset.whiteboardFrame,row]));
      const rowFor = path => rows.get(path);
      const measured = new Map(qa('[data-binding], [data-diagram-entity]', svg).map(node => [node,node.getBBox()]));
      const entityEntries = qa('[data-diagram-entity="true"]', svg).filter(node => !q('[data-diagram-entity]', node)).map(entity => ({entity, box:measured.get(entity), owner:entity.closest('[data-frame-path]')?.dataset.framePath}));
      // Authored SVG has entity bindings, not field metadata. Summarize each
      // real bound group in its measured native bounds; detail restores the
      // exact authored children. Endpoints and source identity never move.
      function prepareEntitySummaries(path) {
      for (const entry of entityEntries) {
        if (entry.owner !== path || entry.ready) continue;
        entry.ready = true;
        const {entity,box} = entry;
        if (!(box.width > 0 && box.height > 0)) continue;
        const make = tag => document.createElementNS('http://www.w3.org/2000/svg', tag);
        const original = make('g'); original.dataset.diagramDetail = 'entity';
        for (const child of [...entity.children]) if (!['title','desc','defs'].includes(child.localName)) original.append(child);
        entity.append(original);
        const summary = make('g'); summary.dataset.diagramSummary = 'entity';
        const rect = make('rect');
        for (const key of ['x','y','width','height']) rect.setAttribute(key, box[key]);
        rect.setAttribute('rx', Math.min(12,box.height/5));
        summary.append(rect);
        const label = entity.dataset.diagramLabel || entity.getAttribute('aria-label') || 'Entity';
        const font = Math.min(42, Math.max(12, box.width / 12), box.height / 3);
        const capacity = Math.max(6,Math.floor((box.width-16)/(font*.58)));
        const words = label.split(/(?<=_)|\s+/), lines = [];
        let line = '';
        for (const word of words) {
          if (line && (line+word).length > capacity) { lines.push(line.trim()); line = ''; }
          line += word + (word.endsWith('_') ? '' : ' ');
        }
        if (line.trim()) lines.push(line.trim());
        const text = make('text'); text.setAttribute('text-anchor','middle'); text.setAttribute('font-size',font);
        lines.forEach((line,index) => {
          const span = make('tspan'); span.textContent = line;
          span.setAttribute('x',box.x+box.width/2);
          span.setAttribute('y',box.y+box.height/2+(index-(lines.length-1)/2)*font*1.2+font*.35);
          text.append(span);
        });
        summary.append(text); entity.append(summary);
        const expanded = board.dataset.whiteboardFields === 'true';
        original.style.display = expanded ? '' : 'none'; original.setAttribute('aria-hidden',String(!expanded));
        summary.style.display = expanded ? 'none' : ''; summary.setAttribute('aria-hidden',String(expanded));
      }
      }
      prepareEntitySummaries(undefined);
      const details = new Map(qa('[data-frame-detail]', svg).map(node => [node.dataset.frameDetail,node]));
      const summaries = new Map(qa('[data-frame-overview]', svg).map(node => [node.dataset.frameOverview,node]));
      // All geometry reads are initialization/resize work, never camera work.
      for (const frame of frames) {
        const border = q(':scope > rect', frame);
        border?.style.removeProperty('fill'); border?.style.removeProperty('stroke');
        const caption = summaries.get(frame.dataset.framePath);
        if (caption) caption.style.display = '';
      }
      const captionBoxes = [...summaries.values()].map(node => ({node, box:node.getBBox()}));
      for (const {node,box} of captionBoxes) node.setAttribute('transform','translate(0 ' + (-box.y-box.height-12) + ')');
      const worldBox = (box,t) => box && t ? {x:t.x+box.x*t.scale_x,y:t.y+box.y*t.scale_y,width:box.width*t.scale_x,height:box.height*t.scale_y} : null;
      const union = (a,b) => !a ? b : !b ? a : {x:Math.min(a.x,b.x),y:Math.min(a.y,b.y),width:Math.max(a.x+a.width,b.x+b.width)-Math.min(a.x,b.x),height:Math.max(a.y+a.height,b.y+b.height)-Math.min(a.y,b.y)};
      const frameStates = frames.map(frame => {
        const path = frame.dataset.framePath, meta = geometry.get(path), detail = details.get(path), summary = summaries.get(path);
        const matrix = detail?.transform.baseVal.consolidate()?.matrix;
        const content = detail && q(':scope > svg, :scope > image', detail);
        const w = Number(content?.getAttribute('width')), h = Number(content?.getAttribute('height'));
        const contentBox = matrix && meta && w > 0 && h > 0 ? worldBox({x:matrix.e,y:matrix.f,width:w*matrix.a,height:h*matrix.d},meta.transform) : meta?.bounds;
        let extent = union(meta?.bounds,worldBox(frame.getBBox(),meta?.transform));
        // Include outlying structured annotations and nested frames even when
        // their authored detail visibility is initially off.
        for (const [child,item] of geometry) if (child.startsWith(path+'/')) extent = union(extent,item.bounds);
        return {frame,path,detail,summary,extent,contentBox,scale:Math.abs((meta?.transform.scale_x || 1)*(matrix?.a || 1)),threshold:Number(frame.dataset.detailScale || .6)};
      });
      const byFrame = new Map(frameStates.map(state => [state.path,state]));
      // Cache native entity/section extents with the existing decoration visibility
      // states. Camera updates only compare boxes and update changed visibility.
      const decorations = qa('[data-diagram-kind="node"], [data-diagram-kind="group"], [data-visibility], [data-diagram-kind="annotation"], [data-diagram-kind="sticky"], [data-diagram-kind="text"], [data-diagram-kind="code"], [data-diagram-kind="graphic"]',svg).map(node => {
        const meta = geometry.get(node.dataset.instancePath);
        let extent = union(meta?.bounds,worldBox(node.getBBox(),meta?.transform));
        if (node.dataset.diagramKind === 'group') for (const [path,item] of geometry) if (path.startsWith(node.dataset.instancePath+'/')) extent = union(extent,item.bounds);
        return {node,owner:byFrame.get(node.parentElement?.closest('[data-frame-path]')?.dataset.framePath),visibility:node.dataset.visibility,bounds:extent};
      });
      const connections = qa('[data-instance-path][data-from][data-to]', svg).map(node => ({node,detail:q('[data-connection-detail]',node),overview:q('[data-connection-overview]',node),meta:geometry.get(node.dataset.instancePath),line:q('[data-connection-detail] > path',node)}));
      const revealNodes = qa('[data-reveal-step]',svg);
      const revealPaths = revealNodes.map(node => node.closest('[data-instance-path]')?.dataset.instancePath);
      const mode = q('[data-whiteboard-mode]',board), replay = q('[data-whiteboard-replay]',board), fitButton = q('[data-whiteboard-fit]',board);
      let controls = [], controlsDirty = true;
      const attribute = (node,name,value) => { if (node && node.getAttribute(name) !== String(value)) node.setAttribute(name,String(value)); };
      const text = (node,value) => { if (node.textContent !== value) node.textContent = value; };
      function visible(node, show) {
        const display = show ? '' : 'none';
        if (node.style.display !== display) {
          if (!show && node.contains(document.activeElement)) viewport.focus({preventScroll:true});
          node.style.display = display;
          controlsDirty = true;
        }
        attribute(node,'aria-hidden',!show);
      }
      function interact(active) {
        if (board.dataset.whiteboardInteracting === String(active)) return;
        attribute(board,'data-whiteboard-interacting',active);
        if (active) for (const node of revealNodes) node.getAnimations().forEach(animation => animation.cancel());
      }
      function paint() {
        // Keyboard pan, zoom buttons, Fit and resize move note owners too.
        // Clear their screen-space popovers before every camera mutation.
        interact(true);
        if (!render) render = requestAnimationFrame(() => {
          render = 0; renderScene();
          if (!motion && !drag?.moved && !interactionTimer) interact(false);
        });
      }
      function renderScene() {
        if (!(viewportRect.width > 0 && viewportRect.height > 0)) return;
        attribute(svg,'viewBox',[view.x,view.y,view.width,view.height].join(' '));
        const scale = Math.min(viewportRect.width/view.width,viewportRect.height/view.height);
        text(level,Math.round(scale*100)+'%');
        // A screen-space overscan restores content before it enters the view.
        const pad = 180/Math.max(scale,.001);
        const intersects = box => !box || box.x+box.width >= view.x-pad && box.x <= view.x+view.width+pad && box.y+box.height >= view.y-pad && box.y <= view.y+view.height+pad;
        const collapsed = [];
        for (const state of frameStates) {
          const {frame,path,detail,summary} = state;
          if (!detail) continue;
          const detailed = Boolean(focused && (focused === path || focused.startsWith(path+'/'))) || scale*state.scale >= state.threshold;
          const culled = !intersects(state.extent);
          if (state.culled !== culled) {
            state.culled = culled; controlsDirty = true;
            attribute(frame,'data-whiteboard-culled',culled); visible(frame,!culled);
            if (!culled) prepareEntitySummaries(path);
          }
          if (state.detailed !== detailed) {
            state.detailed = detailed; controlsDirty = true;
            attribute(frame,'data-whiteboard-detail',detailed);
            if (!detailed && detail.contains(document.activeElement)) viewport.focus({preventScroll:true});
            detail.style.pointerEvents = detailed ? '' : 'none';
            attribute(detail,'aria-hidden',!detailed);
            if (summary) visible(summary,!detailed);
          }
          if (!detailed) collapsed.push(path);
        }
        const visibleEndpoint = path => collapsed.filter(frame => path === frame || path.startsWith(frame + '/')).sort((a,b) => a.length-b.length)[0] || path;
        for (const state of connections) {
          const {node:connection,detail,overview,meta,line} = state;
          const from = visibleEndpoint(connection.dataset.from), to = visibleEndpoint(connection.dataset.to);
          if (!detail || !overview || state.from === from && state.to === to) continue;
          state.from = from; state.to = to;
          const a = geometry.get(from)?.bounds, b = geometry.get(to)?.bounds;
          if (meta && a && b && line) {
            const center = box => ({x:box.x+box.width/2,y:box.y+box.height/2});
            const boundary = (box, toward) => {
              const c = center(box), dx = toward.x-c.x, dy = toward.y-c.y;
              if (!dx && !dy) return c;
              const scale = Math.min(dx ? box.width/2/Math.abs(dx) : Infinity, dy ? box.height/2/Math.abs(dy) : Infinity);
              return {x:c.x+dx*scale,y:c.y+dy*scale};
            };
            const t = meta.transform;
            const world = point => ({x:t.x+point.x*t.scale_x,y:t.y+point.y*t.scale_y});
            const local = point => ({x:(point.x-t.x)/t.scale_x,y:(point.y-t.y)/t.scale_y});
            const bends = meta.bends || [];
            const points = [local(boundary(a,bends.length ? world(bends[0]) : center(b))), ...bends, local(boundary(b,bends.length ? world(bends[bends.length-1]) : center(a)))];
            let path = 'M' + points[0].x + ' ' + points[0].y;
            if (meta.curve === 'smooth' && points.length >= 3) {
              const at = index => points[Math.max(0,Math.min(points.length-1,index))];
              for (let i=0;i<points.length-1;i++) {
                const [p0,p1,p2,p3] = [at(i-1),at(i),at(i+1),at(i+2)];
                path += ' C' + (p1.x+(p2.x-p0.x)/6) + ' ' + (p1.y+(p2.y-p0.y)/6) + ' ' + (p2.x-(p3.x-p1.x)/6) + ' ' + (p2.y-(p3.y-p1.y)/6) + ' ' + p2.x + ' ' + p2.y;
              }
            } else points.slice(1).forEach(point => {path += ' L' + point.x + ' ' + point.y;});
            line.setAttribute('d',path);
            visible(detail,true); visible(overview,false);
          } else {
            const simplified = from !== connection.dataset.from || to !== connection.dataset.to;
            visible(detail,!simplified); visible(overview,simplified);
          }
          connection.dataset.whiteboardFrom = from; connection.dataset.whiteboardTo = to;
          connection.dataset.whiteboardConnectionMode = from === connection.dataset.from && to === connection.dataset.to ? 'detail' : 'overview';
        }
        const overview = !focused && (collapsed.length > 0 || (!frames.length && scale < .6));
        for (const state of decorations) {
          const localOverview = state.owner ? !state.owner.detailed : overview;
          const culled = Boolean(state.owner?.culled) || !intersects(state.bounds);
          const show = !culled && (!state.visibility || (state.visibility === 'overview' ? localOverview : !localOverview));
          if (state.shown !== show) { state.shown = show; visible(state.node,show); }
          attribute(state.node,'data-whiteboard-culled',culled);
        }
        if (controlsDirty) {
          for (const {node,owners} of controls) {
            const hidden = owners.some(owner => owner.culled || !owner.detailed) || Boolean(node.closest('[aria-hidden="true"]'));
            attribute(node,'tabindex',hidden ? -1 : 0);
          }
          controlsDirty = false;
        }
        const sourceRow = rowFor(focused);
        if (evidence.hidden === Boolean(sourceRow)) evidence.hidden = !sourceRow;
        if (sourceRow) {
          attribute(evidence,'data-documentation-target',sourceRow.dataset.sourceTarget);
          attribute(evidence,'data-documentation-revision',sourceRow.dataset.sourceRevision);
        } else if (evidence.hasAttribute('data-documentation-target')) {
          evidence.removeAttribute('data-documentation-target');
          evidence.removeAttribute('data-documentation-revision');
        }
        text(mode,focused ? 'Focus: '+(select.selectedOptions[0]?.textContent || focused) : overview ? 'Full board · focus a section for detail' : 'Detail');
        const hasReveal = revealPaths.some(path => !focused || path === focused || path?.startsWith(focused+'/'));
        if (replay.hidden === hasReveal) replay.hidden = !hasReveal;
        attribute(fitButton,'aria-pressed',fitted);
 board.dispatchEvent(new CustomEvent('whiteboard:paint', {detail:{view, rect:viewportRect}}));
      }
      // Fit inside the clear area between the floating title and controls,
      // while keeping the SVG and pan surface across the entire canvas.
      function fitBox(box) {
        const rect = viewportRect;
        const summary = q('.whiteboard-description > summary', board).getBoundingClientRect();
        const toolbar = q('[data-whiteboard-toolbar]', board).getBoundingClientRect();
        const top = summary.bottom - rect.top + 24, bottom = rect.bottom - toolbar.top + 20;
        const width = Math.max(1, rect.width - 40), height = Math.max(1, rect.height - top - bottom);
        const unit = Math.max(box.width / width, box.height / height);
        return {x:box.x-(width*unit-box.width)/2-20*unit, y:box.y-(height*unit-box.height)/2-top*unit, width:rect.width*unit, height:rect.height*unit};
      }
      function stopMotion() {
        const moving = Boolean(motion); cancelAnimationFrame(motion); motion = 0;
        attribute(board,'data-whiteboard-moving',false);
        if (moving) interact(false);
      }
      function travel(destination) {
        stopMotion();
        if (matchMedia('(prefers-reduced-motion: reduce)').matches) { view = destination; paint(); return; }
        const start = {...view}, started = performance.now();
        attribute(board,'data-whiteboard-moving',true); interact(true);
        const step = now => {
          const progress = Math.min(1, (now-started)/280), eased = 1-Math.pow(1-progress,3);
          for (const key of ['x','y','width','height']) view[key] = start[key]+(destination[key]-start[key])*eased;
          if (render) { cancelAnimationFrame(render); render = 0; }
          renderScene();
          if (progress < 1) motion = requestAnimationFrame(step);
          else stopMotion();
        };
        motion = requestAnimationFrame(step);
      }
      function fit() {
        stopMotion();
        atStart = false; fitted = true; view = fitBox(bounds); focused = ''; select.value = ''; paint();
        board.dispatchEvent(new CustomEvent('whiteboard:focus', {detail:{path:'', fitted:Boolean(openingJSON)}}));
      }
      function home() {
        stopMotion();
        atStart = true; fitted = !openingJSON; view = fitBox(opening); focused = ''; select.value = ''; paint();
        board.dispatchEvent(new CustomEvent('whiteboard:focus', {detail:{path:''}}));
      }
      function focus(path, fitAll = false) {
        if (!path && fitAll) return fit();
        const box = byFrame.get(path)?.contentBox || geometry.get(path)?.bounds;
        if (!box) return home();
        const {x,y,width,height} = box;
        if (!(width > 0 && height > 0)) return;
        atStart = false; focused = path; fitted = false; select.value = path;
        travel(fitBox({x: x - 24, y: y - 24, width: width + 48, height: height + 48}));
        board.dispatchEvent(new CustomEvent('whiteboard:focus', {detail:{path}}));
      }
      function zoom(factor, clientX, clientY) {
        stopMotion();
        fitted = false; atStart = false;
        const nextWidth = Math.max(Math.max(1, bounds.width / 512), Math.min(bounds.width * 2, view.width * factor));
        factor = nextWidth / view.width;
        const scale = Math.min(viewportRect.width/view.width,viewportRect.height/view.height);
        let anchor = {x: view.x + view.width / 2, y: view.y + view.height / 2};
        if (clientX !== undefined) anchor = {x:view.x+(clientX-viewportRect.left-(viewportRect.width-view.width*scale)/2)/scale,y:view.y+(clientY-viewportRect.top-(viewportRect.height-view.height*scale)/2)/scale};
        view = {x: anchor.x + (view.x - anchor.x) * factor, y: anchor.y + (view.y - anchor.y) * factor, width: nextWidth, height: view.height * factor};
        paint();
      }
      q('[data-whiteboard-fit]', board).addEventListener('click', fit);
      q('[data-whiteboard-replay]', board).addEventListener('click', () => replayDiagramReveal(board));
      q('[data-whiteboard-home]', board)?.addEventListener('click', home);
      q('[data-whiteboard-fields]', board).addEventListener('click', event => {
        const show = board.dataset.whiteboardFields !== 'true';
        board.dataset.whiteboardFields = String(show);
        event.currentTarget.setAttribute('aria-pressed', String(show));
        for (const row of qa('[data-field], [data-diagram-detail="field"]', svg)) {
          row.style.display = show ? 'block' : 'none';
          row.setAttribute('aria-hidden', String(!show));
        }
        qa('[data-diagram-detail="entity"]', svg).forEach(node => visible(node, show));
        qa('[data-diagram-summary="entity"]', svg).forEach(node => visible(node, !show));
      });
      qa('[data-whiteboard-zoom]', board).forEach(button => button.addEventListener('click', () => zoom(button.dataset.whiteboardZoom === '1' ? .8 : 1.25)));
      select.addEventListener('change', () => focus(select.value));
      qa('[data-whiteboard-focus-frame]', board).forEach(button => button.addEventListener('click', () => { focus(button.dataset.whiteboardFocusFrame); viewport.focus({preventScroll:true}); }));
      viewport.addEventListener('keydown', event => {
        if (event.ctrlKey || event.metaKey || event.altKey || event.target !== viewport) return;
        if (event.key === '+' || event.key === '=') zoom(.8);
        else if (event.key === '-' || event.key === '_') zoom(1.25);
        else if (event.key === '0' || event.key === 'Escape') fit();
        else if (event.key.startsWith('Arrow')) {
          stopMotion();
          fitted = false; atStart = false;
          const dx = event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : 0;
          const dy = event.key === 'ArrowDown' ? 1 : event.key === 'ArrowUp' ? -1 : 0;
          view.x += dx * view.width * .08; view.y += dy * view.height * .08; paint();
        } else return;
        event.preventDefault();
      });
      viewport.addEventListener('wheel', event => {
        event.preventDefault();
        // Trackpads report pixels, while some mouse wheels report lines/pages.
        const unitX = event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? viewportRect.width : 1;
        const unitY = event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? viewportRect.height : 1;
        let dx = event.deltaX * unitX, dy = event.deltaY * unitY;
        if (event.ctrlKey || event.metaKey) zoom(Math.exp(dy * .002), event.clientX, event.clientY);
        else {
          stopMotion(); fitted = false; atStart = false;
          if (event.shiftKey && !dx) { dx = dy; dy = 0; }
          const scale = Math.min(viewportRect.width/view.width,viewportRect.height/view.height);
          if (!(scale > 0)) return;
          view.x += dx/scale; view.y += dy/scale; paint();
        }
        clearTimeout(interactionTimer); interact(true);
        interactionTimer = setTimeout(() => { interactionTimer = 0; if (!motion && !drag?.moved) interact(false); },160);
      }, {passive:false});
      let drag = null, suppressClick = false;
      viewport.addEventListener('pointerdown', event => {
        if (event.button !== 0) return;
        stopMotion();
        drag = {x:event.clientX, y:event.clientY, start:{...view}, id:event.pointerId, moved:false};
      });
      viewport.addEventListener('pointermove', event => {
        if (!drag || event.pointerId !== drag.id) return;
        const dx = event.clientX - drag.x, dy = event.clientY - drag.y;
        if (!drag.moved && Math.hypot(dx,dy) < 4) return;
        fitted = false; atStart = false;
        if (!drag.moved) { drag.moved = true; viewport.setPointerCapture(event.pointerId); viewport.classList.add('whiteboard-dragging'); interact(true); }
        const scale = Math.min(viewportRect.width / view.width, viewportRect.height / view.height);
        view.x = drag.start.x - dx / scale; view.y = drag.start.y - dy / scale; paint();
      });
      const endDrag = () => {
        if (drag?.moved) { suppressClick = true; setTimeout(() => { suppressClick = false; }, 0); }
        drag = null; viewport.classList.remove('whiteboard-dragging');
        if (!motion && !interactionTimer) interact(false);
      };
      viewport.addEventListener('pointerup', endDrag);
      viewport.addEventListener('pointercancel', endDrag);
      viewport.addEventListener('click', event => {
        if (suppressClick) { event.preventDefault(); event.stopPropagation(); return; }
        const node = event.target.closest('[data-instance-path]');
        if (!node) return;
        if (node.hasAttribute('data-frame-path') || sections.has(node.dataset.instancePath)) {
          event.preventDefault(); event.stopPropagation(); focus(node.dataset.instancePath); return;
        }
        // A child's unbound mark must not inherit its enclosing ERD source.
        if (!node.hasAttribute('data-documentation-target') && !node.classList.contains('diagram-interaction')) { event.preventDefault(); event.stopPropagation(); }
      }, true);
      for (const node of qa('[data-instance-path]', svg)) {
        // Opaque SVG bindings can preserve provenance without a target region.
        // Such metadata must not become an invisible keyboard control.
        if (node.hasAttribute('data-binding')) {
          const bounds = measured.get(node);
          if (!bounds || !(bounds.width > 0 || bounds.height > 0)) continue;
        }
        if (!sections.has(node.dataset.instancePath) && !node.hasAttribute('data-frame-path') && !node.hasAttribute('data-documentation-target') && !node.hasAttribute('data-from') && !node.hasAttribute('data-about') && !q(':scope > desc', node)) continue;
        node.dataset.whiteboardSelectable = 'true';
        const control = node.hasAttribute('data-frame-path') ? qa('[data-frame-overview]', node).find(child => child.dataset.frameOverview === node.dataset.framePath) : sections.has(node.dataset.instancePath) ? q(':scope > path', node) : node;
        if (!control) continue;
        if (control !== node) node.setAttribute('role','group');
        control.setAttribute('tabindex','0'); control.setAttribute('role','button');
        const title = node.getAttribute('aria-label') || q(':scope > title', node)?.textContent || node.dataset.instancePath;
        control.setAttribute('aria-label', (node.hasAttribute('data-frame-path') || sections.has(node.dataset.instancePath) ? 'Focus ' : node.hasAttribute('data-documentation-target') ? 'Inspect ' : 'Inspect ') + title);
        control.addEventListener('keydown', event => {
          if (event.target !== control || control.closest('[aria-hidden="true"]') || (event.key !== 'Enter' && event.key !== ' ')) return;
          event.preventDefault(); event.stopPropagation(); node.dispatchEvent(new MouseEvent('click', {bubbles:true,cancelable:true}));
        });
      }
      controls = qa('[tabindex]',svg).map(node => {
        const owners = []; let owner = node.closest('[data-frame-detail]');
        while (owner) { const state = byFrame.get(owner.dataset.frameDetail); if (state) owners.push(state); owner = owner.parentElement.closest('[data-frame-detail]'); }
        return {node,owners};
      });
      const annotationBounds = new Map([...geometry].map(([path,meta]) => [path,meta.bounds || worldBox(meta.label_box,meta.transform)]));
      for (const [node,box] of measured) {
        if (annotationBounds.get(node.dataset.instancePath)) continue;
        const frame = byFrame.get(node.closest('[data-frame-path]')?.dataset.framePath);
        if (frame?.contentBox) annotationBounds.set(node.dataset.instancePath,{x:frame.contentBox.x+box.x*frame.scale,y:frame.contentBox.y+box.y*frame.scale,width:box.width*frame.scale,height:box.height*frame.scale});
      }
      const affordance = prepareDiagramAffordance(svg, {signal:pageScope.signal, position:node => {
        let box = annotationBounds.get(node.dataset.instancePath);
        if (!box && node.dataset.from && node.dataset.to) {
          const from = geometry.get(node.dataset.from)?.bounds, to = geometry.get(node.dataset.to)?.bounds;
          if (from && to) box = {x:(from.x+from.width/2+to.x+to.width/2)/2,y:(from.y+from.height/2+to.y+to.height/2)/2,width:0,height:0};
        }
        if (!box) return null;
        const scale = Math.min(viewportRect.width/view.width,viewportRect.height/view.height);
        const left = viewportRect.left+(viewportRect.width-view.width*scale)/2;
        const top = viewportRect.top+(viewportRect.height-view.height*scale)/2;
        return {right:left+(box.x+box.width-view.x)*scale,top:top+(box.y-view.y)*scale,
          viewport:{left:Math.max(0,viewportRect.left),top:Math.max(0,viewportRect.top),right:Math.min(innerWidth,viewportRect.right),bottom:Math.min(innerHeight,viewportRect.bottom)}};
      }});
      const templates = new Map(qa('[data-diagram-note-template]',board).map(node => [node.dataset.diagramNoteTemplate,node]));
      prepareDiagramInteractions(svg, {interactionRoot:board, signal:pageScope.signal, affordance, templates});
      const resize = new ResizeObserver(() => { viewportRect = viewport.getBoundingClientRect(); atStart ? home() : fitted ? fit() : paint(); });
      resize.observe(viewport);
      // Fitting reserves space for these overlays. Their labels can wrap
      // after a camera change even when the viewport itself has not resized.
      resize.observe(q('[data-whiteboard-toolbar]', board));
      resize.observe(q('.whiteboard-description > summary', board));
      pageScope.signal.addEventListener('abort', () => { resize.disconnect(); stopMotion(); cancelAnimationFrame(render); clearTimeout(interactionTimer); }, {once:true});
      q('[data-whiteboard-toolbar]', board).hidden = false;
      home();
      prepareWhiteboardNavigation(board, focus);
    }
  }
`
