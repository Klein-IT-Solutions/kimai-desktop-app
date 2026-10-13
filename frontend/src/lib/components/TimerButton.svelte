<script lang="ts">
  import Icon from "./Icon.svelte";
  import { formatDuration } from "../format";

  let {
    running,
    elapsed,
    disabled = false,
    ontoggle,
  }: { running: boolean; elapsed: number; disabled?: boolean; ontoggle: () => void } = $props();
</script>

<div class="flex flex-col items-center gap-4">
  <div
    class="tabular text-[44px] leading-none font-extralight tracking-[-0.02em] transition-colors duration-500 {running
      ? 'text-fg'
      : 'text-fg-subtle'}"
  >
    {formatDuration(elapsed)}
  </div>

  <button
    class="relative grid size-[76px] place-items-center rounded-full text-white transition-all duration-300 ease-apple active:scale-95 disabled:opacity-40 disabled:active:scale-100
      {running ? 'bg-danger shadow-[0_6px_20px_-4px_var(--danger)]' : 'bg-success shadow-[0_6px_20px_-4px_var(--success)]'}"
    aria-label={running ? "Stoppen" : "Starten"}
    {disabled}
    onclick={ontoggle}
  >
    {#if running}
      <!-- The ring grows beyond the button; it must not catch clicks. -->
      <span class="pointer-events-none absolute inset-0 animate-[pulse-ring_2.4s_ease-out_infinite] rounded-full border-2 border-danger"></span>
    {/if}
    <span class="transition-transform duration-300 ease-apple {running ? 'scale-100' : 'translate-x-0.5'}">
      <Icon name={running ? "stop" : "play"} size={running ? 26 : 30} />
    </span>
  </button>
</div>

