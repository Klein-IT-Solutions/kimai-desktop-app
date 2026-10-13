<script lang="ts">
  import { onDestroy, untrack } from "svelte";
  import {
    EntriesService,
    type DayEntries,
    type Entry,
    type Option,
    type Selection,
  } from "../../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
  import SelectionRows from "../components/SelectionRows.svelte";
  import AssistantNotices from "../components/AssistantNotices.svelte";
  import DescriptionField from "../components/DescriptionField.svelte";
  import Button from "../components/Button.svelte";
  import TimeInput from "../components/TimeInput.svelte";
  import { errorMessage } from "../connection.svelte";
  import { tracker } from "../tracker.svelte";
  import { voiceEnabled } from "../voiceSettings.svelte";
  import { claimDescription } from "../descriptionTarget";
  import { clearUndo } from "../assistant.svelte";
  import { clockMinutes, formatHours, minutesClock } from "../format";

  let {
    entry,
    day,
    onClose,
  }: {
    /** Entry to change; null adds a new one to `day`. */
    entry: Entry | null;
    day: DayEntries | null;
    /** Called after saving or deleting (with the entry's date) or on cancel (no date). */
    onClose: (date?: string) => void;
  } = $props();

  const nowClock = () => {
    const d = new Date();
    return minutesClock(d.getHours() * 60 + d.getMinutes());
  };

  // New entries continue after the day's last entry and end now (today) or an hour later.
  function defaults() {
    const finished = (day?.entries ?? []).filter((e) => !e.running && e.end);
    const begin = finished.length ? finished[finished.length - 1].end : "09:00";
    const b = clockMinutes(begin);
    const now = day?.today ? clockMinutes(day.now) : NaN;
    const end = now > b ? minutesClock(now) : minutesClock(b + 60);
    return { begin, end };
  }

  // The editor is recreated for every entry, so the props are read once.
  const initial = untrack(() => entry);
  const d = initial ? null : untrack(defaults);
  const empty: Selection = { id: 0, name: "" };

  let customer = $state<Selection>(initial ? { ...initial.customer } : { ...tracker.state.customer });
  let project = $state<Selection>(initial ? { ...initial.project } : { ...tracker.state.project });
  let activity = $state<Selection>(initial ? { ...initial.activity } : { ...tracker.state.activity });
  let date = $state(initial?.date ?? untrack(() => day?.date) ?? "");
  let begin = $state(initial?.begin ?? d!.begin);
  let end = $state(initial ? initial.end : d!.end);
  let description = $state(initial?.description ?? "");
  const running = !!initial?.running;
  let stopping = $state(false); // running entry gets an end time

  let saving = $state(false);
  let error = $state("");
  let confirmDelete = $state(false);
  let pickerOpen = $state(false);

  // Dictation and the ✨ button work on this entry while the editor is open.
  const release = claimDescription({ get: () => description, set: (t) => (description = t) });
  onDestroy(() => {
    release();
    clearUndo();
  });

  const duration = $derived.by(() => {
    const b = clockMinutes(begin);
    const e = running && !stopping ? clockMinutes(nowClock()) : clockMinutes(end);
    return e > b ? (e - b) * 60 : 0;
  });

  function onSelect(kind: "customer" | "project" | "activity", o: Option) {
    const sel = { id: o.id, name: o.name };
    if (kind === "customer") {
      if (sel.id !== customer.id) project = activity = { ...empty };
      customer = sel;
    } else if (kind === "project") {
      if (sel.id !== project.id) activity = { ...empty };
      project = sel;
    } else activity = sel;
  }

  function stopNow() {
    stopping = true;
    end = nowClock();
  }

  async function save() {
    if (saving) return;
    error = "";
    saving = true;
    try {
      await EntriesService.Save({
        id: initial?.id ?? 0,
        date,
        begin,
        end: running && !stopping ? "" : end,
        customer,
        project,
        activity,
        description,
      });
      onClose(date);
    } catch (e) {
      error = errorMessage(e);
    } finally {
      saving = false;
    }
  }

  async function remove() {
    if (!initial) return;
    if (!confirmDelete) {
      confirmDelete = true;
      setTimeout(() => (confirmDelete = false), 4000);
      return;
    }
    saving = true;
    try {
      await EntriesService.Delete(initial.id);
      onClose(date);
    } catch (e) {
      error = errorMessage(e);
      saving = false;
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && !pickerOpen) {
      e.preventDefault();
      save();
    } else if (e.key === "Escape" && !pickerOpen) {
      onClose();
    }
  }

  const input =
    "bg-transparent text-[13px] text-fg tabular-nums outline-none [color-scheme:light] dark:[color-scheme:dark] disabled:text-fg-subtle";
</script>

<svelte:window onkeydown={onKey} />

<div class="relative flex h-full flex-col px-4 pt-1 pb-3">
  <SelectionRows {customer} {project} {activity} bind:open={pickerOpen} onchange={onSelect} />

  <section class="mt-2.5 overflow-hidden rounded-[10px] bg-field shadow-[inset_0_0_0_0.5px_var(--hairline)] [&>*+*]:border-t [&>*+*]:border-hairline">
    <label class="flex h-10 items-center gap-2.5 px-3">
      <span class="w-[70px] shrink-0">Datum</span>
      <input type="date" class="{input} min-w-0 flex-1 text-right" bind:value={date} disabled={running} required />
    </label>
    <div class="flex h-10 items-center gap-2.5 px-3">
      <span class="w-[70px] shrink-0">Zeit</span>
      <TimeInput label="Von" bind:value={begin} />
      <span class="text-fg-subtle">–</span>
      {#if running && !stopping}
        <span class="text-[13px] text-success">läuft</span>
        <button class="text-[12px] font-medium text-accent hover:opacity-75" onclick={stopNow}>Beenden</button>
      {:else}
        <TimeInput label="Bis" bind:value={end} />
      {/if}
      <span class="ml-auto text-[12px] text-fg-muted tabular-nums">{formatHours(duration)} h</span>
    </div>
  </section>

  <div class="mt-2.5 flex min-h-0 flex-1 flex-col justify-end">
    {#if error}
      <div class="mb-2 rounded-[10px] bg-danger/12 px-3 py-2 text-[12px] leading-snug">{error}</div>
    {/if}
    <AssistantNotices />
    <DescriptionField value={description} oninput={(t) => (description = t)} micAvailable={voiceEnabled.value} />
  </div>

  <div class="mt-2.5 flex gap-2">
    {#if initial}
      <button
        class="h-8 rounded-lg px-3 text-[13px] font-medium transition-colors {confirmDelete ? 'bg-danger text-white' : 'text-danger hover:bg-danger/10'}"
        disabled={saving}
        onclick={remove}
      >
        {confirmDelete ? "Wirklich löschen?" : "Löschen"}
      </button>
    {/if}
    <Button onclick={() => onClose()}>Abbrechen</Button>
    <Button variant="primary" busy={saving} disabled={!project.id || !activity.id || !begin || (!end && !(running && !stopping))} onclick={save}>
      Sichern
    </Button>
  </div>
</div>
