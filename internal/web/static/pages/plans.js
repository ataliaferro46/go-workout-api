// pages/plans.js — list the current user's persisted plans, expand on click,
// allow delete.

(function () {
  'use strict';

  document.addEventListener('DOMContentLoaded', async () => {
    await AUTH.ensureLoggedIn();
    const listEl = document.getElementById('plans-list');
    const emptyEl = document.getElementById('empty');
    const errorEl = document.getElementById('error');
    const userPill = document.getElementById('user-pill');

    const u = AUTH.currentUser();
    if (u && userPill) userPill.textContent = u.email;

    await load();

    async function load() {
      try {
        const data = await API.get('/v1/plans');
        const plans = (data && data.plans) || [];
        if (plans.length === 0) {
          listEl.innerHTML = '';
          emptyEl.style.display = 'block';
          return;
        }
        emptyEl.style.display = 'none';
        listEl.innerHTML = plans.map(renderRow).join('');
        listEl.querySelectorAll('.plan-row-head').forEach(head => {
          head.addEventListener('click', () => {
            head.closest('.plan-row').classList.toggle('expanded');
          });
        });
        // Reorder day controls (up / down) and Start Workout.
        listEl.querySelectorAll('[data-day-action]').forEach(btn => {
          btn.addEventListener('click', async (e) => {
            e.stopPropagation();
            const action = btn.dataset.dayAction;
            const dayCard = btn.closest('.plan-day-card');
            const planRow = btn.closest('.plan-row');
            const planID = planRow.dataset.id;
            const dayIdx = parseInt(dayCard.dataset.day, 10);
            if (action === 'start') {
              await startWorkout(planID, dayIdx, dayCard);
              return;
            }
            await reorderDay(planID, dayIdx, action);
          });
        });

        listEl.querySelectorAll('[data-action="swap"]').forEach(btn => {
          btn.addEventListener('click', async (e) => {
            e.stopPropagation();
            const exRow = btn.closest('.pd-ex');
            const planRow = btn.closest('.plan-row');
            const planID = planRow.dataset.id;
            const dayIdx = parseInt(exRow.dataset.day, 10);
            const orderIdx = parseInt(exRow.dataset.order, 10);
            const altsBox = exRow.querySelector('.pd-ex-alternatives');
            // Toggle: collapse if already open
            if (!altsBox.classList.contains('hidden') && altsBox.dataset.loaded === 'yes') {
              altsBox.classList.add('hidden');
              return;
            }
            altsBox.innerHTML = '<div class="alt-loading">Finding alternatives…</div>';
            altsBox.classList.remove('hidden');
            try {
              const data = await API.get('/v1/plans/' + encodeURIComponent(planID) +
                '/days/' + dayIdx + '/exercises/' + orderIdx + '/alternatives');
              const alts = (data && data.alternatives) || [];
              if (alts.length === 0) {
                altsBox.innerHTML = '<div class="alt-empty">No alternatives in your equipment + injury constraints.</div>';
                altsBox.dataset.loaded = 'yes';
                return;
              }
              altsBox.innerHTML =
                '<div class="alt-label">Swap to:</div>' +
                alts.map(a => {
                  const tags = [];
                  if (a.compound) tags.push('compound');
                  if (a.region) tags.push(a.region.replace(/_/g, ' '));
                  const tagStr = tags.length ? ' <span class="alt-tag">' + tags.join(' · ') + '</span>' : '';
                  return '<button class="alt-option" data-ex-id="' + UI.escape(a.id) + '">' +
                    UI.escape(a.name) + tagStr +
                  '</button>';
                }).join('');
              altsBox.dataset.loaded = 'yes';
              altsBox.querySelectorAll('.alt-option').forEach(opt => {
                opt.addEventListener('click', async (e2) => {
                  e2.stopPropagation();
                  const exID = opt.dataset.exId;
                  opt.textContent = 'Swapping…';
                  opt.disabled = true;
                  try {
                    await API.fetch('/v1/plans/' + encodeURIComponent(planID) +
                      '/days/' + dayIdx + '/exercises/' + orderIdx, {
                      method: 'PATCH',
                      body: JSON.stringify({ exercise_id: exID }),
                    });
                    await load();
                  } catch (err2) {
                    UI.showError(errorEl, err2);
                  }
                });
              });
            } catch (err) {
              altsBox.innerHTML = '<div class="alt-empty">Could not load alternatives: ' + UI.escape(err.message || 'error') + '</div>';
              altsBox.dataset.loaded = 'yes';
            }
          });
        });
        listEl.querySelectorAll('.delete-plan').forEach(btn => {
          btn.addEventListener('click', async (e) => {
            e.stopPropagation();
            const id = btn.dataset.id;
            if (!confirm('Delete this plan?')) return;
            try {
              await API.delete('/v1/plans/' + encodeURIComponent(id));
              await load();
            } catch (err) {
              UI.showError(errorEl, err);
            }
          });
        });
      } catch (err) {
        UI.showError(errorEl, err);
      }
    }
  });

  async function reorderDay(planID, dayIdx, direction) {
    try {
      // Get current plan to know day order.
      const p = await API.get('/v1/plans/' + encodeURIComponent(planID));
      const order = (p.days || []).map(d => d.index);
      const pos = order.indexOf(dayIdx);
      if (pos < 0) return;
      const swapWith = direction === 'up' ? pos - 1 : pos + 1;
      if (swapWith < 0 || swapWith >= order.length) return;
      [order[pos], order[swapWith]] = [order[swapWith], order[pos]];
      await API.fetch('/v1/plans/' + encodeURIComponent(planID) + '/days/reorder', {
        method: 'PATCH', body: JSON.stringify({ order: order }),
      });
      // Refresh list.
      const reload = document.getElementById('plans-list');
      reload.dispatchEvent(new Event('reload'));
      window.location.reload();
    } catch (err) {
      UI.showError(document.getElementById('error'), err);
    }
  }

  async function startWorkout(planID, dayIdx, dayCard) {
    try {
      const p = await API.get('/v1/plans/' + encodeURIComponent(planID));
      const day = (p.days || []).find(d => d.index === dayIdx);
      if (!day) throw new Error('day not found');
      const exercises = (day.exercises || []).map(ex => ({
        name: (ex.exercise && ex.exercise.name) || 'exercise',
        sets: ex.sets || 0,
        reps: Math.round(((ex.reps_low || 0) + (ex.reps_high || 0)) / 2),
        weight_kg: 0,
      }));
      const name = day.name + (day.weekday ? ' (' + day.weekday + ')' : '');
      const w = await API.post('/v1/workouts', {
        name: name,
        notes: '',
        plan_id: planID,
        plan_day_idx: dayIdx,
        exercises: exercises,
      });
      window.location.href = '/workout/' + encodeURIComponent(w.id);
    } catch (err) {
      UI.showError(document.getElementById('error'), err);
    }
  }

  function renderSetTypeBadgeSmall(ex) {
    if (!ex.set_type || ex.set_type === 'standard') return '';
    const labels = {
      amrap: 'AMRAP last set', drop_set: 'Drop set finish',
      twenty_ones: '21s', superset: 'Superset',
    };
    const cls = {
      amrap: 'st-amrap', drop_set: 'st-drop',
      twenty_ones: 'st-21s', superset: 'st-superset',
    };
    if (!labels[ex.set_type]) return '';
    const note = ex.set_type_note ? '<span class="pd-st-note">' + UI.escape(ex.set_type_note) + '</span>' : '';
    return '<div class="pd-set-type ' + cls[ex.set_type] + '">' +
      '<span class="pd-st-label">' + labels[ex.set_type] + '</span>' + note +
    '</div>';
  }

  function renderRow(p) {
    const days = (p.days || []).map(day => {
      const exercises = (day.exercises || []).map(ex => {
        const e = ex.exercise || {};
        const ytURL = 'https://www.youtube.com/results?search_query=' +
          encodeURIComponent(e.name + ' exercise form');
        const warmupTxt = (ex.warmups || []).map(w =>
          w.reps + '×' + Math.round(w.percent_of_working * 100) + '%'
        ).join(' · ');
        const warmupLine = warmupTxt
          ? '<div class="pd-ex-warmup">warmup: ' + warmupTxt + '</div>'
          : '';
        return '<div class="pd-ex" data-day="' + day.index + '" data-order="' + ex.order + '">' +
          '<div class="pd-ex-row">' +
            '<div class="pd-ex-name">' + UI.escape(e.name) + '</div>' +
            '<a class="demo-link" href="' + ytURL + '" target="_blank" rel="noopener" title="Watch demo on YouTube">' +
              '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
                '<polygon points="5 3 19 12 5 21 5 3"></polygon>' +
              '</svg>' +
              '<span>demo</span>' +
            '</a>' +
            '<button class="swap-link" data-action="swap" title="Swap for an alternative exercise">' +
              '<svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
                '<polyline points="17 1 21 5 17 9"></polyline>' +
                '<path d="M3 11V9a4 4 0 0 1 4-4h14"></path>' +
                '<polyline points="7 23 3 19 7 15"></polyline>' +
                '<path d="M21 13v2a4 4 0 0 1-4 4H3"></path>' +
              '</svg>' +
              '<span>swap</span>' +
            '</button>' +
          '</div>' +
          warmupLine +
          '<div class="pd-ex-dose">' +
            ex.sets + '×' + ex.reps_low + '–' + ex.reps_high + ' · ' + ex.rest_seconds + 's rest' +
          '</div>' +
          renderSetTypeBadgeSmall(ex) +
          '<div class="pd-ex-alternatives hidden"></div>' +
        '</div>';
      }).join('');
      const weekdayBadge = day.weekday
        ? '<span class="pd-weekday">' + UI.escape(day.weekday.toUpperCase()) + '</span>'
        : '';
      return '<div class="plan-day-card" data-day="' + day.index + '">' +
        '<div class="pd-name-row">' +
          '<div class="pd-name">' + UI.escape(day.name) + '</div>' +
          '<div class="pd-day-controls">' +
            weekdayBadge +
            '<button class="pd-day-btn" data-day-action="up"   title="Move day earlier">▲</button>' +
            '<button class="pd-day-btn" data-day-action="down" title="Move day later">▼</button>' +
            '<button class="pd-day-btn pd-start-btn" data-day-action="start" title="Start this workout">▶ Start</button>' +
          '</div>' +
        '</div>' +
        exercises +
      '</div>';
    }).join('');

    return '<div class="plan-row" data-id="' + UI.escape(p.id) + '">' +
      '<div class="plan-row-head">' +
        '<div class="plan-row-left">' +
          '<div class="plan-title">' + UI.escape(UI.formatTitleCase(p.goal)) + ' · ' + UI.escape(p.split) + '</div>' +
          '<div class="plan-sub">' +
            '<span class="meta-pill">' + p.days_per_week + ' days</span>' +
            '<span class="meta-pill">' + UI.escape(UI.formatTitleCase(p.experience)) + '</span>' +
            '<span class="when">' + UI.escape(UI.formatDate(p.created_at)) + '</span>' +
          '</div>' +
        '</div>' +
        '<div class="plan-row-right">' +
          '<span class="chevron">›</span>' +
        '</div>' +
      '</div>' +
      '<div class="plan-body">' +
        '<div class="plan-days">' + days + '</div>' +
        '<div class="plan-actions">' +
          '<button class="btn btn-danger delete-plan" data-id="' + UI.escape(p.id) + '">Delete</button>' +
        '</div>' +
      '</div>' +
    '</div>';
  }
})();
