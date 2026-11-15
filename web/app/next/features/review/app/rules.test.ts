import { describe, expect, it } from 'vitest';
import {
  NO_RULES,
  instructionsFor,
  parseRules,
  repoKeyOf,
  visibleRows,
  type IReviewRules,
} from '@/next/features/review/app/rules';
import type { IMrMeta } from '@/next/features/review/model';
import type { AppRow } from '@/next/protocol/v2';

const row = (key: string, author: string, bot: boolean) => ({ key, author, bot }) as AppRow;
const rules = (over: Partial<IReviewRules>): IReviewRules => ({ ...NO_RULES, ...over });

describe('review rules', () => {
  it('reads the runtime answer and refuses what is not one', () => {
    expect(
      parseRules({ instructions: 'a', repos: { k: 'b', bad: 1 }, hideBots: true, bots: ['x', 2] }),
    ).toEqual({
      instructions: 'a',
      repos: { k: 'b' },
      hideBots: true,
      bots: ['x'],
    });
    expect(parseRules({ error: 'not-found' })).toBeNull();
    expect(parseRules(null)).toBeNull();
  });

  it('keys a repository by its host and project', () => {
    const meta = {
      kind: 'mr',
      url: 'https://GitLab.example.com/Team/App/-/merge_requests/4',
      project: 'Team/App',
    };
    expect(repoKeyOf(meta as IMrMeta)).toBe('gitlab.example.com/team/app');
    expect(
      repoKeyOf({
        kind: 'local',
        repo: 'app',
        root: '/w',
        branch: null,
        base: null,
        commits: [],
        uncommitted: true,
      }),
    ).toBeNull();
  });

  it('gives the general instructions, then the repository own', () => {
    const r = rules({ instructions: ' общие ', repos: { 'h/p': 'свои' } });
    expect(instructionsFor(r, 'h/p')).toBe('общие\n\nсвои');
    expect(instructionsFor(r, 'h/other')).toBe('общие');
    expect(instructionsFor(r, null)).toBe('общие');
    expect(instructionsFor(NO_RULES, 'h/p')).toBe('');
  });

  it('hides the bots only when asked, and never from a search', () => {
    const rows = [row('1', 'Анна', false), row('2', 'renovate', true), row('3', 'Release Robot', false)];
    expect(visibleRows(rows, NO_RULES, '')).toHaveLength(3);
    const hide = rules({ hideBots: true, bots: ['release robot'] });
    expect(visibleRows(rows, hide, '').map((r) => r.key)).toEqual(['1']);
    expect(visibleRows(rows, hide, 'renovate')).toHaveLength(3);
  });
});
