import { describe, expect, it } from 'vitest';
import { createSlotLeases } from '@/next/features/pane/slot-lease';

describe('slot leases', () => {
  it('gives every tab a slot of its own and the same one again', () => {
    const leases = createSlotLeases(() => 1);
    expect(leases.lease('port', 'a')).toBe(1);
    expect(leases.lease('port', 'b')).toBe(2);
    expect(leases.lease('port', 'a')).toBe(1);
    expect(leases.slotOf('b')).toBe(2);
  });

  it('keeps ports in 1–6 and sites in 7–8', () => {
    const leases = createSlotLeases(() => 1);
    expect(leases.lease('site', 'a')).toBe(7);
    expect(leases.lease('site', 'b')).toBe(8);
    expect(leases.lease('port', 'c')).toBe(1);
  });

  it('takes the least recently used slot when all are held, and says whose it was', () => {
    let now = 0;
    const leases = createSlotLeases(() => ++now);
    const taken: string[] = [];
    leases.onTaken((tab) => taken.push(tab));
    for (const tab of ['a', 'b', 'c', 'd', 'e', 'f']) leases.lease('port', tab);
    leases.lease('port', 'a');
    expect(leases.lease('port', 'g')).toBe(2);
    expect(taken).toEqual(['b']);
    expect(leases.slotOf('b')).toBeNull();
    expect(leases.slotOf('a')).toBe(1);
  });

  it('sites are only two: the third takes the older', () => {
    let now = 0;
    const leases = createSlotLeases(() => ++now);
    leases.lease('site', 'a');
    leases.lease('site', 'b');
    expect(leases.lease('site', 'c')).toBe(7);
    expect(leases.slotOf('a')).toBeNull();
  });

  it('moves a tab to the other pool when its address changes kind, freeing the old slot', () => {
    const leases = createSlotLeases(() => 1);
    expect(leases.lease('port', 'a')).toBe(1);
    expect(leases.lease('site', 'a')).toBe(7);
    expect(leases.lease('port', 'b')).toBe(1);
  });

  it('frees the slot of a released tab and stops telling the unsubscribed', () => {
    const leases = createSlotLeases(() => 1);
    const heard: string[] = [];
    const off = leases.onTaken((tab) => heard.push(tab));
    leases.lease('site', 'a');
    leases.release('a');
    expect(leases.slotOf('a')).toBeNull();
    expect(leases.lease('site', 'b')).toBe(7);
    off();
    leases.lease('site', 'c');
    leases.lease('site', 'd');
    expect(heard).toEqual([]);
  });
});
