// The proposals of the agents to change a setting, for the chat on screen: the machine's list is read when
// a chat opens and when the machine says it moved (bindings/settings-watch.ts), and the proposal of this chat's agent stands over its field
// until the person says yes or no. The agent waits on its side; a proposal that ran out of time leaves by itself.
import { useCallback, useEffect, useState } from 'react';
import { useSettingsTick } from '@/next/bindings/settings-watch';
import { refusalOf } from '@/next/bindings/settings-changes';
import {
  answerSettingProposal,
  loadSettingProposals,
  type ISettingProposal,
  type TProposalVerb,
} from '@/next/features/settings/changes-api';
import { isRefused } from '@/next/features/settings/platform-answer';
import ProposalCard from '@/next/features/settings/proposal-card';
import type { MachineId } from '@/next/protocol/v2';
import { usePresence } from '@/next/ui/presence';

/** The card's exit, as long as hx-setask-out in settings-changes.css. */
const CARD_EXIT_MS = 180;

const NONE: readonly ISettingProposal[] = [];

interface IProposals {
  list: readonly ISettingProposal[];
  /** Leaves an answered proposal out at once and reads the list again. */
  settle: (id: string) => void;
}

/** The proposals of a machine that wait for the person: read when a chat opens and when the machine says they moved. */
const useProposals = (machine: MachineId): IProposals => {
  const tick = useSettingsTick(machine);
  const [read, setRead] = useState<{ machine: MachineId; list: readonly ISettingProposal[] } | null>(null);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    if (document.visibilityState !== 'visible') return;
    let stale = false;
    void loadSettingProposals(machine).then((res) => {
      if (stale || !('ok' in res)) return;
      const value = res.value;
      if (!isRefused(value)) setRead({ machine, list: value });
    });
    return () => {
      stale = true;
    };
  }, [machine, tick, attempt]);
  const settle = useCallback(
    (id: string) => {
      setRead((was) =>
        was?.machine === machine ? { machine, list: was.list.filter((p) => p.id !== id) } : was,
      );
      setAttempt((n) => n + 1);
    },
    [machine],
  );
  return { list: read?.machine === machine ? read.list : NONE, settle };
};

interface ISettingsProposalsProps {
  machine: MachineId;
  /** The chat: only what its agent proposed is asked here. */
  conversation: string;
}

export const SettingsProposals = ({ machine, conversation }: ISettingsProposalsProps) => {
  const proposals = useProposals(machine);
  const proposal = proposals.list.find((p) => p.conv === conversation) ?? null;
  const card = usePresence(proposal, CARD_EXIT_MS);

  const onAnswer = async (asked: ISettingProposal, verb: TProposalVerb): Promise<string | null> => {
    const refused = refusalOf(await answerSettingProposal(machine, asked.id, verb));
    // Answered, or gone already (answered on another page, out of time): the list read again says which.
    proposals.settle(refused === null ? asked.id : '');
    return refused;
  };

  if (card.value === null) return null;
  return (
    <ProposalCard
      key={card.value.id}
      proposal={card.value}
      closing={card.state === 'closing'}
      onAnswer={onAnswer}
    />
  );
};
