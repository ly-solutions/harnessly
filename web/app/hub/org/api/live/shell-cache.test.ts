// @vitest-environment jsdom
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { OrgView } from '../../../../hub-api/orgs';
import { hubCache } from '../../../lib/cache';
import { liveShell } from './shell';

const view = JSON.parse(
  readFileSync(resolve(import.meta.dirname, '../../../../hub-api/fixtures/orgs/view-admin.json'), 'utf8'),
) as OrgView;
const list = { orgs: [{ slug: view.org.slug, name: view.org.name, role: 'admin' }], looseSpaces: [] };
const me = {
  user: { id: view.me.userId, name: 'Анна', email: 'a@x.ru' },
  identities: [],
  passkeys: [],
  spaces: [],
};

const answer = (body: unknown) =>
  Promise.resolve(
    new Response(JSON.stringify(body), { status: 200, headers: { 'content-type': 'application/json' } }),
  );

beforeEach(() => {
  localStorage.clear();
  hubCache.clear();
  hubCache.setPerson(view.me.userId);
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      if (url === '/api/orgs') return answer(list);
      if (url === '/api/me') return answer(me);
      if (url.includes('/members')) return answer({ members: [] });
      return answer(view);
    }),
  );
});
afterEach(() => vi.unstubAllGlobals());

describe('the shell from a kept copy', () => {
  it('has nothing before the first live read', () => {
    const remember = vi.fn();
    expect(liveShell(remember).cachedShell?.(view.org.slug, 'all')).toBeNull();
    expect(remember).not.toHaveBeenCalled();
  });

  it('is made by the same code as the live shell, and tells the api where it is', async () => {
    const live = await liveShell(vi.fn()).shell(view.org.slug, 'all');
    const remember = vi.fn();
    const cached = liveShell(remember).cachedShell?.(view.org.slug, 'all');
    expect(cached).toEqual(live);
    expect(remember).toHaveBeenCalledOnce();
  });

  it('is not given to another person', async () => {
    await liveShell(vi.fn()).shell(view.org.slug, 'all');
    hubCache.setPerson('somebody-else');
    expect(liveShell(vi.fn()).cachedShell?.(view.org.slug, 'all')).toBeNull();
  });

  it('is refused when it is not shaped like an answer, and dropped', async () => {
    await liveShell(vi.fn()).shell(view.org.slug, 'all');
    hubCache.set(`shell/${view.org.slug}/all`, { view: {}, list: 1 });
    expect(liveShell(vi.fn()).cachedShell?.(view.org.slug, 'all')).toBeNull();
    expect(hubCache.get(`shell/${view.org.slug}/all`)).toBeNull();
  });

  it('is refused for a tenant the copy does not have', async () => {
    await liveShell(vi.fn()).shell(view.org.slug, 'all');
    hubCache.set(`shell/${view.org.slug}/gone`, hubCache.get(`shell/${view.org.slug}/all`));
    expect(liveShell(vi.fn()).cachedShell?.(view.org.slug, 'gone')).toBeNull();
  });
});
