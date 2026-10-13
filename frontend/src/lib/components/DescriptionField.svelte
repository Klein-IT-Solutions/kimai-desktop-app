<script lang="ts">
  import MicButton from "./MicButton.svelte";
  import { voice } from "../voice.svelte";
  import Icon from "./Icon.svelte";
  import { assistant, assistantReady, summarizeDescription } from "../assistant.svelte";

  let {
    value = "",
    oninput,
    status = "",
    micAvailable = false,
  }: {
    value?: string;
    oninput: (text: string) => void;
    status?: "" | "saving" | "saved" | "error";
    micAvailable?: boolean;
  } = $props();
</script>

<div
  class="relative flex items-start gap-2 rounded-[10px] bg-field p-1.5 pl-3 shadow-[inset_0_0_0_0.5px_var(--hairline)] transition-colors focus-within:bg-field-hover"
>
  <textarea
    {value}
    oninput={(e) => oninput(e.currentTarget.value)}
    rows={value.includes("\n") ? 4 : 2}
    placeholder={voice.state === "recording" ? "Ich höre zu …" : "Woran arbeitest du?"}
    class="min-h-[38px] flex-1 resize-none bg-transparent py-1 text-[13px] leading-snug text-fg outline-none placeholder:text-fg-subtle"
  ></textarea>
  <div class="flex shrink-0 flex-col items-center gap-0.5">
    <MicButton enabled={micAvailable} />
    {#if assistantReady()}
      <button
        class="grid size-8 place-items-center rounded-lg transition-colors disabled:opacity-40
          {assistant.busy ? 'text-accent' : 'text-fg-muted hover:bg-field-hover hover:text-accent'}"
        title="In Stichpunkte umwandeln (lokale KI)"
        aria-label="In Stichpunkte umwandeln"
        disabled={assistant.busy || !value.trim()}
        onclick={summarizeDescription}
      >
        {#if assistant.busy}
          <span class="size-4 animate-spin rounded-full border-2 border-current border-r-transparent"></span>
        {:else}
          <Icon name="sparkles" size={16} />
        {/if}
      </button>
    {/if}
  </div>
  {#if assistant.busy}
    <span class="absolute right-12 bottom-1.5 text-[10px] text-accent">Formuliere Stichpunkte …</span>
  {:else if voice.state !== "idle"}
    <span class="absolute right-12 bottom-1.5 text-[10px] {voice.state === 'recording' ? 'text-danger' : 'text-fg-subtle'}">
      {voice.state === "recording" ? "Aufnahme …" : "Transkribiere …"}
    </span>
  {:else if status}
    <span
      class="absolute right-12 bottom-1.5 text-[10px] transition-opacity
        {status === 'error' ? 'text-danger' : status === 'saved' ? 'text-success' : 'text-fg-subtle'}"
    >
      {status === "saving" ? "Speichert …" : status === "saved" ? "In Kimai gespeichert" : "Nicht gespeichert"}
    </span>
  {/if}
</div>
