// The look of a picked block the person changes in the note's card: its type, fill, corner radius and opacity. The page
// shows it at once (the bridge's `tk:styles`); the card writes it into the board's source (hand-edit.ts), or, where the
// source takes no hand edit, the note carries it to the agent. An edit holds only what differs from the block as the page
// has it; null is «as it is».
import type { IPickedLook } from '@/next/features/designly/page-bridge';

export const STYLE_KEYS = [
  'size',
  'weight',
  'slant',
  'line',
  'color',
  'background',
  'radius',
  'opacity',
] as const;
export type TStyleKey = (typeof STYLE_KEYS)[number];

export interface IStyleEdit {
  /** `#rrggbb`; null leaves the fill as the page has it. */
  background: string | null;
  /** Pixels. */
  radius: number | null;
  /** Percent. */
  opacity: number | null;
  /** The type: the size in percent of the block's own, the weight, the colour of the words. */
  size: number | null;
  weight: number | null;
  color: string | null;
  /** CSS `font-style`. */
  slant: TSlant | null;
  /** CSS `text-decoration-line`: `none`, `underline`, `line-through` or both. */
  line: string | null;
}

export type TSlant = 'italic' | 'normal';
export const LINES = ['underline', 'line-through'] as const;
export type TLine = (typeof LINES)[number];

export const NO_EDIT: IStyleEdit = {
  background: null,
  radius: null,
  opacity: null,
  size: null,
  weight: null,
  color: null,
  slant: null,
  line: null,
};

/** The steps of the type's size, percent. */
export const SIZE = { min: 60, max: 200, step: 10 } as const;
export const WEIGHTS = [400, 700] as const;
export const TEXT_SWATCHES = ['#16171A', '#7B6CF0', '#E5484D', '#8A8F98'] as const;
export const RADIUS = { min: 0, max: 40, step: 2 } as const;
export const OPACITY = { min: 10, max: 100, step: 5 } as const;
export const SWATCHES = [
  '#FFFFFF',
  '#0B0B0F',
  '#7C5CFF',
  '#2F80ED',
  '#16A34A',
  '#F59E0B',
  '#E5484D',
] as const;

/** What the block's opacity is in percent. */
export const percentOf = (opacity: number) => Math.round(opacity * 100);

const sameColor = (a: string | null, b: string | null) => a?.toLowerCase() === b?.toLowerCase();

const slantOf = (look: IPickedLook): TSlant => (look.font?.italic ? 'italic' : 'normal');
const lineOf = (look: IPickedLook): string => look.font?.line ?? 'none';

/** The lines with one of them put on or taken off, in the order CSS names them. */
export const toggleLine = (current: string, line: TLine): string => {
  const on = LINES.filter((one) => (one === line) !== current.split(' ').includes(one));
  return on.length > 0 ? on.join(' ') : 'none';
};

/** The edit with `key` set; a value the block already has is no edit. */
export const withValue = <K extends TStyleKey>(
  edit: IStyleEdit,
  look: IPickedLook,
  key: K,
  value: IStyleEdit[K],
): IStyleEdit => {
  const next = { ...edit, [key]: value };
  if (key === 'radius' && next.radius === look.radius) next.radius = null;
  if (key === 'opacity' && next.opacity === percentOf(look.opacity)) next.opacity = null;
  if (key === 'background' && next.background !== null && sameColor(next.background, look.background))
    next.background = null;
  if (key === 'size' && next.size === 100) next.size = null;
  if (key === 'weight' && next.weight === look.font?.weight) next.weight = null;
  if (key === 'color' && next.color !== null && sameColor(next.color, look.font?.color ?? null))
    next.color = null;
  if (key === 'slant' && next.slant === slantOf(look)) next.slant = null;
  if (key === 'line' && next.line === lineOf(look)) next.line = null;
  return next;
};

/** A note kept by an older page has no type of its own: absent is as unset. */
export const changedKeys = (edit: IStyleEdit) => STYLE_KEYS.filter((key) => (edit[key] ?? null) !== null);
export const hasEdit = (edit: IStyleEdit) => changedKeys(edit).length > 0;

/** What a control shows: the edit, else the block's own value (null: unknown, or the fill is as the theme gives it). */
export const shownValues = (edit: IStyleEdit, look: IPickedLook) => ({
  background: edit.background,
  radius: edit.radius ?? look.radius,
  opacity: edit.opacity ?? percentOf(look.opacity),
  size: edit.size ?? 100,
  weight: edit.weight ?? look.font?.weight ?? null,
  color: edit.color,
  slant: edit.slant ?? slantOf(look),
  line: edit.line ?? lineOf(look),
});

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

/** One step of the radius stepper; a radius the page gives in other units starts from nothing. */
export const stepRadius = (current: number | null, direction: -1 | 1) =>
  clamp((current ?? 0) + direction * RADIUS.step, RADIUS.min, RADIUS.max);

