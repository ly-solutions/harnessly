// The requests of the «Модели» page: the presets of the extra vendors with what the machine has of their keys
// (GET /api/v2/providers), the key itself (PUT and DELETE /api/v2/providers/<id>/key), the check of the connection
// (POST /api/v2/providers/<id>/test) and the sign-in of a preset's proxy (POST/GET/DELETE .../login, POST .../logout). A key goes out in the body of the PUT and is never read back: the answers carry
// only «configured» and the last digits.
import { getPage, sendBody, sendPage } from '@/next/core/net/http';
import { machineApi } from '@/next/core/net/machine';
import { parsePage, type MachineId, type ProfileId } from '@/next/protocol/v2';

const PROVIDERS_TIMEOUT_MS = 10_000;
/** The vendor is asked over the internet by the machine: the check is given longer than a settings write. */
const TEST_TIMEOUT_MS = 30_000;

export const loadProviders = (machine: MachineId) =>
  getPage(machineApi(machine).http('/providers'), parsePage.profiles, PROVIDERS_TIMEOUT_MS);

export const putProviderKey = (machine: MachineId, id: ProfileId, key: string) =>
  sendBody(
    'PUT',
    machineApi(machine).http(`/providers/${id}/key`),
    { key },
    parsePage.profileKey,
    PROVIDERS_TIMEOUT_MS,
  );

export const deleteProviderKey = (machine: MachineId, id: ProfileId) =>
  sendPage(
    'DELETE',
    machineApi(machine).http(`/providers/${id}/key`),
    parsePage.profileKey,
    PROVIDERS_TIMEOUT_MS,
  );

export const testProvider = (machine: MachineId, id: ProfileId) =>
  sendPage('POST', machineApi(machine).http(`/providers/${id}/test`), parsePage.profileTest, TEST_TIMEOUT_MS);

/** The proxy's device login: starting it waits for the link and the code. */
const LOGIN_TIMEOUT_MS = 40_000;

export const loadProviderLogin = (machine: MachineId, id: ProfileId) =>
  getPage(machineApi(machine).http(`/providers/${id}/login`), parsePage.login, PROVIDERS_TIMEOUT_MS);

export const startProviderLogin = (machine: MachineId, id: ProfileId) =>
  sendPage('POST', machineApi(machine).http(`/providers/${id}/login`), parsePage.login, LOGIN_TIMEOUT_MS);

export const cancelProviderLogin = (machine: MachineId, id: ProfileId) =>
  sendPage(
    'DELETE',
    machineApi(machine).http(`/providers/${id}/login`),
    parsePage.login,
    PROVIDERS_TIMEOUT_MS,
  );

export const logoutProvider = (machine: MachineId, id: ProfileId) =>
  sendPage(
    'POST',
    machineApi(machine).http(`/providers/${id}/logout`),
    parsePage.loginOK,
    PROVIDERS_TIMEOUT_MS,
  );
