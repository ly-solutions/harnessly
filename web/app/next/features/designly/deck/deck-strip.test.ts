import { describe, expect, it } from 'vitest';
import { stripTurns } from '@/next/features/designly/deck/deck-strip';

describe('stripTurns', () => {
  it('asks for two pictures at a time, in the order of the strip', () => {
    expect(stripTurns(['a', 'b', 'c', 'd'], new Set())).toEqual([true, true, false, false]);
  });

  it('gives the next turn once a picture has loaded or failed', () => {
    expect(stripTurns(['a', 'b', 'c', 'd'], new Set(['a']))).toEqual([true, true, true, false]);
    expect(stripTurns(['a', 'b', 'c', 'd'], new Set(['a', 'b', 'c', 'd']))).toEqual([true, true, true, true]);
  });

  it('skips the slides without a picture and asks again for a picture that changed', () => {
    expect(stripTurns([null, 'a2', 'b', 'c'], new Set(['a1', 'b']))).toEqual([false, true, true, true]);
  });
});
