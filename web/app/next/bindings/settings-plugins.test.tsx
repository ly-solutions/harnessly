// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PluginsPage } from '@/next/bindings/settings-plugins';
import { feedKindsSnapshot, setFeedKind } from '@/next/stores/feed-kinds';

// The machine's review rules, as the runtime would keep them.
const kept = vi.hoisted(() => ({
  rules: { instructions: '', repos: {}, hideBots: false, bots: [] as string[] },
}));
vi.mock('@/next/legacy/code-review-api', () => ({
  loadReviewRules: () => Promise.resolve(kept.rules),
  saveReviewRules: (patch: { hideBots?: boolean; bots?: string[] }) => {
    kept.rules = { ...kept.rules, ...patch };
    return Promise.resolve(kept.rules);
  },
}));

beforeEach(() => setFeedKind('mrDone', false));
afterEach(cleanup);

describe('Settings → Plugins', () => {
  it('has the merge request card of the chat off, and turns it on and off in the feed settings', () => {
    render(<PluginsPage />);
    const row = screen.getByRole('switch', {
      name: /Show the merge request card in the chat|Показывать карточку MR в чате/,
    });
    expect(row.getAttribute('aria-checked')).toBe('false');
    fireEvent.click(row);
    expect(feedKindsSnapshot().mrDone).toBe(true);
    fireEvent.click(row);
    expect(feedKindsSnapshot().mrDone).toBe(false);
  });

  it('hides the bots when asked and keeps the authors the person calls bots too', async () => {
    render(<PluginsPage />);
    const hide = screen.getByRole('switch', { name: /Hide merge requests of bots|Скрывать MR ботов/ });
    expect(hide.getAttribute('aria-checked')).toBe('false');
    expect(screen.queryByRole('textbox')).toBeNull();
    fireEvent.click(hide);
    const field = await screen.findByRole('textbox');
    expect(kept.rules.hideBots).toBe(true);
    const save = screen.getByRole('button', { name: /^(Save|Сохранить)$/ });
    expect((save as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(field, { target: { value: ' Release Robot, ci ,' } });
    fireEvent.click(save);
    await waitFor(() => expect(kept.rules.bots).toEqual(['Release Robot', 'ci']));
  });
});
