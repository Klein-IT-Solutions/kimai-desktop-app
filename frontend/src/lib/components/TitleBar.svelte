<script lang="ts">
  import Icon from "./Icon.svelte";
  import WindowButtons from "./WindowButtons.svelte";
  import { platform } from "../platform";

  let {
    title,
    canGoBack = false,
    showActions = true,
    onBack,
    onSettings,
    onEntries,
  }: {
    title: string;
    canGoBack?: boolean;
    showActions?: boolean;
    onBack?: () => void;
    onSettings?: () => void;
    onEntries?: () => void;
  } = $props();

  const controls = platform.controls;
  const controlsLeft = controls.side === "left";
</script>

<header class="drag relative flex h-10 shrink-0 items-center px-3.5 {controlsLeft ? '' : 'flex-row-reverse'}">
  <WindowButtons {controls} />

  <div class="pointer-events-none absolute inset-x-0 text-center text-[13px] font-semibold tracking-[-0.01em] text-fg/85">
    {title}
  </div>

  <!-- Settings / Done sit on the side opposite the window buttons. -->
  <div class="no-drag flex items-center {controlsLeft ? 'ml-auto' : 'mr-auto'}">
    {#if canGoBack}
      <button
        class="rounded-md px-2 py-1 text-[13px] font-medium text-accent transition-opacity hover:opacity-75"
        onclick={onBack}
      >
        Fertig
      </button>
    {:else if showActions}
      <button
        class="grid size-7 place-items-center rounded-md text-fg-muted transition-colors hover:bg-field-hover hover:text-fg"
        aria-label="Einträge"
        title="Einträge ansehen und nachtragen"
        onclick={onEntries}
      >
        <Icon name="list" size={16} />
      </button>
      <button
        class="grid size-7 place-items-center rounded-md text-fg-muted transition-colors hover:bg-field-hover hover:text-fg"
        aria-label="Einstellungen"
        onclick={onSettings}
      >
        <Icon name="gear" size={16} />
      </button>
    {/if}
  </div>
</header>
