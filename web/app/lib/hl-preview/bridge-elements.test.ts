import { describe, expect, it } from 'vitest';
import { parseParentMessage } from './bridge-core';

describe('the blocks inside a rectangle (HRN-1772)', () => {
  it('takes a sane rectangle to look into, and nothing else', () => {
    expect(parseParentMessage({ type: 'tk:elements-in', req: 1, x: 0, y: 5, w: 10, h: 20 })).toEqual({
      type: 'tk:elements-in',
      req: 1,
      x: 0,
      y: 5,
      w: 10,
      h: 20,
    });
    expect(parseParentMessage({ type: 'tk:elements-in', req: 1, x: 0, y: 5, w: -1, h: 20 })).toBeNull();
    expect(parseParentMessage({ type: 'tk:elements-in', req: 'x', x: 0, y: 5, w: 1, h: 2 })).toBeNull();
    expect(parseParentMessage({ type: 'tk:elements-in', req: 1, x: 0, y: 5, w: 1e9, h: 2 })).toBeNull();
  });
});
