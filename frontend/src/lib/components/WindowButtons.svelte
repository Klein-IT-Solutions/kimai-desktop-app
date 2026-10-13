<script lang="ts">
  import { Window } from "@wailsio/runtime";
  import { ShellService } from "../../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
  import type { WindowControls } from "../platform";

  let { controls }: { controls: WindowControls } = $props();

  // Closing hides the window to the tray (or minimises it when there is no tray).
  const actions: Record<string, () => void> = {
    close: () => ShellService.CloseWindow(),
    minimize: () => Window.Minimise(),
  };
  const labels: Record<string, string> = { close: "Schließen", minimize: "Minimieren" };
</script>

{#if controls.style === "mac"}
  <!-- macOS: traffic lights, symbols appear on hover of the group -->
  <div class="no-drag group flex items-center gap-2">
    {#each controls.buttons as b}
      <button
        class="grid size-3 place-items-center rounded-full shadow-[inset_0_0_0_0.5px_rgba(0,0,0,0.12)] {b === 'close'
          ? 'bg-[#ff5f57]'
          : 'bg-[#febc2e]'}"
        aria-label={labels[b]}
        onclick={actions[b]}
      >
        <svg viewBox="0 0 12 12" class="size-2 opacity-0 transition-opacity group-hover:opacity-100" stroke-width="1.4" stroke-linecap="round">
          {#if b === "close"}
            <path d="M3.5 3.5l5 5m0-5l-5 5" stroke="#4d0000" />
          {:else}
            <path d="M3 6h6" stroke="#5a3a00" />
          {/if}
        </svg>
      </button>
    {/each}
  </div>
{:else if controls.style === "windows"}
  <!-- Windows 11: flat caption buttons flush with the window edge -->
  <div class="no-drag flex self-stretch {controls.side === 'right' ? '-mr-3.5' : '-ml-3.5'}">
    {#each controls.buttons as b}
      <button
        class="grid h-full w-11 place-items-center text-fg/80 transition-colors duration-100 {b === 'close'
          ? 'hover:bg-[#c42b1c] hover:text-white active:bg-[#c42b1c]/85'
          : 'hover:bg-fg/[0.07] active:bg-fg/[0.04]'}"
        aria-label={labels[b]}
        onclick={actions[b]}
      >
        <svg viewBox="0 0 10 10" class="size-2.5" fill="none" stroke="currentColor" stroke-width="1">
          {#if b === "close"}
            <path d="M0.5 0.5l9 9m0-9l-9 9" />
          {:else}
            <path d="M0 5h10" />
          {/if}
        </svg>
      </button>
    {/each}
  </div>
{:else if controls.style === "kde"}
  <!-- KDE Plasma (Breeze): plain symbols, red circle behind close on hover -->
  <div class="no-drag flex items-center gap-1.5">
    {#each controls.buttons as b}
      <button
        class="grid size-[18px] place-items-center rounded-full text-fg/80 transition-colors duration-100 {b === 'close'
          ? 'hover:bg-[#da4453] hover:text-white'
          : 'hover:bg-fg/15'}"
        aria-label={labels[b]}
        onclick={actions[b]}
      >
        <svg viewBox="0 0 12 12" class="size-3" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round">
          {#if b === "close"}
            <path d="M3 3l6 6m0-6l-6 6" />
          {:else}
            <path d="M2.5 4.5L6 8l3.5-3.5" />
          {/if}
        </svg>
      </button>
    {/each}
  </div>
{:else}
  <!-- GNOME (Adwaita): round, softly filled buttons -->
  <div class="no-drag flex items-center gap-2.5">
    {#each controls.buttons as b}
      <button
        class="grid size-6 place-items-center rounded-full bg-fg/10 text-fg transition-colors duration-100 hover:bg-fg/15 active:bg-fg/25"
        aria-label={labels[b]}
        onclick={actions[b]}
      >
        <svg viewBox="0 0 16 16" class="size-4" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
          {#if b === "close"}
            <path d="M5 5l6 6m0-6l-6 6" />
          {:else}
            <path d="M5 10.5h6" />
          {/if}
        </svg>
      </button>
    {/each}
  </div>
{/if}
