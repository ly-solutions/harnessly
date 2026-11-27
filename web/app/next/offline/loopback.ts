// The loopback side of «этот компьютер» (offline/this-computer.ts): the app opened from the hub on
// http://127.0.0.1:<port><base> with a pass in the fragment. Small and loaded before the app's start (main.tsx): the
// pass becomes the session before the start data are asked for.
import { offlineBase } from '@/next/offline/client';

/** Where the loopback page goes back to (the hub's workspace), kept on the loopback's origin. */
const BACK_KEY = 'hx-local-back';
const PING_MS = 4000;

/** The route of this page under the app's base («c/tab-…»), the same on the hub and on the loopback. */
export const routeHere = (): string => {
  const base = offlineBase();
  const path = location.pathname.startsWith(base) ? location.pathname.slice(base.length) : '';
  const search = new URLSearchParams(location.search);
  search.delete('this-computer');
  search.delete('port');
  const rest = search.toString();
  return path + (rest ? `?${rest}` : '');
};

const LOOPBACK_HOST = /^(127\.\d+\.\d+\.\d+|localhost|\[::1\])$/;

/** This page is a runtime's on this computer's loopback, opened from the hub with a pass (the hub itself may be on a
 *  loopback too, in development: it is told apart by the way back the pass left here). */
export const onLoopback = (): boolean => localBack() !== null;

/** A way back is the hub over https, or a hub on this computer's loopback (development). */
const goodBack = (back: string): boolean => {
  try {
    const u = new URL(back);
    return u.protocol === 'https:' || (u.protocol === 'http:' && LOOPBACK_HOST.test(u.hostname));
  } catch {
    return false;
  }
};

/**
 * On the loopback page, before the app's start: a pass in the fragment opens the session, and the way back is kept.
 * Resolves once done (the start data then come signed in).
 */
export async function takeLocalPass(): Promise<void> {
  if (typeof location === 'undefined' || !LOOPBACK_HOST.test(location.hostname)) return;
  if (!location.hash.includes('hx-local=')) return;
  const params = new URLSearchParams(location.hash.slice(1));
  const pass = params.get('hx-local');
  const back = params.get('back');
  history.replaceState(history.state, '', location.pathname + location.search);
  if (!pass) return;
  const keepBack = () => {
    try {
      if (back && goodBack(back)) localStorage.setItem(BACK_KEY, back);
    } catch {
      // No storage: the way back is not offered.
    }
  };
  for (const at of ['/api/auth/local', `${offlineBase()}api/auth/local`]) {
    try {
      const res = await fetch(at, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ pass, enter: true }),
        credentials: 'same-origin',
      });
      if (res.ok) {
        keepBack();
        return;
      }
    } catch {
      // The other address below.
    }
  }
}

/** The hub's workspace this loopback page came from; null on the hub itself or without one. */
export function localBack(): string | null {
  if (typeof location === 'undefined' || !LOOPBACK_HOST.test(location.hostname)) return null;
  try {
    return localStorage.getItem(BACK_KEY);
  } catch {
    return null;
  }
}

/** The same place in the hub. */
export function backToHubUrl(back: string): string {
  return back.replace(/\/?$/, '/') + routeHere();
}

/** Whether the hub answers this loopback page (an opaque answer is an answer). */
export async function hubAnswers(back: string): Promise<boolean> {
  const ctrl = new AbortController();
  const cut = setTimeout(() => ctrl.abort(), PING_MS);
  try {
    await fetch(`${back.replace(/\/?$/, '/')}manifest.json?probe=${Date.now()}`, {
      mode: 'no-cors',
      cache: 'no-store',
      credentials: 'omit',
      signal: ctrl.signal,
    });
    return true;
  } catch {
    return false;
  } finally {
    clearTimeout(cut);
  }
}

/**
 * The machine the install named in the address (`?this-computer=<id>`), read as the page starts: the app's router may
 * tidy the address before the person is known and the pairing can begin (offline/this-computer.ts, pairFromAddress).
 */
export const askedThisComputer: string | null = (() => {
  if (typeof location === 'undefined') return null;
  const id = new URLSearchParams(location.search).get('this-computer');
  return id && /^[a-z2-7]{20}$/.test(id) ? id : null;
})();
