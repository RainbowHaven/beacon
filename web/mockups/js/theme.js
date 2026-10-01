(function () {
  const key = "beacon-mockup-theme";
  const root = document.documentElement;
  const saved = localStorage.getItem(key);
  const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
  root.setAttribute("data-theme", saved || (prefersDark ? "dark" : "light"));

  document.querySelectorAll("[data-theme-toggle]").forEach(function (el) {
    el.checked = root.getAttribute("data-theme") === "dark";
    el.addEventListener("change", function () {
      const theme = el.checked ? "dark" : "light";
      root.setAttribute("data-theme", theme);
      localStorage.setItem(key, theme);
    });
  });
})();
