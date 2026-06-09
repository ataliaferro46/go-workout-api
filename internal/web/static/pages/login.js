// pages/login.js — sign in. Skips ensureLoggedIn because by definition we
// shouldn't already be authenticated; if we are, redirect to /.

(function () {
  'use strict';

  document.addEventListener('DOMContentLoaded', () => {
    AUTH.redirectIfLoggedIn('/');

    const form = document.getElementById('login-form');
    const btn = document.getElementById('submit-btn');
    const btnText = btn.querySelector('.btn-text');
    const arrow = btn.querySelector('.arrow');
    const errBox = document.getElementById('error');

    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      UI.hideError(errBox);
      const fd = new FormData(form);
      btn.disabled = true;
      btnText.textContent = 'Signing in';
      arrow.outerHTML = '<span class="spinner"></span>';

      try {
        await API.post('/v1/auth/login', {
          email: fd.get('email'),
          password: fd.get('password'),
        });
        window.location.href = '/';
      } catch (err) {
        UI.showError(errBox, err);
      } finally {
        btn.disabled = false;
        btnText.textContent = 'Sign in';
        const spin = btn.querySelector('.spinner');
        if (spin) spin.outerHTML = '<span class="arrow">→</span>';
      }
    });
  });
})();
