import { describe, expect, it } from 'vitest';
import {
  clockAt,
  clockPause,
  clockPlay,
  clockSeek,
  clockStart,
  clockTick,
  frameMs,
  markIn,
  markOut,
  needsResync,
  nextSceneTarget,
  prevSceneTarget,
  sceneAt,
  subtitleAt,
  subtitleLines,
  tenthOf,
} from '@/next/features/designly/video/video-clock';

const scenes = [
  { start: 0, dur: 4000 },
  { start: 4000, dur: 6000 },
  { start: 10_000, dur: 2000 },
];
const TOTAL = 12_000;

describe('sceneAt', () => {
  it('finds the scene and the time inside it', () => {
    expect(sceneAt(scenes, 0)).toEqual({ index: 0, local: 0 });
    expect(sceneAt(scenes, 3999)).toEqual({ index: 0, local: 3999 });
    expect(sceneAt(scenes, 7000)).toEqual({ index: 1, local: 3000 });
  });

  it('puts a boundary in the scene that begins there', () => {
    expect(sceneAt(scenes, 4000)).toEqual({ index: 1, local: 0 });
    expect(sceneAt(scenes, 10_000)).toEqual({ index: 2, local: 0 });
  });

  it('clamps before the start and past the end', () => {
    expect(sceneAt(scenes, -50)).toEqual({ index: 0, local: 0 });
    expect(sceneAt(scenes, 99_000)).toEqual({ index: 2, local: 2000 });
    expect(sceneAt([], 10)).toBeNull();
  });
});

describe('scene targets', () => {
  it('goes to the start of the scene, or to the one before when it has barely begun', () => {
    expect(prevSceneTarget(scenes, 7000)).toBe(4000);
    expect(prevSceneTarget(scenes, 4500)).toBe(0);
    expect(prevSceneTarget(scenes, 500)).toBe(0);
  });

  it('goes to the next scene, and nowhere from the last', () => {
    expect(nextSceneTarget(scenes, 100)).toBe(4000);
    expect(nextSceneTarget(scenes, 10_500)).toBeNull();
  });

  it('takes a tenth of the video and a frame of the fps', () => {
    expect(tenthOf(0, TOTAL)).toBe(0);
    expect(tenthOf(5, TOTAL)).toBe(6000);
    expect(tenthOf(9, 15_000)).toBe(13_500);
    expect(frameMs(30)).toBeCloseTo(33.33, 1);
  });

  it('resyncs a scene only past the drift', () => {
    expect(needsResync(1000, 1200)).toBe(false);
    expect(needsResync(1000, 1400)).toBe(true);
  });
});

describe('the transport clock', () => {
  it('plays at its rate from where it was seeked', () => {
    let c = clockSeek(clockStart(), 2000, 100, TOTAL);
    expect(clockAt(c, 5000, TOTAL)).toBe(2000);
    c = clockPlay(c, 1000, TOTAL, null);
    expect(clockAt(c, 1500, TOTAL)).toBe(2500);
    c = { ...c, rate: 2 };
    expect(clockAt(c, 2000, TOTAL)).toBe(4000);
  });

  it('keeps the time it paused at', () => {
    let c = clockPlay(clockStart(1000), 0, TOTAL, null);
    c = clockPause(c, 700, TOTAL);
    expect(c.playing).toBe(false);
    expect(clockAt(c, 9999, TOTAL)).toBe(1700);
  });

  it('plays from the start of the range, or of the video, when it was at the end', () => {
    const ended = clockSeek(clockStart(), TOTAL, 0, TOTAL);
    expect(clockPlay(ended, 5, TOTAL, null).t).toBe(0);
    expect(clockPlay(ended, 5, TOTAL, { t0: 3000, t1: 5000 }).t).toBe(3000);
  });

  it('stops at the end and never runs past it', () => {
    const c = clockPlay(clockStart(11_000), 0, TOTAL, null);
    expect(clockAt(c, 5000, TOTAL)).toBe(TOTAL);
    expect(clockTick(c, 500, TOTAL, null, false)).toBe(c);
    const stopped = clockTick(c, 5000, TOTAL, null, false);
    expect(stopped.playing).toBe(false);
    expect(stopped.t).toBe(TOTAL);
  });

  it('loops the range, and leaves a clock that began past it alone', () => {
    const range = { t0: 2000, t1: 3000 };
    const c = clockPlay(clockStart(2500), 0, TOTAL, range);
    expect(clockTick(c, 400, TOTAL, range, true)).toBe(c);
    const looped = clockTick(c, 600, TOTAL, range, true);
    expect(looped.playing).toBe(true);
    expect(clockAt(looped, 600, TOTAL)).toBe(2000);
    expect(clockTick(c, 600, TOTAL, range, false)).toBe(c);
    const past = clockPlay(clockStart(5000), 0, TOTAL, range);
    expect(clockTick(past, 100, TOTAL, range, true)).toBe(past);
  });
});

describe('the range keys', () => {
  it('marks the start and keeps a later end, or runs to the end', () => {
    expect(markIn(null, 1000, TOTAL)).toEqual({ t0: 1000, t1: TOTAL });
    expect(markIn({ t0: 0, t1: 5000 }, 1000, TOTAL)).toEqual({ t0: 1000, t1: 5000 });
    expect(markIn({ t0: 0, t1: 500 }, 1000, TOTAL)).toEqual({ t0: 1000, t1: TOTAL });
  });

  it('marks the end and keeps an earlier start, or begins at the start', () => {
    expect(markOut(null, 4000)).toEqual({ t0: 0, t1: 4000 });
    expect(markOut({ t0: 1000, t1: 9000 }, 4000)).toEqual({ t0: 1000, t1: 4000 });
    expect(markOut({ t0: 6000, t1: 9000 }, 4000)).toEqual({ t0: 0, t1: 4000 });
  });
});

describe('the subtitle line', () => {
  const notes = 'Первое. Второе! Третье?\nЧетвёртое';

  it('splits the text into sentences', () => {
    expect(subtitleLines(notes)).toEqual(['Первое.', 'Второе!', 'Третье?', 'Четвёртое']);
    expect(subtitleLines('  \n ')).toEqual([]);
  });

  it('spreads the lines evenly over the scene', () => {
    expect(subtitleAt(notes, 0, 4000)).toBe('Первое.');
    expect(subtitleAt(notes, 1999, 4000)).toBe('Второе!');
    expect(subtitleAt(notes, 3999, 4000)).toBe('Четвёртое');
    expect(subtitleAt(notes, 4000, 4000)).toBe('Четвёртое');
  });

  it('shows nothing for a scene without text', () => {
    expect(subtitleAt('', 100, 4000)).toBe('');
  });
});
