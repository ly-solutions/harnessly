// An open design as the design draws it (.hx-site): a browser with the design's pages in tabs, its bar — history,
// the address with the pages, the device and size, more — and the page itself in the chosen viewport, in a case when
// the device has one. The page is the board on the machine's library server, opened in a preview slot of its own.
import BoardFrames, { PROTOTYPE_FRAME_CAPACITY } from './board-frames';
import { useLiveHint, useLiveMark } from '@/next/features/voice-live/live-mark';
import { tourSignal } from '@/next/stores/tour';
import { ChatTerminalSlotContext } from '@/next/features/terminal/chat-terminal';
import {
  pageDownloads,
  projectDownloads,
  ProjectDownloadMenu,
  type IDownloadItem,
  type IProjectDownloadsOf,
} from '@/next/features/designly/project-downloads';
import { KEY_SCOPE } from '@/next/features/designly/sdk/keys';
import { useKeyHint } from '@/next/features/designly/sdk/keys';
import { useKeys } from '@/next/features/designly/sdk/keys';
import {
  Fragment,
  useCallback,
  useContext,
  useEffect,
  useEffectEvent,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ComponentType,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
  type RefObject,
} from 'react';
import {
  BARS,
  BROWSER_NONE,
  browserOf,
  canRotate,
  DEVICE_GROUP,
  deviceOf,
  NATIVE_BROWSER,
  overviewBox,
  overviewOf,
  SCROLL_START,
  trackScroll,
  type ISize,
  type TBars,
  viewportOf,
  viewportVars,
  VP_KIND,
} from '@/next/features/designly/devices';
import {
  CaseParts,
  PhoneBottom,
  PhonePopover,
  PhoneTop,
  ScreenParts,
} from '@/next/features/designly/site-device';
import { Overview, useOverviewGroups } from '@/next/features/designly/site-overview';
import { qrModules } from '@/next/features/designly/qr';
import { PaneChatModes, type TSiteMode } from '@/next/features/designly/site-chat';
import { PANE_CHAT, type TPaneChat } from '@/next/features/designly/pane-chat';
import {
  DEV_H,
  dragDevtools,
  dragTweaks,
  dragViewport,
  type IFloat,
  type IPoint,
  type TDock,
} from '@/next/features/designly/site-drag';
import {
  GUEST_KEY_CODE,
  guestKeyValue,
  readBridgeMessage,
  type IPicked,
  type ITweak,
  type TTweakValue,
} from '@/next/features/designly/page-bridge';
import { DEV_TAB, type TDevTab } from '@/next/features/designly/devtools-data';
import {
  boardChat,
  boardErrorAck,
  boardErrorSink,
  personIsActing,
  readBoardError,
  type IBoardErrorSink,
} from '@/next/features/designly/board-error';
import SiteDevtools from '@/next/features/designly/site-devtools';
import type { IStyleItem } from '@/next/features/designly/element-style';
import { GUEST_ORIGIN, frameOrigin, fromFrame } from '@/next/features/designly/bridge-origin';
import { AnchorContext, createAnchorStore, type IAnchorItem } from '@/next/features/designly/note-anchor';
import { useDomBridge } from '@/next/features/designly/use-dom-bridge';
import TourAnchor, { TOUR_ANCHOR } from '@/next/features/tour/tour-anchor';
import { useTourStep } from '@/next/features/tour/use-tour-step';
import SiteLoader, { SiteHeld } from '@/next/features/designly/site-loader';
import { useFrameRetry } from '@/next/features/designly/frame-retry';
import { loadStageOf, type ILoadProgress } from '@/next/features/designly/site-load';
import { useSiteLoad } from '@/next/features/designly/use-site-load';
import SiteTweaks from '@/next/features/designly/site-tweaks';
import { PaneDock } from '@/next/features/designly/site-dock';
import DeviceMenu from '@/next/features/designly/device-menu';
import PagePicker, { PICKER_PLACE } from '@/next/features/designly/page-picker';
import { ZOOM_PILL, ZoomPill } from '@/next/features/designly/zoom-pill';
import { showPill, usePillPrefs } from '@/next/features/designly/floating-pill';
import { PHONE_PILL, PhonePill, usePhonePillHidden } from '@/next/features/designly/phone-pill';
import { setPhoneView, usePhoneView } from '@/next/features/designly/phone-view';
import { useViewMemory } from '@/next/features/designly/use-view-memory';
import type { IViewMemory } from '@/next/features/designly/view-memory';
import { useCanvasZoom } from '@/next/features/designly/use-canvas-zoom';
import { standardFit } from '@/next/features/designly/site-zoom';
import type { ConvId, MachineId } from '@/next/protocol/v2';
import ImageActions, { type IImageOpen } from '@/next/features/designly/image-gen/image-actions';
import { fileWriter } from '@/next/features/designly/image-gen/canvas';
import { frameScale, screenBoxOf } from '@/next/features/designly/image-gen/image-act';
import Icon from '@/next/ui/icon';
import { SheetGrip, SheetScrim, useSheetDrag } from '@/next/ui/sheet';
import Swap from '@/next/ui/swap';
import LoadFailed, { LOAD_FAILED_SIZE } from '@/next/ui/load-failed';
import { usePhone } from '@/next/ui/use-media';
import { usePresence, usePresentFlag } from '@/next/ui/presence';
import { t } from '@/next/i18n';
import { toast } from '@/next/features/designly/sdk/toast';
import {
  FRAME_VIEW,
  frameView,
  goTake,
  hasTake,
  pageOfLink,
  readPageGo,
  type IPageGo,
  type IPendingTake,
} from '@/next/features/designly/page-link';
import ModeButton from '@/next/features/designly/mode-button';
import CanvasMenu, { type ICanvasMenuItem } from '@/next/features/designly/canvas-menu';
import TextBar from '@/next/features/designly/text-bar';
import {
  BoardTextContext,
  HAND_REFUSAL,
  readHandMessage,
  saveHandEdit,
  type IBoardMenu,
  type ITextSave,
  type ITextState,
} from '@/next/features/designly/hand-edit';
import { DESIGN_TAB, type IDesignComments, type IDesignPanel } from '@/next/features/designly/design-panel';
import '@/next/styles/designly.css';
import '@/next/styles/vendor-devices.css';
import '@/next/styles/site.css';
import '@/next/styles/site-tools.css';
import '@/next/styles/site-pane.css';
import '@/next/styles/site-bar.css';
import '@/next/styles/design-devices.css';

/** A board that loaded but did not say it is drawn (a failed script) leaves the loader after this long, ms. */
const DRAW_WAIT_MS = 8000;
/** The phone a link to a phone's page (a narrow `$preview`) shows it in. */
const LINK_PHONE = 'ip14';
const POP_MS = 200;
/** Folding: the half closes over the screen, then the outer screen wakes; unfolding dims it first (design-devices.css). */
const FOLD = { close: 360, wake: 620, dim: 280, open: 760 } as const;
type TFolding = keyof typeof FOLD | null;
/** The room the phone's pill takes beside a phone: its width and the gap. */
const PILL_ROOM = 64;
const NO_BOARDS: ReadonlySet<string> = new Set();
/** The pill under the page in the pane: a device keeps this much free under it (the pill, its gap and its margin). */
const DOCK_ROOM = 64;
const VP_PAD = 48;
const TWEAKS_OUT_MS = 200;
/** The island stays narrow this long after the page's last scroll message. */
const SCROLL_MS = 700;

export interface ISitePage {
  /** The board's path in the project, the page's key. */
  path: string;
  name: string;
}

export interface ISiteTab {
  id: string;
  /** The page on screen in this tab and its history. */
  hist: readonly string[];
  hi: number;
}

/** Where the chat stands for a design in the right pane, and its switch (in «Ещё»). */
export interface ISitePaneChat {
  mode: TPaneChat;
  onMode: (mode: TPaneChat, section: { width: number; height: number }) => void;
  /** The stage's mode changed (a drag put the chat where the pane has no room for it): the pane puts it back. */
  sync: (section: { width: number; height: number }) => void;
  /** The pane opened: the kept choice is applied once the section has its size. */
  start: (section: { width: number; height: number }) => void;
}

/** A design shown in the right pane: the bar has a share, there is no strip of tabs; the chat is the left one, or
 *  also over the design or at its foot (`chat`). */
export interface ISitePane {
  onShare: () => void;
  chat?: ISitePaneChat | null;
}

/** The project's access when a gateway keeps it: the bar's share button opens the dialog instead of copying the link. */
export interface ISiteAccess {
  /** «Поделиться» for a person who manages, «Запросить правку» for one who may only look. */
  label: string;
  onOpen: () => void;
  /** A deck or a video opens its older share popover instead of the dialog: only where the gateway has no one dialog
   *  (stage «д»). */
  manage?: boolean;
}

/**
 * A visitor of a guest link: the design is looked at the way its owner looks at it (the same bar, devices, sizes, zoom
 * and board settings), without editing, chats, sharing or tools. The board is served with a sandbox, so its window has
 * no origin of its own to speak to: it is addressed as `*` and trusted for being the frame's window.
 */
export interface ISiteGuest {
  /** What the bar's pick button is called and its tip («Комментировать»): the block picked becomes a comment. */
  pickLabel: string;
  pickTip: string;
  /** «Your name: Ann» in «Ещё»: the guest changes the name they comment under; null until there is one. */
  account?: { label: string; onOpen: () => void } | null;
  /** «Во весь экран» on the bar: the page alone over the whole window, without the bar and the device. */
  full?: { label: string; tip: string; onOpen: () => void };
}

/** The switch between looking at the design and editing it, in the bar (the one place it stands on both sizes). */
export interface ISiteMode {
  /** What the screen is in, said alone where there is no switch: «Редактирование макета» or «Макет». */
  state: string;
  editing: boolean;
  /** The way to the other mode (the switch's other part); null: there is none, the mode is a label. ask: editing is
      not allowed, its part asks for access. */
  action: { onClick: () => void; ask: boolean } | null;
  /** «Версии» of the project beside the switch; absent where the project has no history to show. */
  versions?: { open: boolean; onToggle: () => void };
}

