<script lang="ts">
  import {
    AppService,
    SettingsService,
    type ConnectionResult,
    type Info,
    type Settings,
  } from "../../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
  import Button from "../components/Button.svelte";
  import Icon from "../components/Icon.svelte";
  import StatusBanner from "../components/StatusBanner.svelte";
  import TextField from "../components/TextField.svelte";
  import Toggle from "../components/Toggle.svelte";
  import HotkeyRecorder from "../components/HotkeyRecorder.svelte";
  import { ShellService } from "../../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
  import { shell } from "../shell.svelte";
  import BrandLogo from "../brand/BrandLogo.svelte";
  import VoiceSettingsSection from "../components/VoiceSettingsSection.svelte";
  import AssistantSettingsSection from "../components/AssistantSettingsSection.svelte";
  import { BRAND } from "../brand/brand";
  import { connection, errorMessage, refreshConnection } from "../connection.svelte";

  const TOKEN_DOCS = "https://www.kimai.org/documentation/rest-api.html";
  const LANGUAGES = [
    ["de", "Deutsch"],
    ["en", "English"],
    ["fr", "Français"],
    ["es", "Español"],
    ["it", "Italiano"],
    ["nl", "Nederlands"],
  ] as const;

  let { onShowLegal }: { onShowLegal: () => void } = $props();

  let settings = $state<Settings | null>(null);
  let info = $state<Info | null>(null);

  let url = $state("");
  let token = $state("");
  let language = $state("de");
  let showToken = $state(false);

  let testing = $state(false);
  let saving = $state(false);
  let result = $state<ConnectionResult | null>(null);
  let error = $state("");
  let keyringFailure = $state("");
  // Only shown while the keyring is the selected storage.
  const keyringProblem = $derived(
    keyringFailure || (settings?.tokenStorage !== "file" ? (settings?.keyringError ?? "") : ""),
  );

  const dirty = $derived(
    settings !== null &&
      (url.trim() !== settings.kimaiUrl || token.trim() !== "" || language !== settings.language),
  );

  load();

  async function load() {
    try {
      settings = await SettingsService.GetSettings();
      url = settings.kimaiUrl;
      language = settings.language || "de";
      info = await AppService.Info();
      // Show the state of the stored connection right away.
      const r = connection.result;
      if (r && r.status !== "not_configured" && r.status !== "keyring") result = r;
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function test() {
    error = "";
    testing = true;
    try {
      result = await SettingsService.TestConnection(url, token);
    } catch (e) {
      error = errorMessage(e);
    } finally {
      testing = false;
    }
  }

  /** Saves the form. tokenStorage switches between keyring and file ("" keeps it). */
  async function save(tokenStorage = "") {
    error = "";
    keyringFailure = "";
    saving = true;
    try {
      const res = await SettingsService.SaveSettings({ kimaiUrl: url, token, language, tokenStorage });
      settings = res.settings;
      url = settings.kimaiUrl;
      if (res.keyringFailed) {
        // Keep the typed token so the user can choose the file fallback.
        keyringFailure = res.message ?? "";
        result = null;
        return;
      }
      if (res.message) error = res.message;
      token = "";
      showToken = false;
      result = await refreshConnection();
    } catch (e) {
      error = errorMessage(e);
    } finally {
      saving = false;
    }
  }

  async function removeToken() {
    error = "";
    try {
      await SettingsService.RemoveToken();
      settings = await SettingsService.GetSettings();
      result = null;
      await refreshConnection();
    } catch (e) {
      error = errorMessage(e);
    }
  }

  let shellError = $state("");
  let copied = $state(false);

  async function shellCall(fn: () => Promise<unknown>) {
    shellError = "";
    try {
      const res = await fn();
      if (res && typeof res === "object" && "hotkey" in res) shell.status = res as typeof shell.status;
    } catch (e) {
      shellError = errorMessage(e);
    }
  }

  async function copyCommand(cmd: string) {
    try {
      await navigator.clipboard.writeText(cmd);
      copied = true;
      setTimeout(() => (copied = false), 1500);
    } catch {
      /* clipboard unavailable */
    }
  }

  const hotkeyState = $derived.by(() => {
    const h = shell.status?.hotkey;
    if (!h || !h.shortcut) return { tone: "bg-fg-subtle", text: "Deaktiviert" };
    if (h.active) return { tone: "bg-success", text: h.backend === "portal" ? "Aktiv über Desktop-Portal" : "Aktiv" };
    if (h.pending) return { tone: "bg-warning", text: "Warte auf Bestätigung im Systemdialog" };
    return { tone: "bg-danger", text: "Nicht verfügbar" };
  });

  const sessionLabel = (t: string) => (t === "wayland" ? "Wayland" : t === "x11" ? "X11" : "–");
  const platformLabel = (p: string) => ({ linux: "Linux", windows: "Windows", darwin: "macOS" })[p] ?? (p || "–");
  const group =
    "overflow-hidden rounded-[10px] bg-field shadow-[inset_0_0_0_0.5px_var(--hairline)] [&>*+*]:border-t [&>*+*]:border-hairline";
  const heading = "mb-1.5 px-1 text-[11px] font-medium tracking-wide text-fg-muted uppercase";
</script>

<div class="h-full overflow-y-auto px-4 pt-1 pb-4">
  <h2 class={heading}>Kimai</h2>
  <section class={group}>
    <TextField label="Server" type="url" placeholder="https://kimai.example.com" bind:value={url} />
    <TextField
      label="API-Token"
      type={showToken ? "text" : "password"}
      placeholder={settings?.hasToken ? "••••••••••••••••" : "Token einfügen"}
      bind:value={token}
    >
      {#snippet trailing()}
        {#if !token && settings?.hasToken}
          <span
            class="flex shrink-0 items-center gap-1 rounded-full bg-success/15 px-2 py-0.5 text-[11px] font-medium text-success"
            title={settings.tokenStorage === "file" ? "In Datei gespeichert" : "Im Schlüsselbund gespeichert"}
          >
            <Icon name="check" size={10} stroke={3} />
            gespeichert
          </span>
        {/if}
        {#if token}
          <button
            class="no-drag -mr-1 grid size-6 place-items-center rounded text-fg-subtle hover:text-fg"
            aria-label={showToken ? "Token verbergen" : "Token anzeigen"}
            onclick={(e) => {
              e.preventDefault();
              showToken = !showToken;
            }}
          >
            <Icon name={showToken ? "eyeOff" : "eye"} size={14} />
          </button>
        {/if}
      {/snippet}
    </TextField>
  </section>

  <div class="mt-1.5 flex items-center justify-between px-1 text-[11px] text-fg-muted">
    <span>Kimai → Profil → API-Zugang</span>
    <div class="flex gap-3">
      {#if settings?.hasToken}
        <button class="text-danger hover:opacity-75" onclick={removeToken}>Token entfernen</button>
      {/if}
      <button class="flex items-center gap-1 text-accent hover:opacity-75" onclick={() => AppService.OpenURL(TOKEN_DOCS)}>
        Hilfe <Icon name="external" size={11} stroke={2} />
      </button>
    </div>
  </div>

  <div class="mt-3 flex gap-2">
    <Button onclick={test} busy={testing} disabled={!url.trim() || (!token.trim() && !settings?.hasToken)}>
      Verbindung testen
    </Button>
    <Button variant="primary" onclick={() => save()} busy={saving} disabled={!dirty}>Speichern</Button>
  </div>

  {#if keyringProblem}
    <div class="mt-3 rounded-[10px] bg-danger/12 px-3 py-2.5 text-[12px] leading-snug">
      <div class="font-medium">
        {keyringFailure ? "Token konnte nicht im Schlüsselbund gespeichert werden." : "Der Schlüsselbund ist nicht verfügbar."}
      </div>
      <details class="mt-1 text-[11px] text-fg-muted">
        <summary class="cursor-pointer">Details</summary>
        <div class="mt-1 break-words select-text">{keyringProblem}</div>
      </details>
      {#if token.trim()}
        <button class="mt-2 font-medium text-accent hover:opacity-75" onclick={() => save("file")}>
          Stattdessen unverschlüsselt in Datei speichern
        </button>
      {:else}
        <div class="mt-2 text-fg-muted">Token eingeben, um ihn stattdessen in einer Datei zu speichern.</div>
      {/if}
    </div>
  {/if}

  {#if settings?.tokenStorage === "file"}
    <div class="mt-3 rounded-[10px] bg-warning/15 px-3 py-2.5 text-[12px] leading-snug">
      Der Token liegt unverschlüsselt (nur für dich lesbar) in
      <span class="break-all text-fg-muted select-text">{settings.secretsPath}</span>.
      {#if settings.hasToken}
        <button class="mt-1 block font-medium text-accent hover:opacity-75" onclick={() => save("keyring")}>
          In den Schlüsselbund verschieben
        </button>
      {/if}
    </div>
  {/if}

  {#if error}
    <div class="mt-3 rounded-[10px] bg-danger/12 px-3 py-2.5 text-[12px] leading-snug">{error}</div>
  {:else if result && result.status !== "keyring"}
    {#key result.checkedAt}
      <div class="mt-3"><StatusBanner {result} /></div>
    {/key}
  {/if}

  <h2 class="mt-5 {heading}">Allgemein</h2>
  <section class={group}>
    <label class="flex h-10 items-center justify-between gap-2 px-3">
      <span>Sprache</span>
      <select
        bind:value={language}
        class="cursor-pointer appearance-none bg-transparent pr-1 text-right text-fg-muted outline-none"
      >
        {#each LANGUAGES as [code, name]}
          <option value={code}>{name}</option>
        {/each}
      </select>
    </label>
  </section>
  <p class="mt-1.5 px-1 text-[11px] text-fg-muted">Wird für die Spracheingabe verwendet.</p>

  <h2 class="mt-5 {heading}">Spracheingabe</h2>
  <VoiceSettingsSection />

  <h2 class="mt-5 {heading}">Stichpunkte (lokale KI)</h2>
  <AssistantSettingsSection />

  <h2 class="mt-5 {heading}">Hintergrund</h2>
  <section class={group}>
    <div class="flex h-10 items-center justify-between gap-2 px-3">
      <span>Beim Anmelden starten</span>
      <Toggle
        label="Beim Anmelden starten"
        checked={shell.status?.autostart ?? false}
        disabled={!shell.status?.autostartSupported}
        onchange={(v) => shellCall(() => ShellService.SetAutostart(v))}
      />
    </div>
    <div class="flex h-10 items-center justify-between gap-2 px-3">
      <span>Benachrichtigungen</span>
      <div class="flex items-center gap-2">
        <button class="text-[11px] text-accent hover:opacity-75" onclick={() => ShellService.TestNotification()}>Testen</button>
        <Toggle
          label="Benachrichtigungen"
          checked={shell.status?.notifications ?? true}
          onchange={(v) => shellCall(() => ShellService.SetNotifications(v))}
        />
      </div>
    </div>
    <div class="flex h-10 items-center justify-between gap-2 px-3">
      <span class="shrink-0">Ein-/Ausblenden</span>
      <HotkeyRecorder
        value={shell.status?.hotkey.shortcut ?? ""}
        onchange={(acc) => shellCall(() => ShellService.SetHotkey(acc))}
      />
    </div>
  </section>
  <div class="mt-1.5 flex items-center gap-1.5 px-1 text-[11px] text-fg-muted">
    <span class="size-1.5 shrink-0 rounded-full {hotkeyState.tone}"></span>
    <span>{hotkeyState.text}{shell.status?.hotkey.trigger ? ` (${shell.status.hotkey.trigger})` : ""}</span>
  </div>
  {#if shell.status?.hotkey.message && !shell.status.hotkey.active}
    <div class="mt-2 rounded-[10px] bg-warning/15 px-3 py-2.5 text-[12px] leading-snug">
      {shell.status.hotkey.message}
      <button
        class="mt-1.5 block rounded bg-field px-1.5 py-0.5 font-mono text-[11px] select-text hover:bg-field-hover"
        title="Kopieren"
        onclick={() => copyCommand(shell.status!.toggleCommand)}
      >
        {shell.status.toggleCommand}
      </button>
      <span class="text-[11px] text-fg-muted">{copied ? "Kopiert ✓" : "Start/Stopp: " + shell.status.timerCommand}</span>
    </div>
  {/if}
  {#if shell.status && !shell.status.trayAvailable}
    <div class="mt-2 rounded-[10px] bg-warning/15 px-3 py-2.5 text-[12px] leading-snug">
      Kein Tray verfügbar – Schließen minimiert das Fenster. Unter GNOME die Erweiterung
      „AppIndicator and KStatusNotifierItem Support“ aktivieren.
    </div>
  {/if}
  {#if shellError}
    <div class="mt-2 rounded-[10px] bg-danger/12 px-3 py-2.5 text-[12px] leading-snug">{shellError}</div>
  {/if}

  <h2 class="mt-5 {heading}">Info</h2>
  <section class={group}>
    {#each [["Version", info?.version ?? "–"], ["Plattform", platformLabel(info?.platform ?? "")], ...(info?.platform === "linux" ? [["Sitzung", sessionLabel(info?.sessionType ?? "")]] : [])] as [k, v]}
      <div class="flex h-9 items-center justify-between px-3">
        <span>{k}</span>
        <span class="text-fg-muted">{v}</span>
      </div>
    {/each}
    <div class="flex h-9 items-center justify-between gap-3 px-3">
      <span class="shrink-0">Konfiguration</span>
      <span class="truncate text-fg-muted" title={settings?.configPath}>{settings?.configPath ?? "–"}</span>
    </div>
  </section>

  <h2 class="mt-5 {heading}">Über</h2>
  <section class="rounded-[10px] bg-field px-3 py-3 text-center shadow-[inset_0_0_0_0.5px_var(--hairline)]">
    <button class="mx-auto block transition-opacity hover:opacity-80" title={BRAND.website} onclick={() => AppService.OpenURL(BRAND.website)}>
      <BrandLogo height={32} />
    </button>
    <p class="mt-2 text-[12px] text-fg-muted">
      Kimai Tracker {info?.version ?? ""} von {BRAND.company}.<br />Für Einzelpersonen kostenlos, für Teams und Whitelabel lizenzierbar.
    </p>
    <div class="mt-2 flex justify-center gap-4 text-[12px]">
      <button class="flex items-center gap-1 text-accent hover:opacity-75" onclick={() => AppService.OpenURL(BRAND.website)}>
        {BRAND.websiteLabel} <Icon name="external" size={11} stroke={2} />
      </button>
      <button class="flex items-center gap-1 text-accent hover:opacity-75" onclick={() => AppService.OpenURL(BRAND.repository)}>
        Quellcode <Icon name="external" size={11} stroke={2} />
      </button>
      <button class="text-accent hover:opacity-75" onclick={onShowLegal}>Lizenz</button>
    </div>
    <p class="mt-2 text-[10px] leading-snug text-fg-subtle">
      Kein offizielles Produkt des Kimai-Projekts.
    </p>
  </section>

  <button
    class="mt-4 w-full rounded-[10px] bg-field py-2 text-[13px] text-danger shadow-[inset_0_0_0_0.5px_var(--hairline)] transition-colors hover:bg-field-hover"
    onclick={() => ShellService.Quit()}
  >
    Kimai Tracker beenden
  </button>
</div>
