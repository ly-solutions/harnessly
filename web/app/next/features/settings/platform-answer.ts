// How the runtime's plain resources answer the page (cmd/runtime/routes_extproviders.go and its neighbours): the value,
// or a refusal {error: <reason>, message: <words for the person>} with a 4xx or 5xx. The readers here turn the refusal
// into a value of its own, so the page shows the runtime's words and not the status.
import type { PageOutcome } from '@/next/core/net/http';

/** A refusal: the reason by its code and the runtime's words for the person ('' when it gave none). */
export interface IRefused {
  refused: string;
  message: string;
}

export type TAnswer<T> = T | IRefused;
export type TAnswerOutcome<T> = PageOutcome<TAnswer<T>>;

export const isRefused = (v: unknown): v is IRefused => typeof v === 'object' && v !== null && 'refused' in v;

export const obj = (v: unknown, what: string): Record<string, unknown> => {
  if (typeof v !== 'object' || v === null || Array.isArray(v)) throw new Error(`answer: ${what}`);
  return v as Record<string, unknown>;
};
export const str = (v: unknown, what: string): string => {
  if (typeof v !== 'string') throw new Error(`answer: ${what}`);
  return v;
};
export const num = (v: unknown, what: string): number => {
  if (typeof v !== 'number') throw new Error(`answer: ${what}`);
  return v;
};
export const bool = (v: unknown, what: string): boolean => {
  if (typeof v !== 'boolean') throw new Error(`answer: ${what}`);
  return v;
};
/** A list; the runtime writes an empty one as null now and then. */
export const list = <T>(v: unknown, what: string, read: (x: unknown) => T): T[] => {
  if (v === null || v === undefined) return [];
  if (!Array.isArray(v)) throw new Error(`answer: ${what}`);
  return v.map(read);
};
/** A text that may be absent. */
export const opt = (v: unknown): string => (typeof v === 'string' ? v : '');

/** The reader of an answer: `read` for the value, the refusal as it is. */
export const answer =
  <T>(read: (o: Record<string, unknown>) => T) =>
  (v: unknown): TAnswer<T> => {
    const o = obj(v, 'body');
    if (typeof o.error === 'string') return { refused: o.error, message: opt(o.message) };
    return read(o);
  };
