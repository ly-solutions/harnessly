// Settings → Notifications on the browser's own switches (stores/notices.ts) and its permission for notifications of
// the system, asked when the person turns that switch on; «Показать пример» puts a sample on the smart notice's place,
// ▶ plays a sound.
import { useState } from 'react';
import PushTelegramLink from '@/next/bindings/push-telegram';
import { useKeyHint } from '@/next/features/keys/keys-help';
import NoticesSection, { type TNoticeShow } from '@/next/features/settings/notices-section';
import {
  SYSTEM_PERMISSION,
  askSystemPermission,
  playSound,
  systemPermission,
} from '@/next/features/notifications/ping';
import { t } from '@/next/i18n';
import { pushNotice, SAMPLE_CHAT, setNotify, useNotify } from '@/next/stores/notices';
import { pushOfferState, requestPushEnable } from '@/next/stores/push-offer';

const showSample = () => {
  const at = Date.now();
  pushNotice({
    key: `${SAMPLE_CHAT}:${at}`,
    chatId: SAMPLE_CHAT,
    kind: 'done',
    icon: 'circle-check',
    title: t('next.notices.settings.sample.title'),
    text: t('next.notices.settings.sample.text'),
    at,
  });
};

/** A switch of «Что показывать» as the settings it changes. */
const showPatch = (show: TNoticeShow, on: boolean): Parameters<typeof setNotify>[0] => {
  if (show === 'broken') return { kinds: { failed: on, limit: on } };
  if (show === 'machine') return { machine: on };
  return { kinds: { [show]: on } };
};

export const NoticesPage = () => {
  const settings = useNotify();
  const [permission, setPermission] = useState(systemPermission);
  const [pushFailure, setPushFailure] = useState<{ text: string; telegram: boolean } | null>(null);
  const noticeKey = useKeyHint('global.notice');
  return (
    <>
      <NoticesSection
        settings={settings}
        permission={permission}
        noticeKey={noticeKey}
        onShow={(show, on) => setNotify(showPatch(show, on))}
        onSummary={(summary) => setNotify({ summary })}
        onSound={(sound) => setNotify({ sound })}
        onSoundOf={(sound, on) => setNotify({ sounds: { [sound]: on } })}
        onListen={playSound}
        onPlace={(place) => setNotify({ place })}
        onSample={showSample}
        onSystem={(on) => {
          if (!on) {
            setNotify({ system: false });
            return;
          }
          // The same tap also subscribes this device to the machine's web push: its subscribe() asks the
          // permission itself, and without it a phone gets nothing while the page is closed.
          setPushFailure(null);
          const asked =
            pushOfferState() === 'off'
              ? requestPushEnable().then((result) => {
                  if (result.state !== 'on' && result.state !== 'denied' && result.error)
                    setPushFailure({ text: result.error, telegram: result.telegram === true });
                  return systemPermission();
                })
              : askSystemPermission();
          void asked.then((answer) => {
            setPermission(answer);
            setNotify({ system: answer === SYSTEM_PERMISSION.granted });
          });
        }}
      />
      {pushFailure && (
        <div className="hx-setask__fail" role="alert">
          <p>{pushFailure.text}</p>
          {pushFailure.telegram && <PushTelegramLink />}
        </div>
      )}
    </>
  );
};
