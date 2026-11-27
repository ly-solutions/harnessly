import { appBasePath } from '@/platform/config';
import { owner, selectOwner, saveCopy, type Copy } from './storage';

/** The base of the app's pages, where the service worker and the offline copies live; read when used, not on import. */
export const offlineBase = (): string => appBasePath().replace(/\/$/, '') + '/';
let confirmed: string | null = null;
export function confirmOfflineOwner(person: string | null) {
  confirmed = person;
  selectOwner(offlineBase(), person);
}
export function copyWriter() {
  const person = owner(offlineBase());
  if (!person || person !== confirmed) return null;
  return (key: string[], title: string, kind: Copy['kind'], body: Blob, revision?: number) =>
    saveCopy(offlineBase(), person, { id: JSON.stringify(key), title, kind, body, revision, at: Date.now() });
}

/**
 * Lets the page the service worker kept go (sw.js): it carries the settings of the person's machine, and the
 * next person on this browser must not open the app on them offline.
 */
export function forgetOfflinePage(): void {
  try {
    void caches.delete('hx-app:' + encodeURIComponent(offlineBase()) + ':page').catch(() => undefined);
  } catch {
    // No Cache Storage (an insecure origin, an old browser): nothing was kept.
  }
}
