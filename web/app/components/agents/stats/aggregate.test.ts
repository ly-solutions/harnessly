import { describe, expect, it } from 'vitest';
import { mergeTeam } from '@/components/agents/stats/aggregate';
import type { ITeamUsage } from '@/lib/hl-usage/types';

const team = (hubMissing?: string[]): ITeamUsage => ({
  viewer: 'anna@example.com',
  runtimes: [
    {
      person: 'anna@example.com',
      machineId: 'hub',
      machineName: 'hub',
      report: {
        version: 1,
        person: 'anna@example.com',
        machine: 'hub',
        generatedAt: '2026-11-10T00:00:00Z',
        timezone: 'Europe/Moscow',
        days: {},
        usage: [['2026-11-09', 'claude', 'claude-opus-4-6', '', 1, 2, 3, 4, 5, 6]],
        sessions: [],
        collecting: null,
        ...(hubMissing ? { hubMissing } : {}),
      },
    },
  ],
});

describe('mergeTeam', () => {
  it('collects the blocks the hub has no facts for', () => {
    const data = mergeTeam(team(['punchcard', 'projects']));
    expect([...data.missing].sort()).toEqual(['projects', 'punchcard']);
    expect(data.facts).toHaveLength(1);
  });
  it('reads the account a row was paid by, and none from a row without one', () => {
    const data = team();
    data.runtimes[0].report?.usage.push([
      '2026-11-09',
      'claude',
      'kimi-k3.1-pro',
      '',
      1,
      2,
      3,
      4,
      5,
      6,
      'Kimi рабочий',
    ]);
    expect(mergeTeam(data).facts.map((f) => f.account)).toEqual([null, 'Kimi рабочий']);
  });

  it('a runtime report names none', () => {
    expect(mergeTeam(team()).missing.size).toBe(0);
  });
});
