// The menu of a right click (a two-finger press) on the page of an open design: what can be done with the
// block under the pointer — change it with the agent, edit its words by hand, ask about it — then the page's settings,
// the developer tools and the page's downloads.
import { Fragment, useEffect, useLayoutEffect, useRef } from 'react';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';

export interface ICanvasMenuItem {
  id: string;
  icon: string;
  label: string;
  /** A key or a gesture that does the same. */
  kbd?: string;
}

interface ICanvasMenuProps {
  /** Where it opens, in the section's pixels. */
  x: number;
  y: number;
  state: string;
  /** The name of the block it is about; empty on the page's bare ground. */
  label: string;
  /** The parts of the menu, a line between them; empty ones are not drawn. */
  parts: readonly (readonly ICanvasMenuItem[])[];
  onPick: (id: string) => void;
  onClose: () => void;
}

const CanvasMenu = ({ x, y, state, label, parts, onPick, onClose }: ICanvasMenuProps) => {
  const ref = useRef<HTMLDivElement>(null);
  // It stays inside the section: near the right or the bottom edge it opens back from the point.
  useLayoutEffect(() => {
    const el = ref.current;
    const home = el?.offsetParent;
    if (!el || !home) return;
    const left = Math.max(8, Math.min(x, home.clientWidth - el.offsetWidth - 8));
    const top = Math.max(8, Math.min(y, home.clientHeight - el.offsetHeight - 8));
    el.style.setProperty('--cx', `${left}px`);
    el.style.setProperty('--cy', `${top}px`);
    el.querySelector('button')?.focus({ preventScroll: true });
  }, [x, y]);
  useEffect(() => {
    const onDown = (event: PointerEvent) => {
      if (!ref.current?.contains(event.target as Node)) onClose();
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      event.preventDefault();
      onClose();
    };
    window.addEventListener('pointerdown', onDown);
    window.addEventListener('keydown', onKey);
    // A press on the page is in the board's own window.
    const timer = window.setTimeout(() => window.addEventListener('blur', onClose), 0);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener('pointerdown', onDown);
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('blur', onClose);
    };
  }, [onClose]);
  return (
    <div
      ref={ref}
      className="hx-site-menu hx-canvasmenu"
      data-state={state}
      role="menu"
      aria-label={label || t('next.site.canvasMenu')}
      onContextMenu={(event) => event.preventDefault()}
    >
      {label && <div className="hx-dl-pop__label hx-canvasmenu__name">{label}</div>}
      {parts
        .filter((part) => part.length > 0)
        .map((part, i) => (
          <Fragment key={part[0].id}>
            {i > 0 && <div className="hx-more__sep" />}
            {part.map((item) => (
              <button
                key={item.id}
                type="button"
                role="menuitem"
                className="hx-option hx-more__item"
                onClick={() => {
                  onClose();
                  onPick(item.id);
                }}
              >
                <Icon name={item.icon} size="sm" />
                <span className="hx-more__label">{item.label}</span>
                {item.kbd && <kbd className="hx-more__kbd">{item.kbd}</kbd>}
              </button>
            ))}
          </Fragment>
        ))}
    </div>
  );
};

export default CanvasMenu;
