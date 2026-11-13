import { describe, expect, it } from 'vitest';
import { readStoreRev } from '@/next/features/designly/use-store-head';

describe('the store revision a rev event says (HRN-1262)', () => {
  it('is a positive whole number', () => {
    expect(readStoreRev('42')).toBe(42);
  });
  it('is nothing for anything else', () => {
    expect(readStoreRev('0')).toBeNull();
    expect(readStoreRev('-3')).toBeNull();
    expect(readStoreRev('1.5')).toBeNull();
    expect(readStoreRev('x')).toBeNull();
    expect(readStoreRev(7)).toBeNull();
  });
});
