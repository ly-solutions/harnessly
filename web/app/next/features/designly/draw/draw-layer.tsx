// The layer over the board's page for an area or a drawing. It lies over the frame inside the device and the
// canvas's zoom, so its pixels are the page's viewport pixels; the page's scroll (told each frame while the layer is
// there, `tk:scroll-sync`) turns them into the page's own. The wheel still scrolls the page (`tk:scroll-to`); a click
// reaches nothing under it. It also asks the page which blocks lie under a rectangle (`tk:elements-in`).
import {
  use,
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
  type RefObject,
} from 'react';
import { readBridgeMessage, type IPicked } from '@/next/features/designly/page-bridge';
import { AreaCardContext, type INoteArea } from '@/next/features/designly/note-area';
import { ShapesGroup } from '@/next/features/designly/draw/render';
import { rectOf, thin, touches } from '@/next/features/designly/draw/shapes';
import type { IDrawing } from '@/next/features/designly/draw/use-drawing';
import {
  DRAW_TOOL,
  PICK_TOOL,
  type IPageRect,
  type IShape,
  type TPoint,
} from '@/next/features/designly/draw/types';
import { t } from '@/next/i18n';
import '@/next/styles/draw.css';

/** What the view asks of the layer when the card sends. */
export interface IDrawLayerApi {
  elementsIn: (viewRect: IPageRect) => Promise<IPicked[]>;
  /** The page's scroll and viewport now. */
  where: () => { sx: number; sy: number; vw: number; vh: number };
  /** Opens the card «Что здесь поправить?» over an area or a drawing; null where the view lends none. */
  openArea: ((area: INoteArea) => void) | null;
}

export interface IDrawLayerProps {
  frame: RefObject<HTMLIFrameElement | null>;
  origin: string;
  drawing: IDrawing;
  /** The tool on: an area pulled, a drawing made; null: the marks only shown. */
  mode: typeof PICK_TOOL.area | typeof PICK_TOOL.draw | null;
  apiRef: RefObject<IDrawLayerApi | null>;
  /** An area was pulled (page pixels). */
  onArea: (rect: IPageRect) => void;
}

let shapeSeq = 0;
const newId = () => `s${Date.now().toString(36)}${(shapeSeq += 1)}`;
const ELEMENTS_WAIT_MS = 1500;
const ERASER = 10;

interface ILive {
  shape?: IShape;
  /** The shapes while the eraser goes. */
  kept?: readonly IShape[];
  from?: TPoint;
  to?: TPoint;
}

