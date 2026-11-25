// A drawing in the full-size view that can be zoomed and moved: the wheel (a trackpad pinch comes as ctrl+wheel) zooms
// toward the pointer, a drag moves, the buttons step, a double click or the arrow back puts it at the start.
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';

const MIN = 0.2;
const MAX = 8;
const STEP = 1.25;

interface IView {
  k: number;
  x: number;
  y: number;
}

const START: IView = { k: 1, x: 0, y: 0 };
const clamp = (k: number) => Math.min(MAX, Math.max(MIN, k));

const ZoomPan = ({ children }: { children: ReactNode }) => {
  const box = useRef<HTMLDivElement>(null);
  const [view, setView] = useState<IView>(START);
  const drag = useRef<{ x: number; y: number } | null>(null);
  const [grabbed, setGrabbed] = useState(false);

  // Zoom around a point of the box (its centre for the buttons), keeping the spot under it still.
  const zoomAt = useCallback((factor: number, px?: number, py?: number) => {
    const rect = box.current?.getBoundingClientRect();
    const cx = px ?? (rect ? rect.width / 2 : 0);
    const cy = py ?? (rect ? rect.height / 2 : 0);
    setView((v) => {
      const k = clamp(v.k * factor);
      const r = k / v.k;
      return { k, x: cx - (cx - v.x) * r, y: cy - (cy - v.y) * r };
    });
  }, []);

  // A native listener: React's wheel handler is passive, so it could not stop the page from scrolling.
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
      const rect = el.getBoundingClientRect();
      const speed = event.ctrlKey ? 0.01 : 0.0015;
      zoomAt(Math.exp(-event.deltaY * speed), event.clientX - rect.left, event.clientY - rect.top);
    };
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => el.removeEventListener('wheel', onWheel);
  }, [zoomAt]);

  return (
    <div className="hx-zoom">
      <div
        ref={box}
        className="hx-zoom__stage"
        data-dragging={grabbed ? 'true' : undefined}
        onPointerDown={(event) => {
          if (event.button !== 0) return;
          drag.current = { x: event.clientX, y: event.clientY };
          setGrabbed(true);
          event.currentTarget.setPointerCapture(event.pointerId);
        }}
        onPointerMove={(event) => {
          const from = drag.current;
          if (!from) return;
          const dx = event.clientX - from.x;
          const dy = event.clientY - from.y;
          drag.current = { x: event.clientX, y: event.clientY };
          setView((v) => ({ ...v, x: v.x + dx, y: v.y + dy }));
        }}
        onPointerUp={() => {
          drag.current = null;
          setGrabbed(false);
        }}
        onPointerCancel={() => {
          drag.current = null;
          setGrabbed(false);
        }}
        onDoubleClick={() => setView(START)}
      >
        <div
          className="hx-zoom__canvas"
          style={{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.k})` }}
        >
          {children}
        </div>
      </div>
      <div className="hx-zoom__bar" role="group" aria-label={t('next.rich.zoom')}>
        <button
          type="button"
          className="hx-rich__btn"
          aria-label={t('next.rich.zoomOut')}
          onClick={() => zoomAt(1 / STEP)}
        >
          <Icon name="minus" size="sm" />
        </button>
        <span className="hx-zoom__pct">{Math.round(view.k * 100)}%</span>
        <button
          type="button"
          className="hx-rich__btn"
          aria-label={t('next.rich.zoomIn')}
          onClick={() => zoomAt(STEP)}
        >
          <Icon name="plus" size="sm" />
        </button>
        <button
          type="button"
          className="hx-rich__btn"
          aria-label={t('next.rich.zoomReset')}
          onClick={() => setView(START)}
        >
          <Icon name="rotate-ccw" size="sm" />
        </button>
      </div>
    </div>
  );
};

export default ZoomPan;
