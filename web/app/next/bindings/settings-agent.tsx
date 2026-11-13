// The agent of the Settings window: the island at the bottom of the screen. What the person writes goes to
// a chat the window made for it and named to the machine (PUT settings-agent), so its agent changes a setting without
// a card of consent; the island shows that chat's last answer, the changes the journal got since, and the question the
// agent asks before it changes anything.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { refusalOf, type ISettingChangesHandle } from '@/next/bindings/settings-changes';
import { answerQuestion, createConversation, renameConversation } from '@/next/commands';
import { sendMessage } from '@/next/commands/messages';
import { useConversationChannel } from '@/next/core/realtime/hub';
import AgentIsland, {
  ISLAND,
  type IIslandChange,
  type IIslandOption,
  type TIslandPhase,
} from '@/next/features/settings/agent-island';
import {
  CHANGE_BY,
  loadSettingsChats,
  nameSettingsChat,
  type ISettingChange,
} from '@/next/features/settings/changes-api';
import { isRefused } from '@/next/features/settings/platform-answer';
import { t } from '@/next/i18n';
import type { Conversation, ConvId, MachineId, Provider } from '@/next/protocol/v2';
import { goTo, ROUTE } from '@/next/routes/routes';
import { useGates } from '@/next/stores/conversation';
import { useConversation } from '@/next/stores/conversations';
import { useDefaultCwd } from '@/next/stores/projects';

/** How long the card over the field takes to leave (styles/settings.css, .hx-isle__reply[data-state]). */
const CLOSE_MS = 160;

/** The states of a chat whose agent takes the next message as it is. */
const ALIVE: ReadonlySet<Conversation['state']['k']> = new Set([
  'starting',
  'working',
  'waiting-question',
  'waiting-permission',
  'ready',
  'idle',
  'unknown',
]);

/** What was asked and what there was before it: the answer is the turn that came after, the changes the lines the
 *  journal did not have. */
interface IAsked {
  conv: ConvId | null;
  /** The chat's last turn when the words went; null for a chat that had none. */
  turnBefore: string | null;
  /** The journal's lines then; null when it was not read yet (no change is shown as this answer's). */
  journalBefore: ReadonlySet<string> | null;
  /** Why the words did not reach an agent; null while they are on their way or answered. */
  failure: string | null;
}

interface ISettingsAgentProps {
  machine: MachineId;
  /** The agent the chat starts on; null when the machine has none. */
  provider: Provider | null;
  changes: ISettingChangesHandle;
}

