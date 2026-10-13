<script lang="ts">
  let {
    checked = false,
    disabled = false,
    label,
    onchange,
  }: { checked?: boolean; disabled?: boolean; label: string; onchange: (v: boolean) => void } = $props();
</script>

<button
  role="switch"
  aria-checked={checked}
  aria-label={label}
  {disabled}
  class="relative h-[22px] w-[38px] shrink-0 rounded-full transition-colors duration-200 disabled:opacity-40
    {checked ? 'bg-success' : 'bg-fg/15'}"
  onclick={() => onchange(!checked)}
>
  <span
    class="absolute top-[2px] left-[2px] size-[18px] rounded-full bg-white shadow-[0_1px_3px_rgba(0,0,0,0.25)] transition-transform duration-200 ease-apple
      {checked ? 'translate-x-4' : ''}"
  ></span>
</button>
