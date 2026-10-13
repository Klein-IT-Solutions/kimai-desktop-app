/** Formats a duration in seconds as H:MM:SS (or HH:MM:SS from ten hours on). */
export function formatDuration(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  return `${h}:${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
}

/** Formats a duration in seconds as H:MM (hours and minutes). */
export function formatHours(totalSeconds: number): string {
  const m = Math.max(0, Math.round(totalSeconds / 60));
  return `${Math.floor(m / 60)}:${String(m % 60).padStart(2, "0")}`;
}

/** Minutes since midnight of "HH:MM", or NaN. */
export function clockMinutes(clock: string): number {
  const m = /^(\d{1,2}):(\d{2})$/.exec(clock.trim());
  return m ? Number(m[1]) * 60 + Number(m[2]) : NaN;
}

/** "HH:MM" for minutes since midnight (clamped to the day). */
export function minutesClock(minutes: number): string {
  const m = Math.min(23 * 60 + 59, Math.max(0, Math.round(minutes)));
  return `${String(Math.floor(m / 60)).padStart(2, "0")}:${String(m % 60).padStart(2, "0")}`;
}

/** Moves a YYYY-MM-DD date by `days`. */
export function addDays(date: string, days: number): string {
  const d = new Date(`${date}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

/** "Heute", "Gestern" or e.g. "Mo., 12. Okt." for a YYYY-MM-DD date. */
export function dayLabel(date: string, today: string): string {
  if (date === today) return "Heute";
  if (date === addDays(today, -1)) return "Gestern";
  const d = new Date(`${date}T12:00:00Z`);
  return d.toLocaleDateString("de-DE", { weekday: "short", day: "numeric", month: "short", timeZone: "UTC" });
}
