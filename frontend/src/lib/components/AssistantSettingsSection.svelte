<script lang="ts">
  import { Events } from "@wailsio/runtime";
  import {
    AssistantService,
    type AssistantDownload,
    type AssistantSettings,
  } from "../../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
  import Icon from "./Icon.svelte";
  import Toggle from "./Toggle.svelte";
  import { errorMessage } from "../connection.svelte";
  import { assistant } from "../assistant.svelte";

  let settings = $state<AssistantSettings | null>(null);
  let download = $state<AssistantDownload | null>(null);
  let message = $state<{ ok: boolean; text: string } | null>(null);
  let testing = $state(false);

  load();

  async function load() {
    try {
      apply(await AssistantService.GetSettings());
    } catch {
      /* plain browser */
    }
  }

  function apply(s: AssistantSettings) {
    settings = s;
    assistant.settings = s;
  }

  async function save(patch: Partial<{ enabled: boolean; autoDictation: boolean; model: string; server: string }>) {
    if (!settings) return;
    message = null;
    try {
      apply(
        await AssistantService.SaveSettings({
          enabled: settings.enabled,
          autoDictation: settings.autoDictation,
          model: settings.model,
          server: settings.server,
          ...patch,
        }),
      );
    } catch (e) {
      message = { ok: false, text: errorMessage(e) };
    }
  }

  Events.On("assistant:download", async (ev) => {
    const p = ev.data as AssistantDownload;
    download = p.finished ? null : p;
    if (!p.finished) return;
    await load();
    message = p.error
      ? { ok: false, text: p.error }
      : { ok: true, text: p.kind === "runtime" ? "llama.cpp installiert." : "Modell geladen." };
  });

  async function start(fn: () => Promise<void>, kind: string, id: string) {
    message = null;
    try {
      await fn();
      download = { kind, id, done: 0, total: 0, finished: false };
    } catch (e) {
      message = { ok: false, text: errorMessage(e) };
    }
  }

  async function test() {
    testing = true;
    message = null;
    try {
      const out = await AssistantService.Summarize(
        "Heute Vormittag mit dem Kunden telefoniert, die Anforderungen für das neue Kontaktformular besprochen und danach direkt ein Angebot geschrieben und verschickt.",
      );
      message = { ok: true, text: out };
    } catch (e) {
      message = { ok: false, text: errorMessage(e) };
    } finally {
      testing = false;
    }
  }

  const pct = (p: AssistantDownload) => (p.total > 0 ? Math.round((p.done / p.total) * 100) : 0);
  const group =
    "overflow-hidden rounded-[10px] bg-field shadow-[inset_0_0_0_0.5px_var(--hairline)] [&>*+*]:border-t [&>*+*]:border-hairline";
</script>

