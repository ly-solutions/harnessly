import { describe, expect, it } from 'vitest';
import { patchOf, readHandMessage } from '@/next/features/designly/hand-edit';

describe('the board under the person’s hand (HRN-1764)', () => {
  it('reads the right click with its block', () => {
    expect(readHandMessage({ type: 'tk:menu', x: 10, y: 20, label: 'Hero', editable: true })).toEqual({
      k: 'menu',
      menu: { x: 10, y: 20, label: 'Hero', editable: true },
    });
    expect(readHandMessage({ type: 'tk:menu', x: '1', y: 2 })).toBeNull();
    expect(readHandMessage({ type: 'tk:picked' })).toBeNull();
  });

  it('keeps of the styles only the size and the colour, with plain values', () => {
    const read = readHandMessage({
      type: 'tk:text-save',
      tpl: 3,
      was: 'a',
      html: '<b>a</b>',
      fp: 'h1:x',
      style: { color: '#e5484d', 'font-size': 'url(x);', background: 'red' },
    });
    expect(read).toEqual({
      k: 'text-save',
      save: { tpl: 3, was: 'a', html: '<b>a</b>', fp: 'h1:x', style: { color: '#e5484d' } },
    });
  });

  it('writes the words and their styles at once, one version of the history', () => {
    const save = { tpl: 1, was: 'a', html: 'b', fp: 'p:1', style: { color: null } };
    expect(patchOf(save)).toEqual({ op: 'text', tpl: 1, was: 'a', html: 'b', style: { color: null } });
    expect(patchOf({ ...save, html: null })).toEqual({
      op: 'style',
      tpl: 1,
      was: 'p:1',
      style: { color: null },
    });
    expect(patchOf({ ...save, html: null, style: null })).toBeNull();
  });
});
