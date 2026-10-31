import { describe, expect, it } from 'vitest';
import { neighboursOf, repoRootOf } from '@/next/features/pane/neighbours';
import type { IListeningPort } from '@/next/core/net/preview';

const port = (n: number, over: Partial<IListeningPort> = {}): IListeningPort => ({
  port: n,
  pid: n + 1000,
  owner: null,
  command: 'node',
  cwd: null,
  group: 'other',
  kind: 'site',
  responding: true,
  title: null,
  description: null,
  icon: null,
  ...over,
});

describe('repoRootOf', () => {
  it('cuts a monorepo app down to the repository', () => {
    expect(repoRootOf('/home/u/ws/shop-ui/apps/admin')).toBe('/home/u/ws/shop-ui');
    expect(repoRootOf('/home/u/ws/shop-ui/')).toBe('/home/u/ws/shop-ui');
  });
  it('keeps a worktree as a root of its own', () => {
    expect(repoRootOf('/r/app/.worktrees/t1/apps/shell')).toBe('/r/app/.worktrees/t1');
  });
  it('is null with no folder', () => {
    expect(repoRootOf(null)).toBeNull();
    expect(repoRootOf('')).toBeNull();
  });
});

describe('neighboursOf', () => {
  it('takes the chat servers of a chat port', () => {
    const all = [
      port(4400, { group: 'chat' }),
      port(4401, { group: 'chat' }),
      port(4402, { group: 'chat', kind: 'unknown' }),
      port(4403, { group: 'chat', responding: false }),
      port(5000),
    ];
    expect(neighboursOf(all, 4400)).toEqual([4401]);
  });

  it('takes the servers of the same agent task for an agent port', () => {
    const own = { repo: 'a', task: 't1' };
    const all = [
      port(1, { group: 'agent', owner: own }),
      port(2, { group: 'agent', owner: own }),
      port(3, { group: 'agent', owner: { repo: 'a', task: 't2' } }),
    ];
    expect(neighboursOf(all, 1)).toEqual([2]);
  });

  it('ties servers of one repository when they belong to no chat', () => {
    const all = [
      port(6200, { cwd: '/ws/shop-ui/apps/shell' }),
      port(6201, { cwd: '/ws/shop-ui/apps/admin' }),
      port(6300, { cwd: '/ws/other-repo' }),
      port(6400, { cwd: null }),
      port(6500, { cwd: '/ws/shop-ui/apps/x', group: 'chat' }),
    ];
    expect(neighboursOf(all, 6200)).toEqual([6201]);
  });

  it('has none for a port that is not listening or has no folder', () => {
    expect(neighboursOf([port(1)], 2)).toEqual([]);
    expect(neighboursOf([port(1), port(2)], 1)).toEqual([]);
  });

  it('gives at most seven', () => {
    const all = [
      port(1, { group: 'chat' }),
      ...Array.from({ length: 10 }, (_, i) => port(10 + i, { group: 'chat' })),
    ];
    expect(neighboursOf(all, 1)).toHaveLength(7);
  });
});
