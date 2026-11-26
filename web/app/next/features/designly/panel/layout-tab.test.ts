// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';
import {
  askLayout,
  clearLayoutAsk,
  offerLayout,
  setLayoutFronted,
  setPaneShows,
  useLayoutEntry,
} from '@/next/features/designly/panel/layout-tab';
import { renderHook, act } from '@testing-library/react';
import { whoOf } from '@/next/features/designly/panel/when';
import { t } from '@/next/i18n';

describe('«Макет» in the chat’s tools panel (HRN-1764)', () => {
  it('is offered while the design is open beside the chat, and takes the design’s asks', () => {
    const { result } = renderHook(() => useLayoutEntry('tab-1'));
    expect(result.current).toBeNull();
    let take = () => undefined as void;
    act(() => {
      take = offerLayout('tab-1');
    });
    expect(result.current?.part).toBe('info');
    act(() => askLayout('tab-1', 'layout', 'tweaks'));
    expect(result.current?.ask?.tab).toBe('layout');
    expect(result.current?.part).toBe('tweaks');
    act(() => {
      clearLayoutAsk('tab-1');
      setPaneShows('tab-1', 'layout');
    });
    expect(result.current?.ask).toBeNull();
    expect(result.current?.shows).toBe('layout');
    act(() => take());
    expect(result.current).toBeNull();
  });

  it('names a person, not the machine, for an edit of the history', () => {
    expect(whoOf('anna · dev-anna', false, 'Анна')).toBe('Анна');
    expect(whoOf('anna@example.com', false, null)).toBe('anna');
    expect(whoOf('x · y', true, 'Анна')).toBe(`${t('next.ds.overview.agent')} · Анна`);
    expect(whoOf('x · y', true, null)).toBe(t('next.ds.overview.agent'));
    // An edit through the hub is its person's, not the owner's.
    expect(whoOf('petr@example.com · hub', false, 'Анна')).toBe('petr');
    expect(
      whoOf('anna@example.com · hub', false, 'Анна', (e) => (e === 'anna@example.com' ? 'Анна' : null)),
    ).toBe('Анна');
  });
  it('puts «Макет» in front once for each showing of the design', () => {
    const { result } = renderHook(() => useLayoutEntry('tab-3'));
    let take = () => undefined as void;
    act(() => {
      take = offerLayout('tab-3');
    });
    expect(result.current?.fronted).toBe(false);
    act(() => setLayoutFronted('tab-3'));
    expect(result.current?.fronted).toBe(true);
    act(() => take());
    act(() => {
      take = offerLayout('tab-3');
    });
    expect(result.current?.fronted).toBe(false);
    act(() => take());
  });
});
