// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest';
import * as config from '@/platform/config';
import { designAddress } from './design-address';
afterEach(() => vi.restoreAllMocks());
it.each(['/agents', '/m/owner-machine'])('copies a board under the workspace root from %s', (basePath) => {
  const cfg = config.appConfig();
  vi.spyOn(config, 'appConfig').mockReturnValue({ ...cfg, basePath, hubUrl: window.location.origin });
  const page = 'Épreuve (2).dc.html';
  const u = new URL(designAddress('sample', page));
  expect(u.pathname).toBe('/agents/designly/sample');
  expect([...u.searchParams.entries()]).toEqual([['page', page]]);
});

it('carries the card key of a copy another space shared (GitHub #186)', () => {
  const cfg = config.appConfig();
  vi.spyOn(config, 'appConfig').mockReturnValue({
    ...cfg,
    basePath: '/agents',
    hubUrl: window.location.origin,
  });
  const u = new URL(designAddress('sample', 'index.dc.html', 'c-0123456789abcdef'));
  expect(u.searchParams.get('card')).toBe('c-0123456789abcdef');
  expect(new URL(designAddress('sample', null)).searchParams.has('card')).toBe(false);
});
