import { beforeEach, describe, expect, it } from 'vitest';
import { AUTO_OPEN_EVERY_MS, resetAutoOpen, takeAutoOpen } from '@/next/features/notifications/auto-open';

describe('takeAutoOpen', () => {
  beforeEach(resetAutoOpen);

  it('never opens what is not urgent', () => {
    expect(takeAutoOpen('m1', false, 0)).toBe(false);
    expect(takeAutoOpen('m1', false, AUTO_OPEN_EVERY_MS * 2)).toBe(false);
  });

  it('opens an urgent card once in five minutes', () => {
    expect(takeAutoOpen('m1', true, 1000)).toBe(true);
    expect(takeAutoOpen('m1', true, 2000)).toBe(false);
    expect(takeAutoOpen('m1', true, 1000 + AUTO_OPEN_EVERY_MS - 1)).toBe(false);
    expect(takeAutoOpen('m1', true, 1000 + AUTO_OPEN_EVERY_MS)).toBe(true);
  });

  it('counts each machine apart', () => {
    expect(takeAutoOpen('m1', true, 0)).toBe(true);
    expect(takeAutoOpen('m2', true, 1)).toBe(true);
  });
});
