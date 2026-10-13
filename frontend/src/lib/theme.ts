import { Events } from "@wailsio/runtime";
import { AppService } from "../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";

function apply(dark: boolean) {
  document.documentElement.classList.toggle("dark", dark);
}

/**
 * Follows the system colour scheme. The CSS media query is the baseline;
 * on Linux the backend additionally reads the XDG desktop portal, because
 * WebKitGTK does not always propagate the GNOME/KDE dark-mode preference.
 */
export function initTheme() {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  apply(media.matches);
  media.addEventListener("change", (e) => apply(e.matches));

  AppService.IsDarkMode()
    .then((dark) => {
      if (dark) apply(true);
    })
    .catch(() => {
      /* running in a plain browser during development */
    });

  Events.On("theme:changed", (ev) => apply(Boolean(ev.data)));
}
