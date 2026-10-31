// The video's time, pure: where a video time falls in the scenes, the transport clock (play, pause, seek, rate), the
// steps the keys take, and the subtitle line of a scene. Durations are the runtime's; this only lays out what it gets.
// Time comes in as an argument, so nothing here reads a clock.
import type { IVideoRange } from '@/next/features/designly/video/types';

export interface IClockScene {
  /** ms from the video's start. */
  start: number;
  dur: number;
}

export interface IClockAt {
  index: number;
  /** ms from the scene's start. */
  local: number;
}

/** The step of the small and the big jump keys, ms. */
export const STEP_SMALL_MS = 5000;
export const STEP_BIG_MS = 10_000;
/** «Previous scene» restarts the scene when it has played longer than this, ms. */
export const RESTART_MS = 1500;
/** The scene may be this far from the host's clock before it is told the time again, ms. */
export const DRIFT_MS = 300;

export const clampTime = (t: number, total: number): number => Math.max(0, Math.min(total, t));

/** One frame, ms. */
export const frameMs = (fps: number): number => 1000 / fps;

/** The scene a video time falls in, and the time inside it. The end of the video is the end of the last scene. */
export const sceneAt = (scenes: readonly IClockScene[], t: number): IClockAt | null => {
  if (scenes.length === 0) return null;
  const last = scenes.length - 1;
  const time = Math.max(0, t);
  let index = 0;
  for (let i = 0; i <= last; i++) {
    if (scenes[i].start <= time) index = i;
  }
  const scene = scenes[index];
  const local = Math.min(time - scene.start, scene.dur);
  return { index, local };
};

/** Where the previous-scene key goes: the scene's start, or the one before it when this one has barely begun. */
export const prevSceneTarget = (scenes: readonly IClockScene[], t: number): number => {
  const at = sceneAt(scenes, t);
  if (at === null) return 0;
  if (at.local > RESTART_MS || at.index === 0) return scenes[at.index].start;
  return scenes[at.index - 1].start;
};

/** Where the next-scene key goes; null on the last scene. */
export const nextSceneTarget = (scenes: readonly IClockScene[], t: number): number | null => {
  const at = sceneAt(scenes, t);
  if (at === null || at.index + 1 >= scenes.length) return null;
  return scenes[at.index + 1].start;
};

/** The 0–9 keys: a tenth of the video. */
export const tenthOf = (n: number, total: number): number => Math.round((total * n) / 10);

/** Whether a scene's own time is far enough from the expected one to be told it again. */
export const needsResync = (expected: number, reported: number): boolean =>
  Math.abs(expected - reported) > DRIFT_MS;

// ---- the transport clock

export interface IClock {
  /** The time at `since`, ms of the video. */
  t: number;
  playing: boolean;
  rate: number;
  /** The caller's `now` when `t` was set, ms. */
  since: number;
}

export const clockStart = (t = 0): IClock => ({ t, playing: false, rate: 1, since: 0 });

export const clockAt = (c: IClock, now: number, total: number): number =>
  c.playing ? clampTime(c.t + (now - c.since) * c.rate, total) : clampTime(c.t, total);

export const clockSeek = (c: IClock, t: number, now: number, total: number): IClock => ({
  ...c,
  t: clampTime(t, total),
  since: now,
});

export const clockPause = (c: IClock, now: number, total: number): IClock => ({
  ...c,
  t: clockAt(c, now, total),
  playing: false,
  since: now,
});

/** Playing from the end starts again from the beginning of the range, or of the video. */
export const clockPlay = (c: IClock, now: number, total: number, range: IVideoRange | null): IClock => {
  const t = clockAt(c, now, total);
  const from = t >= total ? (range?.t0 ?? 0) : t;
  return { ...c, t: from, playing: true, since: now };
};

/**
 * The clock after a moment of playing: it stops at the end, and with `loop` it goes back to the range's start when
 * it reaches the range's end (a clock that began past the range's end is not looped). The same object when nothing
 * happens.
 */
export const clockTick = (
  c: IClock,
  now: number,
  total: number,
  range: IVideoRange | null,
  loop: boolean,
): IClock => {
  if (!c.playing) return c;
  const t = clockAt(c, now, total);
  if (loop && range !== null && c.t < range.t1 && t >= range.t1) {
    return { ...c, t: range.t0, since: now };
  }
  if (t >= total) return { ...c, t: total, playing: false, since: now };
  return c;
};

// ---- the range keys

/** I: the range starts here; it keeps its end when that is after, and runs to the video's end otherwise. */
export const markIn = (range: IVideoRange | null, t: number, total: number): IVideoRange => ({
  t0: t,
  t1: range !== null && range.t1 > t ? range.t1 : total,
});

/** O: the range ends here; it keeps its start when that is before, and begins at the video's start otherwise. */
export const markOut = (range: IVideoRange | null, t: number): IVideoRange => ({
  t0: range !== null && range.t0 < t ? range.t0 : 0,
  t1: t,
});

// ---- the subtitle line

/** A scene's text as the lines of its subtitles: its sentences (a line break also ends one). */
export const subtitleLines = (notes: string): string[] =>
  notes
    .split(/\n+|(?<=[.!?…])\s+/u)
    .map((line) => line.trim())
    .filter((line) => line !== '');

/** The line on screen at a time of the scene: the lines share the scene's length evenly. '' when there is none. */
export const subtitleAt = (notes: string, local: number, dur: number): string => {
  const lines = subtitleLines(notes);
  if (lines.length === 0 || dur <= 0) return '';
  const index = Math.min(lines.length - 1, Math.max(0, Math.floor((local / dur) * lines.length)));
  return lines[index];
};
