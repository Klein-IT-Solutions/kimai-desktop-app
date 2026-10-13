<script lang="ts">
  import Icon, { type IconName } from "./Icon.svelte";

  let {
    icon,
    label,
    value = "",
    placeholder = "Auswählen",
    disabled = false,
    onclick,
  }: {
    icon: IconName;
    label: string;
    value?: string;
    placeholder?: string;
    disabled?: boolean;
    onclick?: () => void;
  } = $props();
</script>

<button
  class="flex h-10 w-full items-center gap-2.5 px-3 text-left transition-colors duration-150 enabled:hover:bg-field-hover disabled:cursor-default"
  {disabled}
  {onclick}
>
  <span class="text-fg-muted"><Icon name={icon} size={15} /></span>
  <span class="w-[70px] shrink-0 text-fg">{label}</span>
  <span class="min-w-0 flex-1 truncate text-right {value ? 'text-fg' : 'text-fg-subtle'}">
    {value || placeholder}
  </span>
  <span class="text-fg-subtle transition-opacity {disabled ? 'opacity-0' : ''}"><Icon name="chevronRight" size={13} stroke={2.2} /></span>
</button>
