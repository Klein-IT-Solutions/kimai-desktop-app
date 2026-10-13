import { Events } from "@wailsio/runtime";
import {
  ShellService,
  type ShellStatus,
} from "../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";

/** Background features (tray, autostart, notifications, hotkey). */
export const shell = $state<{ status: ShellStatus | null }>({ status: null });

export function initShell() {
  ShellService.Status()
    .then((s) => (shell.status = s))
    .catch(() => {});
  Events.On("shell:changed", (ev) => (shell.status = ev.data as ShellStatus));
}
