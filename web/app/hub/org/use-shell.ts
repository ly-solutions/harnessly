import { useCallback, useEffect, useState } from 'react';

import { hubCache } from '../lib/cache';
import type { OrgApi } from './api';
import { refusedAccess } from './api/errors';
import { restoreShownCurrency } from './money';
import type { ShellData } from './types';

interface ShellState {
  data: ShellData | null;
  error: unknown;
  /** The address `data` is for. */
  key: string;
  /** `data` is the kept copy of the last visit: drawn at once, the live answer is on its way. */
  stale: boolean;
}

const keyOf = (org: string, scopeSlug: string) => `${org}/${scopeSlug}`;

const seed = (api: OrgApi, org: string, scopeSlug: string): ShellData | null => {
  if (hubCache.person() === null) return null;
  const data = api.cachedShell?.(org, scopeSlug) ?? null;
  if (data) restoreShownCurrency();
  return data;
};

/**
 * The shell data of an organization; reloads when the organization or the scope changes. A copy of the last visit
 * fills it at first (the screens draw at once), the live answer replaces it behind them; a refresh that fails keeps
 * the copy, a refusal drops it.
 */
export const useShellLoad = (api: OrgApi, org: string, scopeSlug: string) => {
  const key = keyOf(org, scopeSlug);
  const [state, setState] = useState<ShellState>(() => {
    const data = seed(api, org, scopeSlug);
    return { data, error: null, key, stale: data !== null };
  });
  const [tick, setTick] = useState(0);
  // Another address: its copy when there is one; otherwise what is shown stays until the live answer, as it did.
  if (state.key !== key) {
    const data = seed(api, org, scopeSlug);
    setState({ data: data ?? state.data, error: null, key, stale: data !== null });
  }
  useEffect(() => {
    let alive = true;
    api.shell(org, scopeSlug).then(
      (data) => alive && setState({ data, error: null, key, stale: false }),
      (error: unknown) => {
        if (!alive) return;
        if (refusedAccess(error)) hubCache.remove(`shell/${org}/${scopeSlug}`);
        // A copy on the screen outlives a failed refresh: nothing scary to say about a hub that did not answer once.
        setState((s) =>
          s.stale && s.key === key && !refusedAccess(error) ? s : { data: null, error, key, stale: false },
        );
      },
    );
    return () => {
      alive = false;
    };
  }, [api, org, scopeSlug, key, tick]);
  const refresh = useCallback(() => setTick((n) => n + 1), []);
  return {
    data: state.data,
    error: state.error,
    stale: state.stale,
    /** Fetches again without clearing what is shown (the tenant was renamed). */
    refresh,
    reload: () => {
      setState((s) => ({ ...s, data: null, error: null, stale: false }));
      setTick((n) => n + 1);
    },
  };
};
