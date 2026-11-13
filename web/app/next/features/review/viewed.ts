// «Viewed» marks of the review: per person, per file path, per version of the file. A mark holds the version it was
// put on, so a file that changed since reads as not viewed again. Kept in this browser; a missing or broken storage
// only means the marks last as long as the page. A change on a code host has them on every device: GitHub keeps the
// marks itself (`host`), for the others the machine does (`io`), beside this browser's.
import { useCallback, useEffect, useState, useSyncExternalStore } from 'react';
import type { IReviewFile } from '@/next/features/review/model';

export type TViewedMarks = Readonly<Record<string, string>>;

const PREFIX = 'hx.review.viewed:';
/** Marks kept per person; the oldest go first past this. */
export const VIEWED_MAX = 5000;

export const viewedStorageKey = (person: string): string => `${PREFIX}${person}`;

export const parseViewed = (raw: string | null): TViewedMarks => {
  if (!raw) return {};
  try {
    const value: unknown = JSON.parse(raw);
    if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
    const out: Record<string, string> = {};
    for (const [key, version] of Object.entries(value)) if (typeof version === 'string') out[key] = version;
    return out;
  } catch {
    return {};
  }
};

/** A mark's key: the change's scope (repository or project) and the file path. */
export const markKey = (scope: string, path: string): string => `${scope}\0${path}`;

export const isViewed = (marks: TViewedMarks, scope: string, file: IReviewFile): boolean =>
  marks[markKey(scope, file.path)] === file.version;

/** The marks with this file marked or unmarked at its current version. */
export const setViewed = (
  marks: TViewedMarks,
  scope: string,
  file: IReviewFile,
  viewed: boolean,
): TViewedMarks => {
  const key = markKey(scope, file.path);
  const next: Record<string, string> = { ...marks };
  delete next[key];
  if (viewed) next[key] = file.version;
  const keys = Object.keys(next);
  for (const old of keys.slice(0, Math.max(0, keys.length - VIEWED_MAX))) delete next[old];
  return next;
};

const cache = new Map<string, TViewedMarks>();
const listeners = new Set<() => void>();

const read = (person: string): TViewedMarks => {
  const key = viewedStorageKey(person);
  let marks = cache.get(key);
  if (marks) return marks;
  try {
    marks = parseViewed(window.localStorage.getItem(key));
  } catch {
    marks = {};
  }
  cache.set(key, marks);
  return marks;
};

const write = (person: string, marks: TViewedMarks) => {
  const key = viewedStorageKey(person);
  cache.set(key, marks);
  try {
    window.localStorage.setItem(key, JSON.stringify(marks));
  } catch {
    // Private mode or a full storage: the marks hold for this page.
  }
  for (const listener of [...listeners]) listener();
};

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

/** The machine's marks of one change: path to the version of the file the mark was put on. */
export interface IViewedIo {
  load: (scope: string) => Promise<TViewedMarks | null>;
  /** An empty version takes the mark off. */
  mark: (scope: string, path: string, version: string) => Promise<TViewedMarks | null>;
}

/** The code host's own marks (GitHub): the paths marked there, and the act that marks one. */
export interface IHostViewed {
  paths: readonly string[];
  /** False when the host did not take it. */
  set: (path: string, viewed: boolean) => Promise<boolean>;
}

export interface IViewedOptions {
  io?: IViewedIo | null;
  host?: IHostViewed | null;
}

const NO_MARKS: TViewedMarks = {};

/** The person's marks within one scope and a way to toggle one. */
export const useViewed = (person: string, scope: string, { io = null, host = null }: IViewedOptions = {}) => {
  const marks = useSyncExternalStore(subscribe, () => read(person));
  // What the machine keeps for this scope, as far as it is read; another scope's is not this one's.
  const [kept, setKept] = useState<{ scope: string; marks: TViewedMarks } | null>(null);
  // Marks sent to the host and not answered yet, so that the box does not wait for the round trip.
  const [sent, setSent] = useState<Readonly<Record<string, boolean>>>({});
  const remote = kept?.scope === scope ? kept.marks : NO_MARKS;
  const hostPaths = host?.paths;
  const hosted = hostPaths !== undefined;
  const hostSet = host?.set;

  useEffect(() => {
    if (!io || hosted) return;
    let live = true;
    void io.load(scope).then((loaded) => {
      if (live && loaded) setKept({ scope, marks: loaded });
    });
    return () => {
      live = false;
    };
  }, [io, scope, hosted]);

  const toggle = useCallback(
    (file: IReviewFile, viewed: boolean) => {
      if (hostSet) {
        const settle = () =>
          setSent((was) => {
            const next = { ...was };
            delete next[file.path];
            return next;
          });
        setSent((was) => ({ ...was, [file.path]: viewed }));
        void hostSet(file.path, viewed).then(settle, settle);
        return;
      }
      write(person, setViewed(read(person), scope, file, viewed));
      if (!io) return;
      setKept((was) => {
        const next: Record<string, string> = { ...(was?.scope === scope ? was.marks : {}) };
        delete next[file.path];
        if (viewed) next[file.path] = file.version;
        return { scope, marks: next };
      });
      void io.mark(scope, file.path, viewed ? file.version : '').then((saved) => {
        if (saved) setKept({ scope, marks: saved });
      });
    },
    [person, scope, io, hostSet],
  );
  const viewed = useCallback(
    (file: IReviewFile) => {
      if (hostPaths) return sent[file.path] ?? hostPaths.includes(file.path);
      return isViewed(marks, scope, file) || remote[file.path] === file.version;
    },
    [marks, scope, remote, hostPaths, sent],
  );
  return { viewed, toggle };
};

/** Tests start from an empty cache. */
export const resetViewedCache = () => cache.clear();
