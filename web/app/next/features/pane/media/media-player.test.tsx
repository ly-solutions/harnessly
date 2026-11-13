// @vitest-environment jsdom
// a file that did not load is not called a file the browser cannot play.
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { pinLocale } from '@/next/i18n';
import MediaPlayer from '@/next/features/pane/media/media-player';
import { MEDIA_FAILURE, MEDIA_KIND, mediaFailureOf } from '@/next/features/pane/media/media-model';

const CANT_LOAD = 'The file did not load.';
const CANT_PLAY = 'The browser cannot play this file. Save it and open it in its own app.';
const SRC_NOT_SUPPORTED = 4;

beforeAll(() => {
  pinLocale('en');
  HTMLMediaElement.prototype.pause = () => undefined;
  HTMLMediaElement.prototype.load = () => undefined;
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const answer = (ok: boolean) =>
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({ ok, body: null })),
  );

const open = () => {
  const { container } = render(
    <MediaPlayer
      src="/file?path=a.mp4"
      kind={MEDIA_KIND.video}
      name="a.mp4"
      path="/a.mp4"
      saveHref="/save"
    />,
  );
  return container;
};

const failWith = (container: HTMLElement, code: number) => {
  const element = container.querySelector('video.hx-media__video');
  if (!element) throw new Error('no video');
  Object.defineProperty(element, 'error', { value: { code } });
  fireEvent.error(element);
};

describe('a file the player did not play', () => {
  it('is one that did not load when its address does not answer, whatever the browser calls it', () => {
    expect(mediaFailureOf(SRC_NOT_SUPPORTED, false)).toBe(MEDIA_FAILURE.load);
    expect(mediaFailureOf(2, true)).toBe(MEDIA_FAILURE.load);
    expect(mediaFailureOf(null, true)).toBe(MEDIA_FAILURE.load);
    expect(mediaFailureOf(3, true)).toBe(MEDIA_FAILURE.format);
    expect(mediaFailureOf(SRC_NOT_SUPPORTED, true)).toBe(MEDIA_FAILURE.format);
  });

  it('says the file did not load and loads it again on «Try again»', async () => {
    answer(false);
    const container = open();
    failWith(container, SRC_NOT_SUPPORTED);
    expect(await screen.findByText(CANT_LOAD)).toBeTruthy();
    expect(screen.queryByText(CANT_PLAY)).toBeNull();
    expect(fetch).toHaveBeenCalledWith(
      '/file?path=a.mp4',
      expect.objectContaining({ headers: { Range: 'bytes=0-0' } }),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(screen.queryByText(CANT_LOAD)).toBeNull();
    expect(container.querySelector('video.hx-media__video')).not.toBeNull();
  });

  it('says the browser cannot play it only when the file does load', async () => {
    answer(true);
    const container = open();
    failWith(container, SRC_NOT_SUPPORTED);
    expect(await screen.findByText(CANT_PLAY)).toBeTruthy();
    expect(screen.queryByText(CANT_LOAD)).toBeNull();
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull();
    expect(screen.getByRole('link', { name: 'Download' }).getAttribute('href')).toBe('/save');
  });
});
