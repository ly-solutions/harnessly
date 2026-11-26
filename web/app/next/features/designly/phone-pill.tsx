// The phone's pill: beside a phone or a tablet on the canvas, the way it is drawn — turned or
// upright, its case, its status bar, folded (a foldable), the browser it is shown in (any of them on any device), a
// fingertip instead of the pointer (no hover), and the page's settings. It stands on the phone's right, level with its
// top, and follows it as the canvas moves; with no room there it lies over the phone. Dragged, it stays where it was
// put; its menu (a right click) has the switches with names, back to its place, hide (floating-pill.tsx).
import { Fragment, useEffect, useRef, useState } from 'react';
import {
  PillMenuFoot,
  pillStore,
  useFloatingPill,
  usePillPrefs,
} from '@/next/features/designly/floating-pill';
import { besideBox, PILL_AXIS, PILL_KEY } from '@/next/features/designly/zoom-pill-pos';
import Icon from '@/next/ui/icon';
import { usePresentFlag } from '@/next/ui/presence';
import { t } from '@/next/i18n';
import { BROWSER_SHELF, BROWSERS } from '@/next/features/designly/devices';

/** The phone's pill's place and whether it is hidden, shared by every canvas of the page. */
export const PHONE_PILL = pillStore(PILL_KEY.phone);
export const usePhonePillHidden = (): boolean => usePillPrefs(PHONE_PILL).hidden;

const OUT_MS = 180;
/** What the menu (and a hint beside the pill) needs on the pill's right to open there. */
const MENU_ROOM = 260;
/** The canvas moves smoothly (a fit, a device's new size) this long at most: the pill follows it frame by frame. */
const FOLLOW_MS = 480;

export interface IPhoneToggle {
  on: boolean;
  onToggle: () => void;
}

export interface IPhoneBrowsers {
  /** The browser on screen. */
  id: string;
  /** The device's system's own. */
  native: string;
  onPick: (id: string) => void;
}

interface IPhonePillProps {
  /** There is a phone or a tablet on the canvas: the pill comes, and leaves with its animation when there is none. */
  active: boolean;
  rot: IPhoneToggle;
  /** Null where the device has no case (the bare phone). */
  frame: IPhoneToggle | null;
  status: IPhoneToggle;
  /** A foldable: folded, and what a press does. */
  fold: (IPhoneToggle & { tip: string }) | null;
  browser: IPhoneBrowsers;
  touch: IPhoneToggle;
  /** «Настройки страницы», after a line. */
  settings: (IPhoneToggle & { disabled: boolean }) | null;
  /** What moves the phone on the canvas or changes the pill: another device, a turn. */
  watch: string;
}

interface IItem {
  id: string;
  icon: string;
  label: string;
  /** The hint, where it says more than the name (the turn: what a press does). */
  tip?: string;
  toggle: IPhoneToggle;
}

/** The browsers beside the pill, shelf by shelf; the device's own is said to be its own. */
const BrowserMenu = ({ browser, onClose }: { browser: IPhoneBrowsers; onClose: () => void }) => (
  <div
    className="hx-site-menu hx-brpop"
    role="menu"
    aria-label={t('next.site.browserGroup')}
    onPointerDown={(event) => event.stopPropagation()}
    onContextMenu={(event) => event.stopPropagation()}
  >
    {Object.values(BROWSER_SHELF).map((shelf) => (
      <Fragment key={shelf}>
        <div className="hx-dl-pop__label">{t(`next.site.browserShelf.${shelf}`)}</div>
        {BROWSERS.filter((b) => b.shelf === shelf).map((b) => (
          <button
            key={b.id}
            type="button"
            role="menuitemradio"
            className="hx-option hx-brpop__row"
            aria-checked={b.id === browser.id}
            onClick={() => {
              browser.onPick(b.id);
              onClose();
            }}
          >
            <span className="hx-brpop__dot" aria-hidden="true" />
            <span className="hx-option__name">{t(`next.site.browsers.${b.id}`)}</span>
            {b.id === browser.native && <span className="hx-brpop__own">{t('next.site.browserOwn')}</span>}
          </button>
        ))}
      </Fragment>
    ))}
  </div>
);

