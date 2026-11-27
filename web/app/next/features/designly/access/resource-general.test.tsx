// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { createRef } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PortalContext } from '@/next/ui/portal';
import { t } from '@/next/i18n';
import ResourceShare from '@/next/features/designly/access/resource-share';
import type { IResourceView } from '@/next/features/designly/access/resource-api';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  })) as unknown as typeof window.matchMedia;
});

const host = createRef<HTMLDivElement>();
const inRoot = (ui: React.ReactNode) =>
  render(
    <PortalContext.Provider value={host}>
      <div className="hx-root" ref={host} />
      {ui}
    </PortalContext.Provider>,
  );

const json = (body: unknown) =>
  Promise.resolve(
    new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } }),
  );

const URL_ANYONE = 'https://share.x.ru/s/abc';

const owner: IResourceView = {
  resource: { kind: 'doc', id: 'plan', subkind: '', title: 'Plan' },
  stored: true,
  role: 'owner',
  until: null,
  canManage: true,
  manager: true,
  steward: false,
  owner: { email: 'boss@x.ru', name: 'Boss' },
  homeTeam: null,
  editorsCanShare: true,
  grants: [],
  requests: [],
  links: [],
  general: { level: 'none', url: null },
  myRequest: null,
};

const props = (view: IResourceView, onNext = vi.fn(), onCopyLink = vi.fn()) => ({
  open: true,
  onClose: vi.fn(),
  target: { kind: 'doc' as const, id: 'plan' },
  title: 'Plan',
  view,
  onNext,
  address: 'https://ai.x.ru/docs/plan',
  onCopyLink,
});

// «Общий доступ» is the client link of every kind of resource, the Google Docs way.
describe('«Общий доступ»', () => {
  it('opens a restricted resource to anyone with the link and copies that address', async () => {
    const on: IResourceView = { ...owner, general: { level: 'view', url: URL_ANYONE } };
    const fetchMock = vi.fn(() => json(on));
    vi.stubGlobal('fetch', fetchMock);
    const onNext = vi.fn();
    inRoot(<ResourceShare {...props(owner, onNext)} />);

    const section = (await screen.findByText(t('next.access.general.title'))).closest('section')!;
    expect(within(section).getByText(t('next.access.general.restricted'))).toBeTruthy();
    // No separate «Ссылки без входа» to make: «Общий доступ» is the link.
    expect(screen.queryByText(t('next.access.guestLinks'))).toBeNull();

    fireEvent.click(within(section).getByRole('button', { name: t('next.access.general.scope') }));
    fireEvent.click(await screen.findByRole('menuitemradio', { name: t('next.access.general.anyone') }));
    await waitFor(() => expect(onNext).toHaveBeenCalledWith(on));
    const [url, init] = (fetchMock.mock.calls as unknown as [string, RequestInit][])[0]!;
    expect(url).toContain('/access/doc/plan/general');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body as string)).toEqual({ level: 'view' });
  });

  it('copies the address that opens without an account while it is on', async () => {
    const writeText = vi.fn(() => Promise.resolve());
    Object.assign(navigator, { clipboard: { writeText } });
    const onCopyLink = vi.fn();
    inRoot(
      <ResourceShare
        {...props({ ...owner, general: { level: 'comment', url: URL_ANYONE } }, vi.fn(), onCopyLink)}
      />,
    );
    const section = (await screen.findByText(t('next.access.general.title'))).closest('section')!;
    expect(within(section).getByText(t('next.access.general.anyone'))).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: t('next.access.copyLink') }));
    expect(writeText).toHaveBeenCalledWith(URL_ANYONE);
    expect(onCopyLink).not.toHaveBeenCalled();
  });
});
