// Settings → Notifications on the browser's own switches (stores/notices.ts) and its permission for notifications of
// the system, asked when the person turns that switch on; «Показать пример» puts a sample card in the pile.
import { useState } from 'react';
import NoticesSection from '@/next/features/settings/notices-section';
import { SYSTEM_PERMISSION, askSystemPermission, systemPermission } from '@/next/features/notifications/ping';
import { t } from '@/next/i18n';
import { pushNotice, SAMPLE_CHAT, setNotify, useNotify } from '@/next/stores/notices';

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

export const NoticesPage = () => {
  const settings = useNotify();
  const [permission, setPermission] = useState(systemPermission);
  return (
    <NoticesSection
      settings={settings}
      permission={permission}
      onKind={(kind, on) => setNotify({ kinds: { [kind]: on } })}
      onSound={(sound) => setNotify({ sound })}
      onCards={(cards) => setNotify({ cards })}
      onPlace={(place) => setNotify({ place })}
      onHideAfter={(hideAfter) => setNotify({ hideAfter })}
      onSample={showSample}
      onSystem={(on) => {
        if (!on) {
          setNotify({ system: false });
          return;
        }
        void askSystemPermission().then((answer) => {
          setPermission(answer);
          setNotify({ system: answer === SYSTEM_PERMISSION.granted });
        });
      }}
    />
  );
};
