// pages/signup.js — create account + tell the user to check their email.

(function () {
  'use strict';

  document.addEventListener('DOMContentLoaded', () => {
    AUTH.redirectIfLoggedIn('/');

    const form = document.getElementById('signup-form');
    const btn = document.getElementById('submit-btn');
    const btnText = btn.querySelector('.btn-text');
    const arrow = btn.querySelector('.arrow');
    const errBox = document.getElementById('error');
    const successBox = document.getElementById('success');
    const sentTo = document.getElementById('sent-to');

    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      UI.hideError(errBox);
      successBox.classList.add('hidden');
      const fd = new FormData(form);
      const email = fd.get('email');

      btn.disabled = true;
      btnText.textContent = 'Creating';
      arrow.outerHTML = '<span class="spinner"></span>';

      try {
        await API.post('/v1/auth/signup', {
          email,
          password: fd.get('password'),
        });
        sentTo.textContent = email;
        successBox.classList.remove('hidden');
        form.style.display = 'none';
      } catch (err) {
        UI.showError(errBox, err);
      } finally {
        btn.disabled = false;
        btnText.textContent = 'Create account';
        const spin = btn.querySelector('.spinner');
        if (spin) spin.outerHTML = '<span class="arrow">→</span>';
      }
    });
  });
})();