/** The phone on the canvas, in the coordinates of the pill's parent. */
const phoneBox = (area: HTMLElement) => {
  const box = area.querySelector('.hx-vpbox');
  if (!box) return null;
  const a = area.getBoundingClientRect();
  const b = box.getBoundingClientRect();
  return { left: b.left - a.left, top: b.top - a.top, right: b.right - a.left };
};

/** The pill measures its place again while the canvas pans, zooms or the device changes its size. */
const followPhone = (area: HTMLElement, again: () => void) => {
  const canvas = area.querySelector('.hx-vparea');
  const box = area.querySelector('.hx-vpbox');
  let frame = 0;
  let until = 0;
  const tick = () => {
    again();
    frame = performance.now() < until ? requestAnimationFrame(tick) : 0;
  };
  const kick = () => {
    until = performance.now() + FOLLOW_MS;
    if (frame === 0) frame = requestAnimationFrame(tick);
  };
  const moves = new MutationObserver(kick);
  if (canvas) moves.observe(canvas, { attributes: true, attributeFilter: ['style', 'data-dev', 'data-rot'] });
  const sizes = new ResizeObserver(kick);
  if (box) sizes.observe(box);
  return () => {
    moves.disconnect();
    sizes.disconnect();
    cancelAnimationFrame(frame);
  };
};

