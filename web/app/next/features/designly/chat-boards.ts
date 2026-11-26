// The boards of a design the agent wrote in a chat: every finished turn says which pages of the design it wrote
// (TurnOutput.pages). The board switcher lists them as «В этом чате» and marks them with a dot; «Макет» →
// «Страницы» marks them the same way. Only the turns the page has loaded are looked at.
import { useCallback, useRef, useSyncExternalStore } from 'react';
import { feedStore, key, turnStore } from '@/next/stores/conversation';
import type { ConvId } from '@/next/protocol/v2';

const NONE: ReadonlySet<string> = new Set();

/** The pages written by the loaded turns of a chat, in the order they were last written. */
export const boardsOfChat = (conv: ConvId, design: string): string[] => {
  const found = new Set<string>();
  for (const row of feedStore.get(conv)?.rows ?? []) {
    if (row.k !== 'turn') continue;
    for (const output of turnStore.get(key(conv, row.id))?.turn?.outputs ?? []) {
      if (output.kind !== 'design' || output.ref !== design) continue;
      for (const page of output.pages ?? []) {
        found.delete(page);
        found.add(page);
      }
    }
  }
  return [...found];
};

export const useChatBoards = (conv: ConvId | null, design: string): ReadonlySet<string> => {
  const memo = useRef<{ sign: string; set: ReadonlySet<string> }>({ sign: '', set: NONE });
  const subscribe = useCallback(
    (changed: () => void) => {
      if (conv === null) return () => undefined;
      const feed = feedStore.subscribeKey(conv, changed);
      const turns = turnStore.subscribeAll((keys) => {
        const prefix = key(conv, '');
        for (const k of keys) if (k.startsWith(prefix)) return changed();
      });
      return () => {
        feed();
        turns();
      };
    },
    [conv],
  );
  const get = () => {
    const pages = conv === null ? [] : boardsOfChat(conv, design);
    const sign = pages.join('\n');
    if (memo.current.sign !== sign) memo.current = { sign, set: pages.length > 0 ? new Set(pages) : NONE };
    return memo.current.set;
  };
  return useSyncExternalStore(subscribe, get, get);
};
