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
    // Mode tabs: strength vs cardio
    document.querySelectorAll('.mode-tab').forEach(t => {
      t.addEventListener('click', () => {
        document.querySelectorAll('.mode-tab').forEach(x => x.classList.remove('active'));
        t.classList.add('active');
        const mode = t.dataset.mode;
        document.getElementById('strength-card').style.display = mode === 'strength' ? '' : 'none';
        document.getElementById('cardio-card').style.display = mode === 'cardio' ? '' : 'none';
      });
    });

    // Cardio duration slider + submit
    const dur = document.getElementById('cardio-duration');
    const durVal = document.getElementById('cardio-dur-val');
    const durHint = document.getElementById('cardio-dur-hint');
    dur.addEventListener('input', () => {
      durVal.textContent = dur.value;
      durHint.textContent = dur.value;
    });
    document.getElementById('cardio-form').addEventListener('submit', async (e) => {
      e.preventDefault();
      const fd = new FormData(e.target);
      const body = {
        activity: fd.get('activity'),
        intensity: fd.get('intensity'),
        duration_minutes: parseInt(fd.get('duration_minutes'), 10),
        distance_km: parseFloat(fd.get('distance_km')) || 0,
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

    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      UI.hideError(errBox);
      const fd = new FormData(form);
      const muscles = fd.getAll('muscles');
      if (muscles.length === 0) {
        UI.showError(errBox, { code: 'validation_failed', message: 'Pick at least one muscle group.' });
        return;
      }
      const equipment = fd.getAll('equipment');
      if (equipment.length === 0) {
        UI.showError(errBox, { code: 'validation_failed', message: 'Pick at least one piece of equipment.' });
        return;
      }
      const injuries = fd.getAll('injuries');

      const body = {
        goal: fd.get('goal'),
        experience: fd.get('experience'),
        available_equipment: equipment,
        injuries: injuries.length ? injuries : [],
        muscles: muscles,
        session_minutes: parseInt(fd.get('session_minutes'), 10),
      };
      const setsOv = parseInt(fd.get('sets_override'), 10);
      if (setsOv > 0) body.sets_override = setsOv;

      const submit = document.getElementById('quick-submit-btn');
      submit.disabled = true;
      submit.querySelector('.btn-text').textContent = 'Building…';

      try {
        const day = await API.post('/v1/plans/quick-day', body);
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
