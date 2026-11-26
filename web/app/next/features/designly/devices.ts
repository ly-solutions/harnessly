// What the open design is looked at on: the window itself («По окну»), an exact size, or a device in its case. The
// viewport is the page's CSS size; the box is what is drawn and `fit` scales it into the area so all of it shows. A phone
// or a tablet in a case also draws its own interface (a status bar, the browser's bars) which takes room from the page.

export const PHONE_OS = { ios: 'ios', android: 'android', ipad: 'ipad', duo: 'duo' } as const;
export type TPhoneOs = (typeof PHONE_OS)[keyof typeof PHONE_OS];

/** A phone, a tablet, a computer or the view itself; phones and tablets are also the chips of «Все экраны». */
export const DEVICE_GROUP = {
  view: 'view',
  phones: 'phones',
  tablets: 'tablets',
  computers: 'computers',
} as const;
export type TDeviceGroup = (typeof DEVICE_GROUP)[keyof typeof DEVICE_GROUP];

/** The shelves of the device menu: «По окну» and «Все экраны» stand open, the rest unfold one at a time. */
export const DEVICE_SHELF = {
  view: 'view',
  iphone: 'iphone',
  samsung: 'samsung',
  other: 'other',
  tablets: 'tablets',
  computers: 'computers',
} as const;
export type TDeviceShelf = (typeof DEVICE_SHELF)[keyof typeof DEVICE_SHELF];

/**
 * A drawn case of our own (design-devices.css): its edges round the screen, the corners of the body and of the screen.
 * The page is shown in it at the device's true size. The colours and the cuts are the CSS's, by the case's name.
 */
export interface ICase {
  top: number;
  side: number;
  bottom: number;
  radius: number;
  screen: number;
}

export const CASES = {
  se: { top: 74, side: 14, bottom: 74, radius: 46, screen: 0 },
  notch: { top: 12, side: 12, bottom: 12, radius: 56, screen: 46 },
  island: { top: 11, side: 11, bottom: 11, radius: 60, screen: 50 },
  air: { top: 9, side: 9, bottom: 9, radius: 58, screen: 50 },
  punch: { top: 9, side: 9, bottom: 9, radius: 40, screen: 33 },
  ultra: { top: 7, side: 7, bottom: 7, radius: 16, screen: 10 },
  flip: { top: 9, side: 9, bottom: 9, radius: 34, screen: 28 },
  flipc: { top: 12, side: 12, bottom: 12, radius: 44, screen: 34 },
  foldc: { top: 9, side: 9, bottom: 9, radius: 26, screen: 20 },
  foldo: { top: 12, side: 12, bottom: 12, radius: 26, screen: 16 },
  pixel: { top: 10, side: 10, bottom: 10, radius: 48, screen: 40 },
  duo: { top: 14, side: 14, bottom: 14, radius: 24, screen: 8 },
  bare: { top: 0, side: 0, bottom: 0, radius: 14, screen: 14 },
  ipadpro: { top: 20, side: 20, bottom: 20, radius: 38, screen: 22 },
  ipadhome: { top: 72, side: 24, bottom: 72, radius: 36, screen: 0 },
  tab: { top: 16, side: 16, bottom: 16, radius: 30, screen: 16 },
} as const satisfies Record<string, ICase>;
export type TCase = keyof typeof CASES;
/** The screen's corners with the case taken off. */
const NO_CASE_RADIUS = 14;

/** The little silhouette of a device in the menu: the shape and the proportions of the real screen. */
export type TGlyph =
  | 'fill'
  | 'all'
  | 'notch'
  | 'island'
  | 'punch'
  | 'home'
  | 'flip'
  | 'fold'
  | 'duo'
  | 'bare'
  | 'tablet'
  | 'tabhome'
  | 'laptop'
  | 'monitor';

/** A case of Devices.css (vendor-devices.css): its classes, the size of the case and of its screen. */
export interface IDevicesCss {
  cls: string;
  color: string;
  fw: number;
  fh: number;
  sw: number;
  sh: number;
}

export interface IDevice {
  id: string;
  icon: string;
  group: TDeviceGroup;
  shelf: TDeviceShelf;
  glyph: TGlyph;
  vw?: number;
  vh?: number;
  /** The drawn case; none for a computer or the view. */
  case?: TCase;
  /** The case of Devices.css drawn instead of ours while the device stands upright and unfolded. */
  css?: IDevicesCss;
  /** The system it runs: its status bar and the browser it starts with. */
  os?: TPhoneOs;
  /** A foldable folded: its outer screen and that screen's case; `hinge` is the way it folds. */
  fold?: { vw: number; vh: number; case: TCase; hinge: 'v' | 'h' };
}

