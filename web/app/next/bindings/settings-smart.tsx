// Settings → «Умные функции» on the machine the person has open: the features with whose model does each,
// read when the page opens and again whenever the journal says one of them changed (an agent's change, a change taken
// back). A pick moves its row only when the machine has kept it.
import { useEffect, useState } from 'react';
import { refusalOf, type ISettingChangesHandle } from '@/next/bindings/settings-changes';
import { ChangesGroup } from '@/next/features/settings/changed-mark';
import { isRefused } from '@/next/features/settings/platform-answer';
import { loadSmart, pickSmart, type ISmartChoice, type ISmartView } from '@/next/features/settings/smart-api';
import SmartSection, { smartKey, type TSmartState } from '@/next/features/settings/smart-section';
import type { MachineId } from '@/next/protocol/v2';
import { goTo, ROUTE } from '@/next/routes/routes';
import { toast } from '@/next/ui/toast';

type TLoad = { k: 'loading' } | { k: 'failed'; text: string } | { k: 'ready'; view: ISmartView };

interface ISmartPageProps {
  machine: MachineId;
  changes: ISettingChangesHandle;
  now: number;
}

export const SmartPage = ({ machine, changes, now }: ISmartPageProps) => {
  const [box, setBox] = useState<{ machine: MachineId; load: TLoad }>({ machine, load: { k: 'loading' } });
  const [attempt, setAttempt] = useState(0);
  const [busy, setBusy] = useState<ReadonlySet<string>>(new Set());
  const stamp = changes.stampOf(smartKey(''));

  useEffect(() => {
    let live = true;
    void loadSmart(machine).then((res) => {
      if (!live) return;
      const refused = refusalOf(res);
      if (refused === null && 'ok' in res && !isRefused(res.value))
        setBox({ machine, load: { k: 'ready', view: res.value } });
      else
        setBox((was) =>
          was.machine === machine && was.load.k === 'ready'
            ? was
            : { machine, load: { k: 'failed', text: refused ?? '' } },
        );
      setBusy(new Set());
    });
    return () => {
      live = false;
    };
  }, [machine, attempt, stamp]);

  const load: TLoad = box.machine === machine ? box.load : { k: 'loading' };

  const onPick = async (feature: string, choice: ISmartChoice | null) => {
    setBusy((set) => new Set(set).add(feature));
    const refused = refusalOf(await pickSmart(machine, feature, choice));
    if (refused !== null) {
      toast(refused, { tone: 'error' });
      setBusy((set) => new Set([...set].filter((id) => id !== feature)));
      return;
    }
    // The row is read from the machine again, and the journal has a new line.
    setAttempt((n) => n + 1);
    changes.refresh();
  };

  let state: TSmartState = { k: 'loading' };
  if (load.k === 'ready') state = load;
  else if (load.k === 'failed')
    state = {
      k: 'failed',
      text: load.text,
      onRetry: () => {
        setBox({ machine, load: { k: 'loading' } });
        setAttempt((n) => n + 1);
      },
    };

  return (
    <>
      <SmartSection
        state={state}
        busy={busy}
        marks={changes.marks}
        onPick={(feature, choice) => void onPick(feature, choice)}
        onOpenProviders={() => goTo({ kind: ROUTE.settings, section: 'accounts' })}
      />
      <ChangesGroup journal={changes.journal} now={now} />
    </>
  );
};
