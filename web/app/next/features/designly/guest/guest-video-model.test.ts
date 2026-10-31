import { describe, expect, it } from 'vitest';
import type { IGuestComment } from '@/next/features/designly/guest/guest-comments';
import {
  GATEWAY_ANCHOR,
  marksOf,
  mediaName,
  pickOf,
  sceneAt,
  scenesOf,
  videoAnchor,
} from '@/next/features/designly/guest/guest-video-model';

const boards = [
  { id: 'a', title: 'One', path: 'One.dc.html', w: null, h: null },
  { id: 'b', title: 'Two', path: 'Two.dc.html', w: null, h: null },
];
const scenes = scenesOf(
  [
    { slide: 0, title: 'One', t: 0 },
    { slide: 1, title: 'Two', t: 4000 },
  ],
  boards,
  9000,
);

describe('the scenes of a guest video', () => {
  it('end where the next starts, the last with the video', () => {
    expect(scenes.map((s) => [s.id, s.start, s.end])).toEqual([
      ['a', 0, 4000],
      ['b', 4000, 9000],
    ]);
    expect(sceneAt(scenes, 4000)?.id).toBe('b');
    expect(sceneAt(scenes, 3999)?.id).toBe('a');
    expect(sceneAt([], 10)).toBeNull();
  });

  it('name the MP4 of a format', () => {
    expect(mediaName('16:9')).toBe('video.mp4');
    expect(mediaName('9:16')).toBe('video-9x16.mp4');
  });
});

describe('a pick on the strip', () => {
  it('is a moment when short, a stretch otherwise, in the time of the scene', () => {
    expect(pickOf(scenes, 4100, 4200)).toMatchObject({ t0: 100, t1: null });
    expect(pickOf(scenes, 6500, 5000)).toMatchObject({ t0: 1000, t1: 2500 });
  });

  it('stays inside the scene where the drag began', () => {
    expect(pickOf(scenes, 3000, 8000)).toMatchObject({ t0: 3000, t1: 4000 });
  });
});

describe('the anchor of a video comment', () => {
  it('carries v, scene, t0, t1 and format as the state', () => {
    const pick = pickOf(scenes, 5000, 7000);
    if (pick === null) throw new Error('no pick');
    expect(videoAnchor(pick, '9:16').state).toEqual({
      v: 'video',
      scene: 'b',
      t0: 1000,
      t1: 3000,
      format: '9:16',
    });
  });

  it('leaves t1 out for a moment', () => {
    const pick = pickOf(scenes, 100, 100);
    if (pick === null) throw new Error('no pick');
    expect(videoAnchor(pick, '16:9').state).not.toHaveProperty('t1');
  });

  it('stays within the gateway limits even for a long title and scene id', () => {
    const long = 'x'.repeat(500);
    const wide = scenesOf([{ slide: 0, title: long, t: 0 }], [{ ...boards[0], id: long }], 60000);
    const pick = pickOf(wide, 0, 60000);
    if (pick === null) throw new Error('no pick');
    const anchor = videoAnchor(pick, '16:9');
    expect(Object.keys(anchor.state).length).toBeLessThanOrEqual(GATEWAY_ANCHOR.keys);
    for (const value of Object.values(anchor.state))
      if (typeof value === 'string') expect(value.length).toBeLessThanOrEqual(GATEWAY_ANCHOR.value);
    expect(new TextEncoder().encode(JSON.stringify(anchor)).length).toBeLessThanOrEqual(GATEWAY_ANCHOR.bytes);
  });
});

const comment = (id: string, state: Record<string, string | number | boolean> | null): IGuestComment => ({
  id,
  board: 'Two.dc.html',
  text: `text ${id}`,
  name: 'Ann',
  at: '',
  anchor: state && {
    selector: 'body',
    dataId: null,
    tag: 'div',
    label: '',
    rect: { x: 0, y: 0, width: 0, height: 0 },
    state,
  },
  point: null,
});

describe('the markers of the comments', () => {
  it('are placed on the clock of the video, in time order; other comments are left out', () => {
    const marks = marksOf(
      [
        comment('late', { v: 'video', scene: 'b', t0: 2000, t1: 3000, format: '16:9' }),
        comment('early', { v: 'video', scene: 'a', t0: 500, format: '9:16' }),
        comment('board', null),
        comment('unseen', { v: 'video', scene: 'zzz', t0: 1, format: '16:9' }),
        comment('bad', { v: 'video', scene: 'a', t0: -5, format: '16:9' }),
      ],
      scenes,
    );
    expect(marks.map((m) => [m.id, m.at, m.to])).toEqual([
      ['early', 500, null],
      ['late', 6000, 7000],
    ]);
  });
});
