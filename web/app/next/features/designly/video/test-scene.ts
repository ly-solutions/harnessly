// A scene for the timeline's and the comments' tests.
import type { IVideoScene } from '@/next/features/designly/video/types';

export const scene = (
  id: string,
  start: number,
  dur: number,
  moments: IVideoScene['moments'] = [],
): IVideoScene => ({
  id,
  path: `${id}.dc.html`,
  title: id,
  start,
  dur,
  auto: false,
  transition: { kind: 'none', ms: 0 },
  moments,
  words: 0,
  notes: '',
  version: 'v1',
  blank: false,
  keys: {},
});
