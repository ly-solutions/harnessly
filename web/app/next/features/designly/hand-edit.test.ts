import { describe, expect, it } from 'vitest';
import { patchOf, readHandMessage } from '@/next/features/designly/hand-edit';

describe('the board under the person’s hand (HRN-1764, HRN-1770)', () => {
  it('reads the right click with its block', () => {
    expect(readHandMessage({ type: 'tk:menu', x: 10, y: 20, label: 'Hero' })).toEqual({
      k: 'menu',
      menu: { x: 10, y: 20, label: 'Hero' },
    });
    expect(readHandMessage({ type: 'tk:menu', x: '1', y: 2 })).toBeNull();
    expect(readHandMessage({ type: 'tk:picked' })).toBeNull();
  });

  it('reads what the board says of the held block', () => {
    expect(
      readHandMessage({
        type: 'tk:hand-state',
        on: true,
        tpl: 3,
        fp: 'h1:x',
        editable: true,
        changed: true,
        selected: false,
        size: 24,
      }),
    ).toEqual({
      k: 'state',
      state: {
        tpl: 3,
        fp: 'h1:x',
        editable: true,
        dynamic: false,
        changed: true,
        selected: false,
        size: 24,
      },
    });
    expect(readHandMessage({ type: 'tk:hand-state', on: true, tpl: -1 })).toMatchObject({
      state: { tpl: null, editable: false },
    });
    expect(readHandMessage({ type: 'tk:hand-state', on: false })).toEqual({ k: 'state', state: null });
  });

  it('reads the words of a block let go of, and the presses the card answers', () => {
    const save = { tpl: 3, fp: 'h1:x', was: 'a', html: '<b>a</b>' };
    expect(readHandMessage({ type: 'tk:hand-save', ...save })).toEqual({ k: 'save', save });
    expect(readHandMessage({ type: 'tk:hand-save', ...save, html: 7 })).toBeNull();
    expect(readHandMessage({ type: 'tk:hand-save', ...save, tpl: 1.5 })).toBeNull();
    expect(readHandMessage({ type: 'tk:hand-pick', x: 4, y: 5 })).toEqual({ k: 'pick', x: 4, y: 5 });
    expect(readHandMessage({ type: 'tk:hand-away' })).toEqual({ k: 'away' });
    expect(readHandMessage({ type: 'tk:hand-key', key: 'Escape' })).toEqual({ k: 'key', key: 'Escape' });
    expect(readHandMessage({ type: 'tk:hand-key', key: 'Tab' })).toBeNull();
  });

  it('writes the words and the block’s styles at once, one version of the history', () => {
    const save = { tpl: 1, fp: 'p:1', was: 'a', html: 'b' };
    const style = { color: '#e5484d' };
    expect(patchOf(save, style)).toEqual({ op: 'text', tpl: 1, was: 'a', html: 'b', style });
    expect(patchOf(save, null)).toEqual({ op: 'text', tpl: 1, was: 'a', html: 'b' });
    expect(patchOf({ ...save, html: null }, style)).toEqual({ op: 'style', tpl: 1, was: 'p:1', style });
    expect(patchOf({ ...save, html: null }, null)).toBeNull();
    expect(patchOf({ ...save, html: null, fp: '' }, style)).toBeNull();
  });
});
