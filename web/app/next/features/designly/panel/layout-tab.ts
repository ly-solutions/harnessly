// «Макет» is a tab of the chat's own tools panel, next to its grid, while a design is open beside the chat:
// the design (designly-site) says so for the chat and draws the tab's content into the place the panel gives it (a
// portal, so the design's state and contexts stay its own); the design's buttons ask the panel to show a part of it.
import { useSyncExternalStore } from 'react';
import type { TDesignTab } from '@/next/features/designly/design-panel';

/** The id of the tab in the panel's strip. */
export const LAYOUT_TAB = 'layout';

export type TLayoutPart = Exclude<TDesignTab, 'tools'>;

/** What the panel shows now: the tools, «Макет», another tab, or nothing (it is shut). */
export type TPaneShows = 'tools' | 'layout' | 'other' | 'shut';

export interface ILayoutEntry {
  /** The part of «Макет» on screen. */
  part: TLayoutPart;
  /** The open threads, beside the tab's name. */
  badge: number;
  /** Where the panel draws the tab's content; null while it does not. */
  slot: HTMLElement | null;
  /** What the panel shows (the panel says it). */
  shows: TPaneShows;
  /** A request to show a tab: the panel takes each once. */
  ask: { tab: 'tools' | 'layout'; seq: number } | null;
  /** The panel has put «Макет» in front once for this showing of the design: after that the person's pick stands. */
  fronted: boolean;
}

const entries = new Map<string, ILayoutEntry>();
const listeners = new Set<() => void>();
let seq = 0;

const emit = () => {
  for (const l of listeners) l();
};

const put = (conv: string, change: Partial<ILayoutEntry>) => {
  const cur = entries.get(conv);
  if (!cur) return;
  const next = { ...cur, ...change };
  if ((Object.keys(change) as (keyof ILayoutEntry)[]).every((k) => cur[k] === next[k])) return;
  entries.set(conv, next);
  emit();
};

/** The design open beside the chat offers «Макет»; the returned call takes it back. */
export const offerLayout = (conv: string): (() => void) => {
  if (!entries.has(conv)) {
    entries.set(conv, { part: 'info', badge: 0, slot: null, shows: 'shut', ask: null, fronted: false });
    emit();
  }
  return () => {
    entries.delete(conv);
    emit();
  };
};

export const setLayoutSlot = (conv: string, slot: HTMLElement | null) => put(conv, { slot });
export const setLayoutPart = (conv: string, part: TLayoutPart) => put(conv, { part });
export const setLayoutBadge = (conv: string, badge: number) => put(conv, { badge });
export const setPaneShows = (conv: string, shows: TPaneShows) => put(conv, { shows });

/** «Макет» was put in front for this showing of the design. */
export const setLayoutFronted = (conv: string) => put(conv, { fronted: true });

/** The panel took the request. */
export const clearLayoutAsk = (conv: string) => put(conv, { ask: null });

/** Asks the panel to show the tools or «Макет» (on a part). */
export const askLayout = (conv: string, tab: 'tools' | 'layout', part?: TLayoutPart) =>
  put(conv, { ask: { tab, seq: ++seq }, ...(part ? { part } : {}) });

const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => listeners.delete(l);
};

export const useLayoutEntry = (conv: string | null): ILayoutEntry | null =>
  useSyncExternalStore(
    subscribe,
    () => (conv === null ? null : (entries.get(conv) ?? null)),
    () => null,
  );
