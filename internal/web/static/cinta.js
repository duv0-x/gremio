// Flechas de la cinta de atributos. Sin JavaScript la cinta se desliza igual con el dedo, el trackpad o el teclado.
// static/vista.js vuelve a llamar a cintas() cuando reemplaza la vista.
const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;
function cintas(root) {
  for (const cinta of root.querySelectorAll("[data-cinta]")) {
    const track = cinta.querySelector("[data-cinta-track]");
    for (const btn of cinta.querySelectorAll("[data-cinta-dir]")) {
      btn.hidden = false;
      btn.addEventListener("click", () =>
        track.scrollBy({ left: Number(btn.dataset.cintaDir) * track.clientWidth * 0.8, behavior: reduce ? "auto" : "smooth" }),
      );
    }
  }
}
cintas(document);
