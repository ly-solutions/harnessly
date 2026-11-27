// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { pinLocale } from '@/next/i18n';

const state = vi.hoisted(() => ({ phone: false, on: true as boolean | null }));

vi.mock('@/next/ui/use-media', () => ({ usePhone: () => state.phone }));
vi.mock('@/next/features/devmode/api', () => ({
  useDevMode: () => state.on,
  loadTrace: () =>
    Promise.resolve({
      ok: true,
      value: {
        on: true,
        conns: ['cx1'],
        entries: [
          {
            seq: 1,
            at: '2026-11-27T10:00:00Z',
            conn: 'cx1',
            part: 'recipe',
            level: 'warn',
            message: 'guard refused list_jobs',
            data: { reason: 'destructive' },
          },
        ],
      },
    }),
  followTrace: () => () => {},
}));

import DevTracePanel from '@/next/features/devmode/trace-panel';

beforeAll(() => pinLocale('en'));
afterEach(() => {
  cleanup();
  state.phone = false;
  state.on = true;
  window.localStorage.clear();
});

const conns = [{ id: 'cx1', name: 'Gen jobs' }];

describe('DevTracePanel', () => {
  it('is not drawn on a phone even with the mode on', () => {
    state.phone = true;
    const { container } = render(<DevTracePanel machine={'m1' as never} conns={conns} />);
    expect(container.firstChild).toBeNull();
  });

  it('is not drawn while the mode is off or not read yet', () => {
    state.on = false;
    const off = render(<DevTracePanel machine={'m1' as never} conns={conns} />);
    expect(off.container.firstChild).toBeNull();
    off.unmount();
    state.on = null;
    const unknown = render(<DevTracePanel machine={'m1' as never} conns={conns} />);
    expect(unknown.container.firstChild).toBeNull();
  });

  it('on a computer with the mode on unfolds to the lines and a line to its raw JSON', async () => {
    render(<DevTracePanel machine={'m1' as never} conns={conns} />);
    const fold = screen.getByRole('button', { name: /Developer log/ });
    expect(fold.getAttribute('aria-expanded')).toBe('false');
    fireEvent.click(fold);
    const line = await screen.findByText('guard refused list_jobs');
    expect(screen.getAllByText('Gen jobs').length).toBeGreaterThan(0);
    expect(screen.getByRole('combobox', { name: 'Connection' })).toBeTruthy();
    fireEvent.click(line);
    expect(screen.getByText(/"reason": "destructive"/)).toBeTruthy();
  });
});
