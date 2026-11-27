// «Этот компьютер» in Settings → Machines: which of the person's machines runs on the computer this browser
// is on. Paired, the app opens on it without a network. Not yet: find it among the machines (the one that answers on
// this computer's loopback), or take the one install command, after which the app opens and pairs by itself.
import { useState } from 'react';
import { appConfig } from '@/platform/config';
import CopyCommand from '@/next/features/settings/copy-command';
import { Group, Note, SetButton } from '@/next/features/settings/rows';
import { t } from '@/next/i18n';
import {
  findThisComputer,
  forgetThisComputer,
  openThisComputer,
  useThisComputer,
} from '@/next/offline/this-computer';
import type { MachineId } from '@/next/protocol/v2';

interface IThisComputerCardProps {
  /** The person's machines that answer now, by id and name. */
  machines: readonly { id: MachineId; name: string }[];
}

/** The hub's one install command for a new machine of the person (null without a hub or a space). */
async function installCommand(hub: string): Promise<string | null> {
  try {
    const me = (await (await fetch(`${hub}/api/me`, { credentials: 'same-origin' })).json()) as {
      spaces?: { id: string }[];
    };
    const space = me.spaces?.[0]?.id;
    if (!space) return null;
    const res = await fetch(`${hub}/api/spaces/${encodeURIComponent(space)}/machines/codes`, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ profile: 'full', confirmFull: true }),
    });
    if (!res.ok) return null;
    const body = (await res.json()) as { command?: string };
    return typeof body.command === 'string' ? body.command : null;
  } catch {
    return null;
  }
}

type TFind = 'idle' | 'busy' | 'none';
type TCommand = { k: 'idle' } | { k: 'busy' } | { k: 'ready'; command: string } | { k: 'failed' };

const ThisComputerCard = ({ machines }: IThisComputerCardProps) => {
  const paired = useThisComputer();
  const [find, setFind] = useState<TFind>('idle');
  const [command, setCommand] = useState<TCommand>({ k: 'idle' });
  const hub = appConfig().hubUrl;
  const name = paired ? (machines.find((m) => m.id === paired.machine)?.name ?? paired.machine) : null;

  return (
    <Group title={t('next.thisComputer.title')}>
      {paired ? (
        <>
          <Note>{t('next.thisComputer.on', { machine: name ?? '' })}</Note>
          <div className="hx-check__acts hx-tc__acts">
            <SetButton tone="ghost" icon="laptop" onClick={() => openThisComputer()}>
              {t('next.thisComputer.open')}
            </SetButton>
            <SetButton tone="ghost" icon="x" onClick={() => forgetThisComputer()}>
              {t('next.thisComputer.forget')}
            </SetButton>
          </div>
        </>
      ) : (
        <>
          <Note>{t('next.thisComputer.off')}</Note>
          <div className="hx-check__acts hx-tc__acts">
            <SetButton
              tone="ghost"
              icon="search"
              busy={find === 'busy'}
              result={find === 'none' ? 'failed' : undefined}
              disabled={machines.length === 0}
              onClick={() => {
                setFind('busy');
                void findThisComputer(machines.map((m) => m.id)).then((m) => setFind(m ? 'idle' : 'none'));
              }}
            >
              {find === 'none' ? t('next.thisComputer.notHere') : t('next.thisComputer.find')}
            </SetButton>
            {hub && command.k !== 'ready' && (
              <SetButton
                tone="primary"
                icon="terminal"
                busy={command.k === 'busy'}
                onClick={() => {
                  setCommand({ k: 'busy' });
                  void installCommand(hub).then((c) =>
                    setCommand(c ? { k: 'ready', command: c } : { k: 'failed' }),
                  );
                }}
              >
                {t('next.thisComputer.getCommand')}
              </SetButton>
            )}
          </div>
          {command.k === 'ready' && (
            <>
              <Note>{t('next.thisComputer.install')}</Note>
              <CopyCommand command={command.command} />
            </>
          )}
          {command.k === 'failed' && <Note>{t('next.thisComputer.noCommand')}</Note>}
        </>
      )}
    </Group>
  );
};

export default ThisComputerCard;
