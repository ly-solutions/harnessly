import { describe, expect, it } from 'vitest';
import { DEFAULT_PREFS, parsePrefs } from '@/next/features/review/prefs';

describe('review prefs', () => {
  it('never chose the column: null, so the place decides', () => {
    expect(parsePrefs(null).panel).toBeNull();
    expect(parsePrefs('{"layout":"inline"}').panel).toBeNull();
  });
  it('keeps the chosen column state, open or folded', () => {
    expect(parsePrefs('{"panel":true}').panel).toBe(true);
    expect(parsePrefs('{"panel":false}').panel).toBe(false);
    expect(parsePrefs('{"panel":"yes"}').panel).toBeNull();
    expect(parsePrefs('{broken')).toEqual(DEFAULT_PREFS);
  });
});
