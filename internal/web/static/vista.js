// Cambia de rol o de pestaña sin recargar: trae la página, reemplaza #vista y deja el scroll donde estaba, con una
// transición suave donde el navegador la soporta. Sin JavaScript, los enlaces [data-swap] navegan como siempre.
let path = location.pathname;

async function swap(url, push) {
  let doc;
  try {
    const res = await fetch(url);
    if (!res.ok) throw new Error(res.status);
    doc = new DOMParser().parseFromString(await res.text(), "text/html");
  } catch {
    location.href = url;
    return;
  }
  const next = doc.getElementById("vista");
  if (!next) {
    location.href = url;
    return;
  }
  if (push) history.pushState(null, "", url);
  path = location.pathname;
  const update = () => {
    document.getElementById("vista").replaceWith(next);
    document.title = doc.title;
    cintas(next);
  };
  if (document.startViewTransition && !reduce) document.startViewTransition(update);
  else update();
}

document.addEventListener("click", (e) => {
  const a = e.target.closest("a[data-swap]");
  if (!a || e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
  e.preventDefault();
  if (a.pathname !== location.pathname) swap(a.href, true);
});

// Atrás y adelante entre vistas; los cambios de #ancla (los paneles de detalle) no cambian la ruta y se ignoran.
addEventListener("popstate", () => {
  if (location.pathname !== path) swap(location.href, false);
});
