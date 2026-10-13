<script lang="ts">
  let { value, onchange }: { value: string; onchange: (accelerator: string) => void } = $props();

  let recording = $state(false);
  let preview = $state("");

  const MOD_KEYS = new Set(["Shift", "Control", "Alt", "Meta", "Super", "Hyper", "OS", "AltGraph"]);

  function keyName(e: KeyboardEvent): string | null {
    if (/^Key[A-Z]$/.test(e.code)) return e.code.slice(3);
    if (/^Digit[0-9]$/.test(e.code)) return e.code.slice(5);
    if (/^F([1-9]|1[0-2])$/.test(e.key)) return e.key;
    if (e.code === "Space") return "Space";
    return null;
  }

  function onkeydown(e: KeyboardEvent) {
    if (!recording) return;
    e.preventDefault();
    e.stopPropagation();
    if (e.key === "Escape") {
      recording = false;
      return;
    }
    if (e.key === "Backspace" || e.key === "Delete") {
      recording = false;
      onchange("");
      return;
    }
    const mods = [e.metaKey && "Super", e.ctrlKey && "Ctrl", e.altKey && "Alt", e.shiftKey && "Shift"].filter(Boolean) as string[];
    if (MOD_KEYS.has(e.key)) {
      preview = mods.join("+") + "+…";
      return;
    }
    const key = keyName(e);
    if (!key) return;
    recording = false;
    onchange([...mods, key].join("+"));
  }
</script>

<svelte:window {onkeydown} />

<button
  class="min-w-[120px] rounded-md px-2 py-1 text-right text-[12px] transition-colors
    {recording ? 'bg-accent/15 text-accent' : 'text-fg-muted hover:bg-field-hover'}"
  onclick={() => {
    recording = !recording;
    preview = "";
  }}
  onblur={() => (recording = false)}
  title="Klicken und neue Tastenkombination drücken (Esc bricht ab, Rücktaste entfernt)"
>
  {#if recording}
    {preview || "Tasten drücken …"}
  {:else}
    {value ? value.replaceAll("+", " + ") : "Keins"}
  {/if}
</button>
