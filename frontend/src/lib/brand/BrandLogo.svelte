<script lang="ts">
  import { BRAND, brandLogoDarkUrl, brandLogoUrl } from "./brand";

  let { height = 20 }: { height?: number } = $props();
</script>

<span class="inline-flex items-center gap-2">
  {#if brandLogoUrl}
    <!-- The logo is an icon with its own background, so it works in light and dark mode. -->
    <img src={brandLogoUrl} alt="" style="height:{height}px" class="w-auto {brandLogoDarkUrl ? 'dark:hidden' : ''}" />
    {#if brandLogoDarkUrl}
      <img src={brandLogoDarkUrl} alt="" style="height:{height}px" class="hidden w-auto dark:block" />
    {/if}
  {/if}
  <span class="inline-flex items-baseline gap-1 leading-none font-semibold tracking-[-0.01em]" style="font-size:{Math.round(height * 0.6)}px">
    <span class="text-fg">Klein</span><span class="text-[#f58220]">IT</span><span class="font-normal text-fg-muted">Solutions</span>
  </span>
</span>