const DrawLayer = ({ frame, origin, drawing, mode, apiRef, onArea }: IDrawLayerProps) => {
  const openArea = use(AreaCardContext);
  const box = useRef<HTMLDivElement>(null);
  const [scroll, setScroll] = useState<{ sx: number; sy: number }>({ sx: 0, sy: 0 });
  const [live, setLive] = useState<ILive | null>(null);
  const [label, setLabel] = useState<{ at: TPoint; view: TPoint; room: number } | null>(null);
  const pending = useRef(new Map<number, (items: IPicked[]) => void>());
  const req = useRef(0);
  const scrollNow = useRef(scroll);

  const post = (message: Record<string, unknown>) =>
    frame.current?.contentWindow?.postMessage(message, origin);

  // The page's scroll, each frame, while the layer is there; the blocks asked of it.
  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      const el = frame.current;
      if (!el || event.source !== el.contentWindow) return;
      const m = readBridgeMessage(event.data);
      if (m?.k === 'doc-scroll' || m?.k === 'scroll') {
        const next = { sx: 0, sy: m.y };
        scrollNow.current = next;
        setScroll(next);
      } else if (m?.k === 'elements') {
        pending.current.get(m.req)?.(m.items);
        pending.current.delete(m.req);
      }
    };
    window.addEventListener('message', onMessage);
    frame.current?.contentWindow?.postMessage({ type: 'tk:scroll-sync', on: true }, origin);
    const el = frame.current;
    return () => {
      window.removeEventListener('message', onMessage);
      el?.contentWindow?.postMessage({ type: 'tk:scroll-sync', on: false }, origin);
    };
  }, [frame, origin]);

  useEffect(() => {
    apiRef.current = {
      openArea,
      elementsIn: (r) =>
        new Promise((resolve) => {
          const id = (req.current += 1);
          pending.current.set(id, resolve);
          window.setTimeout(() => {
            if (pending.current.delete(id)) resolve([]);
          }, ELEMENTS_WAIT_MS);
          frame.current?.contentWindow?.postMessage(
            { type: 'tk:elements-in', req: id, x: r.x, y: r.y, w: Math.max(0, r.w), h: Math.max(0, r.h) },
            origin,
          );
        }),
      where: () => ({
        ...scrollNow.current,
        vw: box.current?.offsetWidth ?? 0,
        vh: box.current?.offsetHeight ?? 0,
      }),
    };
    return () => {
      apiRef.current = null;
    };
  }, [apiRef, frame, origin, openArea]);

  /** A pointer's place in the viewport's CSS px (the layer is scaled with the frame) and on the page. */
  const placeOf = (e: { clientX: number; clientY: number }): { view: TPoint; page: TPoint } => {
    const el = box.current;
    if (!el) return { view: [0, 0], page: [0, 0] };
    const r = el.getBoundingClientRect();
    const k = el.offsetWidth > 0 ? r.width / el.offsetWidth : 1;
    const view: TPoint = [(e.clientX - r.left) / k, (e.clientY - r.top) / k];
    return { view, page: [view[0] + scroll.sx, view[1] + scroll.sy] };
  };

  const onDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (mode === null || e.button !== 0) return;
    e.preventDefault();
    box.current?.focus({ preventScroll: true });
    const { view, page } = placeOf(e);
    if (mode === PICK_TOOL.area) {
      e.currentTarget.setPointerCapture(e.pointerId);
      setLive({ from: page, to: page });
      return;
    }
    const { tool, color, width } = drawing;
    if (tool === DRAW_TOOL.label) {
      setLabel({ at: page, view, room: box.current?.offsetWidth ?? 0 });
      return;
    }
    e.currentTarget.setPointerCapture(e.pointerId);
    if (tool === DRAW_TOOL.eraser) {
      setLive({ kept: drawing.shapes.filter((s) => !touches(s, page, ERASER)) });
      return;
    }
    const two = tool === DRAW_TOOL.arrow || tool === DRAW_TOOL.rect;
    setLive({ shape: { id: newId(), kind: tool, color, w: width, pts: two ? [page, page] : [page] } });
  };

  const onMove = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (!live) return;
    const { page } = placeOf(e);
    if (live.from) {
      setLive({ ...live, to: page });
      return;
    }
    if (live.kept) {
      setLive({ kept: live.kept.filter((s) => !touches(s, page, ERASER)) });
      return;
    }
    const s = live.shape;
    if (!s) return;
    const two = s.kind === 'arrow' || s.kind === 'rect';
    setLive({ shape: { ...s, pts: two ? [s.pts[0], page] : [...s.pts, page] } });
  };

  const onUp = () => {
    if (!live) return;
    setLive(null);
    if (live.from && live.to) {
      const r = rectOf(live.from, live.to);
      if (r.w >= 8 && r.h >= 8) onArea(r);
      return;
    }
    if (live.kept) {
      if (live.kept.length !== drawing.shapes.length) drawing.commit(live.kept);
      return;
    }
    const s = live.shape;
    if (!s) return;
    if (s.kind === 'arrow' || s.kind === 'rect') {
      const [a, b] = s.pts;
      if (Math.hypot(a[0] - b[0], a[1] - b[1]) < 6) return;
      drawing.commit([...drawing.shapes, s]);
      return;
    }
    drawing.commit([...drawing.shapes, { ...s, pts: thin(s.pts) }]);
  };

  const putLabel = (text: string) => {
    const words = text.trim();
    if (label && words !== '')
      drawing.commit([
        ...drawing.shapes,
        {
          id: newId(),
          kind: 'label',
          color: drawing.color,
          w: drawing.width,
          pts: [label.at],
          text: words.slice(0, 200),
        },
      ]);
    setLabel(null);
  };

  const shown = live?.kept ?? (live?.shape ? [...drawing.shapes, live.shape] : drawing.shapes);
  const pulled = live?.from && live.to ? rectOf(live.from, live.to) : (drawing.area ?? null);
  if (mode === null && shown.length === 0 && pulled === null) return null;
  return (
    <div
      ref={box}
      className="hx-draw-layer"
      data-mode={mode ?? undefined}
      data-tool={mode === PICK_TOOL.draw ? drawing.tool : undefined}
      tabIndex={-1}
      aria-label={t(mode === PICK_TOOL.area ? 'next.site.draw.areaHint' : 'next.site.draw.hint')}
      onPointerDown={onDown}
      onPointerMove={onMove}
      onPointerUp={onUp}
      onPointerCancel={() => setLive(null)}
      onWheel={(e) => {
        if (mode === null) return;
        post({ type: 'tk:scroll-to', y: Math.max(0, scroll.sy + e.deltaY) });
      }}
    >
      <svg className="hx-draw-layer__svg" aria-hidden="true">
        {pulled && (
          <rect
            className="hx-draw-area"
            x={pulled.x - scroll.sx}
            y={pulled.y - scroll.sy}
            width={pulled.w}
            height={pulled.h}
            rx={4}
          />
        )}
        <ShapesGroup shapes={shown} sx={scroll.sx} sy={scroll.sy} />
      </svg>
      {label && (
        <input
          className="hx-draw-label"
          style={{
            left: Math.max(4, Math.min(label.view[0], label.room - 208)),
            top: label.view[1] - 13,
          }}
          aria-label={t('next.site.draw.label')}
          placeholder={t('next.site.draw.labelPh')}
          maxLength={200}
          ref={(el) => el?.focus({ preventScroll: true })}
          onPointerDown={(e) => e.stopPropagation()}
          onBlur={(e) => putLabel(e.currentTarget.value)}
          onKeyDown={(e) => {
            e.stopPropagation();
            if (e.key === 'Enter') putLabel(e.currentTarget.value);
            if (e.key === 'Escape') setLabel(null);
          }}
        />
      )}
    </div>
  );
};

export default DrawLayer;
