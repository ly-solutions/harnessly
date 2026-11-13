// The proposals of the agents to change a setting, for the chat on screen: the machine's list is read every
// few seconds while a chat is open and the page is visible, and the proposal of this chat's agent stands over its field
// until the person says yes or no. The agent waits on its side; a proposal that ran out of time leaves by itself.
import { useCallback, useSyncExternalStore } from 'react';
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

export const PROPOSALS_POLL_MS = 4000;
/** A machine that did not answer (an older runtime has no such address) is asked again only now and then. */
const QUIET_POLL_MS = 60_000;
/** The card's exit, as long as hx-setask-out in settings-changes.css. */
const CARD_EXIT_MS = 180;

interface IEntry {
  proposals: readonly ISettingProposal[];
  listeners: Set<() => void>;
  timer: ReturnType<typeof setTimeout> | null;
  inFlight: boolean;
}

const entries = new Map<MachineId, IEntry>();
const NONE: readonly ISettingProposal[] = [];

const entryOf = (machine: MachineId): IEntry => {
  let entry = entries.get(machine);
  if (!entry) {
    entry = { proposals: NONE, listeners: new Set(), timer: null, inFlight: false };
    entries.set(machine, entry);
  }
  return entry;
};

const put = (entry: IEntry, next: readonly ISettingProposal[]) => {
  const same =
    entry.proposals.length === next.length && entry.proposals.every((p, i) => p.id === next[i]?.id);
  if (same) return;
  entry.proposals = next;
  entry.listeners.forEach((fn) => fn());
};

const look = async (machine: MachineId): Promise<void> => {
  const entry = entryOf(machine);
  if (entry.timer) clearTimeout(entry.timer);
  entry.timer = null;
  if (entry.listeners.size === 0) return;
  let wait = PROPOSALS_POLL_MS;
  if (!entry.inFlight && document.visibilityState === 'visible') {
    entry.inFlight = true;
    const res = await loadSettingProposals(machine);
    entry.inFlight = false;
    if ('ok' in res && !isRefused(res.value)) put(entry, res.value);
    else wait = QUIET_POLL_MS;
  }
  if (entry.listeners.size > 0 && entry.timer === null)
    entry.timer = setTimeout(() => void look(machine), wait);
};

/** The proposals of a machine that wait for the person; one poll per machine however many chats show them. */
const useProposals = (machine: MachineId): readonly ISettingProposal[] => {
  const subscribe = useCallback(
    (fn: () => void) => {
      const entry = entryOf(machine);
      entry.listeners.add(fn);
      // The first reader starts the poll; the next ones get a fresh look.
      void look(machine);
      return () => {
        entry.listeners.delete(fn);
        if (entry.listeners.size === 0 && entry.timer) {
          clearTimeout(entry.timer);
          entry.timer = null;
        }
      };
    },
    [machine],
  );
  return useSyncExternalStore(subscribe, () => entryOf(machine).proposals);
};

interface ISettingsProposalsProps {
  machine: MachineId;
  /** The chat: only what its agent proposed is asked here. */
  conversation: string;
}

export const SettingsProposals = ({ machine, conversation }: ISettingsProposalsProps) => {
  const proposal = useProposals(machine).find((p) => p.conv === conversation) ?? null;
  const card = usePresence(proposal, CARD_EXIT_MS);

  const onAnswer = async (asked: ISettingProposal, verb: TProposalVerb): Promise<string | null> => {
    const refused = refusalOf(await answerSettingProposal(machine, asked.id, verb));
    // Answered, or gone already (answered on another page, out of time): the list says which.
    const entry = entryOf(machine);
    if (refused === null)
      put(
        entry,
        entry.proposals.filter((p) => p.id !== asked.id),
      );
    void look(machine);
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
