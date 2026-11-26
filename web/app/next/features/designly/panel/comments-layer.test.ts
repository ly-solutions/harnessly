import { describe, expect, it } from 'vitest';
import { mentionedIn, mentionMatches, mentionQuery } from '@/next/features/designly/panel/comments-layer';

const people = [
  { name: 'Анна', email: 'anna@example.com' },
  { name: 'Пётр', email: 'petr@example.com' },
];

describe('@ in a comment (HRN-1764)', () => {
  it('reads the word being typed after @', () => {
    expect(mentionQuery('Посмотри @Ан')).toBe('Ан');
    expect(mentionQuery('@')).toBe('');
    expect(mentionQuery('почта a@b')).toBeNull();
    expect(mentionQuery('@Анна готово')).toBeNull();
  });

  it('offers the agent first, then the people the word finds', () => {
    expect(mentionMatches(people, '').map((p) => p.name)).toEqual(['harnessly', 'Анна', 'Пётр']);
    expect(mentionMatches(people, 'пёт').map((p) => p.name)).toEqual(['Пётр']);
  });

  it('names the people the text calls, never the agent', () => {
    expect(mentionedIn('@Анна и @harnessly, гляньте', people)).toEqual([
      { name: 'Анна', email: 'anna@example.com' },
    ]);
  });
});
