<script lang="ts">
  import { Events } from "@wailsio/runtime";
  import {
    AppService,
    VoiceService,
    type DownloadProgress,
    type ModelStatus,
    type VoiceSettings,
  } from "../../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
  import Button from "./Button.svelte";
  import HotkeyRecorder from "./HotkeyRecorder.svelte";
  import Icon from "./Icon.svelte";
  import TextField from "./TextField.svelte";
  import { errorMessage } from "../connection.svelte";
  import { voiceEnabled } from "../voiceSettings.svelte";

  const MODELS_DOCS = "https://github.com/ggml-org/whisper.cpp#quick-start";
  const ENGINES = [
    ["", "Aus"],
    ["local", "Lokal"],
    ["api", "API"],
  ] as const;

  let settings = $state<VoiceSettings | null>(null);
  let engine = $state("");
  let whisperBinary = $state("");
  let whisperModel = $state("");
  let apiUrl = $state("");
  let apiModel = $state("");
  let apiKey = $state("");
  let pushToTalk = $state("");

  let models = $state<ModelStatus[]>([]);
  let download = $state<DownloadProgress | null>(null);
  let showCustomModel = $state(false);
  let showExternal = $state(false);

  let busy = $state(false);
  let message = $state<{ ok: boolean; text: string } | null>(null);

  const dirty = $derived(
    settings !== null &&
      (engine !== settings.engine ||
        whisperBinary !== settings.whisperBinary ||
        whisperModel !== settings.whisperModel ||
        apiUrl !== settings.apiUrl ||
        apiModel !== settings.apiModel ||
        apiKey.trim() !== "" ||
        pushToTalk !== settings.pushToTalk.shortcut),
  );

  load();

  async function load() {
    try {
      apply(await VoiceService.GetSettings());
      models = (await VoiceService.Models()) ?? [];
    } catch {
      /* plain browser */
    }
  }

  Events.On("voice:download", async (ev) => {
    const p = ev.data as DownloadProgress;
    download = p;
    if (!p.finished) return;
    models = (await VoiceService.Models()) ?? [];
    if (p.error) {
      message = { ok: false, text: p.error };
    } else {
      const m = models.find((x) => x.id === p.id);
      if (m) whisperModel = m.path;
      if (settings) settings.whisperModel = whisperModel;
      message = { ok: true, text: `Modell „${m?.name ?? p.id}“ geladen.` };
    }
    download = null;
  });

  async function startDownload(id: string) {
    message = null;
    try {
      await VoiceService.DownloadModel(id);
      download = { id, done: 0, total: 0, finished: false };
    } catch (e) {
      message = { ok: false, text: errorMessage(e) };
    }
  }

  async function detectBinary() {
    const p = await VoiceService.DetectBinary().catch(() => "");
    if (p) whisperBinary = p;
    else message = { ok: false, text: "Kein externes whisper-cli gefunden – die eingebaute Version wird verwendet." };
  }

  const pct = (p: DownloadProgress) => (p.total > 0 ? Math.round((p.done / p.total) * 100) : 0);

  function apply(s: VoiceSettings) {
    settings = s;
    engine = s.engine;
    whisperBinary = s.whisperBinary;
    whisperModel = s.whisperModel;
    apiUrl = s.apiUrl;
    apiModel = s.apiModel;
    pushToTalk = s.pushToTalk.shortcut;
    apiKey = "";
    voiceEnabled.value = s.engine !== "";
  }

  async function save() {
    busy = true;
    message = null;
    try {
      apply(await VoiceService.SaveSettings({ engine, whisperBinary, whisperModel, apiUrl, apiModel, apiKey, pushToTalk }));
      if (engine) await check();
    } catch (e) {
      message = { ok: false, text: errorMessage(e) };
    } finally {
      busy = false;
    }
  }

  async function check() {
    try {
      message = { ok: true, text: await VoiceService.CheckSettings() };
    } catch (e) {
      message = { ok: false, text: errorMessage(e) };
    }
  }

  async function browse(kind: "binary" | "model") {
    const path = await VoiceService.BrowseFile(kind === "binary" ? "whisper-cli auswählen" : "Whisper-Modell (ggml-*.bin) auswählen").catch(() => "");
    if (!path) return;
    if (kind === "binary") whisperBinary = path;
    else whisperModel = path;
  }

  const ptt = $derived.by(() => {
    const h = settings?.pushToTalk;
    if (!settings?.engine) return { tone: "bg-fg-subtle", text: "Aktiv, sobald eine Transkription gewählt ist" };
    if (!h?.shortcut) return { tone: "bg-fg-subtle", text: "Kein Tastenkürzel" };
    if (h.active) return { tone: "bg-success", text: "Aktiv – gedrückt halten, sprechen, loslassen" };
    if (h.pending) return { tone: "bg-warning", text: "Warte auf Bestätigung im Systemdialog" };
    return { tone: "bg-danger", text: h.message || "Nicht verfügbar" };
  });

  const group =
    "overflow-hidden rounded-[10px] bg-field shadow-[inset_0_0_0_0.5px_var(--hairline)] [&>*+*]:border-t [&>*+*]:border-hairline";
</script>

