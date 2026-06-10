// pages/quick.js — ad-hoc single-day workout builder.
//
// Flow:
//   1. POST /v1/plans/quick-day with chosen muscles + filters → returns a
//      PlanDay (exercises, prescriptions, set types, warmups).
//   2. POST /v1/workouts with that day's exercises → returns a Workout id.
//   3. Navigate to /workout/{id}.

(function () {
  'use strict';

  document.addEventListener('DOMContentLoaded', async () => {
    await AUTH.ensureLoggedIn();
    // Mode tabs: auto-build vs custom vs cardio
    document.querySelectorAll('.mode-tab').forEach(t => {
      t.addEventListener('click', () => {
        document.querySelectorAll('.mode-tab').forEach(x => x.classList.remove('active'));
        t.classList.add('active');
        const mode = t.dataset.mode;
        document.getElementById('strength-card').style.display = mode === 'strength' ? '' : 'none';
        document.getElementById('custom-card').style.display = mode === 'custom' ? '' : 'none';
        document.getElementById('cardio-card').style.display = mode === 'cardio' ? '' : 'none';
      });
    });

    initCustomBuilder();

    // Cardio duration slider + submit
    const dur = document.getElementById('cardio-duration');
    const durVal = document.getElementById('cardio-dur-val');
    const durHint = document.getElementById('cardio-dur-hint');
    dur.addEventListener('input', () => {
      durVal.textContent = dur.value;
      durHint.textContent = dur.value;
    });
    function initCustomBuilder() {
      const search = document.getElementById('custom-search');
      const results = document.getElementById('custom-search-results');
      const list = document.getElementById('custom-list');
      const startBtn = document.getElementById('custom-start-btn');
      const picked = [];
      let exerciseLib = null;

      // Lazy load exercise library
      async function ensureLib() {
        if (exerciseLib) return exerciseLib;
        const data = await API.get('/v1/exercises');
        exerciseLib = (data.exercises || data || []);
        return exerciseLib;
      }

      function refreshList() {
        list.innerHTML = picked.length === 0
          ? '<p class="form-help">No exercises yet — search above to add some.</p>'
          : picked.map((ex, i) => renderRow(ex, i)).join('');
        startBtn.disabled = picked.length === 0;
        list.querySelectorAll('[data-remove]').forEach(b => {
          b.addEventListener('click', () => {
            picked.splice(parseInt(b.dataset.remove, 10), 1);
            refreshList();
          });
        });
      }

      function renderRow(ex, idx) {
        return '<div class="custom-row">' +
          '<div class="custom-row-name">' + UI.escape(ex.name) + '</div>' +
          '<div class="custom-row-inputs">' +
            '<input type="number" min="1" value="' + ex.sets + '" data-set-input="' + idx + '" placeholder="sets" class="input">' +
            '<span>×</span>' +
            '<input type="number" min="1" value="' + ex.reps + '" data-rep-input="' + idx + '" placeholder="reps" class="input">' +
            '<span>@</span>' +
            '<input type="number" min="0" step="0.5" value="' + ex.weight_kg + '" data-weight-input="' + idx + '" placeholder="kg" class="input">' +
            '<button class="btn btn-ghost" data-remove="' + idx + '">×</button>' +
          '</div>' +
        '</div>';
      }

      let st;
      search.addEventListener('input', () => {
        clearTimeout(st);
        st = setTimeout(async () => {
          const q = search.value.trim().toLowerCase();
          if (q.length < 2) { results.innerHTML = ''; return; }
          const lib = await ensureLib();
          const matches = lib.filter(e => e.name.toLowerCase().includes(q)).slice(0, 10);
          results.innerHTML = matches.length === 0
            ? '<p class="form-help">No matches.</p>'
            : matches.map(m => '<button class="food-result" data-add-id="' + UI.escape(m.id) + '">' +
                '<div class="food-name">' + UI.escape(m.name) + '</div>' +
                '<div class="food-macros">' + UI.escape(m.primary_muscle || '') + ' · ' + (m.compound ? 'compound' : 'isolation') + '</div>' +
              '</button>').join('');
          results.querySelectorAll('[data-add-id]').forEach(b => {
            b.addEventListener('click', () => {
              const m = matches.find(x => x.id === b.dataset.addId);
              picked.push({ name: m.name, sets: 3, reps: 10, weight_kg: 0 });
              search.value = '';
              results.innerHTML = '';
              refreshList();
            });
          });
        }, 200);
      });

      list.addEventListener('input', (e) => {
        const t = e.target;
        const i = parseInt(t.dataset.setInput || t.dataset.repInput || t.dataset.weightInput, 10);
        if (isNaN(i) || !picked[i]) return;
        if (t.dataset.setInput) picked[i].sets = parseInt(t.value, 10) || 0;
        if (t.dataset.repInput) picked[i].reps = parseInt(t.value, 10) || 0;
        if (t.dataset.weightInput) picked[i].weight_kg = parseFloat(t.value) || 0;
      });

      startBtn.addEventListener('click', async () => {
        if (picked.length === 0) return;
        const name = document.getElementById('custom-name').value.trim() || 'Custom workout';
        startBtn.disabled = true;
        startBtn.querySelector('.btn-text').textContent = 'Creating…';
        try {
          const w = await API.post('/v1/workouts', { name: name, notes: '', exercises: picked });
          window.location.href = '/workout/' + encodeURIComponent(w.id);
        } catch (err) {
          UI.showError(document.getElementById('error'), err);
          startBtn.disabled = false;
          startBtn.querySelector('.btn-text').textContent = 'Start workout →';
        }
      });

      refreshList();
    }

    // Sync cardio distance label with user's unit preference
    const cardioDistUnit = document.getElementById('cardio-dist-unit');
    if (cardioDistUnit) {
      cardioDistUnit.textContent = '(' + UNITS.distanceLabel() + ', optional)';
    }

    document.getElementById('cardio-form').addEventListener('submit', async (e) => {
      e.preventDefault();
      const fd = new FormData(e.target);
      const distDisplay = parseFloat(fd.get('distance_display')) || 0;
      const body = {
        activity: fd.get('activity'),
        intensity: fd.get('intensity'),
        duration_minutes: parseInt(fd.get('duration_minutes'), 10),
        distance_km: distDisplay > 0 ? UNITS.displayToKm(distDisplay) : 0,
      };
      const submit = document.getElementById('cardio-submit-btn');
      submit.disabled = true;
      submit.querySelector('.btn-text').textContent = 'Logging…';
      try {
        const w = await API.post('/v1/workouts/cardio', body);
        window.location.href = '/workout/' + encodeURIComponent(w.id);
      } catch (err) {
        UI.showError(document.getElementById('error'), err);
        submit.disabled = false;
        submit.querySelector('.btn-text').textContent = 'Log cardio →';
      }
    });

    const form = document.getElementById('quick-form');
    const errBox = document.getElementById('error');
    const minutesSlider = document.getElementById('quick-minutes');
    const minutesVal = document.getElementById('quick-min-val');
    const minutesHint = document.getElementById('quick-min-hint');
    minutesSlider.addEventListener('input', () => {
      minutesVal.textContent = minutesSlider.value;
      minutesHint.textContent = minutesSlider.value;
    });

    const setsSlider = document.getElementById('quick-sets');
    const setsVal = document.getElementById('quick-sets-val');
    const setsHint = document.getElementById('quick-sets-hint');
    setsSlider.addEventListener('input', () => {
      const v = setsSlider.value;
      if (v === '0') { setsVal.textContent = 'auto'; setsHint.textContent = 'auto'; }
      else { setsVal.textContent = v; setsHint.textContent = v + ' per exercise'; }
    });

    // Recovery toggle (mirrors home.js): when Oura is connected, "Auto"
    // shows and is default-selected, sending recovery_aware=true.
    let quickRecovery = null;
    let quickRecoveryAware = false;
    document.querySelectorAll('#quick-recovery-toggle .recovery-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        document.querySelectorAll('#quick-recovery-toggle .recovery-btn').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        const v = btn.dataset.recovery;
        if (v === 'auto') { quickRecovery = null; quickRecoveryAware = true; }
        else { quickRecovery = v === '' ? null : parseFloat(v); quickRecoveryAware = false; }
      });
    });
    (async function initRecoveryFromBio() {
      try {
        const data = await API.get('/v1/biometrics/providers');
        const connected = (data.providers || []).filter(p => p.connected);
        if (connected.length === 0) return;
        const names = connected.map(p => p.name[0].toUpperCase() + p.name.slice(1)).join(' / ');
        const autoBtn = document.getElementById('quick-auto-recovery-btn');
        autoBtn.classList.remove('hidden');
        document.getElementById('quick-auto-recovery-label').textContent = 'from ' + names;
        document.getElementById('quick-recovery-status').textContent = names + ' linked — auto on by default';
        document.querySelectorAll('#quick-recovery-toggle .recovery-btn').forEach(b => b.classList.remove('active'));
        autoBtn.classList.add('active');
        quickRecovery = null; quickRecoveryAware = true;
      } catch (e) { /* biometrics disabled; stay on manual */ }
    })();

    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      UI.hideError(errBox);
      const fd = new FormData(form);
      const muscles = fd.getAll('muscles');
      if (muscles.length === 0) {
        UI.showError(errBox, { code: 'validation_failed', message: 'Pick at least one muscle group.' });
        return;
      }
      const goals = fd.getAll('goals');
      if (goals.length === 0) {
        UI.showError(errBox, { code: 'validation_failed', message: 'Pick at least one goal.' });
        return;
      }
      const equipment = fd.getAll('equipment');
      if (equipment.length === 0) {
        UI.showError(errBox, { code: 'validation_failed', message: 'Pick at least one piece of equipment.' });
        return;
      }
      const injuries = fd.getAll('injuries');

      const body = {
        goal: goals[0], // primary
        secondary_goals: goals.slice(1),
        experience: fd.get('experience'),
        available_equipment: equipment,
        injuries: injuries.length ? injuries : [],
        muscles: muscles,
        session_minutes: parseInt(fd.get('session_minutes'), 10),
      };
      const setsOv = parseInt(fd.get('sets_override'), 10);
      if (setsOv > 0) body.sets_override = setsOv;
      if (quickRecovery !== null) body.recovery_hint = quickRecovery;

      const submit = document.getElementById('quick-submit-btn');
      submit.disabled = true;
      submit.querySelector('.btn-text').textContent = 'Building…';

      try {
        const url = '/v1/plans/quick-day' + (quickRecoveryAware ? '?recovery_aware=true' : '');
        const day = await API.post(url, body);
        const exercises = (day.exercises || []).map(ex => ({
          name: (ex.exercise && ex.exercise.name) || 'exercise',
          sets: ex.sets || 0,
          reps: Math.round(((ex.reps_low || 0) + (ex.reps_high || 0)) / 2),
          weight_kg: 0,
        }));
        const w = await API.post('/v1/workouts', {
          name: day.name || 'Quick workout',
          notes: '',
          exercises: exercises,
        });
        window.location.href = '/workout/' + encodeURIComponent(w.id);
      } catch (err) {
        UI.showError(errBox, err);
        submit.disabled = false;
        submit.querySelector('.btn-text').textContent = 'Build & start →';
      }
    });
  });
})();
