const form = document.querySelector('[data-auth]');
if (form) form.addEventListener('submit', async (event) => {
  event.preventDefault();
  const button = form.querySelector('button');
  const status = document.querySelector('[role="alert"]');
  const data = Object.fromEntries(new FormData(form));
  status.textContent = '';
  if (data.confirm_password !== undefined && data.password !== data.confirm_password) {
    status.textContent = 'Your passwords do not match.'; return;
  }
  delete data.confirm_password;
  button.disabled = true;
  status.textContent = 'Please wait…';
  try {
    const response = await fetch(form.getAttribute('action'), {
      method: 'POST', credentials: 'same-origin',
      headers: {'Content-Type': 'application/json'}, body: JSON.stringify(data)
    });
    const result = await response.json();
    if (!response.ok) { status.textContent = result.error || 'Please try again.'; return; }
    window.location.assign(form.dataset.auth === 'logout' ? '/' : '/profile');
  } catch {
    status.textContent = 'Unable to connect. Please try again.';
  } finally { button.disabled = false; }
});
