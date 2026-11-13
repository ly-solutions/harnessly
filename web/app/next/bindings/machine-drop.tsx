// A machine's card in the stack under the chat's column, opened in place: reads the machine's figures while
// it is open and shows what weighs on it; a program is stopped from here, as from the smart notice's open card.
import { useState } from 'react';
import { machineDetail, useMachineStats } from '@/next/bindings/smart-notice';
import { procKey, stopProcess } from '@/next/features/pane/machine/api';
import MachineMore from '@/next/features/smart-notice/machine-more';
import type { ISmartMachine } from '@/next/features/smart-notice/smart-notice';
import { t } from '@/next/i18n';
import type { MachineId } from '@/next/protocol/v2';
import { toast } from '@/next/ui/toast';

const MachineDrop = ({ machine }: { machine: ISmartMachine }) => {
  const id = machine.id as MachineId;
  // A stall is told by the machine's own verdict; a full disk is nobody's: neither needs the figures of programs.
  const figures = machine.metric === 'mem' || machine.metric === 'cpu';
  const stats = useMachineStats(figures ? id : null);
  const [stopping, setStopping] = useState<string | null>(null);
  const detail = machineDetail(machine.metric, stats, machine.waiting);
  if (detail.k !== 'machine') return null;
  const onStop = (heavyKey: string) => {
    const proc = stats?.procs.find((p) => procKey(p) === heavyKey);
    if (!proc) return;
    setStopping(heavyKey);
    stopProcess(id, proc)
      .then(() => toast(t('next.smart.machine.stopped'), { description: proc.name }))
      .catch((error: unknown) =>
        toast(t('next.smart.machine.stopFailed'), {
          tone: 'error',
          description: error instanceof Error ? error.message : undefined,
        }),
      )
      .finally(() => setStopping(null));
  };
  return <MachineMore detail={detail} stopping={stopping} onStop={onStop} />;
};

export default MachineDrop;
