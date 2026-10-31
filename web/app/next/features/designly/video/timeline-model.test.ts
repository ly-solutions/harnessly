import { describe, expect, it } from 'vitest';
import {
  clampAnchor,
  dotBoxes,
  markerBox,
  msToFrac,
  normalizeRange,
  pxToMs,
  sceneIndexAt,
  spanOfRange,
  tileBoxes,
} from '@/next/features/designly/video/timeline-model';
import { scene } from '@/next/features/designly/video/test-scene';

const scenes = [scene('a', 0, 4000), scene('b', 4000, 6000), scene('c', 10000, 5000)];

describe('px and ms', () => {
  it('converts both ways and clamps to the track', () => {
    expect(pxToMs(100, 400, 20000)).toBe(5000);
    expect(pxToMs(-10, 400, 20000)).toBe(0);
    expect(pxToMs(900, 400, 20000)).toBe(20000);
    expect(pxToMs(10, 0, 20000)).toBe(0);
    expect(msToFrac(5000, 20000)).toBe(0.25);
    expect(msToFrac(5000, 0)).toBe(0);
  });
});

describe('normalizeRange', () => {
  it('orders the points and keeps them inside', () => {
    expect(normalizeRange(5000, 2000, 15000)).toEqual({ t0: 2000, t1: 5000 });
    expect(normalizeRange(-100, 99999, 15000)).toEqual({ t0: 0, t1: 15000 });
  });
  it('is null when too short to be a range', () => {
    expect(normalizeRange(1000, 1100, 15000)).toBeNull();
  });
});

describe('scenes', () => {
  it('finds the scene a time is in; the end belongs to the last', () => {
    expect(sceneIndexAt(scenes, 0)).toBe(0);
    expect(sceneIndexAt(scenes, 4000)).toBe(1);
    expect(sceneIndexAt(scenes, 15000)).toBe(2);
    expect(sceneIndexAt(scenes, 15001)).toBe(-1);
  });
  it('lays tiles out by duration', () => {
    const t = tileBoxes(scenes, 15000);
    expect(t[1].left).toBeCloseTo(4 / 15);
    expect(t[1].width).toBeCloseTo(6 / 15);
  });
  it('cuts a range at the end of the scene it starts in', () => {
    const span = spanOfRange(scenes, { t0: 3000, t1: 7000 });
    expect(span).toMatchObject({ index: 0, t0: 3000, t1: 4000 });
    expect(spanOfRange(scenes, { t0: 5000, t1: 6000 })).toMatchObject({ index: 1, t0: 1000, t1: 2000 });
  });
  it('turns what is left under the minimum into a moment', () => {
    expect(spanOfRange(scenes, { t0: 3900, t1: 7000 })).toMatchObject({ index: 0, t0: 3900, t1: null });
  });
});

describe('markers and moments across scenes', () => {
  it('places a point and a range on the whole video', () => {
    const point = markerBox(scenes[1], clampAnchor(scenes[1], 1500, null), 15000);
    expect(point.left).toBeCloseTo(5500 / 15000);
    expect(point.width).toBe(0);
    const stretch = markerBox(scenes[2], clampAnchor(scenes[2], 1000, 3000), 15000);
    expect(stretch.left).toBeCloseTo(11000 / 15000);
    expect(stretch.width).toBeCloseTo(2000 / 15000);
  });
  it('marks derived moments', () => {
    const withMoments = [
      scene('a', 0, 4000, [
        { block: 'T', tpl: null, at: 1000, out: null, buildIn: '', derived: false },
        { block: 'U', tpl: null, at: 2000, out: null, buildIn: '', derived: true },
      ]),
      scene('b', 4000, 4000, [
        { block: 'V', tpl: null, at: 500, out: null, buildIn: 'fade', derived: false },
      ]),
    ];
    const dots = dotBoxes(withMoments, 8000);
    expect(dots.map((d) => d.derived)).toEqual([false, true, false]);
    expect(dots[2].left).toBeCloseTo(4500 / 8000);
  });
});

describe('clampAnchor', () => {
  it('leaves a time that still fits', () => {
    expect(clampAnchor(scenes[0], 1000, 2000)).toEqual({ t0: 1000, t1: 2000, changed: false });
    expect(clampAnchor(scenes[0], 1000, null)).toEqual({ t0: 1000, t1: null, changed: false });
  });
  it('pulls a range back when the scene got shorter and flags it', () => {
    expect(clampAnchor(scenes[0], 3900, 6000)).toEqual({ t0: 3900, t1: null, changed: true });
    expect(clampAnchor(scenes[0], 1000, 6000)).toEqual({ t0: 1000, t1: 4000, changed: true });
    expect(clampAnchor(scenes[0], 9000, null)).toEqual({ t0: 4000, t1: null, changed: true });
  });
});
