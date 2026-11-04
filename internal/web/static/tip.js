// Tooltips de las siglas (tipBox): al mostrarse, los corre para que no se salgan de la pantalla y los pone arriba
// si abajo no caben; Esc los cierra. Sin este archivo se ven igual, sin esos ajustes.
document.documentElement.classList.add("tip-js");
const margin = 8;
function place(tip) {
  const box = tip.querySelector(":scope > .tip-box");
  if (!box) return;
  delete tip.dataset.tipOff;
  tip.style.removeProperty("--tip-dx");
  tip.classList.remove("tip-up");
  const r = box.getBoundingClientRect();
  let dx = 0;
  if (r.left < margin) dx = margin - r.left;
  else if (r.right > innerWidth - margin) dx = innerWidth - margin - r.right;
  if (dx) tip.style.setProperty("--tip-dx", `${Math.round(dx)}px`);
  if (r.bottom > innerHeight - margin && tip.getBoundingClientRect().top > r.height + 2 * margin) tip.classList.add("tip-up");
}
for (const type of ["mouseover", "focusin"]) {
  document.addEventListener(type, (e) => {
    const tip = e.target.closest?.(".tip") ?? e.target.querySelector?.(":scope .tip");
    if (tip && (type === "focusin" || !tip.contains(e.relatedTarget))) place(tip);
  });
}
document.addEventListener("keydown", (e) => {
  if (e.key !== "Escape") return;
  for (const tip of document.querySelectorAll(".tip:hover, .tip:focus")) tip.dataset.tipOff = "";
});
