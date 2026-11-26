// The pick button's tools for the design's view: which one is chosen (kept for the chat), whether the area
// or the drawing is on, the layer over the page, the palette, and the card they open — the view only puts these in
// place. The card is the view's own («Что здесь поправить?»): the layer, under the view's section, reaches it.
import { useEffect, useRef, useState } from 'react';
import type { IDrawLayerApi, IDrawLayerProps } from '@/next/features/designly/draw/draw-layer';
import DrawPalette from '@/next/features/designly/draw/palette';
import { drawMessage } from '@/next/features/designly/draw/send';
import { drawingBox } from '@/next/features/designly/draw/shapes';
import { keepPickTool, readPickTool, useDrawing } from '@/next/features/designly/draw/use-drawing';
import { PICK_TOOL, type IPageRect, type TPickTool } from '@/next/features/designly/draw/types';
import type { INoteArea } from '@/next/features/designly/note-area';
import { addSiteComment } from '@/next/features/designly/panel/api';
import { styleLine } from '@/next/features/designly/element-style';
import { toast } from '@/next/features/designly/sdk/toast';
import { t } from '@/next/i18n';
import type { MachineId } from '@/next/protocol/v2';

export interface IDrawToolOptions {
  conv: string | null;
  machine: MachineId | null;
  artifact: string | null;
  board: string | null;
  /** The device's name and the canvas's zoom, for the agent. */
  device: string;
  zoom: number;
  /** The block pick (the tool «Выбрать блок»). */
  pickOn: boolean;
  togglePick: () => void;
}

export const useDrawTool = (o: IDrawToolOptions) => {
  const [seen, setSeen] = useState(o.conv);
  const [tool, setTool] = useState<TPickTool>(() => readPickTool(o.conv));
  const [active, setActive] = useState(false);
  if (seen !== o.conv) {
    setSeen(o.conv);
    setTool(readPickTool(o.conv));
    setActive(false);
  }
  const drawing = useDrawing();
  const apiRef = useRef<IDrawLayerApi | null>(null);
  const latest = useRef(o);
  useEffect(() => {
    latest.current = o;
  });

  const openCard = async (kind: INoteArea['kind'], area?: IPageRect) => {
    const now = latest.current;
    const layer = apiRef.current;
    const page = kind === 'area' ? area : drawingBox(drawing.shapes);
    const open = layer?.openArea;
    if (!layer || !open || !page || !now.machine || !now.artifact || !now.board) return;
    const where = layer.where();
    const viewRect = { x: page.x - where.sx, y: page.y - where.sy, w: page.w, h: page.h };
    const blocks = await layer.elementsIn(viewRect);
    const shapes = drawing.shapes;
    const { machine, artifact, board } = { machine: now.machine, artifact: now.artifact, board: now.board };
    const view = {
      board,
      device: now.device,
      vw: where.vw,
      vh: where.vh,
      zoom: now.zoom,
      sx: where.sx,
      sy: where.sy,
    };
    open({
      kind,
      rect: { x: viewRect.x, y: viewRect.y, width: viewRect.w, height: viewRect.h },
      blocks,
      message: async (words) => {
        const at = apiRef.current?.where();
        const sent = await drawMessage({
          machine,
          artifact,
          kind,
          view: at ? { ...view, sx: at.sx, sy: at.sy } : view,
          shapes: kind === 'drawing' ? shapes : [],
          area: kind === 'area' ? area : undefined,
          elementsIn: (r) => apiRef.current?.elementsIn(r) ?? Promise.resolve([]),
          comment: words.comment,
          style: blocks[0] ? styleLine(words.style, blocks[0].look) : '',
          url: words.url,
          port: words.port,
        });
        drawing.clear();
        return sent;
      },
      onComment: async (text) => {
        try {
          await addSiteComment(machine, artifact, {
            board,
            x: Math.max(0, Math.min(100, ((page.x + page.w) / Math.max(1, where.vw)) * 100)),
            y: Math.max(0, page.y),
            block: blocks[0]?.selector ?? '',
            label: '',
            text,
            parent: '',
            mentions: [],
            agent: false,
            drawing: kind === 'drawing' ? shapes : [],
          });
          drawing.clear();
        } catch (error) {
          toast(error instanceof Error ? error.message : t('next.site.draw.failed'), { tone: 'error' });
          throw error;
        }
      },
    });
  };

  const finish = () => {
    setActive(false);
    if (tool === PICK_TOOL.draw && drawing.shapes.length > 0) void openCard('drawing');
  };

  /** The bar's button: the chosen tool on or off. */
  const toggle = () => {
    if (tool === PICK_TOOL.block) o.togglePick();
    else if (active) finish();
    else setActive(true);
  };

  /** A tool chosen from the list (or by its key): it is kept for the chat and turned on. */
  const choose = (next: TPickTool) => {
    keepPickTool(o.conv, next);
    setTool(next);
    if (next === PICK_TOOL.block) {
      setActive(false);
      if (!o.pickOn) o.togglePick();
      return;
    }
    if (o.pickOn) o.togglePick();
    setActive(true);
  };

  /** A tool's key: the tool on, or off when it is the one on. */
  const key = (x: TPickTool) => {
    if (x === tool && (x === PICK_TOOL.block ? o.pickOn : active)) toggle();
    else choose(x);
  };

  const latestFinish = useRef(finish);
  useEffect(() => {
    latestFinish.current = finish;
  });
  // While a tool is on: Esc ends it (the card opens for what was drawn), ⌘Z and ⇧⌘Z go back and forth.
  useEffect(() => {
    if (!active) return;
    const onKey = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement | null)?.closest?.('input, textarea, [contenteditable="true"]')) return;
      if (e.key === 'Escape') {
        e.preventDefault();
        latestFinish.current();
      } else if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'z') {
        e.preventDefault();
        if (e.shiftKey) drawing.redo();
        else drawing.undo();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [active, drawing]);

  const mode = active && tool !== PICK_TOOL.block ? tool : null;
  /** The layer is there while a tool is on or something drawn or pulled stays. */
  const layerShown = mode !== null || drawing.shapes.length > 0 || drawing.area !== null;
  const layerProps: Omit<IDrawLayerProps, 'frame' | 'origin'> = {
    drawing,
    mode,
    apiRef,
    onArea: (rect) => {
      drawing.setArea(rect);
      setActive(false);
      void openCard('area', rect);
    },
  };
  const palette = mode === PICK_TOOL.draw ? <DrawPalette drawing={drawing} onDone={finish} /> : null;
  return {
    tool,
    on: tool === PICK_TOOL.block ? o.pickOn : active,
    toggle,
    choose,
    key,
    layerShown,
    layerProps,
    palette,
  };
};
