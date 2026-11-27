// The offer to turn notifications on for this device, in the chat: one tap asks the browser and subscribes
// (a browser lets a page ask only from a tap). It stands in a chat while this device gets no push from the machine:
// on its own until «Not now» (a week), and again when the chat's agent asks (harnessly push ask) or proposes a send
// while no device would hear of it. A tab of Safari on an iPhone cannot subscribe: it is told how to install the app.
import { useState, useSyncExternalStore } from 'react';
import { t } from '@/next/i18n';
import {
  isAppleMobile,
  offerSnoozed,
  onPushOfferState,
  pushOfferState,
  requestPushEnable,
  snoozeOffer,
} from '@/next/stores/push-offer';
import PushTelegramLink from '@/next/bindings/push-telegram';
import { Button } from '@/next/ui/button';
import '@/next/styles/settings-changes.css';

const usePushOfferState = () =>
  useSyncExternalStore(onPushOfferState, pushOfferState, () => 'unknown' as const);

interface IPushOfferProps {
  /** Why it stands now beyond the plain offer: the agent asked (its words) or a send waits; '' for the plain offer. */
  reason: string;
  /** The words of the agent that asked, if it did. */
  why: string;
  /** A send of this chat waits for the person's yes and no device would hear of it. */
  forSend: boolean;
}

export const PushOffer = ({ reason, why, forSend }: IPushOfferProps) => {
  const state = usePushOfferState();
  const [hiddenFor, setHiddenFor] = useState<string | null>(null);
  const [snoozed, setSnoozed] = useState(offerSnoozed);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const [telegram, setTelegram] = useState(false);

  if (state !== 'off' && state !== 'needs-install' && state !== 'denied') return null;
  if (hiddenFor === reason || (reason === '' && snoozed)) return null;

  const notNow = () => {
    if (reason === '') {
      snoozeOffer();
      setSnoozed(true);
    }
    setHiddenFor(reason);
  };
  const enable = () => {
    if (busy) return;
    setBusy(true);
    setFailure(null);
    // Straight from the tap: the bridge's listener runs inside this click.
    void requestPushEnable().then((result) => {
      setBusy(false);
      if (result.state !== 'on' && result.state !== 'denied')
        setFailure(result.error ?? t('next.connectors.push.failed'));
      setTelegram(result.telegram === true);
    });
  };

  const lead = why || (forSend ? t('next.connectors.push.forSend') : t('next.connectors.push.lead'));
  let hint = '';
  if (state === 'needs-install') hint = t('next.connectors.push.install');
  else if (state === 'denied')
    hint = isAppleMobile() ? t('next.connectors.push.deniedApple') : t('next.connectors.push.denied');

  return (
    <section className="hx-setask" aria-label={t('next.connectors.push.aria')}>
      <div className="hx-setask__text">
        <span className="hx-setask__lead">{lead}</span>
        <strong className="hx-setask__title">{t('next.connectors.push.title')}</strong>
        {hint !== '' && <span className="hx-setask__why">{hint}</span>}
      </div>
      {failure && (
        <p className="hx-setask__fail" role="alert">
          {failure}
        </p>
      )}
      {failure && telegram && <PushTelegramLink />}
      <div className="hx-setask__acts">
        <Button size="sm" variant="ghost" disabled={busy} onClick={notNow}>
          {t('next.connectors.push.notNow')}
        </Button>
        {state === 'off' && (
          <Button size="sm" variant="primary" busy={busy} disabled={busy} onClick={enable}>
            {t('next.connectors.push.enable')}
          </Button>
        )}
      </div>
    </section>
  );
};
