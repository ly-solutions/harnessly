// @vitest-environment jsdom
import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { TRoute } from '@/next/routes/routes';

const sendView = vi.fn();
const view = { current: undefined as Record<string, unknown> | undefined };
vi.mock('@/next/commands', () => ({ setConversationView: (...a: unknown[]) => sendView(...a) }));
vi.mock('@/next/stores/machines', () => ({ hasFeature: () => true }));
vi.mock('@/next/stores/conversations', () => ({
  useConversation: () => ({ id: 'tab-1', machineId: 'm1', view: view.current }),
  useConversationList: () => [{ id: 'tab-1', machineId: 'm1', view: view.current }],
}));

import { EXPORT_MS, useChatViewSync } from '@/next/bindings/use-chat-view-sync';
import { keepScreen, keptScreen } from '@/next/stores/chat-screen';
import { rememberPlace, forgetPlaces } from '@/next/features/chat/feed-place';

const CHAT: TRoute = { kind: 'chat', id: 'tab-1' } as TRoute;

beforeEach(() => {
  vi.useFakeTimers();
  localStorage.clear();
  sessionStorage.clear();
  forgetPlaces();
  sendView.mockReset();
  view.current = undefined;
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe('the chat view on the machine', () => {
  it('sends nothing for a chat nobody arranged, and one write for a change, not one per tick', () => {
    renderHook(() => useChatViewSync(CHAT));
    act(() => void vi.advanceTimersByTime(EXPORT_MS * 2));
    expect(sendView).not.toHaveBeenCalled();
    keepScreen('tab-1', '/designly/d1?chat=tab-1');
    rememberPlace('tab-1', { k: 'at', key: 'row-1', offset: 3 });
    act(() => void vi.advanceTimersByTime(EXPORT_MS * 3));
    expect(sendView).toHaveBeenCalledTimes(1);
    const [machine, conv, body] = sendView.mock.calls[0] as [string, string, Record<string, unknown>];
    expect([machine, conv]).toEqual(['m1', 'tab-1']);
    expect(body.screen).toBe('/designly/d1?chat=tab-1');
    expect(typeof body.at).toBe('number');
  });

  it('takes a newer view of the machine into the stores', () => {
    view.current = {
      at: 5000,
      screen: '/apps/acme.notes?chat=tab-1',
      layouts: {},
      pane: { open: false, maxed: false, width: null, tabs: [], active: '' },
      feed: { k: 'bottom' },
    };
    renderHook(() => useChatViewSync({ kind: 'home' } as TRoute));
    expect(keptScreen('tab-1')?.kind).toBe('app');
  });
});
