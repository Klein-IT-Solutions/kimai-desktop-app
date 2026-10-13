<script lang="ts">
  import { AppService, type Legal } from "../../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";

  let legal = $state<Legal | null>(null);
  let tab = $state<"license" | "notices">("license");
  AppService.Legal()
    .then((l) => (legal = l))
    .catch(() => {});
</script>

<div class="flex h-full flex-col px-4 pt-1 pb-3">
  <div class="mb-2 grid grid-cols-2 rounded-lg bg-field p-0.5 text-[12px] shadow-[inset_0_0_0_0.5px_var(--hairline)]">
    {#each [["license", "Lizenz"], ["notices", "Drittanbieter"]] as [id, label]}
      <button
        class="rounded-md py-1 transition-colors {tab === id ? 'bg-[var(--surface)] font-medium shadow-sm' : 'text-fg-muted hover:text-fg'}"
        onclick={() => (tab = id as typeof tab)}
      >
        {label}
      </button>
    {/each}
  </div>
  <pre
    class="min-h-0 flex-1 overflow-auto rounded-[10px] bg-field p-3 font-sans text-[11px] leading-relaxed whitespace-pre-wrap text-fg-muted shadow-[inset_0_0_0_0.5px_var(--hairline)] select-text">{legal
      ? tab === "license"
        ? legal.license
        : legal.notices
      : "…"}</pre>
</div>
