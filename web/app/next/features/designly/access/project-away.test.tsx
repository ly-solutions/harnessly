// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import ProjectAway from './project-away';
import { AWAY } from '@/next/features/designly/project-read';
import { t } from '@/next/i18n';
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it('opens the exact requested board and follows a later address change', async () => {
  const second = 'Étude (2).dc.html';
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            boards: [
              { title: 'First', path: 'a.dc.html' },
              { title: 'Second', path: second },
            ],
          }),
        ),
    ),
  );
  const onPage = vi.fn();
  const props = { project: 'prototype', why: AWAY.absentStoreOff, ownerName: '', onBack: vi.fn(), onPage };
  const v = render(<ProjectAway {...props} page={second} />);
  const frame = await screen.findByTitle('Second');
  expect(frame.getAttribute('src')).toContain(encodeURIComponent(second));
  fireEvent.click(screen.getByRole('button', { name: 'First' }));
  expect(onPage).toHaveBeenCalledWith('a.dc.html');
  v.rerender(<ProjectAway {...props} page="gone.dc.html" />);
  expect(screen.getByRole('status').textContent).toBe(t('next.access.pageNotFound'));
  expect(v.container.querySelector('iframe')).toBeNull();
});

it('shows the owner snapshot read-only, with its capture time, when the machine did not give the project (GitHub #180)', async () => {
  const urls: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      urls.push(url);
      if (url.includes('/snapshot'))
        return new Response(
          JSON.stringify({
            capturedAt: '2026-11-09T09:00:00.000Z',
            expiresAt: '2026-11-16T09:00:00.000Z',
            boards: [
              { n: 0, title: 'First' },
              { n: 1, title: 'Second' },
            ],
          }),
        );
      return new Response('{"error":"down"}', { status: 503 });
    }),
  );
  const v = render(
    <ProjectAway
      project="prototype"
      card="abc"
      why={AWAY.silentStoreOff}
      ownerName=""
      onBack={vi.fn()}
      page={null}
    />,
  );
  const first = await screen.findByAltText(t('next.access.snapshot.view.alt', { title: 'First' }));
  expect(first.getAttribute('src')).toContain('/snapshot/img/0');
  expect(first.getAttribute('src')).toContain('card=abc');
  expect(v.container.querySelector('iframe')).toBeNull();
  expect(screen.getByRole('status').textContent).toContain(
    new Date('2026-11-09T09:00:00.000Z').toLocaleString(),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Second' }));
  expect(
    (await screen.findByAltText(t('next.access.snapshot.view.alt', { title: 'Second' }))).getAttribute('src'),
  ).toContain('/snapshot/img/1');
});

it('says the live project needs the owner machine when no snapshot was left', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response('{"code":"no_snapshot"}', { status: 404 })),
  );
  render(
    <ProjectAway project="prototype" why={AWAY.silentStoreOff} ownerName="" onBack={vi.fn()} page={null} />,
  );
  await Promise.resolve();
  expect(screen.getByRole('status').textContent).toContain(t('next.access.away.silentStoreOff.title'));
  expect(screen.queryByRole('img')).toBeNull();
});
