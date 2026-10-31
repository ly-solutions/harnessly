// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import UserMessage from '@/next/features/chat/user-message';
import CopyButton, { COPIED_MS } from '@/next/features/chat/copy-button';
import { pinLocale } from '@/next/i18n';

beforeAll(() => pinLocale('en'));
const writeText = vi.fn(() => Promise.resolve());
beforeEach(() => {
  vi.useFakeTimers();
  writeText.mockClear();
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const click = async () => {
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Copy' }));
  });
};

describe('copy button', () => {
  it('copies the agent answer source and shows the check for a moment', async () => {
    const md = '# Title\n\n**bold** and `code`';
    render(<CopyButton text={md} />);
    await click();
    expect(writeText).toHaveBeenCalledWith(md);
    const done = screen.getByRole('button', { name: 'Copied' });
    expect(done.getAttribute('data-copied')).toBe('true');
    act(() => {
      vi.advanceTimersByTime(COPIED_MS);
    });
    expect(screen.getByRole('button', { name: 'Copy' }).hasAttribute('data-copied')).toBe(false);
  });

  it("copies the person's own message source", async () => {
    render(
      <UserMessage
        text={'make **this** a list:\n- a\n- b'}
        attachments={[]}
        outgoing={null}
        own
        offline={false}
        onRetry={vi.fn()}
        onDelete={vi.fn()}
      />,
    );
    await click();
    expect(writeText).toHaveBeenCalledWith('make **this** a list:\n- a\n- b');
    expect(screen.getByRole('button', { name: 'Copied' })).toBeTruthy();
  });
});
