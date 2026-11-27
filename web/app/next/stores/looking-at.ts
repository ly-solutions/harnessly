/** The chat on screen, as the visibility report tells the server: an action waiting in it needs no push. */
let lookingAt: string | null = null;
export const LOOKING_CHANGED_EVENT = 'hl:looking-changed';

export const getLookingAt = (): string | null => lookingAt;

export const setLookingAt = (id: string | null): void => {
  if (id === lookingAt) return;
  lookingAt = id;
  if (typeof window !== 'undefined') window.dispatchEvent(new Event(LOOKING_CHANGED_EVENT));
};
