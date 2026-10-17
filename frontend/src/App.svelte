<script lang="ts">
  import { Events } from "@wailsio/runtime";
  import { fly } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import { EntriesService, type DayEntries, type Entry } from "../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
  import TitleBar from "./lib/components/TitleBar.svelte";
  import TrackerView from "./lib/views/TrackerView.svelte";
  import SettingsView from "./lib/views/SettingsView.svelte";
  import EntriesView from "./lib/views/EntriesView.svelte";
  import EntryEditorView from "./lib/views/EntryEditorView.svelte";
  import LegalView from "./lib/views/LegalView.svelte";
  import { errorMessage } from "./lib/connection.svelte";
  import { tracker } from "./lib/tracker.svelte";

  type View = "tracker" | "settings" | "entries" | "entry" | "legal";
  // Depth decides the slide direction.
  const depth: Record<View, number> = { tracker: 0, settings: 1, entries: 1, entry: 2, legal: 2 };

  let view = $state<View>("tracker");
  let dir = $state<1 | -1>(1);
  // Entry editor: the entry (null = new) and the day it belongs to; where to return.
  let editing = $state<{ entry: Entry | null; day: DayEntries | null; back: View } | null>(null);
  let entriesDate = $state("");

  function show(next: View) {
    dir = depth[next] >= depth[view] ? 1 : -1;
    view = next;
  }

  // The tray menu can open the settings.
  Events.On("navigate", (ev) => {
    if (ev.data === "settings" || ev.data === "tracker") show(ev.data);
  });

  function editEntry(entry: Entry | null, day: DayEntries | null, back: View) {
    editing = { entry, day, back };
    show("entry");
  }

  // "seit 14:32 Uhr" on the timer opens its entry.
  async function editRunning() {
    try {
      const day = await EntriesService.Day("");
      const list = day.entries ?? [];
      const entry = list.find((e) => e.id === tracker.state.timesheetId) ?? list.find((e) => e.running);
      if (entry) editEntry(entry, day, "tracker");
    } catch (e) {
      tracker.error = errorMessage(e);
    }
  }

  function closeEditor(date?: string) {
    const back = editing?.back ?? "entries";
    if (date && back === "entries") entriesDate = date;
    show(back);
    editing = null;
  }

  const titles: Record<View, string> = { tracker: "Kimai Tracker", settings: "Einstellungen", entries: "Einträge", entry: "Eintrag", legal: "Lizenz" };
  const title = $derived(view === "entry" && editing && !editing.entry ? "Eintrag nachtragen" : titles[view]);

  const slide = (d: 1 | -1) => ({ x: 40 * d, duration: 260, easing: cubicOut, opacity: 0 });
</script>

<main class="window flex flex-col">
  <TitleBar
    {title}
    canGoBack={view === "settings" || view === "entries" || view === "legal"}
    showActions={view === "tracker"}
    onBack={() => show(view === "legal" ? "settings" : "tracker")}
    onSettings={() => show("settings")}
    onEntries={() => show("entries")}
  />

  <div class="relative min-h-0 flex-1">
    {#key view}
      <div class="absolute inset-0" in:fly={slide(dir)} out:fly={{ ...slide(dir === 1 ? -1 : 1), duration: 180 }}>
        {#if view === "tracker"}
          <TrackerView onOpenSettings={() => show("settings")} onEditRunning={editRunning} onOpenEntries={() => show("entries")} />
        {:else if view === "settings"}
          <SettingsView onShowLegal={() => show("legal")} />
        {:else if view === "entries"}
          <EntriesView
            bind:date={entriesDate}
            onEdit={(entry) => editEntry(entry, null, "entries")}
            onAdd={(day) => editEntry(null, day, "entries")}
          />
        {:else if view === "legal"}
          <LegalView />
        {:else if editing}
          <EntryEditorView entry={editing.entry} day={editing.day} onClose={closeEditor} />
        {/if}
      </div>
    {/key}
  </div>
</main>
