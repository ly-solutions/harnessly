// The geometry of the shapes: their boxes, the eraser's touch, the SVG they are drawn as.
import type { IPageRect, IShape, TPoint } from '@/next/features/designly/draw/types';

/** How wide a shape's line is drawn: the marker four times its pen. */
export const strokeOf = (s: Pick<IShape, 'kind' | 'w'>) => (s.kind === 'marker' ? s.w * 4 : s.w);

const LABEL_H = 22;
const LABEL_CHAR = 8.4;

/** The box a shape takes on the page, its line included. */
export const shapeBox = (s: IShape): IPageRect => {
  if (s.kind === 'label') {
    const [x, y] = s.pts[0];
    return { x, y: y - LABEL_H / 2, w: Math.max(24, (s.text ?? '').length * LABEL_CHAR + 16), h: LABEL_H };
  }
  let x0 = Infinity;
  let y0 = Infinity;
  let x1 = -Infinity;
  let y1 = -Infinity;
  for (const [x, y] of s.pts) {
    x0 = Math.min(x0, x);
    y0 = Math.min(y0, y);
    x1 = Math.max(x1, x);
    y1 = Math.max(y1, y);
  }
  const pad = strokeOf(s) / 2 + (s.kind === 'arrow' ? 8 : 0);
  return { x: x0 - pad, y: y0 - pad, w: x1 - x0 + pad * 2, h: y1 - y0 + pad * 2 };
};

/** The box of all the shapes; null for none. */
export const drawingBox = (shapes: readonly IShape[]): IPageRect | null => {
  if (shapes.length === 0) return null;
  const boxes = shapes.map(shapeBox);
  const x0 = Math.min(...boxes.map((b) => b.x));
  const y0 = Math.min(...boxes.map((b) => b.y));
  const x1 = Math.max(...boxes.map((b) => b.x + b.w));
  const y1 = Math.max(...boxes.map((b) => b.y + b.h));
  return { x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
};

/** A rectangle from two corners, either way round. */
export const rectOf = (a: TPoint, b: TPoint): IPageRect => ({
  x: Math.min(a[0], b[0]),
  y: Math.min(a[1], b[1]),
  w: Math.abs(a[0] - b[0]),
  h: Math.abs(a[1] - b[1]),
});

const segDist = (p: TPoint, a: TPoint, b: TPoint) => {
  const dx = b[0] - a[0];
  const dy = b[1] - a[1];
  const len = dx * dx + dy * dy;
  const t = len === 0 ? 0 : Math.max(0, Math.min(1, ((p[0] - a[0]) * dx + (p[1] - a[1]) * dy) / len));
  return Math.hypot(p[0] - (a[0] + t * dx), p[1] - (a[1] + t * dy));
};

/** Whether the eraser at p (radius r) touches the shape. */
export const touches = (s: IShape, p: TPoint, r: number): boolean => {
  const reach = r + strokeOf(s) / 2;
  if (s.kind === 'label') {
    const b = shapeBox(s);
    return p[0] >= b.x - r && p[0] <= b.x + b.w + r && p[1] >= b.y - r && p[1] <= b.y + b.h + r;
  }
  if (s.kind === 'rect') {
    const [a, c] = s.pts;
    const corners: TPoint[] = [a, [c[0], a[1]], c, [a[0], c[1]], a];
    return corners.slice(1).some((q, i) => segDist(p, corners[i], q) <= reach);
  }
  if (s.pts.length === 1) return Math.hypot(p[0] - s.pts[0][0], p[1] - s.pts[0][1]) <= reach;
  return s.pts.slice(1).some((q, i) => segDist(p, s.pts[i], q) <= reach);
};

/** A stroke's points thinned: a point closer than `min` to the last kept one is dropped (the last one stays). */
export const thin = (pts: readonly TPoint[], min = 1.5): TPoint[] => {
  if (pts.length <= 2) return [...pts];
  const out: TPoint[] = [pts[0]];
  for (const p of pts.slice(1, -1)) {
    const last = out[out.length - 1];
    if (Math.hypot(p[0] - last[0], p[1] - last[1]) >= min) out.push(p);
  }
  out.push(pts[pts.length - 1]);
  return out;
};

/** The arrow's head: the two points beside its tip. */
export const arrowHead = (a: TPoint, b: TPoint, w: number): [TPoint, TPoint] => {
  const len = 10 + w * 2;
  const angle = Math.atan2(b[1] - a[1], b[0] - a[0]);
  const side = (turn: number): TPoint => [
    b[0] - len * Math.cos(angle + turn),
    b[1] - len * Math.sin(angle + turn),
  ];
  return [side(Math.PI / 7), side(-Math.PI / 7)];
};

const fmt = (n: number) => String(Math.round(n * 10) / 10);

/** A stroke as an SVG path. */
export const strokePath = (pts: readonly TPoint[]): string =>
  pts.length === 0
    ? ''
    : pts.length === 1
      ? `M${fmt(pts[0][0])} ${fmt(pts[0][1])}l0.01 0`
      : pts.map(([x, y], i) => `${i === 0 ? 'M' : 'L'}${fmt(x)} ${fmt(y)}`).join('');

/** The colour's name for the agent. */
export const COLOR_NAMES: Record<string, string> = {
  '#e5484d': 'red',
  '#f5a524': 'orange',
  '#30a46c': 'green',
  '#3e63dd': 'blue',
  '#1c2024': 'black',
};
