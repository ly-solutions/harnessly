// Drawing over a Sinopia board and picking an area of it: the shapes are kept in the page's CSS pixels from
// its top-left (the document's, not the screen's), so they stand where they were drawn through the scroll, the zoom and
// the device's frame.

/** The bar's pick button has three tools: a block (as before), an area, a drawing. */
export const PICK_TOOL = { block: 'block', area: 'area', draw: 'draw' } as const;
export type TPickTool = (typeof PICK_TOOL)[keyof typeof PICK_TOOL];

/** The tools of the palette. */
export const DRAW_TOOL = {
  pen: 'pen',
  marker: 'marker',
  arrow: 'arrow',
  rect: 'rect',
  label: 'label',
  eraser: 'eraser',
} as const;
export type TDrawTool = (typeof DRAW_TOOL)[keyof typeof DRAW_TOOL];
export type TShapeKind = Exclude<TDrawTool, 'eraser'>;

export type TPoint = readonly [number, number];

/** One shape: a stroke (the points it went through), an arrow or a frame (its two ends), a label (its point, words). */
export interface IShape {
  id: string;
  kind: TShapeKind;
  color: string;
  w: number;
  pts: readonly TPoint[];
  text?: string;
}

/** A rectangle of the page in its CSS pixels. */
export interface IPageRect {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** The colours of the palette: the accent red first. */
export const DRAW_COLORS = ['#e5484d', '#f5a524', '#30a46c', '#3e63dd', '#1c2024'] as const;
/** The two widths of the pen; the marker draws four times as wide. */
export const DRAW_WIDTHS = [3, 6] as const;

/** Where the person stood when they drew: the board, the device and its size, the zoom, the page's scroll. */
export interface IDrawView {
  board: string;
  device: string;
  /** The page's viewport, CSS px. */
  vw: number;
  vh: number;
  /** The canvas's zoom, %. */
  zoom: number;
  /** The document's scroll when sent. */
  sx: number;
  sy: number;
}
