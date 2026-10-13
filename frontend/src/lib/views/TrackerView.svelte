<script lang="ts">
  import { onDestroy } from "svelte";
  import { slide } from "svelte/transition";
  import type { Option } from "../../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
  import SelectionRows from "../components/SelectionRows.svelte";
  import AssistantNotices from "../components/AssistantNotices.svelte";
  import TimerButton from "../components/TimerButton.svelte";
  import DescriptionField from "../components/DescriptionField.svelte";
  import { connection } from "../connection.svelte";
  import { voiceEnabled } from "../voiceSettings.svelte";
  import {
    canStart,
    selectActivity,
    selectCustomer,
    selectProject,
    setDescription,
    toggleTimer,
    tracker,
  } from "../tracker.svelte";

  let { onOpenSettings, onEditRunning }: { onOpenSettings: () => void; onEditRunning: () => void } = $props();

  let now = $state(Date.now());
  const tick = setInterval(() => (now = Date.now()), 250);
  onDestroy(() => clearInterval(tick));

  const s = $derived(tracker.state);
  const connected = $derived(connection.result?.ok ?? false);
  const elapsed = $derived(s.running && s.begin ? Math.max(0, (now - Date.parse(s.begin)) / 1000) : 0);
  const locked = $derived(s.running || tracker.busy);
  let pickerOpen = $state(false);

  function onSelect(kind: "customer" | "project" | "activity", o: Option) {
    if (kind === "customer") selectCustomer(o);
    else if (kind === "project") selectProject(o);
    else selectActivity(o);
  }

  function onKey(e: KeyboardEvent) {
    // Ctrl/Cmd+Enter starts or stops the timer from anywhere in the window.
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && !pickerOpen) {
      if (s.running || (canStart() && connected)) toggleTimer();
      e.preventDefault();
    }
  }

  const status = $derived.by(() => {
    const r = connection.result;
    if (connection.checking && !r) return { tone: "bg-fg-subtle", text: "Verbindung wird geprüft …" };
    if (!r || r.status === "not_configured") return { tone: "bg-warning", text: "Kimai verbinden" };
    if (r.ok) return { tone: "bg-success", text: `${r.userName} · ${host(r.baseUrl)}` };
    if (r.status === "offline") return { tone: "bg-danger", text: "Offline – Kimai nicht erreichbar" };
    if (r.status === "unauthorized") return { tone: "bg-danger", text: "API-Token ungültig" };
    if (r.status === "keyring") return { tone: "bg-danger", text: "Schlüsselbund nicht verfügbar" };
    return { tone: "bg-danger", text: "Verbindungsproblem" };
  });

  function host(url: string) {
    try {
      return new URL(url).host;
    } catch {
      return url;
    }
  }

  const hint = $derived.by(() => {
    if (s.running) return "";
    if (!connected) return "";
    if (!s.customer.id) return "Kunde auswählen";
    if (!s.project.id) return "Projekt auswählen";
    if (!s.activity.id) return "Tätigkeit auswählen";
    return "";
  });
</script>

<svelte:window onkeydown={onKey} />

<div class="relative flex h-full flex-col px-4 pt-1 pb-3">
  <div class="transition-opacity {s.running ? 'opacity-80' : ''}">
    <SelectionRows customer={s.customer} project={s.project} activity={s.activity} disabled={locked} bind:open={pickerOpen} onchange={onSelect} />
  </div>

  <div class="flex flex-1 flex-col items-center justify-center gap-2 py-2">
    <TimerButton
      running={s.running}
      {elapsed}
      disabled={tracker.busy || (!s.running && (!canStart() || !connected))}
      ontoggle={toggleTimer}
    />
    <div class="h-4 text-[11px] text-fg-subtle">
      {#if tracker.busy}
        {s.running ? "Wird gestoppt …" : "Wird gestartet …"}
      {:else if s.running && s.begin}
        <button class="underline-offset-2 hover:text-fg-muted hover:underline" title="Startzeit, Projekt oder Beschreibung ändern" onclick={onEditRunning}>
          seit {new Date(s.begin).toLocaleTimeString("de-DE", { hour: "2-digit", minute: "2-digit" })} Uhr ✎
        </button>
      {:else}
        {hint}
      {/if}
    </div>
  </div>

  {#if tracker.error}
    <button
      transition:slide={{ duration: 180 }}
      class="mb-2 rounded-[10px] bg-danger/12 px-3 py-2 text-left text-[12px] leading-snug"
      title="Ausblenden"
      onclick={() => (tracker.error = "")}
    >
      {tracker.error}
    </button>
  {/if}

  <AssistantNotices />

  <DescriptionField
    value={s.description}
    oninput={setDescription}
    status={s.running ? tracker.descriptionStatus : ""}
    micAvailable={voiceEnabled.value}
  />

  <button
    class="mt-2.5 flex min-w-0 items-center justify-center gap-1.5 text-[11px] text-fg-subtle transition-colors hover:text-fg-muted"
    onclick={onOpenSettings}
  >
    <span class="size-1.5 shrink-0 rounded-full transition-colors {status.tone}"></span>
    <span class="truncate">{status.text}</span>
  </button>

</div>
