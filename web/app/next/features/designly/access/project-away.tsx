// What a person with a place in a project sees when the project can be had from neither side: the
// hub's store has no copy of it (or could not be read) and the machine did not answer (or does not have it). A project
// the store or a machine on the link can give opens without this card. The card names the reason as it is; the page
// keeps looking on its own and opens the project when it arrives, unless nothing can bring it (a hub with no store, a
// machine closed to the person: GitHub #82); the way back is always there.
import { useEffect, useState } from 'react';
import {
  storeBoardHref,
  storeIndexHref,
  storeSnapshotHref,
  storeSnapshotImageHref,
  type IStoreSnapshot,
} from '@/next/features/designly/api';
import { t } from '@/next/i18n';
import { AWAY, type TAway } from '@/next/features/designly/project-read';
import '@/next/styles/design-access.css';

interface IProjectAwayProps {
  /** Why the project is on neither side. */
  why: TAway;
  project: string;
  /** The card key of the holder's copy this project is opened from (several holders may share a slug). */
  card?: string;
  page: string | null;
  onPage?: (page: string) => void;
  /** The owner's name from the gateway; empty when it is not known. */
  ownerName: string;
  onBack: () => void;
}

const TITLE = {
  [AWAY.silentNoCopy]: 'next.access.away.silentNoCopy.title',
  [AWAY.silentUnreadable]: 'next.access.away.silentUnreadable.title',
  [AWAY.absentNoCopy]: 'next.access.away.absentNoCopy.title',
  [AWAY.absentUnreadable]: 'next.access.away.absentUnreadable.title',
  [AWAY.silentStoreOff]: 'next.access.away.silentStoreOff.title',
  [AWAY.absentStoreOff]: 'next.access.away.absentStoreOff.title',
  [AWAY.machineDenied]: 'next.access.away.machineDenied.title',
} as const;

const LEAD = {
  [AWAY.silentNoCopy]: 'next.access.away.silentNoCopy.lead',
  [AWAY.silentUnreadable]: 'next.access.away.silentUnreadable.lead',
  [AWAY.absentNoCopy]: 'next.access.away.absentNoCopy.lead',
  [AWAY.absentUnreadable]: 'next.access.away.absentUnreadable.lead',
  [AWAY.silentStoreOff]: 'next.access.away.silentStoreOff.lead',
  [AWAY.absentStoreOff]: 'next.access.away.absentStoreOff.lead',
  [AWAY.machineDenied]: 'next.access.away.machineDenied.lead',
} as const;

const ProjectAway = ({ why, ownerName, onBack, project, card, page, onPage }: IProjectAwayProps) => {
  const [boards, setBoards] = useState<{ title: string; path: string }[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [snapshot, setSnapshot] = useState<IStoreSnapshot | null>(null);
  const [shot, setShot] = useState(0);
  useEffect(() => {
    if (why !== AWAY.absentStoreOff && why !== AWAY.silentStoreOff) return;
    const abort = new AbortController();
    void fetch(storeIndexHref(project, card), {
      signal: abort.signal,
      cache: 'no-store',
    })
      .then(async (response) => (response.ok ? response.json() : null))
      .then(async (index) => {
        if (abort.signal.aborted) return;
        if (Array.isArray(index?.boards)) {
          setBoards(
            index.boards.filter(
              (board: { path?: unknown; title?: unknown } | null) =>
                typeof board?.path === 'string' && typeof board?.title === 'string',
            ),
          );
          return;
        }
        // The owner's machine did not give the project: the snapshot the owner left, if there is one. The hub answers it
        // only while the machine is offline and the person still holds a grant.
        const response = await fetch(storeSnapshotHref(project, card), {
          signal: abort.signal,
          cache: 'no-store',
        });
        const left = response.ok ? ((await response.json()) as IStoreSnapshot) : null;
        if (!abort.signal.aborted && Array.isArray(left?.boards) && left.boards.length > 0) setSnapshot(left);
      })
      .catch(() => {});
    return () => abort.abort();
  }, [why, project, card]);
  const selectedPage = page ?? selected;
  const current = selectedPage ? boards.find((board) => board.path === selectedPage) : boards[0];
  if (boards.length === 0 && snapshot) {
    const board = snapshot.boards.find((b) => b.n === shot) ?? snapshot.boards[0];
    return (
      <section style={{ display: 'flex', flexDirection: 'column', height: '100%' }} data-snapshot="">
        <p role="status">
          {t('next.access.snapshot.view.captured', { at: new Date(snapshot.capturedAt).toLocaleString() })}
        </p>
        <nav>
          {snapshot.boards.map((b) => (
            <button type="button" key={b.n} onClick={() => setShot(b.n)} aria-pressed={board.n === b.n}>
              {b.title}
            </button>
          ))}
        </nav>
        <div style={{ flex: 1, overflow: 'auto' }}>
          <img
            style={{ maxWidth: '100%', height: 'auto' }}
            src={storeSnapshotImageHref(project, board.n, card)}
            alt={t('next.access.snapshot.view.alt', { title: board.title })}
          />
        </div>
      </section>
    );
  }
  if (boards.length > 0)
    return (
      <section style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        <nav>
          {boards.map((board) => (
            <button
              type="button"
              key={board.path}
              onClick={() => (onPage ? onPage(board.path) : setSelected(board.path))}
              aria-pressed={current?.path === board.path}
            >
              {board.title}
            </button>
          ))}
        </nav>
        {current ? (
          <iframe
            style={{ flex: 1, border: 0 }}
            title={current.title}
            sandbox="allow-scripts"
            src={storeBoardHref(project, current.path, '', card)}
          />
        ) : (
          <p role="status">{t('next.access.pageNotFound')}</p>
        )}
      </section>
    );
  return (
    <section className="hx-acc-card" aria-label={t(TITLE[why])} role="status" data-why={why}>
      <h2 className="hx-acc-card__title">{t(TITLE[why])}</h2>
      <p className="hx-acc-card__lead">{t(LEAD[why])}</p>
      {ownerName && <p className="hx-acc-card__lead">{t('next.access.owner', { name: ownerName })}</p>}
      <div className="hx-acc-card__acts">
        <button type="button" className="hx-btn" data-variant="ghost" onClick={onBack}>
          {t('next.access.back')}
        </button>
      </div>
    </section>
  );
};

export default ProjectAway;
