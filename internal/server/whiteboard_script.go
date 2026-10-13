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
      let view = {...bounds}, focused = '', fitted = true, motion = 0;
      const frameRows = qa('[data-whiteboard-frame]', board);
      const geometry = new Map(JSON.parse(q('[data-whiteboard-metadata]', board).dataset.whiteboardMetadata).map(item => [item.path,item]));
      const frames = qa('[data-frame-path]', svg);
      const select = q('[data-whiteboard-focus]', board);
      const level = q('[data-whiteboard-level]', board);
      const inspection = q('[data-whiteboard-inspection]', board);
      const rowFor = path => frameRows.find(row => row.dataset.whiteboardFrame === path);
      // Captions supplement the embedded drawings; they never replace them.
      // Keep this a viewer presentation so historical persisted SVGs stay valid.
      for (const frame of frames) {
        const caption = qa('[data-frame-overview]', frame).find(node => node.dataset.frameOverview === frame.dataset.framePath);
        if (!caption) continue;
        caption.style.display = '';
        const box = caption.getBBox();
        caption.setAttribute('transform', 'translate(0 ' + (-box.y-box.height-12) + ')');
      }
      function visible(node, show) {
        node.style.display = show ? '' : 'none';
        node.setAttribute('aria-hidden', String(!show));
      }
      function paint() {
        svg.setAttribute('viewBox', [view.x, view.y, view.width, view.height].join(' '));
        const scale = Math.min(viewport.clientWidth / view.width, viewport.clientHeight / view.height);
        level.textContent = Math.round(scale * 100) + '%';
        const collapsed = [];
        for (const frame of frames) {
          const path = frame.dataset.framePath;
          const detail = qa('[data-frame-detail]', frame).find(node => node.dataset.frameDetail === path);
          const summary = qa('[data-frame-overview]', frame).find(node => node.dataset.frameOverview === path);
          if (!detail) continue;
          const matrix = detail.getScreenCTM();
          const effectiveScale = matrix ? Math.hypot(matrix.a, matrix.b) : scale;
          const detailed = (focused && (focused === path || focused.startsWith(path + '/'))) || effectiveScale >= Number(frame.dataset.detailScale || .6);
          frame.dataset.whiteboardDetail = String(detailed);
          detail.style.display = '';
          detail.style.pointerEvents = detailed ? '' : 'none';
          detail.setAttribute('aria-hidden', String(!detailed));
          if (summary) visible(summary, !detailed);
          if (!detailed) collapsed.push(path);
        }
        // A low-zoom diagram is one focus target. Its tiny internal marks stay
        // drawn, but become individual inspection targets only at detail scale.
        for (const control of qa('[tabindex]', svg)) {
          let owner = control.closest('[data-frame-detail]'), preview = false;
          while (owner) {
            if (owner.parentElement.dataset.whiteboardDetail === 'false') preview = true;
            owner = owner.parentElement.closest('[data-frame-detail]');
          }
          control.setAttribute('tabindex', preview ? '-1' : '0');
        }
        const visibleEndpoint = path => collapsed.filter(frame => path === frame || path.startsWith(frame + '/')).sort((a,b) => a.length-b.length)[0] || path;
        for (const connection of qa('[data-instance-path][data-from][data-to]', svg)) {
          const from = visibleEndpoint(connection.dataset.from), to = visibleEndpoint(connection.dataset.to);
          const detail = q('[data-connection-detail]', connection), overview = q('[data-connection-overview]', connection);
          if (!detail || !overview) continue;
          const meta = geometry.get(connection.dataset.instancePath), a = geometry.get(from)?.bounds, b = geometry.get(to)?.bounds;
          const line = q(':scope > path', detail);
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
        const overview = !focused && collapsed.length > 0;
        for (const node of qa('[data-visibility]', svg)) {
          const owner = node.parentElement?.closest('[data-frame-path]');
          const localOverview = owner ? owner.dataset.whiteboardDetail !== 'true' : overview;
          visible(node, node.dataset.visibility === 'overview' ? localOverview : !localOverview);
        }
        q('[data-whiteboard-mode]', board).textContent = focused ? 'Focus: ' + (select.selectedOptions[0]?.textContent || focused) : collapsed.length ? 'Overview · focus a diagram for detail' : 'Detail';
        q('[data-whiteboard-fit]', board).setAttribute('aria-pressed', String(fitted));
      }
      // Fit inside the clear area between the floating title and controls,
      // while keeping the SVG and pan surface across the entire canvas.
      function fitBox(box) {
        const rect = viewport.getBoundingClientRect();
        const summary = q('.whiteboard-description > summary', board).getBoundingClientRect();
        const toolbar = q('[data-whiteboard-toolbar]', board).getBoundingClientRect();
        const top = summary.bottom - rect.top + 24, bottom = rect.bottom - toolbar.top + 20;
        const width = Math.max(1, rect.width - 40), height = Math.max(1, rect.height - top - bottom);
        const unit = Math.max(box.width / width, box.height / height);
        return {x:box.x-(width*unit-box.width)/2-20*unit, y:box.y-(height*unit-box.height)/2-top*unit, width:rect.width*unit, height:rect.height*unit};
      }
      function stopMotion() {
        cancelAnimationFrame(motion); motion = 0;
        board.dataset.whiteboardMoving = 'false';
      }
      function travel(destination) {
        stopMotion();
        if (matchMedia('(prefers-reduced-motion: reduce)').matches) { view = destination; paint(); return; }
        const start = {...view}, started = performance.now();
        board.dataset.whiteboardMoving = 'true';
        const step = now => {
          const progress = Math.min(1, (now-started)/280), eased = 1-Math.pow(1-progress,3);
          for (const key of ['x','y','width','height']) view[key] = start[key]+(destination[key]-start[key])*eased;
          paint();
          if (progress < 1) motion = requestAnimationFrame(step);
          else stopMotion();
        };
        motion = requestAnimationFrame(step);
      }
      function fit() {
        stopMotion();
        fitted = true; view = fitBox(bounds); focused = ''; select.value = ''; inspection.hidden = true; paint();
        board.dispatchEvent(new CustomEvent('whiteboard:focus', {detail:{path:''}}));
      }
      function focus(path) {
        const row = rowFor(path);
        if (!row) return fit();
        let x = Number(row.dataset.x), y = Number(row.dataset.y), width = Number(row.dataset.width), height = Number(row.dataset.height);
        const frame = frames.find(node => node.dataset.framePath === path);
        const detail = frame && qa('[data-frame-detail]', frame).find(node => node.dataset.frameDetail === path);
        const content = detail && q(':scope > svg, :scope > image', detail);
        if (content) {
          const transform = detail.transform.baseVal.consolidate()?.matrix;
          const world = geometry.get(path)?.transform;
          const w = Number(content.getAttribute('width')), h = Number(content.getAttribute('height'));
          if (transform && world && w > 0 && h > 0) {
            x = world.x + transform.e * world.scale_x; y = world.y + transform.f * world.scale_y;
            width = w * transform.a * world.scale_x; height = h * transform.d * world.scale_y;
          }
        }
        if (!(width > 0 && height > 0)) return;
        focused = path; fitted = false; select.value = path;
        travel(fitBox({x: x - 24, y: y - 24, width: width + 48, height: height + 48}));
        board.dispatchEvent(new CustomEvent('whiteboard:focus', {detail:{path}}));
      }
      function zoom(factor, clientX, clientY) {
        stopMotion();
        fitted = false;
        const nextWidth = Math.max(Math.max(1, bounds.width / 512), Math.min(bounds.width * 2, view.width * factor));
        factor = nextWidth / view.width;
        const matrix = svg.getScreenCTM();
        let anchor = {x: view.x + view.width / 2, y: view.y + view.height / 2};
        if (matrix && clientX !== undefined) anchor = new DOMPoint(clientX, clientY).matrixTransform(matrix.inverse());
        view = {x: anchor.x + (view.x - anchor.x) * factor, y: anchor.y + (view.y - anchor.y) * factor, width: nextWidth, height: view.height * factor};
        paint();
      }
      q('[data-whiteboard-fit]', board).addEventListener('click', fit);
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
          fitted = false;
          const dx = event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : 0;
          const dy = event.key === 'ArrowDown' ? 1 : event.key === 'ArrowUp' ? -1 : 0;
          view.x += dx * view.width * .08; view.y += dy * view.height * .08; paint();
        } else return;
        event.preventDefault();
      });
      viewport.addEventListener('wheel', event => {
        if (!event.ctrlKey && !event.metaKey) return;
        event.preventDefault(); zoom(Math.exp(event.deltaY * .002), event.clientX, event.clientY);
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
        fitted = false; drag.moved = true; viewport.setPointerCapture(event.pointerId); viewport.classList.add('whiteboard-dragging');
        const scale = Math.min(viewport.clientWidth / view.width, viewport.clientHeight / view.height);
        view.x = drag.start.x - dx / scale; view.y = drag.start.y - dy / scale; paint();
      });
      const endDrag = () => {
        if (drag?.moved) { suppressClick = true; setTimeout(() => { suppressClick = false; }, 0); }
        drag = null; viewport.classList.remove('whiteboard-dragging');
      };
      viewport.addEventListener('pointerup', endDrag);
      viewport.addEventListener('pointercancel', endDrag);
      viewport.addEventListener('click', event => {
        if (suppressClick) { event.preventDefault(); event.stopPropagation(); return; }
        const node = event.target.closest('[data-instance-path]');
        if (!node) return;
        if (node.hasAttribute('data-frame-path')) {
          event.preventDefault(); event.stopPropagation(); focus(node.dataset.framePath); return;
        }
        const connection = node.closest('[data-from][data-to]');
        if (connection) {
          const title = q('title', connection)?.textContent || connection.getAttribute('aria-label') || 'Connection';
          inspection.textContent = title + ' · ' + connection.dataset.from + ' → ' + connection.dataset.to + '. This explanatory connection does not create a recorded relationship.';
          inspection.hidden = false;
        }
        if (!connection && node.hasAttribute('data-about')) {
          inspection.textContent = (q('title', node)?.textContent || node.getAttribute('aria-label') || '') + ' · About ' + node.dataset.about;
          inspection.hidden = false;
        }
        // A child's unbound mark must not inherit its enclosing ERD source.
        if (!node.hasAttribute('data-documentation-target')) { event.preventDefault(); event.stopPropagation(); }
      }, true);
      for (const node of qa('[data-instance-path]', svg)) {
        if (!node.hasAttribute('data-frame-path') && !node.hasAttribute('data-documentation-target') && !node.hasAttribute('data-from') && !node.hasAttribute('data-about')) continue;
        node.dataset.whiteboardSelectable = 'true';
        const control = node.hasAttribute('data-frame-path') ? qa('[data-frame-overview]', node).find(child => child.dataset.frameOverview === node.dataset.framePath) : node;
        if (!control) continue;
        if (control !== node) node.setAttribute('role','group');
        control.setAttribute('tabindex','0'); control.setAttribute('role','button');
        const title = q('title', node)?.textContent || node.getAttribute('aria-label') || node.dataset.instancePath;
        control.setAttribute('aria-label', (node.hasAttribute('data-frame-path') ? 'Focus ' : node.hasAttribute('data-documentation-target') ? 'Inspect ' : 'Inspect connection ') + title);
        control.addEventListener('keydown', event => {
          if (event.target !== control || (event.key !== 'Enter' && event.key !== ' ')) return;
          event.preventDefault(); event.stopPropagation(); node.dispatchEvent(new MouseEvent('click', {bubbles:true,cancelable:true}));
        });
      }
      const resize = new ResizeObserver(() => fitted ? fit() : paint()); resize.observe(viewport);
      pageScope.signal.addEventListener('abort', () => { resize.disconnect(); stopMotion(); }, {once:true});
      q('[data-whiteboard-toolbar]', board).hidden = false;
      fit();
    }
  }
`
