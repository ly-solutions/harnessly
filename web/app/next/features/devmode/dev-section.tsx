// Settings → Developer mode: the machine's switch of the connectors' trace. The settings list leaves the
// section out on a phone, and the mode does nothing there: the panel is drawn on a computer only.
import { useState } from 'react';
import { setDevMode, useDevMode } from '@/next/features/devmode/api';
import { Group, SwitchRow } from '@/next/features/settings/rows';
import { t } from '@/next/i18n';
import type { MachineId } from '@/next/protocol/v2';

const DevSection = ({ machine }: { machine: MachineId }) => {
  const on = useDevMode(machine);
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState(false);
  return (
    <Group
      title={t('next.devmode.group')}
      foot={refused ? t('next.devmode.refused') : t('next.devmode.foot')}
    >
      <SwitchRow
        label={t('next.devmode.switch')}
        hint={t('next.devmode.switch.hint')}
        on={on === true}
        busy={busy || on === null}
        onChange={(next) => {
          setBusy(true);
          void setDevMode(machine, next).then((ok) => {
            setBusy(false);
            setRefused(!ok);
          });
        }}
      />
    </Group>
  );
};

export default DevSection;
