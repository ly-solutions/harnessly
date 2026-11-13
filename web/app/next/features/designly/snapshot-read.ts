// A design is looked at from the hub's snapshot when its machine does not answer: the machine is asked first,
// and when it has not answered in SNAPSHOT_WAIT_MS (or has refused) the same read goes to the hub with `from=store`, which
// answers from the project's last revision in the store. Editing never takes this road: a save goes to the machine.
import { runtimeJson } from '@/next/core/net/runtime-json';
import type { MachineId } from '@/next/protocol/v2';

/** How long the machine is waited for before the hub's snapshot is asked, ms. */
export const SNAPSHOT_WAIT_MS = 2500;

const fromStore = (path: `/${string}`): `/${string}` => `${path}${path.includes('?') ? '&' : '?'}from=store`;

/** A read of the machine's routes with the hub's snapshot behind it. */
export const readOrSnapshot = async <T>(m: MachineId, path: `/${string}`, init?: RequestInit): Promise<T> => {
  try {
    return await runtimeJson<T>(m, path, init, SNAPSHOT_WAIT_MS);
  } catch (failure) {
    if (init?.signal?.aborted) throw failure;
    return runtimeJson<T>(m, fromStore(path), init);
  }
};

/** «версия от ЧЧ:ММ»: the time of a snapshot in the person's clock. */
export const snapshotTime = (at: number): string =>
  new Date(at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
