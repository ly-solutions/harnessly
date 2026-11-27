import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  KIT_PART,
  kitPreviewHref,
  fetchProjectHistory,
  nextVersion,
  restoreProjectVersion,
  storeVersionPageHref,
  versionPageHref,
  versionsVia,
} from '@/next/features/designly/versions/versions-api';

describe('versions-api', () => {
  it('counts the next version as the runtime does', () => {
    expect(nextVersion('0.5.1', KIT_PART.minor)).toBe('0.6.0');
    expect(nextVersion('0.5.1', KIT_PART.patch)).toBe('0.5.2');
    expect(nextVersion('0.5.1', KIT_PART.major)).toBe('1.0.0');
    expect(nextVersion('1.2.3-rc.1', KIT_PART.patch)).toBe('1.2.4');
  });

  it('addresses a page at a version and on another kit version', () => {
    const rev = 'a'.repeat(40);
    expect(versionPageHref('http://127.0.0.1:7/', 'deck', rev, 'Слайд 1.dc.html?x=1')).toBe(
      `http://127.0.0.1:7/a/deck/r/${rev}/${encodeURIComponent('Слайд 1.dc.html')}`,
    );
    expect(kitPreviewHref('http://127.0.0.1:7', 'deck', '0.6.0', 'lib/page.html')).toBe(
      'http://127.0.0.1:7/a/deck/k/0.6.0/lib/page.html',
    );
  });

  it('reads the versions from the hub when the page shows the store copy or the hub snapshot', () => {
    expect(versionsVia({ storeRev: null, snapshot: false })).toEqual({});
    expect(versionsVia({ storeRev: 4, snapshot: false }, 'k1')).toEqual({ hub: true, card: 'k1' });
    expect(versionsVia({ storeRev: null, snapshot: true })).toEqual({ hub: true, card: undefined });
  });

  it('addresses a page of a version in the hub store', () => {
    const rev = '0'.repeat(39) + '3';
    expect(storeVersionPageHref('deck', rev, 'Слайд 1.dc.html?x=1', 'c1')).toContain(
      `/design/store/deck/v/${rev}/f/${encodeURIComponent('Слайд 1.dc.html')}?card=c1`,
    );
  });

  describe('which side answers', () => {
    afterEach(() => vi.unstubAllGlobals());
    const asked = () => {
      const fetcher = vi.fn(() => Promise.resolve(new Response('{"points":[],"named":[]}', { status: 200 })));
      vi.stubGlobal('fetch', fetcher);
      return fetcher;
    };
    const urlOf = (f: ReturnType<typeof asked>) => String((f.mock.calls[0] as unknown[])[0]);

    it('asks the hub store, not the machine, for a store copy', async () => {
      const f = asked();
      await fetchProjectHistory('m1' as never, 'deck', undefined, { hub: true });
      expect(urlOf(f)).toContain('/design/store/deck/versions');
      expect(urlOf(f)).not.toContain('/m/m1/');
    });

    it('keeps the machine path when the machine has the project', async () => {
      const f = asked();
      await fetchProjectHistory('m1' as never, 'deck');
      expect(urlOf(f)).toContain('/design/project/versions?artifact=deck');
    });

    it('restores through the hub with the version in the body', async () => {
      const f = asked();
      await restoreProjectVersion('m1' as never, 'deck', 'r1', { hub: true });
      expect(urlOf(f)).toContain('/design/store/deck/restore');
      expect(((f.mock.calls[0] as unknown[])[1] as RequestInit).body).toBe('{"version":"r1"}');
    });
  });
});
