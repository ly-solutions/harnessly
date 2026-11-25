// Data hooks of the organization hub. A load is `null` until it arrives (never `[]` — empty is a result, not a wait);
// a failure stays in `error` and is drawn, never replaced by stand-in data. A mutation is pessimistic: `pending` while
// it runs, the screen changes only after it succeeds.
import { useCallback, useEffect, useState } from 'react';

import { hubCache } from '../lib/cache';
import { type OrgApi } from './api';
import { refusedAccess } from './api/errors';
import { useOrg } from './context';
import { dataMode } from './mode';

export interface Loaded<T> {
  data: T | null;
  error: unknown;
  reload: () => void;
  /** Fetches again without clearing what is shown. */
  refresh: () => void;
  /** `data` is the kept copy of an earlier visit and the live answer has not come yet. */
  stale: boolean;
}

interface LoadState<T> {
  data: T | null;
  error: unknown;
  stale: boolean;
}

/** The kept copy of a load, or null. Only in live mode, only for a person we know, only for a named load. */
export const kept = <T>(key: string | null): T | null => {
  if (key === null || hubCache.person() === null || dataMode() !== 'live') return null;
  return hubCache.get<T>(key);
};

/**
 * Loads data for a screen. `name` (when given) makes the answer worth keeping: the next visit draws from the copy at
 * once, under the same organization, scope, name and `deps`, and the live answer replaces it behind the screen
 * (stale-while-revalidate). Name only loads that carry nothing secret. What a live answer was parsed into is
 * what is kept, so a copy is shown through no looser reading than a live one.
 */
export const useLoad = <T>(
  load: (api: OrgApi) => Promise<T>,
  deps: readonly unknown[],
  name?: string,
): Loaded<T> => {
  const { api, shell, scope } = useOrg();
  // New inputs: forget the old answer during render, so the screen shows "loading" and not a stale result; the copy
  // kept for the new inputs, if there is one, is shown instead.
  const depsKey = JSON.stringify(deps);
  const scopeSlug = scope.kind === 'org' ? 'all' : scope.tenant.slug;
  const cacheKey = name === undefined ? null : `load/${shell.org.slug}/${scopeSlug}/${name}/${depsKey}`;
  const seeded = (): LoadState<T> => {
    const data = kept<T>(cacheKey);
    return { data, error: null, stale: data !== null };
  };
  const [state, setState] = useState<LoadState<T>>(seeded);
  const [tick, setTick] = useState(0);
  const [seen, setSeen] = useState(cacheKey ?? depsKey);
  if (seen !== (cacheKey ?? depsKey)) {
    setSeen(cacheKey ?? depsKey);
    setState(seeded());
  }
  useEffect(() => {
    let alive = true;
    load(api).then(
      (data) => {
        if (!alive) return;
        if (cacheKey !== null) hubCache.set(cacheKey, data);
        setState({ data, error: null, stale: false });
      },
      (error: unknown) => {
        if (!alive) return;
        const refused = refusedAccess(error);
        if (refused && cacheKey !== null) hubCache.remove(cacheKey);
        // A copy on the screen outlives a refresh that failed for any reason but a refusal.
        setState((s) => (s.stale && !refused ? s : { data: null, error, stale: false }));
      },
    );
    return () => {
      alive = false;
    };
    // `deps` (as depsKey) say when to refetch; `load` is the caller's closure over them.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, tick, cacheKey, depsKey]);
  const reload = useCallback(() => {
    setState({ data: null, error: null, stale: false });
    setTick((n) => n + 1);
  }, []);
  const refresh = useCallback(() => setTick((n) => n + 1), []);
  return { ...state, reload, refresh };
};

export interface Action<A extends unknown[], R> {
  run: (...args: A) => Promise<R | undefined>;
  pending: boolean;
  error: unknown;
  reset: () => void;
}

export const useAction = <A extends unknown[], R>(
  fn: (api: OrgApi, ...args: A) => Promise<R>,
): Action<A, R> => {
  const { api } = useOrg();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const run = useCallback(
    async (...args: A) => {
      setPending(true);
      setError(null);
      try {
        return await fn(api, ...args);
      } catch (e) {
        setError(e);
        return undefined;
      } finally {
        setPending(false);
      }
    },
    [api, fn],
  );
  return { run, pending, error, reset: () => setError(null) };
};
