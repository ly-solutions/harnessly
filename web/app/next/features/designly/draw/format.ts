// What the agent reads of a drawing or an area: an `<annotation kind="drawing|area">` block — where the
// person stood (the board, the device, its size, the zoom, the scroll), each shape with its colour, its box in the
// page's pixels and the blocks under it (with the board's file and line when found), the pictures attached, the
// person's words. Whatever the page supplied is one line each and cannot close the block.
import { COLOR_NAMES, shapeBox } from '@/next/features/designly/draw/shapes';
import type { IDrawView, IPageRect, IShape } from '@/next/features/designly/draw/types';
import type { IPicked } from '@/next/features/designly/page-bridge';

const stripControls = (text: string) =>
  text
    .replace(/\x1b/g, '␛')
    .replace(/[\u2028\u2029\u0085]/g, '\n')
    .replace(/[\x00-\x08\x0b-\x1f\x7f-\x9f]/g, '');
const oneLine = (text: string) => stripControls(text).replace(/\s+/g, ' ').trim();
const clip = (text: string, max: number) => (text.length > max ? `${text.slice(0, max - 1)}…` : text);
const keepInside = (text: string) => text.replace(/<(\/?)(annotation)/gi, '<\\$1$2');
const escapeAttr = (text: string) =>
  text.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c] ?? c);

/** A block under a shape, with where its words are in the board's source when that was found. */
export interface IDrawnBlock {
  picked: IPicked;
  /** «file.dc.html:42»; null when not found. */
  template: string | null;
}

export interface IDrawnShape {
  shape: IShape;
  blocks: readonly IDrawnBlock[];
}

export interface IDrawFormatInput {
  kind: 'drawing' | 'area';
  view: IDrawView;
  /** The area pulled (kind area). */
  area?: IPageRect;
  /** The blocks inside the area (kind area). */
  areaBlocks?: readonly IDrawnBlock[];
  shapes: readonly IDrawnShape[];
  /** The names of the attached pictures: the board with the drawing, the drawing alone. */
  pictures: readonly string[];
  comment: string;
  /** The look tried on the first block, as the card words it; '' none. */
  style?: string;
  url: string;
  port: number;
}

const r = (n: number) => Math.round(n);
const box = (b: IPageRect) => `x=${r(b.x)} y=${r(b.y)} w=${r(b.w)} h=${r(b.h)}`;
const colorName = (c: string) => COLOR_NAMES[c] ?? c;

const blockLine = ({ picked, template }: IDrawnBlock) => {
  const text = clip(oneLine(picked.text), 80);
  const name = picked.component ? ` (${clip(oneLine(picked.component), 60)})` : '';
  return keepInside(
    `  - ${clip(oneLine(picked.selector), 240)}${name}${text ? ` — «${text}»` : ''}${template ? ` · template: ${oneLine(template)}` : ''}`,
  );
};

const shapeLine = (i: number, { shape }: IDrawnShape) => {
  const head = `shape ${i + 1}: ${shape.kind}, ${colorName(shape.color)}, box ${box(shapeBox(shape))}`;
  if (shape.kind === 'arrow') {
    const [a, b] = shape.pts;
    return `${head}, from (${r(a[0])}, ${r(a[1])}) to (${r(b[0])}, ${r(b[1])}) — it points at the blocks at its tip`;
  }
  if (shape.kind === 'label')
    return keepInside(
      `${head}, text «${clip(oneLine(shape.text ?? ''), 200)}» — the blocks listed are the nearest`,
    );
  if (shape.kind === 'marker') return `${head} — a highlighter over what it covers`;
  return head;
};

export const formatDrawing = (input: IDrawFormatInput): string => {
  const { kind, view } = input;
  const attributes = [
    ['kind', kind],
    ['url', oneLine(input.url)],
    ['target', `port:${input.port}`],
  ]
    .map(([name, value]) => `${name}="${escapeAttr(value)}"`)
    .join(' ');
  const lines = [
    `<annotation ${attributes}>`,
    keepInside(
      `board: ${oneLine(view.board)} · device: ${oneLine(view.device)} ${r(view.vw)}×${r(view.vh)} · zoom: ${r(view.zoom)}% · scroll: ${r(view.sx)}, ${r(view.sy)}`,
    ),
    'coordinates: CSS pixels of the page from its top-left (the document, not the screen)',
  ];
  if (kind === 'area' && input.area) {
    lines.push(`area: ${box(input.area)}`);
    const blocks = input.areaBlocks ?? [];
    if (blocks.length > 0) lines.push('blocks inside:', ...blocks.map(blockLine));
  }
  input.shapes.forEach((s, i) => {
    lines.push(shapeLine(i, s));
    if (s.blocks.length > 0) lines.push(...s.blocks.map(blockLine));
  });
  if (input.pictures.length > 0)
    lines.push(
      keepInside(
        `pictures: ${input.pictures.map(oneLine).join(', ')} (the board's visible part with the marks over it; the marks alone on a clear ground)`,
      ),
    );
  if (input.style)
    lines.push(
      keepInside(`style (already tried on the page, change the source to match): ${oneLine(input.style)}`),
    );
  const typed = stripControls(input.comment).trim();
  if (typed) lines.push(`comment: ${keepInside(typed)}`);
  lines.push('</annotation>');
  return lines.join('\n');
};
