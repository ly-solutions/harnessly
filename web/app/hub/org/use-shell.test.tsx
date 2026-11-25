// @vitest-environment jsdom
import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { hubCache } from '../lib/cache';
import type { OrgApi } from './api';
import { OrgApiError } from './api/errors';
import type { ShellData } from './types';
import { useShellLoad } from './use-shell';

const shellOf = (name: string) => ({ org: { slug: 'acme', name } }) as unknown as ShellData;
const deferred = <T,>() => {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
};
const apiOf = (shell: OrgApi['shell'], cachedShell?: OrgApi['cachedShell']) =>
  ({ shell, cachedShell }) as unknown as OrgApi;

beforeEach(() => {
  localStorage.clear();
  hubCache.clear();
  hubCache.setPerson('u1');
});
afterEach(() => cleanup());

describe('useShellLoad', () => {
  it('draws from the kept shell at once and replaces it with the live one', async () => {
    const d = deferred<ShellData>();
    const api = apiOf(
      () => d.promise,
      () => shellOf('old'),
    );
    const { result } = renderHook(() => useShellLoad(api, 'acme', 'all'));
    expect(result.current.data).toEqual(shellOf('old'));
    expect(result.current.stale).toBe(true);
    await act(async () => d.resolve(shellOf('new')));
    expect(result.current.data).toEqual(shellOf('new'));
    expect(result.current.stale).toBe(false);
  });

  it('keeps the copy when the live read fails quietly', async () => {
    const d = deferred<ShellData>();
    const api = apiOf(
      () => d.promise,
      () => shellOf('old'),
    );
    const { result } = renderHook(() => useShellLoad(api, 'acme', 'all'));
    await act(async () => d.reject(new OrgApiError('network', '')));
    expect(result.current.data).toEqual(shellOf('old'));
    expect(result.current.error).toBeNull();
  });

  it('drops the copy and shows the error when the hub refuses', async () => {
    hubCache.set('shell/acme/all', { x: 1 });
    const d = deferred<ShellData>();
    const api = apiOf(
      () => d.promise,
      () => shellOf('old'),
    );
    const { result } = renderHook(() => useShellLoad(api, 'acme', 'all'));
    await act(async () => d.reject(new OrgApiError('forbidden', '')));
    expect(result.current.data).toBeNull();
    expect(result.current.error).toBeInstanceOf(OrgApiError);
    expect(hubCache.get('shell/acme/all')).toBeNull();
  });

  it('does not look for a copy for an unknown person, or in an api that keeps none', () => {
    const cached = vi.fn(() => shellOf('old'));
    hubCache.setPerson(null);
    const a = renderHook(() =>
      useShellLoad(
        apiOf(() => new Promise(() => {}), cached),
        'acme',
        'all',
      ),
    );
    expect(a.result.current.data).toBeNull();
    expect(cached).not.toHaveBeenCalled();
    hubCache.setPerson('u1');
    const b = renderHook(() =>
      useShellLoad(
        apiOf(() => new Promise(() => {})),
        'acme',
        'all',
      ),
    );
    expect(b.result.current.data).toBeNull();
  });
});
