// A machine's card that holds still. The runtime already says a level only once it held (machines/health.go);
// what the page hears can still blink: the link drops and comes back, a machine leaves the list for a moment, the cause
// changes from one look to the next. So a notice, once shown, stays the same card: it keeps its key and its «since»
// for the whole episode, what it says changes no more often than CHANGE_MS, and it goes only after the machine has
// not said it for HOLD_MS. One that comes back within that time is the same card, in the same place.
import { useEffect, useMemo, useState } from 'react';
import type { IMachineNotice } from '@/next/features/notifications/from-machines';

/** A notice the machine stopped saying stays this long: a reconnect and one good look are shorter. */
export const HOLD_MS = 30_000;
/** What a shown notice says (the cause, its figures) changes no more often than this. */
export const CHANGE_MS = 15_000;

export interface ISteadyNotice {
  notice: IMachineNotice;
  /** When what it says last changed. */
  changedAt: number;
  /** Since when the machine does not say it; null while it does. */
  goneAt: number | null;
}

const sameWords = (a: IMachineNotice, b: IMachineNotice): boolean =>
  a.name === b.name &&
  a.urgent === b.urgent &&
  a.alert.metric === b.alert.metric &&
  a.alert.text === b.alert.text &&
  a.alert.icon === b.alert.icon &&
  (a.alert.waiting ?? []).join('\n') === (b.alert.waiting ?? []).join('\n');

/** The notice as its episode shows it: the key and «since» of the first look, the words of this one. */
const worded = (first: IMachineNotice, now: IMachineNotice): IMachineNotice => ({
  ...now,
  since: first.since,
  alert: { ...now.alert, key: first.alert.key },
});

/**
 * What is shown, given what was shown and what the machines say now. A card keeps its place in the list; a new one
 * comes after the ones there. The same list comes back while nothing changed.
 */
export const steadyNotices = (
  was: readonly ISteadyNotice[],
  raw: readonly IMachineNotice[],
  now: number,
): readonly ISteadyNotice[] => {
  const said = new Map(raw.map((n) => [n.id, n]));
  let changed = false;
  const next: ISteadyNotice[] = [];
  for (const held of was) {
    const fresh = said.get(held.notice.id);
    said.delete(held.notice.id);
    if (!fresh) {
      const goneAt = held.goneAt ?? now;
      if (now - goneAt >= HOLD_MS) {
        changed = true;
        continue;
      }
      if (held.goneAt === null) changed = true;
      next.push(held.goneAt === null ? { ...held, goneAt } : held);
      continue;
    }
    const back = held.goneAt !== null;
    const reworded = !sameWords(held.notice, fresh) && now - held.changedAt >= CHANGE_MS;
    if (!back && !reworded) {
      next.push(held);
      continue;
    }
    changed = true;
    next.push({
      notice: reworded ? worded(held.notice, fresh) : held.notice,
      changedAt: reworded ? now : held.changedAt,
      goneAt: null,
    });
  }
  for (const fresh of said.values()) {
    changed = true;
    // The episode's own key: the machine and when it came to this level, whatever runs short within it.
    const first = { ...fresh, alert: { ...fresh.alert, key: `${fresh.id}:${fresh.since}` } };
    next.push({ notice: first, changedAt: now, goneAt: null });
  }
  return changed ? next : was;
};

/** When the list has to be looked at again without the machines saying anything new; null: never. */
export const steadyDue = (held: readonly ISteadyNotice[], raw: readonly IMachineNotice[]): number | null => {
  const said = new Map(raw.map((n) => [n.id, n]));
  let due: number | null = null;
  const sooner = (at: number) => {
    due = due === null ? at : Math.min(due, at);
  };
  for (const h of held) {
    const fresh = said.get(h.notice.id);
    if (h.goneAt !== null) sooner(h.goneAt + HOLD_MS);
    else if (fresh && !sameWords(h.notice, fresh)) sooner(h.changedAt + CHANGE_MS);
  }
  return due;
};

/** The machines' notices, held still: looked at when the machines say something new, and again when a hold or a
 *  pause runs out. */
export const useSteadyNotices = (raw: readonly IMachineNotice[]): readonly IMachineNotice[] => {
  const [held, setHeld] = useState<readonly ISteadyNotice[]>(() => steadyNotices([], raw, Date.now()));
  const due = steadyDue(held, raw);
  useEffect(() => {
    const look = () => setHeld((list) => steadyNotices(list, raw, Date.now()));
    const first = window.setTimeout(look, 0);
    const again = due === null ? null : window.setTimeout(look, Math.max(0, due - Date.now()) + 20);
    return () => {
      window.clearTimeout(first);
      if (again !== null) window.clearTimeout(again);
    };
  }, [raw, due]);
  return useMemo(() => held.map((h) => h.notice), [held]);
};
