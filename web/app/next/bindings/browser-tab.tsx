// One browser tab of the pane: the page in a preview slot of the machine, asked for again on every address and reload.
import { useEffect, useRef, useState } from 'react';
import BrowserView from '@/next/features/pane/browser-view';
import { fetchPorts, openPreview, slotNonce, type IListeningPort } from '@/next/core/net/preview';
import { neighboursOf } from '@/next/features/pane/neighbours';
import { slotLeases } from '@/next/features/pane/slot-lease';
import { useTheme } from '@/platform/theme';
import { standSignInUrl, useStandSignIn } from '@/next/features/pane/stand-sign-in';
import { useBrowserPins } from '@/next/stores/browser-pins';
import { worthRemembering } from '@/next/features/pane/pins';
import { useMe, personKey, type MeState } from '@/next/stores/me';
import { LOCAL_PERSON } from '@/next/core/persist';
import { useLinkRestored } from '@/next/stores/connection';
import { t } from '@/next/i18n';
import type { Conversation } from '@/next/protocol/v2';
import type { TTab } from '@/next/features/pane/pane-tabs';

interface IBrowserTabProps {
  conv: Conversation;
  tab: Extract<TTab, { kind: 'browser' }>;
  onChange: (tab: Extract<TTab, { kind: 'browser' }>) => void;
  onNewTab: () => void;
}

/** Whose pins these are: the account's, the local person's when nobody signs in; unknown while it loads or failed. */
const personOf = (me: MeState): string | null => {
  if (me.k === 'ready') return personKey(me.me);
  return me.k === 'signed-out' ? LOCAL_PERSON : null;
};

/** One browser tab: the page in its slot, asked for again on every address and reload. */
const BrowserTab = ({ conv, tab, onChange, onNewTab }: IBrowserTabProps) => {
  const [reloadKey, setReloadKey] = useState(0);
  // A reload asked from outside the tab (the agent published the page again) is one more press of «Обновить».
  const asked = tab.reload ?? 0;
  const [heard, setHeard] = useState(asked);
  if (asked !== heard) {
    setHeard(asked);
    setReloadKey((n) => n + 1);
  }
  const [slot, setSlot] = useState<{
    key: string;
    src: string | null;
    error: string | null;
    slot: number | null;
    nonce: string | null;
    origin: string | null;
  } | null>(null);
  const [ports, setPorts] = useState<IListeningPort[] | null>(null);
  const [portsError, setPortsError] = useState<string | null>(null);
  const [portsAttempt, setPortsAttempt] = useState(0);
  const restored = useLinkRestored(conv.machineId);
  const { resolvedTheme } = useTheme();
  const me = useMe();
  const person = personOf(me);
  const pins = useBrowserPins(person);
  const at = tab.hist[tab.hi] ?? null;
  const frame = useRef<HTMLIFrameElement>(null);
  const key = at ? `${at.text}|${reloadKey}` : '';

  useEffect(() => {
    if (!at) return;
    let live = true;
    const { target } = at;
    const asked = `${at.text}|${reloadKey}`;
    const failed = (error: string) =>
      setSlot({ key: asked, src: null, error, slot: null, nonce: null, origin: null });
    // Every tab holds a slot of its own: a second tab on the same one would take the first one's page over.
    const slotNo = slotLeases.lease(target.kind === 'port' ? 'port' : 'site', tab.id);
    // The machine's servers say whether a port is down and which other servers the page leans on (an MFE's remotes).
    const listing =
      target.kind === 'port'
        ? fetchPorts(conv.machineId, [conv.cwd]).then(
            (list) => list,
            () => null,
          )
        : Promise.resolve(null);
    listing
      .then((list) => {
        if (!live) return;
        if (target.kind === 'port' && list !== null && !list.some((p) => p.port === target.port)) {
          failed(t('next.pane.portDown', { port: target.port }));
          return;
        }
        const neighbours = target.kind === 'port' && list !== null ? neighboursOf(list, target.port) : [];
        return openPreview(conv.machineId, target, slotNo, at.path, neighbours).then((opened) => {
          if (live)
            setSlot({
              key: asked,
              src: opened.enterUrl,
              error: null,
              slot: slotNo,
              nonce: opened.nonce,
              origin: opened.origin,
            });
        });
      })
      .catch((failure: Error) => live && failed(failure.message));
    return () => {
      live = false;
    };
  }, [conv.machineId, conv.cwd, tab.id, at, reloadKey]);

  // The tab's slot went to a tab used less recently; it asks again, and gets another.
  useEffect(
    () =>
      slotLeases.onTaken((taken) => {
        if (taken === tab.id) setReloadKey((n) => n + 1);
      }),
    [tab.id],
  );

  // Another window (or the older panels) opened the slot: its cookie is not ours any more.
  const heldSlot = slot?.slot ?? null;
  const heldNonce = slot?.nonce ?? null;
  useEffect(() => {
    if (heldSlot === null) return;
    const check = () => {
      const now = slotNonce(heldSlot);
      if (now !== null && now !== heldNonce) setReloadKey((n) => n + 1);
    };
    window.addEventListener('storage', check);
    return () => window.removeEventListener('storage', check);
  }, [heldSlot, heldNonce]);

  // The servers are asked for on every tab: a new tab lists them, the omnibox suggests them.
  useEffect(() => {
    const abort = new AbortController();
    fetchPorts(conv.machineId, [conv.cwd], abort.signal).then(
      (list) => {
        const shown = list.filter((p) => p.responding && p.kind === 'site');
        setPorts([...shown.filter((p) => p.group === 'chat'), ...shown.filter((p) => p.group !== 'chat')]);
        setPortsError(null);
      },
      (failure: Error) => {
        if (!abort.signal.aborted) setPortsError(failure.message);
      },
    );
    return () => abort.abort();
  }, [conv.machineId, conv.cwd, portsAttempt, restored]);

  const stand = useStandSignIn({
    frame,
    origin: slot?.origin ?? null,
    signInUrl: at?.target.kind === 'url' ? standSignInUrl(at.target.origin, at.path) : null,
    onReload: () => setReloadKey((n) => n + 1),
  });

  return (
    <BrowserView
      address={at?.text ?? ''}
      canBack={tab.hi > 0}
      canForward={tab.hi < tab.hist.length - 1}
      src={slot?.key === key ? slot.src : null}
      error={slot?.key === key ? slot.error : null}
      ports={ports}
      portsError={ports === null ? portsError : null}
      onRetryPorts={() => {
        setPortsError(null);
        setPortsAttempt((n) => n + 1);
      }}
      onWantPorts={() => setPortsAttempt((n) => n + 1)}
      title={at?.title ?? null}
      pins={pins?.pins ?? null}
      visits={pins?.visits ?? []}
      onPin={(pin) => pins?.pin(pin)}
      onUnpin={(text) => pins?.unpin(text)}
      reloadKey={reloadKey}
      onFrameLoad={(el) => {
        frame.current = el;
        stand.onFrameLoad();
      }}
      signIn={stand.state}
      onSignIn={stand.start}
      onSignInRefresh={stand.refresh}
      theme={resolvedTheme}
      onGo={(next) => {
        if (worthRemembering(next)) pins?.visit(next.text);
        onChange({ ...tab, hist: [...tab.hist.slice(0, tab.hi + 1), next], hi: tab.hi + 1 });
      }}
      onHistory={(step) =>
        onChange({ ...tab, hi: Math.max(0, Math.min(tab.hist.length - 1, tab.hi + step)) })
      }
      onReload={() => setReloadKey((n) => n + 1)}
      onNewTab={onNewTab}
    />
  );
};

export default BrowserTab;
