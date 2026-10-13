<script lang="ts">
  import { Events } from "@wailsio/runtime";
  import Icon from "./Icon.svelte";
  import { cancelRecording, startRecording, stopRecording, voice } from "../voice.svelte";

  let { enabled = false }: { enabled?: boolean } = $props();

  // Hold to talk; a short click toggles recording instead.
  const HOLD_MS = 350;
  let pressedAt = 0;
  let startedByPress = false;

  function onpointerdown(e: PointerEvent) {
    if (e.button !== 0) return;
    if (!enabled) {
      Events.Emit("navigate", "settings");
      return;
    }
    if (voice.state === "recording") {
      startedByPress = false;
      return; // second click of the toggle mode: stop on pointerup
    }
    if (voice.state !== "idle") return;
    pressedAt = Date.now();
    startedByPress = true;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    startRecording();
  }

  function onpointerup() {
    if (!enabled || voice.state !== "recording") return;
    if (startedByPress && Date.now() - pressedAt < HOLD_MS) {
      startedByPress = false; // short click: keep recording until the next click
      return;
    }
    stopRecording();
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === "Escape" && voice.state !== "idle") {
      cancelRecording();
      e.preventDefault();
    }
  }

  const title = $derived(
    !enabled
      ? "Spracheingabe in den Einstellungen einrichten"
      : voice.state === "recording"
        ? "Loslassen oder klicken zum Beenden (Esc bricht ab)"
        : voice.state === "transcribing"
          ? "Wird transkribiert …"
          : "Gedrückt halten zum Sprechen – oder klicken",
  );
</script>

<svelte:window {onkeydown} />

<button
  class="relative grid size-8 shrink-0 place-items-center rounded-lg transition-colors duration-150
    {voice.state === 'recording'
    ? 'bg-danger text-white'
    : voice.state === 'transcribing'
      ? 'text-accent'
      : enabled
        ? 'text-fg-muted hover:bg-field-hover hover:text-accent'
        : 'text-fg-subtle hover:bg-field-hover'}"
  aria-label="Spracheingabe"
  aria-pressed={voice.state === "recording"}
  {title}
  {onpointerdown}
  {onpointerup}
  oncontextmenu={(e) => e.preventDefault()}
>
  {#if voice.state === "recording"}
    <!-- Ring grows with the input level so users see the microphone hears them. -->
    <span
      class="pointer-events-none absolute inset-0 rounded-lg bg-danger/40 transition-transform duration-75"
      style="transform: scale({1 + Math.min(voice.level * 6, 0.6)})"
    ></span>
  {/if}
  {#if voice.state === "transcribing"}
    <span class="size-4 animate-spin rounded-full border-2 border-current border-r-transparent"></span>
  {:else}
    <span class="relative"><Icon name="mic" size={17} /></span>
  {/if}
</button>
