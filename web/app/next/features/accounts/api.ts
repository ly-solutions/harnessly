// The requests of an agent's sign-in (Claude Code, Codex) under /api/v2/agents/<agent>/: the login the machine runs
// (POST/GET/DELETE login, POST login/code), the Codex API key (PUT key) and the sign-out (POST logout). A code or a key
// goes out in the body and is never read back. `account` names one more sign-in of the CLI (Codex's other homes): the
// same routes, in that sign-in's home.
import { getPage, sendBody, sendPage } from '@/next/core/net/http';
import { machineApi } from '@/next/core/net/machine';
import { parsePage, type MachineId, type Provider } from '@/next/protocol/v2';
import type { LoginVia } from '@/next/features/accounts/model';

const AUTH_TIMEOUT_MS = 15_000;
/** The address of a route of the agent; the machine goes into the query, so the path is given whole. */
const route = (machine: MachineId, provider: Provider, tail: string, account?: string) =>
  machineApi(machine).http(
    account === undefined
      ? `/agents/${provider}/${tail}`
      : `/agents/${provider}/${tail}?account=${encodeURIComponent(account)}`,
  );

/** The mode the machine takes for a way of signing in: Codex has only the device login. */
const modeOf = (provider: Provider, via: LoginVia) => (provider === 'codex' ? 'device' : via);

export const loadLogin = (machine: MachineId, provider: Provider, account?: string) =>
  getPage(route(machine, provider, 'login', account), parsePage.login, AUTH_TIMEOUT_MS);

export const startLogin = (machine: MachineId, provider: Provider, via: LoginVia, account?: string) =>
  sendBody(
    'POST',
    route(machine, provider, 'login', account),
    { mode: modeOf(provider, via) },
    parsePage.login,
    AUTH_TIMEOUT_MS,
  );

export const cancelLogin = (machine: MachineId, provider: Provider, account?: string) =>
  sendPage('DELETE', route(machine, provider, 'login', account), parsePage.login, AUTH_TIMEOUT_MS);

export const sendLoginCode = (
  machine: MachineId,
  provider: Provider,
  id: string,
  code: string,
  account?: string,
) =>
  sendBody(
    'POST',
    route(machine, provider, 'login/code', account),
    { id, code },
    parsePage.login,
    AUTH_TIMEOUT_MS,
  );

export const logoutAgent = (machine: MachineId, provider: Provider, account?: string) =>
  sendPage('POST', route(machine, provider, 'logout', account), parsePage.loginOK, AUTH_TIMEOUT_MS);

export const putAgentKey = (machine: MachineId, provider: Provider, key: string) =>
  sendBody('PUT', route(machine, provider, 'key'), { key }, parsePage.loginOK, AUTH_TIMEOUT_MS);
