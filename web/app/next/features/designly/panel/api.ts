// A site board's comments and shared settings over the runtime (`<platform>/design/site/*`,
// cmd/runtime/routes_design_site.go; the hub saves the writes as edits of the project): read as they come, a
// refusal is a thrown RuntimeError.
import { platformPath } from '@/next/core/net/machine';
import { postJson, runtimeJson } from '@/next/core/net/runtime-json';
import { readOrSnapshot } from '@/next/features/designly/snapshot-read';
import type { TTweakValue } from '@/next/features/designly/page-bridge';
import type { MachineId } from '@/next/protocol/v2';

type TPath = `/${string}`;
const site = (path: string): TPath => platformPath(`/design/site${path}` as TPath);
const jsonInit = (method: 'PUT' | 'PATCH', body: unknown): RequestInit => ({
  method,
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(body),
});

/** The emojis a comment may get (internal/design/site_comments.go SiteReactions). */
export const REACTIONS = ['👍', '❤️', '🎉', '👀', '✅', '🤔'] as const;

export interface IPerson {
  name: string;
  email: string;
}

export interface IReaction {
  e: string;
  n: number;
  mine: boolean;
  names: string[];
}

export interface ISiteComment {
  id: string;
  board: string;
  /** The thread's first comment for a reply; '' for the first one. */
  parent: string;
  /** Where on the page, in percents of its width and height. */
  x: number;
  y: number;
  /** The data-block it points at; '' a point only. */
  block: string;
  label: string;
  text: string;
  by: IPerson;
  byAgent: boolean;
  at: string;
  resolved: boolean;
  mentions: IPerson[];
  agent: boolean;
  reactions: IReaction[];
}

/** A thread: its first comment and the replies under it, oldest first. */
export interface IThread {
  root: ISiteComment;
  replies: ISiteComment[];
}

const isRecord = (v: unknown): v is Record<string, unknown> =>
  typeof v === 'object' && v !== null && !Array.isArray(v);
const str = (v: unknown) => (typeof v === 'string' ? v : '');
const num = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);
const personOf = (v: unknown): IPerson =>
  isRecord(v) ? { name: str(v.name), email: str(v.email) } : { name: '', email: '' };

export const readSiteComment = (v: unknown): ISiteComment | null => {
  if (!isRecord(v) || typeof v.id !== 'string' || v.id === '') return null;
  return {
    id: v.id,
    board: str(v.board),
    parent: str(v.parent),
    x: num(v.x),
    y: num(v.y),
    block: str(v.block),
    label: str(v.label),
    text: str(v.text),
    by: personOf(v.by),
    byAgent: v.byAgent === true,
    at: str(v.at),
    resolved: v.resolved === true,
    mentions: Array.isArray(v.mentions) ? v.mentions.map(personOf) : [],
    agent: v.agent === true,
    reactions: Array.isArray(v.reactions)
      ? v.reactions.filter(isRecord).map((r) => ({
          e: str(r.e),
          n: num(r.n),
          mine: r.mine === true,
          names: Array.isArray(r.names) ? r.names.map(str) : [],
        }))
      : [],
  };
};

/** The comments put together into threads, the newest thread first; a reply whose thread is gone is dropped. */
export const threadsOf = (comments: readonly ISiteComment[]): IThread[] => {
  const byId = new Map<string, IThread>();
  for (const c of comments) if (c.parent === '') byId.set(c.id, { root: c, replies: [] });
  for (const c of comments) if (c.parent !== '') byId.get(c.parent)?.replies.push(c);
  return [...byId.values()].sort((a, b) => (a.root.at < b.root.at ? 1 : a.root.at > b.root.at ? -1 : 0));
};

export const fetchSiteComments = async (
  m: MachineId,
  artifact: string,
  signal?: AbortSignal,
): Promise<ISiteComment[]> => {
  const answer = await readOrSnapshot(m, site(`/comments?artifact=${encodeURIComponent(artifact)}`), {
    signal,
    cache: 'no-store',
  });
  const list = isRecord(answer) && Array.isArray(answer.comments) ? answer.comments : [];
  return list.map(readSiteComment).filter((c): c is ISiteComment => c !== null);
};

export interface ICommentDraft {
  board: string;
  x: number;
  y: number;
  block: string;
  label: string;
  text: string;
  parent: string;
  mentions: IPerson[];
  agent: boolean;
}

const must = (v: ISiteComment | null): ISiteComment => {
  if (v === null) throw new Error('bad answer');
  return v;
};

export const addSiteComment = async (m: MachineId, artifact: string, draft: ICommentDraft) =>
  must(readSiteComment(await runtimeJson(m, site('/comments'), postJson({ artifact, ...draft }))));

export const resolveSiteComment = async (m: MachineId, artifact: string, id: string, resolved: boolean) =>
  must(
    readSiteComment(await runtimeJson(m, site('/comments'), jsonInit('PATCH', { artifact, id, resolved }))),
  );

export const reactSiteComment = async (m: MachineId, artifact: string, id: string, react: string) =>
  must(readSiteComment(await runtimeJson(m, site('/comments'), jsonInit('PATCH', { artifact, id, react }))));

/** The comment that calls @harnessly goes to the agent of the chat: the person's own machine is asked. */
export const tellAgent = (m: MachineId, artifact: string, id: string, chat: string) =>
  runtimeJson(m, site('/comments/agent'), postJson({ artifact, id, chat }));

// ---- the boards' shared settings

/** By board path, the settings picked (name → value). */
export type TSiteProps = Record<string, Record<string, TTweakValue>>;

const readProps = (v: unknown): TSiteProps => {
  const out: TSiteProps = {};
  if (!isRecord(v) || !isRecord(v.boards)) return out;
  for (const [board, values] of Object.entries(v.boards)) {
    if (!isRecord(values)) continue;
    const clean: Record<string, TTweakValue> = {};
    for (const [k, x] of Object.entries(values))
      if (typeof x === 'string' || typeof x === 'number' || typeof x === 'boolean') clean[k] = x;
    out[board] = clean;
  }
  return out;
};

export const fetchSiteProps = async (m: MachineId, artifact: string, signal?: AbortSignal) =>
  readProps(
    await readOrSnapshot(m, site(`/props?artifact=${encodeURIComponent(artifact)}`), {
      signal,
      cache: 'no-store',
    }),
  );

/** Puts a board's values whole: a name left out goes back to the board's own; {} the board as it is drawn. */
export const putSiteProps = async (
  m: MachineId,
  artifact: string,
  board: string,
  values: Record<string, TTweakValue>,
) => readProps(await runtimeJson(m, site('/props'), jsonInit('PUT', { artifact, board, values })));

/** What «Макет» → «Страницы» does with a page (internal/design/site_pages.go): the same commands as the agents'
 *  `design page create/rename/remove`, a duplicate being a variant of the page. A page is named by its file. */
export type TPageAction =
  | { action: 'create'; title: string }
  | { action: 'duplicate'; page: string }
  | { action: 'rename'; page: string; title: string }
  | { action: 'remove'; page: string };

/** Does it; the answer is the page's file. */
export const changeSitePages = async (m: MachineId, artifact: string, act: TPageAction): Promise<string> => {
  const answer = await runtimeJson<unknown>(m, site('/pages'), postJson({ artifact, ...act }));
  const page = isRecord(answer) && isRecord(answer.page) ? answer.page : null;
  return page !== null && typeof page.file === 'string' ? page.file : '';
};
