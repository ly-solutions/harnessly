import { describe, expect, it, vi } from 'vitest';
import {
  addPin,
  defaultPins,
  isPinned,
  MAX_VISITS,
  parsePins,
  parseVisits,
  pinLabel,
  pushVisit,
  removePin,
  worthRemembering,
} from '@/next/features/pane/pins';

vi.mock('@/platform/config', async (importOriginal) => {
  const real = await importOriginal<typeof import('@/platform/config')>();
  return {
    ...real,
    appConfig: () => ({
      ...real.appConfig(),
      defaultPins: [
        { text: 'admin.example.com', name: 'Admin' },
        { text: 'shop.example.com', name: '' },
      ],
    }),
  };
});
import { searchAddress } from '@/next/features/pane/address';

describe('pins', () => {
  it('adds a page once and removes it by its address', () => {
    const one = addPin([], { text: 'a.example.org' });
    expect(addPin(one, { text: 'a.example.org', name: 'again' })).toEqual(one);
    expect(isPinned(one, 'a.example.org')).toBe(true);
    expect(removePin(one, 'a.example.org')).toEqual([]);
  });

  it("is called by the person's name, the shipped name, or the address", () => {
    const translate = (key: string) => `T:${key}`;
    expect(pinLabel({ text: 'x.org', name: 'Mine', key: 'k' }, translate)).toBe('Mine');
    expect(pinLabel({ text: 'x.org', key: 'k' }, translate)).toBe('T:k');
    expect(pinLabel({ text: 'x.org' }, translate)).toBe('x.org');
  });

  it('starts with the pages the installation names', () => {
    expect(defaultPins()).toEqual([
      { text: 'admin.example.com', name: 'Admin' },
      { text: 'shop.example.com' },
    ]);
  });

  it('reads what was saved, and nothing for a broken or absent record', () => {
    expect(parsePins(null)).toBeNull();
    expect(parsePins('{oops')).toBeNull();
    expect(parsePins('{"a":1}')).toBeNull();
    expect(parsePins('[]')).toEqual([]);
    expect(parsePins('[{"text":"a.org","name":"A"},{"text":3},"x"]')).toEqual([{ text: 'a.org', name: 'A' }]);
  });

  it('keeps visits latest first, each once, and a limited number', () => {
    expect(pushVisit(['a', 'b', 'c'], 'b')).toEqual(['b', 'a', 'c']);
    const many = Array.from({ length: MAX_VISITS + 5 }, (_, i) => `h${i}`);
    expect(pushVisit(many, 'new')).toHaveLength(MAX_VISITS);
    expect(parseVisits('["a",1,"b"]')).toEqual(['a', 'b']);
    expect(parseVisits('nope')).toEqual([]);
  });

  it('does not remember searches', () => {
    expect(worthRemembering(searchAddress('words'))).toBe(false);
    expect(worthRemembering({ target: { kind: 'port', port: 1 }, path: '/', text: 'localhost:1' })).toBe(
      true,
    );
  });
});
