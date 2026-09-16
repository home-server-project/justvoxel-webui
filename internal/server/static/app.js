document.addEventListener("submit", (event) => {
  const form = event.target.closest(".action-form");
  if (!form) return;

  const submitted = form.querySelector('button[type="submit"]');
  document.querySelectorAll(".action-form button").forEach((button) => {
    button.disabled = true;
  });

  if (submitted && form.dataset.progress) {
    submitted.textContent = form.dataset.progress;
  }
});
