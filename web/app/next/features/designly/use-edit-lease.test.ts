import { describe, expect, it } from 'vitest';
import { othersLease, type IEditLease } from '@/next/features/designly/use-edit-lease';

const lease = (over: Partial<IEditLease> = {}): IEditLease => ({
  person: 'mate@example.com',
  name: 'Мейт',
  machine: 'vm-1',
  machineName: 'vm-1',
  until: 2_000,
  mine: false,
  ...over,
});

describe('othersLease (HRN-1794)', () => {
  it('names someone else editing', () => expect(othersLease(lease(), 1_000)?.name).toBe('Мейт'));
  it('does not name the page itself', () => expect(othersLease(lease({ mine: true }), 1_000)).toBeNull());
  it('drops a lease that ran out', () => expect(othersLease(lease(), 3_000)).toBeNull());
  it('is null with no lease', () => expect(othersLease(null)).toBeNull());
});
