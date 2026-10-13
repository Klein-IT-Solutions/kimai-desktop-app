<script lang="ts">
  import { onDestroy, untrack } from "svelte";
  import { Events } from "@wailsio/runtime";
  import { EntriesService, type DayEntries, type Entry } from "../../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
  import Icon from "../components/Icon.svelte";
  import { errorMessage } from "../connection.svelte";
  import { addDays, dayLabel, formatHours } from "../format";

  let {
    date = $bindable(""),
    onEdit,
    onAdd,
  }: {
    /** Shown day (YYYY-MM-DD); empty means today. */
    date?: string;
    onEdit: (entry: Entry) => void;
    onAdd: (day: DayEntries) => void;
  } = $props();

  let day = $state<DayEntries | null>(null);
  let loading = $state(true);
  let error = $state("");
  let today = $state("");

  async function load() {
    loading = true;
    error = "";
    try {
      const d = await EntriesService.Day(date);
      day = d;
      if (d.today) today = d.date;
      if (!date) date = d.date;
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }

  // Load when the day changes (not again when `date` is filled in after the first load).
  $effect(() => {
    const d = date;
    untrack(() => {
      if (!day || day.date !== d) load();
    });
  });

  // Started or stopped elsewhere (tray, hotkey, Kimai web UI).
  const off = Events.On("tracker:changed", () => load());
  onDestroy(() => off());

  const go = (days: number) => (date = addDays(date || today, days));
  const entries = $derived(day?.entries ?? []);
</script>

<div class="flex h-full flex-col px-4 pt-1 pb-3">
  <div class="flex h-9 items-center justify-between">
    <button class="grid size-7 place-items-center rounded-md text-fg-muted hover:bg-field-hover hover:text-fg" aria-label="Vorheriger Tag" onclick={() => go(-1)}>
      <Icon name="chevronLeft" size={16} />
    </button>
    <div class="text-center">
      <div class="text-[13px] font-semibold">{day ? dayLabel(day.date, today || day.date) : "…"}</div>
      <div class="text-[11px] text-fg-muted">
        {#if day}Gesamt {formatHours(day.total)} h{/if}
      </div>
    </div>
    <button
      class="grid size-7 place-items-center rounded-md text-fg-muted hover:bg-field-hover hover:text-fg disabled:opacity-30 disabled:hover:bg-transparent"
      aria-label="Nächster Tag"
      disabled={!!day?.today}
      onclick={() => go(1)}
    >
      <Icon name="chevronRight" size={16} />
    </button>
  </div>

  <div class="mt-2 min-h-0 flex-1 overflow-y-auto">
    {#if error}
      <div class="rounded-[10px] bg-danger/12 px-3 py-2.5 text-[12px] leading-snug">{error}</div>
    {:else if !loading && entries.length === 0}
      <div class="flex h-full flex-col items-center justify-center gap-1 text-center text-fg-muted">
        <Icon name="clock" size={28} stroke={1.4} />
        <p class="mt-1 text-[13px]">Keine Einträge an diesem Tag.</p>
        <p class="text-[11px] text-fg-subtle">Vergessen zu starten? Einfach nachtragen.</p>
      </div>
    {:else}
      <ul class="overflow-hidden rounded-[10px] bg-field shadow-[inset_0_0_0_0.5px_var(--hairline)] [&>*+*]:border-t [&>*+*]:border-hairline">
        {#each entries as entry (entry.id)}
          <li>
            <button class="block w-full px-3 py-2 text-left transition-colors hover:bg-field-hover" onclick={() => onEdit(entry)}>
              <div class="flex items-baseline justify-between gap-2">
                <span class="text-[12px] text-fg-muted tabular-nums">
                  {entry.begin} – {#if entry.running}<span class="text-success">läuft</span>{:else}{entry.end}{/if}
                </span>
                <span class="text-[12px] font-medium tabular-nums">{formatHours(entry.duration)}</span>
              </div>
              <div class="truncate text-[13px]">{entry.project.name} · {entry.activity.name}</div>
              <div class="truncate text-[11px] text-fg-subtle">
                {entry.customer.name}{#if entry.description}{" · "}{entry.description.replace(/\s*\n\s*/g, " ")}{/if}
              </div>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <button
    class="mt-3 flex h-9 items-center justify-center gap-1.5 rounded-[10px] bg-accent text-[13px] font-medium text-white shadow-[0_1px_2px_rgba(0,0,0,0.15)] transition hover:brightness-110 disabled:opacity-45"
    disabled={!day}
    onclick={() => day && onAdd(day)}
  >
    <Icon name="plus" size={15} stroke={2.2} /> Eintrag nachtragen
  </button>
</div>
