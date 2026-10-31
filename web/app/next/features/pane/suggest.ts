// What the omnibox offers under the text: the machine's servers, the pinned pages, the ones visited lately, and the
// search for the words as the last row.
import { classifyInput, parseAddress, type IAddress } from '@/next/features/pane/address';
import type { IListeningPort } from '@/next/core/net/preview';
import { pinLabel, type IPin } from '@/next/features/pane/pins';

export type TSuggestKind = 'port' | 'pin' | 'visit' | 'search';

export interface ISuggestion {
  kind: TSuggestKind;
  /** Unique in the list. */
  id: string;
  label: string;
  /** The address or the process under the label; null when the label is the address. */
  hint: string | null;
  address: IAddress;
}

export interface ISuggestSources {
  ports: readonly IListeningPort[] | null;
  pins: readonly IPin[] | null;
  visits: readonly string[];
  /** Turns a locale key into its text (pins shipped with a name). */
  translate: (key: string) => string;
}

export const SUGGEST_LIMIT = 8;

const matches = (q: string, ...parts: (string | null)[]) =>
  q === '' || parts.some((part) => part !== null && part.toLowerCase().includes(q));

export const suggest = (input: string, sources: ISuggestSources, limit = SUGGEST_LIMIT): ISuggestion[] => {
  const typed = input.trim();
  const q = typed.replace(/^[?:]\s*/, '').toLowerCase();
  const rows: ISuggestion[] = [];
  const seen = new Set<string>();
  const add = (row: ISuggestion) => {
    if (seen.has(row.address.text)) return;
    seen.add(row.address.text);
    rows.push(row);
  };

  for (const p of sources.ports ?? []) {
    if (!p.responding || p.kind !== 'site') continue;
    const address = parseAddress(String(p.port));
    const label = p.title ?? p.command ?? `:${p.port}`;
    if (address && matches(q, label, `localhost:${p.port}`, p.command))
      add({
        kind: 'port',
        id: `port:${p.port}`,
        label,
        hint: `localhost:${p.port}`,
        address: { ...address, title: label },
      });
  }
  for (const pin of sources.pins ?? []) {
    const address = parseAddress(pin.text);
    const label = pinLabel(pin, sources.translate);
    if (address && matches(q, label, pin.text))
      add({
        kind: 'pin',
        id: `pin:${pin.text}`,
        label,
        hint: label === pin.text ? null : pin.text,
        address: { ...address, title: label },
      });
  }
  for (const text of sources.visits) {
    const address = parseAddress(text);
    if (address && matches(q, text))
      add({ kind: 'visit', id: `visit:${text}`, label: text, hint: null, address });
  }

  const list = rows.slice(0, limit);
  const asked = classifyInput(typed);
  // The words themselves are one more way to go; an address typed in full is what Enter opens already.
  if (asked.kind === 'search')
    list.push({ kind: 'search', id: 'search', label: asked.q, hint: null, address: asked.address });
  return list;
};
