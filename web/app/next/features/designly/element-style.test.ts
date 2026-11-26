import { describe, expect, it } from 'vitest';
import {
  changedKeys,
  hasEdit,
  itemsOf,
  NO_EDIT,
  shownValues,
  stepRadius,
  styleLine,
  toCss,
  toggleLine,
  toItem,
  withValue,
} from '@/next/features/designly/element-style';

const look = { background: '#112233', radius: 16, opacity: 1 };

describe('withValue', () => {
  it('holds only what differs from the block', () => {
    const radius = withValue(NO_EDIT, look, 'radius', 8);
    expect(radius).toEqual({ ...NO_EDIT, background: null, radius: 8, opacity: null });
    expect(withValue(radius, look, 'radius', 16)).toEqual(NO_EDIT);
    expect(withValue(NO_EDIT, look, 'opacity', 100)).toEqual(NO_EDIT);
    expect(withValue(NO_EDIT, look, 'opacity', 80).opacity).toBe(80);
    expect(withValue(NO_EDIT, look, 'background', '#112233')).toEqual(NO_EDIT);
    expect(withValue({ ...NO_EDIT, background: '#FFFFFF' }, look, 'background', null)).toEqual(NO_EDIT);
  });

  it('lists what changed', () => {
    const edit = withValue(withValue(NO_EDIT, look, 'radius', 4), look, 'background', '#ffffff');
    expect(changedKeys(edit)).toEqual(['background', 'radius']);
    expect(hasEdit(edit)).toBe(true);
    expect(hasEdit(NO_EDIT)).toBe(false);
  });
});

describe('shownValues and stepRadius', () => {
  it('shows the edit over the block, the fill as the theme when it is not changed', () => {
    expect(shownValues(NO_EDIT, look)).toMatchObject({ background: null, radius: 16, opacity: 100 });
    expect(shownValues({ ...NO_EDIT, background: '#FFFFFF', radius: 2, opacity: 35 }, look)).toMatchObject({
      background: '#FFFFFF',
      radius: 2,
      opacity: 35,
    });
  });

  it('steps by two inside 0 to 40, from nothing for a radius in other units', () => {
    expect(stepRadius(16, 1)).toBe(18);
    expect(stepRadius(0, -1)).toBe(0);
    expect(stepRadius(40, 1)).toBe(40);
    expect(stepRadius(null, 1)).toBe(2);
  });
});

describe('what the page wears', () => {
  const note = (id: string, pick: number | null, edit = NO_EDIT) => ({
    id,
    picked: { id: pick },
    style: edit,
  });
  const radius = { ...NO_EDIT, radius: 4 };

  it('turns an edit into the bridge look: opacity as a share', () => {
    expect(toItem(3, { ...NO_EDIT, background: '#FFFFFF', radius: 8, opacity: 80 })).toEqual({
      id: 3,
      background: '#FFFFFF',
      radius: 8,
      opacity: 0.8,
    });
  });

  it('takes the notes that changed a block, with the card in place of its note, and a new card besides', () => {
    const notes = [note('a', 1, radius), note('b', 2), note('c', null, radius)];
    expect(itemsOf(notes, null).map((i) => i.id)).toEqual([1]);
    const editing = { id: 'b', picked: { id: 2 }, style: { ...NO_EDIT, opacity: 50 } };
    expect(itemsOf(notes, editing).map((i) => i.id)).toEqual([1, 2]);
    const reverting = { id: 'a', picked: { id: 1 }, style: NO_EDIT };
    expect(itemsOf(notes, reverting)).toEqual([]);
    const fresh = { picked: { id: 9 }, style: radius };
    expect(itemsOf(notes, fresh).map((i) => i.id)).toEqual([1, 9]);
  });
});

describe('styleLine', () => {
  it('is CSS the agent can read, with what the block had', () => {
    expect(styleLine({ ...NO_EDIT, background: '#FFFFFF', radius: 8, opacity: 85 }, look)).toBe(
      'background: #FFFFFF (was #112233); border-radius: 8px (was 16px); opacity: 0.85 (was 1)',
    );
  });

  it('says nothing for no edit and keeps quiet about what it does not know', () => {
    expect(styleLine(NO_EDIT, look)).toBe('');
    expect(
      styleLine(
        { ...NO_EDIT, background: '#FFFFFF', radius: 8, opacity: null },
        { ...look, background: null, radius: null },
      ),
    ).toBe('background: #FFFFFF; border-radius: 8px');
  });
});

describe('the type of a block (HRN-1764, HRN-1770)', () => {
  const typed = {
    ...look,
    font: { size: 20, weight: 400, color: '#111111', italic: false, line: 'none' },
  };

  it('keeps only what differs from the block', () => {
    expect(withValue(NO_EDIT, typed, 'size', 100)).toEqual(NO_EDIT);
    expect(withValue(NO_EDIT, typed, 'weight', 400)).toEqual(NO_EDIT);
    expect(withValue(NO_EDIT, typed, 'slant', 'normal')).toEqual(NO_EDIT);
    expect(withValue(NO_EDIT, typed, 'line', 'none')).toEqual(NO_EDIT);
    expect(withValue(NO_EDIT, typed, 'color', '#111111')).toEqual(NO_EDIT);
  });

  it('puts a line on and takes it off beside the other', () => {
    expect(toggleLine('none', 'underline')).toBe('underline');
    expect(toggleLine('underline', 'line-through')).toBe('underline line-through');
    expect(toggleLine('underline line-through', 'underline')).toBe('line-through');
    expect(toggleLine('line-through', 'line-through')).toBe('none');
  });

  const edit = { ...NO_EDIT, size: 150, weight: 700, slant: 'italic' as const, color: '#E5484D' };

  it('sends the size in pixels of the block’s own and says what was', () => {
    expect(toItem(2, edit, typed)).toMatchObject({
      size: 30,
      weight: 700,
      slant: 'italic',
      color: '#E5484D',
    });
    expect(styleLine(edit, typed)).toBe(
      'font-size: 30px (was 20px); font-weight: 700 (was 400); font-style: italic; color: #E5484D (was #111111)',
    );
  });

  it('writes the edit as the CSS the board’s source takes', () => {
    expect(
      toCss({ ...edit, background: '#FFFFFF', radius: 0, opacity: 50, line: 'underline' }, typed),
    ).toEqual({
      'font-size': '30px',
      'font-weight': '700',
      'font-style': 'italic',
      'text-decoration-line': 'underline',
      color: '#E5484D',
      background: '#FFFFFF',
      'border-radius': '0px',
      opacity: '0.5',
    });
    expect(toCss(NO_EDIT, typed)).toBeNull();
  });

  it('reads a note kept before the type as no change of it', () => {
    const old = { background: null, radius: 4, opacity: null } as unknown as typeof NO_EDIT;
    expect(styleLine(old, typed)).toBe('border-radius: 4px (was 16px)');
    expect(toCss(old, typed)).toEqual({ 'border-radius': '4px' });
  });
});
