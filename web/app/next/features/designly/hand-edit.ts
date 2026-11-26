// The person's own hand on a board: the board's right click and a text edited where it stands, as the
// board's script tells them (design/texts/client.js, `tk:menu`, `tk:text-*`), and the edit written into the board's
// source by the hand edit of the runtime (POST /design/deck/patch), which keeps a version «Manual edit».
import { createContext } from 'react';
import { patchDeck } from '@/next/features/designly/deck/api';
import type { IPicked } from '@/next/features/designly/page-bridge';
import {
  DECK_PATCH_REFUSAL,
  type TDeckPatch,
  type TDeckPatchRefusal,
} from '@/next/features/designly/deck/types';
import type { MachineId } from '@/next/protocol/v2';

const MAX_LABEL = 60;
const MAX_HTML = 20_000;
const MAX_COORD = 100_000;
/** The styles a hand edit sets on the whole block. */
const HAND_PROPS = ['font-size', 'color'] as const;
const CSS_VALUE = /^[#\w.%() ,-]{1,40}$/;

/** A right click on the board: where (the board's viewport), what block it was on, and whether its text can be edited. */
export interface IBoardMenu {
  x: number;
  y: number;
  /** The block's name; empty on the page's bare ground. */
  label: string;
  editable: boolean;
}

/** A text being edited on the board. */
export interface ITextState {
  label: string;
  /** Its font size in the board's pixels. */
  size: number;
}

export interface ITextSave {
  tpl: number;
  /** The element's words as they were, as the source holds them. */
  was: string;
  /** The new words; null where only the styles changed. */
  html: string | null;
  /** The element's fingerprint before the edit: a style lands by it. */
  fp: string;
  style: Record<string, string | null> | null;
}

export type THandMessage =
  | { k: 'menu'; menu: IBoardMenu }
  | { k: 'text-state'; state: ITextState | null }
  | { k: 'text-save'; save: ITextSave }
  | { k: 'text-dynamic' };

const isNum = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v);
const words = (v: unknown, max: number) => (typeof v === 'string' ? v.slice(0, max) : '');

const readStyle = (raw: unknown): Record<string, string | null> | null => {
  if (!raw || typeof raw !== 'object') return null;
  const out: Record<string, string | null> = {};
  for (const prop of HAND_PROPS) {
    const value = (raw as Record<string, unknown>)[prop];
    if (value === null) out[prop] = null;
    else if (typeof value === 'string' && CSS_VALUE.test(value)) out[prop] = value;
  }
  return Object.keys(out).length > 0 ? out : null;
};

/** A message of the board about the person's hand, checked; null for every other message. */
export const readHandMessage = (data: unknown): THandMessage | null => {
  if (!data || typeof data !== 'object') return null;
  const m = data as Record<string, unknown>;
  if (m.type === 'tk:menu') {
    if (!isNum(m.x) || !isNum(m.y)) return null;
    const at = (v: number) => Math.max(0, Math.min(MAX_COORD, v));
    return {
      k: 'menu',
      menu: { x: at(m.x), y: at(m.y), label: words(m.label, MAX_LABEL), editable: m.editable === true },
    };
  }
  if (m.type === 'tk:text-state') {
    if (m.on !== true) return { k: 'text-state', state: null };
    return {
      k: 'text-state',
      state: { label: words(m.label, MAX_LABEL), size: isNum(m.size) && m.size > 0 ? m.size : 0 },
    };
  }
  if (m.type === 'tk:text-dynamic') return { k: 'text-dynamic' };
  if (m.type !== 'tk:text-save') return null;
  if (!Number.isInteger(m.tpl) || (m.tpl as number) < 0 || typeof m.was !== 'string') return null;
  const html = typeof m.html === 'string' && m.html.length <= MAX_HTML ? m.html : null;
  const style = readStyle(m.style);
  const fp = words(m.fp, 64);
  if (html === null && (style === null || fp === '')) return null;
  return { k: 'text-save', save: { tpl: m.tpl as number, was: m.was.slice(0, MAX_HTML), html, fp, style } };
};

/**
 * The one patch of an edit, so it is one version of the history: the new words with the styles they were given, or the
 * styles alone (they land by the element as it was); null where nothing is to be written.
 */
export const patchOf = ({ tpl, was, html, fp, style }: ITextSave): TDeckPatch | null => {
  if (html !== null) return { op: 'text', tpl, was, html, ...(style ? { style } : {}) };
  return style && fp !== '' ? { op: 'style', tpl, was: fp, style } : null;
};

/** Writes an edit into the board's source; why it was not taken, else null. */
export const saveHandEdit = async (
  machine: MachineId,
  artifact: string,
  file: string,
  save: ITextSave,
): Promise<TDeckPatchRefusal | null> => {
  const patch = patchOf(save);
  if (patch === null) return null;
  const done = await patchDeck(machine, artifact, { file }, patch);
  return done.ok ? null : done.code;
};

export { DECK_PATCH_REFUSAL as HAND_REFUSAL };

/** «Править текст» of a note's card: the view starts the hand edit on the picked block; null where words are not edited. */
export const BoardTextContext = createContext<((picked: IPicked) => void) | null>(null);
