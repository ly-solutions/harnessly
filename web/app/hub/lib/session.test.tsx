// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from './api';
import { hubCache } from './cache';
import type { Config, Me } from './types';

const state = vi.hoisted(() => ({
  api: {} as { config: () => Promise<unknown>; me: { get: () => Promise<unknown> } },
}));
vi.mock('./env', () => ({ useApi: () => state.api }));

import { SessionProvider, sessionCopy, useSession } from './session';

const config = {
  version: '1',
  publicUrl: '',
  edition: 'cloud',
  signup: 'open',
  auth: {},
} as unknown as Config;
const me = (id: string) =>
  ({
    user: { id, name: id, email: `${id}@x.ru` },
    identities: [],
    passkeys: [],
    spaces: [],
  }) as unknown as Me;

const Who = () => {
  const s = useSession();
  return <p>{s.me ? `me:${s.me.user.id}` : 'signed-out'}</p>;
};
const mount = () =>
  render(
    <SessionProvider failed={(e) => <p>failed:{e.code}</p>} loading={<p>loading</p>}>
      <Who />
    </SessionProvider>,
  );
const never = () => new Promise<never>(() => {});

beforeEach(() => {
  localStorage.clear();
  hubCache.clear();
});
afterEach(() => cleanup());

describe('session copy', () => {
  it('accepts a signed-in or signed-out copy and nothing else', () => {
    expect(sessionCopy({ config, me: me('u1') })?.me?.user.id).toBe('u1');
    expect(sessionCopy({ config, me: null })?.me).toBeNull();
    expect(sessionCopy(null)).toBeNull();
    expect(sessionCopy({ config: {}, me: null })).toBeNull();
    expect(sessionCopy({ config, me: { user: {} } })).toBeNull();
  });
});

describe('SessionProvider', () => {
  it('shows loading and keeps a copy when there is none', async () => {
    state.api = { config: () => Promise.resolve(config), me: { get: () => Promise.resolve(me('u1')) } };
    mount();
    expect(screen.getByText('loading')).toBeTruthy();
    await screen.findByText('me:u1');
    expect(hubCache.getShared('session')).toMatchObject({ me: { user: { id: 'u1' } } });
  });

  it('draws from the copy while the live answers are on their way', async () => {
    hubCache.setShared('session', { config, me: me('u1') });
    state.api = { config: never, me: { get: never } };
    mount();
    expect(screen.getByText('me:u1')).toBeTruthy();
  });

  it('asks the configuration and the person at the same time', async () => {
    const calls: string[] = [];
    state.api = {
      config: () => (calls.push('config'), never()),
      me: { get: () => (calls.push('me'), never()) },
    };
    mount();
    await waitFor(() => expect(calls).toEqual(['me', 'config']));
  });

  it('goes to signed-out and drops everything when the live me says so', async () => {
    hubCache.setShared('session', { config, me: me('u1') });
    hubCache.setPerson('u1');
    hubCache.set('machines', [1]);
    state.api = {
      config: () => Promise.resolve(config),
      me: { get: () => Promise.reject(new ApiError(401, 'unauthenticated', '')) },
    };
    mount();
    await screen.findByText('signed-out');
    expect(hubCache.person()).toBeNull();
    expect(hubCache.getShared('session')).toMatchObject({ me: null });
    expect(Object.keys(localStorage).some((k) => k.includes('machines'))).toBe(false);
  });

  it('keeps the copy when the live refresh fails for a reason that is not a refusal', async () => {
    hubCache.setShared('session', { config, me: me('u1') });
    state.api = { config: () => Promise.reject(new ApiError(0, 'network', '')), me: { get: never } };
    mount();
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.getByText('me:u1')).toBeTruthy();
  });

  it('drops the copy on a 403', async () => {
    hubCache.setShared('session', { config, me: me('u1') });
    state.api = {
      config: () => Promise.resolve(config),
      me: { get: () => Promise.reject(new ApiError(403, 'forbidden', '')) },
    };
    mount();
    await screen.findByText('failed:forbidden');
    expect(hubCache.getShared('session')).toBeNull();
  });

  it('shows the error when there was no copy', async () => {
    state.api = { config: () => Promise.reject(new ApiError(0, 'network', '')), me: { get: never } };
    mount();
    await screen.findByText('failed:network');
  });
});
