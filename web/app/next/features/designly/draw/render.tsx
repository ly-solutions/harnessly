// The shapes as SVG: one description of each shape's parts, drawn by React on the page and written as markup
// for the picture sent to the agent, so both show the same.
import { createElement } from 'react';
import { arrowHead, strokeOf, strokePath } from '@/next/features/designly/draw/shapes';
import type { IShape } from '@/next/features/designly/draw/types';

interface IPart {
  tag: 'path' | 'rect' | 'text' | 'polygon';
  attrs: Record<string, string | number>;
  text?: string;
}

const LABEL_FONT = 13;

/** The SVG parts of a shape, in the page's pixels. */
export const shapeParts = (s: IShape): IPart[] => {
  const line = {
    fill: 'none',
    stroke: s.color,
    'stroke-width': strokeOf(s),
    'stroke-linecap': 'round',
    'stroke-linejoin': 'round',
  };
  if (s.kind === 'pen') return [{ tag: 'path', attrs: { ...line, d: strokePath(s.pts) } }];
  if (s.kind === 'marker')
    return [{ tag: 'path', attrs: { ...line, d: strokePath(s.pts), 'stroke-opacity': 0.35 } }];
  if (s.kind === 'rect') {
    const [a, b] = s.pts;
    return [
      {
        tag: 'rect',
        attrs: {
          ...line,
          x: Math.min(a[0], b[0]),
          y: Math.min(a[1], b[1]),
          width: Math.abs(a[0] - b[0]),
          height: Math.abs(a[1] - b[1]),
          rx: 4,
        },
      },
    ];
  }
  if (s.kind === 'arrow') {
    const [a, b] = s.pts;
    const [l, r] = arrowHead(a, b, s.w);
    return [
      { tag: 'path', attrs: { ...line, d: strokePath([a, b]) } },
      {
        tag: 'polygon',
        attrs: {
          fill: s.color,
          stroke: s.color,
          'stroke-linejoin': 'round',
          'stroke-width': 1,
          points: `${b[0]},${b[1]} ${l[0]},${l[1]} ${r[0]},${r[1]}`,
        },
      },
    ];
  }
  const [x, y] = s.pts[0];
  const text = s.text ?? '';
  const w = Math.max(24, text.length * 8.4 + 16);
  return [
    { tag: 'rect', attrs: { x, y: y - 11, width: w, height: 22, rx: 6, fill: s.color } },
    {
      tag: 'text',
      attrs: {
        x: x + 8,
        y: y + 4.5,
        fill: '#ffffff',
        'font-size': LABEL_FONT,
        'font-family': 'Inter, system-ui, sans-serif',
        'font-weight': 600,
      },
      text,
    },
  ];
};

const reactName = (k: string) => k.replace(/-([a-z])/g, (_, c: string) => c.toUpperCase());

/** The shapes drawn in a group moved by the page's scroll. */
export const ShapesGroup = ({ shapes, sx, sy }: { shapes: readonly IShape[]; sx: number; sy: number }) => (
  <g transform={`translate(${-sx} ${-sy})`}>
    {shapes.flatMap((s) =>
      shapeParts(s).map((p, i) =>
        createElement(
          p.tag,
          {
            key: `${s.id}-${i}`,
            ...Object.fromEntries(Object.entries(p.attrs).map(([k, v]) => [reactName(k), v])),
          },
          p.text,
        ),
      ),
    )}
  </g>
);

const esc = (v: string | number) =>
  String(v).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c] ?? c);

/** The shapes as a standalone SVG document of w×h, the page scrolled to (sx, sy). */
export const shapesMarkup = (
  shapes: readonly IShape[],
  w: number,
  h: number,
  sx: number,
  sy: number,
  area?: { x: number; y: number; w: number; h: number },
): string => {
  const parts = shapes
    .flatMap(shapeParts)
    .map((p) => {
      const attrs = Object.entries(p.attrs)
        .map(([k, v]) => `${k}="${esc(v)}"`)
        .join(' ');
      return p.text !== undefined ? `<${p.tag} ${attrs}>${esc(p.text)}</${p.tag}>` : `<${p.tag} ${attrs}/>`;
    })
    .join('');
  // An area is drawn as the dashed frame the person pulled.
  const frame = area
    ? `<rect x="${area.x}" y="${area.y}" width="${area.w}" height="${area.h}" rx="4" fill="none" stroke="#e5484d" stroke-width="2" stroke-dasharray="6 4"/>`
    : '';
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}"><g transform="translate(${-sx} ${-sy})">${frame}${parts}</g></svg>`;
};
