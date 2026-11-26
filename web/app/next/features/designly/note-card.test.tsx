// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { NO_EDIT, type IStyleEdit } from '@/next/features/designly/element-style';
import { BoardHandContext, type IBoardHand, type IHandState } from '@/next/features/designly/hand-edit';
import { noteLeft } from '@/next/features/designly/annotation';
import { placeCard } from '@/next/features/designly/note-place';
import { NOTE_OUT_MS, NotePopover, type INotePopoverProps } from '@/next/features/designly/site-notes';
import { t } from '@/next/i18n';

const look = {
  background: null,
  radius: 8,
  opacity: 1,
  font: { size: 20, weight: 400, color: '#111111', italic: false, line: 'none' },
};
const picked = {
  id: 1,
  look,
  selector: 'h1',
  tag: 'h1',
  text: 'Title',
  rect: { x: 0, y: 0, width: 100, height: 20 },
  component: null,
  html: '<h1>Title</h1>',
};
const held: IHandState = {
  tpl: 2,
  fp: 'h1:x',
  editable: true,
  dynamic: false,
  changed: false,
  selected: false,
  size: 20,
};

const handOf = (over: Partial<IBoardHand> = {}): IBoardHand => ({
  state: held,
  signal: null,
  caretAsked: () => false,
  hold: vi.fn(),
  drop: vi.fn(),
  command: vi.fn(),
  wrap: vi.fn(),
  revert: vi.fn(),
  commit: vi.fn().mockResolvedValue({ ok: true, words: null }),
  ...over,
});

const draftOf = (style: IStyleEdit = NO_EDIT) => ({
  picked,
  at: { x: 0, y: 40, top: 0, right: false },
  text: '',
  style,
});

const card = (hand: IBoardHand | null, over: Partial<INotePopoverProps> = {}) => {
  const props: INotePopoverProps = {
    n: 1,
    draft: draftOf(),
    text: '',
    onText: vi.fn(),
    mic: null,
    batch: false,
    onOk: vi.fn(),
    onCancel: vi.fn(),
    onDelete: vi.fn(),
    onStyle: vi.fn(),
    gone: false,
    ...over,
  };
  const view = render(
    <BoardHandContext.Provider value={hand}>
      <NotePopover {...props} />
    </BoardHandContext.Provider>,
  );
  return { props, view };
};

const ok = () => document.querySelector('.hx-ann__ok') as HTMLElement;
const press = (name: string) => fireEvent.click(screen.getByRole('button', { name: t(name) }));
const later = () => act(() => vi.advanceTimersByTime(NOTE_OUT_MS + 1));

