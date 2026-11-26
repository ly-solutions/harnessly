import { describe, expect, it, vi } from 'vitest';

import { connectBuiltin } from '@/next/features/app-views/builtin-app';
import type { IRoutineInfo, IRoutineRun } from '@/next/features/app-views/routines';
import {
  allArtifacts,
  isOver,
  runChatOf,
  runState,
  sourcesOf,
  whenText,
} from '@/next/features/app-views/routines-app/model';

// The routines app: a builtin app that reads the routines only through hl.routines, with the rights of its
// manifest, and draws a run's chat from what that says.
const routine: IRoutineInfo = {
  id: 'r1',
  name: 'Дайджест',
  enabled: true,
  model: 'sonnet',
  trigger: { schedule: { cron: '0 9 * * 1-5' } },
  routine: { words: 'по будням в 9:00' },
  next: ['2026-11-27T06:00:00Z'],
  stats: { runs: 2, ok: 1, quiet: 1, failed: 0 },
};
const run = (id: string, over: Partial<IRoutineRun> = {}): IRoutineRun => ({
  id,
  ruleId: 'r1',
  fingerprint: 'schedule:1',
  status: 'done',
  outcome: 'ok',
  conversationId: `tab-${id}`,
  createdAt: '2026-11-26T06:00:00Z',
  ...over,
});

describe('hl.routines of the routines app', () => {
  it('reads a chat, a routine and its runs under routines.read, and turns it off under routines.write', async () => {
    const routines = vi.fn(async (op: string) => {
      if (op === 'forChat') return { routine, run: run('a'), runs: [run('a')] };
      if (op === 'runs') return { runs: [run('a')] };
      return { routine: { ...routine, enabled: false } };
    });
    const hl = connectBuiltin('harnessly.routines', { handlers: { routines }, context: { target: 'side' } });
    expect(hl.has('routines.read')).toBe(true);
    expect(hl.has('routines.write')).toBe(true);
    expect(hl.has('chat.read')).toBe(false);
    expect((await hl.routines.forChat('tab-a')).routine?.name).toBe('Дайджест');
    expect(await hl.routines.runs('r1', { limit: 500 })).toHaveLength(1);
    expect((await hl.routines.setEnabled('r1', false)).enabled).toBe(false);
    expect(routines).toHaveBeenCalledWith('forChat', { chat: 'tab-a' });
    expect(routines).toHaveBeenCalledWith('runs', { id: 'r1', limit: 100 });
    expect(routines).toHaveBeenCalledWith('setEnabled', { id: 'r1', enabled: false });
  });

  it('is refused to an app without the right', async () => {
    const routines = vi.fn(async () => ({}));
    const hl = connectBuiltin('harnessly.sinopia', { handlers: { routines } });
    expect(hl.has('routines.read')).toBe(false);
    await expect(hl.routines.get('r1')).rejects.toMatchObject({ code: 'permission_denied' });
    expect(routines).not.toHaveBeenCalled();
  });
});

describe('what the routines app shows', () => {
  it('is a run chat only with a routine and a run', () => {
    expect(runChatOf({ routine: null })).toBeNull();
    expect(runChatOf({ routine, run: run('a') })?.runs).toEqual([]);
  });

  it('says how a run went, and when it is over', () => {
    expect(runState(run('a', { outcome: 'quiet' }))).toBe('quiet');
    expect(runState(run('a', { outcome: undefined, status: 'running' }))).toBe('running');
    expect(runState(run('a', { outcome: undefined, status: 'failed' }))).toBe('failed');
    expect(isOver(run('a', { outcome: undefined, status: 'running' }))).toBe(false);
    expect(isOver(run('a'))).toBe(true);
  });

  it('lists every run’s artifacts once, newest first, and borrows the sources of the last run that named them', () => {
    const doc = { kind: 'doc', title: 'Дайджест', url: 'https://docs.example.com/d/1' };
    const runs = [
      run('b', { result: { artifacts: [doc] } }),
      run('a', {
        result: {
          artifacts: [doc, { kind: 'mr', title: 'MR 12', url: 'https://git.example.com/mr/12' }],
          sources: [{ kind: 'link', title: 'arXiv', url: 'https://arxiv.org' }],
        },
      }),
    ];
    expect(allArtifacts(runs).map((x) => `${x.run.id}:${x.artifact.title}`)).toEqual([
      'b:Дайджест',
      'a:MR 12',
    ]);
    expect(sourcesOf({ routine, run: runs[0], runs }).map((s) => s.title)).toEqual(['arXiv']);
  });

  it('writes a moment in the person’s words', () => {
    const now = new Date(2026, 10, 26, 12, 0);
    const words = { today: 'сегодня', tomorrow: 'завтра', yesterday: 'вчера' };
    expect(whenText(new Date(2026, 10, 27, 9, 0).toISOString(), now, words, 'ru')).toBe('завтра, 09:00');
    expect(whenText(new Date(2026, 10, 26, 9, 5).toISOString(), now, words, 'ru')).toBe('сегодня, 09:05');
    expect(whenText(new Date(2026, 10, 20, 9, 0).toISOString(), now, words, 'ru')).toMatch(
      /^20 нояб\.?, 09:00$/,
    );
  });
});
