import { describe, expect, it } from 'vitest';
import { isQuestion, keywordsOf, matching, termsOf } from '@/next/features/search/terms';

describe('the words of the search', () => {
  it('tells a question for the model from plain words', () => {
    expect(isQuestion('где мы решили про провайдеров')).toBe(true);
    expect(isQuestion('сводки уведомлений?')).toBe(true);
    expect(isQuestion('провайдеры')).toBe(false);
    expect(isQuestion('как')).toBe(false);
  });

  it('keeps what a question is about, without the endings', () => {
    expect(keywordsOf('Где мы решили про провайдеров?')).toEqual(['провайдер', 'реши']);
  });

  it('wants every word of a plain query and any keyword of a question', () => {
    const items = ['Провайдеры по API', 'Сводки уведомлений', 'Провайдеры и сводки'];
    const texts = (item: string) => [item];
    expect(matching(items, termsOf('провайдеры сводки'), texts)).toEqual(['Провайдеры и сводки']);
    expect(matching(items, termsOf('где провайдеры и сводки?'), texts)).toEqual([
      'Провайдеры и сводки',
      'Провайдеры по API',
      'Сводки уведомлений',
    ]);
    expect(matching(items, termsOf(''), texts)).toEqual([]);
  });
});