export const PhonePill = ({
  active,
  rot,
  frame,
  status,
  fold,
  browser,
  touch,
  settings,
  watch,
}: IPhonePillProps) => {
  const hidden = usePillPrefs(PHONE_PILL).hidden;
  const shown = usePresentFlag(active && !hidden, OUT_MS);
  const [axis, setAxis] = useState<string>(PILL_AXIS.col);
  const [browsers, setBrowsers] = useState(false);
  if (browsers && !shown.mounted) setBrowsers(false);
  const ref = useRef<HTMLDivElement>(null);
  // The list of browsers folds on a press elsewhere and on Esc.
  useEffect(() => {
    if (!browsers) return;
    const onDown = (event: PointerEvent) => {
      if (!(event.target as Element).closest('.hx-brpop, [data-item="browser"]')) setBrowsers(false);
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setBrowsers(false);
    };
    const onBlur = () => setBrowsers(false);
    window.addEventListener('pointerdown', onDown);
    window.addEventListener('keydown', onKey);
    // A press on the page is in its own window.
    window.addEventListener('blur', onBlur);
    return () => {
      window.removeEventListener('pointerdown', onDown);
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('blur', onBlur);
    };
  }, [browsers]);
  const pill = useFloatingPill(ref, PHONE_PILL, {
    auto: (area, el) => {
      const box = phoneBox(area);
      if (!box) return null;
      // Measured standing up; lying over the phone it takes its other size before it is kept inside the canvas.
      el.dataset.axis = PILL_AXIS.col;
      const at = besideBox({ w: area.clientWidth, h: area.clientHeight }, box, {
        w: el.offsetWidth,
        h: el.offsetHeight,
      });
      el.dataset.axis = at.axis;
      return at.pos;
    },
    follow: followPhone,
    watch: `${watch}|${shown.mounted}`,
    // The menu and the hints open to the right where they have room, else to the left; a pill put by hand stands up.
    placed: (el, area) => {
      if (PHONE_PILL.get().pos !== null) el.dataset.axis = PILL_AXIS.col;
      const a = area.getBoundingClientRect();
      const p = el.getBoundingClientRect();
      el.dataset.side = a.right - p.left >= MENU_ROOM ? 'end' : 'start';
      el.dataset.drop = p.top + p.height / 2 - a.top > a.height / 2 ? 'up' : 'down';
      setAxis(el.dataset.axis ?? PILL_AXIS.col);
    },
  });
  if (!shown.mounted) return null;

  const items: IItem[] = [
    { id: 'rot', icon: 'rotate-cw', label: t('next.site.rotate'), toggle: rot },
    frame && { id: 'frame', icon: 'smartphone', label: t('next.site.phoneFrame'), toggle: frame },
    { id: 'status', icon: 'wifi', label: t('next.site.phoneStatus'), toggle: status },
    fold && { id: 'fold', icon: 'fold-vertical', label: fold.tip, toggle: fold },
    {
      id: 'touch',
      icon: 'locate',
      label: t('next.site.phoneTouch'),
      tip: t('next.site.phoneTouchTip'),
      toggle: touch,
    },
  ].filter((item): item is IItem => item !== null);
  const button = (item: IItem) => (
    <button
      key={item.id}
      type="button"
      className="hx-head-btn hx-tipped"
      aria-label={item.label}
      aria-pressed={item.toggle.on}
      data-item={item.id}
      onClick={item.toggle.onToggle}
    >
      <Icon name={item.icon} size="sm" />
      <span className="hx-tip">{item.tip ?? item.label}</span>
    </button>
  );
  const browserName = t(`next.site.browsers.${browser.id}`);

  return (
    <div
      ref={ref}
      className="hx-phonepill"
      data-state={shown.state}
      data-browsers={browsers}
      role="toolbar"
      aria-label={t('next.site.phonePill')}
      aria-orientation={axis === PILL_AXIS.col ? 'vertical' : 'horizontal'}
      {...pill.bind}
    >
      {items.filter((item) => item.id !== 'touch').map(button)}
      <button
        type="button"
        className="hx-head-btn hx-tipped"
        aria-label={t('next.site.browserOf', { name: browserName })}
        aria-haspopup="menu"
        aria-expanded={browsers}
        data-item="browser"
        onClick={() => setBrowsers((open) => !open)}
      >
        <Icon name="globe" size="sm" />
        {!browsers && <span className="hx-tip">{t('next.site.browserOf', { name: browserName })}</span>}
      </button>
      {items.filter((item) => item.id === 'touch').map(button)}
      {settings && (
        <>
          <span className="hx-phonepill__sep" />
          <button
            type="button"
            className="hx-head-btn hx-tipped"
            aria-label={t('next.site.tweaks.title')}
            aria-pressed={settings.on}
            aria-disabled={settings.disabled}
            data-item="settings"
            onClick={() => !settings.disabled && settings.onToggle()}
          >
            <Icon name="sliders-horizontal" size="sm" />
            <span className="hx-tip">
              {settings.disabled ? t('next.site.tweaksNone') : t('next.site.tweaks.title')}
            </span>
          </button>
        </>
      )}
      {browsers && <BrowserMenu browser={browser} onClose={() => setBrowsers(false)} />}
      {pill.menu.value !== null && (
        <div
          className="hx-site-menu hx-phonepill__menu"
          data-state={pill.menu.state}
          role="menu"
          aria-label={t('next.site.phonePill')}
          onPointerDown={(event) => event.stopPropagation()}
        >
          <div className="hx-dl-pop__label">{t('next.site.phonePill')}</div>
          {items.map((item) => (
            <button
              key={item.id}
              type="button"
              role="menuitemcheckbox"
              aria-checked={item.toggle.on}
              className="hx-option hx-phonepill__opt"
              onClick={item.toggle.onToggle}
            >
              <Icon name={item.icon} size="sm" />
              <span className="hx-option__name">{item.label}</span>
              <span className="hx-switch" data-on={item.toggle.on} aria-hidden="true">
                <i />
              </span>
            </button>
          ))}
          <PillMenuFoot
            placed={pill.prefs.pos !== null}
            onReset={pill.reset}
            onHide={pill.hide}
            resetLabel={t('next.site.phonePillReset')}
            hideLabel={t('next.site.phonePillHide')}
            hint={t('next.site.phonePillHint')}
          />
        </div>
      )}
    </div>
  );
};
