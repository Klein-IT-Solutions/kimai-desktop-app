<script lang="ts">
  import { slide } from "svelte/transition";
  import type { ConnectionResult } from "../../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
  import Icon from "./Icon.svelte";

  let { result }: { result: ConnectionResult } = $props();

  const tone = $derived(result.ok ? "success" : result.status === "not_configured" ? "warning" : "danger");
  const details = $derived(
    result.ok ? [result.serverVersion && `Kimai ${result.serverVersion}`, hostOf(result.baseUrl)].filter(Boolean).join(" · ") : "",
  );

  function hostOf(url: string) {
    try {
      return new URL(url).host;
    } catch {
      return url;
    }
  }
</script>

<div
  transition:slide={{ duration: 200 }}
  class="flex items-start gap-2.5 rounded-[10px] px-3 py-2.5 text-[12px] leading-snug
    {tone === 'success' ? 'bg-success/12 text-fg' : tone === 'warning' ? 'bg-warning/15 text-fg' : 'bg-danger/12 text-fg'}"
  role="status"
>
  <span
    class="mt-px grid size-4 shrink-0 place-items-center rounded-full text-white
      {tone === 'success' ? 'bg-success' : tone === 'warning' ? 'bg-warning' : 'bg-danger'}"
  >
    {#if result.ok}
      <Icon name="check" size={10} stroke={3} />
    {:else}
      <span class="text-[10px] leading-none font-bold">!</span>
    {/if}
  </span>
  <div class="min-w-0">
    <div class="font-medium">{result.message}</div>
    {#if details}<div class="truncate text-fg-muted">{details}</div>{/if}
  </div>
</div>