const cssCase = (
  cls: string,
  color: string,
  fw: number,
  fh: number,
  sw: number,
  sh: number,
): IDevicesCss => ({
  cls: `device-${cls}`,
  color: `device-${color}`,
  fw,
  fh,
  sw,
  sh,
});
const IPHONE_14 = cssCase('iphone-14', 'starlight', 428, 868, 390, 830);
const iphonePro = (color: string) => cssCase('iphone-14-pro', color, 428, 868, 390, 830);

const phone = (
  shelf: TDeviceShelf,
  id: string,
  vw: number,
  vh: number,
  os: TPhoneOs,
  kind: TCase,
  glyph: TGlyph,
  extra: Partial<IDevice> = {},
): IDevice => ({
  id,
  icon: 'smartphone',
  group: DEVICE_GROUP.phones,
  shelf,
  vw,
  vh,
  os,
  case: kind,
  glyph,
  ...extra,
});
const tablet = (
  shelf: TDeviceShelf,
  id: string,
  vw: number,
  vh: number,
  os: TPhoneOs,
  kind: TCase,
  glyph: TGlyph,
  extra: Partial<IDevice> = {},
): IDevice => ({
  ...phone(shelf, id, vw, vh, os, kind, glyph, extra),
  icon: 'tablet',
  group: DEVICE_GROUP.tablets,
});
const computer = (id: string, vw: number, vh: number, glyph: 'laptop' | 'monitor'): IDevice => ({
  id,
  icon: glyph,
  group: DEVICE_GROUP.computers,
  shelf: DEVICE_SHELF.computers,
  vw,
  vh,
  glyph,
});

// The menu's order: the view, then the phones shelf by shelf, the tablets and the computers. The sizes are the CSS
// viewports of Chromium's device list (the S24 Ultra and the Surface Duo, absent there, are the makers').
export const DEVICES: readonly IDevice[] = [
  { id: 'fill', icon: 'monitor', group: DEVICE_GROUP.view, shelf: DEVICE_SHELF.view, glyph: 'fill' },
  { id: 'overview', icon: 'layout-grid', group: DEVICE_GROUP.view, shelf: DEVICE_SHELF.view, glyph: 'all' },
  phone(DEVICE_SHELF.iphone, 'ipse', 375, 667, PHONE_OS.ios, 'se', 'home', {
    css: cssCase('iphone-8', 'spacegray', 428, 868, 375, 667),
  }),
  phone(DEVICE_SHELF.iphone, 'ip14', 390, 844, PHONE_OS.ios, 'notch', 'notch', { css: IPHONE_14 }),
  phone(DEVICE_SHELF.iphone, 'ip15', 393, 852, PHONE_OS.ios, 'island', 'island', {
    css: iphonePro('silver'),
  }),
  phone(DEVICE_SHELF.iphone, 'ipair', 420, 912, PHONE_OS.ios, 'air', 'island', { css: iphonePro('gold') }),
  phone(DEVICE_SHELF.iphone, 'ip16', 440, 956, PHONE_OS.ios, 'island', 'island', { css: iphonePro('black') }),
  phone(DEVICE_SHELF.samsung, 's24', 360, 780, PHONE_OS.android, 'punch', 'punch'),
  phone(DEVICE_SHELF.samsung, 's24u', 384, 824, PHONE_OS.android, 'ultra', 'punch'),
  phone(DEVICE_SHELF.samsung, 'flip', 360, 880, PHONE_OS.android, 'flip', 'flip', {
    fold: { vw: 360, vh: 374, case: 'flipc', hinge: 'h' },
  }),
  tablet(DEVICE_SHELF.samsung, 'fold', 928, 1080, PHONE_OS.android, 'foldo', 'fold', {
    group: DEVICE_GROUP.phones,
    fold: { vw: 484, vh: 1188, case: 'foldc', hinge: 'v' },
  }),
  phone(DEVICE_SHELF.other, 'px9', 360, 808, PHONE_OS.android, 'pixel', 'punch', {
    css: cssCase('google-pixel-6-pro', 'black', 404, 862, 376, 816),
  }),
  tablet(DEVICE_SHELF.other, 'duo', 1114, 720, PHONE_OS.duo, 'duo', 'duo', { group: DEVICE_GROUP.phones }),
  phone(DEVICE_SHELF.other, 'bare', 393, 852, PHONE_OS.ios, 'bare', 'bare'),
  tablet(DEVICE_SHELF.tablets, 'ipad', 834, 1194, PHONE_OS.ipad, 'ipadpro', 'tablet', {
    css: cssCase('ipad-pro', 'spacegray', 560, 778, 506, 724),
  }),
  tablet(DEVICE_SHELF.tablets, 'ipad9', 810, 1080, PHONE_OS.ipad, 'ipadhome', 'tabhome', {
    css: cssCase('ipad-pro-2017', 'spacegray', 560, 800, 510, 680),
  }),
  tablet(DEVICE_SHELF.tablets, 'tabs9', 640, 1024, PHONE_OS.android, 'tab', 'tablet'),
  computer('lap', 1280, 800, 'laptop'),
  computer('fhd', 1920, 1080, 'monitor'),
];