export interface ISiteViewProps {
  title: string;
  pages: readonly ISitePage[];
  tabs: readonly ISiteTab[];
  tab: string;
  /** The board's address for the frame; null while the slot opens. */
  src: string | null;
  /** The board's last saved revision, shown under a thin «Открываем правку…» while the editing frame opens. */
  frameScope?: string;
  nextFrame?: string | null;
  heldSrc?: string | null;
  /** The same board at an address the slot lets in without a ticket (its cookie is set); the frames of «Обзор» use it. */
  plainSrc?: string | null;
  /** The address of this page for another device (a code to point a phone at); null while there is none to give. */
  phoneUrl?: string | null;
  /** The slot did not open: why. */
  error: string | null;
  /** Bumped by a reload: the frame loads again. */
  reloadKey: number;
  onNavigate: (path: string) => void;
  onHistory: (step: -1 | 1) => void;
  onReload: (hard: boolean) => void;
  /** Opens the current board in its own browser tab; absent while there is no board URL to open. */
  onOpenOutside?: () => void;
  onCopyLink: () => void;
  onRetry: () => void;
  onOpenDrawer: () => void;
  /** The chat beside the site and the field over it; without it the design is only looked at. */
  stage?: ISiteStage;
  /** The design is looked at while its chat is kept: the chat slides away and comes back with editing. */
  away?: boolean;
  /** The tools panel beside the design («Инструменты», «Макет»): the bar's settings, versions and panel buttons. */
  designPanel?: IDesignPanel;
  /** «Комментарий для команды» of the viewing mode; absent where people cannot comment. */
  comments?: IDesignComments;
  /** The boards the agent wrote in the design's chat: the switcher marks them. */
  chatBoards?: ReadonlySet<string>;
  /** «Новая доска» of the switcher: asks the agent for one. */
  onNewBoard?: () => void;
  /**
   * A board's error card sends its error to the design's chat: this takes it (and, looking at the design, switches to
   * editing, opens the chat and sends). Absent: the card offers to copy only.
   */
  onBoardError?: (words: string) => void;
  /** Set when the design is shown in the right pane. */
  pane?: ISitePane;
  /** The right pane of the design's chat (browser, files, terminal), as beside a chat: its switch on the bar. */
  toolPane?: { open: boolean; onToggle: () => void } | null;
  /** The design whose view (device, canvas, page) is kept in the browser; without it nothing is. */
  memoryKey?: string;
  /** The view a design starts with where the browser has kept none (a guest link carries the owner's). */
  viewSeed?: IViewMemory;
  /** Set for a visitor of a guest link. */
  guest?: ISiteGuest;
  /** The mode at the head of the bar («Макет» ↔ «Редактирование макета», a click switches); absent in a pane. */
  mode?: ISiteMode;
  /** Set where the person may change the design: the pictures of its boards get «Изменить» / «Сгенерировать» on them. */
  images?: { machine: MachineId; artifact: string };
  /** The page alone over the view's whole box: no bar, no device, no zoom (a guest's full screen). */
  full?: boolean;
  /**
   * The board's settings drawn by the design's panel («Макет → Настройки»): the page's offer goes to onOffer
   * (null while the frame has none) and `send` is set to the way a change goes to the page. With it the floating
   * «Живые настройки» are not drawn.
   */
  tweaksPort?: ITweaksPort;
  /** A layer over the page (the people's comments): it is given the frame of the page on screen. */
  commentsLayer?: ComponentType<ICommentsLayerSlot>;
  /** Set where the gateway keeps the project's access and the person has something to do there. */
  access?: ISiteAccess;
  /** What the project is (an app's document type): its chip on the bar, before the share button. */
  kit?: ReactNode;
  /** «Скачать» (project-downloads.tsx) in «Ещё» and on a page's row: the project of the pages. */
  downloads?: Omit<IProjectDownloadsOf, 'board'>;
  /** The page's preview slot, whose record DevTools read; without it the panel is not offered. */
  devtools?: {
    machine: MachineId;
    slot: string | null;
    /** The design the page is a board of. */ artifact: string;
  };
}

/**
 * What the failure card says. Editing that did not open says so in words, not with the slot's technical error
 * a look at the design keeps its reason.
 */
const failureWords = (error: string | null, editing: boolean): { title: string; reason: string } => {
  if (error === null) return { title: t('next.site.stalled'), reason: t('next.site.stalledReason') };
  if (editing) return { title: t('next.site.editFailed'), reason: t('next.site.editFailedReason') };
  return { title: t('next.site.failed'), reason: error };
};

export interface ISiteStage {
  /** The design's chat the column shows (the apps' selection items send their tasks to it); null while it has none. */
  conv?: ConvId | null;
  mode: TSiteMode;
  /** The section's size goes with it: the first window of «Окно» is placed by it. */
  onMode: (mode: TSiteMode, section: { width: number; height: number }) => void;
  /** The island is folded into its pill. */
  min: boolean;
  /** The docked chat is folded to nothing: its field is not on screen. */
  folded?: boolean;
  run: 'idle' | 'run' | 'done';
  /** The agent of the design's chat is starting or working on a turn (the chat's own «Собираю контекст…»). */
  busy?: boolean;
  float: IFloat | null;
  /** The side of the docked chat, and its width. */
  dock: TDock;
  chatW: number;
  chat: ReactNode;
  island: ReactNode;
  /** A space's finished turn, said as the field's one line (renders nothing itself; HRN-1469). */
  spaceTurn?: ReactNode;
  /** Over the page: the note at a picked block, the bar of the edits. */
  overlay: ReactNode;
  /** On the page, in its viewport's pixels: the pins of the notes. */
  pins: ReactNode;
  /** A click on the page picks a block (through the slot's bridge) instead of acting on it. */
  pick: boolean;
  /** A finger held on a block picks it too (a guest on a phone), without turning picking on. */
  hold?: boolean;
  /** `at` is the block's bottom-left corner in the section's pixels, where its note opens. */
  onPicked: (picked: IPicked, at: { x: number; y: number; right: boolean }) => void;
  /** The settings the board offers now (its `tk:tweaks`), as the page said them. */
  onControls?: (controls: ITweak[]) => void;
  /** The page ended the pick itself: Esc in it, or it went to another address. */
  onPickEnd: () => void;
  /** The page on screen has the slot's bridge (it said hello), or a new page has not yet. */
  onBridge: (ready: boolean) => void;
  /** The looks the page should wear (the notes' edits of the picked blocks); the view sends them to the bridge. */
  styles: IStyleItem[];
  /** The bridge answered the looks: these blocks it no longer has. */
  onStyled: (gone: number[]) => void;
  /** The blocks the pins stand on: the view asks the bridge to follow them and hands its word to the pins. */
  anchors: IAnchorItem[];
  /** The blocks (template numbers) a picture is being drawn into on the page: the board shows a veil on each. */
  veils?: readonly number[];
  /** Whether the page is being picked from (the mode of edits always picks); null while there is no bridge to pick with. */
  picking: boolean | null;
  /** Turns picking on or off: the same switch as the island's button. */
  onPick: () => void;
  /** On a phone in «Чат»: the site is opened over the chat. */
  sview: boolean;
  onSview: (open: boolean) => void;
  /** GitHub #123: closes the design and leaves the same chat across the full width; undefined without a chat. */
  onToChat?: () => void;
  /** Words from the page's tools (a console line, a request) go into the field. */
  onAttach: (words: string) => void;
  /**
   * A deck's context for the chat: one transient chip («Слайд 3 · Заголовок») sent with the next message as an
   * `<annotation>`; null takes it away. Absent from a stage that has no chat field.
   */
  context?: (ctx: { label: string; text: string; selected?: boolean; block?: boolean } | null) => void;
  /** The chat alone, without the window it stands in (a deck's panel hosts it); absent where the stage has no chat. */
  deckChat?: ReactNode;
  /** Sends a message on the person's behalf (a deck's «Короче», a layout change, «＋ слайд»). */
  ask?: (
    text: string,
  ) => void; /** Sends a board's error to the design's chat, opened where it is seen (a board's «Отправить в чат»). */
  report?: (text: string) => void;
  /** Puts words into the chat's field, opened, for the person to send (an app's view where its chat is not yet on). */
  fill?: (text: string) => void;
  /**
   * A space's selection on the page: the chat's window opens beside it. null: nothing selected, the window
   * rises from the field.
   */
  anchor?: (rect: { x: number; y: number; w: number; h: number } | null) => void;
  /** A space's view went back to its list: a chat window open over it folds to its island, never covering it. */
  fold?: () => void;
  /** The section the stage stands in: the window's place is counted from it. */
  bindSection?: (el: HTMLElement | null) => void;
}

interface IMoreItem {
  id: string;
  icon: string;
  label: string;
  /** A separator above it. */
  sep?: boolean;
  kbd?: string;
  /** Set for an item that is on or off. */
  on?: boolean;
  disabled?: boolean;
  /** The rows the item unfolds under itself («Скачать»). */
  sub?: readonly IDownloadItem[];
  pick: () => void;
}

/** «Скачать» in «…»: the page's picture first, then its documents, then the whole project. */
const DOWNLOAD_ORDER = ['page-png', 'page-pdf', 'page-html', 'all-pdf'];
const downloadRank = (id: string) => {
  const at = DOWNLOAD_ORDER.indexOf(id);
  return at < 0 ? DOWNLOAD_ORDER.length : at;
};

const POP = {
  pages: 'pages',
  device: 'device',
  more: 'more',
  phone: 'phone',
  throttle: 'throttle',
} as const;
type TPop = (typeof POP)[keyof typeof POP];

/** One popover at a time; it leaves with its own animation. */
const usePop = () => {
  const [pop, setPop] = useState<TPop | null>(null);
  const { value, state } = usePresence(pop, POP_MS);
  return {
    pop,
    shownPop: value,
    state,
    toggle: (id: TPop) => setPop((current) => (current === id ? null : id)),
    open: (id: TPop) => setPop(id),
    close: () => setPop(null),
  };
};

const useAreaSize = (ref: React.RefObject<HTMLElement | null>): ISize | null => {
  const [size, setSize] = useState<ISize | null>(null);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => setSize({ w: el.clientWidth, h: el.clientHeight });
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [ref]);
  return size;
};

/** The board's settings as the design's panel takes them from the view. */
export interface ITweaksPort {
  onOffer: (controls: ITweak[] | null) => void;
  /** The view hands over the way a change goes to the page on screen. */
  onSend: (send: (name: string, value: TTweakValue) => void) => void;
}

/** What a layer over the page is given: the frame of the page on screen and the origin it is talked to at. */
export interface ICommentsLayerSlot {
  frame: RefObject<HTMLIFrameElement | null>;
  origin: string | null;
}

