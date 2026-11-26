import { describe, expect, it } from 'vitest';
import type { ITweak } from '@/next/features/designly/page-bridge';
import { readSiteComment, threadsOf, type ISiteComment } from '@/next/features/designly/panel/api';
import { keptValues, toApply } from '@/next/features/designly/panel/use-site-props';
import { findPages } from '@/next/features/designly/panel/pages-tab';
import { stepOption, tweakChanged, tweakKind, TWEAK_KIND } from '@/next/features/designly/tweaks';

const tweak = (over: Partial<ITweak>): ITweak => ({
  name: 'x',
  label: 'X',
  section: '',
  editor: 'enum',
  value: null,
  def: null,
  options: [],
  min: null,
  max: null,
  step: null,
  ...over,
});

const opts = (...v: string[]) => v.map((value) => ({ value, text: value }));

const comment = (over: Partial<ISiteComment>): ISiteComment => ({
  ...(readSiteComment({ id: 'c' }) as ISiteComment),
  ...over,
});

describe('the design panel (HRN-1764)', () => {
  it('puts the comments into threads, the newest first, replies under their first comment', () => {
    const list = [
      comment({ id: 'a', at: '2026-11-01T10:00:00Z' }),
      comment({ id: 'b', at: '2026-11-02T10:00:00Z' }),
      comment({ id: 'r1', parent: 'a', at: '2026-11-03T10:00:00Z' }),
      comment({ id: 'lost', parent: 'gone' }),
    ];
    const th = threadsOf(list);
    expect(th.map((t) => t.root.id)).toEqual(['b', 'a']);
    expect(th[1].replies.map((r) => r.id)).toEqual(['r1']);
  });

  it('reads a comment answered by the runtime, dropping what is not one', () => {
    expect(readSiteComment({})).toBeNull();
    const c = readSiteComment({
      id: 'x',
      reactions: [{ e: '👍', n: 2, mine: true, names: ['A', 'B'] }],
      agent: true,
    });
    expect(c?.reactions[0]).toEqual({ e: '👍', n: 2, mine: true, names: ['A', 'B'] });
    expect(c?.agent).toBe(true);
  });

  it('keeps for everybody only what stands away from the board', () => {
    const controls = [
      tweak({ name: 'theme', value: 'dark', def: 'light', options: opts('light', 'dark') }),
      tweak({ name: 'period', value: '7', def: '7', options: opts('7', '30') }),
    ];
    expect(keptValues(controls)).toEqual({ theme: 'dark' });
    expect(tweakChanged(controls[0])).toBe(true);
    // Kept values the page does not stand at go to it; a name not kept goes back to the board's own.
    const page = [
      tweak({ name: 'theme', value: 'light', def: 'light', options: opts('light', 'dark') }),
      tweak({ name: 'period', value: '30', def: '7', options: opts('7', '30') }),
      tweak({ name: 'bad', value: 'a', def: 'a', options: opts('a', 'b') }),
    ];
    expect(toApply(page, { theme: 'dark', bad: 'zzz' })).toEqual([
      { name: 'theme', value: 'dark' },
      { name: 'period', value: '7' },
    ]);
  });

  it('draws a scenario as steps and goes round them', () => {
    const steps = tweak({ editor: 'steps', value: 'c', options: opts('a', 'b', 'c') });
    expect(tweakKind(steps)).toBe(TWEAK_KIND.step);
    expect(stepOption(steps, 1).value).toBe('a');
    expect(stepOption(steps, -1).value).toBe('b');
  });

  it('finds the pages by every word of the search', () => {
    const pages = [
      { path: 'Home.dc.html', name: 'Главная' },
      { path: 'Pay.dc.html', name: 'Оплата — телефон' },
    ];
    expect(findPages(pages, 'опл тел').map((p) => p.path)).toEqual(['Pay.dc.html']);
    expect(findPages(pages, '')).toHaveLength(2);
  });
});
