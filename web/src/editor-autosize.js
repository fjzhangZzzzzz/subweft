// Measure rendered lines, including wrapping, rather than reserving empty rows.
export function fitTextareas(root = document) {
  for (const input of root.querySelectorAll(".drawer textarea")) {
    if (!input.getClientRects().length) continue;
    input.rows = 1;
    input.style.height = "auto";
    const style = getComputedStyle(input);
    const border =
      parseFloat(style.borderTopWidth) + parseFloat(style.borderBottomWidth);
    input.style.height = `${Math.ceil(input.scrollHeight + border)}px`;
  }
}

const schedule = () => requestAnimationFrame(() => fitTextareas());
document.addEventListener("input", (event) => {
  if (event.target.matches(".drawer textarea")) fitTextareas();
});
// Collapsed sections cannot be measured until opened.
document.addEventListener("toggle", schedule, true);
window.addEventListener("resize", schedule);
