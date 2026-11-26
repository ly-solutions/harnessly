// The menu of the bar's device button: the size of the viewport and the canvas's scale on one grid, then
// the devices with their silhouettes — «По окну», «Все экраны», and the shelves (iPhone, Samsung, other, tablets,
// computers) that unfold one at a time. Turning, the case, the browser and folding are on the pill beside the phone.
import { useState, type KeyboardEvent } from 'react';
import {
  DEVICE_SHELF,
  deviceShelves,
  deviceSizeText,
  keptSize,
  VIEWPORT_SIZE,
  type IDevice,
  type ISize,
  type TGlyph,
} from '@/next/features/designly/devices';
import type { IZoomControls } from '@/next/features/designly/site-dock';
import Icon from '@/next/ui/icon';
import { SheetGrip } from '@/next/ui/sheet';
import { t } from '@/next/i18n';

/** The little silhouette of a device: the shape and the proportions of its screen. */
export const DeviceGlyph = ({ glyph }: { glyph: TGlyph }) => (
  <span className="hx-dgw" aria-hidden="true">
    <span className="hx-dg" data-k={glyph}>
      <i />
    </span>
  </span>
);

interface IDeviceMenuProps {
  state: string;
  device: IDevice;
  /** The viewport now; null while the area is not measured. */
  size: ISize | null;
  zoom: IZoomControls;
  onPick: (id: string) => void;
  /** An own size of «По окну», typed. */
  onSize: (size: ISize) => void;
}

const DeviceRow = ({ device, on, onPick }: { device: IDevice; on: boolean; onPick: () => void }) => (
  <button
    type="button"
    role="menuitemradio"
    className="hx-option hx-devopt"
    aria-checked={on}
    onClick={onPick}
  >
    <DeviceGlyph glyph={device.glyph} />
    <span className="hx-devopt__name">{t(`next.site.devices.${device.id}`)}</span>
    <span className="hx-devopt__size">{deviceSizeText(device)}</span>
    <Icon name="check" size="sm" className="hx-devopt__check" />
  </button>
);

const DeviceMenu = ({ state, device, size, zoom, onPick, onSize }: IDeviceMenuProps) => {
  // The shelf of the device on screen is shut too: its row says which one is on.
  const [open, setOpen] = useState<string | null>(null);
  // «По окну» the page is the window's own size: its zoom is always 100%. The controls say so, and a press
  // only shakes the lock.
  const locked = device.id === 'fill';
  const [shake, setShake] = useState(0);
  const lockedTip = t('next.site.zoomLocked');
  const apply = (input: HTMLInputElement) => {
    const form = input.closest('.hx-devsize');
    const read = (side: string) =>
      Number.parseInt(form?.querySelector<HTMLInputElement>(`[data-size="${side}"]`)?.value ?? '', 10);
    const next = keptSize({ w: read('w'), h: read('h') });
    if (next && (next.w !== size?.w || next.h !== size?.h)) onSize(next);
  };
  const onKey = (event: KeyboardEvent<HTMLInputElement>) => {
    const el = event.currentTarget;
    if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
      event.preventDefault();
      const step = (event.shiftKey ? 10 : 1) * (event.key === 'ArrowUp' ? 1 : -1);
      el.value = String(Math.max(VIEWPORT_SIZE.min, (Number.parseInt(el.value, 10) || 0) + step));
      apply(el);
    }
    if (event.key === 'Enter') {
      event.preventDefault();
      apply(el);
    }
  };
  const side = (k: 'w' | 'h', label: string) => (
    <input
      key={`${k}${size?.[k]}`}
      className="hx-devsize__in"
      data-size={k}
      inputMode="numeric"
      defaultValue={size?.[k]}
      aria-label={label}
      onKeyDown={onKey}
      onBlur={(event) => apply(event.currentTarget)}
    />
  );
  return (
    <div className="hx-site-menu hx-site-menu--end hx-devmenu hx-sheet-phone" data-state={state} role="menu">
      <SheetGrip />
      <div className="hx-devsize">
        <span className="hx-devsize__label">{t('next.site.size')}</span>
        {side('w', t('next.site.width'))}
        <span className="hx-devsize__x">×</span>
        {side('h', t('next.site.height'))}
        <span />
        <span className="hx-devsize__label">{t('next.site.zoom')}</span>
        <span
          className="hx-devscale"
          role="group"
          aria-label={t('next.site.zoom')}
          data-locked={locked || undefined}
          title={locked ? lockedTip : undefined}
        >
          <button
            type="button"
            aria-label={t('next.site.zoomOut')}
            aria-disabled={locked || !zoom.canOut}
            onClick={() => (locked ? setShake((n) => n + 1) : zoom.canOut && zoom.zoomOut())}
          >
            <Icon name="minus" size="sm" />
          </button>
          <span aria-live="polite" className="hx-devscale__pct">
            {locked && (
              <Icon
                key={shake}
                name="lock"
                size="sm"
                className={shake > 0 ? 'hx-devscale__lock hx-devscale__lock--shake' : 'hx-devscale__lock'}
              />
            )}
            {locked ? 100 : zoom.pct}%
          </span>
          <button
            type="button"
            aria-label={t('next.site.zoomIn')}
            aria-disabled={locked || !zoom.canIn}
            onClick={() => (locked ? setShake((n) => n + 1) : zoom.canIn && zoom.zoomIn())}
          >
            <Icon name="plus" size="sm" />
          </button>
        </span>
        <button
          type="button"
          className="hx-head-btn hx-tipped hx-devsize__fit"
          aria-label={t('next.site.zoomFit')}
          aria-disabled={locked || undefined}
          onClick={() => (locked ? setShake((n) => n + 1) : zoom.fit())}
        >
          <Icon name="maximize-2" size="sm" />
          <span className="hx-tip" data-align="end">
            {t('next.site.zoomFit')}
          </span>
        </button>
        {locked && <p className="hx-devsize__hint">{lockedTip}</p>}
      </div>
      <div className="hx-dl-pop__label">{t('next.site.device')}</div>
      {deviceShelves().map(({ shelf, devices }) => {
        if (shelf === DEVICE_SHELF.view)
          return devices.map((d) => (
            <DeviceRow key={d.id} device={d} on={d.id === device.id} onPick={() => onPick(d.id)} />
          ));
        const current = devices.find((d) => d.id === device.id);
        const unfolded = open === shelf;
        return (
          <div key={shelf} className="hx-devshelf">
            <button
              type="button"
              role="menuitem"
              className="hx-option hx-devopt hx-devshelf__head"
              aria-expanded={unfolded}
              data-on={current !== undefined}
              onClick={() => setOpen(unfolded ? null : shelf)}
            >
              <DeviceGlyph glyph={devices[0].glyph === 'home' ? 'island' : devices[0].glyph} />
              <span className="hx-devopt__name">{t(`next.site.shelf.${shelf}`)}</span>
              <span className="hx-devopt__size">
                {current && t(`next.site.devices.${current.id}`).replace(/^(iPhone|Galaxy) /, '')}
              </span>
              <Icon name="chevron-down" size="sm" className="hx-devshelf__chev" />
            </button>
            {unfolded && (
              <div className="hx-devshelf__list">
                {devices.map((d) => (
                  <DeviceRow key={d.id} device={d} on={d.id === device.id} onPick={() => onPick(d.id)} />
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
};

export default DeviceMenu;