<section class={group}>
  <div class="flex h-10 items-center justify-between gap-2 px-3">
    <span>Stichpunkte mit lokaler KI</span>
    <Toggle label="Lokale KI" checked={settings?.enabled ?? false} onchange={(v) => save({ enabled: v })} />
  </div>
  {#if settings?.enabled}
    <div class="flex h-10 items-center justify-between gap-2 px-3">
      <span>Diktate automatisch umwandeln</span>
      <Toggle label="Diktate automatisch umwandeln" checked={settings.autoDictation} onchange={(v) => save({ autoDictation: v })} />
    </div>
  {/if}
</section>

{#if settings?.enabled}
  <div class="mt-3 mb-1.5 px-1 text-[11px] font-medium text-fg-muted">Laufzeit</div>
  <section class={group}>
    <div class="flex min-h-11 items-center gap-2 px-3 py-1.5">
      <div class="min-w-0 flex-1">
        <div class="text-[13px]">llama.cpp</div>
        {#if download?.kind === "runtime"}
          <div class="mt-1 h-1 overflow-hidden rounded-full bg-fg/10">
            <div class="h-full rounded-full bg-accent transition-[width]" style="width:{pct(download)}%"></div>
          </div>
        {:else}
          <div class="truncate text-[11px] text-fg-muted" title={settings.server}>{settings.server || "Nicht installiert (≈ 30 MB)"}</div>
        {/if}
      </div>
      {#if download?.kind === "runtime"}
        <span class="text-[11px] text-fg-muted tabular-nums">{pct(download)} %</span>
        <button class="text-[12px] text-danger hover:opacity-75" onclick={() => AssistantService.CancelDownload()}>Abbrechen</button>
      {:else if settings.server}
        <span class="flex items-center gap-1 text-[12px] font-medium text-success"><Icon name="check" size={12} stroke={2.6} /> Bereit</span>
      {:else}
        <button class="text-[12px] text-accent hover:opacity-75 disabled:opacity-40" disabled={!!download}
          onclick={() => start(() => AssistantService.InstallRuntime(), "runtime", "llama.cpp")}>Laden</button>
      {/if}
    </div>
  </section>

  <div class="mt-3 mb-1.5 px-1 text-[11px] font-medium text-fg-muted">Modell</div>
  <section class={group}>
    {#each settings.models as m (m.id)}
      {@const selected = m.installed && settings.model === m.path}
      <div class="flex min-h-11 items-center gap-2 px-3 py-1.5">
        <div class="min-w-0 flex-1">
          <div class="text-[13px]">{m.name} <span class="text-[11px] text-fg-subtle">· {(m.sizeMb / 1000).toFixed(1).replace(".", ",")} GB</span></div>
          {#if download?.kind === "model" && download.id === m.id}
            <div class="mt-1 h-1 overflow-hidden rounded-full bg-fg/10">
              <div class="h-full rounded-full bg-accent transition-[width]" style="width:{pct(download)}%"></div>
            </div>
          {:else}
            <div class="truncate text-[11px] text-fg-muted" title={`Lizenz des Modells: ${m.license}`}>{m.description} · {m.license}</div>
          {/if}
        </div>
        {#if download?.kind === "model" && download.id === m.id}
          <span class="text-[11px] text-fg-muted tabular-nums">{pct(download)} %</span>
          <button class="text-[12px] text-danger hover:opacity-75" onclick={() => AssistantService.CancelDownload()}>Abbrechen</button>
        {:else if selected}
          <span class="flex items-center gap-1 text-[12px] font-medium text-success"><Icon name="check" size={12} stroke={2.6} /> Aktiv</span>
        {:else if m.installed}
          <button class="text-[12px] text-accent hover:opacity-75" onclick={() => save({ model: m.path })}>Verwenden</button>
        {:else}
          <button class="text-[12px] text-accent hover:opacity-75 disabled:opacity-40" disabled={!!download}
            onclick={() => start(() => AssistantService.DownloadModel(m.id), "model", m.id)}>Laden</button>
        {/if}
      </div>
    {/each}
  </section>
  <p class="mt-1.5 px-1 text-[11px] leading-snug text-fg-muted">
    Läuft komplett auf diesem Rechner, nichts verlässt das Gerät. Das Modell wird nur bei Bedarf geladen und nach
    5 Minuten Pause wieder aus dem Speicher entfernt. Der Knopf ✨ neben dem Mikrofon wandelt die Beschreibung um.
  </p>

  <button
    class="mt-3 w-full rounded-lg bg-field py-1.5 text-[13px] font-medium shadow-[inset_0_0_0_0.5px_var(--hairline)] transition-colors hover:bg-field-hover disabled:opacity-45"
    disabled={!settings.ready || testing}
    onclick={test}
  >
    {testing ? "Modell wird geladen …" : "Testen"}
  </button>
{/if}

{#if message}
  <div class="mt-2 rounded-[10px] px-3 py-2 text-[12px] leading-snug whitespace-pre-line {message.ok ? 'bg-success/12' : 'bg-danger/12'}">
    {message.text}
  </div>
{/if}
