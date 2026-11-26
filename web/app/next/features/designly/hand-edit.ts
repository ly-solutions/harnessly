// The person's own hand on a board: the board's right click, and the block the note's card holds —
// a plain text is edited right where it stands — as the board's script tells them (design/texts/client.js, `tk:menu`,
// `tk:hand-*`). What was changed by hand is written into the board's source by the runtime's hand edit
// (POST /design/deck/patch) in one write, which keeps a version «Manual edit».
import { createContext } from 'react';
import { patchDeck } from '@/next/features/designly/deck/api';
import type { IPicked } from '@/next/features/designly/page-bridge';
import type { TDeckPatch, TDeckPatchRefusal } from '@/next/features/designly/deck/types';
import type { MachineId } from '@/next/protocol/v2';

export { DECK_PATCH_REFUSAL as HAND_REFUSAL } from '@/next/features/designly/deck/types';

const MAX_LABEL = 60;
const MAX_HTML = 20_000;
const MAX_COORD = 100_000;

/** A right click on the board: where (the board's viewport) and what block it was on. */
export interface IBoardMenu {
  x: number;
  y: number;
  /** The block's name; empty on the page's bare ground. */
  label: string;
}

/** What the board says of the block it holds for the card. */
export interface IHandState {
  /** The block's number in the board's source; null where the source does not name it: nothing is written by hand. */
  tpl: number | null;
  /** The board's fingerprint of the block: a style lands by it. */
  fp: string;
  /** A plain text: it is edited where it stands. */
  editable: boolean;
  /** A text the board's code fills in. */
  dynamic: boolean;
  /** The words differ from the ones the block had. */
  changed: boolean;
  /** Some of the words are selected: the card's type goes to them, else to the whole block. */
  selected: boolean;
  /** The font size at the selection, else of the block, in the board's pixels; 0 when unknown. */
  size: number;
}

/** The words of a block let go of to be saved. */
export interface ITextSave {
  tpl: number;
  fp: string;
  /** The element's words as they were, as the source holds them. */
  was: string;
  /** The new words; null where they did not change. */
  html: string | null;
}

export const HAND_KEY = { escape: 'Escape', enter: 'Enter' } as const;
export type THandKey = (typeof HAND_KEY)[keyof typeof HAND_KEY];

export type THandMessage =
  | { k: 'menu'; menu: IBoardMenu }
  | { k: 'state'; state: IHandState | null }
  | { k: 'save'; save: ITextSave }
  /** A double click on a plain text: the host picks it, the caret goes there. */
  | { k: 'pick'; x: number; y: number }
  /** A press on the board away from the held block. */
  | { k: 'away' }
  | { k: 'key'; key: THandKey }
  | { k: 'dynamic' };

const isNum = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v);
const isNumber = (v: unknown): v is number => Number.isInteger(v) && (v as number) >= 0;
const words = (v: unknown, max: number) => (typeof v === 'string' ? v.slice(0, max) : '');
const inside = (v: number) => Math.max(0, Math.min(MAX_COORD, v));

const readState = (m: Record<string, unknown>): IHandState => ({
  tpl: isNumber(m.tpl) ? m.tpl : null,
  fp: words(m.fp, 64),
  editable: m.editable === true,
  dynamic: m.dynamic === true,
  changed: m.changed === true,
  selected: m.selected === true,
  size: isNum(m.size) && m.size > 0 ? m.size : 0,
});

const readSave = (m: Record<string, unknown>): ITextSave | null => {
  if (!isNumber(m.tpl) || typeof m.was !== 'string') return null;
  if (m.html !== null && !(typeof m.html === 'string' && m.html.length <= MAX_HTML)) return null;
  return { tpl: m.tpl, fp: words(m.fp, 64), was: m.was.slice(0, MAX_HTML), html: m.html };
};

/** A message of the board about the person's hand, checked; null for every other message. */
export const readHandMessage = (data: unknown): THandMessage | null => {
  if (!data || typeof data !== 'object') return null;
  const m = data as Record<string, unknown>;
  switch (m.type) {
    case 'tk:menu':
      if (!isNum(m.x) || !isNum(m.y)) return null;
      return { k: 'menu', menu: { x: inside(m.x), y: inside(m.y), label: words(m.label, MAX_LABEL) } };
    case 'tk:hand-state':
      return { k: 'state', state: m.on === true ? readState(m) : null };
    case 'tk:hand-save': {
      const save = readSave(m);
      return save && { k: 'save', save };
    }
    case 'tk:hand-pick':
      return isNum(m.x) && isNum(m.y) ? { k: 'pick', x: inside(m.x), y: inside(m.y) } : null;
    case 'tk:hand-away':
      return { k: 'away' };
    case 'tk:hand-key':
      return m.key === HAND_KEY.escape || m.key === HAND_KEY.enter ? { k: 'key', key: m.key } : null;
    case 'tk:text-dynamic':
      return { k: 'dynamic' };
    default:
      return null;
  }
};

/**
 * The one patch of what was changed by hand, so it is one version of the history: the new words with the block's styles,
 * or the styles alone (they land by the element as it was); null where nothing is to be written.
 */
export const patchOf = (
  { tpl, fp, was, html }: ITextSave,
  style: Record<string, string> | null,
): TDeckPatch | null => {
  if (html !== null) return { op: 'text', tpl, was, html, ...(style ? { style } : {}) };
  return style && fp !== '' ? { op: 'style', tpl, was: fp, style } : null;
};

/** Writes an edit into the board's source; why it was not taken, else null. */
export const saveHandEdit = async (
  machine: MachineId,
  artifact: string,
  file: string,
  patch: TDeckPatch,
): Promise<TDeckPatchRefusal | null> => {
  const done = await patchDeck(machine, artifact, { file }, patch);
  return done.ok ? null : done.code;
};

/** What a commit of the hand came to. */
export interface IHandWritten {
  /** The source has it: false where the patch was refused (the page says why). */
  ok: boolean;
  /** The words as they were and as they are; null where they did not change. */
  words: { was: string; now: string } | null;
}

export type THandCommand = 'bold' | 'italic' | 'underline' | 'strikeThrough';

/** Something the person did on the board that the card answers: each has its own number. */
export interface IHandSignal {
  n: number;
  kind: 'away' | THandKey;
}

/** The hand the view lends the note's card on a board that takes hand edits. */
export interface IBoardHand {
  /** The held block; null until the board answers. */
  state: IHandState | null;
  signal: IHandSignal | null;
  /** The card is opening by a double click on the text: the caret goes there, not into the card's field. */
  caretAsked: () => boolean;
  hold: (picked: IPicked) => void;
  /** Lets the block go with its words as they were. */
  drop: () => void;
  /** The selected words made bold, italic and so on. */
  command: (command: THandCommand) => void;
  /** A colour or a size of the selected words. */
  wrap: (prop: 'color' | 'font-size', value: string | null) => void;
  /** The words as they were; the block stays held. */
  revert: () => void;
  /** Lets the block go and writes its words with `style` into the source. */
  commit: (style: Record<string, string> | null) => Promise<IHandWritten>;
}

export const BoardHandContext = createContext<IBoardHand | null>(null);
