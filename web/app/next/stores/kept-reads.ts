// the last answers of the reads a screen waits for (the person's machines, me, the sidebar's lists, the installed
// apps…), kept in the person's IndexedDB (core/persist.ts, area `reads`), so a reload draws them at once and the live
// answer replaces them when it comes. Only what is the person's own and holds no secret goes here: a read is kept by the
// code that made it, after its own parsing, and read back through the same parsing before it is drawn.

type Listener = (key: string, value: unknown) => void;

const kept = new Map<string, unknown>();
const listeners = new Set<Listener>();
const restoredListeners = new Set<() => void>();

/** What the last page (or this one) read for the key; undefined when nothing is kept. */
export const keptRead = <T>(key: string): T | undefined => kept.get(key) as T | undefined;

/** Keeps a live answer: the page holds it now and the persistence writes it a moment later. */
export function keepRead(key: string, value: unknown): void {
  if (value === undefined) return;
  kept.set(key, value);
  for (const l of [...listeners]) l(key, value);
}

/** Drops what is kept for the key (an answer that says the data is gone, or no longer the person's). */
export function dropRead(key: string): void {
  if (!kept.has(key)) return;
  kept.delete(key);
  for (const l of [...listeners]) l(key, undefined);
}

/** The kept answers of the last page; one read live since the page opened is the newer word and stays. */
export function restoreReads(entries: readonly (readonly [string, unknown])[]): void {
  for (const [k, v] of entries) if (!kept.has(k) && v !== undefined) kept.set(k, v);
  for (const l of [...restoredListeners]) l();
}

/** Called with each answer kept (undefined: dropped), not with the restored ones. */
export function onKeepRead(l: Listener): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}

/** Called once the last page's answers are restored. */
export function onReadsRestored(l: () => void): () => void {
  restoredListeners.add(l);
  return () => restoredListeners.delete(l);
}

/** Another person (or nobody) from here: nothing of the previous one stays in memory. */
export function forgetReads(): void {
  kept.clear();
}
