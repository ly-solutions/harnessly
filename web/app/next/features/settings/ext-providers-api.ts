// The registry of the model providers a person connects by API (internal/extproviders) as the page reads and
// changes it. A key goes one way only: out in the body of a POST or PATCH; the answers say only whether one is kept.
import { getPage, sendBody, sendJson, sendPage } from '@/next/core/net/http';
import { platformPath, runtimePath } from '@/next/core/net/machine';
import { answer, bool, list, num, obj, opt, str } from '@/next/features/settings/platform-answer';
import type { MachineId } from '@/next/protocol/v2';

const TIMEOUT_MS = 10_000;
/** The machine asks the provider over the internet: a check is given longer than a write. */
const CHECK_TIMEOUT_MS = 40_000;

export const EXT_API = { openai: 'openai', anthropic: 'anthropic' } as const;
export type TExtApi = (typeof EXT_API)[keyof typeof EXT_API];

export interface IExtModel {
  id: string;
  label: string;
}

export interface IExtProvider {
  id: string;
  name: string;
  baseUrl: string;
  host: string;
  api: TExtApi;
  /** The preset it was made from; '' for an address of the person's own. */
  preset: string;
  hasKey: boolean;
  models: readonly IExtModel[];
}

export interface IExtPreset {
  id: string;
  name: string;
  baseUrl: string;
  api: TExtApi;
  /** Where the vendor gives out keys; '' when the preset names no page. */
  keyUrl: string;
}

/** What a check of the connection found: the models on success, else the reason in the runtime's words. */
export interface IExtCheck {
  ok: boolean;
  reason: string;
  message: string;
  models: readonly IExtModel[];
}

export interface IExtRegistry {
  providers: readonly IExtProvider[];
  presets: readonly IExtPreset[];
}

const readApi = (v: unknown): TExtApi => {
  if (v !== EXT_API.openai && v !== EXT_API.anthropic) throw new Error('answer: api');
  return v;
};

const readModel = (v: unknown): IExtModel => {
  const o = obj(v, 'model');
  const id = str(o.id, 'model.id');
  return { id, label: opt(o.label) || id };
};

const readProvider = (v: unknown): IExtProvider => {
  const o = obj(v, 'provider');
  return {
    id: str(o.id, 'id'),
    name: str(o.name, 'name'),
    baseUrl: str(o.baseUrl, 'baseUrl'),
    host: str(o.host, 'host'),
    api: readApi(o.api),
    preset: opt(o.preset),
    hasKey: bool(o.hasKey, 'hasKey'),
    models: list(o.models, 'models', readModel),
  };
};

const readPreset = (v: unknown): IExtPreset => {
  const o = obj(v, 'preset');
  return {
    id: str(o.id, 'id'),
    name: str(o.name, 'name'),
    baseUrl: str(o.baseUrl, 'baseUrl'),
    api: readApi(o.api),
    keyUrl: opt(o.keyUrl),
  };
};

const readCheck = (o: Record<string, unknown>): IExtCheck => ({
  ok: bool(o.ok, 'ok'),
  reason: opt(o.reason),
  message: opt(o.message),
  models: list(o.models, 'models', readModel),
});

const readOne = answer((o) => readProvider(o.provider));

const url = (machine: MachineId, rest = '') => runtimePath(platformPath(`/ext-providers${rest}`), machine);
const of = (id: string) => `/${encodeURIComponent(id)}`;

/** GET <platform>/ext-providers as the runtime answers it. */
export const readExtRegistry = answer((o): IExtRegistry => ({
  providers: list(o.providers, 'providers', readProvider),
  presets: list(o.presets, 'presets', readPreset),
}));

export const loadExtProviders = (machine: MachineId) => getPage(url(machine), readExtRegistry, TIMEOUT_MS);

export interface IExtDraft {
  name: string;
  baseUrl: string;
  api: TExtApi;
  preset: string;
  key: string;
  models: readonly IExtModel[];
}

export const createExtProvider = (machine: MachineId, draft: IExtDraft) =>
  sendBody('POST', url(machine), draft, readOne, TIMEOUT_MS);

/** Changes what the patch names. A key that is '' is not sent: the kept one stays. */
export const patchExtProvider = (
  machine: MachineId,
  id: string,
  patch: Partial<Pick<IExtDraft, 'name' | 'baseUrl' | 'api' | 'key'>>,
) => sendJson(url(machine, of(id)), patch, readOne, TIMEOUT_MS);

export const removeExtProvider = (machine: MachineId, id: string) =>
  sendPage(
    'DELETE',
    url(machine, of(id)),
    answer(() => true),
    TIMEOUT_MS,
  );

/** Checks an address and a key before the provider is made. */
export const checkExtDraft = (machine: MachineId, draft: Pick<IExtDraft, 'baseUrl' | 'api' | 'key'>) =>
  sendBody('POST', url(machine, '/check'), draft, answer(readCheck), CHECK_TIMEOUT_MS);

/** Checks a kept provider with its kept key; the models it lists are kept by the machine. */
export const checkExtProvider = (machine: MachineId, id: string) =>
  sendPage('POST', url(machine, `${of(id)}/check`), answer(readCheck), CHECK_TIMEOUT_MS);

/** Asks the provider for its models again. `check` is how that went; without it the kept list came back. */
export const refreshExtModels = (machine: MachineId, id: string) =>
  getPage(
    url(machine, `${of(id)}/models?refresh=1`),
    answer((o) => ({
      models: list(o.models, 'models', readModel),
      check: o.check === undefined || o.check === null ? null : readCheck(obj(o.check, 'check')),
      at: o.at === undefined ? 0 : num(o.at, 'at'),
    })),
    CHECK_TIMEOUT_MS,
  );
