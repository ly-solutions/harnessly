// A video comment (or a stretch of the timeline) as the agent reads it: one `<annotation kind="video">` block that goes
// with the next message. Whatever the page, a guest or the board supplied is one line each and cannot close the block,
// so those words cannot forge a line or an instruction. No React here.
import { clockTenths } from '@/next/features/designly/video/comments-model';
import type { IVideoScene, TVideoFormat } from '@/next/features/designly/video/types';

export const VIDEO_CTX_ID = 'video-context';

const MAX_LINE = 200;
const MAX_HTML = 300;
const MAX_COMMENT = 4000;

const stripControls = (text: string) =>
  text
    .replace(/\u001b/g, '␛')
    .replace(/[\u2028\u2029\u0085]/g, '\n')
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/g, '');
const oneLine = (text: string) => stripControls(text).replace(/\s+/g, ' ').trim();
const clip = (text: string, max: number) => (text.length > max ? `${text.slice(0, max - 1)}…` : text);
const keepInside = (text: string) => text.replace(/<(\/?)(annotation)/gi, '<\\$1$2');
const attr = (text: string) =>
  oneLine(text).replace(
    /[&<>"]/g,
    (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c] ?? c,
  );

/** 2.4 — seconds with one decimal, no unit. */
const secs = (ms: number): string => (Math.floor(ms / 100) / 10).toFixed(1);

export interface IVideoAnnotationInput {
  n: number;
  /** The project's id, for the frame command. */
  project: string;
  scene: IVideoScene;
  /** 1-based scene number. */
  sceneNo: number;
  format: TVideoFormat;
  /** ms from the scene's start; t1 null: a moment. */
  t0: number;
  t1: number | null;
  /** The block's data-block; '' when none. */
  block: string;
  /** The block's markup where known; '' otherwise. */
  html: string;
  /** The cached frame's path under the project; '' when it is not there yet. */
  frame: string;
  /** '' for a stretch sent without words. */
  comment: string;
}

const timeLine = (scene: IVideoScene, t0: number, t1: number | null): string => {
  const from = scene.start + t0;
  if (t1 === null) return `time: ${clockTenths(from)} (в сцене ${secs(t0)} с)`;
  const to = scene.start + t1;
  return `time: ${clockTenths(from)}–${clockTenths(to)} (в сцене ${secs(t0)}–${secs(t1)} с)`;
};

const frameLine = (i: IVideoAnnotationInput): string =>
  i.frame !== ''
    ? `frame: ${oneLine(i.frame)}   (открой Read)`
    : `frame: нет в кэше — design video frame ${oneLine(i.project)} ${oneLine(i.scene.id)} --at ${secs(i.t0)}s --format ${i.format}`;

/** The block in the format of the concept's §5. */
export const formatVideoAnnotation = (i: IVideoAnnotationInput): string => {
  const block = oneLine(i.block);
  const html = oneLine(i.html);
  const typed = clip(stripControls(i.comment).trim(), MAX_COMMENT);
  return [
    `<annotation n="${i.n}" kind="video">`,
    keepInside(
      `scene: ${i.sceneNo} «${clip(oneLine(i.scene.title), MAX_LINE)}» (board: ${clip(oneLine(i.scene.path), MAX_LINE)}, page ${attr(i.scene.id)}) · format ${i.format}`,
    ),
    timeLine(i.scene, i.t0, i.t1),
    ...(block === ''
      ? []
      : [keepInside(`block: ${clip(block, MAX_LINE)}${html === '' ? '' : ` — ${clip(html, MAX_HTML)}`}`)]),
    keepInside(frameLine(i)),
    ...(typed === '' ? [] : [`comment: ${keepInside(typed)}`]),
    '</annotation>',
  ].join('\n');
};
