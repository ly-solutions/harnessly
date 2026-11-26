// «Макет → Версии»: the project's history as the versions panel had it, compact — a line each, «Сравнить» and
// «Вернуть» on hover, the name of a new version at the top.
import { useState } from 'react';
import { Button } from '@/next/ui/button';
import Icon from '@/next/ui/icon';
import { t } from '@/next/i18n';
import type { MachineId } from '@/next/protocol/v2';
import { Compare } from '@/next/features/designly/versions/versions-panel';
import {
  restoreProjectVersion,
  saveProjectVersion,
  type IProjectHistory,
  type IProjectPoint,
} from '@/next/features/designly/versions/versions-api';
import { whenOf, whoOf } from '@/next/features/designly/panel/when';

export interface IVersionsTabProps {
  /** The project's owner: the person behind this machine's own edits. */
  owner: string | null;
  /** A person's name by their address, where it is known. */
  nameOf?: (email: string) => string | null;
  machine: MachineId;
  id: string;
  origin: string | null;
  page: string | null;
  mayEdit: boolean;
  history: IProjectHistory | null;
  error: string | null;
  reload: () => void;
  onRestored: () => void;
}

const VersionsTab = ({
  owner,
  nameOf,
  machine,
  id,
  origin,
  page,
  mayEdit,
  history,
  error,
  reload,
  onRestored,
}: IVersionsTabProps) => {
  const [name, setName] = useState('');
  const [saving, setSaving] = useState(false);
  const [restoring, setRestoring] = useState<string | null>(null);
  const [comparing, setComparing] = useState<string | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const fail = (e: unknown) => setFailure(e instanceof Error ? e.message : t('next.versions.failed'));
  const save = async () => {
    setSaving(true);
    setFailure(null);
    try {
      await saveProjectVersion(machine, id, name);
      setName('');
      reload();
    } catch (e) {
      fail(e);
    } finally {
      setSaving(false);
    }
  };
  const restore = async (point: IProjectPoint) => {
    setRestoring(point.id);
    setFailure(null);
    try {
      await restoreProjectVersion(machine, id, point.id);
      reload();
      onRestored();
    } catch (e) {
      fail(e);
    } finally {
      setRestoring(null);
    }
  };
  const points = history?.points ?? [];
  const named = new Set(history?.named.map((n) => n.id) ?? []);
  return (
    <div className="hx-ds__pane">
      {mayEdit && (
        <form
          className="hx-ds-save"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim() !== '') void save();
          }}
        >
          <input
            value={name}
            maxLength={80}
            placeholder={t('next.versions.nameHint')}
            aria-label={t('next.versions.nameHint')}
            onChange={(e) => setName(e.target.value)}
          />
          <Button size="sm" busy={saving} disabled={name.trim() === ''} onClick={() => void save()}>
            {t('next.versions.save')}
          </Button>
        </form>
      )}
      {(failure ?? error) !== null && <div className="hx-ds-error">{failure ?? error}</div>}
      {points.map((p, index) => {
        const current = index === 0 && !history?.unsaved;
        return (
          <div key={p.id}>
            <div
              className="hx-ds-ver"
              data-current={current || undefined}
              data-named={named.has(p.id) || undefined}
            >
              <Icon name={named.has(p.id) ? 'bookmark' : p.agent ? 'sparkles' : 'user'} size="sm" />
              <div className="hx-ds-ver__main">
                <div className="hx-ds-ver__t">{p.name ?? p.subject}</div>
                <div className="hx-ds__muted">
                  {whenOf(p.at)} · {whoOf(p.who, p.agent, owner, nameOf)}
                </div>
              </div>
              {current ? (
                <span className="hx-ds-ver__now">{t('next.ds.versions.now')}</span>
              ) : (
                <div className="hx-ds-ver__acts">
                  <button
                    type="button"
                    className="hx-ds-icon-btn"
                    aria-label={t('next.versions.compare')}
                    title={t('next.versions.compare')}
                    aria-expanded={comparing === p.id}
                    onClick={() => setComparing(comparing === p.id ? null : p.id)}
                  >
                    <Icon name="git-compare" size="sm" />
                  </button>
                  {mayEdit && (
                    <button
                      type="button"
                      className="hx-ds-icon-btn"
                      aria-label={t('next.versions.restore')}
                      title={restoring === p.id ? t('next.versions.restoring') : t('next.versions.restore')}
                      disabled={restoring !== null}
                      onClick={() => void restore(p)}
                    >
                      <Icon name="undo-2" size="sm" />
                    </button>
                  )}
                </div>
              )}
            </div>
            {comparing === p.id && (
              <Compare machine={machine} id={id} origin={origin} point={p} page={page} />
            )}
          </div>
        );
      })}
      {history !== null && points.length === 0 && (
        <div className="hx-ds__empty">{t('next.versions.empty')}</div>
      )}
    </div>
  );
};

export default VersionsTab;