beforeAll(() => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});
beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe('the card of a picked block (HRN-1770)', () => {
  it('holds the block while it is open and has no mode to enter or leave', () => {
    const hand = handOf();
    const { view } = card(hand);
    expect(hand.hold).toHaveBeenCalledWith(picked);
    expect(ok().getAttribute('aria-disabled')).toBe('true');
    view.unmount();
    expect(hand.drop).toHaveBeenCalled();
  });

  it('changes the whole block with no words selected, the selected words with some', () => {
    const hand = handOf();
    const { props } = card(hand);
    press('next.site.text.bold');
    expect(props.onStyle).toHaveBeenCalledWith({ ...NO_EDIT, weight: 700 });
    cleanup();
    const selecting = handOf({ state: { ...held, selected: true } });
    const other = card(selecting);
    press('next.site.text.bold');
    press('next.site.text.larger');
    expect(selecting.command).toHaveBeenCalledWith('bold');
    expect(selecting.wrap).toHaveBeenCalledWith('font-size', '22px');
    expect(other.props.onStyle).not.toHaveBeenCalled();
  });

  it('saves the hand’s changes alone, without the agent', () => {
    const hand = handOf();
    const { props } = card(hand, { draft: draftOf({ ...NO_EDIT, weight: 700 }) });
    expect(ok().textContent).toContain(t('next.notes.save'));
    fireEvent.click(ok());
    later();
    expect(hand.commit).toHaveBeenCalledWith({ 'font-weight': '700' });
    expect(props.onOk).toHaveBeenCalledWith('', expect.any(Promise), 'send');
  });

  it('sends the words to the agent with what was changed', () => {
    const hand = handOf({ state: { ...held, changed: true } });
    const { props } = card(hand, { text: 'Brighter' });
    expect(ok().getAttribute('aria-label')).toBe(t('next.notes.toAgent'));
    fireEvent.click(ok());
    later();
    expect(hand.commit).toHaveBeenCalledWith(null);
    expect(props.onOk).toHaveBeenCalledWith('Brighter', expect.any(Promise), 'send');
  });

  it('keeps what was done on a press away', () => {
    const hand = handOf({ state: { ...held, changed: true } });
    const { props } = card(hand);
    fireEvent.pointerDown(document.body);
    expect(hand.commit).toHaveBeenCalled();
    expect(props.onOk).toHaveBeenCalledWith('', expect.any(Promise), 'keep');
  });

  it('just closes on a press away with nothing done', () => {
    const hand = handOf();
    const { props } = card(hand);
    fireEvent.pointerDown(document.body);
    expect(hand.commit).not.toHaveBeenCalled();
    expect(props.onCancel).toHaveBeenCalled();
  });

  it('gives everything back on Esc', () => {
    const hand = handOf({ state: { ...held, changed: true } });
    const { props } = card(hand);
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });
    later();
    expect(hand.drop).toHaveBeenCalled();
    expect(hand.commit).not.toHaveBeenCalled();
    expect(props.onCancel).toHaveBeenCalled();
  });

  it('waits for its button over a drawing, which goes without words too', () => {
    const area = {
      kind: 'drawing' as const,
      rect: picked.rect,
      blocks: [],
      message: vi.fn(),
      onComment: vi.fn(),
    };
    const { props } = card(null, { draft: { ...draftOf(), area } });
    fireEvent.pointerDown(document.body);
    expect(props.onOk).not.toHaveBeenCalled();
    expect(props.onCancel).not.toHaveBeenCalled();
    expect(ok().getAttribute('aria-disabled')).toBeNull();
    fireEvent.click(ok());
    later();
    expect(props.onOk).toHaveBeenCalledWith('', null, 'send');
    cleanup();
    card(null, { draft: { ...draftOf(), area }, text: 'Here' });
    press('next.notes.comment');
    expect(area.onComment).toHaveBeenCalledWith('Here');
  });

  it('leaves the look to the agent where the source takes no hand', () => {
    const { props } = card(null, { draft: draftOf({ ...NO_EDIT, radius: 2 }) });
    expect(ok().getAttribute('aria-label')).toBe(t('next.notes.toAgent'));
    fireEvent.click(ok());
    later();
    expect(props.onOk).toHaveBeenCalledWith('', null, 'send');
  });
});

describe('what a closed card leaves', () => {
  const style = { ...NO_EDIT, radius: 2 };

  it('is nothing for the agent where the source has the changes and there are no words', async () => {
    await expect(noteLeft('', Promise.resolve({ ok: true, words: null }), style, look)).resolves.toBeNull();
  });

  it('tells the agent what the source already has', async () => {
    const written = Promise.resolve({ ok: true, words: { was: 'Old &amp; one', now: '<b>New</b>' } });
    await expect(noteLeft('Brighter', written, style, look)).resolves.toEqual({
      text: 'Brighter',
      style: NO_EDIT,
      saved: ['text: «New» (was «Old & one»)', 'border-radius: 2px (was 8px)'],
    });
  });

  it('keeps the look for the agent where the write was refused', async () => {
    await expect(noteLeft('', Promise.resolve({ ok: false, words: null }), style, look)).resolves.toEqual({
      text: '',
      style,
      saved: [],
    });
  });
});

describe('where the card stands', () => {
  const board = { left: 300, top: 50, right: 1300, bottom: 850 };
  const size = { width: 344, height: 200 };

  it('is under its block', () => {
    expect(placeCard({ x: 400, top: 100, bottom: 160 }, size, board, false)).toMatchObject({
      x: 400,
      y: 170,
      above: false,
    });
  });

  it('is above a block low on the board, and never over it', () => {
    const at = placeCard({ x: 400, top: 700, bottom: 800 }, size, board, false);
    expect(at).toMatchObject({ y: 490, above: true });
    expect(at.y + size.height).toBeLessThan(700);
  });

  it('is squeezed into the larger room before it lies over the block', () => {
    const at = placeCard({ x: 400, top: 240, bottom: 640 }, { width: 344, height: 400 }, board, false);
    expect(at).toMatchObject({ above: false, y: 650, max: 192 });
  });

  it('stays inside the board', () => {
    expect(placeCard({ x: 1290, top: 100, bottom: 160 }, size, board, false).x).toBe(948);
    expect(placeCard({ x: 320, top: 100, bottom: 160 }, size, board, true).x).toBe(308);
  });
});