/** The ids kept by a browser or a guest link from before the devices were renamed. */
const DEVICE_ALIAS: Readonly<Record<string, string>> = { px7: 'px9' };
export const deviceIdOf = (id: string): string => DEVICE_ALIAS[id] ?? id;

/** A phone or a tablet turns (lying: its width and height swapped); «По окну» and the computers do not. */
export const canRotate = (device: IDevice): boolean =>
  device.group === DEVICE_GROUP.phones || device.group === DEVICE_GROUP.tablets;

export const deviceOf = (id: string): IDevice =>
  DEVICES.find((d) => d.id === deviceIdOf(id)) ?? DEVICES.find((d) => d.id === 'fill') ?? DEVICES[0];

/** The menu's shelves in their order, each with its devices in the order of DEVICES. */
export const deviceShelves = (): { shelf: TDeviceShelf; devices: IDevice[] }[] =>
  Object.values(DEVICE_SHELF).map((shelf) => ({ shelf, devices: DEVICES.filter((d) => d.shelf === shelf) }));

/** The size a menu row shows on its right («393×852»); null for the view's rows, which have none of their own. */
export const deviceSizeText = (device: IDevice): string | null =>
  device.vw && device.vh ? `${device.vw}×${device.vh}` : null;

export interface ISize {
  w: number;
  h: number;
}

/** The sizes «По окну» takes: whatever was typed, dragged, turned or kept stays within them. */
export const VIEWPORT_SIZE = { min: 200, max: 3840 } as const;
/** The least scale a box is drawn at: an area measured empty (a hidden or squeezed pane) must not take it to 0%. */
export const MIN_FIT = 0.05;

/** The size within the limits, or null when a side is below the least (a box measured in a squeezed area). */
export const keptSize = (size: ISize | null): ISize | null =>
  size === null || !(size.w >= VIEWPORT_SIZE.min && size.h >= VIEWPORT_SIZE.min)
    ? null
    : {
        w: Math.min(VIEWPORT_SIZE.max, Math.round(size.w)),
        h: Math.min(VIEWPORT_SIZE.max, Math.round(size.h)),
      };

export const VP_KIND = { fill: 'fill', size: 'size', frame: 'frame', overview: 'overview' } as const;
export type TVpKind = (typeof VP_KIND)[keyof typeof VP_KIND];

export interface IViewport {
  kind: TVpKind;
  rot: boolean;
  /** The page's CSS viewport. */
  vw: number;
  vh: number;
  /** The CSS height of the screen in a case (the page is scaled by k into it). */
  cvh: number;
  /** The drawn box before fit. */
  bw: number;
  bh: number;
  fit: number;
  k: number;
  /** How a phone or a tablet is cased: a case of Devices.css, ours, or none (the screen alone); null for the rest. */
  skin: TSkin | null;
}

export type TSkin = { k: 'css'; css: IDevicesCss } | { k: 'own'; case: TCase; edges: ICase; framed: boolean };

// The field (island, notes) floats over the page: a device keeps the whole area, its bottom as narrow as the sides.
const PAD = 48;
/** «По окну» is the design set into the window edge to edge, with no canvas round it; the area's padding in
 *  site.css is the same. */
export const FILL_INSET = 0;

export interface IViewportOpts {
  /** Room kept under the box (the pane's pill). */
  padBottom?: number;
  /** Room kept beside the box (the phone's pill). */
  padSide?: number;
  /** The device's case is drawn; off, the screen alone. */
  framed?: boolean;
  /** A foldable folded: its outer screen. */
  folded?: boolean;
}

