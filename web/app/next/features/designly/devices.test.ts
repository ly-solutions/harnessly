import { describe, expect, it } from 'vitest';
import {
  BARS,
  browserOf,
  BROWSERS,
  DEVICE_GROUP,
  DEVICE_SHELF,
  deviceShelves,
  deviceOf,
  deviceSizeText,
  FILL_INSET,
  MIN_FIT,
  overviewBox,
  overviewOf,
  SCROLL_START,
  syncedScroll,
  trackScroll,
  viewportOf,
  viewportVars,
  VP_KIND,
} from '@/next/features/designly/devices';

const AREA = { w: 1172, h: 776 };

describe('viewportOf', () => {
  it('never draws a box at 0% in an area measured empty', () => {
    expect(viewportOf(deviceOf('ip15'), { w: 0, h: 0 }, null, false).fit).toBeGreaterThan(0);
    expect(viewportOf(deviceOf('fill'), { w: 600, h: 40 }, { w: 300, h: 900 }, false).fit).toBeGreaterThan(0);
  });

  it('«По окну» is the area with a margin of canvas round it', () => {
    expect(viewportOf(deviceOf('fill'), AREA, null, false)).toMatchObject({
      kind: VP_KIND.fill,
      vw: 1172 - 2 * FILL_INSET,
      vh: 776 - 2 * FILL_INSET,
      fit: 1,
    });
  });

  it('an own size of «По окну» is drawn at that size and scaled to fit', () => {
    const vp = viewportOf(deviceOf('fill'), AREA, { w: 1920, h: 1080 }, false);
    expect(vp).toMatchObject({ kind: VP_KIND.size, vw: 1920, vh: 1080 });
    expect(vp.fit).toBeCloseTo((1172 - 48) / 1920, 4);
  });

  it('a phone in its case of Devices.css scales the page into the case screen', () => {
    const vp = viewportOf(deviceOf('ip15'), AREA, null, false);
    expect(vp.kind).toBe(VP_KIND.frame);
    expect(vp.k).toBeCloseTo(390 / 393, 4);
    expect(vp.cvh).toBe(Math.round(830 / (390 / 393)));
    expect(vp.fit).toBeCloseTo((776 - 48) / 868, 4);
  });

  it('a lying device is drawn in our own case, at its true size', () => {
    const vp = viewportOf(deviceOf('ipad'), AREA, null, true);
    expect(vp).toMatchObject({ rot: true, vw: 1194, vh: 834, bw: 1194 + 40, bh: 834 + 40, k: 1 });
    expect(vp.skin).toMatchObject({ k: 'own', case: 'ipadpro', framed: true });
  });

  it('a device without a case of Devices.css has our own round its true viewport', () => {
    const vp = viewportOf(deviceOf('s24'), AREA, null, false);
    expect(vp).toMatchObject({ vw: 360, vh: 780, bw: 378, bh: 798, k: 1 });
    expect(viewportVars(vp)['--pt']).toBe('9px');
  });

  it('without its case a device is the screen alone', () => {
    const vp = viewportOf(deviceOf('ip15'), AREA, null, false, { framed: false });
    expect(vp).toMatchObject({ bw: 393, bh: 852, k: 1 });
    expect(vp.skin).toMatchObject({ k: 'own', framed: false });
  });

  it('a foldable folded shows its outer screen in the case of it', () => {
    const open = viewportOf(deviceOf('fold'), AREA, null, false);
    expect(open).toMatchObject({ vw: 928, vh: 1080 });
    const shut = viewportOf(deviceOf('fold'), AREA, null, false, { folded: true });
    expect(shut).toMatchObject({ vw: 484, vh: 1188 });
    expect(shut.skin).toMatchObject({ k: 'own', case: 'foldc' });
  });

  it('keeps room beside the box for the pill', () => {
    const narrow = { w: 500, h: 2000 };
    const beside = viewportOf(deviceOf('ip15'), narrow, null, false, { padSide: 64 });
    expect(beside.fit).toBeLessThan(viewportOf(deviceOf('ip15'), narrow, null, false).fit);
  });

  it('an unknown device is «По окну»', () => {
    expect(deviceOf('nope').id).toBe('fill');
  });
});

describe('the device menu', () => {
  it('stands the devices on their shelves in order', () => {
    expect(deviceShelves().map(({ shelf, devices }) => [shelf, devices.map((d) => d.id)])).toEqual([
      [DEVICE_SHELF.view, ['fill', 'overview']],
      [DEVICE_SHELF.iphone, ['ipse', 'ip14', 'ip15', 'ipair', 'ip16']],
      [DEVICE_SHELF.samsung, ['s24', 's24u', 'flip', 'fold']],
      [DEVICE_SHELF.other, ['px9', 'duo', 'bare']],
      [DEVICE_SHELF.tablets, ['ipad', 'ipad9', 'tabs9']],
      [DEVICE_SHELF.computers, ['lap', 'fhd']],
    ]);
  });

  it('knows a device kept under its former id', () => {
    expect(deviceOf('px7').id).toBe('px9');
  });

  it('shows the size alone on the right, and none for the view', () => {
    expect(deviceSizeText(deviceOf('ip15'))).toBe('393×852');
    expect(deviceSizeText(deviceOf('fhd'))).toBe('1920×1080');
    expect(deviceSizeText(deviceOf('fill'))).toBeNull();
    expect(deviceSizeText(deviceOf('overview'))).toBeNull();
  });
});

