// «Private snapshot» of the share dialog (GitHub #180): the owner picks boards and a term and leaves them as pictures
// for the people who already have access, for while this machine is off. Nothing is published until the owner presses
// the button, and it neither grants access nor takes it away.
import { useEffect, useState } from 'react';
import { type TTtlValue, TTL } from '@/next/features/designly/access/api';
import type { IScopePage } from '@/next/features/designly/access/link-scope';
import { fetchSnapshot, publishSnapshot, removeSnapshot } from '@/next/features/designly/access/snapshot-api';
import TtlPicker, { STEPS } from '@/next/features/designly/access/ttl-picker';
import { useAction } from '@/next/features/designly/access/use-action';
import type { IStoreSnapshot } from '@/next/features/designly/api';
import { t } from '@/next/i18n';

interface IProps {
  project: string;
  pages?: readonly IScopePage[];
}

const AccessSnapshot = ({ project, pages = [] }: IProps) => {
  const action = useAction();
  const [current, setCurrent] = useState<IStoreSnapshot | null>(null);
  const [scope, setScope] = useState<string[] | null>(null);
  const [ttl, setTtl] = useState<TTtlValue>(TTL.week);
  useEffect(() => {
    const abort = new AbortController();
    void fetchSnapshot(project, abort.signal)
      .then((snapshot) => !abort.signal.aborted && setCurrent(snapshot))
      .catch(() => {});
    return () => abort.abort();
  }, [project]);
  const idle = action.busy === null;
  return (
    <section className="hx-acc-sec" data-section="snapshot">
      <h4 className="hx-acc-sec__title">{t('next.access.snapshot.title')}</h4>
      <p className="hx-acc-note">{t('next.access.snapshot.lead')}</p>
      {current && (
        <p className="hx-acc-note" role="status">
          {t('next.access.snapshot.current', {
            at: new Date(current.capturedAt).toLocaleString(),
            until: new Date(current.expiresAt).toLocaleDateString(),
            n: current.boards.length,
          })}
        </p>
      )}
      <div className="hx-acc-row">
        {pages.length > 1 && (
          <fieldset disabled={!idle}>
            <legend>{t('next.access.snapshot.boards')}</legend>
            {pages.map((page) => (
              <label key={page.path}>
                <input
                  type="checkbox"
                  checked={scope === null || scope.includes(page.path)}
                  onChange={(e) => {
                    const held = scope ?? pages.map((p) => p.path);
                    const next = e.target.checked
                      ? [...held, page.path]
                      : held.filter((p) => p !== page.path);
                    setScope(next.length === pages.length ? null : next);
                  }}
                />
                {page.title}
              </label>
            ))}
          </fieldset>
        )}
        <TtlPicker
          label={t('next.access.snapshot.term')}
          value={ttl}
          steps={STEPS}
          withDate={false}
          disabled={!idle}
          onChange={setTtl}
        />
        <button
          type="button"
          className="hx-btn"
          disabled={!idle || scope?.length === 0}
          onClick={() =>
            void action.run('snapshot', async () => setCurrent(await publishSnapshot(project, scope, ttl)))
          }
        >
          {action.busy === 'snapshot'
            ? t('next.access.snapshot.busy')
            : t(current ? 'next.access.snapshot.replace' : 'next.access.snapshot.publish')}
        </button>
        {current && (
          <button
            type="button"
            className="hx-btn"
            data-variant="ghost"
            disabled={!idle}
            onClick={() =>
              void action.run('snapshot-remove', async () => {
                await removeSnapshot(project);
                setCurrent(null);
              })
            }
          >
            {t('next.access.snapshot.remove')}
          </button>
        )}
      </div>
      {action.error && (
        <p className="hx-acc-note" role="alert">
          {action.error}
        </p>
      )}
    </section>
  );
};

export default AccessSnapshot;
