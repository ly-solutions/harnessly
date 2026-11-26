// The routines app's reach of the host: it runs in the page as the host's own app (builtin-app.ts) and reads
// the routines only through hl.routines, with the rights of its manifest (internal/apps/builtin/routines). A chat is
// asked about once it is a chat the hub started for a run (its mark `from.app` is `routines`), so another chat costs
// nothing; while the run works it is asked again every 10 s, after it every minute, and at once after a change.
import { useEffect, useSyncExternalStore } from 'react';
import { pageClock } from '@/next/core/clock';
import { connectBuiltin, type IBuiltinHl } from '@/next/features/app-views/builtin-app';
import { routinesHandler } from '@/next/features/app-views/routines';
import { isOver, runChatOf, type IRunChat } from '@/next/features/app-views/routines-app/model';
import { locale } from '@/next/i18n';
import type { ConvId, MachineId } from '@/next/protocol/v2';
import { useConversation } from '@/next/stores/conversations';

export const ROUTINES_APP = 'harnessly.routines';
/** The mark the hub puts on a run's chat (automations runFrom). */
export const RUNS_FOLDER_APP = 'routines';

const POLL_RUNNING_MS = 10_000;
const POLL_IDLE_MS = 60_000;

export type TRunChatState = { k: 'loading' } | { k: 'none' } | { k: 'ready'; v: IRunChat };

const apps = new Map<MachineId, IBuiltinHl>();

/** The app's SDK on a machine, made once. */
export const routinesHl = (machine: MachineId): IBuiltinHl => {
  let hl = apps.get(machine);
  if (!hl) {
    hl = connectBuiltin(ROUTINES_APP, {
      handlers: { routines: routinesHandler(machine) },
      context: { target: 'side', view: 'runs', locale: locale() },
    });
    apps.set(machine, hl);
  }
  return hl;
};

interface IEntry {
  machine: MachineId;
  chat: ConvId;
  state: TRunChatState;
  subs: Set<() => void>;
  timer: unknown;
  asking: boolean;
}

const entries = new Map<string, IEntry>();
const LOADING: TRunChatState = { k: 'loading' };

const schedule = (e: IEntry) => {
  const clock = pageClock();
  if (e.timer !== null) clock.clearTimeout(e.timer);
  e.timer = null;
  if (e.subs.size === 0) return;
  const running = e.state.k === 'ready' && !isOver(e.state.v.run);
  e.timer = clock.setTimeout(() => void ask(e), running ? POLL_RUNNING_MS : POLL_IDLE_MS);
};

const ask = async (e: IEntry) => {
  if (e.asking) return;
  e.asking = true;
  try {
    const got = runChatOf(await routinesHl(e.machine).routines.forChat(e.chat));
    e.state = got ? { k: 'ready', v: got } : { k: 'none' };
  } catch {
    // The hub or the machine did not answer: what was shown stays, a first look shows nothing.
    if (e.state.k === 'loading') e.state = { k: 'none' };
  } finally {
    e.asking = false;
  }
  e.subs.forEach((s) => s());
  schedule(e);
};

const entryOf = (machine: MachineId, chat: ConvId): IEntry => {
  const key = `${machine}|${chat}`;
  let e = entries.get(key);
  if (!e) {
    e = { machine, chat, state: LOADING, subs: new Set(), timer: null, asking: false };
    entries.set(key, e);
  }
  return e;
};

/** Ask every chat of a machine again: a routine was turned on or off. */
export const refreshRunChats = (machine: MachineId) => {
  for (const e of entries.values()) if (e.machine === machine && e.subs.size > 0) void ask(e);
};

const NONE: TRunChatState = { k: 'none' };

/** Whether the hub started this chat for a run of a rule: only then is it asked about. */
export const useIsRunChat = (chat: ConvId | null): boolean =>
  useConversation(chat)?.from?.app === RUNS_FOLDER_APP;

/** The routine of a run's chat; `none` for any other chat (asked nothing). */
export const useRunChat = (machine: MachineId | null, chat: ConvId | null): TRunChatState => {
  const marked = useIsRunChat(chat);
  const e = machine && chat && marked ? entryOf(machine, chat) : null;
  useEffect(() => {
    if (!e) return;
    const first = e.subs.size === 0;
    const sub = () => undefined;
    e.subs.add(sub);
    if (first) void ask(e);
    return () => {
      e.subs.delete(sub);
      if (e.subs.size === 0) schedule(e);
    };
  }, [e]);
  return useSyncExternalStore(
    (l) => {
      if (!e) return () => undefined;
      e.subs.add(l);
      return () => e.subs.delete(l);
    },
    () => (e ? e.state : NONE),
    () => NONE,
  );
};

/** Turns a routine on or off through the app's SDK (routines.write), then asks the chats again. */
export const setRoutineEnabled = async (machine: MachineId, id: string, enabled: boolean) => {
  await routinesHl(machine).routines.setEnabled(id, enabled);
  refreshRunChats(machine);
};
