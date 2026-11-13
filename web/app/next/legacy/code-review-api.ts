// The review of a merge request over the runtime's code-review routes (cmd/runtime/routes_code_review.go): read with
// the person's GitLab or GitHub connection (or the deployment's, which only reads), and the acts, which the runtime
// does only with the person's own connection; and the open pull requests of the person's GitHub for the start.
import { appFetch } from '@/lib/app-path';
import type {
  IMrAct,
  IMrConnection,
  IMrReview,
  TMrAnswer,
  TMrRefusal,
} from '@/next/features/review/app/mr-review';
import type { IGithubList } from '@/next/features/review/app/home';
import { parseRules, type IReviewRules, type IReviewRulesPatch } from '@/next/features/review/app/rules';
import { parseViewed, type IViewedIo, type TViewedMarks } from '@/next/features/review/viewed';
import type { AppRow } from '@/next/protocol/v2';
import { t } from '@/next/i18n';

const PATH = '/api/platform/code-review';

const refusalOf = async (res: Response): Promise<TMrRefusal> => {
  const body = (await res.json().catch(() => null)) as {
    error?: unknown;
    host?: unknown;
    scope?: unknown;
  } | null;
  const error = typeof body?.error === 'string' ? body.error : null;
  if (error === 'no-connection') return { k: 'no-connection' };
  if (error === 'no-personal')
    return { k: 'no-personal', host: typeof body?.host === 'string' ? body.host : '' };
  if (error === 'denied') return { k: 'denied', scope: typeof body?.scope === 'string' ? body.scope : null };
  return { k: 'failed', text: error ?? t('next.reviewApp.httpError', { status: res.status }) };
};

const isReview = (v: unknown): v is IMrReview =>
  typeof v === 'object' &&
  v !== null &&
  Array.isArray((v as IMrReview).threads) &&
  Array.isArray((v as IMrReview).drafts);

const answerOf = async (res: Response): Promise<TMrAnswer> => {
  if (!res.ok) return { ok: false, refusal: await refusalOf(res) };
  const body = (await res.json().catch(() => null)) as {
    connection?: IMrConnection;
    review?: unknown;
  } | null;
  if (!body?.connection || !isReview(body.review))
    return { ok: false, refusal: { k: 'failed', text: t('next.reviewApp.notRead') } };
  return { ok: true, connection: body.connection, review: body.review };
};

const offline = (): TMrAnswer => ({
  ok: false,
  refusal: { k: 'failed', text: t('next.reviewApp.offlineText') },
});

export const loadMrReview = async (url: string): Promise<TMrAnswer> => {
  try {
    return await answerOf(await appFetch(`${PATH}?${new URLSearchParams({ url })}`));
  } catch {
    return offline();
  }
};

export const actMrReview = async (url: string, act: IMrAct): Promise<TMrAnswer> => {
  try {
    return await answerOf(
      await appFetch(PATH, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url, act }),
      }),
    );
  } catch {
    return offline();
  }
};

export interface INewMr {
  remote: string;
  source: string;
  target: string;
  title: string;
  draft: boolean;
}

export const openMergeRequest = async (
  mr: INewMr,
): Promise<{ ok: true; url: string } | { ok: false; refusal: TMrRefusal }> => {
  try {
    const res = await appFetch(`${PATH}/open`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(mr),
    });
    if (!res.ok) return { ok: false, refusal: await refusalOf(res) };
    const body = (await res.json().catch(() => null)) as { url?: unknown } | null;
    return typeof body?.url === 'string'
      ? { ok: true, url: body.url }
      : { ok: false, refusal: { k: 'failed', text: t('next.reviewApp.notRead') } };
  } catch {
    return { ok: false, refusal: { k: 'failed', text: t('next.reviewApp.offlineText') } };
  }
};

const isRow = (v: unknown): v is AppRow => {
  if (typeof v !== 'object' || v === null) return false;
  const r = v as Partial<AppRow>;
  return (
    typeof r.url === 'string' &&
    typeof r.title === 'string' &&
    typeof r.key === 'string' &&
    Array.isArray(r.me)
  );
};

/** The open pull requests of the person's GitHub for the start of the app; off without a GitHub connection. */
export const loadGithubPulls = async (): Promise<IGithubList> => {
  try {
    const res = await appFetch(`${PATH}/github`);
    const body = (await res.json().catch(() => null)) as Record<string, unknown> | null;
    const state = body?.state;
    if (!res.ok || (state !== 'ready' && state !== 'off' && state !== 'failed'))
      return { state: 'failed', reason: 'unreachable', host: null, rows: [] };
    return {
      state,
      reason: typeof body?.reason === 'string' ? body.reason : null,
      host: typeof body?.host === 'string' ? body.host : null,
      rows: Array.isArray(body?.rows) ? body.rows.filter(isRow) : [],
    };
  } catch {
    return { state: 'failed', reason: 'unreachable', host: null, rows: [] };
  }
};

const JSON_POST = { method: 'POST', headers: { 'Content-Type': 'application/json' } } as const;

/** The person's review rules (GET <platform>/code-review/prefs); null when the runtime did not give them. */
export const loadReviewRules = async (): Promise<IReviewRules | null> => {
  try {
    const res = await appFetch(`${PATH}/prefs`);
    return res.ok ? parseRules(await res.json()) : null;
  } catch {
    return null;
  }
};

export const saveReviewRules = async (patch: IReviewRulesPatch): Promise<IReviewRules | null> => {
  try {
    const res = await appFetch(`${PATH}/prefs`, { ...JSON_POST, body: JSON.stringify(patch) });
    return res.ok ? parseRules(await res.json()) : null;
  } catch {
    return null;
  }
};

const marksOf = async (res: Response): Promise<TViewedMarks | null> => {
  if (!res.ok) return null;
  const body = (await res.json().catch(() => null)) as { marks?: unknown } | null;
  return body && typeof body.marks === 'object' && body.marks !== null
    ? parseViewed(JSON.stringify(body.marks))
    : null;
};

/** The «viewed» marks the machine keeps for a change (path to the file's version); null when it gave none. */
export const loadViewedMarks = async (scope: string): Promise<TViewedMarks | null> => {
  try {
    return await marksOf(await appFetch(`${PATH}/viewed?${new URLSearchParams({ scope })}`));
  } catch {
    return null;
  }
};

/** Puts a file's mark at a version on the machine; an empty version takes it off. */
export const markViewedFile = async (
  scope: string,
  path: string,
  version: string,
): Promise<TViewedMarks | null> => {
  try {
    return await marksOf(
      await appFetch(`${PATH}/viewed`, { ...JSON_POST, body: JSON.stringify({ scope, path, version }) }),
    );
  } catch {
    return null;
  }
};

/** The machine's marks, for the screens that show a change. */
export const VIEWED_IO: IViewedIo = { load: loadViewedMarks, mark: markViewedFile };
