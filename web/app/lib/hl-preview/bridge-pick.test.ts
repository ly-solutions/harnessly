// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';
import * as core from './bridge-core';

describe('tk:pick from the parent', () => {
  it('reads a plain pick as before', () => {
    expect(core.parseParentMessage({ type: 'tk:pick', on: true })).toEqual({ type: 'tk:pick', on: true });
    expect(core.parseParentMessage({ type: 'tk:pick', on: 1 })).toBeNull();
  });

  it('reads keep only while picking is turned on', () => {
    expect(core.parseParentMessage({ type: 'tk:pick', on: true, keep: true })).toEqual({
      type: 'tk:pick',
      on: true,
      keep: true,
    });
    expect(core.parseParentMessage({ type: 'tk:pick', on: false, keep: true })).toEqual({
      type: 'tk:pick',
      on: false,
    });
    expect(core.parseParentMessage({ type: 'tk:pick', on: true, keep: 'yes' })).toEqual({
      type: 'tk:pick',
      on: true,
    });
  });
});

describe('the block a pick on a board means (HRN-1770)', () => {
  const pick = (html: string, at: string) => {
    document.body.innerHTML = html;
    return (core.wholeBlock(document.querySelector(at)) as Element).tagName.toLowerCase();
  };

  it('is the whole line for a bold word or a coloured piece, also one the hand has just made', () => {
    expect(pick('<h1 data-dc-tpl="1">a <b data-dc-tpl="2">b</b></h1>', 'b')).toBe('h1');
    expect(
      pick('<h1 data-dc-tpl="1"><em data-dc-tpl="2">x <span style="color: red">y</span></em></h1>', 'span'),
    ).toBe('h1');
  });

  it('is the element itself for a span of the layout and on a page that is no board', () => {
    expect(pick('<div data-dc-tpl="1"><span class="badge" data-dc-tpl="2">n</span></div>', 'span')).toBe(
      'span',
    );
    expect(pick('<div data-dc-tpl="1"><span data-dc-tpl="2">n</span></div>', 'span')).toBe('span');
    expect(pick('<p>a <b>b</b></p>', 'b')).toBe('b');
  });
});
