import { Events } from "@wailsio/runtime";
import {
  TrackerService,
  type ConnectionResult,
  type Option,
  type Selection,
  type TrackerState,
} from "../../bindings/github.com/klein-it-solutions/kimai-desktop-app/internal/app";
import { errorMessage } from "./connection.svelte";

const empty: Selection = { id: 0, name: "" };

/** Reactive timer state shared by the tracker UI. */
export const tracker = $state({
  state: {
    running: false,
    timesheetId: 0,
    customer: { ...empty },
    project: { ...empty },
    activity: { ...empty },
    description: "",
  } as TrackerState,
  busy: false,
  error: "",
  descriptionStatus: "" as "" | "saving" | "saved" | "error",
});

function apply(st: TrackerState) {
  tracker.state = st;
}

export function initTracker() {
  TrackerService.State()
    .then(apply)
    .catch(() => {});
  Events.On("tracker:changed", (ev) => apply(ev.data as TrackerState));
  // Whenever the connection becomes usable, pick up a timer running in Kimai.
  Events.On("connection:changed", (ev) => {
    if ((ev.data as ConnectionResult).ok) {
      tracker.error = ""; // an earlier offline error is obsolete now
      syncTracker();
    }
  });
}

export async function syncTracker() {
  try {
    apply(await TrackerService.Sync());
  } catch {
    /* offline – the connection status shows it */
  }
}

const toSel = (o: Option | null): Selection => (o ? { id: o.id, name: o.name } : { ...empty });

function persistSelection() {
  const s = tracker.state;
  TrackerService.SaveSelection(s.customer, s.project, s.activity, s.description).catch(() => {});
}

export function selectCustomer(o: Option) {
  const s = tracker.state;
  if (s.customer.id !== o.id) {
    s.project = { ...empty };
    s.activity = { ...empty };
  }
  s.customer = toSel(o);
  persistSelection();
}

export function selectProject(o: Option) {
  const s = tracker.state;
  if (s.project.id !== o.id) s.activity = { ...empty };
  s.project = toSel(o);
  persistSelection();
}

export function selectActivity(o: Option) {
  tracker.state.activity = toSel(o);
  persistSelection();
}

export const canStart = () => {
  const s = tracker.state;
  return !s.running && s.customer.id > 0 && s.project.id > 0 && s.activity.id > 0;
};

export async function toggleTimer() {
  if (tracker.busy) return;
  tracker.busy = true;
  tracker.error = "";
  try {
    const s = tracker.state;
    if (s.running) {
      flushDescription();
      apply(await TrackerService.Stop());
    } else {
      clearTimeout(descTimer);
      apply(
        await TrackerService.Start({
          customer: s.customer,
          project: s.project,
          activity: s.activity,
          description: s.description,
        }),
      );
    }
  } catch (e) {
    tracker.error = errorMessage(e);
  } finally {
    tracker.busy = false;
  }
}

// Description edits are sent to Kimai shortly after typing stops.
let descTimer: ReturnType<typeof setTimeout> | undefined;

export function setDescription(text: string) {
  tracker.state.description = text;
  clearTimeout(descTimer);
  descTimer = setTimeout(saveDescription, 700);
}

function flushDescription() {
  if (descTimer) {
    clearTimeout(descTimer);
    descTimer = undefined;
    saveDescription();
  }
}

async function saveDescription() {
  descTimer = undefined;
  const text = tracker.state.description;
  if (!tracker.state.running) {
    persistSelection();
    return;
  }
  tracker.descriptionStatus = "saving";
  try {
    await TrackerService.SetDescription(text);
    tracker.descriptionStatus = "saved";
    setTimeout(() => {
      if (tracker.descriptionStatus === "saved") tracker.descriptionStatus = "";
    }, 1500);
  } catch (e) {
    tracker.descriptionStatus = "error";
    tracker.error = errorMessage(e);
  }
}
