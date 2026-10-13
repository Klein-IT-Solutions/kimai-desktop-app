import { setDescription, tracker } from "./tracker.svelte";

/**
 * The description that dictation and the bullet-point AI write into: the
 * timer's description, or the entry being edited while the editor is open.
 */
export interface DescriptionTarget {
  get(): string;
  set(text: string): void;
}

const timerTarget: DescriptionTarget = {
  get: () => tracker.state.description,
  set: setDescription,
};

let current: DescriptionTarget = timerTarget;

/** Makes `target` receive dictation and AI results; call the returned function to release it. */
export function claimDescription(target: DescriptionTarget): () => void {
  current = target;
  return () => {
    if (current === target) current = timerTarget;
  };
}

export const descriptionTarget = () => current;
