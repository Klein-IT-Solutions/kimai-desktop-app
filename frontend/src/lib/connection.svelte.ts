import { Events } from "@wailsio/runtime";
import {
  SettingsService,
  type ConnectionResult,
} from "../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";

/** Reactive state of the stored Kimai connection, shared by all views. */
export const connection = $state<{ result: ConnectionResult | null; checking: boolean }>({
  result: null,
  checking: false,
});

export async function refreshConnection(): Promise<ConnectionResult | null> {
  connection.checking = true;
  try {
    connection.result = await SettingsService.CheckConnection();
  } catch {
    /* plain browser during development */
  } finally {
    connection.checking = false;
  }
  return connection.result;
}

export function initConnection() {
  Events.On("connection:changed", (ev) => {
    connection.result = ev.data as ConnectionResult;
  });
  // Network back (e.g. Wi-Fi reconnected): check right away instead of
  // waiting for the next background check, so saved entries are sent.
  window.addEventListener("online", () => {
    if (connection.result && !connection.result.ok && connection.result.status === "offline") refreshConnection();
  });
  connection.checking = true;
  SettingsService.LastConnection()
    .then((r) => (connection.result = r))
    .catch(() => {})
    .finally(() => (connection.checking = false));
}

/** Kimai is configured but cannot be reached; entries are kept locally. */
export const isOffline = () => connection.result?.status === "offline";

/** Extracts a readable message from a rejected backend call. */
export function errorMessage(e: unknown): string {
  if (e && typeof e === "object" && "message" in e) return String((e as { message: unknown }).message);
  return String(e);
}