/** The viewport for a device in an area; `size` is the person's own size of «По окну», `rot` turns a device. */
export const viewportOf = (
  device: IDevice,
  area: ISize,
  size: ISize | null,
  rot: boolean,
  { padBottom = PAD, padSide = 0, framed = true, folded = false }: IViewportOpts = {},
): IViewport => {
  const plain = { rot: false, fit: 1, k: 1, skin: null };
  if (device.id === 'overview' || (device.id === 'fill' && !size)) {
    const w = Math.max(0, area.w - 2 * FILL_INSET);
    const h = Math.max(0, area.h - 2 * FILL_INSET);
    const kind = device.id === 'overview' ? VP_KIND.overview : VP_KIND.fill;
    return { ...plain, kind, vw: w, vh: h, cvh: h, bw: w, bh: h };
  }
  const custom = device.id === 'fill';
  const turned = !custom && rot;
  const outer = folded ? device.fold : undefined;
  const base: ISize =
    custom && size ? size : { w: outer?.vw ?? device.vw ?? area.w, h: outer?.vh ?? device.vh ?? area.h };
  const vw = turned ? base.h : base.w;
  const vh = turned ? base.w : base.h;
  const fitOf = (w: number, h: number) =>
    Math.max(MIN_FIT, Math.min(1, (area.w - PAD - padSide) / w, (area.h - padBottom) / h));
  const kind = outer?.case ?? device.case;
  if (custom || !kind) {
    return { ...plain, kind: VP_KIND.size, vw, vh, cvh: vh, bw: vw, bh: vh, fit: fitOf(vw, vh) };
  }
  // A case of Devices.css stands upright and unfolded: its screen is fixed, the page of width vw is scaled into it.
  if (framed && !turned && !outer && device.css) {
    const { css } = device;
    const k = css.sw / vw;
    const cvh = Math.round(css.sh / k);
    const skin: TSkin = { k: 'css', css };
    return {
      rot: false,
      kind: VP_KIND.frame,
      vw,
      vh,
      cvh,
      bw: css.fw,
      bh: css.fh,
      fit: fitOf(css.fw, css.fh),
      k,
      skin,
    };
  }
  const cased: ICase = framed
    ? CASES[kind]
    : { top: 0, side: 0, bottom: 0, radius: NO_CASE_RADIUS, screen: NO_CASE_RADIUS };
  // Lying, the case lies with the device: its top is on the left.
  const edges: ICase = turned
    ? { top: cased.side, side: cased.top, bottom: cased.side, radius: cased.radius, screen: cased.screen }
    : cased;
  const bw = turned ? vw + cased.top + cased.bottom : vw + 2 * cased.side;
  const bh = turned ? vh + 2 * cased.side : vh + cased.top + cased.bottom;
  const skin: TSkin = { k: 'own', case: kind, edges, framed: framed && kind !== 'bare' };
  return { rot: turned, kind: VP_KIND.frame, vw, vh, cvh: vh, bw, bh, fit: fitOf(bw, bh), k: 1, skin };
};

/** The custom properties the viewport's CSS reads (--bw, --vw, --k, --fit and the case's sizes). */
export const viewportVars = (vp: IViewport): Record<string, string> => {
  const vars: Record<string, string> = {
    '--bw': `${vp.bw}px`,
    '--bh': `${vp.bh}px`,
    '--vw': `${vp.vw}px`,
    '--vh': `${vp.cvh}px`,
    '--k': vp.k.toFixed(4),
    '--fit': vp.fit.toFixed(4),
  };
  if (vp.skin?.k === 'css') {
    const { css } = vp.skin;
    vars['--dw'] = `${css.fw}px`;
    vars['--dh'] = `${css.fh}px`;
    vars['--sw'] = `${css.sw}px`;
    vars['--sh'] = `${css.sh}px`;
  }
  if (vp.skin?.k === 'own') {
    const { edges } = vp.skin;
    vars['--pt'] = `${edges.top}px`;
    vars['--ps'] = `${edges.side}px`;
    // Lying, the far side is the case's bottom.
    vars['--pr'] = `${vp.rot ? CASES[vp.skin.case].bottom : edges.side}px`;
    vars['--pb'] = `${edges.bottom}px`;
    vars['--rb'] = `${edges.radius}px`;
    vars['--rs'] = `${edges.screen}px`;
  }
  return vars;
};

