<script lang="ts">
  import { clockMinutes, minutesClock } from "../format";

  // 24-hour time field that accepts quick input: "9" → 09:00, "930" → 09:30,
  // "9.30" → 09:30. Arrow keys move by 5 minutes.
  let { value = $bindable(""), label, class: cls = "" }: { value?: string; label: string; class?: string } = $props();

  let text = $state("");
  $effect(() => {
    text = value;
  });

  function normalize(raw: string): string | null {
    const t = raw.trim().replace(/[.,h ]/g, ":");
    let h: number, m: number;
    let match = /^(\d{1,2})(?::(\d{1,2}))?$/.exec(t);
    if (match) {
      h = Number(match[1]);
      m = Number(match[2] ?? 0);
    } else if ((match = /^(\d{1,2})(\d{2})$/.exec(t))) {
      h = Number(match[1]);
      m = Number(match[2]);
    } else return null;
    if (h > 23 || m > 59) return null;
    return minutesClock(h * 60 + m);
  }

  function commit() {
    const n = normalize(text);
    if (n) value = n;
    text = value;
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === "Enter") commit();
    if (e.key === "ArrowUp" || e.key === "ArrowDown") {
      e.preventDefault();
      const base = clockMinutes(normalize(text) ?? value);
      if (!Number.isNaN(base)) value = minutesClock(base + (e.key === "ArrowUp" ? 5 : -5));
    }
  }
</script>

<input
  type="text"
  inputmode="numeric"
  placeholder="hh:mm"
  aria-label={label}
  spellcheck="false"
  autocomplete="off"
  bind:value={text}
  onblur={commit}
  {onkeydown}
  onfocus={(e) => {
    // Select after the click's mouseup, which would clear an immediate selection.
    const el = e.currentTarget;
    setTimeout(() => el.select(), 0);
  }}
  class="w-[46px] rounded bg-transparent text-center text-[13px] text-fg tabular-nums outline-none focus:bg-field-hover {cls}"
/>
