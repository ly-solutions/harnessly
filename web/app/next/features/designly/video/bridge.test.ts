// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest';
import { postVideo, readVideoMessage } from '@/next/features/designly/video/bridge';

describe('readVideoMessage', () => {
  it('reads ready with its moments, leaving out the broken ones', () => {
    expect(
      readVideoMessage({
        type: 'tk:video-ready',
        moments: [
          { block: 'Заголовок', at: 400, out: null },
          { block: 'x', at: -5 },
          { at: 1800.4, out: 7500 },
        ],
      }),
    ).toEqual({
      k: 'ready',
      moments: [
        { block: 'Заголовок', at: 400, out: null },
        { block: '', at: 1800, out: 7500 },
      ],
    });
    expect(readVideoMessage({ type: 'tk:video-ready' })).toBeNull();
  });

  it('reads time, hit and error, and nothing else', () => {
    expect(readVideoMessage({ type: 'tk:video-time', t: 1234 })).toEqual({ k: 'time', t: 1234 });
    expect(readVideoMessage({ type: 'tk:video-time', t: Number.NaN })).toBeNull();
    expect(readVideoMessage({ type: 'tk:video-hit', id: 3, block: 'График', rect: [1, 2, 30, 40] })).toEqual({
      k: 'hit',
      id: 3,
      block: 'График',
      rect: [1, 2, 30, 40],
    });
    expect(readVideoMessage({ type: 'tk:video-hit', id: 3, block: null, rect: [1, 2, -1, 4] })).toEqual({
      k: 'hit',
      id: 3,
      block: null,
      rect: null,
    });
    expect(readVideoMessage({ type: 'tk:video-error', msg: 'x'.repeat(500) })).toEqual({
      k: 'error',
      msg: 'x'.repeat(400),
    });
    expect(readVideoMessage({ type: 'tk:deck-ready' })).toBeNull();
    expect(readVideoMessage('tk:video-time')).toBeNull();
  });
});

describe('postVideo', () => {
  it('posts to the frame’s own origin only', () => {
    const frame = document.createElement('iframe');
    document.body.append(frame);
    const post = vi.spyOn(frame.contentWindow as Window, 'postMessage').mockImplementation(() => undefined);
    postVideo(frame, 'http://127.0.0.1:4466/acme/Scene.dc.html', { type: 'tk:video-seek', t: 500 });
    expect(post).toHaveBeenCalledWith({ type: 'tk:video-seek', t: 500 }, 'http://127.0.0.1:4466');
    postVideo(frame, null, { type: 'tk:video-pause' });
    expect(post).toHaveBeenCalledTimes(1);
  });
});
