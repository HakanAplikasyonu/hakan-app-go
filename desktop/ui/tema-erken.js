// İlk boyamadan önce tema (#325): klasik, eşzamanlı betik (CSP 'self' izin verir;
// satır içi betik yasak). Ayrıntılı mantık ve değişiklik dinleme tema.js'te.
(function () {
  var p = "m3", k = "system";
  try {
    p = localStorage.getItem("hakan.palet") === "n3" ? "n3" : "m3";
    k = localStorage.getItem("hakan.kip") || "system";
  } catch (e) {}
  var koyu = k === "dark" || (k !== "light" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.setAttribute("data-theme", p + "-" + (koyu ? "dark" : "light"));
})();