/** The devices «Все экраны» shows, by group (the bare phone is the size of iPhone 15 Pro, so it is left out). */
export const OVERVIEW_IDS = ['ip14', 'ip15', 'ip16', 'px9', 's24', 'ipad', 'lap', 'fhd'] as const;
/** The groups «Все экраны» can show: its chips. */
export const OVERVIEW_GROUPS = [DEVICE_GROUP.phones, DEVICE_GROUP.tablets, DEVICE_GROUP.computers] as const;
export type TOverviewGroup = (typeof OVERVIEW_GROUPS)[number];
/** The gap between two screens of a group, and the wider one between two groups (site.css .hx-ovw). */
const OVERVIEW_GAP = 28;
const OVERVIEW_GROUP_GAP = 64;
/** The row's padding above the screens (the chips stand there) and at the sides, a caption with its gap, the room
 *  kept under the screens for the field. */
const OVERVIEW_TOP = 60;
const OVERVIEW_SIDE = 24;
const OVERVIEW_CAPTION = 32;
const OVERVIEW_BOTTOM = 20;
const OVERVIEW_FIELD = 96;
/** The least scale the row starts at however many screens it has: smaller, a page is unreadable, and the canvas pans. */
const OVERVIEW_LEAST = 0.35;

export interface IOverviewCard {
  device: IDevice;
  w: number;
  h: number;
  /** Shared by all the cards: the devices keep their true proportions. */
  scale: number;
  /** The first card of a group after another: the wider gap is before it. */
  lead: boolean;
}

/**
 * The cards of «Все экраны» for an area and the groups shown, at one scale. The whole row fits when it can; with many
 * screens it stays at a readable scale, as tall as the area lets it, and runs past the area's right edge to be panned to.
 */
export const overviewOf = (
  area: ISize,
  underField: boolean,
  groups: readonly TOverviewGroup[] = OVERVIEW_GROUPS,
): IOverviewCard[] => {
  const devices = OVERVIEW_IDS.map(deviceOf).filter((d) => groups.some((g) => g === d.group));
  if (devices.length === 0) return [];
  const leads = devices.filter((d, i) => i > 0 && devices[i - 1].group !== d.group).length;
  const room = area.h - OVERVIEW_TOP - OVERVIEW_CAPTION - (underField ? OVERVIEW_FIELD : OVERVIEW_BOTTOM);
  const across =
    area.w - 2 * OVERVIEW_SIDE - OVERVIEW_GAP * (devices.length - 1 - leads) - OVERVIEW_GROUP_GAP * leads;
  const sum = devices.reduce((total, d) => total + (d.vw ?? 0), 0);
  const tallest = Math.max(...devices.map((d) => d.vh ?? 0));
  const scale = Math.max(MIN_FIT, Math.min(1, room / tallest, Math.max(across / sum, OVERVIEW_LEAST)));
  return devices.map((device, i) => ({
    device,
    w: device.vw ?? 0,
    h: device.vh ?? 0,
    scale,
    lead: i > 0 && devices[i - 1].group !== device.group,
  }));
};

const gapBefore = (card: IOverviewCard): number => (card.lead ? OVERVIEW_GROUP_GAP : OVERVIEW_GAP);

/** The row of «Все экраны» as drawn: the padding, a caption over each screen, the gaps, and room for the field. */
export const overviewBox = (cards: readonly IOverviewCard[], underField: boolean): ISize => {
  if (cards.length === 0) return { w: 0, h: 0 };
  const across = cards.reduce((total, c, i) => total + c.w * c.scale + (i === 0 ? 0 : gapBefore(c)), 0);
  const tallest = Math.max(...cards.map((c) => c.h * c.scale));
  return {
    w: 2 * OVERVIEW_SIDE + across,
    h: OVERVIEW_TOP + OVERVIEW_CAPTION + tallest + (underField ? OVERVIEW_FIELD : OVERVIEW_BOTTOM),
  };
};

/** Where the other screens go when one is scrolled to `y`: the same CSS pixels from the top (each page stops at its own
 *  end), never above it, in whole pixels so a page told the same place twice does not move. */
export const syncedScroll = (y: number): number => Math.max(0, Math.round(y));

/**
 * A browser a phone or a tablet is shown in (site-device.tsx draws it): its bars over and under the page. Any of them
 * may be put on any device; each system starts with its own.
 */
export interface IBrowser {
  id: string;
  shelf: TBrowserShelf;
  /** A bar over the page: the browser's address, or the head of a mini app. */
  top?: 'addr' | 'tg';
  /** The address is centred, with «аА» on its left (Safari). */
  centred?: boolean;
  aa?: boolean;
  /** Icons before and after the address of the top bar; `tabs` is the counter of open tabs. */
  lead?: readonly string[];
  trail?: readonly string[];
  /** A bar under the page: an address, a row of icons, or both. */
  addrBelow?: boolean;
  icons?: readonly string[];
  /** Round buttons and the address under glass over the page (Safari of iOS 26). */
  float?: boolean;
}

