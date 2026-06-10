// pages/home.js — plan generator form behavior.

(function () {
  'use strict';

  document.addEventListener('DOMContentLoaded', async () => {
    const user = await AUTH.ensureLoggedIn();
    // Tiny easter egg — show a quote card on the home page when a
    // particular friend signs in. Matched loosely by email so capitalization
    // and aliases don't break it.
    if (user && user.email && /burgess|will\.b|wburgess/i.test(user.email)) {
      injectWillBurgessQuote();
    }
    const form = document.getElementById('plan-form');
    if (!form) return;

    const submitBtn = document.getElementById('submit-btn');
    const btnText = submitBtn.querySelector('.btn-text');
    const arrow = submitBtn.querySelector('.arrow');
    const result = document.getElementById('result');
    const errorBox = document.getElementById('error');
    const daysHint = document.getElementById('days-hint');
    const minRecoverySlider = document.getElementById('min-recovery');
    const minRecoveryVal = document.getElementById('min-recovery-val');
    const recoveryHint = document.getElementById('recovery-hint');

    // Weekday toggles — multi-select. The user's choices determine both
    // days_per_week (count) and the ordered weekdays array we send.
    const selectedDays = new Set();
    const WEEKDAY_ORDER = ['mon','tue','wed','thu','fri','sat','sun'];
    document.querySelectorAll('.weekday-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        const day = btn.dataset.day;
        if (selectedDays.has(day)) {
          selectedDays.delete(day);
          btn.classList.remove('active');
        } else {
          if (selectedDays.size >= 6) {
            // 6 days is the engine max — extra clicks are no-ops.
            return;
          }
          selectedDays.add(day);
          btn.classList.add('active');
        }
        const n = selectedDays.size;
        daysHint.textContent = n === 0
          ? "tap the days you'll train"
          : n + (n === 1 ? ' day' : ' days') + ' selected';
      });
    });

    minRecoverySlider.addEventListener('input', () => {
      minRecoveryVal.textContent = minRecoverySlider.value;
      recoveryHint.textContent = minRecoverySlider.value === '0'
        ? 'no recovery floor'
        : minRecoverySlider.value + (minRecoverySlider.value === '1' ? ' day' : ' days');
    });

    // Sets-per-exercise override. 0 = auto (use goal defaults); 1..5 = fixed.
    const setsOverrideSlider = document.getElementById('sets-override');
    const setsOverrideVal = document.getElementById('sets-override-val');
    const setsHint = document.getElementById('sets-hint');
    setsOverrideSlider.addEventListener('input', () => {
      const v = setsOverrideSlider.value;
      if (v === '0') {
        setsOverrideVal.textContent = 'auto';
        setsHint.textContent = 'auto (from goal)';
      } else {
        setsOverrideVal.textContent = v;
        setsHint.textContent = v === '1'
          ? '1 set — high intensity'
          : v + ' sets per exercise';
      }
    });

    // Single-select recovery buttons. The "auto" mode sets recoveryAware,
    // which makes the server pull recovery from the user's connected
    // biometrics provider (Oura/Whoop) instead of using a manual value.
    let recovery = null;
    let recoveryAware = false;
    document.querySelectorAll('.recovery-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        document.querySelectorAll('.recovery-btn').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        const v = btn.dataset.recovery;
        if (v === 'auto') {
          recovery = null;
          recoveryAware = true;
        } else {
          recovery = v === '' ? null : parseFloat(v);
          recoveryAware = false;
        }
      });
    });

    // On load, ask the backend whether the user has any connected biometric
    // provider with fresh data. If yes, surface the Auto button and make it
    // the default selection.
    initRecoveryFromBiometrics();

    async function initRecoveryFromBiometrics() {
      try {
        const provData = await API.get('/v1/biometrics/providers');
        const connected = (provData.providers || []).filter(p => p.connected);
        if (connected.length === 0) return;
        const names = connected.map(p => p.name[0].toUpperCase() + p.name.slice(1)).join(' / ');
        const autoBtn = document.getElementById('auto-recovery-btn');
        autoBtn.classList.remove('hidden');
        document.getElementById('auto-recovery-label').textContent = 'from ' + names;
        document.getElementById('recovery-status').textContent =
          names + ' linked — auto recovery on by default';

        // Default-select the Auto button (replacing the "Off" default).
        document.querySelectorAll('.recovery-btn').forEach(b => b.classList.remove('active'));
        autoBtn.classList.add('active');
        recovery = null;
        recoveryAware = true;
      } catch (e) {
        // Biometrics may be disabled on the server; that's fine, keep
        // manual mode as default.
      }
    }

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
      // Ordered weekdays from the selected set, in Mon→Sun order.
      const weekdays = WEEKDAY_ORDER.filter(d => selectedDays.has(d));
      if (weekdays.length < 2) {
        UI.showError(errorBox, {
          code: 'validation_failed',
          message: 'Pick at least 2 training days',
        });
        return;
      }
      const injuries = fd.getAll('injuries');
      const split = fd.get('split') || 'auto';

      const goals = fd.getAll('goals');
      if (goals.length === 0) {
        UI.showError(errorBox, {
          code: 'validation_failed',
          message: 'Pick at least one goal',
        });
        return;
      }
      const body = {
        goal: goals[0], // primary goal — engine uses this for prescription
        secondary_goals: goals.slice(1), // server can use these for nuance
        experience: fd.get('experience'),
        days_per_week: weekdays.length,
        available_equipment: equipment,
        injuries: injuries.length ? injuries : undefined,
        split: split,
        weekdays: weekdays,
        min_recovery_days: parseInt(minRecoverySlider.value, 10),
      };
      if (recovery !== null) body.recovery_hint = recovery;
      const setsOv = parseInt(setsOverrideSlider.value, 10);
      if (setsOv > 0) body.sets_override = setsOv;

      // Loading state — rewrite the whole button to avoid stale-reference
      // bugs on the second click after a failed first try.
      const ORIGINAL_BTN_HTML =
        '<span class="btn-text">Generate Plan</span><span class="arrow">→</span>';
      submitBtn.disabled = true;
      submitBtn.innerHTML = '<span class="btn-text">Generating</span><span class="spinner"></span>';

      try {
        const url = '/v1/plans/generate?seed=' + Date.now() +
          (recoveryAware ? '&recovery_aware=true' : '');
        const plan = await API.post(url, body);
        renderPlan(result, plan);
      } catch (err) {
        UI.showError(errorBox, err);
      } finally {
        submitBtn.disabled = false;
        submitBtn.innerHTML = ORIGINAL_BTN_HTML;
      }
    });
  });

  function injectWillBurgessQuote() {
    const card = document.createElement('div');
    card.className = 'eg-card';
    card.innerHTML =
      '<div class="eg-card-quote">' +
        '<span class="eg-quote-mark">"</span>' +
        'There\'s no commandment that says thou shall not send.' +
      '</div>' +
      '<div class="eg-card-attr">— Will Burgess</div>' +
      '<button class="eg-card-close" aria-label="Dismiss">×</button>';
    document.body.appendChild(card);
    card.querySelector('.eg-card-close').addEventListener('click', () => card.remove());
  }

  function renderSetTypeBadge(ex) {
    if (!ex.set_type || ex.set_type === 'standard') return '';
    const labelByType = {
      amrap:        { label: 'AMRAP last set', cls: 'st-amrap' },
      drop_set:     { label: 'Drop set finish', cls: 'st-drop' },
      twenty_ones:  { label: '21s', cls: 'st-21s' },
      superset:     { label: 'Superset', cls: 'st-superset' },
    };
    const meta = labelByType[ex.set_type];
    if (!meta) return '';
    const note = ex.set_type_note ? UI.escape(ex.set_type_note) : '';
    return '<div class="set-type-badge ' + meta.cls + '">' +
      '<span class="set-type-label">' + meta.label + '</span>' +
      (note ? '<span class="set-type-note">' + note + '</span>' : '') +
    '</div>';
  }

  function renderPlan(el, plan) {
    const warnings = (plan.warnings || []).map(w =>
      '<li>' + UI.escape(w) + '</li>'
    ).join('');

    const days = (plan.days || []).map(day => {
      const exercises = (day.exercises || []).map(ex => {
        const e = ex.exercise || {};
        const compound = e.compound ? ' <span class="compound">compound</span>' : '';
        const ytURL = 'https://www.youtube.com/results?search_query=' +
          encodeURIComponent(e.name + ' exercise form');
        const warmupChips = (ex.warmups || []).map(w => {
          const pct = Math.round(w.percent_of_working * 100);
          return '<span class="warmup-chip">' + w.reps + ' × ' + pct + '%</span>';
        }).join('');
        const warmupRow = warmupChips
          ? '<div class="exercise-warmup"><span class="warmup-label">warmup</span>' + warmupChips + '</div>'
          : '';
        const setTypeBadge = renderSetTypeBadge(ex);
        const exerciseClass = ex.set_type === 'superset' ? 'exercise superset' : 'exercise';
        return '<div class="' + exerciseClass + '">' +
          '<div class="exercise-row">' +
            '<div class="exercise-name">' + UI.escape(e.name) + '</div>' +
            '<a class="demo-link" href="' + ytURL + '" target="_blank" rel="noopener" title="Watch demo on YouTube">' +
              '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
                '<polygon points="5 3 19 12 5 21 5 3"></polygon>' +
              '</svg>' +
              '<span>demo</span>' +
            '</a>' +
          '</div>' +
          warmupRow +
          '<div class="exercise-dose">' +
            '<span>' + ex.sets + ' × ' + ex.reps_low + '–' + ex.reps_high + '</span>' +
            '<span>rest ' + ex.rest_seconds + 's</span>' +
            compound +
          '</div>' +
          setTypeBadge +
        '</div>';
      }).join('');
      const weekdayBadge = day.weekday
        ? '<div class="day-weekday">' + UI.escape(day.weekday.toUpperCase()) + '</div>'
        : '';
      return '<div class="day-card">' +
        '<div class="day-header">' +
          '<div class="day-name">' + UI.escape(day.name) + '</div>' +
          (weekdayBadge || '<div class="day-num">day ' + day.index + '</div>') +
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
