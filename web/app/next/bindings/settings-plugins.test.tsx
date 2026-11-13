// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { PluginsPage } from '@/next/bindings/settings-plugins';
import { feedKindsSnapshot, setFeedKind } from '@/next/stores/feed-kinds';

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
});
