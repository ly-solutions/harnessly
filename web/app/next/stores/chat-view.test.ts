// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest';
import { rememberPlace, forgetPlaces, placeOf } from '@/next/features/chat/feed-place';
import { keepLayout, keptLayout } from '@/next/features/designly/site-layout';
import { chatPaneOf, chatPanes, resetChatPanes } from '@/next/stores/pane-state';
import { keepScreen, keptScreen } from '@/next/stores/chat-screen';
import { applyView, isPlain, noteLayoutKey, parseView, seenAt, viewOf } from '@/next/stores/chat-view';

const LAYOUT = {
  mode: 'float',
  min: false,
  dock: 'right',
  chatW: 400,
  float: { x: 1, y: 2, w: 300, h: 300 },
  opened: 'chat',
  chosen: true,
} as const;

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  resetChatPanes();
  forgetPlaces();
});

describe('the view of a chat', () => {
  it('is plain for a chat nobody arranged', () => {
    expect(isPlain(viewOf('tab-1'))).toBe(true);
  });

  it('gathers the screen, the layouts, the pane and the feed place of the chat', () => {
    keepScreen('tab-1', '/designly/d1?chat=tab-1');
    noteLayoutKey('tab-1', 'k1');
    keepLayout('k1', LAYOUT);
    chatPanes.update('tab-1', null, (p) => ({ ...p, open: true, width: 480 }));
    rememberPlace('tab-1', { k: 'at', key: 'row-7', offset: 12 });
    const view = viewOf('tab-1');
    expect(view.screen).toBe('/designly/d1?chat=tab-1');
    expect(view.layouts.k1?.dock).toBe('right');
    expect(view.pane).toMatchObject({ open: true, width: 480 });
    expect(view.feed).toEqual({ k: 'at', key: 'row-7', offset: 12 });
    expect(isPlain(view)).toBe(false);
  });

  it('is put back into the stores of another device as it was, and its version is noted', () => {
    const view = parseView({
      at: 1000,
      screen: '/apps/acme.notes?chat=tab-2',
      layouts: { k2: LAYOUT },
      pane: { open: true, maxed: false, width: null, tabs: [], active: '' },
      feed: { k: 'at', key: 'row-3', offset: 4 },
    });
    expect(view).not.toBeNull();
    if (!view) return;
    applyView('tab-2', null, view);
    expect(keptScreen('tab-2')?.kind).toBe('app');
    expect(keptLayout('k2')?.chatW).toBe(400);
    expect(chatPaneOf('tab-2').open).toBe(true);
    expect(placeOf('tab-2')).toEqual({ k: 'at', key: 'row-3', offset: 4 });
    expect(seenAt('tab-2')).toBe(1000);
    // What was put in reads back as the same view.
    expect(viewOf('tab-2').screen).toBe('/apps/acme.notes?chat=tab-2');
  });

  it('leaves out the pieces that are not one, and is nothing without a version', () => {
    expect(parseView({ screen: '/x' })).toBeNull();
    expect(parseView('x')).toBeNull();
    const view = parseView({
      at: 5,
      screen: '/designly/d1?chat=tab-9',
      layouts: { a: 7 },
      pane: 'x',
      feed: 3,
    });
    expect(view?.layouts).toEqual({});
    expect(view?.pane.tabs).toEqual([]);
    expect(view?.feed.k).toBe('bottom');
    expect(parseView({ at: 5, screen: '/tab-1' })?.screen).toBeNull();
  });

  it('a newer view from the machine clears the screen when the other device left it', () => {
    keepScreen('tab-3', '/designly/d1?chat=tab-3');
    const view = parseView({ at: 9, screen: null });
    if (view) applyView('tab-3', null, view);
    expect(keptScreen('tab-3')).toBeNull();
  });
});
