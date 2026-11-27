// Triggers of an MCP connection: which tools can be a trigger (the runtime's classifier, nothing called), a
// draft recipe of one (its tool called once), a check that shows the newest things as they will be events, and the
// trigger saved as a connection of the connector mcp-recipe (GET|POST <platform>/mcp/<id>/triggers[/draft|/try]).
import { getPage, sendBody, type PageOutcome } from '@/next/core/net/http';
import { platformPath, runtimePath } from '@/next/core/net/machine';
import type { MachineId } from '@/next/protocol/v2';

const CALL_TIMEOUT_MS = 60_000;

export interface IGateway {
  list: string;
  call: string;
  nameArg: string;
  argsArg: string;
  serverArg?: string;
  server?: string;
}

export interface ICandidateArg {
  name: string;
  type: string;
  required: boolean;
  /** Takes «since»: the recipe passes its cursor here. */
  cursor: boolean;
  description: string;
}

export interface ICandidate {
  tool: string;
  gateway: IGateway | null;
  description: string;
  score: number;
  fits: boolean;
  excluded: boolean;
  plus: readonly string[];
  minus: readonly string[];
  label: string;
  labelRu: string;
  readOnly: boolean;
  args: readonly ICandidateArg[];
}

/** A poll recipe, as internal/connectors/mcprecipe keeps it. */
export interface IRecipe {
  trigger: string;
  tool: string;
  args?: Record<string, unknown>;
  items?: string;
  id: string;
  at?: string;
  actor?: string;
  title?: string;
  url?: string;
  thread?: string;
  cursor?: string;
  every?: string;
  via?: IGateway;
  filter?: Record<string, string>;
  mode?: string;
  watch?: string[];
  facts?: string[];
  next?: { path: string; arg: string };
  path?: string;
  query?: Record<string, string>;
  script?: string;
  version?: number;
}

export interface ISample {
  id: string;
  at: string;
  actor: string;
  title: string;
  url: string;
}

export type TTriggerAnswer<T> = { ok: true; value: T } | { ok: false; error: string };

type TRaw<T> = { value: T } | { error: string };

const isObj = (v: unknown): v is Record<string, unknown> =>
  typeof v === 'object' && v !== null && !Array.isArray(v);
const s = (v: unknown): string => (typeof v === 'string' ? v : '');
const strs = (v: unknown): string[] =>
  Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [];

const answer =
  <T>(read: (o: Record<string, unknown>) => T) =>
  (v: unknown): TRaw<T> => {
    if (!isObj(v)) return { error: 'bad answer' };
    if (typeof v.error === 'string') return { error: s(v.message) || v.error };
    return { value: read(v) };
  };

const gatewayOf = (v: unknown): IGateway | null =>
  isObj(v)
    ? {
        list: s(v.list),
        call: s(v.call),
        nameArg: s(v.nameArg),
        argsArg: s(v.argsArg),
        ...(s(v.serverArg) ? { serverArg: s(v.serverArg) } : {}),
        ...(s(v.server) ? { server: s(v.server) } : {}),
      }
    : null;

export const readCandidate = (v: unknown): ICandidate => {
  const o = isObj(v) ? v : {};
  return {
    tool: s(o.tool),
    gateway: gatewayOf(o.gateway),
    description: s(o.description),
    score: typeof o.score === 'number' ? o.score : 0,
    fits: o.fits === true,
    excluded: o.excluded === true,
    plus: strs(o.plus),
    minus: strs(o.minus),
    label: s(o.label),
    labelRu: s(o.labelRu),
    readOnly: o.readOnly === true,
    args: Array.isArray(o.args)
      ? o.args.filter(isObj).map((a) => ({
          name: s(a.name),
          type: s(a.type),
          required: a.required === true,
          cursor: a.cursor === true,
          description: s(a.description),
        }))
      : [],
  };
};

const readSample = (v: unknown): ISample => {
  const o = isObj(v) ? v : {};
  return { id: s(o.id), at: s(o.at), actor: s(o.actor), title: s(o.title), url: s(o.url) };
};

const samplesOf = (v: unknown): ISample[] => (Array.isArray(v) ? v.map(readSample) : []);

const settle = async <T>(p: Promise<PageOutcome<TRaw<T>>>): Promise<TTriggerAnswer<T>> => {
  const res = await p;
  if (!('ok' in res))
    return {
      ok: false,
      error: res.r === 'failed' ? `${res.code}${res.status ? ` ${res.status}` : ''}` : res.r,
    };
  return 'error' in res.value ? { ok: false, error: res.value.error } : { ok: true, value: res.value.value };
};

const path = (machine: MachineId, id: string, rest = '') =>
  runtimePath(platformPath(`/mcp/${encodeURIComponent(id)}/triggers${rest}`), machine);

export const loadCandidates = (machine: MachineId, id: string) =>
  settle(
    getPage(
      path(machine, id),
      answer((o) => (Array.isArray(o.candidates) ? o.candidates.map(readCandidate) : [])),
      CALL_TIMEOUT_MS,
    ),
  );

export interface IDraftInput {
  tool: string;
  description: string;
  via: IGateway | null;
  args: Record<string, unknown>;
  cursorArg: string;
  cursorNumber: boolean;
  unmarked: boolean;
}

export const draftTrigger = (machine: MachineId, id: string, input: IDraftInput) =>
  settle(
    sendBody(
      'POST',
      path(machine, id, '/draft'),
      { ...input, via: input.via ?? undefined },
      answer((o) => ({
        recipe: (isObj(o.recipe) ? o.recipe : {}) as unknown as IRecipe,
        samples: samplesOf(o.samples),
      })),
      CALL_TIMEOUT_MS,
    ),
  );

export const tryTrigger = (machine: MachineId, id: string, recipe: IRecipe, unmarked: boolean) =>
  settle(
    sendBody(
      'POST',
      path(machine, id, '/try'),
      { recipe, unmarked },
      answer((o) => samplesOf(o.samples)),
      CALL_TIMEOUT_MS,
    ),
  );

export const saveTrigger = (
  machine: MachineId,
  id: string,
  recipe: IRecipe,
  unmarked: boolean,
  name: string,
) =>
  settle(
    sendBody(
      'POST',
      path(machine, id),
      { recipe, unmarked, name },
      answer((o) => s(isObj(o.connection) ? o.connection.id : '')),
      CALL_TIMEOUT_MS,
    ),
  );

/** What the trigger panel needs: the calls bound to one machine and connection. */
export interface ITriggerApi {
  load: () => Promise<TTriggerAnswer<ICandidate[]>>;
  draft: (input: IDraftInput) => Promise<TTriggerAnswer<{ recipe: IRecipe; samples: ISample[] }>>;
  test: (recipe: IRecipe, unmarked: boolean) => Promise<TTriggerAnswer<ISample[]>>;
  save: (recipe: IRecipe, unmarked: boolean, name: string) => Promise<TTriggerAnswer<string>>;
  /** Where the triggers' catalog is (the hub's /events → Triggers); null without a hub. */
  eventsUrl: string | null;
}

export const triggerApi = (machine: MachineId, id: string, hubUrl: string | null): ITriggerApi => ({
  load: () => loadCandidates(machine, id),
  draft: (input) => draftTrigger(machine, id, input),
  test: (recipe, unmarked) => tryTrigger(machine, id, recipe, unmarked),
  save: (recipe, unmarked, name) => saveTrigger(machine, id, recipe, unmarked, name),
  eventsUrl: hubUrl ? `${hubUrl}/events?tab=triggers` : null,
});