<!-- Engine -->
<div class="flex rounded-[9px] bg-field p-0.5 shadow-[inset_0_0_0_0.5px_var(--hairline)]">
  {#each ENGINES as [value, label]}
    <button
      class="flex-1 rounded-[7px] py-1 text-[12px] transition-all duration-150
        {engine === value ? 'bg-[var(--surface)] font-medium text-fg shadow-[0_1px_3px_rgba(0,0,0,0.12)]' : 'text-fg-muted hover:text-fg'}"
      onclick={() => (engine = value)}
    >
      {label}
    </button>
  {/each}
</div>

{#if engine === "local"}
  <div class="mt-2 mb-1.5 px-1 text-[11px] font-medium text-fg-muted">Modell</div>
  <section class={group}>
    {#each models as m (m.id)}
      {@const selected = m.installed && whisperModel === m.path}
      <div class="flex min-h-11 items-center gap-2 px-3 py-1.5">
        <div class="min-w-0 flex-1">
          <div class="text-[13px]">{m.name} <span class="text-[11px] text-fg-subtle">· {m.sizeMb} MB</span></div>
          {#if download && download.id === m.id}
            <div class="mt-1 h-1 overflow-hidden rounded-full bg-fg/10">
              <div class="h-full rounded-full bg-accent transition-[width] duration-200" style="width:{pct(download)}%"></div>
            </div>
          {:else}
            <div class="truncate text-[11px] text-fg-muted">{m.description}</div>
          {/if}
        </div>
        {#if download && download.id === m.id}
          <span class="text-[11px] text-fg-muted tabular-nums">{pct(download)} %</span>
          <button class="text-[12px] text-danger hover:opacity-75" onclick={() => VoiceService.CancelDownload()}>Abbrechen</button>
        {:else if selected}
          <span class="flex items-center gap-1 text-[12px] font-medium text-success"><Icon name="check" size={12} stroke={2.6} /> Aktiv</span>
        {:else if m.installed}
          <button class="text-[12px] text-accent hover:opacity-75" onclick={() => (whisperModel = m.path)}>Verwenden</button>
        {:else}
          <button class="text-[12px] text-accent hover:opacity-75 disabled:opacity-40" disabled={!!download} onclick={() => startDownload(m.id)}>Laden</button>
        {/if}
      </div>
    {/each}
  </section>
  <button class="mt-1.5 px-1 text-[11px] text-fg-muted hover:text-fg" onclick={() => (showCustomModel = !showCustomModel)}>
    {showCustomModel ? "▾" : "▸"} Eigene Modelldatei
  </button>
  {#if showCustomModel || (whisperModel && !models.some((m) => m.path === whisperModel))}
    <section class="mt-1.5 {group}">
      <TextField label="Datei" placeholder="~/models/ggml-small.bin" bind:value={whisperModel}>
        {#snippet trailing()}
          <button class="no-drag shrink-0 rounded px-1.5 text-fg-subtle hover:text-fg" title="Durchsuchen" onclick={() => browse("model")}>…</button>
        {/snippet}
      </TextField>
    </section>
  {/if}
  <button class="mt-1 block px-1 text-[11px] text-fg-muted hover:text-fg" onclick={() => (showExternal = !showExternal)}>
    {showExternal || whisperBinary ? "▾" : "▸"} Externes whisper-cli verwenden
  </button>
  {#if showExternal || whisperBinary}
    <section class="mt-1.5 {group}">
      <TextField label="Programm" placeholder="Leer = eingebaut" bind:value={whisperBinary}>
        {#snippet trailing()}
          {#if !whisperBinary}
            <button class="no-drag shrink-0 text-[12px] text-accent hover:opacity-75" onclick={detectBinary}>Suchen</button>
          {/if}
          <button class="no-drag shrink-0 rounded px-1.5 text-fg-subtle hover:text-fg" title="Durchsuchen" onclick={() => browse("binary")}>…</button>
        {/snippet}
      </TextField>
    </section>
  {/if}
  <p class="mt-1.5 px-1 text-[11px] leading-snug text-fg-muted">
    Läuft komplett offline – whisper.cpp ist in die App eingebaut, nur das Modell wird einmalig geladen
    (von Hugging Face).
  </p>
{:else if engine === "api"}
  <section class="mt-2 {group}">
    <TextField label="URL" type="url" placeholder="https://api.openai.com/v1" bind:value={apiUrl} />
    <TextField label="Modell" placeholder="whisper-1" bind:value={apiModel} />
    <TextField label="API-Key" type="password" placeholder={settings?.hasApiKey ? "••••••••••••" : "Optional"} bind:value={apiKey}>
      {#snippet trailing()}
        {#if !apiKey && settings?.hasApiKey}
          <span class="flex shrink-0 items-center gap-1 rounded-full bg-success/15 px-2 py-0.5 text-[11px] font-medium text-success">
            <Icon name="check" size={10} stroke={3} /> gespeichert
          </span>
        {/if}
      {/snippet}
    </TextField>
  </section>
  <p class="mt-1.5 px-1 text-[11px] leading-snug text-fg-muted">
    OpenAI oder ein kompatibler Dienst (z. B. Groq, ein eigener faster-whisper-Server). Die Aufnahme wird dorthin gesendet.
  </p>
{/if}

{#if engine}
  <section class="mt-2 {group}">
    <div class="flex h-10 items-center justify-between gap-2 px-3">
      <span class="shrink-0">Push-to-talk</span>
      <HotkeyRecorder value={pushToTalk} onchange={(acc) => (pushToTalk = acc)} />
    </div>
  </section>
  <div class="mt-1.5 flex items-start gap-1.5 px-1 text-[11px] leading-snug text-fg-muted">
    <span class="mt-1 size-1.5 shrink-0 rounded-full {ptt.tone}"></span>
    <span>{ptt.text}</span>
  </div>
{/if}

<div class="mt-3 flex gap-2">
  <Button onclick={check} disabled={!settings?.engine || dirty}>Prüfen</Button>
  <Button variant="primary" onclick={save} {busy} disabled={!dirty}>Speichern</Button>
</div>

{#if message}
  <div class="mt-2 rounded-[10px] px-3 py-2 text-[12px] leading-snug {message.ok ? 'bg-success/12' : 'bg-danger/12'}">
    {message.text}
  </div>
{/if}
