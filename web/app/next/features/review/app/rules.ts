// What the person told the «Ревью кода» app once, kept on the machine (<platform>/code-review/prefs): the instructions
// the agent gets with every review (general, and one repository's own on top), and whether the merge requests of bots
// are hidden from the lists.
import type { TReviewMeta } from '@/next/features/review/model';
import type { AppRow } from '@/next/protocol/v2';

export interface IReviewRules {
  instructions: string;
  /** A repository's own instructions, by its key (`repoKeyOf`). */
  repos: Readonly<Record<string, string>>;
  hideBots: boolean;
  /** Authors the person calls bots too, beside the ones the code host marks. */
  bots: readonly string[];
}

export interface IReviewRulesPatch {
  instructions?: string;
  repo?: { key: string; text: string };
  hideBots?: boolean;
  bots?: string[];
}

/** The rules as a screen holds them, with the way to change them: false when the machine did not keep the change. */
export interface IReviewRulesApi {
  rules: IReviewRules;
  change: (patch: IReviewRulesPatch) => Promise<boolean>;
}

/** As long as a text may be (the runtime refuses a longer one). */
export const RULES_TEXT_MAX = 8000;

export const NO_RULES: IReviewRules = { instructions: '', repos: {}, hideBots: false, bots: [] };

const isRecord = (v: unknown): v is Record<string, unknown> =>
  typeof v === 'object' && v !== null && !Array.isArray(v);

/** The runtime's answer as rules; null when it is not one. */
export const parseRules = (v: unknown): IReviewRules | null => {
  if (!isRecord(v) || typeof v.hideBots !== 'boolean') return null;
  const repos: Record<string, string> = {};
  if (isRecord(v.repos))
    for (const [key, text] of Object.entries(v.repos)) if (typeof text === 'string') repos[key] = text;
  return {
    instructions: typeof v.instructions === 'string' ? v.instructions : '',
    repos,
    hideBots: v.hideBots,
    bots: Array.isArray(v.bots) ? v.bots.filter((b): b is string => typeof b === 'string') : [],
  };
};

/** The repository a change belongs to, as its instructions are kept: `<host>/<project>`; null for local work. */
export const repoKeyOf = (meta: TReviewMeta): string | null => {
  if (meta.kind === 'local' || !meta.url) return null;
  try {
    return `${new URL(meta.url).host}/${meta.project}`.toLowerCase();
  } catch {
    return null;
  }
};

/** What the agent is told for a change of this repository: the general instructions, then the repository's own. */
export const instructionsFor = (rules: IReviewRules, key: string | null): string =>
  [rules.instructions, key ? (rules.repos[key] ?? '') : '']
    .map((text) => text.trim())
    .filter(Boolean)
    .join('\n\n');

const isBot = (row: AppRow, rules: IReviewRules): boolean => {
  if (row.bot) return true;
  const author = row.author?.trim().toLowerCase();
  return Boolean(author) && rules.bots.some((b) => b.trim().toLowerCase() === author);
};

/** The rows the lists show: without the bots' when they are hidden. Words in the field find a hidden one too. */
export const visibleRows = (rows: readonly AppRow[], rules: IReviewRules, q: string): readonly AppRow[] =>
  rules.hideBots && !q.trim() ? rows.filter((row) => !isBot(row, rules)) : rows;
