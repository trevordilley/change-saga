package server

// Included in the existing local script's closure so drawer and annotation
// integrations share one mutation path without a frontend dependency. It is
// made for each review page that arrives; its listener lives as long as the
// page's scope does.
const reviewAsyncJavaScript = `
  let reviewAsync = null;
  function makeReviewAsync(signal) {
    const deck = q('#page [data-review-deck]');
    if (!deck) return null;
    let pending = false, uncertain = false, revision = 0;
    const status = document.createElement('div');
    status.className = 'review-annotation-status'; status.hidden = true;
    status.setAttribute('role', 'status'); deck.append(status);
    function announce(message, target, check = false) {
      const host=q('.review-annotation-compose:not([hidden])') || q('form[data-saving]');
      if(host) { host.prepend(status); status.style.position='static'; status.style.maxWidth='none'; status.style.margin='8px 0'; }
      else { deck.append(status); status.removeAttribute('style'); }
      status.replaceChildren(document.createTextNode(message)); status.hidden = false;
      if (check && target) {
        const button = document.createElement('button'); button.type = 'button'; button.textContent = 'Check saved feedback';
        button.addEventListener('click', async () => {
          button.disabled = true;
          const requestedRevision=revision;
          try {
            const response = await fetch('/reviews/' + encodeURIComponent(deck.dataset.review) + '/feedback?target=' + encodeURIComponent(target), {headers:{Accept:'application/json'},cache:'no-store'});
            if (!response.ok) throw new Error('Feedback could not be checked.');
            const feedback=await response.json();
            if(requestedRevision!==revision || pending) { button.disabled=false; return; }
            apply(feedback);
            deck.dispatchEvent(new CustomEvent('review-feedback-checked'));
            announce(uncertain ? 'Showing saved feedback. The earlier save may still finish; inspect it and reload before submitting again. Your draft is retained.' : 'Showing saved feedback.', target, uncertain);
          } catch (_) { announce('Feedback could not be checked. Your draft is retained.', target, true); }
        }); status.append(' ',button);
      }
    }
    function slideFor(target) {
      return qa('.review-deck-slide').find(slide => target === slide.dataset.slideTarget || target?.startsWith(slide.dataset.slideTarget + ':item:'));
    }
    function apply(feedback) {
      if (!feedback) return;
      const slide = slideFor(feedback.target);
      if (!slide) return;
      const fresh = document.createElement('template'); fresh.innerHTML = feedback.menu;
      const list = q('.review-decision-list',slide), next = fresh.content.querySelector('.review-decision-list');
      if (list && next) list.replaceChildren(...next.childNodes);
      const range = q('.review-range',slide), newRange = fresh.content.querySelector('.review-range');
      if (range && newRange) range.replaceChildren(...newRange.childNodes);
      const decisions=feedback.report.decisions || [];
      const outOfDate = decisions.some(d => d.currency === 'out_of_date');
      slide.classList.toggle('out-of-date',outOfDate);
      const thumbnail = qa('[data-slide-thumbnail]').find(button => button.dataset.slideTarget === feedback.target)?.closest('[data-slide-thumbnail-card]');
      const badge = thumbnail && q('.slide-thumbnail-status',thumbnail);
      if (badge) {
        const names = {approved:'Approved',changes_requested:'Changes requested',none:'Not reviewed'};
        const states = decisions.map(d => names[d.state] + (d.currency === 'out_of_date' ? '; out of date' : d.currency === 'unknown' ? '; currency unknown' : ''));
        const label = (states.join('; ') || 'Not reviewed') + (feedback.report.open_threads ? '; ' + feedback.report.open_threads + ' open threads' : '');
        badge.dataset.reviewState = decisions.map(d=>d.state).join('') || 'none';
        badge.setAttribute('aria-label',label); badge.title = label;
      }
      const roots = [document,...qa('template[id^="review-item-"]').map(t=>t.content)];
      for (const [target,html] of Object.entries(feedback.threads)) {
        for (const root of roots) for (const container of qa('[data-review-threads-for]',root).filter(n=>n.dataset.reviewThreadsFor===target)) {
          const template = document.createElement('template'); template.innerHTML = html;
          for (const thread of [...template.content.children]) {
            const existing = [...container.children].find(n=>n.dataset.reviewThread===thread.dataset.reviewThread);
            if (!existing) { container.append(thread); continue; }
            existing.className=thread.className; existing.dataset.threadState=thread.dataset.threadState;
            const state = q('.review-thread-head span',existing); if(state) state.textContent=thread.dataset.threadState;
            for (const comment of qa(':scope > .review-comment',thread)) {
              if (![...existing.children].some(n=>n.id===comment.id)) existing.insertBefore(comment,q(':scope > .review-reply',existing));
            }
          }
        }
      }
      if (feedback.frozen || feedback.snapshot !== slide.dataset.reviewSnapshot) {
        slide.dataset.reviewStale = 'true';
        qa('form button,[data-review-annotation-toggle]',slide).forEach(button=>button.disabled=true);
        if (feedback.frozen) {
          deck.dataset.reviewFrozen='true';
          qa('form[action*="/reviews/"] button,.review-annotation-toolbox button,.review-annotation-toolbox input,[data-review-annotation-toggle]').forEach(control=>control.disabled=true);
        }
        announce(feedback.frozen ? 'This review has landed and is now read-only.' : 'The source or slide changed. Reload and inspect the current slide before saving.',feedback.target);
      }
    }
    async function post(path, fields, target) {
      if (pending) return {saved:false, message:'A review save is already in progress.'};
      if (uncertain) return {saved:false, ambiguous:true, message:'The earlier save may have completed. Check saved feedback before submitting again.'};
      const slide = slideFor(target);
      if (!slide || slide.dataset.reviewStale || deck.dataset.reviewFrozen) return {saved:false,message:'Reload and inspect the current slide before saving.'};
      pending = true; revision++;
      try {
        const body = new URLSearchParams(fields); body.set('token',deck.dataset.reviewToken); body.set('snapshot',slide.dataset.reviewSnapshot || '');
        const response = await fetch(path,{method:'POST',body,redirect:'error',headers:{Accept:'application/json','Content-Type':'application/x-www-form-urlencoded'}});
        if (!response.ok) {
          if (response.status >= 500) throw new Error('Save outcome unknown');
          const message = (await response.text()).trim() || 'The save was refused. Your draft is retained.';
          announce(message,target); return {saved:false,message};
        }
        const result = await response.json();
        if (result.saved !== true || !result.event_id) throw new Error('Missing save receipt');
        try { apply(result.feedback); }
        catch (_) { result.warning='Its display could not be refreshed; check saved feedback.'; }
        if (result.warning) announce('Saved. ' + result.warning,target,true);
        else if (!slide.dataset.reviewStale) status.hidden=true;
        return result;
      } catch (_) {
        uncertain = true;
        const message = 'The save may have completed, but its confirmation was lost. Your draft is retained. Check saved feedback before submitting again.';
        announce(message,target,true); return {saved:false,ambiguous:true,message};
      } finally { pending=false; }
    }
    document.addEventListener('submit', async event => {
      const form = event.target;
      if (!(form instanceof HTMLFormElement) || !form.matches('.review-decision-quick,.review-decision-form,.review-comment-form')) return;
      if (!new URL(form.action).pathname.startsWith('/reviews/')) return;
      event.preventDefault();
      if (form.dataset.saving || form.dataset.uncertain) return;
      const fields = new URLSearchParams(new FormData(form));
      if (event.submitter?.name) fields.set(event.submitter.name,event.submitter.value);
      const target = form.closest('.review-deck-slide')?.dataset.slideTarget || form.closest('[data-review-target]')?.dataset.reviewTarget || fields.get('target');
      const focusAtSubmit=document.activeElement;
      form.dataset.saving='true'; form.setAttribute('aria-busy','true');
      const buttons=qa('button',form), fieldsToLock=qa('textarea',form);
      [...buttons,...fieldsToLock].forEach(b=>b.disabled=true);
      const result = await post(form.action,fields,target);
      delete form.dataset.saving; form.removeAttribute('aria-busy'); fieldsToLock.forEach(field=>field.disabled=false);
      if (result.saved) {
        form.reset();
        const details=form.closest('details'); if(details && !result.warning) details.open=false;

      } else if (result.ambiguous) form.dataset.uncertain='true';
      if (!form.dataset.uncertain && !slideFor(target)?.dataset.reviewStale) buttons.forEach(b=>b.disabled=false);
      if(result.saved && form.contains(focusAtSubmit) && slideFor(target)?.classList.contains('active') && (document.activeElement===focusAtSubmit || document.activeElement===document.body)) {
        const details=form.closest('details'); (details && q('summary',details) || event.submitter)?.focus({preventScroll:true});
      }
    }, {signal});
    return {post,apply,slideFor,announce};
  }
`
