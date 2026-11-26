// The pictures sent to the agent with a drawing or an area: the drawing alone on a clear ground, and the
// board's visible part (taken by the machine's browser at the person's viewport and scroll) with the drawing over it.
import { platformPath, runtimePath } from '@/next/core/net/machine';
import { shapesMarkup } from '@/next/features/designly/draw/render';
import type { IPageRect, IShape } from '@/next/features/designly/draw/types';
import type { MachineId } from '@/next/protocol/v2';

const SCALE = 2;
const SHOT_WAIT_MS = 45_000;

const loadImage = (src: string) =>
  new Promise<HTMLImageElement>((resolve, reject) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error('image'));
    img.src = src;
  });

const toPng = (canvas: HTMLCanvasElement) =>
  new Promise<Blob>((resolve, reject) =>
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('png'))), 'image/png'),
  );

/** The drawing alone, w×h CSS px of the page's viewport scrolled to (sx, sy), at twice the pixels. */
export const drawingPng = async (
  shapes: readonly IShape[],
  w: number,
  h: number,
  sx: number,
  sy: number,
  area?: IPageRect,
): Promise<Blob> => {
  const svg = shapesMarkup(shapes, w, h, sx, sy, area);
  const img = await loadImage(`data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`);
  const canvas = document.createElement('canvas');
  canvas.width = Math.round(w * SCALE);
  canvas.height = Math.round(h * SCALE);
  const g = canvas.getContext('2d');
  if (!g) throw new Error('canvas');
  g.drawImage(img, 0, 0, canvas.width, canvas.height);
  return toPng(canvas);
};

/** The board's visible part as the machine's browser shows it at this viewport and scroll. */
export const boardShot = async (
  machine: MachineId,
  artifact: string,
  board: string,
  w: number,
  h: number,
  y: number,
): Promise<Blob> => {
  const ctl = new AbortController();
  const timer = window.setTimeout(() => ctl.abort(), SHOT_WAIT_MS);
  try {
    const res = await fetch(runtimePath(platformPath('/design/site/shot'), machine), {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        artifact,
        board,
        w: Math.round(w),
        h: Math.round(h),
        y: Math.max(0, Math.round(y)),
        scale: SCALE,
      }),
      signal: ctl.signal,
    });
    if (!res.ok || !(res.headers.get('content-type') ?? '').startsWith('image/png')) throw new Error('shot');
    return await res.blob();
  } finally {
    window.clearTimeout(timer);
  }
};

/** The drawing over the board's picture, at the picture's size. */
export const overShot = async (shot: Blob, drawing: Blob): Promise<Blob> => {
  const [base, over] = await Promise.all([
    loadImage(URL.createObjectURL(shot)),
    loadImage(URL.createObjectURL(drawing)),
  ]);
  const canvas = document.createElement('canvas');
  canvas.width = base.naturalWidth;
  canvas.height = base.naturalHeight;
  const g = canvas.getContext('2d');
  if (!g) throw new Error('canvas');
  g.drawImage(base, 0, 0);
  g.drawImage(over, 0, 0, canvas.width, canvas.height);
  URL.revokeObjectURL(base.src);
  URL.revokeObjectURL(over.src);
  return toPng(canvas);
};
