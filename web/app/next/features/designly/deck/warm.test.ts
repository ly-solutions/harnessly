import { describe, expect, it } from 'vitest';
import { DECK_WARM_PARALLEL, nextToWarm, warmOrder } from '@/next/features/designly/deck/warm';

describe('warmOrder', () => {
  it('loads the neighbours first, then outward, the next side first', () => {
    expect(warmOrder(7, 3)).toEqual([4, 2, 5, 1, 6, 0]);
  });

  it('goes on to one side where the other ends', () => {
    expect(warmOrder(5, 0)).toEqual([1, 2, 3, 4]);
    expect(warmOrder(5, 4)).toEqual([3, 2, 1, 0]);
    expect(warmOrder(4, 1)).toEqual([2, 0, 3]);
  });

  it('has nothing to load for a deck of one slide', () => {
    expect(warmOrder(1, 0)).toEqual([]);
    expect(warmOrder(0, 0)).toEqual([]);
  });
});

describe('nextToWarm', () => {
  const ahead = ['b', 'c', 'd', 'e'];
  const frame = (src: string, loaded: boolean) => ({ src, loaded });

  it('starts the first ones of the order, a few at a time', () => {
    expect(nextToWarm(ahead, [frame('a', true)], 'a')).toEqual(['b', 'c'].slice(0, DECK_WARM_PARALLEL));
  });

  it('starts the next one as soon as one of those loading has loaded', () => {
    expect(nextToWarm(ahead, [frame('a', true), frame('b', false), frame('c', false)], 'a', 2)).toEqual([]);
    expect(nextToWarm(ahead, [frame('a', true), frame('b', true), frame('c', false)], 'a', 2)).toEqual(['d']);
  });

  it('does not count the slide on screen as loading out of sight, nor load it again', () => {
    expect(nextToWarm(['a', 'b', 'c'], [frame('a', false)], 'a', 2)).toEqual(['b', 'c']);
  });

  it('skips the slides that already have a frame, a kept one after a turn too', () => {
    expect(nextToWarm(ahead, [frame('c', true), frame('a', true), frame('e', true)], 'a', 2)).toEqual([
      'b',
      'd',
    ]);
  });

  it('has nothing to start once every slide has a frame', () => {
    expect(nextToWarm(['b'], [frame('a', true), frame('b', true)], 'a', 2)).toEqual([]);
  });
});
