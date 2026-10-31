// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import GuestVideo from '@/next/features/designly/guest/guest-video';
import type { IGuestConfig } from '@/next/features/designly/guest/guest-config';

const config = (over: Partial<IGuestConfig> = {}): IGuestConfig => ({
  v: 1,
  token: 't',
  title: 'Launch',
  rights: 'view',
  expiresAt: null,
  view: { v: 1, deck: 'video', video: true, stats: false, format: '9:16' },
  boards: [
    { id: 'a', title: 'One', path: 'One.dc.html', w: null, h: null },
    { id: 'b', title: 'Two', path: 'Two.dc.html', w: null, h: null },
  ],
  kind: 'slides',
  deck: {
    type: 'video',
    video: {
      chapters: [
        { page: 'a', title: 'One', t: 0 },
        { page: 'b', title: 'Two', t: 4000 },
      ],
      formats: ['16:9', '9:16'],
      durMs: 9000,
    },
    pdf: false,
  },
  filesBase: '/s/t/f/',
  commentsUrl: '',
  mediaBase: '/s/t/m/',
  seenUrl: '/s/t/seen',
  ...over,
});

beforeEach(() => {
  vi.stubGlobal('matchMedia', () => ({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }));
  HTMLMediaElement.prototype.pause = vi.fn();
  HTMLMediaElement.prototype.play = vi.fn().mockResolvedValue(undefined);
});
afterEach(cleanup);

describe('the video guest page', () => {
  it('plays the format of the link and switches to the other one', () => {
    const { container } = render(<GuestVideo config={config()} />);
    expect(container.querySelector('video')?.getAttribute('src')).toBe('/s/t/m/video-9x16.mp4');
    fireEvent.click(screen.getByRole('button', { name: '16:9' }));
    expect(container.querySelector('video')?.getAttribute('src')).toBe('/s/t/m/video.mp4');
  });

  it('has no switch for a single published format', () => {
    const one = config();
    one.deck!.video!.formats = ['16:9'];
    render(<GuestVideo config={one} />);
    expect(screen.queryByRole('button', { name: '9:16' })).toBeNull();
  });

  it('lists the chapters', () => {
    render(<GuestVideo config={config()} />);
    expect(screen.getByText('Two')).toBeTruthy();
  });

  it('says so when the owner did not hand the video out', () => {
    const { container } = render(
      <GuestVideo config={config({ view: { v: 1, deck: 'video', video: false } })} />,
    );
    expect(container.querySelector('video')).toBeNull();
  });

  it('offers no comment button on a view-only link', () => {
    render(<GuestVideo config={config()} />);
    expect(screen.queryByText('Комментарий')).toBeNull();
    expect(screen.queryByText('Comment')).toBeNull();
  });
});
