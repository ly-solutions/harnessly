// The pages a person keeps one tap away in the browser tab, and the ones they went to lately. Pure: the store keeps them.
import type { IAddress } from '@/next/features/pane/address';
import { appConfig } from '@/platform/config';

export interface IPin {
  /** The address as the bar shows it; it is also the pin's identity. */
  text: string;
  /** The person's own name for it. */
  name?: string;
  /** A name from the locale files, for the pages every person starts with. */
  key?: string;
}

/** The pages every person starts with: the installation names them (RUNTIME_DEFAULT_PINS), none by default. */
export const defaultPins = (): readonly IPin[] =>
  appConfig()
    .defaultPins.filter((pin) => typeof pin?.text === 'string' && pin.text !== '')
    .map((pin) => (pin.name ? { text: pin.text, name: pin.name } : { text: pin.text }));

export const MAX_PINS = 30;
export const MAX_VISITS = 30;

const isPin = (x: unknown): x is IPin => {
  if (typeof x !== 'object' || x === null) return false;
  const pin = x as Record<string, unknown>;
  return (
    typeof pin.text === 'string' &&
    pin.text !== '' &&
    (pin.name === undefined || typeof pin.name === 'string') &&
    (pin.key === undefined || typeof pin.key === 'string')
  );
};

/** The pins a person saved; null when they saved none yet (then they start with the defaults). */
export const parsePins = (raw: string | null): IPin[] | null => {
  if (raw === null) return null;
  try {
    const saved: unknown = JSON.parse(raw);
    return Array.isArray(saved) ? saved.filter(isPin) : null;
  } catch {
    return null;
  }
};

export const parseVisits = (raw: string | null): string[] => {
  if (raw === null) return [];
  try {
    const saved: unknown = JSON.parse(raw);
    return Array.isArray(saved) ? saved.filter((x): x is string => typeof x === 'string') : [];
  } catch {
    return [];
  }
};

export const isPinned = (pins: readonly IPin[], text: string): boolean => pins.some((p) => p.text === text);

export const addPin = (pins: readonly IPin[], pin: IPin): IPin[] =>
  isPinned(pins, pin.text) ? [...pins] : [...pins, pin].slice(-MAX_PINS);

export const removePin = (pins: readonly IPin[], text: string): IPin[] => pins.filter((p) => p.text !== text);

/** The pin as it is called: the person's name, the shipped name, or the address itself. */
export const pinLabel = (pin: IPin, translate: (key: string) => string): string => {
  if (pin.name) return pin.name;
  if (pin.key) return translate(pin.key);
  return pin.text;
};

/** The pages visited, the latest first, each once. */
export const pushVisit = (visits: readonly string[], text: string): string[] =>
  [text, ...visits.filter((v) => v !== text)].slice(0, MAX_VISITS);

/** A visit worth remembering: a page, not a search. */
export const worthRemembering = (address: IAddress): boolean => !address.text.startsWith('?');
