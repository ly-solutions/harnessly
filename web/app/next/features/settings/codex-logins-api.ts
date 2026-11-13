// The sign-ins of the Codex CLI beside the main one (cmd/runtime/routes_codex_logins.go): each is a ChatGPT plan of its
// own that a new Codex chat can start on. The list says whose plan a sign-in holds and never a token.
import { getPage, sendBody, sendJson, sendPage } from '@/next/core/net/http';
import { platformPath, runtimePath } from '@/next/core/net/machine';
import { answer, list, num, obj, opt, str } from '@/next/features/settings/platform-answer';
import type { MachineId } from '@/next/protocol/v2';

const TIMEOUT_MS = 10_000;

export interface ICodexLogin {
  id: string;
  name: string;
  /** The profile a chat on this sign-in starts with. */
  profile: string;
  /** Whose plan it is signed in with; '' when it is not signed in. */
  account: string;
  /** A new Codex chat starts on this sign-in unless another is picked. */
  main: boolean;
}

export interface ICodexLogins {
  logins: readonly ICodexLogin[];
  /** How many sign-ins the machine keeps. */
  max: number;
}

const readLogin = (v: unknown): ICodexLogin => {
  const o = obj(v, 'login');
  return {
    id: str(o.id, 'id'),
    name: str(o.name, 'name'),
    profile: str(o.profile, 'profile'),
    account: opt(o.account),
    main: o.main === true,
  };
};

const url = (machine: MachineId, tail = '') =>
  runtimePath(platformPath(`/codex-logins${tail}` as `/${string}`), machine);

const one = answer((o) => readLogin(o.login));

export const loadCodexLogins = (machine: MachineId) =>
  getPage(
    url(machine),
    answer((o): ICodexLogins => ({ logins: list(o.logins, 'logins', readLogin), max: num(o.max, 'max') })),
    TIMEOUT_MS,
  );

export const addCodexLogin = (machine: MachineId, name: string) =>
  sendBody('POST', url(machine), { name }, one, TIMEOUT_MS);

export const renameCodexLogin = (machine: MachineId, id: string, name: string) =>
  sendJson(url(machine, `/${encodeURIComponent(id)}`), { name }, one, TIMEOUT_MS);

export const removeCodexLogin = (machine: MachineId, id: string) =>
  sendPage(
    'DELETE',
    url(machine, `/${encodeURIComponent(id)}`),
    answer(() => true),
    TIMEOUT_MS,
  );

/** Makes the sign-in the one a new chat starts on, or gives that back to the CLI's own sign-in. */
export const setCodexLoginMain = (machine: MachineId, id: string, main: boolean) =>
  sendBody(
    'PUT',
    url(machine, `/${encodeURIComponent(id)}`),
    { main },
    answer(() => true),
    TIMEOUT_MS,
  );
