// A feature stand is behind the team's sign-in. The slot answers with its own wall instead of following that chain,
// and the wall asks the parent (the pane) for «Войти» through the bridge: tk:ping → tk:sign-in. The sign-in is a real
// visit to the stand's start address, in a tab of the browser; coming back reopens the stand with the session it has.
import { useEffect, useRef, useState, type RefObject } from 'react';
import { appConfig } from '@/platform/config';

export const SIGN_IN = { none: 'none', asked: 'asked', waiting: 'waiting' } as const;
export type TSignIn = (typeof SIGN_IN)[keyof typeof SIGN_IN];

/** The wall answers late on a slow stand: the ping is repeated at these delays after a load. */
export const PING_DELAYS_MS = [300, 1_000, 2_500] as const;

const OAUTH2_PATH = '/oauth2/';
const LABEL = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;
const NOT_STANDS = /^(?:ai-|auth$)/;

/** The address that signs a person in to the stand and brings them back to the page; null when it is not a stand. */
export const standSignInUrl = (origin: string, path: string): string | null => {
  const domain = appConfig().standDomain;
  let page: URL;
  try {
    page = new URL(`${origin}${path}`);
  } catch {
    return null;
  }
  if (!domain || page.protocol !== 'https:' || page.port || !page.hostname.endsWith(domain)) return null;
  const label = page.hostname.slice(0, -domain.length);
  if (!LABEL.test(label) || NOT_STANDS.test(label)) return null;
  const start = new URL(`${OAUTH2_PATH}start`, page.origin);
  start.searchParams.set(
    'rd',
    page.pathname.startsWith(OAUTH2_PATH) ? page.origin : `${page.origin}${page.pathname}${page.search}`,
  );
  return start.href;
};

interface IInstalledCheck {
  matchMedia: (query: string) => { matches: boolean };
  navigator: object;
}

/** An installed app keeps its own cookies: the sign-in must happen in its window, a tab of the browser would not fill them. */
export const isInstalledApp = (win: IInstalledCheck): boolean =>
  (win.navigator as { standalone?: boolean }).standalone === true ||
  win.matchMedia('(display-mode: standalone)').matches;

interface IStandSignInArgs {
  frame: RefObject<HTMLIFrameElement | null>;
  /** The slot's origin, the only one whose messages count; null before the slot is open. */
  origin: string | null;
  /** The stand's own sign-in address for this page; null when the page is not a stand. */
  signInUrl: string | null;
  /** Opens the page again (the person is back). */
  onReload: () => void;
}

export const useStandSignIn = ({ frame, origin, signInUrl, onReload }: IStandSignInArgs) => {
  const [state, setState] = useState<TSignIn>(SIGN_IN.none);
  const pings = useRef<number[]>([]);

  const stopPings = () => {
    for (const timer of pings.current) window.clearTimeout(timer);
    pings.current = [];
  };
  const ping = () => {
    if (origin !== null) frame.current?.contentWindow?.postMessage({ type: 'tk:ping' }, origin);
  };

  useEffect(() => {
    if (origin === null) return;
    const onMessage = (event: MessageEvent) => {
      const known = frame.current?.contentWindow ?? null;
      if (known === null || event.source !== known || event.origin !== origin) return;
      const data: unknown = event.data;
      if (typeof data === 'object' && data !== null && (data as { type?: unknown }).type === 'tk:sign-in')
        setState(signInUrl ? SIGN_IN.asked : SIGN_IN.none);
    };
    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
  }, [frame, origin, signInUrl]);

  useEffect(() => stopPings, []);

  const reopen = () => {
    setState(SIGN_IN.none);
    onReload();
  };
  useEffect(() => {
    if (state !== SIGN_IN.waiting) return;
    let done = false;
    const back = () => {
      if (done || document.visibilityState !== 'visible') return;
      done = true;
      setState(SIGN_IN.none);
      onReload();
    };
    window.addEventListener('focus', back);
    document.addEventListener('visibilitychange', back);
    return () => {
      window.removeEventListener('focus', back);
      document.removeEventListener('visibilitychange', back);
    };
  }, [state, onReload]);

  return {
    state,
    /** The frame loaded a document: it is asked whether it is the wall. */
    onFrameLoad: () => {
      stopPings();
      setState(SIGN_IN.none);
      ping();
      pings.current = PING_DELAYS_MS.map((delay) => window.setTimeout(ping, delay));
    },
    /** «Войти»: in a tab of the browser (then the pane waits for the person), or in this window of an installed app. */
    start: () => {
      if (!signInUrl) return;
      if (isInstalledApp(window)) {
        window.location.assign(signInUrl);
        return;
      }
      // noopener: the sign-in pages, and the stand it ends on, get no handle on the app.
      window.open(signInUrl, '_blank', 'noopener');
      setState(SIGN_IN.waiting);
    },
    refresh: reopen,
  };
};
