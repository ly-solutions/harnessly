// What the routines app (internal/apps/builtin/routines) shows of a routine and its runs, worked out from what
// hl.routines answers. Pure: the cards and the widgets draw what this says.
import type {
  IRoutineArtifact,
  IRoutineInfo,
  IRoutineOfChat,
  IRoutineRun,
} from '@/next/features/app-views/routines';

/** How a run ended, as the app says it; `running` while it works, `waiting` before it started. */
export type TRunState = 'ok' | 'quiet' | 'failed' | 'running' | 'waiting';

/** The routine of a run's chat, with the run itself: what the app draws. */
export interface IRunChat {
  routine: IRoutineInfo;
  run: IRoutineRun;
  runs: readonly IRoutineRun[];
}

/** The answer of hl.routines.forChat as a run's chat, or null for any other chat. */
export const runChatOf = (got: IRoutineOfChat | null | undefined): IRunChat | null =>
  got?.routine && got.run ? { routine: got.routine, run: got.run, runs: got.runs ?? [] } : null;

/** The run is over: it reported, or it failed without a report. */
export const isOver = (run: IRoutineRun): boolean =>
  run.outcome !== undefined || run.status === 'done' || run.status === 'failed' || run.status === 'rejected';

export const runState = (run: IRoutineRun): TRunState => {
  if (run.outcome) return run.outcome;
  if (run.status === 'failed' || run.status === 'rejected') return 'failed';
  if (run.status === 'dispatched' || run.status === 'running') return 'running';
  if (run.status === 'done') return 'ok';
  return 'waiting';
};

/** The run was started by hand («Запустить сейчас»), not by the schedule. */
export const isManual = (run: IRoutineRun): boolean =>
  run.fingerprint.startsWith('manual:') || run.result?.manual === true;

/** The run's summary: what the agent reported, else why it failed. */
export const runSummary = (run: IRoutineRun): string =>
  (run.result?.summary || run.summary || run.result?.error || run.error || '').trim();

const keyOf = (a: IRoutineArtifact): string => (a.url || `${a.kind}:${a.title}`).trim();

/** The artifacts of all the runs, newest first, each once (the same document of every run is its latest version). */
export const allArtifacts = (
  runs: readonly IRoutineRun[],
): { artifact: IRoutineArtifact; run: IRoutineRun }[] => {
  const seen = new Set<string>();
  const out: { artifact: IRoutineArtifact; run: IRoutineRun }[] = [];
  for (const run of runs)
    for (const artifact of run.result?.artifacts ?? []) {
      const key = keyOf(artifact);
      if (artifact.title.trim() === '' || seen.has(key)) continue;
      seen.add(key);
      out.push({ artifact, run });
    }
  return out;
};

/** The sources of this run; a run that named none shows those of the latest run that did. */
export const sourcesOf = (chat: IRunChat): readonly IRoutineArtifact[] => {
  const own = chat.run.result?.sources ?? [];
  if (own.length > 0 || !isOver(chat.run)) return own;
  return chat.runs.find((r) => (r.result?.sources?.length ?? 0) > 0)?.result?.sources ?? [];
};

/** The host of a link, for a source's second line. */
export const hostOf = (url: string | undefined): string => {
  if (!url) return '';
  try {
    return new URL(url).host.replace(/^www\./, '');
  } catch {
    return '';
  }
};

const DAY = 86_400_000;
const startOfDay = (d: Date): number => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

/**
 * A moment in the person's words: «сегодня, 09:00», «завтра, 09:00», «вчера, 18:30», else the date
 * («28 нояб., 09:00»). The words of the days come from the caller (the dictionary).
 */
export const whenText = (
  iso: string,
  now: Date,
  words: { today: string; tomorrow: string; yesterday: string },
  lang: string,
): string => {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return '';
  const hm = at.toLocaleTimeString(lang, { hour: '2-digit', minute: '2-digit' });
  const days = Math.round((startOfDay(at) - startOfDay(now)) / DAY);
  if (days === 0) return `${words.today}, ${hm}`;
  if (days === 1) return `${words.tomorrow}, ${hm}`;
  if (days === -1) return `${words.yesterday}, ${hm}`;
  const date = at.toLocaleDateString(lang, {
    day: 'numeric',
    month: 'short',
    ...(at.getFullYear() === now.getFullYear() ? {} : { year: 'numeric' }),
  });
  return `${date}, ${hm}`;
};

/** The schedule in words: as the person said it, else the cron as it is. */
export const scheduleText = (routine: IRoutineInfo): string =>
  routine.routine.words?.trim() || routine.trigger.schedule?.cron || '';