describe('«Все экраны»', () => {
  it('is the area itself and puts the screens in a row by group at one scale', () => {
    expect(viewportOf(deviceOf('overview'), AREA, null, false)).toMatchObject({
      kind: VP_KIND.overview,
      vw: 1172,
      vh: 776,
    });
    const cards = overviewOf(AREA, false);
    expect(cards.map((c) => c.device.id)).toEqual([
      'ip14',
      'ip15',
      'ip16',
      'px9',
      's24',
      'ipad',
      'lap',
      'fhd',
    ]);
    expect(new Set(cards.map((c) => c.scale)).size).toBe(1);
    // A wider gap stands before the first tablet and the first computer.
    expect(cards.filter((c) => c.lead).map((c) => c.device.id)).toEqual(['ipad', 'lap']);
  });

  it('shows only the groups chosen, larger when there are fewer', () => {
    const phones = overviewOf(AREA, false, [DEVICE_GROUP.phones]);
    expect(phones.map((c) => c.device.group)).toEqual(Array(5).fill(DEVICE_GROUP.phones));
    expect(phones.some((c) => c.lead)).toBe(false);
    expect(phones[0].scale).toBeGreaterThan(overviewOf(AREA, false)[0].scale);
    // Phones alone fit the width of the area.
    expect(overviewBox(phones, false).w).toBeLessThanOrEqual(AREA.w + 0.5);
    expect(overviewOf(AREA, false, [])).toEqual([]);
  });

  it('keeps many screens readable and lets the row run past the area to be panned', () => {
    const all = overviewOf(AREA, false);
    expect(all[0].scale).toBeGreaterThanOrEqual(0.35 - 1e-9);
    expect(overviewBox(all, false).w).toBeGreaterThan(AREA.w);
    // The row never grows taller than the area.
    expect(overviewBox(all, false).h).toBeLessThanOrEqual(AREA.h + 0.5);
  });

  it('leaves room under the field and never draws smaller than the least scale', () => {
    const wide = { w: 6000, h: 700 };
    expect(overviewOf(wide, true)[0].scale).toBeLessThan(overviewOf(wide, false)[0].scale);
    expect(overviewOf({ w: 200, h: 100 }, false)[0].scale).toBe(MIN_FIT);
  });

  it('scrolls the other screens to the same place in whole CSS pixels, never above the top', () => {
    expect(syncedScroll(240.4)).toBe(240);
    expect(syncedScroll(-12)).toBe(0);
    expect(syncedScroll(0)).toBe(0);
  });
});

describe('the phone interface', () => {
  it('a phone and a tablet have a system; a bare size has none', () => {
    expect(deviceOf('ip15').os).toBe('ios');
    expect(deviceOf('duo').os).toBe('duo');
    expect(deviceOf('lap').os).toBeUndefined();
  });

  it("the browser is the asked one, any on any device, else the system's own", () => {
    expect(browserOf(deviceOf('ip15'), null)?.id).toBe('safari26');
    expect(browserOf(deviceOf('ip15'), 'chrome')?.id).toBe('chrome');
    expect(browserOf(deviceOf('s24'), null)?.id).toBe('chrome');
    expect(browserOf(deviceOf('ipad'), 'nope')?.id).toBe('ipados');
    expect(browserOf(deviceOf('lap'), null)).toBeNull();
    // The ids kept from before still say their browser.
    expect(browserOf(deviceOf('s24'), 'sam85')?.id).toBe('samsung');
    expect(browserOf(deviceOf('ip15'), 'iostop')?.id).toBe('safaritop');
  });

  it('has twelve browsers, each shelf its own', () => {
    expect(BROWSERS).toHaveLength(12);
    expect(new Set(BROWSERS.map((b) => b.id)).size).toBe(12);
  });

  it('the bars fold on a run down and unfold on a run up or at the top', () => {
    let track = trackScroll(SCROLL_START, 40);
    expect(track.bars).toBe(BARS.min);
    track = trackScroll(track, 44);
    expect(track.bars).toBe(BARS.min);
    // A short step back is a tremble.
    track = trackScroll(track, 40);
    expect(track.bars).toBe(BARS.min);
    track = trackScroll(track, 20);
    expect(track.bars).toBe(BARS.full);
    track = trackScroll(trackScroll(track, 300), 5);
    expect(track.bars).toBe(BARS.full);
  });
});

describe('overviewBox', () => {
  it('is the drawn row: the screens, the gaps (wider between groups), the captions and the padding', () => {
    const cards = overviewOf({ w: 1400, h: 700 }, false);
    const box = overviewBox(cards, false);
    const screens = cards.reduce((t, c) => t + c.w * c.scale, 0);
    // Eight screens: five gaps within groups, two between them, the sides.
    expect(box.w).toBeCloseTo(48 + screens + 28 * 5 + 64 * 2);
    expect(overviewBox(cards, true).h - box.h).toBe(76);
    expect(overviewBox([], true)).toEqual({ w: 0, h: 0 });
  });
});
