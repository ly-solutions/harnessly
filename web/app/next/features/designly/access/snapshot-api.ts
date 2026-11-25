// The owner's side of a private snapshot (GitHub #180): the boards left as pictures for readers who already hold a
// grant while the owner machine is offline. Publishing gives nobody access; removing it takes none away.
import { send, type TTtlValue } from '@/next/features/designly/access/api';
import type { IStoreSnapshot } from '@/next/features/designly/api';

const at = (project: string) => `/access/design/${encodeURIComponent(project)}/snapshot` as `/${string}`;

const snapshotOf = (answer: unknown): IStoreSnapshot | null => {
  const value = (answer as { snapshot?: IStoreSnapshot | null } | null)?.snapshot;
  return value && typeof value.capturedAt === 'string' && Array.isArray(value.boards) ? value : null;
};

export const fetchSnapshot = async (project: string, signal?: AbortSignal) =>
  snapshotOf(await send('GET', at(project), undefined, signal));

/** `pages` null is every board. The owner machine captures them, so this waits for it. */
export const publishSnapshot = async (project: string, pages: string[] | null, ttl: TTtlValue) =>
  snapshotOf(await send('PUT', at(project), { pages, ttl }, undefined, 10 * 60_000));

export const removeSnapshot = async (project: string) => {
  await send('DELETE', at(project));
};
