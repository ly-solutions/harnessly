// The timeline's arithmetic: pixels ↔ ms, the range a drag makes, where scenes, moments and comment markers stand.
// Times are ms; the starts and durations are the runtime's, this only lays them out. No React here.
import type { IVideoMoment, IVideoRange, IVideoScene } from '@/next/features/designly/video/types';

/** A drag shorter than this is a click, a range shorter than this is a moment. */
export const RANGE_MIN_MS = 200;
/** A pointer that moved less than this many px since it went down is a click. */
export const DRAG_SLOP_PX = 4;
/** One arrow key on the ruler. */
export const SEEK_STEP_MS = 1000;

const clamp = (v: number, min: number, max: number) => Math.max(min, Math.min(max, v));

/** Where `px` (from the track's left edge) is in the video, ms. */
export const pxToMs = (px: number, width: number, total: number): number =>
  width <= 0 ? 0 : Math.round(clamp(px / width, 0, 1) * total);

/** Where `ms` stands on the track, 0..1. */
export const msToFrac = (ms: number, total: number): number => (total <= 0 ? 0 : clamp(ms / total, 0, 1));

/** The range between two points of a drag, in order and inside the video; null when it is too short to be one. */
export const normalizeRange = (a: number, b: number, total: number): IVideoRange | null => {
  const t0 = clamp(Math.min(a, b), 0, total);
  const t1 = clamp(Math.max(a, b), 0, total);
  return t1 - t0 < RANGE_MIN_MS ? null : { t0, t1 };
};

/** The index of the scene `time` is in (a scene owns its start, the last one also the video's end); -1 for none. */
export const sceneIndexAt = (scenes: readonly IVideoScene[], time: number): number => {
  const at = scenes.findIndex((s) => time >= s.start && time < s.start + s.dur);
  if (at >= 0) return at;
  const last = scenes.at(-1);
  return last && time === last.start + last.dur ? scenes.length - 1 : -1;
};

/** A stretch inside one scene, ms from the scene's start; t1 null is a moment. */
export interface ISceneSpan {
  scene: IVideoScene;
  index: number;
  t0: number;
  t1: number | null;
}

/**
 * The range as a comment can hold it: a comment belongs to one scene, so a range that runs on into the next is cut at the
 * scene it starts in. What is left shorter than RANGE_MIN_MS is a moment.
 */
export const spanOfRange = (scenes: readonly IVideoScene[], range: IVideoRange): ISceneSpan | null => {
  const index = sceneIndexAt(scenes, range.t0);
  const scene = scenes[index];
  if (!scene) return null;
  const t0 = range.t0 - scene.start;
  const t1 = Math.min(range.t1, scene.start + scene.dur) - scene.start;
  return { scene, index, t0, t1: t1 - t0 < RANGE_MIN_MS ? null : t1 };
};

/** A moment of the playhead as a span: the scene it is in and the time in it. */
export const spanOfTime = (scenes: readonly IVideoScene[], time: number): ISceneSpan | null => {
  const index = sceneIndexAt(scenes, time);
  const scene = scenes[index];
  return scene ? { scene, index, t0: time - scene.start, t1: null } : null;
};

export interface ITileBox {
  id: string;
  index: number;
  /** Fractions of the track, 0..1. */
  left: number;
  width: number;
}

/** The scene tiles: sized by duration. */
export const tileBoxes = (scenes: readonly IVideoScene[], total: number): ITileBox[] =>
  scenes.map((s, index) => ({
    id: s.id,
    index,
    left: msToFrac(s.start, total),
    width: msToFrac(s.dur, total),
  }));

export interface IDotBox {
  key: string;
  block: string;
  left: number;
  derived: boolean;
}

/** The moments track: a dot where each block comes in. */
export const dotBoxes = (scenes: readonly IVideoScene[], total: number): IDotBox[] =>
  scenes.flatMap((s) =>
    s.moments.map((m: IVideoMoment, i) => ({
      key: `${s.id}:${i}`,
      block: m.block,
      left: msToFrac(s.start + Math.min(m.at, s.dur), total),
      derived: m.derived,
    })),
  );

/** A stretch of a comment as the scene now has it. */
export interface IAnchor {
  t0: number;
  t1: number | null;
  /** The scene got shorter than the comment's time: it was pulled back to the scene's end. */
  changed: boolean;
}

/** A comment's time held inside its scene. A range that collapses to a point becomes a moment. */
export const clampAnchor = (scene: IVideoScene, t0: number, t1: number | null): IAnchor => {
  const a = Math.min(t0, scene.dur);
  const b = t1 === null ? null : Math.min(t1, scene.dur);
  const range = b === null || b - a < RANGE_MIN_MS ? null : b;
  return { t0: a, t1: range, changed: a !== t0 || b !== t1 };
};

export interface IMarkerBox {
  /** Fractions of the track, 0..1; width 0 is a point. */
  left: number;
  width: number;
}

/** A comment's marker on the track, from its scene and its (already held) time. */
export const markerBox = (scene: IVideoScene, anchor: IAnchor, total: number): IMarkerBox => {
  const left = msToFrac(scene.start + anchor.t0, total);
  const end = anchor.t1 === null ? left : msToFrac(scene.start + anchor.t1, total);
  return { left, width: end - left };
};
