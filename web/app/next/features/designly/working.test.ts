import { describe, expect, it } from 'vitest';
import type { IDesign } from '@/next/features/designly/api';
import { LIVE_CHAT_MS, liveChats, workingDesigns } from '@/next/features/designly/working';
import type { Conversation } from '@/next/protocol/v2';

const design = (id: string, updatedAt: number, extra: Partial<IDesign> = {}): IDesign => ({
  id,
  title: id,
  updatedAt,
  kind: 'site',
  pinned: false,
  cover: null,
  projectPath: `/srv/design/${id}`,
  boards: [],
  convs: [],
  ...extra,
});

const conv = (id: string, lastActivityAt: number, working = false) =>
  ({
    id,
    lastActivityAt,
    state: working ? { k: 'working', since: 0, turnId: null, compacting: false } : { k: 'idle' },
  }) as unknown as Conversation;

describe('liveChats', () => {
  it('keeps the chats at work or active within a day', () => {
    const now = 10 * LIVE_CHAT_MS;
    const live = liveChats(
      [conv('fresh', now - 1000), conv('old', now - 2 * LIVE_CHAT_MS), conv('busy', 0, true)],
      now,
    );
    expect([...live].sort()).toEqual(['busy', 'fresh']);
  });
});

describe('workingDesigns', () => {
  it('keeps only the designs a live chat works on, and the open one', () => {
    const rows = workingDesigns(
      [
        design('idle', 30),
        design('own', 10, { convs: ['tab-1'] }),
        design('touched', 20, { touchedBy: ['tab-2'] }),
        design('stale', 40, { convs: ['tab-old'] }),
        design('open', 5),
      ],
      new Set(['tab-1', 'tab-2']),
      'open',
    );
    expect(rows.map((r) => r.id)).toEqual(['touched', 'own', 'open']);
  });

  it('puts the pinned first, then the latest changed', () => {
    const rows = workingDesigns(
      [
        design('old', 10, { convs: ['a'] }),
        design('new', 30, { convs: ['b'] }),
        design('pinned', 5, { convs: ['c'], pinned: true }),
      ],
      new Set(['a', 'b', 'c']),
      null,
    );
    expect(rows.map((r) => r.id)).toEqual(['pinned', 'new', 'old']);
  });
});