export const BROWSER_SHELF = {
  iphone: 'iphone',
  android: 'android',
  tablet: 'tablet',
  inapp: 'inapp',
} as const;
export type TBrowserShelf = (typeof BROWSER_SHELF)[keyof typeof BROWSER_SHELF];

const SAFARI_TOOLS = ['chevron-left', 'chevron-right', 'share', 'book-open', 'copy'] as const;
/** No browser at all: a page opened from the home screen. */
export const BROWSER_NONE = 'home';

export const BROWSERS: readonly IBrowser[] = [
  { id: 'safari26', shelf: 'iphone', float: true },
  { id: 'safari18', shelf: 'iphone', addrBelow: true, icons: SAFARI_TOOLS },
  { id: 'safaritop', shelf: 'iphone', top: 'addr', aa: true, centred: true, icons: SAFARI_TOOLS },
  {
    id: 'chromeios',
    shelf: 'iphone',
    top: 'addr',
    icons: ['chevron-left', 'chevron-right', 'plus', 'copy', 'ellipsis'],
  },
  { id: 'chrome', shelf: 'android', top: 'addr', lead: ['house'], trail: ['tabs', 'ellipsis'] },
  {
    id: 'samsung',
    shelf: 'android',
    top: 'addr',
    icons: ['chevron-left', 'chevron-right', 'house', 'bookmark-plus', 'copy', 'menu'],
  },
  { id: 'yandex', shelf: 'android', addrBelow: true, icons: ['chevron-left', 'house', 'copy', 'menu'] },
  { id: 'firefox', shelf: 'android', addrBelow: true, icons: ['house', 'copy', 'ellipsis'] },
  {
    id: 'edge',
    shelf: 'android',
    top: 'addr',
    icons: ['chevron-left', 'chevron-right', 'plus', 'copy', 'menu'],
  },
  {
    id: 'ipados',
    shelf: 'tablet',
    top: 'addr',
    aa: true,
    centred: true,
    lead: ['panel-left', 'chevron-left', 'chevron-right'],
    trail: ['share', 'plus', 'copy'],
  },
  { id: 'telegram', shelf: 'inapp', top: 'tg' },
  { id: BROWSER_NONE, shelf: 'inapp' },
];

/** The browser a system starts with. */
export const NATIVE_BROWSER: Record<TPhoneOs, string> = {
  ios: 'safari26',
  android: 'chrome',
  ipad: 'ipados',
  duo: 'edge',
};

/** The ids kept from before: Safari and Samsung Internet had other names. */
const BROWSER_ALIAS: Readonly<Record<string, string>> = {
  ios26: 'safari26',
  ios18: 'safari18',
  iostop: 'safaritop',
  sam85: 'samsung',
  ipad: 'ipados',
  pwa: BROWSER_NONE,
};

/** The browser a device shows: the one asked for, else its system's own; null where it has no system. */
export const browserOf = (device: IDevice, wanted: string | null): IBrowser | null => {
  if (!device.os) return null;
  const asked = wanted === null ? null : (BROWSER_ALIAS[wanted] ?? wanted);
  const own = NATIVE_BROWSER[device.os];
  return BROWSERS.find((b) => b.id === asked) ?? BROWSERS.find((b) => b.id === own) ?? null;
};

export const BARS = { full: 'full', min: 'min' } as const;
export type TBars = (typeof BARS)[keyof typeof BARS];

/** Where the page was scrolled to, how far it has gone in one direction, and so what the bars do. */
export interface IScrollTrack {
  y: number;
  acc: number;
  bars: TBars;
}

export const SCROLL_START: IScrollTrack = { y: 0, acc: 0, bars: BARS.full };
/** A run of this many pixels one way folds or unfolds the bars: a trembling finger does not move them. */
const SCROLL_STEP = 12;

export const trackScroll = (prev: IScrollTrack, y: number): IScrollTrack => {
  const moved = y - prev.y;
  const acc = Math.sign(moved) === Math.sign(prev.acc) ? prev.acc + moved : moved;
  let bars = prev.bars;
  if (y < SCROLL_STEP || acc < -SCROLL_STEP) bars = BARS.full;
  else if (acc > SCROLL_STEP) bars = BARS.min;
  return { y, acc, bars };
};
