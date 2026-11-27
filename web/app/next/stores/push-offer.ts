// The web push of this device as the current interface sees it: its state, mirrored by the push bridge
// (hooks/use-push-bridge.ts), and the one way to turn it on — an event the bridge answers with enablePush. The event is
// dispatched straight from the tap: its listener runs inside the same gesture, which a browser (iOS above all) requires
// for the permission prompt.

export type TPushOfferState = 'unknown' | 'unsupported' | 'needs-install' | 'denied' | 'off' | 'on';

export interface IPushEnableResult {
  state: TPushOfferState;
  error?: string;
  /** Push may not work on this device (Android whose push service did not answer): offer the Telegram bot. */
  telegram?: boolean;
}

/** The id of the Telegram bots' part of Settings → Notifications, where a device without push is sent. */
export const TELEGRAM_BOTS_ID = 'hx-notify-bots';

export const PUSH_ENABLE_EVENT = 'hl:push-enable';

export interface IPushEnableDetail {
  done: (result: IPushEnableResult) => void;
  /** The bridge sets it while handling the event: without it, nobody will answer. */
  taken: boolean;
}

let state: TPushOfferState = 'unknown';
const listeners = new Set<() => void>();

export const pushOfferState = (): TPushOfferState => state;

export const setPushOfferState = (next: TPushOfferState): void => {
  if (next === state) return;
  state = next;
  listeners.forEach((fn) => fn());
};

export const onPushOfferState = (fn: () => void): (() => void) => {
  listeners.add(fn);
  return () => listeners.delete(fn);
};

/** Turns push on for this device; call it straight from a click. Without the bridge it answers the state as it is. */
export const requestPushEnable = (): Promise<IPushEnableResult> =>
  new Promise((resolve) => {
    const detail: IPushEnableDetail = { done: resolve, taken: false };
    window.dispatchEvent(new CustomEvent<IPushEnableDetail>(PUSH_ENABLE_EVENT, { detail }));
    if (!detail.taken) resolve({ state });
  });

const SNOOZE_KEY = 'hl-push-offer-snooze';
/** «Not now» hides the offer for a week. */
export const SNOOZE_MS = 7 * 24 * 60 * 60 * 1000;

export const offerSnoozed = (now = Date.now()): boolean => {
  try {
    const until = Number(localStorage.getItem(SNOOZE_KEY) ?? '0');
    return Number.isFinite(until) && until > now;
  } catch {
    return false;
  }
};

export const snoozeOffer = (now = Date.now()): void => {
  try {
    localStorage.setItem(SNOOZE_KEY, String(now + SNOOZE_MS));
  } catch {
    // A private window keeps nothing: the offer comes back next time.
  }
};

/** iPhone or iPad: the hint for a browser tab says how to add the app to the Home Screen. */
export const isAppleMobile = (): boolean =>
  typeof navigator !== 'undefined' &&
  (/iPhone|iPad|iPod/.test(navigator.userAgent) ||
    (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1));
