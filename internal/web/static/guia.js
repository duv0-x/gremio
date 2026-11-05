// Guía "¿Cómo leer esta ficha?" (gremio#61). Sin JavaScript es el <details id="guia"> con los pasos; con este
// archivo, su botón abre un recorrido que resalta cada zona [data-guia] y explica su paso en #guia-tarjeta. Se abre
// solo en la primera visita (localStorage). En un bloque, porque cinta.js ya declara `reduce` en el ámbito global.
{
  const seen = "gremio-guia";
  const still = matchMedia("(prefers-reduced-motion: reduce)");
  const mobile = matchMedia("(max-width: 639px)");
  const $ = (id) => document.getElementById(id);
  let tour = null; // { steps, i, els, back }

  function open(back) {
    const steps = [...document.querySelectorAll("#guia [data-guia-paso]")]
      .map((li) => ({ li, zone: document.querySelector(`[data-guia="${li.dataset.guiaPaso}"]`) }))
      .filter((s) => s.zone?.getClientRects().length);
    if (!steps.length) return;
    try {
      localStorage.setItem(seen, "1");
    } catch {}
    tour = { steps, i: 0, back, els: [$("guia-capa"), $("guia-foco"), $("guia-tarjeta")] };
    for (const el of tour.els) el.hidden = false;
    addEventListener("scroll", place, { passive: true });
    addEventListener("resize", place);
    show(0);
    $("guia-tarjeta").focus();
  }

  function close() {
    if (!tour) return;
    for (const el of tour.els) el.hidden = true;
    removeEventListener("scroll", place);
    removeEventListener("resize", place);
    const back = tour.back;
    tour = null;
    if (back?.isConnected) back.focus();
  }

  function show(i) {
    tour.i = i;
    const { li, zone } = tour.steps[i];
    const last = i === tour.steps.length - 1;
    const card = $("guia-tarjeta");
    $("guia-paso").textContent = `Paso ${i + 1} de ${tour.steps.length}`;
    $("guia-titulo").textContent = li.querySelector("h3").textContent;
    $("guia-texto").textContent = li.querySelector("p").textContent.replace(/\s+/g, " ").trim();
    card.querySelector('[data-guia-accion="atras"]').hidden = i === 0;
    const next = card.querySelector('[data-guia-accion="siguiente"]');
    next.textContent = last ? "Terminar" : "Siguiente";
    // Si el foco quedó en Atrás y Atrás se ocultó, pasa a Siguiente.
    if (card.contains(document.activeElement) && document.activeElement.hidden) next.focus();
    zone.style.scrollMarginTop = "24px";
    zone.scrollIntoView({ block: mobile.matches ? "start" : "center", behavior: still.matches ? "auto" : "smooth" });
    place();
  }

  // Pone el foco de luz sobre la zona y la tarjeta debajo, encima o, si la zona no deja lugar, abajo a la derecha.
  // En móvil la tarjeta va fija abajo (CSS).
  function place() {
    if (!tour) return;
    const pad = 8;
    const gap = 12;
    const margin = 16;
    const r = tour.steps[tour.i].zone.getBoundingClientRect();
    Object.assign($("guia-foco").style, {
      top: `${r.top - pad}px`,
      left: `${r.left - pad}px`,
      width: `${r.width + 2 * pad}px`,
      height: `${r.height + 2 * pad}px`,
    });
    const card = $("guia-tarjeta");
    if (mobile.matches) {
      card.style.top = card.style.left = "";
      return;
    }
    const { offsetWidth: w, offsetHeight: h } = card;
    let top = innerHeight - h - margin;
    if (innerHeight - r.bottom - pad - gap >= h + margin) top = r.bottom + pad + gap;
    else if (r.top - pad - gap >= h + margin) top = r.top - pad - gap - h;
    card.style.top = `${top}px`;
    card.style.left = `${Math.min(Math.max(r.left, margin), innerWidth - w - margin)}px`;
  }

  document.addEventListener("click", (e) => {
    const summary = e.target.closest("#guia > summary");
    if (summary) {
      e.preventDefault();
      open(summary);
      return;
    }
    const btn = e.target.closest("[data-guia-accion]");
    if (!btn || !tour) return;
    const action = btn.dataset.guiaAccion;
    if (action === "atras") show(tour.i - 1);
    else if (action === "siguiente" && tour.i < tour.steps.length - 1) show(tour.i + 1);
    else close();
  });

  // Esc cierra; Tab no sale de la tarjeta (aria-modal).
  document.addEventListener("keydown", (e) => {
    if (!tour) return;
    if (e.key === "Escape") {
      e.preventDefault();
      close();
      return;
    }
    if (e.key !== "Tab") return;
    const btns = [...$("guia-tarjeta").querySelectorAll("button:not([hidden])")];
    const first = btns[0];
    const lastBtn = btns[btns.length - 1];
    if (e.shiftKey && (document.activeElement === first || !btns.includes(document.activeElement))) {
      e.preventDefault();
      lastBtn.focus();
    } else if (!e.shiftKey && (document.activeElement === lastBtn || !btns.includes(document.activeElement))) {
      e.preventDefault();
      first.focus();
    }
  });

  // Atrás y adelante del navegador reemplazan #vista (static/vista.js), con la guía adentro.
  addEventListener("popstate", close);

  // Primera visita: espera a que carguen fuentes e imágenes para medir bien. Sin localStorage no se abre sola.
  const firstVisit = () => {
    try {
      if (localStorage.getItem(seen)) return;
    } catch {
      return;
    }
    const summary = document.querySelector("#guia > summary");
    if (summary) open(summary);
  };
  if (document.readyState === "complete") firstVisit();
  else addEventListener("load", firstVisit);
}
