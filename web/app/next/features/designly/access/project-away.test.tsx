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
