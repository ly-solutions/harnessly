// A card that opens by itself: a machine about to fail its work shows what is wrong for a few seconds and
// folds again. It must not nag: only an urgent card opens, and one machine's card no more often than once in
// AUTO_OPEN_EVERY_MS, wherever it stands (the smart notice or the stack under the column share this count).

/** A card that just came opens by itself this long after: once it stands in its place. */
export const AUTO_OPEN_DELAY_MS = 400;
/** How long a card that opened by itself stays open. */
export const AUTO_OPEN_MS = 5000;
/** One machine's card opens by itself no more often than this. */
export const AUTO_OPEN_EVERY_MS = 5 * 60_000;

const last = new Map<string, number>();

/** True when the card of `id` may open by itself now; the moment is then taken, so the next ask within the pause
 *  answers false. */
export const takeAutoOpen = (id: string, urgent: boolean, now: number): boolean => {
  if (!urgent) return false;
  const was = last.get(id);
  if (was !== undefined && now - was < AUTO_OPEN_EVERY_MS) return false;
  last.set(id, now);
  return true;
};

/** For tests: nothing has opened yet. */
export const resetAutoOpen = () => last.clear();
