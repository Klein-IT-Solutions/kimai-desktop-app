<script lang="ts">
  import type { Snippet } from "svelte";

  let {
    variant = "secondary",
    disabled = false,
    busy = false,
    onclick,
    children,
  }: {
    variant?: "primary" | "secondary";
    disabled?: boolean;
    busy?: boolean;
    onclick?: () => void;
    children: Snippet;
  } = $props();
</script>

<button
  class="relative flex h-8 flex-1 items-center justify-center gap-1.5 rounded-lg px-3 text-[13px] font-medium transition-all duration-150 active:scale-[0.98] disabled:opacity-45 disabled:active:scale-100
    {variant === 'primary'
    ? 'bg-accent text-white shadow-[0_1px_2px_rgba(0,0,0,0.15)] enabled:hover:brightness-110'
    : 'bg-field text-fg shadow-[inset_0_0_0_0.5px_var(--hairline)] enabled:hover:bg-field-hover'}"
  disabled={disabled || busy}
  {onclick}
>
  {#if busy}
    <span class="size-3 animate-spin rounded-full border-[1.5px] border-current border-r-transparent"></span>
  {/if}
  {@render children()}
</button>
