// The words the search looks by: every word of a plain query, or the keywords of a question for the model.
import { foldText } from '@/next/features/search/highlight';

const QUESTION_START =
  /^(где|как|что|почему|зачем|когда|кто|какой|какая|какие|сколько|where|how|what|why|when|who|which)\s+\S+/i;

/** Whether the words are a question for the model: a question mark at the end, or a question word and more. */
export const isQuestion = (query: string): boolean => {
  const text = query.trim();
  return /\?$/.test(text) || QUESTION_START.test(text);
};

/** Words of a question that say nothing of what it is about. */
const STOP = new Set(
  'где как что почему зачем когда кто какой какая какие сколько это этот эта эти или для при про над под без был была были быть есть если чтобы тоже также уже еще наш наша наши мой моя мои where how what why when who which the and for with that this was were are our have has did does'.split(
    ' ',
  ),
);
const KEYWORD_FROM = 4;
const KEYWORDS_MAX = 4;

/** The start of a word that its other forms share: «провайдеров» is found by «провайдер». */
const stemOf = (word: string): string =>
  word.length > KEYWORD_FROM + 2 ? word.slice(0, word.length - 2) : word.slice(0, KEYWORD_FROM);

/** What a question is about: its longest words without their endings, the longest first. */
export const keywordsOf = (question: string): string[] =>
  [...new Set(foldText(question).split(/[^\p{L}\p{N}]+/u))]
    .filter((word) => [...word].length >= KEYWORD_FROM && !STOP.has(word))
    .sort((a, b) => b.length - a.length)
    .slice(0, KEYWORDS_MAX)
    .map(stemOf);

/** The words a text is looked for by: every one of them for a plain query, any of a question's keywords. */
export interface ITerms {
  list: readonly string[];
  any: boolean;
}

/** `ask` is set when the person asked the model by hand (Tab) about words that do not read as a question. */
export const termsOf = (query: string, ask: boolean = isQuestion(query)): ITerms =>
  ask
    ? { list: keywordsOf(query), any: true }
    : { list: foldText(query).trim().split(/\s+/).filter(Boolean), any: false };

/** How many of the words the texts hold; 0 when they do not answer the terms. */
export const scoreOf = (terms: ITerms, ...texts: string[]): number => {
  const all = foldText(texts.join(' '));
  const hits = terms.list.filter((word) => all.includes(word)).length;
  return terms.any || hits === terms.list.length ? hits : 0;
};

/** The items that answer the terms, the best first (the order they came in decides between equals). */
export const matching = <T>(items: readonly T[], terms: ITerms, textsOf: (item: T) => string[]): T[] =>
  items
    .map((item, at) => ({ item, at, score: scoreOf(terms, ...textsOf(item)) }))
    .filter((scored) => scored.score > 0)
    .sort((a, b) => b.score - a.score || a.at - b.at)
    .map((scored) => scored.item);
