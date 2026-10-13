<script lang="ts">
  import { slide } from "svelte/transition";
  import { voice } from "../voice.svelte";
  import { assistant, undoSummary } from "../assistant.svelte";
</script>

<!-- Messages of the local AI and speech input, shown above a description field. -->
{#if assistant.error}
  <button
    transition:slide={{ duration: 180 }}
    class="mb-2 rounded-[10px] bg-danger/12 px-3 py-2 text-left text-[12px] leading-snug"
    onclick={() => (assistant.error = "")}
  >
    {assistant.error}
  </button>
{:else if assistant.undo !== null}
  <div transition:slide={{ duration: 180 }} class="mb-2 flex items-center justify-between rounded-[10px] bg-accent/10 px-3 py-1.5 text-[12px]">
    <span>In Stichpunkte umgewandelt</span>
    <button class="font-medium text-accent hover:opacity-75" onclick={undoSummary}>Rückgängig</button>
  </div>
{/if}

{#if voice.error}
  <button
    transition:slide={{ duration: 180 }}
    class="mb-2 rounded-[10px] bg-warning/15 px-3 py-2 text-left text-[12px] leading-snug"
    title="Ausblenden"
    onclick={() => (voice.error = "")}
  >
    {voice.error}
  </button>
{/if}
