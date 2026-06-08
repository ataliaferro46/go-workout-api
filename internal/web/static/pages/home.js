// pages/home.js — plan generator form behavior.

(function () {
  'use strict';

  document.addEventListener('DOMContentLoaded', () => {
    const form = document.getElementById('plan-form');
    if (!form) return;

    const submitBtn = document.getElementById('submit-btn');
    const btnText = submitBtn.querySelector('.btn-text');
    const arrow = submitBtn.querySelector('.arrow');
    const result = document.getElementById('result');
    const errorBox = document.getElementById('error');
    const daysSlider = document.getElementById('days');
    const daysVal = document.getElementById('days-val');
    const daysHint = document.getElementById('days-hint');

    // Live slider readout.
    daysSlider.addEventListener('input', () => {
      daysVal.textContent = daysSlider.value;
      daysHint.textContent = daysSlider.value + ' days';
    });

    // Single-select recovery buttons.
    let recovery = null;
    document.querySelectorAll('.recovery-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        document.querySelectorAll('.recovery-btn').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        const v = btn.dataset.recovery;
        recovery = v === '' ? null : parseFloat(v);
      });
    });

    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      UI.hideError(errorBox);

      const fd = new FormData(form);
      const equipment = fd.getAll('equipment');
      if (equipment.length === 0) {
        UI.showError(errorBox, {
          code: 'validation_failed',
          message: 'Pick at least one piece of equipment',
        });
        return;
      }
      const injuries = fd.getAll('injuries');

      const body = {
        goal: fd.get('goal'),
        experience: fd.get('experience'),
        days_per_week: parseInt(fd.get('days_per_week'), 10),
        available_equipment: equipment,
        injuries: injuries.length ? injuries : undefined,
      };
      if (recovery !== null) body.recovery_hint = recovery;

      // Loading state.
      submitBtn.disabled = true;
      btnText.textContent = 'Generating';
      arrow.outerHTML = '<span class="spinner"></span>';

      try {
        const plan = await API.post('/v1/plans/generate?seed=' + Date.now(), body);
        renderPlan(result, plan);
      } catch (err) {
        UI.showError(errorBox, err);
      } finally {
        submitBtn.disabled = false;
        btnText.textContent = 'Generate Plan';
        const spin = submitBtn.querySelector('.spinner');
        if (spin) spin.outerHTML = '<span class="arrow">→</span>';
      }
    });
  });

  function renderPlan(el, plan) {
    const warnings = (plan.warnings || []).map(w =>
      '<li>' + UI.escape(w) + '</li>'
    ).join('');

    const days = (plan.days || []).map(day => {
      const exercises = (day.exercises || []).map(ex => {
        const e = ex.exercise || {};
        const compound = e.compound ? ' <span class="compound">compound</span>' : '';
        return '<div class="exercise">' +
          '<div class="exercise-name">' + UI.escape(e.name) + '</div>' +
          '<div class="exercise-dose">' +
            '<span>' + ex.sets + ' × ' + ex.reps_low + '–' + ex.reps_high + '</span>' +
            '<span>rest ' + ex.rest_seconds + 's</span>' +
            compound +
          '</div>' +
        '</div>';
      }).join('');
      return '<div class="day-card">' +
        '<div class="day-header">' +
          '<div class="day-name">' + UI.escape(day.name) + '</div>' +
          '<div class="day-num">day ' + day.index + '</div>' +
        '</div>' +
        exercises +
      '</div>';
    }).join('');

    el.innerHTML =
      '<div class="result-header">' +
        '<div class="result-title">Your week</div>' +
        '<div class="result-meta">' +
          '<span class="meta-pill">' + UI.escape(plan.split) + '</span>' +
          '<span class="meta-pill">' + plan.days_per_week + ' days</span>' +
          '<span class="meta-pill">' + UI.escape(UI.formatTitleCase(plan.goal)) + '</span>' +
          '<span class="meta-pill">' + UI.escape(UI.formatTitleCase(plan.experience)) + '</span>' +
        '</div>' +
      '</div>' +
      (warnings ? '<div class="callout callout-warning"><strong>Notes</strong><ul>' + warnings + '</ul></div>' : '') +
      '<div class="days-grid">' + days + '</div>';
    el.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }
})();
