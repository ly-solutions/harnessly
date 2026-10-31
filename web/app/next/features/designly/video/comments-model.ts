// A video's comment list: the team's (the runtime's meta/comments) and the guests' (the gateway's, whose anchor state is
// {scene, t0, t1, format}) merged into one, in the order they were left and numbered by it. A comment on a time keeps
// its scene-relative t0/t1 (held inside the scene by clampAnchor); one made on the board has no time. No React here.
import type { IGuestCommentOwned } from '@/next/features/designly/access/api';
import { DECK_GUEST_PREFIX } from '@/next/features/designly/deck/types';
import { clampAnchor } from '@/next/features/designly/video/timeline-model';
import {
  VIDEO_FORMATS,
  type IVideoComment,
  type IVideoCommentTarget,
  type IVideoScene,
  type TVideoFormat,
} from '@/next/features/designly/video/types';

/** A guest's comment as the gateway lists it; the anchor's `state` (any shape) is checked here. */
export type TGuestVideoComment = IGuestCommentOwned;

export interface IVideoCommentTime {
  /** ms from the scene's start. */
  t0: number;
  t1: number | null;
  format: TVideoFormat;
  /** The scene is shorter now than the time the comment was made at. */
  changed: boolean;
}

export interface IVideoCommentItem {
  /** The id the resolve route takes: the team's ULID, or `g-<id>` of a guest's. */
  key: string;
  guest: boolean;
  /** 1-based number in the list. */
  n: number;
  /** The scene's id and its index in the video. */
  page: string;
  scene: number;
  who: string | null;
  text: string;
  /** The block it points at; '' when none. */
  block: string;
  /** The cached frame's path under the project; '' when none. */
  frame: string;
  /** Percent of the frame; null when the comment has no point. */
  x: number | null;
  y: number | null;
  /** null: made on the board, not on a time. */
  time: IVideoCommentTime | null;
  resolved: boolean;
  at: string;
}

const isFormat = (v: unknown): v is TVideoFormat => VIDEO_FORMATS.some((f) => f === v);
const ms = (v: unknown): number | null =>
  typeof v === 'number' && Number.isFinite(v) && v >= 0 ? Math.round(v) : null;

interface IGuestState {
  scene: string;
  t0: number;
  t1: number | null;
  format: TVideoFormat;
}

const readGuestState = (v: unknown): IGuestState | null => {
  if (typeof v !== 'object' || v === null) return null;
  const s = v as Record<string, unknown>;
  const t0 = ms(s.t0);
  const t1 = s.t1 === null || s.t1 === undefined ? null : ms(s.t1);
  if (typeof s.scene !== 'string' || t0 === null || !isFormat(s.format)) return null;
  if (s.t1 !== null && s.t1 !== undefined && t1 === null) return null;
  return { scene: s.scene, t0, t1, format: s.format };
};

const whoOf = (c: IVideoComment): string => (c.by.name !== '' ? c.by.name : c.by.email.split('@')[0]);

/** The merged list; a comment on a scene the video no longer has is left out. */
export const mergeVideoComments = (
  scenes: readonly IVideoScene[],
  team: readonly IVideoComment[],
  guests: readonly TGuestVideoComment[] | null,
  guestResolved: readonly string[],
): IVideoCommentItem[] => {
  const byId = new Map(scenes.map((s, i) => [s.id, i]));
  const byPath = new Map(scenes.map((s, i) => [s.path, i]));
  const items: Omit<IVideoCommentItem, 'n'>[] = [];

  for (const c of team) {
    const scene = byId.get(c.page);
    if (scene === undefined) continue;
    const at = c.video ? clampAnchor(scenes[scene], c.video.t0, c.video.t1) : null;
    items.push({
      key: c.id,
      guest: false,
      page: c.page,
      scene,
      who: whoOf(c),
      text: c.text,
      block: c.video ? c.video.block : c.label,
      frame: c.video ? c.video.frame : '',
      x: c.video ? null : c.x,
      y: c.video ? null : c.y,
      time: c.video && at ? { t0: at.t0, t1: at.t1, format: c.video.format, changed: at.changed } : null,
      resolved: c.resolved,
      at: c.at,
    });
  }

  const done = new Set(guestResolved);
  for (const g of guests ?? []) {
    const state = readGuestState(g.state);
    const scene = state ? (byId.get(state.scene) ?? byPath.get(state.scene)) : byPath.get(g.board);
    if (scene === undefined) continue;
    const key = `${DECK_GUEST_PREFIX}${g.id}`;
    const at = state ? clampAnchor(scenes[scene], state.t0, state.t1) : null;
    items.push({
      key,
      guest: true,
      page: scenes[scene].id,
      scene,
      who: g.name === '' ? null : g.name,
      text: g.text,
      block: g.label,
      frame: '',
      x: g.x === null ? null : g.x * 100,
      y: g.y === null ? null : g.y * 100,
      time: state && at ? { t0: at.t0, t1: at.t1, format: state.format, changed: at.changed } : null,
      resolved: done.has(key),
      at: g.at,
    });
  }

  // ISO times order as text; a stable sort keeps the team's before a guest's of the same second.
  items.sort((a, b) => (a.at < b.at ? -1 : a.at > b.at ? 1 : 0));
  return items.map((item, i) => ({ ...item, n: i + 1 }));
};

export const unresolvedCount = (items: readonly IVideoCommentItem[]): number =>
  items.filter((c) => !c.resolved).length;

/** ms of the whole video a comment's time starts at. */
export const commentStart = (scenes: readonly IVideoScene[], item: IVideoCommentItem): number | null =>
  item.time ? scenes[item.scene].start + item.time.t0 : null;

/** 0:12 — minutes and whole seconds. */
export const clock = (ms: number): string => {
  const s = Math.floor(ms / 1000);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
};

/** 0:12.4 — with tenths, for what the agent reads. */
export const clockTenths = (ms: number): string => {
  const tenths = Math.floor(ms / 100);
  return `${clock(ms)}.${tenths % 10}`;
};

/** «0:12–0:15» for a stretch, «0:12» for a moment; ms of the whole video. */
export const spanText = (from: number, to: number | null): string =>
  to === null ? clock(from) : `${clock(from)}–${clock(to)}`;

export interface ITargetParts {
  /** 1-based scene number. */
  scene: number;
  time: string;
  /** The block picked, or the scene's title where none. */
  name: string;
  format: TVideoFormat;
}

/** The parts of «Сцена 3 · 0:12–0:15 · Заголовок · 9:16»; null when the target's scene is gone. */
export const targetParts = (
  scenes: readonly IVideoScene[],
  target: IVideoCommentTarget,
): ITargetParts | null => {
  const index = scenes.findIndex((s) => s.id === target.page);
  if (index < 0) return null;
  const scene = scenes[index];
  const block = target.pick?.block ?? null;
  return {
    scene: index + 1,
    time: spanText(scene.start + target.t0, target.t1 === null ? null : scene.start + target.t1),
    name: block !== null && block !== '' ? block : scene.title,
    format: target.format,
  };
};
