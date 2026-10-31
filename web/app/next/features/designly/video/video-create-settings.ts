// What a new video is asked to be, as the create screen holds it, and the request's `video` it is sent as. No React.
import type { DesignlyVideo } from '@/next/protocol/v2';
import { VIDEO_FORMAT, type TVideoFormat } from '@/next/features/designly/video/types';

/** The formats choice: one of them or both. */
export const VIDEO_FORMATS_PICK = { landscape: 'landscape', portrait: 'portrait', both: 'both' } as const;
export type TVideoFormatsPick = (typeof VIDEO_FORMATS_PICK)[keyof typeof VIDEO_FORMATS_PICK];

export interface IVideoCreateSettings {
  formats: TVideoFormatsPick;
  /** null: the length does not matter, the agent decides. */
  seconds: number | null;
  subs: boolean;
}

export const DEFAULT_VIDEO_SETTINGS: IVideoCreateSettings = {
  formats: VIDEO_FORMATS_PICK.landscape,
  seconds: null,
  subs: true,
};

const FORMATS_OF: Record<TVideoFormatsPick, TVideoFormat[]> = {
  landscape: [VIDEO_FORMAT.landscape],
  portrait: [VIDEO_FORMAT.portrait],
  both: [VIDEO_FORMAT.landscape, VIDEO_FORMAT.portrait],
};

/** The request's `video`. */
export const toWireVideo = (s: IVideoCreateSettings): DesignlyVideo => ({
  formats: [...FORMATS_OF[s.formats]],
  seconds: s.seconds,
  subs: s.subs,
});
