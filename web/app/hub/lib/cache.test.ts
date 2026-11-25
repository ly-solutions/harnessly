import { beforeEach, describe, expect, it } from 'vitest';

import { type CacheStorage, createCache, denied, MAX_ENTRY_BYTES, MAX_TOTAL_BYTES } from './cache';

class MemoryStorage implements CacheStorage {
  readonly map = new Map<string, string>();
  /** Refuses a write past this many characters in total, like a browser's quota. */
  quota = Infinity;
  get length() {
    return this.map.size;
  }
  key(i: number) {
    return [...this.map.keys()][i] ?? null;
  }
  getItem(k: string) {
    return this.map.get(k) ?? null;
  }
  setItem(k: string, v: string) {
    let n = v.length;
    for (const [key, val] of this.map) if (key !== k) n += val.length;
    if (n > this.quota) throw new Error('QuotaExceededError');
    this.map.set(k, v);
  }
  removeItem(k: string) {
    this.map.delete(k);
  }
}

let st: MemoryStorage;
let clock: number;
const make = () =>
  createCache(
    () => st,
    () => ++clock,
  );

beforeEach(() => {
  st = new MemoryStorage();
  clock = 1000;
});

describe('hub cache', () => {
  it('keeps nothing until the person is known', () => {
    const c = make();
    c.set('overview', { a: 1 });
    expect(st.map.size).toBe(0);
    expect(c.get('overview')).toBeNull();
  });

  it('round-trips a value per person and never shows it to another', () => {
    const c = make();
    c.setPerson('u1');
    c.set('machines', { list: [1, 2] });
    expect(c.get('machines')).toEqual({ list: [1, 2] });
    c.setPerson('u2');
    expect(c.get('machines')).toBeNull();
    // The first person's copies are gone, not just hidden.
    expect([...st.map.keys()].some((k) => k.includes('u1'))).toBe(false);
  });

  it('survives a new cache over the same storage (a reload)', () => {
    const a = make();
    a.setPerson('u1');
    a.set('k', [1]);
    const b = make();
    b.setPerson('u1');
    expect(b.get('k')).toEqual([1]);
  });

  it('keeps shared entries across a change of person until clear', () => {
    const c = make();
    c.setShared('session', { me: 1 });
    c.setPerson('u1');
    expect(c.getShared('session')).toEqual({ me: 1 });
    c.clear();
    expect(c.getShared('session')).toBeNull();
    expect(c.person()).toBeNull();
    expect(st.map.size).toBe(0);
  });

  it('clear leaves foreign keys of the storage alone', () => {
    st.setItem('hl-hub-last-org', 'acme');
    const c = make();
    c.setPerson('u1');
    c.set('k', 1);
    c.clear();
    expect([...st.map.keys()]).toEqual(['hl-hub-last-org']);
  });

  it('refuses keys that name a secret', () => {
    for (const key of [
      'env/acme/all',
      'tokens',
      'invite-link/x',
      'load/acme/all/invites/[]',
      'vault',
      'safe/list',
      'provider-keys',
      'machine/codes',
    ]) {
      expect(denied(key), key).toBe(true);
    }
    for (const key of [
      'overview',
      'machines',
      'teams/acme/all',
      'capabilities/acme',
      'accounts',
      'session',
    ]) {
      expect(denied(key), key).toBe(false);
    }
    const c = make();
    c.setPerson('u1');
    c.set('env/acme', { value: 'x' });
    c.setShared('tokens', 1);
    expect(st.map.size).toBe(0);
    expect(c.get('env/acme')).toBeNull();
  });

  it('drops an entry that does not parse', () => {
    const c = make();
    c.setPerson('u1');
    c.set('k', { a: 1 });
    const [key] = [...st.map.keys()];
    st.map.set(key!, '5|{broken');
    expect(c.get('k')).toBeNull();
    expect(st.map.has(key!)).toBe(false);
  });

  it('does not keep one huge entry', () => {
    const c = make();
    c.setPerson('u1');
    c.set('big', 'x'.repeat(MAX_ENTRY_BYTES));
    expect(c.get('big')).toBeNull();
    expect(c.size()).toBe(0);
  });

  it('evicts the oldest writes to stay under the total cap', () => {
    const c = make();
    c.setPerson('u1');
    const chunk = 'y'.repeat(MAX_ENTRY_BYTES / 2 - 200);
    for (let i = 0; i < 8; i++) c.set(`k${i}`, chunk);
    expect(c.size()).toBeLessThanOrEqual(MAX_TOTAL_BYTES);
    expect(c.get('k0')).toBeNull();
    expect(c.get('k7')).toBe(chunk);
  });

  it('rewriting a key makes it the newest', () => {
    const c = make();
    c.setPerson('u1');
    const chunk = 'z'.repeat(MAX_ENTRY_BYTES / 2 - 200);
    c.set('a', chunk);
    c.set('b', chunk);
    c.set('a', chunk);
    for (let i = 0; i < 5; i++) c.set(`n${i}`, chunk);
    expect(c.get('a')).toBeNull();
    expect(c.get('n4')).toBe(chunk);
  });

  it('survives a storage that refuses writes', () => {
    const c = make();
    c.setPerson('u1');
    c.set('old', 'o'.repeat(100));
    st.quota = 50;
    expect(() => c.set('k', 'v'.repeat(100))).not.toThrow();
    expect(c.get('k')).toBeNull();
  });

  it('frees the older half when the browser quota is below ours', () => {
    const c = make();
    c.setPerson('u1');
    for (let i = 0; i < 4; i++) c.set(`k${i}`, 'a'.repeat(100));
    st.quota = 700;
    c.set('fresh', 'b'.repeat(100));
    expect(c.get('fresh')).toEqual('b'.repeat(100));
  });

  it('works with no storage at all', () => {
    const c = createCache(() => null);
    c.setPerson('u1');
    c.set('k', 1);
    expect(c.get('k')).toBeNull();
    expect(() => c.clear()).not.toThrow();
  });

  it('works when the storage getter throws', () => {
    const c = createCache(() => {
      throw new Error('blocked');
    });
    c.setPerson('u1');
    expect(() => c.set('k', 1)).not.toThrow();
    expect(c.get('k')).toBeNull();
  });

  it('indexes copies written by an earlier page when it first writes', () => {
    const a = make();
    a.setPerson('u1');
    const chunk = 'q'.repeat(MAX_ENTRY_BYTES / 2 - 200);
    for (let i = 0; i < 4; i++) a.set(`old${i}`, chunk);
    const b = make();
    b.setPerson('u1');
    for (let i = 0; i < 4; i++) b.set(`new${i}`, chunk);
    expect(b.size()).toBeLessThanOrEqual(MAX_TOTAL_BYTES);
    expect(b.get('old0')).toBeNull();
    expect(b.get('new3')).toBe(chunk);
  });
});
