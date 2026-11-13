import { describe, expect, it } from 'vitest';
import type { IMachineNotice } from '@/next/features/notifications/from-machines';
import { CHANGE_MS, HOLD_MS, steadyDue, steadyNotices } from '@/next/features/notifications/machine-steady';

const notice = (over: Partial<IMachineNotice['alert']> = {}, since = 1000): IMachineNotice => ({
  id: 'm1',
  name: 'dev',
  since,
  urgent: true,
  alert: { key: 'm1:mem', icon: 'memory-stick', text: 'Память почти кончилась', metric: 'mem', ...over },
});

describe('steadyNotices', () => {
  it('gives a new notice the key of its episode', () => {
    const [one] = steadyNotices([], [notice()], 5000);
    expect(one?.notice.alert.key).toBe('m1:1000');
    expect(one?.goneAt).toBeNull();
  });

  it('keeps the card through a reconnect: the same key, since and place', () => {
    const shown = steadyNotices([], [notice()], 5000);
    const lost = steadyNotices(shown, [], 6000);
    expect(lost.map((h) => h.notice.alert.key)).toEqual(['m1:1000']);
    expect(steadyDue(lost, [])).toBe(6000 + HOLD_MS);
    // The machine is heard again, with a new «since» and a key of its own: it is still the same card.
    const back = steadyNotices(lost, [notice({ key: 'm1:mem,cpu' }, 9000)], 9000);
    expect(back[0]?.notice.alert.key).toBe('m1:1000');
    expect(back[0]?.notice.since).toBe(1000);
    expect(back[0]?.goneAt).toBeNull();
  });

  it('does not take the card off by one good look, only after the hold', () => {
    const shown = steadyNotices([], [notice()], 0);
    const gone = steadyNotices(shown, [], 1000);
    expect(steadyNotices(gone, [], 1000 + HOLD_MS - 1)).toHaveLength(1);
    expect(steadyNotices(gone, [], 1000 + HOLD_MS)).toHaveLength(0);
  });

  it('changes what the card says no more often than CHANGE_MS', () => {
    const shown = steadyNotices([], [notice()], 0);
    const disk = notice({ key: 'm1:disk', metric: 'disk', text: 'Диск почти заполнен' });
    const soon = steadyNotices(shown, [disk], CHANGE_MS - 1);
    expect(soon).toBe(shown);
    expect(steadyDue(soon, [disk])).toBe(CHANGE_MS);
    const later = steadyNotices(soon, [disk], CHANGE_MS);
    expect(later[0]?.notice.alert.metric).toBe('disk');
    expect(later[0]?.notice.alert.key).toBe('m1:1000');
  });

  it('answers the same list while nothing changed', () => {
    const shown = steadyNotices([], [notice()], 0);
    expect(steadyNotices(shown, [notice()], 60_000)).toBe(shown);
    expect(steadyDue(shown, [notice()])).toBeNull();
  });

  it('starts a new episode after the hold, with a key of its own', () => {
    const shown = steadyNotices([], [notice()], 0);
    const gone = steadyNotices(steadyNotices(shown, [], 1000), [], 1000 + HOLD_MS);
    const again = steadyNotices(gone, [notice({}, 90_000)], 90_000);
    expect(again[0]?.notice.alert.key).toBe('m1:90000');
  });
});
