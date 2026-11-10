// The look the page is drawn in: the one the viewer chose in the footer,
// kept in this browser, or the system's while they chose none. It runs in
// the page's head, before anything is drawn, so that the page never shows
// the other look first, and again when the system's look changes, or the
// choice does, in the footer or in another tab. style.css keys on
// data-theme, and the bar a phone draws above the page takes the look's
// colour.
(() => {
  const root = document.documentElement;
  const system = matchMedia("(prefers-color-scheme: light)");
  const set = () => {
    let chosen = "";
    try {
      chosen = localStorage.getItem("overlook.theme") ?? "";
    } catch {
      // A browser that keeps nothing, such as in a private window, follows
      // the system.
    }
    const theme = chosen === "light" || chosen === "dark" ? chosen : system.matches ? "light" : "dark";
    root.dataset.theme = theme;
    for (const meta of document.querySelectorAll('meta[name="theme-color"]')) {
      meta.media = meta.dataset.theme === theme ? "all" : "not all";
    }
  };
  set();
  system.addEventListener("change", set);
  addEventListener("storage", (e) => {
    if (e.key === "overlook.theme") set();
  });
})();
