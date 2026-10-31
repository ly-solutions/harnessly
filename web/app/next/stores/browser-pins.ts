// The browser tab's pinned pages and visits, per person, in this browser (localStorage, read once per person and
// written at every change). Storage that throws is the same as none: the pins hold for the page.
import { useCallback, useMemo, useSyncExternalStore } from 'react';
import {
  addPin,
  defaultPins,
  parsePins,
  parseVisits,
  pushVisit,
  removePin,
  type IPin,
} from '@/next/features/pane/pins';

export interface IPinsState {
  pins: readonly IPin[];
  visits: readonly string[];
}

const PINS_KEY = 'hx-browser-pins';
const VISITS_KEY = 'hx-browser-visits';

const states = new Map<string, IPinsState>();
const listeners = new Set<() => void>();

const read = (key: string): string | null => {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
};

const write = (key: string, value: unknown) => {
  try {
    window.localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // The change holds for this page.
  }
};

/** The person's pins and visits; the defaults until they saved pins of their own. */
export const pinsOf = (person: string): IPinsState => {
  const kept = states.get(person);
  if (kept) return kept;
  const loaded: IPinsState = {
    pins: parsePins(read(`${PINS_KEY}:${person}`)) ?? defaultPins(),
    visits: parseVisits(read(`${VISITS_KEY}:${person}`)),
  };
  states.set(person, loaded);
  return loaded;
};

const change = (person: string, next: Partial<IPinsState>) => {
  const now = { ...pinsOf(person), ...next };
  states.set(person, now);
  if (next.pins) write(`${PINS_KEY}:${person}`, next.pins);
  if (next.visits) write(`${VISITS_KEY}:${person}`, next.visits);
  for (const listener of [...listeners]) listener();
};

export const pinPage = (person: string, pin: IPin) =>
  change(person, { pins: addPin(pinsOf(person).pins, pin) });
export const unpinPage = (person: string, text: string) =>
  change(person, { pins: removePin(pinsOf(person).pins, text) });
export const visitPage = (person: string, text: string) =>
  change(person, { visits: pushVisit(pinsOf(person).visits, text) });

/** Forgets what is held in memory (tests start from the storage again). */
export const resetPinsMemory = () => states.clear();

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => listeners.delete(listener);
};

export interface IBrowserPins extends IPinsState {
  pin: (pin: IPin) => void;
  unpin: (text: string) => void;
  visit: (text: string) => void;
}

/** The person's pins and visits; null until the person is known (null is not «none»). */
export const useBrowserPins = (person: string | null): IBrowserPins | null => {
  const state = useSyncExternalStore(
    subscribe,
    () => (person === null ? null : pinsOf(person)),
    () => null,
  );
  const pin = useCallback((p: IPin) => person !== null && pinPage(person, p), [person]);
  const unpin = useCallback((text: string) => person !== null && unpinPage(person, text), [person]);
  const visit = useCallback((text: string) => person !== null && visitPage(person, text), [person]);
  return useMemo(() => (state ? { ...state, pin, unpin, visit } : null), [state, pin, unpin, visit]);
};
