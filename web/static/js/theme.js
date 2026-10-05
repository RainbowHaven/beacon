(function () {
  const themeKey = "beacon_theme";
  const sidebarKey = "beacon_sidebar";
  const root = document.documentElement;

  function readCookie(name) {
    const parts = document.cookie.split(";");
    for (let i = 0; i < parts.length; i++) {
      const p = parts[i].trim();
      if (p.startsWith(name + "=")) {
        return decodeURIComponent(p.slice(name.length + 1));
      }
    }
    return "";
  }

  function writeCookie(name, value) {
    document.cookie = name + "=" + encodeURIComponent(value) + "; Path=/; Max-Age=31536000; SameSite=Lax";
  }

  function themeActionLabel(theme) {
    return theme === "dark" ? "Light mode" : "Dark mode";
  }

  function applyTheme(theme) {
    root.setAttribute("data-theme", theme);
    writeCookie(themeKey, theme);
    const next = themeActionLabel(theme);
    document.querySelectorAll("[data-theme-toggle]").forEach(function (el) {
      el.setAttribute("aria-pressed", theme === "dark" ? "true" : "false");
      el.setAttribute("aria-label", next);
      el.title = next;
    });
  }

  function applySidebar(collapsed) {
    if (collapsed) {
      root.setAttribute("data-sidebar", "collapsed");
    } else {
      root.removeAttribute("data-sidebar");
    }
    writeCookie(sidebarKey, collapsed ? "collapsed" : "expanded");
    document.querySelectorAll("[data-sidebar-toggle]").forEach(function (btn) {
      const label = collapsed ? "Expand sidebar" : "Collapse sidebar";
      btn.setAttribute("aria-expanded", collapsed ? "false" : "true");
      btn.setAttribute("aria-label", label);
      btn.title = label;
    });
  }

  const savedTheme = readCookie(themeKey);
  const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
  applyTheme(savedTheme === "dark" || savedTheme === "light" ? savedTheme : (prefersDark ? "dark" : "light"));
  applySidebar(readCookie(sidebarKey) === "collapsed");

  document.querySelectorAll("[data-theme-toggle]").forEach(function (el) {
    el.addEventListener("click", function () {
      applyTheme(root.getAttribute("data-theme") === "dark" ? "light" : "dark");
    });
  });

  document.querySelectorAll("[data-sidebar-toggle]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      const collapsed = root.getAttribute("data-sidebar") !== "collapsed";
      applySidebar(collapsed);
    });
  });

  document.querySelectorAll("form.filter-bar").forEach(function (form) {
    form.addEventListener("change", function () {
      if (typeof form.requestSubmit === "function") {
        form.requestSubmit();
      } else {
        form.submit();
      }
    });
  });

  document.querySelectorAll("[data-open-modal]").forEach(function (el) {
    el.addEventListener("click", function () {
      const id = el.getAttribute("data-open-modal");
      const dialog = id ? document.getElementById(id) : null;
      if (dialog && typeof dialog.showModal === "function") {
        dialog.showModal();
      }
    });
  });
})();
