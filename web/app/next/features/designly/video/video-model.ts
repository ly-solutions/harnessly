// What the video's shell works out from the state it is given: the scene under a time, the format and source in use,
// the target of a new comment, a render's badge. Pure; durations and starts come from the runtime and are only read.
import { DECK_JOB } from '@/next/features/designly/deck/types';
import {
  VIDEO_SOURCE,
  type IVideoCommentTarget,
  type IVideoPick,
  type IVideoRange,
  type IVideoRenderInfo,
  type IVideoRenderJob,
  type IVideoScene,
  type IVideoState,
  type TVideoFormat,
  type TVideoSource,
} from '@/next/features/designly/video/types';

/** The scene a time (ms of the whole video) falls in; the last scene owns the very end. Null for no scenes. */
export const sceneAt = (scenes: readonly IVideoScene[], time: number): IVideoScene | null => {
  if (scenes.length === 0) return null;
  const found = scenes.find((s) => time >= s.start && time < s.start + s.dur);
  if (found) return found;
  return time < scenes[0].start ? scenes[0] : scenes[scenes.length - 1];
};

/** The format on screen: the person's pick while the video still has it, else the first enabled one. */
export const formatOf = (video: IVideoState, chosen: TVideoFormat | null): TVideoFormat => {
  const enabled = video.settings.formats;
  return chosen !== null && enabled.includes(chosen) ? chosen : enabled[0];
};

export const renderOf = (video: IVideoState, format: TVideoFormat): IVideoRenderInfo | null =>
  video.renders.find((r) => r.format === format) ?? null;

export const RENDER_BADGE = { none: 'none', current: 'current', outdated: 'outdated' } as const;
export type TRenderBadge = (typeof RENDER_BADGE)[keyof typeof RENDER_BADGE];

/** A published MP4 is current while its inputs key is today's key; any other key makes it outdated. */
export const renderBadge = (info: IVideoRenderInfo | null): TRenderBadge => {
  if (info === null || !info.published) return RENDER_BADGE.none;
  return info.key === info.currentKey ? RENDER_BADGE.current : RENDER_BADGE.outdated;
};

export const jobActive = (job: IVideoRenderJob | null): job is IVideoRenderJob =>
  job !== null && (job.state === DECK_JOB.queued || job.state === DECK_JOB.running);

/** What the player shows: the MP4 only while the format has one published. */
export const sourceOf = (video: IVideoState, format: TVideoFormat, want: TVideoSource): TVideoSource =>
  want === VIDEO_SOURCE.mp4 && renderOf(video, format)?.published === true
    ? VIDEO_SOURCE.mp4
    : VIDEO_SOURCE.live;

/** The comment on the moment under the playhead, or on the part of a range inside one scene. */
export const targetOf = (
  video: IVideoState,
  format: TVideoFormat,
  time: number,
  range: IVideoRange | null,
  pick: IVideoPick | null,
): IVideoCommentTarget | null => {
  const at = range ? range.t0 : time;
  const scene = pick
    ? (video.scenes.find((s) => s.id === pick.page) ?? sceneAt(video.scenes, at))
    : sceneAt(video.scenes, at);
  if (scene === null) return null;
  const t0 = Math.min(scene.dur, Math.max(0, at - scene.start));
  // A range reaching past the scene's end is cut at it: a comment belongs to one scene.
  const t1 = range ? Math.min(scene.dur, Math.max(t0, range.t1 - scene.start)) : null;
  return { page: scene.id, t0, t1: t1 !== null && t1 > t0 ? t1 : null, format, pick };
};

/** «0:12»: ms as minutes and seconds. */
export const clock = (ms: number): string => {
  const total = Math.max(0, Math.floor(ms / 1000));
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`;
};
