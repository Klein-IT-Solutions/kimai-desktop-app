<script lang="ts">
  import { TrackerService, type Option, type Selection } from "../../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
  import SelectRow from "./SelectRow.svelte";
  import Picker from "./Picker.svelte";

  type Kind = "customer" | "project" | "activity";

  let {
    customer,
    project,
    activity,
    disabled = false,
    open = $bindable(false),
    onchange,
  }: {
    customer: Selection;
    project: Selection;
    activity: Selection;
    disabled?: boolean;
    /** True while one of the lists is open. */
    open?: boolean;
    /** Called for every choice; the parent clears dependent fields. */
    onchange: (kind: Kind, option: Option) => void;
  } = $props();

  let picker = $state<Kind | null>(null);
  $effect(() => {
    open = picker !== null;
  });

  /** Opens the first missing list (used by the editor for new entries). */
  export function openNext() {
    picker = !customer.id ? "customer" : !project.id ? "project" : !activity.id ? "activity" : null;
  }

  // After a choice, open the next picker – or pick automatically when there is only one option.
  async function afterCustomer(customerId: number) {
    const items = (await TrackerService.Projects(customerId, false).catch(() => null))?.items ?? [];
    if (items.length === 1) {
      onchange("project", items[0]);
      return afterProject(items[0].id);
    }
    picker = "project";
  }

  async function afterProject(projectId: number) {
    const items = (await TrackerService.Activities(projectId, false).catch(() => null))?.items ?? [];
    if (items.length === 1) {
      onchange("activity", items[0]);
      picker = null;
      return;
    }
    picker = activity.id ? null : "activity";
  }

  function pickCustomer(o: Option) {
    const changed = o.id !== customer.id;
    onchange("customer", o);
    picker = null;
    if (changed || !project.id) afterCustomer(o.id);
  }

  function pickProject(o: Option) {
    const changed = o.id !== project.id;
    onchange("project", o);
    picker = null;
    if (changed || !activity.id) afterProject(o.id);
  }

  function pickActivity(o: Option) {
    onchange("activity", o);
    picker = null;
  }
</script>

<section
  class="overflow-hidden rounded-[10px] bg-field shadow-[inset_0_0_0_0.5px_var(--hairline)] [&>*+*]:border-t [&>*+*]:border-hairline"
>
  <SelectRow icon="building" label="Kunde" value={customer.name} {disabled} onclick={() => (picker = "customer")} />
  <SelectRow
    icon="folder"
    label="Projekt"
    value={project.name}
    placeholder={customer.id ? "Auswählen" : "–"}
    disabled={disabled || !customer.id}
    onclick={() => (picker = "project")}
  />
  <SelectRow
    icon="tag"
    label="Tätigkeit"
    value={activity.name}
    placeholder={project.id ? "Auswählen" : "–"}
    disabled={disabled || !project.id}
    onclick={() => (picker = "activity")}
  />
</section>

{#if picker === "customer"}
  <Picker
    title="Kunde"
    placeholder="Kunde suchen …"
    selectedId={customer.id}
    load={(r) => TrackerService.Customers(r)}
    onselect={pickCustomer}
    onclose={() => (picker = null)}
  />
{:else if picker === "project"}
  <Picker
    title="Projekt"
    placeholder="Projekt suchen …"
    selectedId={project.id}
    load={(r) => TrackerService.Projects(customer.id, r)}
    onselect={pickProject}
    onclose={() => (picker = null)}
  />
{:else if picker === "activity"}
  <Picker
    title="Tätigkeit"
    placeholder="Tätigkeit suchen …"
    selectedId={activity.id}
    load={(r) => TrackerService.Activities(project.id, r)}
    onselect={pickActivity}
    onclose={() => (picker = null)}
  />
{/if}
