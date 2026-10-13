<script lang="ts">
  import { fade, fly } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import type { Option, OptionList } from "../../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
  import Icon from "./Icon.svelte";
  import { errorMessage } from "../connection.svelte";

  let {
    title,
    placeholder = "Suchen …",
    selectedId = 0,
    load,
    onselect,
    onclose,
  }: {
    title: string;
    placeholder?: string;
    selectedId?: number;
    load: (refresh: boolean) => Promise<OptionList>;
    onselect: (o: Option, all: Option[]) => void;
    onclose: () => void;
  } = $props();

  let items = $state<Option[]>([]);
  let stale = $state("");
  let error = $state("");
  let loading = $state(true);
  let query = $state("");
  let active = $state(0);
  let listEl = $state<HTMLElement>();
  let inputEl = $state<HTMLInputElement>();

  const norm = (s: string) =>
    s
      .toLocaleLowerCase("de")
      .normalize("NFD")
      .replace(/[̀-ͯ]/g, "");

  const filtered = $derived.by(() => {
    const q = norm(query.trim());
    if (!q) return items;
    return items.filter((o) => norm(`${o.name} ${o.hint ?? ""}`).includes(q));
  });
  const recentCount = $derived(query.trim() ? 0 : filtered.filter((o) => o.recent).length);

  async function fetch(refresh: boolean) {
    loading = true;
    error = "";
    try {
      const res = await load(refresh);
      items = res.items ?? [];
      stale = res.stale ? res.message || "Offline – zwischengespeicherte Daten." : "";
      const sel = items.findIndex((o) => o.id === selectedId);
      active = sel >= 0 && !query ? sel : 0;
      queueScroll();
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }

  fetch(false);
  $effect(() => inputEl?.focus());

  // Keep the highlighted entry in range while filtering.
  $effect(() => {
    if (active >= filtered.length) active = Math.max(0, filtered.length - 1);
  });

  function queueScroll() {
    requestAnimationFrame(() => listEl?.querySelector<HTMLElement>(`[data-index="${active}"]`)?.scrollIntoView({ block: "nearest" }));
  }

  function choose(o: Option | undefined) {
    if (o) onselect(o, items);
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === "ArrowDown") {
      active = Math.min(active + 1, filtered.length - 1);
      queueScroll();
      e.preventDefault();
    } else if (e.key === "ArrowUp") {
      active = Math.max(active - 1, 0);
      queueScroll();
      e.preventDefault();
    } else if (e.key === "Enter") {
      choose(filtered[active]);
      e.preventDefault();
    } else if (e.key === "Escape") {
      onclose();
      e.preventDefault();
    }
  }
</script>

<!-- Backdrop -->
<button
  class="absolute inset-0 z-10 cursor-default bg-black/10 dark:bg-black/30"
  aria-label="Schließen"
  transition:fade={{ duration: 150 }}
  onclick={onclose}
></button>

<div
  class="absolute inset-x-3 top-1 z-20 flex max-h-[calc(100%-1rem)] flex-col overflow-hidden rounded-xl bg-[var(--surface)] shadow-[0_8px_24px_rgba(0,0,0,0.18),inset_0_0_0_0.5px_var(--border-window)] backdrop-blur-xl"
  role="dialog"
  aria-label={title}
  transition:fly={{ y: -8, duration: 180, easing: cubicOut }}
>
  <div class="flex items-center gap-2 border-b border-hairline px-3 py-2">
    <span class="text-fg-subtle"><Icon name="search" size={14} /></span>
    <input
      bind:this={inputEl}
      bind:value={query}
      {placeholder}
      {onkeydown}
      oninput={() => (active = 0)}
      spellcheck="false"
      class="min-w-0 flex-1 bg-transparent text-[13px] text-fg outline-none placeholder:text-fg-subtle"
    />
    <button
      class="grid size-6 place-items-center rounded-md text-fg-subtle transition-colors hover:bg-field-hover hover:text-fg disabled:opacity-40"
      title="Aus Kimai neu laden"
      aria-label="Neu laden"
      disabled={loading}
      onclick={() => fetch(true)}
    >
      <span class={loading ? "animate-spin" : ""}><Icon name="refresh" size={13} /></span>
    </button>
  </div>

  {#if stale}
    <div class="border-b border-hairline bg-warning/12 px-3 py-1.5 text-[11px] text-fg-muted">{stale}</div>
  {/if}

  <div bind:this={listEl} class="min-h-0 overflow-y-auto py-1" role="listbox">
    {#if error}
      <div class="px-3 py-4 text-center text-[12px] text-danger">{error}</div>
    {:else if loading && items.length === 0}
      <div class="px-3 py-4 text-center text-[12px] text-fg-subtle">Lädt …</div>
    {:else if filtered.length === 0}
      <div class="px-3 py-4 text-center text-[12px] text-fg-subtle">
        {query ? "Keine Treffer" : `Keine Einträge in Kimai`}
      </div>
    {:else}
      {#each filtered as o, i (o.id)}
        {#if i === 0 && recentCount > 0}
          <div class="px-3 pt-1 pb-0.5 text-[10px] font-medium tracking-wide text-fg-subtle uppercase">Zuletzt genutzt</div>
        {:else if i === recentCount && recentCount > 0}
          <div class="mx-3 my-1 border-t border-hairline"></div>
        {/if}
        <button
          data-index={i}
          role="option"
          aria-selected={o.id === selectedId}
          class="mx-1 flex w-[calc(100%-0.5rem)] items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px] transition-colors duration-75
            {i === active ? 'bg-accent text-white' : 'text-fg'}"
          onmouseenter={() => (active = i)}
          onclick={() => choose(o)}
        >
          {#if o.color}
            <span class="size-2 shrink-0 rounded-full" style="background:{o.color}"></span>
          {/if}
          <span class="min-w-0 flex-1 truncate">{o.name}</span>
          {#if o.hint}
            <span class="shrink-0 text-[11px] {i === active ? 'text-white/75' : 'text-fg-subtle'}">{o.hint}</span>
          {/if}
          {#if o.recent && !query}
            <span class={i === active ? "text-white/75" : "text-fg-subtle"}><Icon name="clock" size={12} /></span>
          {/if}
          {#if o.id === selectedId}
            <span class={i === active ? "text-white" : "text-accent"}><Icon name="check" size={13} stroke={2.4} /></span>
          {/if}
        </button>
      {/each}
    {/if}
  </div>
</div>
