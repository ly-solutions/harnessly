import { afterEach, describe, expect, it } from 'vitest';
import { flush, setScheduler } from '@/next/core/store';
import { checkNet, netStore, noteNoAnswer, onBackOnline, startNet, stopNet, type INetEnv } from './net';

setScheduler('sync');

/** A device whose browser flag, server and timers the test holds. */
const device = (o: { browser?: boolean; server?: boolean } = {}) => {
  const state = { browser: o.browser ?? true, server: o.server ?? true, probes: 0 };
  const handlers = new Map<string, Set<() => void>>();
  const timers = new Map<number, () => void>();
  let next = 1;
  const env: INetEnv = {
    browserOnline: () => state.browser,
    reach: async () => {
      state.probes += 1;
      return state.server;
    },
    on: (event, fn) => {
      const set = handlers.get(event) ?? new Set();
      set.add(fn);
      handlers.set(event, set);
      return () => set.delete(fn);
    },
    setTimeout: (fn) => {
      const id = next++;
      timers.set(id, fn);
      return id;
    },
    clearTimeout: (h) => void timers.delete(h as number),
    now: () => 1000,
  };
  const fire = (event: string) => {
    for (const fn of [...(handlers.get(event) ?? [])]) fn();
  };
  const tick = async () => {
    const all = [...timers.values()];
    timers.clear();
    for (const fn of all) fn();
    await settle();
  };
  return { state, env, fire, tick, timers };
};

const settle = async () => {
  for (let i = 0; i < 5; i += 1) await Promise.resolve();
  flush();
};

afterEach(() => {
  stopNet();
  netStore.set({ online: true, since: 0 });
  flush();
});

describe('device network (HRN-1803)', () => {
  it('goes offline at once when the browser loses its network, and back when the server answers', async () => {
    const d = device();
    startNet(d.env);
    await settle();
    expect(netStore.get().online).toBe(true);

    d.state.browser = false;
    d.fire('offline');
    flush();
    expect(netStore.get()).toEqual({ online: false, since: 1000 });

    d.state.browser = true;
    d.fire('online');
    await settle();
    expect(netStore.get().online).toBe(true);
  });

  it('believes a browser that says «online» only once the server answers', async () => {
    const d = device({ server: false });
    startNet(d.env);
    await settle();
    expect(netStore.get().online).toBe(false);
    // While offline it asks the server again on its own.
    expect(d.timers.size).toBe(1);
    d.state.server = true;
    await d.tick();
    expect(netStore.get().online).toBe(true);
    expect(d.timers.size).toBe(0);
  });

  it('checks, not believes, a request that found no network', async () => {
    const d = device();
    startNet(d.env);
    await settle();
    const before = d.state.probes;
    noteNoAnswer();
    await settle();
    expect(d.state.probes).toBe(before + 1);
    expect(netStore.get().online).toBe(true);

    d.state.server = false;
    noteNoAnswer();
    await settle();
    expect(netStore.get().online).toBe(false);
  });

  it('tells a page waiting for the network that the device is back, once per return', async () => {
    const d = device({ server: false });
    startNet(d.env);
    await settle();
    let back = 0;
    const stop = onBackOnline(() => (back += 1));
    d.state.server = true;
    await checkNet();
    await settle();
    await checkNet();
    await settle();
    expect(back).toBe(1);
    stop();
  });
});
