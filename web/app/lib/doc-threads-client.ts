import { appFetch, prefixAppPath } from '@/lib/app-path';
import type {
  IDocAccess,
  IDocDetailResponse,
  IDocDiffResponse,
  IDocListResponse,
  IDocNative,
  IDocPeopleResponse,
  IDocSource,
  IDocThread,
  IDocVersionsResponse,
  TDocThreadStatus,
} from '@/types/doc-threads';

const BASE = '/api/platform/docs';

/** A refusal of the gateway: its words, the HTTP status and its code (`version_conflict` for an edit made on an old version). */
export class DocThreadsError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string | null,
  ) {
    super(message);
  }
}

/** The gateway says what went wrong in `{error}`; that text is shown as it is, the status line only when there is none. */
const asJson = async <T>(response: Response): Promise<T> => {
  if (!response.ok) {
    const said = await response
      .json()
      .then((body: { error?: unknown; code?: unknown }) => ({
        error: typeof body?.error === 'string' ? body.error : '',
        code: typeof body?.code === 'string' ? body.code : null,
      }))
      .catch(() => ({ error: '', code: null }));
    throw new DocThreadsError(
      said.error || `${response.status} ${response.statusText}`,
      response.status,
      said.code,
    );
  }
  return response.json() as Promise<T>;
};

export const fetchDocList = async (): Promise<IDocListResponse> => asJson(await appFetch(BASE));

export const fetchDocDetail = async (slug: string): Promise<IDocDetailResponse> =>
  asJson(await appFetch(`${BASE}/${encodeURIComponent(slug)}`));

export const createDocThread = async (
  slug: string,
  anchor: IDocThread['anchor'],
  body: string,
): Promise<{ thread: IDocThread }> =>
  asJson(
    await appFetch(`${BASE}/${encodeURIComponent(slug)}/threads`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ anchor, body }),
    }),
  );

export const replyToDocThread = async (
  slug: string,
  threadId: string,
  body: string,
): Promise<{ thread: IDocThread }> =>
  asJson(
    await appFetch(`${BASE}/${encodeURIComponent(slug)}/threads/${encodeURIComponent(threadId)}/comments`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ body }),
    }),
  );

export const setDocThreadStatus = async (
  slug: string,
  threadId: string,
  status: TDocThreadStatus,
): Promise<{ thread: IDocThread }> =>
  asJson(
    await appFetch(`${BASE}/${encodeURIComponent(slug)}/threads/${encodeURIComponent(threadId)}/status`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ status }),
    }),
  );

export const markDocRead = async (slug: string): Promise<void> => {
  // A change declares JSON: the hub's CSRF guard refuses a bare POST.
  await appFetch(`${BASE}/${encodeURIComponent(slug)}/read`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: '{}',
  });
};

/** The page of one version when `version` is given (its id), else the current one. */
export const docContentSrc = (slug: string, version: string | null = null): string =>
  prefixAppPath(
    `${BASE}/${encodeURIComponent(slug)}/content${version ? `?v=${encodeURIComponent(version)}` : ''}`,
  );

/** The object read by the runtime itself: Markdown or blocks of a document, one page (`cursor`) of a board. */
export const fetchDocNative = async (
  slug: string,
  opts: { version?: string | null; cursor?: string | null } = {},
): Promise<IDocNative> => {
  const q = new URLSearchParams();
  if (opts.version) q.set('v', opts.version);
  if (opts.cursor) q.set('cursor', opts.cursor);
  const tail = q.size ? `?${q.toString()}` : '';
  return asJson(await appFetch(`${BASE}/${encodeURIComponent(slug)}/native${tail}`));
};

/** An image of a board, read by the runtime with the person's own provider connection. */
export const docAssetSrc = (slug: string, asset: string): string =>
  prefixAppPath(`${BASE}/${encodeURIComponent(slug)}/native/asset?id=${encodeURIComponent(asset)}`);

/**
 * Whether the runtime can give the provider's embed address at all. The frame itself cannot be asked whether it loaded,
 * so the content route is asked without following its redirect: a redirect (an opaque answer) is an address, anything
 * else is the runtime refusing.
 */
export const probeDocEmbed = async (slug: string): Promise<boolean> => {
  try {
    const response = await appFetch(`${BASE}/${encodeURIComponent(slug)}/content`, { redirect: 'manual' });
    return response.type === 'opaqueredirect' || response.ok;
  } catch {
    return false;
  }
};

export const fetchDocVersions = async (slug: string): Promise<IDocVersionsResponse> =>
  asJson(await appFetch(`${BASE}/${encodeURIComponent(slug)}/versions`));

export const fetchDocDiff = async (slug: string, from: string, to: string): Promise<IDocDiffResponse> =>
  asJson(
    await appFetch(
      `${BASE}/${encodeURIComponent(slug)}/diff?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
    ),
  );

/** People for the @ list; `q` is what was typed after the `@`. */
export const fetchDocPeople = async (q: string): Promise<IDocPeopleResponse> =>
  asJson(await appFetch(`${BASE}/people?q=${encodeURIComponent(q)}`));

export const fetchDocAccess = async (slug: string): Promise<IDocAccess> =>
  asJson(await appFetch(`${BASE}/${encodeURIComponent(slug)}/access`));

/** Opens (true) or closes (false) the public link; closing revokes the old address for good. */
export const setDocPublic = async (slug: string, open: boolean): Promise<IDocAccess> =>
  asJson(
    await appFetch(`${BASE}/${encodeURIComponent(slug)}/access`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ public: open }),
    }),
  );

/** The Markdown of the current version and whether the person may edit it. */
export const fetchDocSource = async (slug: string): Promise<IDocSource> =>
  asJson(await appFetch(`${BASE}/${encodeURIComponent(slug)}/source`));

/** Saves the edited Markdown as a new version; refused with `version_conflict` (409) when `baseVersion` is no longer current. */
export const saveDocSource = async (
  slug: string,
  markdown: string,
  baseVersion: number,
): Promise<{ version: number }> =>
  asJson(
    await appFetch(`${BASE}/${encodeURIComponent(slug)}/source`, {
      method: 'PUT',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ markdown, baseVersion }),
    }),
  );

/** Saves one block of a page edited in place as a new version; `before` is its text as the page showed it. */
export const saveDocBlock = async (
  slug: string,
  edit: { index: number; before: string; html: string; baseVersion: number },
): Promise<{ version: number }> =>
  asJson(
    await appFetch(`${BASE}/${encodeURIComponent(slug)}/source/block`, {
      method: 'PUT',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(edit),
    }),
  );
