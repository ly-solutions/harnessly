// What this device can show without a network: a chat whose feed is in the page (read in this session or
// taken back from IndexedDB at the start), a design with an offline copy. The sidebar marks the rest while offline.
import { useEffect, useState } from 'react';
import { useKey } from '@/next/core/store';
import { feedStore } from '@/next/stores/conversation';
import { offlineBase } from '@/next/offline/client';
import { owner, shelf, type Copy } from '@/next/offline/storage';
import type { ConvId } from '@/next/protocol/v2';

/** The chat's feed is on this device: it opens offline. */
export const useChatKept = (conv: ConvId): boolean => {
  const feed = useKey(feedStore, conv);
  return !!feed?.loaded && !feed.error;
};

/** The key of a design's offline copy (features/designly/offline-download.ts writes it). */
export const designCopyKey = (machine: string, artifact: string): string =>
  JSON.stringify(['design', machine, artifact]);

/** The offline copy of a design; null when this device has none (or another person's shelf is here). */
export async function readDesignCopy(machine: string, artifact: string): Promise<Copy | null> {
  const person = owner(offlineBase());
  if (!person) return null;
  try {
    const db = await shelf(offlineBase(), person);
    try {
      const req = db.transaction('copies').objectStore('copies').get(designCopyKey(machine, artifact));
      return await new Promise<Copy | null>((resolve, reject) => {
        req.onsuccess = () => resolve((req.result as Copy | undefined) ?? null);
        req.onerror = () => reject(req.error);
      });
    } finally {
      db.close();
    }
  } catch {
    return null;
  }
}

/** The ids of the designs with an offline copy, read once per call of `when` changing (null while reading). */
export function useKeptDesigns(when: boolean): ReadonlySet<string> | null {
  const [kept, setKept] = useState<ReadonlySet<string> | null>(null);
  useEffect(() => {
    if (!when) return;
    let alive = true;
    void (async () => {
      const person = owner(offlineBase());
      const ids = new Set<string>();
      if (person) {
        try {
          const db = await shelf(offlineBase(), person);
          try {
            const req = db.transaction('copies').objectStore('copies').getAllKeys();
            const keys = await new Promise<IDBValidKey[]>((resolve, reject) => {
              req.onsuccess = () => resolve(req.result);
              req.onerror = () => reject(req.error);
            });
            for (const k of keys) {
              try {
                const parsed: unknown = JSON.parse(String(k));
                if (Array.isArray(parsed) && parsed[0] === 'design' && typeof parsed[2] === 'string')
                  ids.add(parsed[2]);
              } catch {
                // A key of another kind.
              }
            }
          } finally {
            db.close();
          }
        } catch {
          // No shelf: nothing is kept.
        }
      }
      if (alive) setKept(ids);
    })();
    return () => {
      alive = false;
    };
  }, [when]);
  return kept;
}