export const stepSize = (current: number, direction: -1 | 1) =>
  clamp(current + direction * SIZE.step, SIZE.min, SIZE.max);

/** The font size in pixels the percent of an edit makes of the block's own; null where either is unknown. */
const pixelsOf = (size: number | null, look: IPickedLook | null) =>
  size === null || !look?.font ? null : Math.round((look.font.size * size) / 100);

/** A look as the bridge takes it. */
export interface IStyleItem {
  id: number;
  background: string | null;
  radius: number | null;
  /** 0 to 1. */
  opacity: number | null;
  size?: number;
  weight?: number;
  color?: string;
  slant?: TSlant;
  line?: string;
}

export const toItem = (id: number, edit: IStyleEdit, look: IPickedLook | null = null): IStyleItem => {
  const size = pixelsOf(edit.size ?? null, look);
  return {
    id,
    background: edit.background,
    radius: edit.radius,
    opacity: edit.opacity === null ? null : edit.opacity / 100,
    ...(size === null ? {} : { size }),
    ...(edit.weight ? { weight: edit.weight } : {}),
    ...(edit.color ? { color: edit.color } : {}),
    ...(edit.slant ? { slant: edit.slant } : {}),
    ...(edit.line ? { line: edit.line } : {}),
  };
};

/** The edit as the CSS the board's source takes (the hand edit of the patch route); null where there is none. */
export const toCss = (edit: IStyleEdit, look: IPickedLook | null): Record<string, string> | null => {
  const size = pixelsOf(edit.size ?? null, look);
  const css: Record<string, string> = {
    ...(size === null ? {} : { 'font-size': `${size}px` }),
    ...(edit.weight ? { 'font-weight': String(edit.weight) } : {}),
    ...(edit.slant ? { 'font-style': edit.slant } : {}),
    ...(edit.line ? { 'text-decoration-line': edit.line } : {}),
    ...(edit.color ? { color: edit.color } : {}),
    ...(edit.background ? { background: edit.background } : {}),
    ...((edit.radius ?? null) === null ? {} : { 'border-radius': `${edit.radius}px` }),
    ...((edit.opacity ?? null) === null ? {} : { opacity: String((edit.opacity as number) / 100) }),
  };
  return Object.keys(css).length > 0 ? css : null;
};

/** The looks the page should wear: of every note, the card's own edit in place of its note's. */
interface IStyled {
  picked: { id: number | null; look?: IPickedLook | null };
  style: IStyleEdit;
}

export const itemsOf = (
  notes: readonly (IStyled & { id: string })[],
  draft: (IStyled & { id?: string }) | null,
): IStyleItem[] => {
  const items: IStyleItem[] = [];
  for (const note of notes) {
    const own = draft?.id === note.id ? draft : note;
    if (own.picked.id !== null && hasEdit(own.style))
      items.push(toItem(own.picked.id, own.style, own.picked.look));
  }
  if (draft && draft.id === undefined && draft.picked.id !== null && hasEdit(draft.style))
    items.push(toItem(draft.picked.id, draft.style, draft.picked.look));
  return items;
};

/** The line of the annotation block: what is tried on the page and was before, in CSS the agent can put into the source. */
export const styleLine = (edit: IStyleEdit, look: IPickedLook | null): string => {
  const parts: string[] = [];
  if ((edit.size ?? null) !== null) {
    const pixels = pixelsOf(edit.size, look);
    parts.push(
      pixels === null ? `font-size: ${edit.size}%` : `font-size: ${pixels}px (was ${look?.font?.size}px)`,
    );
  }
  if ((edit.weight ?? null) !== null) {
    const was = look?.font ? ` (was ${look.font.weight})` : '';
    parts.push(`font-weight: ${edit.weight}${was}`);
  }
  if ((edit.slant ?? null) !== null) {
    parts.push(`font-style: ${edit.slant}`);
  }
  if ((edit.line ?? null) !== null) {
    parts.push(`text-decoration-line: ${edit.line}`);
  }
  if ((edit.color ?? null) !== null) {
    const was = look?.font?.color ? ` (was ${look.font.color})` : '';
    parts.push(`color: ${edit.color}${was}`);
  }
  if (edit.background !== null) {
    const was = look?.background ? ` (was ${look.background})` : '';
    parts.push(`background: ${edit.background}${was}`);
  }
  if (edit.radius !== null) {
    const was = look && look.radius !== null ? ` (was ${look.radius}px)` : '';
    parts.push(`border-radius: ${edit.radius}px${was}`);
  }
  if (edit.opacity !== null) {
    const was = look ? ` (was ${Number(look.opacity.toFixed(2))})` : '';
    parts.push(`opacity: ${edit.opacity / 100}${was}`);
  }
  return parts.join('; ');
};
