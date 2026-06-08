// pages/plans.js — list the current user's persisted plans, expand on click,
// allow delete.

(function () {
  'use strict';

  document.addEventListener('DOMContentLoaded', async () => {
    const listEl = document.getElementById('plans-list');
    const emptyEl = document.getElementById('empty');
    const errorEl = document.getElementById('error');
    const userPill = document.getElementById('user-pill');

    userPill.textContent = 'user: ' + API.getUserID();

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

  function renderRow(p) {
    const days = (p.days || []).map(day => {
      const exercises = (day.exercises || []).map(ex => {
        const e = ex.exercise || {};
        return '<div class="pd-ex">' +
          '<div class="pd-ex-name">' + UI.escape(e.name) + '</div>' +
          '<div class="pd-ex-dose">' +
            ex.sets + '×' + ex.reps_low + '–' + ex.reps_high + ' · ' + ex.rest_seconds + 's rest' +
          '</div>' +
        '</div>';
      }).join('');
      return '<div class="plan-day-card">' +
        '<div class="pd-name">' + UI.escape(day.name) + '</div>' +
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