const SettingsAgent = ({ machine, provider, changes }: ISettingsAgentProps) => {
  const defaultCwd = useDefaultCwd(machine);
  // The window's chats of this machine, the newest last; null until read.
  const [chats, setChats] = useState<{ machine: MachineId; list: readonly string[] } | null>(null);
  const [asked, setAsked] = useState<IAsked | null>(null);
  const [closing, setClosing] = useState(false);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    let live = true;
    void loadSettingsChats(machine).then((res) => {
      if (live && 'ok' in res && !isRefused(res.value)) setChats({ machine, list: res.value });
    });
    return () => {
      live = false;
    };
  }, [machine]);
  useEffect(() => () => clearTimeout(closeTimer.current ?? undefined), []);

  const kept = chats?.machine === machine ? ((chats.list.at(-1) ?? null) as ConvId | null) : null;
  const conv = asked?.conv ?? kept;
  const record = useConversation(conv);
  useConversationChannel(conv ? machine : null, conv);
  const gates = useGates(conv);

  const journal = changes.journal.k === 'ready' ? changes.journal.changes : null;

  const alive = conv !== null && record !== undefined && ALIVE.has(record.state.k);
  const lastTurn = record?.lastTurn?.id ?? null;

  /** A new chat of the window, named to the machine; the words of why not when it was not made. */
  const startChat = useCallback(async (): Promise<{ id: ConvId } | { failure: string }> => {
    if (provider === null) return { failure: t('next.settings.agentIsle.noAgent') };
    if (!defaultCwd) return { failure: t('next.newChat.noFolder') };
    const id = await createConversation(machine, { where: { cwd: defaultCwd }, provider, model: null })
      .conversation;
    if (id === null) return { failure: t('next.settings.agentIsle.noChat') };
    // Named before the first word reaches the agent: a chat the machine does not know asks the person's yes.
    const named = await nameSettingsChat(machine, id);
    const refused = refusalOf(named);
    if (refused !== null) return { failure: refused };
    setChats((was) => ({ machine, list: [...(was?.machine === machine ? was.list : []), id] }));
    void renameConversation(machine, id, t('next.settings.agentIsle.chatTitle'));
    return { id };
  }, [provider, defaultCwd, machine]);

  const onSend = useCallback(
    async (words: string) => {
      clearTimeout(closeTimer.current ?? undefined);
      setClosing(false);
      const journalBefore = journal === null ? null : new Set(journal.map((change) => change.id));
      if (alive && conv) {
        setAsked({ conv, turnBefore: lastTurn, journalBefore, failure: null });
        sendMessage(machine, { conversationId: conv, text: words });
        return;
      }
      setAsked({ conv: null, turnBefore: null, journalBefore, failure: null });
      const chat = await startChat();
      if ('failure' in chat) {
        setAsked({ conv: null, turnBefore: null, journalBefore, failure: chat.failure });
        return;
      }
      setAsked({ conv: chat.id, turnBefore: null, journalBefore, failure: null });
      sendMessage(machine, {
        conversationId: chat.id,
        text: `${t('next.settings.agentIsle.brief')}\n\n${words}`,
      });
    },
    [alive, conv, lastTurn, journal, machine, startChat],
  );

  const onDismiss = useCallback(() => {
    setClosing(true);
    closeTimer.current = setTimeout(() => {
      setAsked(null);
      setClosing(false);
    }, CLOSE_MS);
  }, []);

  // What the journal got since the words went, an agent's and still the setting's newest (not taken back).
  const made = useMemo((): IIslandChange[] => {
    const before = asked?.journalBefore;
    if (!before || journal === null) return [];
    return journal
      .filter((change: ISettingChange) => change.by === CHANGE_BY.agent && !before.has(change.id))
      .flatMap((change) => {
        const mark = changes.marks[change.key];
        return mark
          ? [
              {
                id: change.id,
                title: change.title,
                was: change.oldLabel,
                now: change.newLabel,
                busy: mark.busy,
                onUndo: mark.onUndo,
              },
            ]
          : [];
      });
  }, [asked, journal, changes.marks]);

  let phase: TIslandPhase = ISLAND.idle;
  let text = '';
  let options: IIslandOption[] = [];
  if (asked !== null && asked.failure !== null) {
    phase = ISLAND.failed;
    text = asked.failure;
  } else if (asked !== null && asked.conv === null) {
    // The chat is being made.
    phase = ISLAND.thinking;
  } else if (asked !== null && asked.conv !== null) {
    const chat = asked.conv;
    const turn = record?.lastTurn ?? null;
    const answered = turn !== null && turn.id !== asked.turnBefore && turn.status !== 'live';
    const state = record?.state.k;
    phase = ISLAND.thinking;
    if (state === 'waiting-question') {
      phase = ISLAND.ask;
      const question = gates.questions.find((q) => q.state.k === 'open');
      const item = question?.items.length === 1 ? question.items[0] : undefined;
      text = item?.question ?? t('next.settings.agentIsle.asks');
      if (question && item && !item.multi)
        options = item.options.map((option, i) => ({
          label: option.label,
          hint: option.description ?? '',
          onPick: () => void answerQuestion(machine, chat, question.id, [{ i: 0, choices: [i] }]),
        }));
    } else if (state === 'waiting-permission') {
      phase = ISLAND.ask;
      text = t('next.settings.agentIsle.asks');
    } else if (answered && turn.status === 'done') {
      phase = ISLAND.done;
      text = turn.snippet;
    } else if (answered) {
      phase = ISLAND.failed;
      text = t('next.settings.agentIsle.failed');
    } else if (state === 'launch-failed' || state === 'gone' || state === 'stopped') {
      phase = ISLAND.failed;
      text = t('next.settings.agentIsle.noChat');
    }
  }

  const suggestions = useMemo(
    () => [t('next.settings.agentIsle.suggest1'), t('next.settings.agentIsle.suggest2')],
    [],
  );

  return (
    <AgentIsland
      phase={phase}
      text={text}
      changes={made}
      options={options}
      closing={closing}
      suggestions={suggestions}
      onSend={(words) => void onSend(words)}
      onDismiss={onDismiss}
      onOpenChat={conv ? () => goTo({ kind: ROUTE.chat, id: conv }) : undefined}
    />
  );
};

export default SettingsAgent;
