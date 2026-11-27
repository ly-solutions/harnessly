// Whether this device reaches the app's server at all: the one word on «offline» the whole page shares. The
// browser's own flag says only that a network is there, not that the server is (a dead VPN, a hotel portal, a server
// down), so a «yes» from the browser is checked with a small request, and a request of the page that found no network
// asks for that check too. Going offline and back changes this store only: nothing reloads, nothing is thrown away.

import { createStore, useStore } from '@/next/core/store';
import { offlineBase } from '@/next/offline/client';

export interface INet {
  online: boolean;
  /** When the device went offline last (page ms), 0 while it never did. */
  since: number;
}

export const netStore = createStore<INet>({ online: true, since: 0 });

/** How often an offline device asks the server again, ms. */
export const PROBE_EVERY_MS = 10_000;
/** A probe that has no answer in this long counts as none. */
export const PROBE_TIMEOUT_MS = 5000;

export interface INetEnv {
  browserOnline: () => boolean;
  /** Resolves true when the server answered anything at all (a sign-in redirect is an answer). */
  reach: () => Promise<boolean>;
  on: (event: 'online' | 'offline' | 'visible', fn: () => void) => () => void;
  setTimeout: (fn: () => void, ms: number) => unknown;
  clearTimeout: (handle: unknown) => void;
  now: () => number;
}

const browserEnv = (): INetEnv => ({
  browserOnline: () => (typeof navigator === 'undefined' ? true : navigator.onLine !== false),
  reach: async () => {
    const ctrl = new AbortController();
    const cut = setTimeout(() => ctrl.abort(), PROBE_TIMEOUT_MS);
    try {
      // The query keeps the service worker out of it: only the network answers a probe.
      await fetch(`${offlineBase()}manifest.json?probe=${Date.now()}`, {
        cache: 'no-store',
        redirect: 'manual',
        credentials: 'same-origin',
        signal: ctrl.signal,
      });
      return true;
    } catch {
      return false;
    } finally {
      clearTimeout(cut);
    }
  },
  on: (event, fn) => {
    if (typeof window === 'undefined') return () => {};
    if (event === 'visible') {
      const onVis = () => {
        if (document.visibilityState === 'visible') fn();
      };
      document.addEventListener('visibilitychange', onVis);
      return () => document.removeEventListener('visibilitychange', onVis);
    }
    window.addEventListener(event, fn);
    return () => window.removeEventListener(event, fn);
  },
  setTimeout: (fn, ms) => setTimeout(fn, ms),
  clearTimeout: (h) => clearTimeout(h as ReturnType<typeof setTimeout>),
  now: () => Date.now(),
});

let env: INetEnv | null = null;
let timer: unknown = null;
let probing: Promise<void> | null = null;
let stops: (() => void)[] = [];

const setOnline = (online: boolean) => {
  const was = netStore.get();
  if (was.online === online) return;
  netStore.set({ online, since: online ? was.since : (env?.now() ?? Date.now()) });
};

const plan = () => {
  if (!env) return;
  if (timer !== null) env.clearTimeout(timer);
  timer = null;
  if (!netStore.get().online) timer = env.setTimeout(() => void checkNet(), PROBE_EVERY_MS);
};

/**
 * Asks the server now; a second call while one is under way waits for it. A browser that says «offline» is not taken
 * at its word either: Android keeps navigator.onLine false after a VPN or network switch while requests go through,
 * and the pill then stayed for good — «Проверить» included. Only the server's silence keeps the device offline.
 */
export function checkNet(): Promise<void> {
  const e = env;
  if (!e) return Promise.resolve();
  if (!e.browserOnline()) setOnline(false);
  probing ??= e
    .reach()
    .then((ok) => {
      if (env !== e) return;
      setOnline(ok);
      plan();
    })
    .finally(() => {
      probing = null;
    });
  return probing;
}

/**
 * A request of the page failed without an answer (no network): the device may be offline. Checked, not believed: a
 * single lost request of a flaky network must not paint the whole page offline.
 */
export function noteNoAnswer(): void {
  if (env && netStore.get().online) void checkNet();
}

/** Starts watching the network; returns the stop. Tests pass their own environment. */
export function startNet(given?: INetEnv): () => void {
  stopNet();
  const e = (env = given ?? browserEnv());
  if (!e.browserOnline()) setOnline(false);
  stops = [
    // The browser lost its network: that much it knows for sure, at once.
    e.on('offline', () => {
      setOnline(false);
      plan();
    }),
    e.on('online', () => void checkNet()),
    e.on('visible', () => {
      if (!netStore.get().online) void checkNet();
    }),
  ];
  void checkNet();
  return stopNet;
}

export function stopNet(): void {
  for (const s of stops) s();
  stops = [];
  if (env && timer !== null) env.clearTimeout(timer);
  timer = null;
  env = null;
  probing = null;
}

/** True while the device does not reach the server. */
export const useOffline = (): boolean => !useStore(netStore).online;

/**
 * Calls `fn` each time the device is back: the server answers again. The browser's own «online» comes too late or never
 * when only the server was out of reach (a VPN back up), so a page that waits for the network listens here as well.
 */
export function onBackOnline(fn: () => void): () => void {
  let was = netStore.get().online;
  return netStore.subscribe(() => {
    const now = netStore.get().online;
    if (now && !was) fn();
    was = now;
  });
}
