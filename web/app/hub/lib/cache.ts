// What the hub's screens last showed a person, kept in localStorage so a reload draws at once from it and then reads
// the hub again behind the screen (stale-while-revalidate). Nothing here is trusted: a screen takes a copy
// back only through the code that parses live answers, and a failed or refused refresh drops it.
//
// Keys: `hlc1:<person>:<key>`, the person being the signed-in user's id, so a copy never reaches another person of the
// same browser. A key without a person (`getShared`) is for what is read before the person is known (the session).
// The value is `<written at, ms>|<json>`. The whole store is capped; the oldest write goes first.
//
// Never put here: tokens, invite tokens or links, env values or anything of the safe/vault, provider keys. The
// `denied` rule below refuses such keys as a second line; the first is that callers name what they keep.

const PREFIX = 'hlc1:';
const SHARED = '*';
/** All entries together, in bytes of the stored text (two per UTF-16 unit). */
export const MAX_TOTAL_BYTES = 2 * 1024 * 1024;
/** One entry; a bigger answer is simply not kept. */
export const MAX_ENTRY_BYTES = 512 * 1024;

/** Key parts that say the data is a secret or carries one. */
const DENIED =
  /(^|[/:._-])(env|secrets?|tokens?|invites?|invite-?links?|safe|vault|provider-?keys?|api-?keys?|credentials?|passkeys?|codes?|sealed)([/:._-]|$)/i;

export const denied = (key: string): boolean => DENIED.test(key);

export interface CacheStorage {
  readonly length: number;
  key(i: number): string | null;
  getItem(k: string): string | null;
  setItem(k: string, v: string): void;
  removeItem(k: string): void;
}

interface Meta {
  at: number;
  bytes: number;
}

export interface HubCache {
  /** Whose copies are read and written; null: none (nothing is kept). Copies of any other person are dropped. */
  setPerson(id: string | null): void;
  person(): string | null;
  get<T>(key: string): T | null;
  set(key: string, data: unknown): void;
  remove(key: string): void;
  /** For what is read before the person is known. */
  getShared<T>(key: string): T | null;
  setShared(key: string, data: unknown): void;
  /** Forgets everything (a sign-out, a person that is not who the copy was for). */
  clear(): void;
  /** Bytes in use (the stored text, two per unit). */
  size(): number;
}

const emptyStorage: CacheStorage = {
  length: 0,
  key: () => null,
  getItem: () => null,
  setItem: () => {},
  removeItem: () => {},
};

export const createCache = (storage: () => CacheStorage | null, now: () => number = Date.now): HubCache => {
  let who: string | null = null;
  // Built from the storage on first use: every key of ours with when it was written and how big it is.
  let index: Map<string, Meta> | null = null;

  const open = (): CacheStorage | null => {
    try {
      return storage();
    } catch {
      return null;
    }
  };

  const meta = (st: CacheStorage): Map<string, Meta> => {
    if (index) return index;
    const m = new Map<string, Meta>();
    try {
      for (let i = 0; i < st.length; i++) {
        const k = st.key(i);
        if (!k?.startsWith(PREFIX)) continue;
        const v = st.getItem(k) ?? '';
        const at = Number(v.slice(0, v.indexOf('|')));
        m.set(k, { at: Number.isFinite(at) ? at : 0, bytes: (k.length + v.length) * 2 });
      }
    } catch {
      /* Storage unreadable: an empty index, the writes will say. */
    }
    index = m;
    return m;
  };

  const drop = (st: CacheStorage, k: string) => {
    index?.delete(k);
    try {
      st.removeItem(k);
    } catch {
      /* Nothing to do. */
    }
  };

  const total = (m: Map<string, Meta>) => {
    let n = 0;
    for (const v of m.values()) n += v.bytes;
    return n;
  };

  const oldestFirst = (m: Map<string, Meta>) => [...m].sort((a, b) => a[1].at - b[1].at);

  /** Drops the oldest entries until `room` more bytes fit under the cap. */
  const makeRoom = (st: CacheStorage, room: number) => {
    const m = meta(st);
    let used = total(m);
    for (const [k, v] of oldestFirst(m)) {
      if (used + room <= MAX_TOTAL_BYTES) break;
      used -= v.bytes;
      drop(st, k);
    }
  };

  const read = <T>(storageKey: string): T | null => {
    const st = open();
    if (!st) return null;
    try {
      const v = st.getItem(storageKey);
      if (v === null) return null;
      return JSON.parse(v.slice(v.indexOf('|') + 1)) as T;
    } catch {
      // A copy that does not parse is no copy.
      drop(st, storageKey);
      return null;
    }
  };

  const write = (storageKey: string, data: unknown) => {
    const st = open();
    if (!st) return;
    let json: string | undefined;
    try {
      json = JSON.stringify(data);
    } catch {
      return;
    }
    if (json === undefined) return;
    const stamp = now();
    const value = `${stamp}|${json}`;
    const bytes = (storageKey.length + value.length) * 2;
    if (bytes > MAX_ENTRY_BYTES) {
      drop(st, storageKey);
      return;
    }
    const m = meta(st);
    m.delete(storageKey);
    makeRoom(st, bytes);
    for (let attempt = 0; attempt < 2; attempt++) {
      try {
        st.setItem(storageKey, value);
        m.set(storageKey, { at: stamp, bytes });
        return;
      } catch {
        // Quota: the browser's own limit is lower than ours. Let go of the older half and try once more.
        const old = oldestFirst(m);
        for (const [k] of old.slice(0, Math.ceil(old.length / 2))) drop(st, k);
      }
    }
    drop(st, storageKey);
  };

  const keyOf = (person: string, key: string) => `${PREFIX}${encodeURIComponent(person)}:${key}`;

  return {
    person: () => who,
    setPerson(id) {
      if (id === who) return;
      who = id;
      const st = open();
      if (!st) return;
      // Copies of someone else go; the shared ones are the session's to judge.
      const mine = id === null ? null : `${PREFIX}${encodeURIComponent(id)}:`;
      for (const k of [...meta(st).keys()]) {
        if (!k.startsWith(`${PREFIX}${SHARED}:`) && !(mine && k.startsWith(mine))) drop(st, k);
      }
    },
    get<T>(key: string) {
      return who === null || denied(key) ? null : read<T>(keyOf(who, key));
    },
    set(key, data) {
      if (who === null || denied(key)) return;
      write(keyOf(who, key), data);
    },
    remove(key) {
      const st = open();
      if (!st || who === null) return;
      drop(st, keyOf(who, key));
    },
    getShared<T>(key: string) {
      return denied(key) ? null : read<T>(keyOf(SHARED, key));
    },
    setShared(key, data) {
      if (denied(key)) return;
      write(keyOf(SHARED, key), data);
    },
    clear() {
      who = null;
      const st = open();
      if (!st) return;
      try {
        const keys: string[] = [];
        for (let i = 0; i < st.length; i++) {
          const k = st.key(i);
          if (k?.startsWith(PREFIX)) keys.push(k);
        }
        for (const k of keys) st.removeItem(k);
      } catch {
        /* Storage blocked. */
      }
      index = new Map();
    },
    size: () => total(meta(open() ?? emptyStorage)),
  };
};

/** The browser's: localStorage, or nothing where it is blocked. */
export const hubCache = createCache(() => {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage;
  } catch {
    return null;
  }
});
