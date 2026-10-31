// What a person types as an address in the pane: «4400» or «:4400» (a port of this machine), «localhost:3000/x»,
// «stand-2.preview.example.com/adminpanel», «https://…». A port opens in a port slot, anything else as a site.
import type { TPreviewTarget } from '@/next/core/net/preview';

export interface IAddress {
  target: TPreviewTarget;
  path: string;
  /** As the address bar shows it: without the scheme. */
  text: string;
  /** What the page is called, when the person chose it by name (a server, a pinned page). */
  title?: string;
}

const LOCAL = /^(localhost|127\.0\.0\.1)$/i;
const HOST = /^(localhost|\d{1,3}(\.\d{1,3}){3}|[a-z0-9-]+(\.[a-z0-9-]+)+)(:\d{2,5})?$/i;

export const parseAddress = (raw: string): IAddress | null => {
  const typed = raw.trim();
  const bare = typed.replace(/^:/, '');
  if (/^\d{2,5}$/.test(bare)) {
    const port = Number(bare);
    if (port < 1 || port > 65535) return null;
    return { target: { kind: 'port', port }, path: '/', text: `localhost:${port}` };
  }
  const secure = !/^http:\/\//i.test(typed);
  const rest = typed.replace(/^https?:\/\//i, '');
  const slash = rest.indexOf('/');
  const host = slash < 0 ? rest : rest.slice(0, slash);
  const path = slash < 0 ? '/' : rest.slice(slash);
  if (!HOST.test(host) || /\s/.test(path)) return null;
  const [name, portText] = host.split(':');
  const text = `${host}${path === '/' ? '' : path}`;
  if (LOCAL.test(name)) {
    if (!portText) return null;
    return { target: { kind: 'port', port: Number(portText) }, path, text };
  }
  return {
    target: { kind: 'url', origin: `${secure ? 'https' : 'http'}://${host.toLowerCase()}` },
    path,
    text,
  };
};

/** What the omnibox made of the text: a place to go, or words to search for. */
export type TInput =
  | { kind: 'empty' }
  | { kind: 'port' | 'url'; address: IAddress }
  | { kind: 'search'; q: string; address: IAddress };

/** The search the pane opens: DuckDuckGo's plain page, which does not need scripts and is not afraid of frames. */
export const SEARCH_ORIGIN = 'https://html.duckduckgo.com';

/** The address bar shows a search as «? words», so that Enter on it asks the same again. */
export const searchAddress = (q: string): IAddress => ({
  target: { kind: 'url', origin: SEARCH_ORIGIN },
  path: `/html/?q=${encodeURIComponent(q)}`,
  text: `? ${q}`,
});

export const classifyInput = (raw: string): TInput => {
  const typed = raw.trim();
  if (typed === '') return { kind: 'empty' };
  if (typed.startsWith('?')) {
    const q = typed.slice(1).trim();
    return q === '' ? { kind: 'empty' } : { kind: 'search', q, address: searchAddress(q) };
  }
  const address = parseAddress(typed);
  if (address) return { kind: address.target.kind === 'port' ? 'port' : 'url', address };
  return { kind: 'search', q: typed, address: searchAddress(typed) };
};

/** Where Enter goes with this text; null for nothing typed. */
export const resolveInput = (raw: string): IAddress | null => {
  const input = classifyInput(raw);
  return input.kind === 'empty' ? null : input.address;
};
