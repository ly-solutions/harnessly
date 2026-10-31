// The pure part of a video's guest page: the scenes laid on the video's time from the published chapters, the media's
// name per format, the comment's anchor within the gateway's limits, and the markers of the comments on the strip.
import type { IChapterOfSlide } from '@/next/features/designly/deck/video-chapters';
import type { IGuestBoard } from '@/next/features/designly/guest/guest-config';
import {
  cleanState,
  type IGuestAnchor,
  type IGuestComment,
} from '@/next/features/designly/guest/guest-comments';
import type { TVideoFormat } from '@/next/features/designly/video/types';

/** What the gateway keeps of an anchor: at most 20 settings, a text value of 80 characters, 4 KB in all. */
export const GATEWAY_ANCHOR = { keys: 20, value: 80, bytes: 4096 } as const;

/** Under this a drag on the strip is a click: a moment, not a stretch. */
export const RANGE_MIN_MS = 300;

export const ANCHOR_KIND = 'video';

export const mediaName = (format: TVideoFormat): string =>
  format === '9:16' ? 'video-9x16.mp4' : 'video.mp4';

export interface IGuestScene {
  id: string;
  /** The board's path: the key of its comments and of the views. */
  path: string;
  title: string;
  /** ms from the video's start. */
  start: number;
  /** ms from the video's start; the last scene ends with the video. */
  end: number;
}

/** The scenes the guest can see, from the chapters (one per scene, in time) and the video's length. */
export const scenesOf = (
  chapters: readonly IChapterOfSlide[],
  boards: readonly IGuestBoard[],
  total: number,
): IGuestScene[] =>
  chapters.map((c, i) => ({
    id: boards[c.slide].id,
    path: boards[c.slide].path,
    title: c.title,
    start: c.t,
    end: i + 1 < chapters.length ? chapters[i + 1].t : Math.max(total, c.t),
  }));

/** The scene a time (ms of the video) is in; null before the first scene or without scenes. */
export const sceneAt = (scenes: readonly IGuestScene[], ms: number): IGuestScene | null => {
  let found: IGuestScene | null = null;
  for (const s of scenes) if (s.start <= ms) found = s;
  return found;
};

/** A place on the strip: a moment (t1 null) or a stretch, in ms of the video, inside one scene. */
export interface IGuestPick {
  scene: IGuestScene;
  /** ms from the scene's start. */
  t0: number;
  t1: number | null;
}

/** Two points of a drag (ms of the video) as a pick: ordered, kept inside the scene of the first, a short one is a moment. */
export const pickOf = (scenes: readonly IGuestScene[], from: number, to: number): IGuestPick | null => {
  const scene = sceneAt(scenes, Math.min(from, to));
  if (scene === null) return null;
  const a = Math.max(scene.start, Math.min(from, to));
  const b = Math.min(scene.end, Math.max(from, to));
  const t0 = Math.round(a - scene.start);
  if (b - a < RANGE_MIN_MS) return { scene, t0, t1: null };
  return { scene, t0, t1: Math.round(b - scene.start) };
};

/** The anchor of a comment on a pick: `state` carries {v:'video', scene, t0, t1?, format}. */
export const videoAnchor = (pick: IGuestPick, format: TVideoFormat): IGuestAnchor => {
  const controls: { name: string; value: unknown }[] = [
    { name: 'v', value: ANCHOR_KIND },
    { name: 'scene', value: pick.scene.id },
    { name: 't0', value: pick.t0 },
    { name: 'format', value: format },
  ];
  if (pick.t1 !== null) controls.splice(3, 0, { name: 't1', value: pick.t1 });
  return {
    selector: 'body',
    dataId: null,
    tag: 'div',
    label: pick.scene.title.slice(0, 120),
    rect: { x: 0, y: 0, width: 0, height: 0 },
    state: cleanState(controls),
  };
};

/** A comment on a time, placed on the video's clock. */
export interface IGuestMark {
  id: string;
  /** ms of the video. */
  at: number;
  /** ms of the video; null for a moment. */
  to: number | null;
  scene: string;
  name: string;
  text: string;
}

const whole = (v: unknown): number | null =>
  typeof v === 'number' && Number.isInteger(v) && v >= 0 ? v : null;

/** The comments that point at a time, in time order. Comments on a board, or of a scene the guest cannot see, are left out. */
export const marksOf = (comments: readonly IGuestComment[], scenes: readonly IGuestScene[]): IGuestMark[] =>
  comments
    .flatMap((c) => {
      const state = c.anchor?.state;
      if (!state || state.v !== ANCHOR_KIND) return [];
      const scene = scenes.find((s) => s.id === state.scene);
      const t0 = whole(state.t0);
      if (!scene || t0 === null) return [];
      const t1 = whole(state.t1);
      return [
        {
          id: c.id,
          at: scene.start + t0,
          to: t1 === null ? null : scene.start + t1,
          scene: scene.id,
          name: c.name,
          text: c.text,
        },
      ];
    })
    .sort((a, b) => a.at - b.at);
