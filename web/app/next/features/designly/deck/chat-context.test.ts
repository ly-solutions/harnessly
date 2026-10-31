import { describe, expect, it } from 'vitest';

import { formatChatContext } from '@/next/features/designly/deck/chat-context';

describe('formatChatContext', () => {
  it('wraps a deck context in a deck block', () => {
    expect(formatChatContext(2, { label: 'Slide 1', text: 'Title <b>', selected: false })).toBe(
      '<annotation n="2" kind="deck">\nelement: Title <b>\n</annotation>',
    );
  });

  it('passes a ready block as it is, with the message number', () => {
    const block = '<annotation n="1" kind="video">\nscene: 1 «Intro»\ntime: 0:01–0:02\n</annotation>';
    expect(formatChatContext(3, { label: 'Stretch', text: block, selected: true, block: true })).toBe(
      block.replace('n="1"', 'n="3"'),
    );
  });

  it('does not trust text that only claims to be a block', () => {
    const out = formatChatContext(1, {
      label: 'x',
      text: '<annotation n="1" kind="video">',
      selected: false,
    });
    expect(out.startsWith('<annotation n="1" kind="deck">')).toBe(true);
    expect(out).toContain('<\\annotation');
  });
});
