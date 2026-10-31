// The servers a page leans on: an MFE shell loads its remotes from other ports (http://localhost:6201/remoteEntry.json).
// The proxy can only turn those addresses into slot paths for ports named in the opening, so they are named here.
import type { IListeningPort } from '@/next/core/net/preview';

/** The runtime takes at most this many. */
export const MAX_NEIGHBOURS = 7;

/** Folders below these belong to one repository root: a monorepo's apps and packages share it. */
const SUB_ROOTS = /\/(apps|packages|libs|services|projects)(\/|$)/;
/** A worktree is a root of its own: <repo>/.worktrees/<task>. */
const WORKTREE = /^(.*\/\.worktrees\/[^/]+)(\/|$)/;

/** The repository folder of a process's working folder, as far as the path tells; null when there is none. */
export const repoRootOf = (cwd: string | null): string | null => {
  if (cwd === null) return null;
  const folder = cwd.replace(/\/+$/, '');
  if (folder === '') return null;
  const worktree = WORKTREE.exec(folder);
  const base = worktree ? worktree[1] : folder;
  const sub = SUB_ROOTS.exec(base);
  const root = sub ? base.slice(0, sub.index) : base;
  return root === '' ? null : root;
};

const inside = (a: string, b: string) => a === b || a.startsWith(`${b}/`) || b.startsWith(`${a}/`);

/** A server that answers: a silent or unprobed port would only add a dead path to the claim. */
const answers = (p: IListeningPort) => p.responding && p.kind !== 'unknown';

/** The ports the page at `port` leans on; none when the port is not among the listening ones. */
export const neighboursOf = (all: readonly IListeningPort[], port: number): number[] => {
  const target = all.find((p) => p.port === port);
  if (!target) return [];
  const near = (p: IListeningPort): boolean => {
    if (target.group === 'chat') return p.group === 'chat';
    if (target.group === 'agent')
      return (
        p.group === 'agent' && p.owner?.repo === target.owner?.repo && p.owner?.task === target.owner?.task
      );
    // Servers started from a shared checkout belong to no chat: the same repository is what ties them.
    const mine = repoRootOf(target.cwd);
    const theirs = repoRootOf(p.cwd);
    return p.group === 'other' && mine !== null && theirs !== null && inside(mine, theirs);
  };
  return all
    .filter((p) => p.port !== port && answers(p) && near(p))
    .map((p) => p.port)
    .slice(0, MAX_NEIGHBOURS);
};