const SiteView = ({
  title,
  pages,
  tabs,
  tab,
  src,
  frameScope,
  nextFrame,
  heldSrc = null,
  plainSrc = null,
  phoneUrl = null,
  error,
  reloadKey,
  onNavigate,
  onHistory,
  onReload,
  onOpenOutside,
  onCopyLink,
  onRetry,
  onOpenDrawer,
  stage: given,
  away = false,
  designPanel,
  comments,
  chatBoards,
  onNewBoard,
  onBoardError,
  pane,
  toolPane = null,
  access,
  kit,
  devtools,
  memoryKey,
  viewSeed,
  guest,
  mode,
  images,
  downloads,
  full = false,
  tweaksPort,
  commentsLayer: CommentsLayerSlot,
}: ISiteViewProps) => {
  // Looking at the design, nothing of the chat beside it acts on the page: the chat itself is only drawn leaving.
  const stage = away ? undefined : given;
  // The design is looked at, not edited: a guest, a viewer, the viewing mode.
  const looking = guest !== undefined || away || mode?.editing === false;
  const devKey = useKeyHint('site.devtools');
  const inspectKey = useKeyHint('site.inspect');
  const hardKey = useKeyHint('site.hardReload');
  const pickKey = useKeyHint('site.pick');
  const pop = usePop();
  // A phone has one column: the chat docked to the right is stacked like the left one there.
  const phone = usePhone();
  // A phone has one thin bar: back, the page, the mode, «…». The strip of tabs and what sits in it fold into «…».
  const compact = phone && !pane && !guest;
  const memory = useViewMemory(memoryKey ?? null, viewSeed);
  const [deviceId, setDeviceId] = useState(memory.initial.device);
  const [size, setSize] = useState<ISize | null>(memory.initial.size);
  const [rot, setRot] = useState(memory.initial.rot);
  // A phone in a case draws its interface (the status bar, the browser's bars); the bars fold as the page is scrolled.
  const [chrome, setChrome] = useState(memory.initial.chrome);
  // A foldable folded and its animation; a device picked wakes its screen, a turn of it turns the case.
  const [folded, setFolded] = useState(false);
  const [folding, setFolding] = useState<TFolding>(null);
  const [wake, setWake] = useState<'a' | 'b' | null>(null);
  const [turned, setTurned] = useState(false);
  const foldTimers = useRef<number[]>([]);
  useEffect(() => () => foldTimers.current.forEach((id) => window.clearTimeout(id)), []);
  const [browserId, setBrowserId] = useState<string | null>(memory.initial.browser);
  const [bars, setBars] = useState<TBars>(BARS.full);
  const [loadedFor, setLoadedFor] = useState<string | null>(null);
  // A board of the library says when it is drawn (tk:drawn, after lib-ready): its frame is loading until then.
  const [libReadyFor, setLibReadyFor] = useState<string | null>(null);
  const [drawnFor, setDrawnFor] = useState<string | null>(null);
  const [drawGaveUp, setDrawGaveUp] = useState<string | null>(null);
  /** What the page of a frame said of its way (the bridge's progress); a frame of another page starts without. */
  const [progress, setProgress] = useState<(ILoadProgress & { key: string }) | null>(null);
  // «Скачать» of «…» unfolded; it folds again with the menu.
  const [savingOpen, setSavingOpen] = useState(false);
  if (savingOpen && pop.pop !== POP.more) setSavingOpen(false);
  // «Скачать страницу…» of one page: from its row in the list of pages, at that point.
  const [pageSaving, setPageSaving] = useState<{ path: string; x: number; y: number; open: boolean } | null>(
    null,
  );
  // The person's hand on the board (hand-edit.ts): the menu of a right click and a text edited in place.
  const [boardMenu, setBoardMenu] = useState<(IBoardMenu & { at: IPoint; board: boolean }) | null>(null);
  const menuShown = usePresence(boardMenu, POP_MS);
  const [texted, setTexted] = useState<{ key: string; state: ITextState } | null>(null);
  // DevTools under the page, and the settings the board offers in a panel over it.
  const [dev, setDev] = useState(false);
  const [devTab, setDevTab] = useState<TDevTab>(DEV_TAB.console);
  const [devH, setDevH] = useState<number>(DEV_H.start);
  const [tweaksOpen, setTweaksOpen] = useState(false);
  const [tweaksPos, setTweaksPos] = useState<IPoint | null>(null);
  /** What the board of a frame offered; a frame of another page (or a reloaded one) starts without. */
  const [offer, setOffer] = useState<{ key: string; controls: ITweak[] } | null>(null);
  const areaRef = useRef<HTMLDivElement>(null);
  const sectionRef = useRef<HTMLElement>(null);
  const frameRef = useRef<HTMLIFrameElement>(null);
  const anchorsRef = useRef<HTMLDivElement>(null);
  // Where the page says the pinned blocks are: the pins read it in the frame it comes, without a render.
  const [anchorStore] = useState(createAnchorStore);
  const area = useAreaSize(areaRef);

  const current = tabs.find((x) => x.id === tab) ?? tabs[0];
  const pagePath = current?.hist[current.hi] ?? null;
  // «Назад» is only for a link of the prototype followed while looking: the pages the links were followed from.
  const [trail, setTrail] = useState<readonly string[]>([]);
  if (!looking && trail.length > 0) setTrail([]);
  const page = pages.find((p) => p.path === pagePath) ?? null;
  // The hands-on tour waits for these (stores/tour.ts): the page, the device and the mode the person turned to.
  useEffect(() => {
    if (pagePath) tourSignal(`site:page:${pagePath}`);
  }, [pagePath]);
  useEffect(() => tourSignal(`site:device:${deviceId}`), [deviceId]);
  useEffect(() => {
    if (mode?.editing) tourSignal('site:edit');
  }, [mode?.editing]);
  // A phone only looks: a design opened for editing there goes back to looking.
  const leaveEdit = compact && mode?.editing === true && mode.action !== null && !mode.action.ask;
  const toLooking = useEffectEvent(() => mode?.action?.onClick());
  useEffect(() => {
    if (leaveEdit) toLooking();
  }, [leaveEdit]);
  // The switcher reads the page and the project; a project of one page is named once.
  const addrName = page?.name ?? title;
  // The full screen shows the page as «Адаптивно» does, over the whole box; the device chosen comes back after it.
  // A phone shows the page at its own width, never a device drawn inside it.
  const fitted = full || compact;
  const device = deviceOf(fitted ? 'fill' : deviceId);
  // The page's tree for DevTools' «Elements», read through the board's own script while that tab is in view.
  const dom = useDomBridge(
    frameRef,
    src,
    reloadKey,
    dev && devTab === DEV_TAB.elements && devtools !== undefined,
  );
  // The pane's keys are the page's alone: its chat is beside it, and a key there must not act on the page.
  const keys = pane === undefined;
  // The pane's pill stands under the page like the field over the site's: a device keeps room for it (and for the
  // field over the pill when the pane's chat is «Снизу»).
  const paneBottomChat = pane?.chat?.mode === PANE_CHAT.bottom;
  const underField =
    guest === undefined &&
    ((keys && (stage?.mode === 'island' || stage?.mode === 'notes')) || pane !== undefined);
  const paneRoom = paneBottomChat ? 2 * DOCK_ROOM : DOCK_ROOM;
  // A phone has no floating pill and no zoom over the page (the design's mobile.css): the address switches the pages,
  // the device's menu holds the zoom, «…» the settings, so the page's own header and tab bar stay free.
  const dockShown = pane !== undefined && !phone;
  // How the phone is drawn (its case, the status bar, a fingertip) is the person's own for every design (phone-view.ts).
  const phoneView = usePhoneView();
  // A phone or a tablet has its pill beside it, on a computer's window.
  const rotatable = !full && canRotate(device);
  const pillBeside = rotatable && !phone;
  const foldedNow = folded && device.fold !== undefined;
  const vp = area
    ? viewportOf(device, area, fitted ? null : size, !fitted && rot && canRotate(device), {
        padBottom: dockShown ? VP_PAD + paneRoom : VP_PAD,
        padSide: pillBeside ? PILL_ROOM : 0,
        framed: phoneView.frame,
        folded: foldedNow,
      })
    : null;
  const framed = vp?.kind === VP_KIND.frame;
  const overview = vp?.kind === VP_KIND.overview;
  const skin = vp?.skin ?? null;
  // A phone or a tablet shows its system: the status bar and the browser, or none («С экрана «Домой»»).
  const phoneUi = framed && device.os !== undefined;
  const phoneStatus = phoneUi && phoneView.status;
  const phoneBrowser = phoneUi ? browserOf(device, chrome ? browserId : BROWSER_NONE) : null;
  // A phone or a tablet touched: the page's hover is off and the pointer is a fingertip (the bridge, `tk:touch`).
  const touchOn = phoneView.touch && !full && canRotate(device);
  // «Обзор» keeps room at the bottom for the field, or for a guest's list of comments.
  const ovwRoom = underField || guest !== undefined;
  // The pane's field over the pill keeps its own room in the area (site-pane.css).
  const fieldRoom = paneBottomChat && dockShown ? DOCK_ROOM : 0;
  const ovwGroups = useOverviewGroups();
  const cards =
    vp && overview ? overviewOf({ w: vp.vw, h: vp.vh - fieldRoom }, ovwRoom, ovwGroups.groups) : [];
  // «Все экраны» draws its screens at the cards' scale: that is its fit, so the zoom reads the screens' true scale.
  const ovwFit = cards.length > 0 ? cards[0].scale : 1;
  const qr = useMemo(() => (phoneUrl ? qrModules(phoneUrl) : null), [phoneUrl]);
  // The canvas: the page's box zooms and pans over its area; another device or size starts at the fit again.
  const canvas = useCanvasZoom(
    areaRef,
    sectionRef,
    overview ? ovwFit : (vp?.fit ?? 1),
    vp !== null && error === null && !full,
    // Another set of groups in «Все экраны» is another row: it starts at its fit too.
    `${device.id}|${size?.w ?? 0}x${size?.h ?? 0}|${rot}|${foldedNow}|${full}|${overview ? ovwGroups.groups.join(',') : ''}`,
    overview ? overviewBox(cards, ovwRoom) : { w: vp ? vp.bw * vp.fit : 0, h: vp ? vp.bh * vp.fit : 0 },
    // «Вписать»: a phone or a tablet whole, a computer whole up to its own size, «По окну» filling the window.
    standardFit(vp?.kind ?? VP_KIND.fill, device.group),
    { start: memory.initial, onChange: memory.save },
  );
  const pillHidden = usePillPrefs(ZOOM_PILL).hidden;
  const phonePillHidden = usePhonePillHidden();
  // What the device says is kept with the canvas's view; the page is restored once, then followed.
  const { save } = memory;
  useEffect(() => {
    save({ device: deviceId, size, rot, chrome, browser: browserId });
  }, [save, deviceId, size, rot, chrome, browserId]);
  const pageRestored = useRef(memory.initial.page === null);
  const keepPage = useEffectEvent((path: string) => {
    if (!pageRestored.current) {
      pageRestored.current = true;
      const wanted = memory.initial.page;
      // Only a design opened on its first page is taken back: a link to another page (a note's chip) wins.
      const first = path === pages[0]?.path;
      if (wanted !== null && wanted !== path && first && pages.some((p) => p.path === wanted)) {
        onNavigate(wanted);
        return;
      }
    }
    memory.save({ page: path });
  });
  useEffect(() => {
    if (memoryKey !== undefined && pagePath) keepPage(pagePath);
  }, [memoryKey, pagePath]);
  const latestCanvas = useRef(canvas);
  useEffect(() => {
    latestCanvas.current = canvas;
  });

  // The viewport's sizes go to the CSS through custom properties on its element.
  useLayoutEffect(() => {
    const el = areaRef.current;
    if (!el || !vp) return;
    for (const [name, value] of Object.entries(viewportVars(vp))) el.style.setProperty(name, value);
  });

  // DevTools' height goes to the CSS the same way (the field over the page stands above it).
  useLayoutEffect(() => {
    sectionRef.current?.style.setProperty('--dev-h', `${devH}px`);
  }, [devH]);

  // The chat's width goes to the CSS the same way.
  const chatW = stage?.chatW;
  useLayoutEffect(() => {
    if (chatW !== undefined) sectionRef.current?.style.setProperty('--site-chat', `${chatW}px`);
  }, [chatW]);

  // The window of «Окно» goes to the CSS through custom properties on the section.
  const float = stage?.float ?? null;
  useLayoutEffect(() => {
    const el = sectionRef.current;
    if (!el || !float) return;
    el.style.setProperty('--fx', `${float.x}px`);
    el.style.setProperty('--fy', `${float.y}px`);
    el.style.setProperty('--fw', `${float.w}px`);
    el.style.setProperty('--fh', `${float.h}px`);
  }, [float]);

  // The pane's choice of the chat's place: applied once the section has its size, kept when a drag moves the chat.
  const paneChat = pane?.chat ?? null;
  const sectionSize = () => {
    const rect = sectionRef.current?.getBoundingClientRect();
    return { width: rect?.width ?? 0, height: rect?.height ?? 0 };
  };
  const latestPaneChat = useRef(paneChat);
  useEffect(() => {
    latestPaneChat.current = paneChat;
  });
  useLayoutEffect(() => {
    // Once, when the pane opens.
    latestPaneChat.current?.start(sectionSize());
  }, []);
  const stageMode = stage?.mode;
  useEffect(() => {
    latestPaneChat.current?.sync(sectionSize());
  }, [stageMode]);
  const paneOwnChat = paneChat !== null && paneChat.mode !== PANE_CHAT.left;

  // The slot's bridge: the pick goes to the page while it is wanted (again after the page loads anew), its answers come
  // only from the slot's window and origin.
  const pick = stage?.pick ?? false;
  const latestStage = useRef(stage);
  // A board's error card sends its error to the chat's field (board-error.ts): once per error, for the board on screen.
  const boardErrors = useRef<IBoardErrorSink | null>(null);
  const latestBoardError = useRef(onBoardError);
  const latestPagePath = useRef(pagePath);
  useEffect(() => {
    latestPagePath.current = pagePath;
    latestBoardError.current = onBoardError;
  });
  // Where the page was scrolled to: what the bars of a phone do (devices.ts trackScroll).
  const track = useRef(SCROLL_START);
  /** The bars of a phone start unfolded again: another page, another browser, another device. */
  const unfoldBars = () => {
    track.current = SCROLL_START;
    setBars(BARS.full);
  };
  /** Another device: it stands upright and unfolded, and its screen wakes. */
  const pickDevice = (id: string) => {
    foldTimers.current.forEach((timer) => window.clearTimeout(timer));
    setDeviceId(id);
    setSize(null);
    setRot(false);
    setTurned(false);
    setFolded(false);
    setFolding(null);
    setWake((was) => (was === 'a' ? 'b' : 'a'));
    unfoldBars();
  };
  // A link of the board to another page of the design (page-link.ts): the site switches its page as its own
  // list does (the tab's history, so Back steps back), and the link's props and block wait for the new board's script.
  const pendingTake = useRef<IPendingTake | null>(null);
  const followLink = (go: IPageGo, post: (message: unknown) => void) => {
    const path = pageOfLink(go, pages, pagePath);
    if (path === null) {
      toast(
        guest
          ? t('next.site.linkNotShared')
          : t('next.site.linkMissing', { page: go.ref.slice(go.ref.lastIndexOf('/') + 1) }),
        {
          tone: 'error',
        },
      );
      return;
    }
    if (path === pagePath) {
      if (hasTake(go)) post(goTake(go));
      return;
    }
    pendingTake.current = { path, props: go.props, hash: go.hash };
    if (looking && pagePath !== null) setTrail((list) => [...list, pagePath]);
    onNavigate(path);
  };
  // The page a link opened tells the frame it asks for: a phone's page is shown in a phone, a wide one leaves the phone.
  const followFrame = (w: number) => {
    if (full) return;
    const view = frameView(w);
    const onPhone = device.group === DEVICE_GROUP.phones;
    if (view === FRAME_VIEW.any || (view === FRAME_VIEW.phone) === onPhone) return;
    pickDevice(view === FRAME_VIEW.phone ? LINK_PHONE : 'fill');
  };
  const latestFollow = useRef(followLink);
  const latestFrame = useRef(followFrame);
  useEffect(() => {
    latestFollow.current = followLink;
    latestFrame.current = followFrame;
  });
  /** A button on a picture of the page was pressed: the editor or the generator is open for it. */
  const [imageOpen, setImageOpen] = useState<IImageOpen | null>(null);
  const imagesOn = images !== undefined && guest === undefined;
  const latestImagesOn = useRef(imagesOn);
  // The keys of the page act on the latest of what they toggle (they listen once).
  const latestPick = useRef<() => void>(() => undefined);
  const latestInspect = useRef<() => void>(() => undefined);
  const latestDom = useRef(dom);
  const latestReload = useRef<(hard: boolean) => void>(() => undefined);
  // The element of the page a step of the tour is about: the page follows it beside the notes' blocks.
  const tourFrame = useTourStep()?.frame ?? null;
  const latestTourFrame = useRef(tourFrame);
  useEffect(() => {
    latestStage.current = stage;
    latestImagesOn.current = imagesOn;
    latestTourFrame.current = tourFrame;
  });
  const scrollTimer = useRef(0);
  const onPageScroll = (y: number) => {
    const section = sectionRef.current;
    if (section) {
      section.dataset.scrolling = 'true';
      window.clearTimeout(scrollTimer.current);
      scrollTimer.current = window.setTimeout(() => {
        section.dataset.scrolling = 'false';
      }, SCROLL_MS);
    }
    track.current = trackScroll(track.current, y);
    setBars(track.current.bars);
  };
  const latestScroll = useRef(onPageScroll);
  useEffect(() => {
    latestScroll.current = onPageScroll;
  });
  useEffect(() => () => window.clearTimeout(scrollTimer.current), []);
  const turn = () => {
    setRot((r) => !r);
    setTurned(true);
    setWake(null);
  };
  const fold = () => {
    if (folding !== null) return;
    const [first, second] = folded ? (['dim', 'open'] as const) : (['close', 'wake'] as const);
    setWake(null);
    setFolding(first);
    foldTimers.current = [
      window.setTimeout(() => {
        setFolded(!folded);
        setFolding(second);
        unfoldBars();
      }, FOLD[first]),
      window.setTimeout(() => setFolding(null), FOLD[first] + FOLD[second]),
    ];
  };
  /** The frame that has been loaded for this page: before its first load it is a blank page of this window's own origin,
   *  and a message to the slot's origin would be refused there (a console error in WebKit at every new page). */
  const loadedKey = useRef<string | null>(null);
  /** The slot's frame and origin, or null while there is no page. */
  const frameOf = () => {
    const frame = frameRef.current;
    if (!frame || src === null) return null;
    // A guest's board and one in a slot by path have an opaque origin (their sandbox): nothing to name, the window
    // itself is what is trusted.
    const origin = guest ? GUEST_ORIGIN : frameOrigin(src);
    return origin === null ? null : { frame, origin };
  };
  /** The slot's window and origin once its page has loaded, or null before. */
  const slotOf = () => (loadedKey.current === `${src}#${reloadKey}` ? frameOf() : null);
  const ping = () => {
    const slot = slotOf();
    slot?.frame.contentWindow?.postMessage({ type: 'tk:ping' }, slot.origin);
  };
  // A design kept hidden behind another tab (features/shell/kept-screens) has its effects taken down: the board is told
  // it is out of sight, as a deck tells its slides (tk:deck-live), so it stops its stream and polling until it is
  // shown again, when it catches up. The frame is taken at the effect's start: refs are gone by the time it is cleaned.
  useEffect(() => {
    const target = frameOf();
    target?.frame.contentWindow?.postMessage({ type: 'tk:deck-live', on: true }, target.origin);
    return () => target?.frame.contentWindow?.postMessage({ type: 'tk:deck-live', on: false }, target.origin);
    // frameOf reads the frame of this src.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [src, reloadKey]);
  const sendStyles = () => {
    const slot = slotOf();
    slot?.frame.contentWindow?.postMessage(
      { type: 'tk:styles', items: latestStage.current?.styles ?? [] },
      slot.origin,
    );
  };
  const latestTouch = useRef(touchOn);
  useEffect(() => {
    latestTouch.current = touchOn;
  });
  const sendTouch = () => {
    const slot = slotOf();
    slot?.frame.contentWindow?.postMessage({ type: 'tk:touch', on: latestTouch.current }, slot.origin);
  };
  // The board takes the right click for the design's menu, and a double click edits a text where the person may.
  // The hand edit writes a board of the project's root only (the patch route takes no other).
  const textOn =
    images !== undefined && guest === undefined && pagePath !== null && /^[^/]+\.dc\.html$/.test(pagePath);
  const menuOn = guest === undefined && !full;
  const latestHand = useRef({ textOn, menuOn });
  useEffect(() => {
    latestHand.current = { textOn, menuOn };
  });
  const sendHand = () => {
    const slot = slotOf();
    slot?.frame.contentWindow?.postMessage(
      { type: 'tk:menu-mode', on: latestHand.current.menuOn },
      slot.origin,
    );
    slot?.frame.contentWindow?.postMessage(
      { type: 'tk:text-mode', on: latestHand.current.textOn },
      slot.origin,
    );
  };
  const postBoard = (message: Record<string, unknown>) => {
    const slot = frameOf();
    slot?.frame.contentWindow?.postMessage(message, slot.origin);
  };
  /** An edit made by hand goes into the board's source, as a version of its own. */
  const saveText = (save: ITextSave) => {
    if (!images || latestPagePath.current === null) return;
    void saveHandEdit(images.machine, images.artifact, latestPagePath.current, save).then(
      (refusal) => {
        if (refusal === null) {
          toast(t('next.site.text.saved'));
          return;
        }
        toast(t(refusal === HAND_REFUSAL.dynamic ? 'next.site.text.dynamic' : 'next.site.text.changed'), {
          tone: 'error',
        });
        // The page shows words the source does not have: it is loaded again.
        latestReload.current(false);
      },
      (failure: Error) => {
        toast(t('next.site.text.failed'), { tone: 'error', description: failure.message });
        latestReload.current(false);
      },
    );
  };
  const latestSaveText = useRef(saveText);
  useEffect(() => {
    latestSaveText.current = saveText;
  });
  /** The page is on a canvas: its script tells Space and the middle button's drag (a board's says no hello). */
  const onCanvas = () => {
    const slot = slotOf();
    slot?.frame.contentWindow?.postMessage({ type: 'tk:canvas', on: true }, slot.origin);
  };
  // The blocks a picture is being drawn into: a veil on each, and off the ones that are done.
  const veiled = useRef<readonly number[]>([]);
  const sendVeils = (fresh: boolean) => {
    const slot = slotOf();
    if (!slot) return;
    const want = latestStage.current?.veils ?? [];
    const before = fresh ? [] : veiled.current;
    for (const tpl of want) {
      if (!before.includes(tpl))
        slot.frame.contentWindow?.postMessage({ type: 'tk:gen', tpl, on: true }, slot.origin);
    }
    for (const tpl of before) {
      if (!want.includes(tpl))
        slot.frame.contentWindow?.postMessage({ type: 'tk:gen', tpl, on: false }, slot.origin);
    }
    veiled.current = want;
  };
  // Whether the page draws its buttons on pictures: only where the person may change the design.
  const sendImages = () => {
    const slot = slotOf();
    slot?.frame.contentWindow?.postMessage(
      { type: 'tk:images', on: latestImagesOn.current, scale: frameScale(slot.frame) },
      slot.origin,
    );
  };
  const sendAnchors = () => {
    const slot = slotOf();
    const ring = latestTourFrame.current;
    const items = [
      ...(latestStage.current?.anchors ?? []),
      ...(ring ? [{ key: TOUR_ANCHOR, pick: null, selector: ring }] : []),
    ];
    slot?.frame.contentWindow?.postMessage({ type: 'tk:anchors', items }, slot.origin);
  };
  const arm = (on: boolean) => {
    const slot = slotOf();
    slot?.frame.contentWindow?.postMessage({ type: 'tk:pick', on }, slot.origin);
    slot?.frame.contentWindow?.postMessage(
      { type: 'tk:hold', on: latestStage.current?.hold === true },
      slot.origin,
    );
  };
  // The board's own script offers its settings: the site says it draws them (and whether the panel is open), takes the
  // list and sends the changes back.
  const tweaksOpenNow = useRef(tweaksOpen);
  useEffect(() => {
    tweaksOpenNow.current = tweaksOpen;
  });
  const hostTweaks = () => {
    const slot = slotOf();
    slot?.frame.contentWindow?.postMessage(
      { type: 'tk:tweaks-host', on: true, open: tweaksOpenNow.current },
      slot.origin,
    );
  };
  const sendTweak = (name: string, value: TTweakValue) => {
    const slot = slotOf();
    slot?.frame.contentWindow?.postMessage({ type: 'tk:tweak', name, value }, slot.origin);
  };
  const staged = stage !== undefined;
  /** The frame (its src and reload) whose board's own script said it was ready: it says so once, at its load. */
  const libFor = useRef<string | null>(null);
  useEffect(() => {
    const slot = frameOf();
    if (!slot) return;
    const { frame, origin } = slot;
    const key = `${src}#${reloadKey}`;
    // The board's script picks blocks and wears the notes' looks for the stage, as the slot's bridge does.
    const toLib = () => {
      const now = latestStage.current;
      frame.contentWindow?.postMessage(boardChat(!guest && latestBoardError.current !== undefined), origin);
      if (!now) return;
      frame.contentWindow?.postMessage({ type: 'tk:pick', on: now.pick }, origin);
      frame.contentWindow?.postMessage({ type: 'tk:hold', on: now.hold === true }, origin);
      frame.contentWindow?.postMessage({ type: 'tk:styles', items: now.styles }, origin);
      now.onBridge(true);
    };
    latestStage.current?.onBridge(false);
    anchorStore.set(null);
    // Asked again when the page of this frame has loaded already (the effect runs for other reasons too); a page still
    // on its way says «hello» itself, and onLoad asks. A board's script said its word at its load: the editing mode
    // that comes after it is told it straight away.
    if (libFor.current === key) toLib();
    ping();
    const onMessage = (event: MessageEvent) => {
      if (!fromFrame(event, frame.contentWindow, origin)) return;
      const boardError = readBoardError(event.data);
      if (boardError) {
        if (guest || !latestBoardError.current || !personIsActing()) return;
        boardErrors.current ??= boardErrorSink((words) => latestBoardError.current?.(words));
        if (boardErrors.current.take(boardError, latestPagePath.current ?? ''))
          frame.contentWindow?.postMessage(boardErrorAck(boardError), origin);
        return;
      }
      const hand = readHandMessage(event.data);
      if (hand) {
        if (hand.k === 'menu') {
          if (!latestHand.current.menuOn) return;
          const box = frame.getBoundingClientRect();
          const home = sectionRef.current?.getBoundingClientRect();
          const k = frame.offsetWidth > 0 ? box.width / frame.offsetWidth : 1;
          setBoardMenu({
            ...hand.menu,
            at: {
              x: box.left - (home?.left ?? 0) + hand.menu.x * k,
              y: box.top - (home?.top ?? 0) + hand.menu.y * k,
            },
            board: true,
          });
        } else if (!latestHand.current.textOn) return;
        else if (hand.k === 'text-state')
          setTexted(hand.state && { key: `${src}#${reloadKey}`, state: hand.state });
        else if (hand.k === 'text-save') latestSaveText.current(hand.save);
        else toast(t('next.site.text.dynamic'), { tone: 'error' });
        return;
      }
      const go = readPageGo(event.data);
      if (go) {
        latestFollow.current(go, (m) => frame.contentWindow?.postMessage(m, origin));
        return;
      }
      const message = readBridgeMessage(event.data);
      const now = latestStage.current;
      if (message?.k === 'anchored') {
        anchorStore.set(message.report);
        return;
      }
      if (message?.k === 'scroll') {
        latestScroll.current(message.y);
        return;
      }
      if (message?.k === 'wheel') {
        // A pinch or ⌘-wheel over the page: the canvas takes it as its own wheel at the same spot of the screen.
        const at = frame.getBoundingClientRect();
        const k = frame.offsetWidth > 0 ? at.width / frame.offsetWidth : 1;
        areaRef.current?.dispatchEvent(
          new WheelEvent('wheel', {
            bubbles: true,
            cancelable: true,
            ctrlKey: true,
            deltaY: message.dy,
            clientX: at.left + message.x * k,
            clientY: at.top + message.y * k,
          }),
        );
        return;
      }
      if (message?.k === 'space') {
        latestCanvas.current.fromPage.space(message.down);
        return;
      }
      if (message?.k === 'pan') {
        latestCanvas.current.fromPage.pan(message.dx, message.dy, message.drag);
        return;
      }
      if (message?.k === 'progress') {
        setProgress({ key: `${src}#${reloadKey}`, state: message.state, resources: message.resources });
        return;
      }
      if (message?.k === 'tweaks') {
        setOffer({ key: `${src}#${reloadKey}`, controls: message.controls });
        latestStage.current?.onControls?.(message.controls);
        return;
      }
      if (message?.k === 'frame') {
        latestFrame.current(message.w);
        return;
      }
      if (message?.k === 'tweaks-panel') {
        setTweaksOpen(message.open);
        return;
      }
      if (message?.k === 'images-hello') {
        sendImages();
        return;
      }
      if (message?.k === 'image-act') {
        if (latestImagesOn.current && message.act.file !== '')
          setImageOpen({ act: message.act, box: screenBoxOf(frame.getBoundingClientRect(), message.act) });
        return;
      }
      if (message?.k === 'reload') {
        // A board on an opaque origin (a slot by path) cannot reload itself past a proxy in front of the platform: the
        // frame is loaded again from this page, as the reload button does.
        latestReload.current(false);
        return;
      }
      if (message?.k === 'drawn') {
        setDrawnFor(`${src}#${reloadKey}`);
        return;
      }
      if (message?.k === 'lib-ready') {
        setLibReadyFor(`${src}#${reloadKey}`);
        hostTweaks();
        sendImages();
        // A board of the library follows the pinned blocks itself (its script, not the slot's bridge, which says hello).
        anchorStore.set(null);
        sendAnchors();
        sendVeils(true);
        // Its script tells Space and the middle button's drag on a canvas too.
        frame.contentWindow?.postMessage({ type: 'tk:canvas', on: true }, origin);
        sendTouch();
        sendHand();
        // Its error card may send the error to the chat when there is one; it picks blocks for the stage.
        libFor.current = key;
        toLib();
        const take = pendingTake.current;
        if (take && take.path === latestPagePath.current) {
          pendingTake.current = null;
          frame.contentWindow?.postMessage(goTake(take), origin);
        }
        return;
      }
      if (message?.k === 'key') {
        // The guest page listens for its keys on its window: one pressed over the board comes there the same.
        if (guest)
          window.dispatchEvent(
            new KeyboardEvent('keydown', {
              key: guestKeyValue(message.key),
              code: GUEST_KEY_CODE[message.key],
            }),
          );
        return;
      }
      if (message?.k === 'hello') {
        hostTweaks();
        sendImages();
        // The page is on a canvas: its bridge tells Space, the middle button's drag and the wheel it cannot scroll by.
        frame.contentWindow?.postMessage({ type: 'tk:canvas', on: true }, origin);
      }
      if (!message || !now) return;
      if (message.k === 'styled') {
        now.onStyled(message.gone);
        return;
      }
      if (message.k === 'hello') {
        frame.contentWindow?.postMessage({ type: 'tk:pick', on: now.pick }, origin);
        sendStyles();
        anchorStore.set(null);
        sendAnchors();
        now.onBridge(true);
        track.current = SCROLL_START;
        setBars(BARS.full);
      } else if (message.k === 'pick-off') now.onPickEnd();
      // `doc-scroll` is only told to the screens of «Все экраны» (site-overview.tsx), never by this frame.
      else if (message.k === 'picked') {
        const box = frame.getBoundingClientRect();
        const home = sectionRef.current?.getBoundingClientRect();
        const k = frame.offsetWidth > 0 ? box.width / frame.offsetWidth : 1;
        const { rect } = message.picked;
        const x = box.left - (home?.left ?? 0) + rect.x * k;
        const y = box.top - (home?.top ?? 0) + (rect.y + rect.height) * k;
        const width = home?.width ?? 0;
        // The card keeps the corner as it is at the canvas's fit and follows the zoom from there (its CSS clamps it in).
        const at = latestCanvas.current.toFit({ x, y });
        now.onPicked(message.picked, { x: at.x, y: at.y, right: x > width / 2 });
      }
    };
    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
    // slotOf reads the frame of this src; a new function each render must not listen again. The editing mode that comes
    // over a page already loaded asks again: the page said its hello before there was a stage to hear it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [src, reloadKey, staged]);
  useEffect(() => {
    arm(pick);
    // arm reads the frame of this src.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pick, src, reloadKey]);
  // The looks tried on the page: the whole set goes again whenever it changes (an empty set gives every block back).
  const stylesKey = JSON.stringify(stage?.styles ?? []);
  useEffect(() => {
    sendStyles();
    // sendStyles reads the frame of this src and the latest stage; the set's key says when it changed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stylesKey, src, reloadKey]);
  useEffect(() => {
    sendImages();
    // sendImages reads the frame of this src and the latest flag; the flag and the page say when to tell it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [imagesOn, src, reloadKey]);
  useEffect(() => {
    sendTouch();
    // sendTouch reads the frame of this src and the latest flag.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [touchOn, src, reloadKey]);
  useEffect(() => {
    sendHand();
    // sendHand reads the frame of this src and the latest flags.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [textOn, menuOn, src, reloadKey]);
  // A zoom of the canvas shrinks or grows the page: its buttons are told the scale once the smooth step has settled.
  useEffect(() => {
    const timer = window.setTimeout(sendImages, 400);
    return () => window.clearTimeout(timer);
    // sendImages reads the frame of this src; only the zoom and the page's address change the scale.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [canvas.pct, src]);
  const veilsKey = JSON.stringify(stage?.veils ?? []);
  useEffect(() => {
    sendVeils(false);
    // sendVeils reads the frame of this src and the latest stage; the set's key says when it changed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [veilsKey, src, reloadKey]);
  // The ring of the voice's quick edit on its block (live-mark.ts): told again when the board reloads.
  const live = useLiveMark();
  const liveTpl = useRef<number | null>(null);
  useEffect(() => {
    const slot = slotOf();
    if (!slot) return;
    const tpl = live?.tpl ?? liveTpl.current;
    liveTpl.current = live?.tpl ?? null;
    if (tpl === null) return;
    slot.frame.contentWindow?.postMessage({ type: 'tk:live', tpl, phase: live?.phase ?? null }, slot.origin);
    // slotOf reads the frame of this src; the mark and the reload say when to tell the board.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [live, src, reloadKey]);
  // An edit without a picked block: the board flashes the block its changed lines make.
  const liveHint = useLiveHint();
  useEffect(() => {
    const slot = slotOf();
    if (!slot || !liveHint) return;
    slot.frame.contentWindow?.postMessage({ type: 'tk:live-find', hint: liveHint.hint }, slot.origin);
    // slotOf reads the frame of this src; a new hint says when to tell the board.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [liveHint]);
  // The blocks the pins stand on: the set goes again whenever it changes.
  const anchorsKey = JSON.stringify([stage?.anchors ?? [], tourFrame]);
  useEffect(() => {
    sendAnchors();
    // sendAnchors reads the frame of this src and the latest stage; the set's key says when it changed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [anchorsKey, src, reloadKey]);
  useEffect(() => {
    hostTweaks();
    // hostTweaks reads the frame of this src.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tweaksOpen, src, reloadKey]);

  // Outside press and Esc close a popover.
  useEffect(() => {
    if (pop.pop === null) return;
    const onDown = (event: PointerEvent) => {
      const target = event.target as Element;
      if (!target.closest('.hx-browser__anchor, .hx-browser__addr, .hx-tabs__add')) pop.close();
    };
    const onKey = (event: globalThis.KeyboardEvent) => {
      if (event.key === 'Escape') pop.close();
    };
    window.addEventListener('pointerdown', onDown);
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('pointerdown', onDown);
      window.removeEventListener('keydown', onKey);
    };
  });

  // The page's keys (features/keys): «V» turns picking on and off; ⌥⌘I turns DevTools on and off, ⇧⌘C inspects an
  // element on the page, Esc stops that; ⌘R reloads the page of the design, ⇧⌘R clears the cache first (the browser's own
  // reload is not wanted here). The tips of the buttons say the shortcuts as they are now.
  const canPick = keys && stage !== undefined && stage.picking !== null;
  const canDev = keys && devtools !== undefined;
  useKeys(
    KEY_SCOPE.site,
    {
      'site.boards': keys && pages.length > 0 ? () => pop.toggle(POP.pages) : undefined,
      'site.comment': looking && comments ? () => comments.toggle() : undefined,
      'site.pick': canPick ? () => latestPick.current() : undefined,
      'site.devtools': canDev ? () => setDev((on) => !on) : undefined,
      'site.inspect': canDev ? () => latestInspect.current() : undefined,
      'site.stopInspect': () => {
        if (!latestDom.current.inspect) return false;
        latestDom.current.setInspect(false);
      },
      'site.reload': () => latestReload.current(false),
      'site.hardReload': () => latestReload.current(true),
    },
    keys,
  );

  // A handle of the box of «Резина» writes the size on the elements while it is dragged; the state takes it on release.
  const gripViewport = (event: ReactPointerEvent<HTMLElement>, edge: 'e' | 's' | 'se') => {
    const el = areaRef.current;
    if (!el || !vp) return;
    dragViewport(event, el, {
      edge,
      size: { w: vp.vw, h: vp.vh },
      fit: vp.fit,
      onSize: (next) => {
        setDeviceId('fill');
        setRot(false);
        setSize(next);
      },
      restore: () => {
        el.dataset.dev = vp.kind;
        for (const [name, value] of Object.entries(viewportVars(vp))) el.style.setProperty(name, value);
        const label = el.closest('.hx-site')?.querySelector('.hx-browser__dev-size')?.firstChild;
        if (label) label.nodeValue = `${vp.vw} × ${vp.vh}`;
      },
    });
  };

  const frameKey = `${src}#${reloadKey}`;
  // Something else than the board in the frame: it opens anew by itself, and the machine's journal says so.
  useFrameRetry({ frameRef, src, frameKey, loadedFor, libReadyFor, onRetry });
  const tweaks = offer?.key === frameKey ? offer.controls : null;
  const noTweaks = tweaks === null || tweaks.length === 0;
  // The design's panel draws the settings: it hears the offer of the page on screen and sends its changes through here.
  useEffect(() => {
    tweaksPort?.onOffer(tweaks);
  }, [tweaks, tweaksPort]);
  useEffect(() => {
    tweaksPort?.onSend(sendTweak);
  });
  const tweaksPanel = usePresentFlag(tweaksOpen, TWEAKS_OUT_MS);
  const toggleDev = () => {
    if (dev && pop.pop === POP.throttle) pop.close();
    setDev(!dev);
  };
  // Inspecting a page's element and picking a block for a note are two ways to use a click on the page: one at a time.
  const togglePick = () => {
    if (dom.inspect) dom.setInspect(false);
    stage?.onPick();
  };
  const toggleInspect = () => {
    if (dom.inspect) {
      dom.setInspect(false);
      return;
    }
    if (stage?.picking) stage.onPick();
    setDev(true);
    setDevTab(DEV_TAB.elements);
    dom.setInspect(true);
  };
  const inspectDisabled = src === null;
  useEffect(() => {
    latestPick.current = togglePick;
    latestInspect.current = toggleInspect;
    latestDom.current = dom;
    latestReload.current = onReload;
  });
  const toggleTweaks = () => {
    if (tweaksOpen || !noTweaks) setTweaksOpen(!tweaksOpen);
  };
  // The wait is the whole way: the machine opens the page, then the frame loads it. A design with no board has nothing
  // to wait for; a page that has not loaded for too long is a failure, not a wait.
  const waiting = src === null && error === null && (tabs.length > 0 || title === '');
  // A board of the library is drawn only when its script says so; a page without it, at its load.
  const drawing = libReadyFor === frameKey && drawnFor !== frameKey && drawGaveUp !== frameKey;
  const pending = waiting || (src !== null && (loadedFor !== frameKey || drawing));
  useEffect(() => {
    if (loadedFor !== frameKey || !drawing) return;
    const timer = setTimeout(() => setDrawGaveUp(frameKey), DRAW_WAIT_MS);
    return () => clearTimeout(timer);
  }, [loadedFor, frameKey, drawing]);
  const clock = useSiteLoad(`${tab}|${pagePath ?? ''}|${reloadKey}`, pending);
  const stalled = clock.stalled;
  const failed = error !== null || stalled;
  const loading = pending && !failed;
  const failure = failureWords(error, mode?.editing === true);
  const frameProgress = progress?.key === frameKey ? progress : null;
  const loadStage = loadStageOf(frameProgress, clock.slow);
  // The page the followed link was on, while it is still the step back in the history.
  const backTo = trail.at(-1) ?? null;
  const canBack =
    looking && backTo !== null && current !== undefined && current.hist[current.hi - 1] === backTo;
  const goBack = () => {
    setTrail((list) => list.slice(0, -1));
    onHistory(-1);
  };
  const sizeText = vp ? `${vp.vw} × ${vp.vh}` : '';
  // The phone's pill stands by a phone or a tablet on the canvas.
  const phonePillOn = rotatable && !failed;
  /** The host the phone's address bar shows. */
  const host = window.location.hostname;

  const pageName = (path: string) => pages.find((p) => p.path === path)?.name ?? path;

  /** «Скачать»: the downloads run as jobs; their notices say how it goes. */
  const saving = downloads
    ? projectDownloads({
        ...downloads,
        title,
        board: pages.some((p) => p.path === pagePath) ? pagePath : null,
      }).sort((a, b) => downloadRank(a.id) - downloadRank(b.id))
    : [];
  const versions = designPanel
    ? { on: designPanel.tab === DESIGN_TAB.versions, toggle: () => designPanel.open(DESIGN_TAB.versions) }
    : mode?.versions
      ? { on: mode.versions.open, toggle: mode.versions.onToggle }
      : null;
  const closing = (run: () => void) => () => {
    pop.close();
    run();
  };
  /** What «Ещё» offers: only what works here, in parts divided by a line. */
  const offered: (IMoreItem | null | undefined | false)[][] = [
    [
      compact && {
        id: 'drawer',
        icon: 'panel-left',
        label: t('next.header.openMenu'),
        pick: closing(onOpenDrawer),
      },
      compact &&
        toolPane && {
          id: 'tool-pane',
          icon: 'panel-right',
          label: t('next.header.pane'),
          on: toolPane.open,
          pick: closing(toolPane.onToggle),
        },
      compact && {
        id: 'reload',
        icon: 'rotate-cw',
        label: t('next.site.reload'),
        pick: closing(() => onReload(false)),
      },
      onOpenOutside && {
        id: 'outside',
        icon: 'external-link',
        label: t('next.site.openOutside'),
        pick: closing(onOpenOutside),
      },
      versions && {
        id: 'versions',
        icon: 'history',
        label: t('next.versions.title'),
        on: versions.on || undefined,
        pick: closing(versions.toggle),
      },
      guest?.account && {
        id: 'guest-name',
        icon: 'user',
        label: guest.account.label,
        pick: closing(guest.account.onOpen),
      },
      phone && (pane || access)
        ? {
            id: 'share',
            icon: 'share',
            label: access?.label ?? t('next.pane.designShare'),
            pick: closing(() => (access ? access.onOpen() : pane?.onShare())),
          }
        : null,
    ],
    [
      // A QR to open the page on a phone means nothing on the phone itself.
      qr && !phone
        ? { id: 'phone', icon: 'qr-code', label: t('next.site.phone'), pick: () => pop.open(POP.phone) }
        : null,
      // A phone has no «…»-less way to the link: its share button is here too.
      phone && {
        id: 'link',
        icon: 'link',
        label: t('next.site.copyLink'),
        pick: closing(onCopyLink),
      },
    ],
    [
      !looking &&
        devtools && {
          id: 'devtools',
          icon: 'panel-bottom',
          label: t('next.site.devtools'),
          kbd: devKey ?? undefined,
          on: dev,
          pick: closing(toggleDev),
        },
      !looking &&
        devtools && {
          id: 'inspect',
          icon: 'code-xml',
          label: t('next.site.inspect'),
          kbd: inspectKey ?? undefined,
          on: dom.inspect || undefined,
          disabled: inspectDisabled,
          pick: closing(toggleInspect),
        },
    ],
    [
      saving.length > 0 && {
        id: 'download',
        icon: 'download',
        label: t('next.download.label'),
        sub: saving,
        pick: () => setSavingOpen((open) => !open),
      },
    ],
    [
      {
        id: 'hard',
        icon: 'eraser',
        label: t('next.site.hardReload'),
        kbd: hardKey ?? undefined,
        pick: closing(() => onReload(true)),
      },
      // The zoom pill hidden from its own menu comes back from here.
      pillHidden && !pane && !phone
        ? {
            id: 'pill',
            icon: 'eye',
            label: t('next.site.zoomPillShow'),
            pick: closing(() => showPill(ZOOM_PILL)),
          }
        : null,
      // So does the phone's pill, while there is a phone or a tablet to set.
      phonePillHidden && phonePillOn && !phone
        ? {
            id: 'phone-pill',
            icon: 'smartphone',
            label: t('next.site.phonePillShow'),
            pick: closing(() => showPill(PHONE_PILL)),
          }
        : null,
    ],
  ];
  const flat = offered.flatMap((part) =>
    part.filter((item): item is IMoreItem => Boolean(item)).map((item, i) => ({ ...item, sep: i === 0 })),
  );
  if (flat.length > 0) flat[0] = { ...flat[0], sep: false };
  // A phone's «…» holds what looking needs: the versions, sharing, the link, the other device, the menu.
  const PHONE_MORE = ['versions', 'share', 'link', 'phone', 'outside', 'reload', 'drawer'];
  const moreItems = compact
    ? flat
        .filter((item) => PHONE_MORE.includes(item.id))
        .sort((a, b) => PHONE_MORE.indexOf(a.id) - PHONE_MORE.indexOf(b.id))
        .map((item) => ({ ...item, sep: item.id === 'drawer' }))
    : flat;
  // On the phone the bar's menus (pages, device, «…») are bottom sheets: a scrim under them, a swipe down closes them.
  const barMenu = pop.shownPop === POP.pages || pop.shownPop === POP.device || pop.shownPop === POP.more;
  /** «Настройки страницы»: the tools panel's tab, or the panel over the page where there is no tools panel. */
  const settings = designPanel
    ? {
        on: designPanel.tab === DESIGN_TAB.tweaks,
        disabled: false,
        toggle: () => designPanel.open(DESIGN_TAB.tweaks),
      }
    : { on: tweaksOpen, disabled: noTweaks && !tweaksOpen, toggle: toggleTweaks };
  // A pane's chat is the one beside it, which has no «Выделить»: the bar keeps it there.
  const selectOnBar =
    guest !== undefined ||
    pane !== undefined ||
    stage?.folded === true ||
    stage?.mode === 'notes' ||
    (stage?.mode === 'island' && stage.min);
  // The text edited by hand is the page's on screen; looking at the design, there is none.
  const text = texted?.key === frameKey && !looking ? texted.state : null;
  // Picking blocks for notes and writing on the page are two uses of a click: the pick ends the edit.
  const picking = stage?.picking === true;
  const handing = text !== null;
  const endText = useEffectEvent(() => postBoard({ type: 'tk:text-done' }));
  useEffect(() => {
    if (picking && handing) endText();
  }, [picking, handing]);
  /** «Править текст» of a note's card: the board starts the edit on the middle of the picked block. */
  const editTextAt = ({ rect }: IPicked) =>
    postBoard({ type: 'tk:text-at', x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 });
  const closeBoardMenu = useCallback(() => setBoardMenu(null), []);
  const menuParts = (): ICanvasMenuItem[][] => {
    const on = menuShown.value;
    if (on === null) return [];
    const item = (id: string, icon: string, label: string, kbd?: string) => ({ id, icon, label, kbd });
    const block =
      on.board && on.label !== '' && stage && !looking
        ? [
            item('change', 'square-mouse-pointer', t('next.site.ctx.change'), 'V'),
            ...(on.editable && textOn
              ? [item('text', 'pencil', t('next.site.ctx.text'), t('next.site.ctx.twice'))]
              : []),
            item('ask', 'sparkles', t('next.site.ctx.ask')),
          ]
        : [];
    const page = [
      ...(settings.disabled ? [] : [item('tweaks', 'sliders-horizontal', t('next.site.tweaks.title'))]),
      ...(devtools && !looking
        ? [
            item('devtools', 'panel-bottom', t('next.site.devtools')),
            item('inspect', 'code-xml', t('next.site.inspect')),
          ]
        : []),
    ];
    const files = saving
      .filter((row) => row.id === 'page-png' || row.id === 'page-pdf')
      .map((row) =>
        item(row.id, row.icon, t(row.id === 'page-png' ? 'next.site.ctx.png' : 'next.site.ctx.pdf')),
      );
    return [block, page, files];
  };
  const pickFromMenu = (id: string) => {
    const on = menuShown.value;
    if (on === null) return;
    const point = { x: on.x, y: on.y };
    // «Изменить» and «Спросить агента» open the note's card on the block: the card is where the agent is asked.
    if (id === 'change' || id === 'ask') postBoard({ type: 'tk:pick-at', ...point });
    else if (id === 'text') postBoard({ type: 'tk:text-at', ...point });
    else if (id === 'tweaks') {
      if (!settings.on) settings.toggle();
    } else if (id === 'devtools') setDev(true);
    else if (id === 'inspect') {
      if (!dom.inspect) toggleInspect();
    } else saving.find((row) => row.id === id)?.pick();
  };
  const barMenuOf = useCallback(
    () => anchorsRef.current?.querySelector<HTMLElement>('.hx-site-menu') ?? null,
    [],
  );
  useSheetDrag(barMenuOf, pop.close, phone && barMenu);
  // In the pane the section is the pane's own unless its chat is over the design or at its foot.
  const ownMode = guest ? 'view' : (given?.mode ?? 'island');
  const paneMode = pane && !paneOwnChat ? 'pane' : ownMode;
  const termSlot = useContext(ChatTerminalSlotContext);

  return (
    <section
      className="hx-site"
      ref={sectionRef}
      data-mode={paneMode}
      data-variant={pane ? 'pane' : 'site'}
      data-dock={phone ? 'left' : (stage?.dock ?? 'left')}
      data-min={stage?.min ?? false}
      data-folded={given?.folded ?? false}
      data-away={away}
      data-phone={compact}
      data-run={stage?.run ?? 'idle'}
      data-pick={pick}
      data-full={full}
      data-dev={dev}
      data-sview={stage ? stage.sview && stage.mode === 'chat' : false}
      data-handing={handing}
    >
      {given?.chat}
      {!pane && termSlot && <div className="hx-site__term" ref={termSlot} />}
      <div className="hx-site__view">
        <div className="hx-browser">
          {phone && barMenu && <SheetScrim phoneOnly state={pop.state} onClose={pop.close} />}
          <div className="hx-browser__bar" ref={anchorsRef} data-compact={compact}>
            {/* A narrow window shows the design over the chat: the way back to it. */}
            {!pane && !compact && stage && !guest && (
              <button
                type="button"
                className="hx-tochat hx-site__back"
                aria-label={t('next.site.toChat')}
                onClick={() => stage.onSview(false)}
              >
                <Icon name="message-square" size="sm" />
                <span>{t('next.pane.chat')}</span>
              </button>
            )}
            {canBack && (
              <button
                type="button"
                className="hx-head-btn hx-tipped hx-browser__back"
                aria-label={t('next.site.backTo', { name: pageName(backTo) })}
                onClick={goBack}
              >
                <Icon name="arrow-left" />
                <span className="hx-tip" data-pos="below" data-align="start">
                  {t('next.site.backTo', { name: pageName(backTo) })}
                </span>
              </button>
            )}
            {mode && !compact && <ModeButton mode={{ ...mode, versions: undefined }} icons />}
            <div className="hx-browser__addr">
              <button
                type="button"
                className="hx-browser__url"
                aria-haspopup="dialog"
                aria-expanded={pop.pop === POP.pages}
                aria-disabled={pages.length === 0}
                onClick={() => pages.length > 0 && pop.toggle(POP.pages)}
              >
                <span className="hx-browser__page-name">{addrName}</span>
                {title !== addrName && <span className="hx-browser__path">{title}</span>}
                {pages.length > 0 && <Icon name="chevrons-up-down" size="sm" className="hx-browser__chev" />}
                <span className="hx-browser__load" data-on={loading} />
              </button>
              {pop.shownPop === POP.pages && (
                <PagePicker
                  pages={pages}
                  path={pagePath}
                  state={pop.state}
                  place={PICKER_PLACE.start}
                  boards={{
                    changed: chatBoards ?? NO_BOARDS,
                    onNew:
                      onNewBoard &&
                      (() => {
                        pop.close();
                        onNewBoard();
                      }),
                  }}
                  onGo={(to) => {
                    pop.close();
                    onNavigate(to);
                  }}
                  onClose={pop.close}
                  onSave={
                    downloads
                      ? (to, at) => {
                          // The list leaves; the page's downloads open where its button was.
                          pop.close();
                          setPageSaving({ path: to, x: at.right, y: at.bottom, open: true });
                        }
                      : undefined
                  }
                  saveLabel={(p) => t('next.download.pageOf', { name: p.name })}
                />
              )}
            </div>
            {compact && stage && stage.mode === 'chat' && (
              <button
                type="button"
                className="hx-head-btn hx-site__swap"
                aria-label={t('next.site.siteFull')}
                onClick={() => stage.onMode('island', sectionSize())}
              >
                <Icon name="maximize-2" />
              </button>
            )}
            {/* «Выделить» is on the bar only while no field has it: the chat folded, the field in its pill. */}
            {stage && !compact && selectOnBar && (
              <button
                type="button"
                className="hx-head-btn hx-tipped hx-browser__pick"
                aria-pressed={stage.picking ?? false}
                aria-disabled={stage.picking === null}
                aria-label={guest?.pickLabel ?? t('next.site.pick')}
                onClick={() => stage.picking !== null && togglePick()}
              >
                <Icon name="square-mouse-pointer" />
                <span className="hx-tip" data-pos="below">
                  {guest?.pickTip ?? t('next.site.pickTip')}
                  {keys && <kbd>{guest ? 'C' : (pickKey ?? 'V')}</kbd>}
                </span>
              </button>
            )}
            {guest?.full && (
              <button
                type="button"
                className="hx-browser__tool hx-tipped"
                aria-label={guest.full.label}
                onClick={guest.full.onOpen}
              >
                <Icon name="maximize-2" size="sm" />
                <span className="hx-browser__tool-label">{guest.full.label}</span>
                <span className="hx-tip" data-pos="below">
                  {guest.full.tip}
                  <kbd>F</kbd>
                </span>
              </button>
            )}
            <div className="hx-browser__anchor">
              <button
                type="button"
                className="hx-browser__dev hx-tipped"
                aria-expanded={pop.pop === POP.device}
                aria-label={t('next.site.device')}
                onClick={() => pop.toggle(POP.device)}
              >
                <Icon name={device.icon} size="sm" />
                <span className="hx-browser__dev-size">
                  {overview ? t('next.site.devices.overview') : sizeText}
                </span>
                <span className="hx-tip" data-pos="below">
                  {t('next.site.deviceTip')}
                </span>
              </button>
              {pop.shownPop === POP.device && (
                <DeviceMenu
                  state={pop.state}
                  device={device}
                  size={vp && !overview ? { w: vp.vw, h: vp.vh } : null}
                  zoom={canvas}
                  onPick={(id) => {
                    pop.close();
                    pickDevice(id);
                  }}
                  onSize={(next) => {
                    setDeviceId('fill');
                    setRot(false);
                    setSize(next);
                  }}
                />
              )}
            </div>
            {looking && !guest && comments && (
              <button
                type="button"
                className="hx-head-btn hx-tipped hx-browser__comment"
                aria-pressed={comments.on}
                aria-label={t('next.site.comment')}
                onClick={comments.toggle}
              >
                <Icon name="message-square-plus" />
                <span className="hx-tip" data-pos="below">
                  {t('next.site.comment')}
                  <kbd>C</kbd>
                </span>
              </button>
            )}
            {/* Beside a phone the settings are on its pill. */}
            {!compact && !phonePillOn && (
              <button
                type="button"
                className="hx-head-btn hx-tipped hx-browser__opt"
                aria-pressed={settings.on}
                aria-disabled={settings.disabled}
                aria-label={t('next.site.tweaks.title')}
                onClick={settings.toggle}
              >
                <Icon name="sliders-horizontal" />
                <span className="hx-tip" data-pos="below">
                  {settings.disabled ? t('next.site.tweaksNone') : t('next.site.tweaks.title')}
                </span>
              </button>
            )}
            {downloads && pageSaving && (
              <ProjectDownloadMenu
                items={pageDownloads({ ...downloads, board: pageSaving.path }, pageSaving.path, 'page')}
                open={pageSaving.open}
                onOpenChange={(open) => setPageSaving((now) => now && { ...now, open })}
                anchor={{ getBoundingClientRect: () => new DOMRect(pageSaving.x, pageSaving.y, 0, 0) }}
              />
            )}
            {kit}
            {(pane || access) && !phone && (
              <button
                type="button"
                className="hx-head-btn hx-tipped hx-browser__share"
                aria-label={access?.label ?? t('next.pane.designShare')}
                onClick={access ? access.onOpen : pane?.onShare}
              >
                <Icon name="share" />
                <span className="hx-tip" data-pos="below" data-align="end">
                  {access?.label ?? t('next.pane.designShare')}
                </span>
              </button>
            )}
            <div className="hx-browser__anchor">
              <button
                type="button"
                className="hx-head-btn hx-tipped"
                aria-expanded={pop.pop === POP.more || pop.pop === POP.phone}
                aria-label={t('next.site.more')}
                onClick={() => pop.toggle(pop.pop === POP.phone ? POP.phone : POP.more)}
              >
                <Icon name="ellipsis" />
                <span className="hx-tip" data-pos="below" data-align="end">
                  {t('next.site.more')}
                </span>
              </button>
              {pop.shownPop === POP.more && (
                <div
                  className="hx-site-menu hx-site-menu--end hx-more hx-sheet-phone"
                  data-state={pop.state}
                  role="menu"
                >
                  <SheetGrip />
                  {paneChat && (
                    <div className="hx-more__chat">
                      <span className="hx-more__label">{t('next.site.paneChat')}</span>
                      <PaneChatModes
                        mode={paneChat.mode}
                        onMode={(mode) => paneChat.onMode(mode, sectionSize())}
                      />
                    </div>
                  )}
                  {moreItems.map((item) => (
                    <Fragment key={item.id}>
                      {item.sep && <div className="hx-more__sep" />}
                      <button
                        type="button"
                        role={item.on === undefined ? 'menuitem' : 'menuitemcheckbox'}
                        className="hx-option hx-more__item"
                        aria-checked={item.on}
                        aria-expanded={item.sub ? savingOpen : undefined}
                        aria-disabled={item.disabled}
                        onClick={() => !item.disabled && item.pick()}
                      >
                        <Icon name={item.icon} size="sm" />
                        <span className="hx-more__label">{item.label}</span>
                        {keys && <kbd className="hx-more__kbd">{item.kbd}</kbd>}
                        {item.on !== undefined && <Icon name="check" size="sm" className="hx-more__check" />}
                        {item.sub && <Icon name="chevron-down" size="sm" className="hx-more__chev" />}
                      </button>
                      {item.sub && savingOpen && (
                        <div className="hx-more__sub">
                          {item.sub.map((row) => (
                            <button
                              key={row.id}
                              type="button"
                              role="menuitem"
                              className="hx-option hx-more__item"
                              aria-disabled={row.disabled}
                              title={row.hint}
                              onClick={() => !row.disabled && closing(row.pick)()}
                            >
                              <Icon name={row.icon} size="sm" />
                              <span className="hx-more__label">{row.label}</span>
                            </button>
                          ))}
                        </div>
                      )}
                    </Fragment>
                  ))}
                </div>
              )}
              {pop.shownPop === POP.phone && qr && (
                <PhonePopover modules={qr} state={pop.state} onCopy={onCopyLink} />
              )}
            </div>
            {!compact && !guest && (designPanel || toolPane) && (
              <button
                type="button"
                className="hx-head-btn hx-tipped hx-browser__panel"
                aria-pressed={designPanel ? designPanel.tab !== null : toolPane?.open}
                aria-label={t('next.header.pane')}
                onClick={() => (designPanel ? designPanel.open(DESIGN_TAB.tools) : toolPane?.onToggle())}
              >
                <Icon name="panel-right" />
                <span className="hx-tip" data-pos="below" data-align="end">
                  {t('next.site.panelTip')}
                </span>
              </button>
            )}
          </div>

          <div className="hx-browser__page" data-kind="page">
            {text && (
              <div className="hx-textstrip">
                <TextBar text={text} post={postBoard} />
              </div>
            )}
            <div
              ref={areaRef}
              onContextMenu={(event) => {
                // The canvas round the page: the page's menu without a block.
                if (
                  !menuOn ||
                  (event.target as Element).closest('input, textarea, .hx-vpzoom, .hx-phonepill')
                )
                  return;
                event.preventDefault();
                const home = sectionRef.current?.getBoundingClientRect();
                const at = { x: event.clientX - (home?.left ?? 0), y: event.clientY - (home?.top ?? 0) };
                setBoardMenu({ x: 0, y: 0, label: '', editable: false, at, board: false });
              }}
              className="hx-vparea"
              data-dev={vp?.kind ?? VP_KIND.fill}
              data-rot={vp?.rot ?? false}
            >
              {overview && (
                <Overview
                  cards={cards}
                  room={ovwRoom}
                  src={loadedFor === frameKey ? plainSrc : null}
                  guest={guest !== undefined}
                  groups={ovwGroups.groups}
                  onGroup={ovwGroups.toggle}
                  onSpace={canvas.fromPage.space}
                  onPan={canvas.fromPage.pan}
                  onPick={pickDevice}
                />
              )}
              {failed && (
                <LoadFailed
                  size={LOAD_FAILED_SIZE.page}
                  title={failure.title}
                  reason={failure.reason}
                  onRetry={onRetry}
                />
              )}
              {!failed && (
                <div className="hx-vpbox">
                  <div className="hx-vpscale">
                    <div
                      className={
                        skin?.k === 'css'
                          ? `device ${skin.css.cls} ${skin.css.color} hx-dev`
                          : skin?.k === 'own'
                            ? 'hx-dv'
                            : 'hx-bare'
                      }
                      data-kind={skin?.k === 'own' ? skin.case : (device.case ?? 'none')}
                      data-framed={skin?.k === 'own' ? skin.framed : skin !== null}
                      data-fold={folding ?? undefined}
                      data-hinge={device.fold?.hinge}
                      data-turn={turned ? (rot ? 'land' : 'port') : undefined}
                      data-wake={wake ?? undefined}
                    >
                      <div className="device-frame">
                        <div className="device-screen hx-screen">
                          <div
                            className="hx-vp"
                            data-ui={phoneBrowser?.id ?? 'none'}
                            data-os={phoneUi ? device.os : 'none'}
                            data-kind={skin?.k === 'own' ? skin.case : (device.case ?? 'none')}
                            data-status={phoneStatus}
                            data-bars={phoneBrowser ? bars : BARS.full}
                          >
                            {phoneUi && device.os && (
                              <PhoneTop
                                browser={phoneBrowser}
                                os={device.os}
                                status={phoneStatus}
                                host={host}
                                title={title}
                                onUnfold={unfoldBars}
                              />
                            )}
                            <div className="hx-site__frame">
                              <Swap
                                ready={!loading}
                                mounted={src !== null}
                                className="hx-site__fswap"
                                contentClassName="hx-site__held"
                                skeleton={
                                  heldSrc !== null ? (
                                    <SiteHeld src={heldSrc} title={page?.name ?? title} />
                                  ) : (
                                    <SiteLoader stage={loadStage} />
                                  )
                                }
                              >
                                {src !== null && (
                                  <BoardFrames
                                    key={frameScope ?? frameKey}
                                    nextFrame={nextFrame}
                                    capacity={frameScope ? PROTOTYPE_FRAME_CAPACITY : 1}
                                    frameRef={frameRef}
                                    src={src}
                                    title={page?.name ?? title}
                                    ready={() => {
                                      loadedKey.current = frameKey;
                                      setLoadedFor(frameKey);
                                      ping();
                                      hostTweaks();
                                      anchorStore.set(null);
                                      sendAnchors();
                                      onCanvas();
                                      sendTouch();
                                      sendImages();
                                      sendHand();
                                      arm(latestStage.current?.pick === true);
                                      sendStyles();
                                    }}
                                  />
                                )}
                              </Swap>
                              {CommentsLayerSlot && src !== null && (
                                <CommentsLayerSlot
                                  frame={frameRef}
                                  origin={guest ? GUEST_ORIGIN : frameOrigin(src)}
                                />
                              )}
                              {(stage || tourFrame) && (
                                <AnchorContext.Provider value={anchorStore}>
                                  <div className="hx-site__pins">
                                    {stage?.pins}
                                    {tourFrame && <TourAnchor />}
                                  </div>
                                </AnchorContext.Provider>
                              )}
                            </div>
                            {phoneUi && (
                              <PhoneBottom browser={phoneBrowser} host={host} onUnfold={unfoldBars} />
                            )}
                          </div>
                          {skin?.k === 'own' && <ScreenParts />}
                        </div>
                      </div>
                      {skin?.k === 'css' && (
                        <>
                          <div className="device-stripe" />
                          <div className="device-header" />
                          <div className="device-sensors" />
                          <div className="device-btns" />
                          <div className="device-power" />
                          <div className="device-home" />
                        </>
                      )}
                      {skin?.k === 'own' && <CaseParts />}
                    </div>
                  </div>
                  {(['e', 's', 'se'] as const).map((edge) => (
                    <span
                      key={edge}
                      className="hx-vpgrab"
                      data-edge={edge}
                      aria-hidden="true"
                      onPointerDown={(event) => gripViewport(event, edge)}
                    />
                  ))}
                </div>
              )}
              {!failed && <div className="hx-vpshield" aria-hidden="true" />}
            </div>
            {dockShown && !failed && (
              <PaneDock
                pages={pages}
                path={pagePath}
                onGo={onNavigate}
                tweaks={{ on: tweaksOpen, disabled: noTweaks && !tweaksOpen, onToggle: toggleTweaks }}
                zoom={canvas}
              />
            )}
            {!pane && !phone && !failed && <ZoomPill zoom={canvas} />}
            {!phone && (
              <PhonePill
                active={phonePillOn}
                rot={{ on: rot, onToggle: turn }}
                frame={
                  device.case !== undefined && device.case !== 'bare'
                    ? { on: phoneView.frame, onToggle: () => setPhoneView({ frame: !phoneView.frame }) }
                    : null
                }
                status={{ on: phoneView.status, onToggle: () => setPhoneView({ status: !phoneView.status }) }}
                fold={
                  device.fold
                    ? {
                        on: folded,
                        tip: t(folded ? 'next.site.unfold' : `next.site.foldTo.${device.fold.hinge}`),
                        onToggle: fold,
                      }
                    : null
                }
                browser={{
                  id: phoneBrowser?.id ?? BROWSER_NONE,
                  native: device.os ? NATIVE_BROWSER[device.os] : BROWSER_NONE,
                  onPick: (id) => {
                    setBrowserId(id);
                    setChrome(true);
                    unfoldBars();
                  },
                }}
                touch={{ on: phoneView.touch, onToggle: () => setPhoneView({ touch: !phoneView.touch }) }}
                settings={{ on: settings.on, disabled: settings.disabled, onToggle: settings.toggle }}
                watch={`${device.id}|${rot}|${foldedNow}|${phoneView.frame}`}
              />
            )}
          </div>
          {!tweaksPort && tweaksPanel.mounted && tweaks && (
            <SiteTweaks
              controls={tweaks}
              state={tweaksPanel.state}
              pos={tweaksPos}
              onChange={sendTweak}
              onClose={() => setTweaksOpen(false)}
              onGrab={(event) => dragTweaks(event, tweaksPos, setTweaksPos)}
            />
          )}
          {devtools && (
            <SiteDevtools
              machine={devtools.machine}
              slot={devtools.slot}
              artifact={devtools.artifact}
              board={pagePath ?? ''}
              open={dev}
              tab={devTab}
              onTab={setDevTab}
              dom={dom}
              inspect={{ on: dom.inspect, disabled: inspectDisabled, onToggle: toggleInspect }}
              throttle={{
                mounted: pop.shownPop === POP.throttle,
                state: pop.state,
                onToggle: () => pop.toggle(POP.throttle),
                onClose: pop.close,
              }}
              onSend={stage && ((words) => stage.onAttach(words))}
              onClose={toggleDev}
              onGrab={(event) =>
                dragDevtools(event, { h: devH, onHeight: setDevH, onClose: () => setDev(false) })
              }
            />
          )}
        </div>
      </div>
      {stage?.island}
      <BoardTextContext.Provider value={textOn && !looking ? editTextAt : null}>
        {stage?.overlay}
      </BoardTextContext.Provider>
      {menuShown.value !== null && (
        <CanvasMenu
          key={`${menuShown.value.at.x},${menuShown.value.at.y}`}
          x={menuShown.value.at.x}
          y={menuShown.value.at.y}
          state={menuShown.state}
          label={looking ? '' : menuShown.value.label}
          parts={menuParts()}
          onPick={pickFromMenu}
          onClose={closeBoardMenu}
        />
      )}
      {images !== undefined && (
        <ImageActions
          machine={images.machine}
          artifact={images.artifact}
          address={{ file: imageOpen?.act.file ?? '' }}
          open={imagesOn ? imageOpen : null}
          write={fileWriter(images.machine, images.artifact, imageOpen?.act.file ?? '')}
          onClose={() => setImageOpen(null)}
        />
      )}
    </section>
  );
};

export default SiteView;
