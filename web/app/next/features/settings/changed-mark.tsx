// A setting an agent has just changed: the line a lit row carries under its hint, with what the value was
// and a way to take the change back, and the journal of the changes as a group of a page.
import type { ReactNode } from 'react';
import { CHANGE_BY, type ISettingChange } from '@/next/features/settings/changes-api';
import { Group } from '@/next/features/settings/rows';
import { t } from '@/next/i18n';
import { formatAgo } from '@/next/i18n/format';
import Icon from '@/next/ui/icon';
import LoadFailed, { LOAD_FAILED_SIZE } from '@/next/ui/load-failed';
import { Skeleton } from '@/next/ui/skeleton';

/** What a lit row says and does. */
export interface IChangedMark {
  /** The value before the agent's change, in words. */
  was: string;
  /** The change is being taken back. */
  busy: boolean;
  onUndo: () => void;
}

/** The lit settings by their catalog key («runtime.idleStop», «background.notify-summary»). */
export type TChangedMarks = Readonly<Record<string, IChangedMark>>;

export const ChangedMark = ({ mark }: { mark: IChangedMark }) => (
  <span className="hx-set__chg">
    {t('next.settingsChanges.mark')} <s>{mark.was}</s>
    <button type="button" className="hx-set__chg-undo" disabled={mark.busy} onClick={mark.onUndo}>
      {t('next.settingsChanges.undo')}
    </button>
  </span>
);

/** The mark of a row by its setting's key, when an agent has just changed it. */
export const markOf = (marks: TChangedMarks, key: string): ReactNode => {
  const mark = marks[key];
  return mark ? <ChangedMark mark={mark} /> : undefined;
};

/** null: the journal is being read. */
export type TJournal =
  | { k: 'ready'; changes: readonly ISettingChange[] | null }
  | { k: 'failed'; text: string; onRetry: () => void };

const whoOf = (change: ISettingChange): string => {
  if (change.by === CHANGE_BY.person) return t('next.settingsChanges.byPerson');
  return change.chat === ''
    ? t('next.settingsChanges.byAgent')
    : t('next.settingsChanges.byAgentOf', { chat: change.chat });
};

interface IChangesGroupProps {
  journal: TJournal;
  now: number;
}

/** «Что менялось»: who changed which setting, from what to what, and the agent's why. */
export const ChangesGroup = ({ journal, now }: IChangesGroupProps) => {
  const title = t('next.settingsChanges.title');
  if (journal.k === 'failed')
    return (
      <Group title={title}>
        <div className="hx-setlog__failed">
          <LoadFailed
            size={LOAD_FAILED_SIZE.inline}
            title={t('next.settingsChanges.loadFailed')}
            reason={journal.text}
            onRetry={journal.onRetry}
          />
        </div>
      </Group>
    );
  if (journal.changes === null)
    return (
      <div className="hx-set__group">
        <div className="hx-set__group-title">{title}</div>
        <Skeleton className="hx-set__sk" />
      </div>
    );
  if (journal.changes.length === 0)
    return (
      <Group title={title}>
        <div className="hx-set__empty">{t('next.settingsChanges.empty')}</div>
      </Group>
    );
  return (
    <Group title={title}>
      {journal.changes.map((change) => (
        <div key={change.id} className="hx-set__row hx-setlog__row">
          <div className="hx-set__row-text">
            <span className="hx-set__row-label">{change.title}</span>
            <span className="hx-setlog__diff">
              <s>{change.oldLabel}</s>
              <Icon name="arrow-right" size="sm" />
              <span>{change.newLabel}</span>
            </span>
            {change.why !== '' && <span className="hx-set__row-hint">{change.why}</span>}
          </div>
          <span className="hx-setlog__who">
            <span>{whoOf(change)}</span>
            <span>{formatAgo(change.at, now)}</span>
          </span>
        </div>
      ))}
    </Group>
  );
};
