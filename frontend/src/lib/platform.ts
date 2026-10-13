import {
  AppService,
  type Info,
} from "../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
import type { WindowControls } from "../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/desktop";

export type { WindowControls };

/** Best guess before the backend answered (or in a plain browser). */
function guessControls(): WindowControls {
  const ua = navigator.userAgent;
  if (/Mac/.test(ua)) return { style: "mac", side: "left", buttons: ["close", "minimize"] };
  if (/Windows/.test(ua)) return { style: "windows", side: "right", buttons: ["minimize", "close"] };
  return { style: "gnome", side: "right", buttons: ["minimize", "close"] };
}

export const platform: { info: Info | null; controls: WindowControls } = {
  info: null,
  controls: guessControls(),
};

/** Loads platform details before the first render so the title bar doesn't jump. */
export async function initPlatform() {
  try {
    platform.info = await AppService.Info();
    platform.controls = platform.info.windowControls;
  } catch {
    /* plain browser during development */
  }
  document.documentElement.dataset.wc = platform.controls.style;
}
