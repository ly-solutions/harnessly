// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { hubCache } from '../lib/cache';
import type { OrgApi } from './api';
import { OrgApiError } from './api/errors';

const state = vi.hoisted(() => ({ ctx: {} as unknown }));

vi.mock('./context', () => ({ useOrg: () => state.ctx }));
vi.mock('./mode', () => ({ dataMode: () => 'live' }));

import { useLoad } from './hooks';

const api = {} as OrgApi;
const KEY = (deps: string) => `load/acme/all/things/${deps}`;

const deferred = <T,>() => {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
};

beforeEach(() => {
  localStorage.clear();
  hubCache.clear();
  hubCache.setPerson('u1');
  state.ctx = { api, shell: { org: { slug: 'acme' } }, scope: { kind: 'org' } };
});
afterEach(() => cleanup());

describe('useLoad with a name', () => {
  it('draws from the kept copy at once and swaps in the live answer', async () => {
    hubCache.set(KEY('[1]'), { n: 'old' });
    const d = deferred<{ n: string }>();
    const { result } = renderHook(() => useLoad(() => d.promise, [1], 'things'));
    expect(result.current.data).toEqual({ n: 'old' });
    expect(result.current.stale).toBe(true);
    await act(async () => d.resolve({ n: 'new' }));
    expect(result.current.data).toEqual({ n: 'new' });
    expect(result.current.stale).toBe(false);
    expect(hubCache.get(KEY('[1]'))).toEqual({ n: 'new' });
  });

  it('starts empty when nothing is kept, and keeps the first answer for the next visit', async () => {
    const { result } = renderHook(() => useLoad(() => Promise.resolve([1, 2]), [1], 'things'));
    expect(result.current.data).toBeNull();
    await waitFor(() => expect(result.current.data).toEqual([1, 2]));
    expect(hubCache.get(KEY('[1]'))).toEqual([1, 2]);
  });

  it('keeps the copy and says nothing when the refresh fails', async () => {
    hubCache.set(KEY('[1]'), { n: 'old' });
    const d = deferred<unknown>();
    const { result } = renderHook(() => useLoad(() => d.promise, [1], 'things'));
    await act(async () => d.reject(new OrgApiError('network', '')));
    expect(result.current.data).toEqual({ n: 'old' });
    expect(result.current.error).toBeNull();
    expect(hubCache.get(KEY('[1]'))).toEqual({ n: 'old' });
  });

  it.each(['forbidden', 'not_found', 'unauthenticated'] as const)(
    'drops the copy when the hub answers %s',
    async (code) => {
      hubCache.set(KEY('[1]'), { n: 'old' });
      const d = deferred<unknown>();
      const { result } = renderHook(() => useLoad(() => d.promise, [1], 'things'));
      await act(async () => d.reject(new OrgApiError(code, '')));
      expect(result.current.data).toBeNull();
      expect(result.current.error).toBeInstanceOf(OrgApiError);
      expect(hubCache.get(KEY('[1]'))).toBeNull();
    },
  );

  it('shows the error when there was no copy', async () => {
    const { result } = renderHook(() =>
      useLoad(() => Promise.reject(new OrgApiError('network', '')), [1], 'things'),
    );
    await waitFor(() => expect(result.current.error).toBeInstanceOf(OrgApiError));
    expect(result.current.data).toBeNull();
  });

  it('takes the copy of the new inputs when deps change, or shows loading', async () => {
    hubCache.set(KEY('[2]'), 'two');
    const gate = deferred<string>();
    const { result, rerender } = renderHook(
      ({ n }) => useLoad(() => (n === 1 ? Promise.resolve('one') : gate.promise), [n], 'things'),
      {
        initialProps: { n: 1 },
      },
    );
    await waitFor(() => expect(result.current.data).toBe('one'));
    rerender({ n: 2 });
    expect(result.current.data).toBe('two');
    expect(result.current.stale).toBe(true);
    rerender({ n: 3 });
    expect(result.current.data).toBeNull();
    gate.resolve('late');
  });

  it('refresh keeps the shown data and writes the new answer behind it', async () => {
    let n = 0;
    const { result } = renderHook(() => useLoad(() => Promise.resolve(++n), [1], 'things'));
    await waitFor(() => expect(result.current.data).toBe(1));
    act(() => result.current.refresh());
    expect(result.current.data).toBe(1);
    await waitFor(() => expect(result.current.data).toBe(2));
    expect(hubCache.get(KEY('[1]'))).toBe(2);
  });

  it('keeps nothing for a load without a name', async () => {
    const { result } = renderHook(() => useLoad(() => Promise.resolve(7), [1]));
    await waitFor(() => expect(result.current.data).toBe(7));
    expect(localStorage.length).toBe(0);
  });

  it('does not read a copy of another person', () => {
    hubCache.set(KEY('[1]'), 'mine');
    hubCache.setPerson('u2');
    const { result } = renderHook(() => useLoad(() => new Promise<string>(() => {}), [1], 'things'));
    expect(result.current.data).toBeNull();
  });

  it('keeps the copies of one scope apart from another', () => {
    hubCache.set(KEY('[1]'), 'all');
    state.ctx = {
      api,
      shell: { org: { slug: 'acme' } },
      scope: { kind: 'tenant', tenant: { slug: 'prod' } },
    };
    const { result } = renderHook(() => useLoad(() => new Promise<string>(() => {}), [1], 'things'));
    expect(result.current.data).toBeNull();
  });
});
