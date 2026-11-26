// hl.routines: what an app reads of the person's routines — the recurring tasks the hub runs — and the one
// thing it may change, a routine on or off. The bridge has checked the right (routines.read, routines.write); the work
// is the routines screen's own: the page asks its runtime (`<platform>/routines`), the runtime's agent signs the call and
// the hub answers for the machine's owner. A chat is a run of a routine when the hub started it for one
// (conversations.start, automation_runs.conversation_id): `?chat=` asks the hub which.
import { platformPath } from '@/next/core/net/machine';
import { RuntimeError, runtimeJson } from '@/next/core/net/runtime-json';
import { BridgeError, type TRoutinesOp } from '@/next/features/app-views/bridge';
import type { MachineId } from '@/next/protocol/v2';

export type TRunOutcome = 'ok' | 'quiet' | 'failed';

export interface IRoutineArtifact {
  kind: string;
  title: string;
  url?: string;
  meta?: string;
}

export interface IRoutineRun {
  id: string;
  ruleId: string;
  fingerprint: string;
  status: string;
  outcome?: TRunOutcome;
  conversationId?: string;
  summary?: string;
  error?: string;
  createdAt: string;
  dispatchedAt?: string;
  finishedAt?: string;
  result?: {
    summary?: string;
    body?: string;
    artifacts?: IRoutineArtifact[];
    sources?: IRoutineArtifact[];
    error?: string;
    manual?: boolean;
    said?: TRunOutcome;
  };
}

export interface IRoutineInfo {
  id: string;
  name: string;
  enabled: boolean;
  pausedReason?: string;
  state?: string;
  model: string;
  trigger: { schedule?: { cron: string; tz?: string } };
  routine: { words?: string; once?: boolean; until?: string; chatId?: string };
  next: string[];
  stats: { runs: number; ok: number; quiet: number; failed: number };
}

/** The routine whose run a chat is: null for any other chat. */
export interface IRoutineOfChat {
  routine: IRoutineInfo | null;
  run?: IRoutineRun;
  runs?: IRoutineRun[];
}

const base = () => platformPath('/routines');
const one = (id: string) => `${base()}/${encodeURIComponent(id)}` as `/${string}`;

/** The runtime's or the hub's refusal (a routine that is not the person's among them) as the bridge's, in its words. */
const asked = async <T>(call: () => Promise<T>): Promise<T> => {
  try {
    return await call();
  } catch (e) {
    if (e instanceof RuntimeError) throw new BridgeError('upstream_error', e.message);
    throw new BridgeError('host_unavailable', e instanceof Error ? e.message : 'the machine did not answer');
  }
};

/** The work behind hl.routines for the views of one machine. */
export const routinesHandler =
  (machine: MachineId) =>
  async (op: TRoutinesOp, params: Record<string, unknown>): Promise<unknown> => {
    const id = String(params.id ?? '');
    switch (op) {
      case 'forChat': {
        const chat = encodeURIComponent(String(params.chat ?? ''));
        const got = await asked(() =>
          runtimeJson<IRoutineOfChat>(machine, `${base()}?chat=${chat}&limit=20` as `/${string}`),
        );
        return got.routine ? got : { routine: null };
      }
      case 'get':
        return asked(() => runtimeJson<{ routine: IRoutineInfo }>(machine, one(id)));
      case 'runs':
        return asked(() =>
          runtimeJson<{ runs: IRoutineRun[] }>(
            machine,
            `${one(id)}/runs?limit=${Number(params.limit ?? 20)}` as `/${string}`,
          ),
        );
      case 'setEnabled':
        return asked(() =>
          runtimeJson<{ routine: IRoutineInfo }>(machine, one(id), {
            method: 'PATCH',
            headers: { 'content-type': 'application/json' },
            body: JSON.stringify({ enabled: params.enabled === true }),
          }),
        );
    }
  };
