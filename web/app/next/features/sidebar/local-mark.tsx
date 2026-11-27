// The app on this computer's loopback: beside the workspace's name a laptop says the page works here,
// without the hub; once the hub answers again, the way back to the same place there.
import { useEffect, useRef, useState } from 'react';
import { t } from '@/next/i18n';
import { backToHubUrl, hubAnswers, localBack } from '@/next/offline/loopback';
import OfflineMark from '@/next/ui/offline-mark';
import Icon from '@/next/ui/icon';
import Tooltip from '@/next/ui/tooltip';
import { toast } from '@/next/ui/toast';

/** How often the loopback page asks whether the hub is back, ms. */
const HUB_PROBE_MS = 15_000;

const LocalMark = () => {
  const [back] = useState(localBack);
  const [hub, setHub] = useState(false);
  useEffect(() => {
    if (!back) return;
    let alive = true;
    const ask = () => void hubAnswers(back).then((ok) => alive && setHub(ok));
    ask();
    const timer = setInterval(ask, HUB_PROBE_MS);
    window.addEventListener('online', ask);
    return () => {
      alive = false;
      clearInterval(timer);
      window.removeEventListener('online', ask);
    };
  }, [back]);
  // Said once when the hub comes back; the button beside the name stays.
  const told = useRef(false);
  useEffect(() => {
    if (!hub || !back || told.current) return;
    told.current = true;
    toast(t('next.thisComputer.hubBack'), {
      tone: 'success',
      action: { label: t('next.thisComputer.backToHub'), onClick: () => location.assign(backToHubUrl(back)) },
    });
  }, [hub, back]);
  if (!back) return null;
  return (
    <>
      <OfflineMark label={t('next.thisComputer.local')} tone="chip" icon="laptop" />
      {hub && (
        <Tooltip label={t('next.thisComputer.backToHub')}>
          <button
            type="button"
            className="hx-btn hx-local-back"
            data-variant="ghost"
            data-size="sm"
            data-shape="icon"
            aria-label={t('next.thisComputer.backToHub')}
            onClick={() => location.assign(backToHubUrl(back))}
          >
            <Icon name="arrow-up-right" size="sm" />
          </button>
        </Tooltip>
      )}
    </>
  );
};

export default LocalMark;
