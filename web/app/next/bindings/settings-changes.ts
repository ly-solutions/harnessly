// The journal of the settings' changes on the machine the Settings screen is about: read every few seconds
// while the page is visible, so a row an agent has just changed lights up with what it was and «Вернуть». Taking a
// change back is the person's own change of the same setting to the old value; the row goes dark when the machine has
// written that into the journal.
import { useCallback, useEffect, useEffectEvent, useMemo, useState } from 'react';
import { pageText } from '@/next/bindings/outcome';
import { pageClock } from '@/next/core/clock';
import type { TChangedMarks, TJournal } from '@/next/features/settings/changed-mark';
import {
  CHANGE_BY,
  loadSettingChanges,
  putSetting,
  type ISettingChange,
} from '@/next/features/settings/changes-api';
import { isRefused, type TAnswerOutcome } from '@/next/features/settings/platform-answer';
import { t } from '@/next/i18n';
import type { MachineId } from '@/next/protocol/v2';
import { toast } from '@/next/ui/toast';

export const CHANGES_POLL_MS = 5000;
/** A machine that did not answer (an older runtime has no journal) is asked again only now and then. */
const QUIET_POLL_MS = 60_000;
/** How many lines of the journal the page reads. */
const JOURNAL_LINES = 30;
/** How long a row an agent changed stays lit. */
export const LIT_MS = 10 * 60_000;

/** An answer as the words of why it was not taken, or null. */
export const refusalOf = <T>(res: TAnswerOutcome<T>): string | null => {
  if (!('ok' in res)) return pageText(res);
  if (isRefused(res.value)) return res.value.message || t('next.settings.failed');
  return null;
};

/** The newest change of every setting: the journal comes the newest first. */
export const latestByKey = (changes: readonly ISettingChange[]): ReadonlyMap<string, ISettingChange> => {
  const out = new Map<string, ISettingChange>();
  for (const change of changes) if (!out.has(change.key)) out.set(change.key, change);
  return out;
};

/** The settings to light: the ones whose newest change is an agent's and no older than LIT_MS. */
export const litChanges = (changes: readonly ISettingChange[], now: number): ISettingChange[] =>
  [...latestByKey(changes).values()].filter(
    (change) => change.by === CHANGE_BY.agent && now - change.at < LIT_MS,
  );

export interface ISettingChangesHandle {
  journal: TJournal;
  /** The rows to light, by the setting's key. */
  marks: TChangedMarks;
  /** The id of the newest change among the settings whose key starts with `prefix`; '' when there is none. A page
   *  reads its values again when it moves. */
  stampOf: (prefix: string) => string;
  /** Reads the journal again now (the page has just changed a setting itself). */
  refresh: () => void;
}

export const useSettingChanges = (machine: MachineId, now: number): ISettingChangesHandle => {
  // Kept with the machine it was read from: another machine's journal is not shown while this one's is read.
  const [read, setRead] = useState<{ machine: MachineId; changes: readonly ISettingChange[] } | null>(null);
  const [failure, setFailure] = useState<{ machine: MachineId; text: string } | null>(null);
  const [undoing, setUndoing] = useState<readonly string[]>([]);
  // Moves to read the journal again now, out of the poll's turn.
  const [attempt, setAttempt] = useState(0);

  /** Reads the journal; false when the machine did not give it. */
  const look = useEffectEvent(async (m: MachineId): Promise<boolean> => {
    const res = await loadSettingChanges(m, JOURNAL_LINES);
    const refused = refusalOf(res);
    if (refused !== null || !('ok' in res) || isRefused(res.value)) {
      setFailure({ machine: m, text: refused ?? '' });
      return false;
    }
    const next = res.value;
    setFailure(null);
    setRead((was) => {
      const same =
        was?.machine === m &&
        was.changes.length === next.length &&
        was.changes.every((change, i) => change.id === next[i]?.id);
      return same ? was : { machine: m, changes: next };
    });
    return true;
  });

  useEffect(() => {
    const clock = pageClock();
    let timer: unknown = null;
    let looking = false;
    let wait = CHANGES_POLL_MS;
    const again = () => {
      clock.clearTimeout(timer);
      timer = clock.setTimeout(again, wait);
      if (looking || document.visibilityState !== 'visible') return;
      looking = true;
      void look(machine)
        .catch(() => false)
        .then((read) => {
          looking = false;
          wait = read ? CHANGES_POLL_MS : QUIET_POLL_MS;
        });
    };
    again();
    return () => clock.clearTimeout(timer);
  }, [machine, attempt]);

  const changes = read?.machine === machine ? read.changes : null;
  const refresh = useCallback(() => setAttempt((n) => n + 1), []);

  const undo = useCallback(
    async (change: ISettingChange) => {
      setUndoing((list) => [...list, change.id]);
      const refused = refusalOf(await putSetting(machine, change.key, change.old));
      if (refused === null) {
        // The row stays busy until the journal comes with the person's change above the agent's.
        setAttempt((n) => n + 1);
        return;
      }
      toast(refused, { tone: 'error' });
      setUndoing((list) => list.filter((id) => id !== change.id));
    },
    [machine],
  );

  const marks = useMemo((): TChangedMarks => {
    const out: Record<string, TChangedMarks[string]> = {};
    for (const change of litChanges(changes ?? [], now))
      out[change.key] = {
        was: change.oldLabel,
        busy: undoing.includes(change.id),
        onUndo: () => void undo(change),
      };
    return out;
  }, [changes, now, undoing, undo]);

  const stampOf = useCallback(
    (prefix: string) => (changes ?? []).find((change) => change.key.startsWith(prefix))?.id ?? '',
    [changes],
  );

  let journal: TJournal = { k: 'ready', changes };
  if (changes === null && failure?.machine === machine)
    journal = { k: 'failed', text: failure.text, onRetry: refresh };
  return { journal, marks, stampOf, refresh };
};
