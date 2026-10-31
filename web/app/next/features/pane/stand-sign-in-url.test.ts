import { describe, expect, it, vi } from 'vitest';

vi.mock('@/platform/config', () => ({ appConfig: () => ({ standDomain: '.preview.example.com' }) }));

const load = () => import('@/next/features/pane/stand-sign-in');

describe('standSignInUrl', () => {
  it("starts the stand's own oauth2 sign-in and brings the person back to the page", async () => {
    const { standSignInUrl } = await load();
    const url = new URL(
      standSignInUrl('https://agent-abc-12-787126.preview.example.com', '/adminpanel/x?a=1') ?? '',
    );
    expect(url.origin).toBe('https://agent-abc-12-787126.preview.example.com');
    expect(url.pathname).toBe('/oauth2/start');
    expect(url.searchParams.get('rd')).toBe(
      'https://agent-abc-12-787126.preview.example.com/adminpanel/x?a=1',
    );
  });

  it("returns to the stand's root from the sign-in pages themselves", async () => {
    const { standSignInUrl } = await load();
    const url = new URL(standSignInUrl('https://s1.preview.example.com', '/oauth2/callback?code=1') ?? '');
    expect(url.searchParams.get('rd')).toBe('https://s1.preview.example.com');
  });

  it('is null for pages that are not stands', async () => {
    const { standSignInUrl } = await load();
    expect(standSignInUrl('https://example.org', '/')).toBeNull();
    expect(standSignInUrl('https://ai-ann-7.preview.example.com', '/')).toBeNull();
    expect(standSignInUrl('https://auth.preview.example.com', '/')).toBeNull();
    expect(standSignInUrl('http://s1.preview.example.com', '/')).toBeNull();
  });
});
