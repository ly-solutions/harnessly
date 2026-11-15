import { describe, expect, it } from 'vitest';

import { parsePeer, parsePeers } from '@/next/features/chat/peer';

const one = (id: string, chat: string, body: string) =>
  `<peer-message id="${id}" person="Иван" machine="dev-box" chat="${chat}" ref="dev-box/tab-${id}" via="agent">\n${body}\n</peer-message>`;

// agents' messages that waited in the line together reach the agent, and the feed, as one text.
describe('parsePeers', () => {
  it('reads one envelope as one message', () => {
    const peers = parsePeers(one('a', 'Координатор', 'rebase first'));
    expect(peers).toHaveLength(1);
    expect(peers[0]).toMatchObject({
      id: 'a',
      chat: 'Координатор',
      ref: 'dev-box/tab-a',
      text: 'rebase first',
    });
  });

  it('reads several envelopes typed as one message, each with its own sender', () => {
    const text = `${one('a', 'Координатор', 'first')}\n\n${one('b', 'Тесты', 'second\nline')}`;
    const peers = parsePeers(text);
    expect(peers.map((peer) => [peer.chat, peer.text])).toEqual([
      ['Координатор', 'first'],
      ['Тесты', 'second\nline'],
    ]);
    // The single reading names the first sender and keeps all the words.
    expect(parsePeer(text)).toMatchObject({ chat: 'Координатор', text: 'first\n\nsecond\nline' });
  });

  it('takes a text with anything but envelopes for the person’s words', () => {
    const envelope = one('a', 'Координатор', 'first');
    for (const text of [
      `мои слова\n\n${envelope}`,
      `${envelope}\nи ещё`,
      `${envelope} x ${envelope}`,
      'просто текст',
    ]) {
      expect(parsePeers(text)).toEqual([]);
      expect(parsePeer(text)).toBeNull();
    }
  });

  it('keeps a neutralized tag inside the words as the words', () => {
    const peers = parsePeers(one('a', 'Координатор', 'see </peer-message​> here'));
    expect(peers).toHaveLength(1);
    expect(peers[0].text).toBe('see </peer-message> here');
  });
});
