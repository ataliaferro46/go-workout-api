// pages/workout.js — in-progress workout. Renders the day's exercises,
// shows N input rows per exercise (one per prescribed set), and POSTs
// each set as the user logs it.

(function () {
  'use strict';

  // Name → exercise object (so we can look up required_equipment to
  // decide whether to show a weight input). Loaded lazily on first
  // workout render so the page boot stays snappy.
  let exerciseByName = null;
  // Equipment types that mean "no quantifiable weight to enter."
  const BW_ONLY = new Set(['bodyweight', 'pullup_bar', 'bands']);

  document.addEventListener('DOMContentLoaded', async () => {
    await AUTH.ensureLoggedIn();
    const workoutID = extractIDFromPath();
    if (!workoutID) {
      UI.showError(document.getElementById('error'),
        { code: 'bad_url', message: 'No workout id in URL.' });
      return;
    }
    await render(workoutID);
  });

  async function loadExerciseLib() {
    if (exerciseByName) return exerciseByName;
    try {
      const data = await API.get('/v1/exercises');
      const map = {};
      (data.exercises || data || []).forEach(e => { map[e.name] = e; });
      exerciseByName = map;
    } catch (e) {
      exerciseByName = {};
    }
    return exerciseByName;
  }

  function isBodyweightExercise(name) {
    const e = exerciseByName && exerciseByName[name];
    if (!e) return false;
    const eq = e.required_equipment || [];
    if (eq.length === 0) return true;
    return eq.every(x => BW_ONLY.has(x));
  }

  function extractIDFromPath() {
    const m = window.location.pathname.match(/^\/workout\/([^\/]+)/);
    return m ? m[1] : null;
  }

  async function render(workoutID) {
    const body = document.getElementById('workout-body');
    try {
      // Load exercise library + workout in parallel.
      const [w, _lib] = await Promise.all([
        API.get('/v1/workouts/' + encodeURIComponent(workoutID)),
        loadExerciseLib(),
      ]);
      document.getElementById('workout-name').textContent = w.name || 'Workout';
      const meta = [];
      if (w.created_at) meta.push('started ' + UI.formatDate(w.created_at));
      if (w.exercises) meta.push(w.exercises.length + ' exercises');
      document.getElementById('workout-meta').textContent = meta.join(' · ');
      if (w.type === 'cardio' && w.cardio_session) {
        body.innerHTML =
          '<div id="intensity-card" class="hidden"></div>' +
          renderCardio(w.cardio_session);
        loadIntensity(workoutID);
        return;
      }
      body.innerHTML =
        renderUnitToggle() +
        '<div id="intensity-card" class="hidden"></div>' +
        (w.exercises || []).map((ex, i) => renderExercise(ex, i)).join('');
      attachHandlers(workoutID);
      loadIntensity(workoutID);
      attachUnitToggle(workoutID);
      loadLastTimeHints();
    } catch (err) {
      UI.showError(document.getElementById('error'), err);
    }
  }

  // Per-page aliases of the global UNITS helpers; default behavior is
  // imperial (lbs) since UNITS.system() defaults to 'imperial'.
  function unitLabel() { return UNITS.weightLabel(); }
  function kgToDisplay(kg) { return UNITS.kgToDisplay(kg); }
  function displayToKg(val) { return UNITS.displayToKg(val); }

  function renderExercise(ex, position) {
    const setRows = [];
    const targetSets = ex.sets > 0 ? ex.sets : 3;
    const logged = ex.logged_sets || [];
    const loggedBySet = {};
    logged.forEach(s => { loggedBySet[s.set_number] = s; });

    const bw = isBodyweightExercise(ex.name);
    const unit = unitLabel();
    const ytURL = 'https://www.youtube.com/results?search_query=' +
      encodeURIComponent(ex.name + ' exercise form');

    // Render the prescription badges (set type, warmups, rep range) so
    // the live workout mirrors what was on the plan view. Each comes
    // from ex.prescription when the workout was started from a plan.
    const presc = ex.prescription || {};
    const setTypeBadge = renderActiveSetTypeBadge(presc);
    const warmupRow = renderActiveWarmupRow(presc);
    const repRange = (presc.reps_low > 0 && presc.reps_high > 0)
      ? presc.reps_low + '-' + presc.reps_high
      : (ex.reps || '?');
    const restNote = presc.rest_seconds > 0
      ? ' · rest ' + presc.rest_seconds + 's'
      : '';
    // target_reps lets the user prescribe variable reps per set (e.g.,
    // a pyramid 10, 8, 6). If empty, every set uses the same target.
    const targetReps = (ex.target_reps && ex.target_reps.length > 0) ? ex.target_reps : null;

    for (let n = 1; n <= targetSets; n++) {
      const s = loggedBySet[n];
      const done = !!s;
      const wVal = done ? kgToDisplay(s.weight_kg) : '';
      const weightInput = bw
        ? '<input type="hidden" class="set-weight" value="0">'
        : '<span class="set-x">×</span>' +
          '<input type="number" class="set-weight" placeholder="' + unit + '" value="' + wVal + '" min="0" step="0.5">';
      // Per-set target reps: if target_reps is set, show that set's target;
      // otherwise show the prescription's rep range hint.
      const setTargetHint = targetReps && targetReps[n-1]
        ? ' target ' + targetReps[n-1]
        : (presc.reps_low > 0 ? ' target ' + presc.reps_low + '-' + presc.reps_high : '');
      const repsPlaceholder = targetReps && targetReps[n-1] ? targetReps[n-1] : 'reps';
      setRows.push(
        '<div class="set-row ' + (done ? 'set-done' : '') + (bw ? ' bw' : '') + '" data-set="' + n + '" data-pos="' + position + '">' +
          '<div class="set-num">Set ' + n + '<span class="set-target-hint">' + UI.escape(setTargetHint) + '</span></div>' +
          '<div class="set-inputs">' +
            '<input type="number" class="set-reps" placeholder="' + repsPlaceholder + '" value="' + (done ? s.reps : '') + '" min="0">' +
            weightInput +
            '<button class="set-log-btn">' + (done ? '✓ logged' : 'log set') + '</button>' +
          '</div>' +
          (done ? '<div class="set-when">' + UI.escape(UI.formatDate(s.completed_at)) + '</div>' : '') +
        '</div>'
      );
    }

    const targetSetsStr = targetReps ? targetReps.join(',') : '';
    const doseLine = ex.sets > 0
      ? '<span class="dose-static">' + ex.sets + ' × ' + repRange + restNote + '</span>'
      : '';
    return '<div class="ex-card" data-ex-name="' + UI.escape(ex.name) + '" data-position="' + position + '">' +
      '<div class="ex-head">' +
        '<div class="ex-name-row">' +
          '<div class="ex-name">' + UI.escape(ex.name) + '</div>' +
          '<a class="ex-demo" href="' + ytURL + '" target="_blank" rel="noopener" title="Watch demo">' +
            '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg>' +
            ' demo' +
          '</a>' +
          '<button class="ex-edit" data-edit-position="' + position + '" title="Edit prescription">✎ edit</button>' +
          '<button class="ex-swap" data-ex-name="' + UI.escape(ex.name) + '" data-position="' + position + '">↔ swap</button>' +
        '</div>' +
        (doseLine ? '<div class="ex-dose">' + doseLine + '</div>' : '') +
        '<div class="ex-last-time" data-ex-last="' + UI.escape(ex.name) + '"></div>' +
        warmupRow +
        setTypeBadge +
        // Inline editor (hidden by default): sets / target reps / weight
        '<div class="ex-edit-form hidden" data-edit-form="' + position + '">' +
          '<div class="ex-edit-row">' +
            '<label>Sets <input type="number" class="ex-edit-sets" value="' + ex.sets + '" min="1" max="10"></label>' +
            '<label>Reps <input type="text" class="ex-edit-target-reps" placeholder="e.g. 10,8,6" value="' + UI.escape(targetSetsStr) + '"></label>' +
            '<button class="btn btn-primary ex-edit-save" data-save-position="' + position + '">Save</button>' +
            '<button class="btn btn-ghost ex-edit-cancel" data-cancel-position="' + position + '">Cancel</button>' +
          '</div>' +
          '<p class="ex-edit-hint">Type comma-separated reps (10,8,6) for variable schemes, or a single number for uniform.</p>' +
        '</div>' +
      '</div>' +
      '<div class="set-rows">' + setRows.join('') + '</div>' +
    '</div>';
  }

  function renderActiveSetTypeBadge(presc) {
    if (!presc.set_type || presc.set_type === 'standard') return '';
    const meta = {
      amrap:        { label: 'AMRAP last set', cls: 'st-amrap' },
      drop_set:     { label: 'Drop set finish', cls: 'st-drop' },
      twenty_ones:  { label: '21s', cls: 'st-21s' },
      superset:     { label: 'Superset', cls: 'st-superset' },
    }[presc.set_type];
    if (!meta) return '';
    const note = presc.set_type_note ? '<span class="set-type-note">' + UI.escape(presc.set_type_note) + '</span>' : '';
    return '<div class="set-type-badge ' + meta.cls + '">' +
      '<span class="set-type-label">' + meta.label + '</span>' + note +
    '</div>';
  }

  function renderActiveWarmupRow(presc) {
    const ws = presc.warmups || [];
    if (ws.length === 0) return '';
    const chips = ws.map(w => {
      const pct = Math.round(w.percent_of_working * 100);
      return '<span class="warmup-chip">' + w.reps + ' × ' + pct + '%</span>';
    }).join('');
    return '<div class="exercise-warmup"><span class="warmup-label">warmup</span>' + chips + '</div>';
  }

  async function loadIntensity(workoutID) {
    const card = document.getElementById('intensity-card');
    try {
      const data = await API.get('/v1/workouts/' + encodeURIComponent(workoutID) + '/intensity');
      if (!data.available) return; // Oura not connected
      const i = data.intensity;
      if (!i || i.samples_count === 0) {
        card.innerHTML =
          '<div class="intensity-empty">' +
            '<div class="intensity-title">Session intensity</div>' +
            '<div class="intensity-note">No Oura HR samples in this window yet. Make sure your ring synced after the session.</div>' +
          '</div>';
        card.classList.remove('hidden');
        return;
      }
      card.innerHTML =
        '<div class="intensity-grid">' +
          '<div class="intensity-title">Session intensity · <span class="intensity-src">' + UI.escape(i.source || 'oura') + '</span></div>' +
          '<div class="intensity-stats">' +
            statTile('Avg HR', i.avg_bpm + ' bpm') +
            statTile('Peak', i.max_bpm + ' bpm') +
            statTile('Above zone 4', i.minutes_above_140 + ' min') +
            statTile('Duration', i.duration_minutes + ' min') +
          '</div>' +
        '</div>';
      card.classList.remove('hidden');
    } catch (err) {
      // Intensity is opt-in — silent fail
    }
  }

  function statTile(label, value) {
    return '<div class="intensity-tile">' +
      '<div class="intensity-label">' + UI.escape(label) + '</div>' +
      '<div class="intensity-value">' + UI.escape(value) + '</div>' +
    '</div>';
  }

  function renderCardio(cs) {
    const tile = (label, val) =>
      '<div class="intensity-tile">' +
        '<div class="intensity-label">' + UI.escape(label) + '</div>' +
        '<div class="intensity-value">' + UI.escape(val) + '</div>' +
      '</div>';
    const caloriesNote = cs.calories_source === 'met_formula'
      ? 'Estimated via MET formula from your stored weight.'
      : cs.calories_source === 'oura'
        ? 'From your Oura ring.'
        : '';
    return '<div class="cardio-card">' +
      '<div class="cardio-head">' +
        '<div class="cardio-act">' + UI.escape(cs.activity[0].toUpperCase() + cs.activity.slice(1)) + '</div>' +
        '<div class="cardio-int">' + UI.escape(cs.intensity) + '</div>' +
      '</div>' +
      '<div class="intensity-stats">' +
        tile('Duration', cs.duration_minutes + ' min') +
        (cs.distance_km > 0 ? tile('Distance', UNITS.kmToDisplay(cs.distance_km) + ' ' + UNITS.distanceLabel()) : '') +
        tile('Calories', cs.calories || '—') +
      '</div>' +
      (caloriesNote ? '<p class="cardio-note">' + UI.escape(caloriesNote) + '</p>' : '') +
    '</div>';
  }

  function renderUnitToggle() {
    const sys = UNITS.system();
    return '<div class="unit-toggle">' +
      '<button class="unit-btn ' + (sys === 'imperial' ? 'active' : '') + '" data-sys="imperial">lbs</button>' +
      '<button class="unit-btn ' + (sys === 'metric' ? 'active' : '') + '" data-sys="metric">kg</button>' +
    '</div>';
  }

  function attachUnitToggle(workoutID) {
    document.querySelectorAll('.unit-btn').forEach(b => {
      b.addEventListener('click', () => {
        UNITS.setSystem(b.dataset.sys);
        render(workoutID); // re-render with new unit
      });
    });
  }

  async function loadLastTimeHints() {
    const els = document.querySelectorAll('.ex-last-time[data-ex-last]');
    await Promise.all(Array.from(els).map(async el => {
      const name = el.dataset.exLast;
      try {
        const data = await API.get('/v1/workouts/exercise/last?name=' + encodeURIComponent(name));
        const sets = data.sets || [];
        if (sets.length === 0) return;
        const when = data.when ? UI.formatDate(data.when) : '';
        const summary = sets.map(s => {
          if (s.weight_kg > 0) {
            return s.reps + '×' + kgToDisplay(s.weight_kg) + unitLabel();
          }
          return s.reps + ' reps';
        }).join(' · ');
        el.innerHTML = '<span class="last-label">last time' + (when ? ' (' + UI.escape(when) + ')' : '') + ':</span> ' +
          '<span class="last-vals">' + UI.escape(summary) + '</span>';
      } catch (e) { /* silent — last-time is optional */ }
    }));
  }

  function attachEditHandlers(workoutID) {
    document.querySelectorAll('[data-edit-position]').forEach(btn => {
      btn.addEventListener('click', () => {
        const pos = btn.dataset.editPosition;
        const form = document.querySelector('[data-edit-form="' + pos + '"]');
        if (form) form.classList.toggle('hidden');
      });
    });
    document.querySelectorAll('[data-cancel-position]').forEach(btn => {
      btn.addEventListener('click', () => {
        const pos = btn.dataset.cancelPosition;
        const form = document.querySelector('[data-edit-form="' + pos + '"]');
        if (form) form.classList.add('hidden');
      });
    });
    document.querySelectorAll('[data-save-position]').forEach(btn => {
      btn.addEventListener('click', async () => {
        const pos = parseInt(btn.dataset.savePosition, 10);
        const form = document.querySelector('[data-edit-form="' + pos + '"]');
        const setsInput = form.querySelector('.ex-edit-sets');
        const targetInput = form.querySelector('.ex-edit-target-reps');
        const sets = parseInt(setsInput.value, 10) || 1;
        const repsStr = targetInput.value.trim();
        let target_reps = [];
        let reps = 0;
        if (repsStr.includes(',')) {
          target_reps = repsStr.split(',').map(s => parseInt(s.trim(), 10)).filter(n => n > 0);
          reps = target_reps[0] || 0;
        } else if (repsStr) {
          reps = parseInt(repsStr, 10) || 0;
        }
        btn.disabled = true;
        try {
          await API.fetch('/v1/workouts/' + encodeURIComponent(workoutID) +
            '/exercises/' + pos, {
            method: 'PATCH',
            body: JSON.stringify({
              sets: sets, reps: reps, weight_kg: 0, target_reps: target_reps,
            }),
          });
          await render(workoutID);
        } catch (err) {
          btn.disabled = false;
          UI.showError(document.getElementById('error'), err);
        }
      });
    });
  }

  async function openWorkoutSwapPicker(workoutID, position, currentName, cardEl) {
    // Remove any existing picker.
    document.querySelectorAll('.swap-picker').forEach(p => p.remove());
    const picker = document.createElement('div');
    picker.className = 'swap-picker';
    picker.innerHTML = '<div class="swap-picker-head">Find a replacement for <strong>' + UI.escape(currentName) + '</strong>:</div>' +
      '<div class="swap-picker-body"><p class="muted">Loading similar exercises…</p></div>';
    cardEl.appendChild(picker);
    try {
      const data = await API.get('/v1/exercises/alternatives?name=' + encodeURIComponent(currentName));
      const alts = data.alternatives || [];
      if (alts.length === 0) {
        picker.querySelector('.swap-picker-body').innerHTML =
          '<p class="muted">No similar exercises found. Make sure your equipment list isn\'t too restrictive.</p>' +
          '<button class="btn btn-ghost swap-cancel-btn">Cancel</button>';
      } else {
        picker.querySelector('.swap-picker-body').innerHTML =
          alts.map(a => {
            const tag = a.compound ? 'compound' : 'isolation';
            const region = a.region ? ' · ' + a.region.replace(/_/g,' ') : '';
            return '<button class="alt-option" data-swap-to="' + UI.escape(a.name) + '">' +
              '<div class="food-name">' + UI.escape(a.name) + '</div>' +
              '<div class="food-macros">' + tag + region + '</div>' +
            '</button>';
          }).join('') +
          '<button class="btn btn-ghost swap-cancel-btn">Cancel</button>';
      }
      picker.querySelectorAll('[data-swap-to]').forEach(opt => {
        opt.addEventListener('click', async () => {
          const newName = opt.dataset.swapTo;
          opt.textContent = 'Swapping…'; opt.disabled = true;
          try {
            await API.post('/v1/workouts/' + encodeURIComponent(workoutID) +
              '/exercises/' + position + '/swap', { name: newName });
            await render(workoutID);
          } catch (err) {
            UI.showError(document.getElementById('error'), err);
            picker.remove();
          }
        });
      });
      picker.querySelector('.swap-cancel-btn').addEventListener('click', () => picker.remove());
    } catch (err) {
      picker.querySelector('.swap-picker-body').innerHTML =
        '<p class="muted">Could not load alternatives: ' + UI.escape(err.message || 'error') + '</p>';
    }
  }

  function attachHandlers(workoutID) {
    attachEditHandlers(workoutID);
    document.querySelectorAll('.ex-swap').forEach(btn => {
      btn.addEventListener('click', async () => {
        const exName = btn.dataset.exName;
        const position = parseInt(btn.dataset.position, 10);
        await openWorkoutSwapPicker(workoutID, position, exName, btn.closest('.ex-card'));
      });
    });
    document.querySelectorAll('.set-log-btn').forEach(btn => {
      btn.addEventListener('click', async () => {
        const row = btn.closest('.set-row');
        const setNum = parseInt(row.dataset.set, 10);
        const pos = parseInt(row.dataset.pos, 10);
        const reps = parseInt(row.querySelector('.set-reps').value, 10);
        const isBW = row.classList.contains('bw');
        let weightInput = parseFloat(row.querySelector('.set-weight').value);
        // Backend always stores kg — convert from display unit if needed.
        let weight = isBW ? 0 : displayToKg(weightInput);
        if (isNaN(reps)) {
          UI.showError(document.getElementById('error'),
            { code: 'bad_input', message: 'Enter reps.' });
          return;
        }
        if (!isBW && isNaN(weight)) {
          UI.showError(document.getElementById('error'),
            { code: 'bad_input', message: 'Enter weight.' });
          return;
        }
        btn.disabled = true;
        btn.textContent = 'saving…';
        try {
          await API.post('/v1/workouts/' + encodeURIComponent(workoutID) + '/sets', {
            exercise_position: pos,
            set_number: setNum,
            reps: reps,
            weight_kg: weight,
          });
          row.classList.add('set-done');
          btn.textContent = '✓ logged';
        } catch (err) {
          btn.disabled = false;
          btn.textContent = 'log set';
          UI.showError(document.getElementById('error'), err);
        }
      });
    });
  }
})();
