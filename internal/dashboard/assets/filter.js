// assets/filter.js - Tiny vanilla client-side list filtering helper

(function() {
  function applyFilter(inputEl) {
    const group = inputEl.getAttribute('data-filter-input');
    if (!group) return;

    const query = (inputEl.value || '').trim().toLowerCase();
    const items = document.querySelectorAll(`[data-filter-item="${group}"]`);
    const emptyEl = document.querySelector(`[data-filter-empty="${group}"]`);
    const countEl = document.querySelector(`[data-filter-count="${group}"]`);

    let visibleCount = 0;

    items.forEach(function(item) {
      const filterText = (item.getAttribute('data-filter-text') || item.textContent || '').toLowerCase();
      const matches = query === '' || filterText.indexOf(query) !== -1;

      if (matches) {
        item.classList.remove('hidden');
        if (item.style.display === 'none') {
          item.style.display = '';
        }
        visibleCount++;
      } else {
        item.classList.add('hidden');
      }
    });

    if (emptyEl) {
      if (visibleCount === 0) {
        emptyEl.classList.remove('hidden');
        if (emptyEl.style.display === 'none') {
          emptyEl.style.display = '';
        }
      } else {
        emptyEl.classList.add('hidden');
      }
    }

    if (countEl) {
      countEl.textContent = String(visibleCount);
    }
  }

  function initFilters() {
    const inputs = document.querySelectorAll('[data-filter-input]');
    inputs.forEach(function(input) {
      if (input._filterAttached) return;
      input._filterAttached = true;

      input.addEventListener('input', function() {
        applyFilter(input);
      });

      // Run once on init to sync initial state
      if (input.value) {
        applyFilter(input);
      }
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initFilters);
  } else {
    initFilters();
  }

  // Also support dynamic element additions via window.initFilter
  window.initFilter = initFilters;
  window.applyFilter = applyFilter;
})();
