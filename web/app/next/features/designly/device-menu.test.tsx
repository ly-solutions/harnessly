// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render } from '@testing-library/react';
import DeviceMenu from '@/next/features/designly/device-menu';
import { DEVICES, deviceOf } from '@/next/features/designly/devices';
import { t } from '@/next/i18n';

const zoom = () => ({
  pct: 100,
  zoomed: false,
  canIn: true,
  canOut: true,
  zoomIn: vi.fn(),
  zoomOut: vi.fn(),
  fit: vi.fn(),
  std: true,
  zoomTo: vi.fn(),
  locked: false,
  onLock: vi.fn(),
});

describe('the size menu’s zoom (HRN-1764)', () => {
  it('is held at 100% «По окну»: a lock, a hint, the buttons do nothing', () => {
    const z = zoom();
    const { container } = render(
      <DeviceMenu
        state="open"
        device={deviceOf('fill')}
        size={null}
        zoom={z}
        onPick={vi.fn()}
        onSize={vi.fn()}
      />,
    );
    expect(container.querySelector('.hx-devscale__lock')).not.toBeNull();
    expect(container.querySelector('.hx-devsize__hint')?.textContent).toBe(t('next.site.zoomLocked'));
    fireEvent.click(container.querySelector(`[aria-label="${t('next.site.zoomIn')}"]`)!);
    fireEvent.click(container.querySelector(`[aria-label="${t('next.site.zoomOut')}"]`)!);
    expect(z.zoomIn).not.toHaveBeenCalled();
    expect(z.zoomOut).not.toHaveBeenCalled();
  });
  it('works as it did on a device', () => {
    const z = zoom();
    const phone = DEVICES.find((d) => d.id !== 'fill' && d.id !== 'overview')!;
    const { container } = render(
      <DeviceMenu
        state="open"
        device={phone}
        size={{ w: 390, h: 844 }}
        zoom={z}
        onPick={vi.fn()}
        onSize={vi.fn()}
      />,
    );
    expect(container.querySelector('.hx-devscale__lock')).toBeNull();
    fireEvent.click(container.querySelector(`[aria-label="${t('next.site.zoomIn')}"]`)!);
    expect(z.zoomIn).toHaveBeenCalledOnce();
  });
});
