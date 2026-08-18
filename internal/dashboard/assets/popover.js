(function () {
  'use strict';

  function getContent(root) {
    if (!root) return null;
    return root.querySelector(':scope > [data-tui-popover-content]') || root.querySelector('[data-tui-popover-content]');
  }

  function getTrigger(root) {
    if (!root) return null;
    return root.querySelector(':scope > [data-tui-popover-trigger]') || root.querySelector('[data-tui-popover-trigger]');
  }

  function isOpen(root) {
    const content = getContent(root);
    if (!content) return false;
    return !content.classList.contains('hidden') && content.getAttribute('data-tui-popover-state') === 'open';
  }

  function position(root) {
    const trigger = getTrigger(root);
    const content = getContent(root);
    if (!trigger || !content) return;

    if (!content.classList.contains('absolute') && !content.classList.contains('fixed')) {
      content.classList.add('absolute', 'top-full', 'left-0', 'mt-1.5');
    }
  }

  function openPopover(root) {
    const content = getContent(root);
    if (!content) return;

    // Close any other open popovers
    document.querySelectorAll('[data-tui-popover-root]').forEach(function (other) {
      if (other !== root && isOpen(other)) {
        closePopover(other);
      }
    });

    position(root);
    content.classList.remove('hidden');
    content.setAttribute('data-tui-popover-state', 'open');
    content.setAttribute('data-tui-popover-open', 'true');
    const trigger = getTrigger(root);
    if (trigger) trigger.setAttribute('data-tui-popover-open', 'true');
  }

  function closePopover(root) {
    const content = getContent(root);
    if (!content) return;

    content.classList.add('hidden');
    content.setAttribute('data-tui-popover-state', 'closed');
    content.setAttribute('data-tui-popover-open', 'false');
    const trigger = getTrigger(root);
    if (trigger) trigger.setAttribute('data-tui-popover-open', 'false');
  }

  function togglePopover(root) {
    if (isOpen(root)) {
      closePopover(root);
    } else {
      openPopover(root);
    }
  }

  // Handle clicks on triggers and outside
  document.addEventListener('click', function (e) {
    const trigger = e.target.closest('[data-tui-popover-trigger]');
    if (trigger) {
      const root = trigger.closest('[data-tui-popover-root]');
      if (root) {
        e.preventDefault();
        e.stopPropagation();
        togglePopover(root);
        return;
      }
    }

    // Do not close if clicking inside popover content (e.g. calendar days, month selects)
    const inContent = e.target.closest('[data-tui-popover-content]');
    if (inContent) {
      return;
    }

    // Close open popovers when clicking outside
    document.querySelectorAll('[data-tui-popover-root]').forEach(function (root) {
      if (isOpen(root) && !root.contains(e.target)) {
        closePopover(root);
      }
    });
  });

  // Handle ESC key
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') {
      document.querySelectorAll('[data-tui-popover-root]').forEach(function (root) {
        if (isOpen(root)) {
          closePopover(root);
        }
      });
    }
  });

  window.tui = window.tui || {};
  window.tui.popover = {
    open: function (id) {
      const el = document.getElementById(id);
      if (el) openPopover(el.matches('[data-tui-popover-root]') ? el : el.closest('[data-tui-popover-root]'));
    },
    close: function (id) {
      const el = document.getElementById(id);
      if (el) closePopover(el.matches('[data-tui-popover-root]') ? el : el.closest('[data-tui-popover-root]'));
    },
    toggle: function (id) {
      const el = document.getElementById(id);
      if (el) togglePopover(el.matches('[data-tui-popover-root]') ? el : el.closest('[data-tui-popover-root]'));
    },
    closeAll: function () {
      document.querySelectorAll('[data-tui-popover-root]').forEach(closePopover);
    },
    isOpen: function (id) {
      const el = document.getElementById(id);
      return el ? isOpen(el.matches('[data-tui-popover-root]') ? el : el.closest('[data-tui-popover-root]')) : false;
    }
  };
})();
