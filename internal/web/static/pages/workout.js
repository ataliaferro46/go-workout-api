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

  function unitLabel() { return localStorage.getItem('wapi.unit') === 'lbs' ? 'lbs' : 'kg'; }
  function kgToDisplay(kg) {
    if (kg == null || kg === 0) return 0;
    return unitLabel() === 'lbs' ? +(kg * 2.20462).toFixed(1) : +kg.toFixed(1);
  }
  function displayToKg(val) {
    return unitLabel() === 'lbs' ? +(val / 2.20462).toFixed(2) : +val;
  }

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

    for (let n = 1; n <= targetSets; n++) {
      const s = loggedBySet[n];
      const done = !!s;
      const wVal = done ? kgToDisplay(s.weight_kg) : '';
      const weightInput = bw
        ? '<input type="hidden" class="set-weight" value="0">'
        : '<span class="set-x">×</span>' +
          '<input type="number" class="set-weight" placeholder="' + unit + '" value="' + wVal + '" min="0" step="0.5">';
      setRows.push(
        '<div class="set-row ' + (done ? 'set-done' : '') + (bw ? ' bw' : '') + '" data-set="' + n + '" data-pos="' + position + '">' +
          '<div class="set-num">Set ' + n + '</div>' +
          '<div class="set-inputs">' +
            '<input type="number" class="set-reps" placeholder="reps" value="' + (done ? s.reps : '') + '" min="0">' +
            weightInput +
            '<button class="set-log-btn">' + (done ? '✓ logged' : 'log set') + '</button>' +
          '</div>' +
          (done ? '<div class="set-when">' + UI.escape(UI.formatDate(s.completed_at)) + '</div>' : '') +
        '</div>'
      );
    }

    const doseLine = ex.sets > 0
      ? ex.sets + ' × ' + (ex.reps || '?') + ' reps prescribed'
      : '';
    return '<div class="ex-card" data-ex-name="' + UI.escape(ex.name) + '" data-position="' + position + '">' +
      '<div class="ex-head">' +
        '<div class="ex-name-row">' +
          '<div class="ex-name">' + UI.escape(ex.name) + '</div>' +
          '<a class="ex-demo" href="' + ytURL + '" target="_blank" rel="noopener" title="Watch demo">' +
            '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg>' +
            ' demo' +
          '</a>' +
          '<button class="ex-swap" data-ex-name="' + UI.escape(ex.name) + '" data-position="' + position + '">↔ swap</button>' +
        '</div>' +
        (doseLine ? '<div class="ex-dose">' + doseLine + '</div>' : '') +
        '<div class="ex-last-time" data-ex-last="' + UI.escape(ex.name) + '"></div>' +
      '</div>' +
      '<div class="set-rows">' + setRows.join('') + '</div>' +
    '</div>';
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
        (cs.distance_km > 0 ? tile('Distance', cs.distance_km.toFixed(2) + ' km') : '') +
        tile('Calories', cs.calories || '—') +
      '</div>' +
      (caloriesNote ? '<p class="cardio-note">' + UI.escape(caloriesNote) + '</p>' : '') +
    '</div>';
  }

  function renderUnitToggle() {
    const unit = unitLabel();
    return '<div class="unit-toggle">' +
      '<button class="unit-btn ' + (unit === 'kg' ? 'active' : '') + '" data-unit="kg">kg</button>' +
      '<button class="unit-btn ' + (unit === 'lbs' ? 'active' : '') + '" data-unit="lbs">lbs</button>' +
    '</div>';
  }

  function attachUnitToggle(workoutID) {
    document.querySelectorAll('.unit-btn').forEach(b => {
      b.addEventListener('click', () => {
        localStorage.setItem('wapi.unit', b.dataset.unit);
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

  function attachHandlers(workoutID) {
    document.querySelectorAll('.ex-swap').forEach(btn => {
      btn.addEventListener('click', async () => {
        const exName = btn.dataset.exName;
        const newName = prompt(
          'Swap "' + exName + '" — type the start of the replacement exercise name (or cancel).',
          ''
        );
        if (!newName) return;
        // Find first library exercise whose name starts with the typed text
        // (case-insensitive). For a richer picker we'd render alternatives
        // (like /plans does), but this swap fires mid-workout where a fast
        // text path is the right UX.
        const match = Object.values(exerciseByName || {}).find(e =>
          e.name.toLowerCase().startsWith(newName.toLowerCase())
        );
        if (!match) {
          UI.showError(document.getElementById('error'),
            { code: 'no_match', message: 'No exercise found starting with "' + newName + '".' });
          return;
        }
        // Mutating the workout's stored exercise name in-place requires a
        // backend endpoint we haven't built; for now we update the visible
        // card and the last-time hint so the user can proceed.
        const card = btn.closest('.ex-card');
        card.querySelector('.ex-name').textContent = match.name;
        card.setAttribute('data-ex-name', match.name);
        card.querySelector('.ex-last-time').setAttribute('data-ex-last', match.name);
        const ytURL = 'https://www.youtube.com/results?search_query=' + encodeURIComponent(match.name + ' exercise form');
        card.querySelector('.ex-demo').setAttribute('href', ytURL);
        await loadLastTimeHints();
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
