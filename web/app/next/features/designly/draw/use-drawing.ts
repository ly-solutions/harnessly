// The drawing over the board while the person makes it: its shapes with undo and redo, the tool, the colour
// and the width of the palette, the area pulled. The view holds it, so the layer over the page and the palette over
// the bar share it; it stays until sent or cleared.
import { useCallback, useMemo, useState } from 'react';
import {
  DRAW_COLORS,
  DRAW_TOOL,
  DRAW_WIDTHS,
  PICK_TOOL,
  type IPageRect,
  type IShape,
  type TDrawTool,
  type TPickTool,
} from '@/next/features/designly/draw/types';

interface IHistory {
  past: (readonly IShape[])[];
  now: readonly IShape[];
  future: (readonly IShape[])[];
}

const START: IHistory = { past: [], now: [], future: [] };
const DEPTH = 100;

export interface IDrawing {
  shapes: readonly IShape[];
  tool: TDrawTool;
  color: string;
  width: number;
  area: IPageRect | null;
  canUndo: boolean;
  canRedo: boolean;
  setTool: (tool: TDrawTool) => void;
  setColor: (color: string) => void;
  setWidth: (width: number) => void;
  /** Puts the shapes as they are now, a step of the history. */
  commit: (next: readonly IShape[]) => void;
  undo: () => void;
  redo: () => void;
  clear: () => void;
  setArea: (area: IPageRect | null) => void;
}

export const useDrawing = (): IDrawing => {
  const [h, setH] = useState<IHistory>(START);
  const [tool, setTool] = useState<TDrawTool>(DRAW_TOOL.pen);
  const [color, setColor] = useState<string>(DRAW_COLORS[0]);
  const [width, setWidth] = useState<number>(DRAW_WIDTHS[0]);
  const [area, setArea] = useState<IPageRect | null>(null);
  const commit = useCallback(
    (next: readonly IShape[]) =>
      setH((x) => (x.now === next ? x : { past: [...x.past, x.now].slice(-DEPTH), now: next, future: [] })),
    [],
  );
  const undo = useCallback(
    () =>
      setH((x) =>
        x.past.length === 0
          ? x
          : { past: x.past.slice(0, -1), now: x.past[x.past.length - 1], future: [x.now, ...x.future] },
      ),
    [],
  );
  const redo = useCallback(
    () =>
      setH((x) =>
        x.future.length === 0 ? x : { past: [...x.past, x.now], now: x.future[0], future: x.future.slice(1) },
      ),
    [],
  );
  const clear = useCallback(() => {
    setH((x) => (x.now.length === 0 ? x : { past: [...x.past, x.now].slice(-DEPTH), now: [], future: [] }));
    setArea(null);
  }, []);
  return useMemo(
    () => ({
      shapes: h.now,
      tool,
      color,
      width,
      area,
      canUndo: h.past.length > 0,
      canRedo: h.future.length > 0,
      setTool,
      setColor,
      setWidth,
      commit,
      undo,
      redo,
      clear,
      setArea,
    }),
    [h, tool, color, width, area, commit, undo, redo, clear],
  );
};

const toolKey = (conv: string) => `hx-pick-tool:${conv}`;
const TOOLS = Object.values(PICK_TOOL) as string[];

/** The pick tool a chat last used (a new chat starts with the block). */
export const readPickTool = (conv: string | null): TPickTool => {
  if (conv === null) return PICK_TOOL.block;
  try {
    const v = window.localStorage.getItem(toolKey(conv));
    return v !== null && TOOLS.includes(v) ? (v as TPickTool) : PICK_TOOL.block;
  } catch {
    return PICK_TOOL.block;
  }
};

export const keepPickTool = (conv: string | null, tool: TPickTool) => {
  if (conv === null) return;
  try {
    window.localStorage.setItem(toolKey(conv), tool);
  } catch {
    // The choice is a convenience: without the storage it lasts the page.
  }
};
