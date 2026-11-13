// The person's review rules as the page holds them: read from the runtime once, changed only by its answer (the
// switch and the fields wait for it). A runtime without the route reads as no rules, and a change there fails.
import { useSyncExternalStore } from 'react';
import { loadReviewRules, saveReviewRules } from '@/next/legacy/code-review-api';
import { NO_RULES, type IReviewRules, type IReviewRulesPatch } from '@/next/features/review/app/rules';

let current: IReviewRules = NO_RULES;
let asked = false;
const listeners = new Set<() => void>();

const put = (rules: IReviewRules | null): boolean => {
  if (!rules) return false;
  current = rules;
  for (const listener of [...listeners]) listener();
  return true;
};

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  if (!asked) {
    asked = true;
    void loadReviewRules().then(put);
  }
  return () => {
    listeners.delete(listener);
  };
};

export const useReviewRules = (): IReviewRules => useSyncExternalStore(subscribe, () => current);

/** Changes the rules on the machine; false when it did not keep them. */
export const changeReviewRules = async (patch: IReviewRulesPatch): Promise<boolean> =>
  put(await saveReviewRules(patch));
