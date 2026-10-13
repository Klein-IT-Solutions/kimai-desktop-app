<script lang="ts">
  import type { Snippet } from "svelte";

  let {
    value = $bindable(""),
    label,
    type = "text",
    placeholder = "",
    autocomplete = "off",
    trailing,
  }: {
    value?: string;
    label: string;
    type?: "text" | "password" | "url";
    placeholder?: string;
    autocomplete?: HTMLInputElement["autocomplete"];
    trailing?: Snippet;
  } = $props();

  const id = `field-${Math.random().toString(36).slice(2, 8)}`;
</script>

<label for={id} class="flex h-10 items-center gap-2 px-3">
  <span class="w-[70px] shrink-0 text-fg">{label}</span>
  <input
    {id}
    {type}
    {placeholder}
    {autocomplete}
    spellcheck="false"
    autocapitalize="off"
    bind:value
    class="min-w-0 flex-1 bg-transparent text-right text-[13px] text-fg outline-none placeholder:text-fg-subtle focus:text-left"
  />
  {@render trailing?.()}
</label>
