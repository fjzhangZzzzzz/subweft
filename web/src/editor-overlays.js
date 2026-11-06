const menus = () => [...document.querySelectorAll(".row-menu[open]")];
function closeMenu(menu, restore = false) {
  menu.open = false;
  const trigger = menu.querySelector("summary");
  trigger.setAttribute("aria-expanded", "false");
  if (restore) trigger.focus();
}
export function closeMenus() {
  menus().forEach((menu) => closeMenu(menu));
}
export function closeNavigation(restore = false) {
  document.querySelector("#sidebar")?.classList.remove("open");
  const trigger = document.querySelector('[data-action="nav-open"]');
  trigger?.setAttribute("aria-expanded", "false");
  if (restore) trigger?.focus();
}
export function openNavigation() {
  closeMenus();
  document.querySelector("#sidebar")?.classList.add("open");
  document
    .querySelector('[data-action="nav-open"]')
    ?.setAttribute("aria-expanded", "true");
  document.querySelector('#sidebar [data-action="nav-close"]')?.focus();
}
function dismissOutside(target) {
  menus().forEach((menu) => {
    if (!menu.contains(target)) closeMenu(menu);
  });
  const sidebar = document.querySelector("#sidebar.open");
  if (
    sidebar &&
    !sidebar.contains(target) &&
    !target.closest('[data-action="nav-open"]')
  )
    closeNavigation();
}
document.addEventListener("pointerdown", (event) =>
  dismissOutside(event.target),
);
document.addEventListener(
  "click",
  (event) => {
    dismissOutside(event.target);
    const menu = event.target.closest(".row-menu");
    if (!menu) return;
    if (event.target.closest("summary")) {
      menus().forEach((other) => {
        if (other !== menu) closeMenu(other);
      });
    } else if (event.target.closest("button:not(:disabled)")) {
      // Restore before opening a dialog so its return focus is the visible trigger.
      closeMenu(menu, true);
    }
  },
  true,
);
document.addEventListener(
  "toggle",
  (event) => {
    const menu = event.target;
    if (!menu.matches(".row-menu")) return;
    menu
      .querySelector("summary")
      .setAttribute("aria-expanded", String(menu.open));
    if (menu.open)
      menus().forEach((other) => {
        if (other !== menu) closeMenu(other);
      });
  },
  true,
);
document.addEventListener("focusin", (event) => dismissOutside(event.target));
document.addEventListener("focusout", () => {
  // Wait for the browser to finish moving focus before testing containment.
  setTimeout(() => dismissOutside(document.activeElement), 0);
});
window.addEventListener("blur", () => {
  closeMenus();
  closeNavigation();
});
document.addEventListener("keydown", (event) => {
  // Native dialogs own Escape while open, including unsaved form confirmation.
  if (document.querySelector("dialog[open]")) return;
  const menu = event.target.closest(".row-menu");
  if (event.key === "Escape") {
    const opened = menus();
    if (opened.length) {
      event.preventDefault();
      opened.forEach((item) =>
        closeMenu(item, item.contains(document.activeElement)),
      );
    } else if (document.querySelector("#sidebar.open")) {
      event.preventDefault();
      closeNavigation(true);
    }
    return;
  }
  if (!menu) return;
  const trigger = event.target.closest("summary");
  if (trigger && ["Enter", " ", "ArrowDown", "ArrowUp"].includes(event.key)) {
    event.preventDefault();
    closeMenus();
    menu.open = true;
    trigger.setAttribute("aria-expanded", "true");
    const items = menu.querySelectorAll("button:not(:disabled)");
    (event.key === "ArrowUp" ? items[items.length - 1] : items[0])?.focus();
  } else if (
    menu.open &&
    ["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)
  ) {
    event.preventDefault();
    const items = [...menu.querySelectorAll("button:not(:disabled)")];
    const index = items.indexOf(document.activeElement);
    const next =
      event.key === "Home"
        ? 0
        : event.key === "End"
          ? items.length - 1
          : (index + (event.key === "ArrowDown" ? 1 : -1) + items.length) %
            items.length;
    items[next]?.focus();
  }
});
