// Marca que hay JavaScript: muestra lo que solo funciona con htmx (.js-only), como "Agregar otra fila".
document.documentElement.classList.add("js");

// Mientras dura una petición de htmx, el botón que la hizo queda aria-busy y muestra el spinner (DESIGN.md).
const busy = (e) => e.detail.ctx?.request?.submitter || e.target;
document.addEventListener("htmx:before:request", (e) => busy(e).setAttribute("aria-busy", "true"));
document.addEventListener("htmx:finally:request", (e) => busy(e).removeAttribute("aria-busy"));
