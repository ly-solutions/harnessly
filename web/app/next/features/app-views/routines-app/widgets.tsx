// The routines app's widgets in a run's chat (the `side` views of internal/apps/builtin/routines): under the chat's
// right column, beside «Документы» and the person's widgets, in the same cards (hx-wg). Results of every run, the
// sources, the last runs (a click opens that run's chat, this one is marked), the next starts and the memory the runs
// keep. A chat that is no run of a routine shows none of them, and asks the hub nothing.
import { useState, type ReactNode } from 'react';
import { ArtifactRow, STATE_ICON, when } from '@/next/features/app-views/routines-app/cards';
import {
  allArtifacts,
  hostOf,
  runState,
  runSummary,
  sourcesOf,
  type IRunChat,
} from '@/next/features/app-views/routines-app/model';
import { useRunChat } from '@/next/features/app-views/routines-app/store';
import { BUILTIN_APPS } from '@/next/features/app-views/surface.gen';
import { t } from '@/next/i18n';
import type { ConvId, MachineId } from '@/next/protocol/v2';
import { goTo, ROUTE } from '@/next/routes/routes';
import Icon from '@/next/ui/icon';
import '@/next/styles/widgets.css';
import '@/next/styles/routines-app.css';

type TSideView = 'results' | 'sources' | 'runs' | 'next' | 'memory';

/** The app's side views in the order of its manifest, with their names and icons. */
const isSideView = (id: string): id is TSideView =>
  id === 'results' || id === 'sources' || id === 'runs' || id === 'next' || id === 'memory';

const SIDE_VIEWS: readonly { id: TSideView; icon: string }[] = (
  BUILTIN_APPS.find((a) => a.id === 'harnessly.routines')?.views ?? []
).flatMap((v: { id: string; icon: string; targets: readonly string[] }) =>
  v.targets.includes('side') && isSideView(v.id) ? [{ id: v.id, icon: v.icon }] : [],
);

const FOLD_KEY = 'hx-rtw-folded';
const readFolded = (): Set<string> => {
  try {
    const v: unknown = JSON.parse(localStorage.getItem(FOLD_KEY) ?? '[]');
    return new Set(Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []);
  } catch {
    return new Set();
  }
};
const saveFolded = (s: ReadonlySet<string>) => {
  try {
    localStorage.setItem(FOLD_KEY, JSON.stringify([...s]));
  } catch {
    // Blocked storage: the fold holds while the page is open.
  }
};

const Card = ({
  id,
  title,
  icon,
  folded,
  onFold,
  children,
}: {
  id: string;
  title: string;
  icon: string;
  folded: boolean;
  onFold: (id: string, folded: boolean) => void;
  children: ReactNode;
}) => (
  <section className="hx-wg hx-rtw" data-view={id} data-folded={folded || undefined} aria-label={title}>
    <header className="hx-wg__head">
      <button
        type="button"
        className="hx-wg__name"
        aria-expanded={!folded}
        title={t(folded ? 'next.routinesApp.unfold' : 'next.routinesApp.fold')}
        onClick={() => onFold(id, !folded)}
      >
        <Icon name={icon} size="sm" className="hx-wg__icon" />
        <span className="hx-wg__title">{title}</span>
        <Icon name="chevron-down" size="sm" className="hx-wg__chev" />
      </button>
    </header>
    <div className="hx-wg__clip" inert={folded || undefined}>
      <div className="hx-wg__keep">
        <div className="hx-wg__body">{children}</div>
      </div>
    </div>
  </section>
);

const Quiet = ({ text }: { text: string }) => <p className="hx-wg__quiet">{text}</p>;

const Results = ({ chat }: { chat: IRunChat }) => {
  const all = allArtifacts(chat.runs).slice(0, 8);
  if (all.length === 0) return <Quiet text={t('next.routinesApp.empty.results')} />;
  return (
    <div className="hx-rtw__arts">
      {all.map(({ artifact, run }, i) => (
        <ArtifactRow key={i} artifact={artifact} meta={when(run.finishedAt ?? run.createdAt)} />
      ))}
    </div>
  );
};

const Sources = ({ chat }: { chat: IRunChat }) => {
  const list = sourcesOf(chat);
  if (list.length === 0) return <Quiet text={t('next.routinesApp.empty.sources')} />;
  return (
    <div className="hx-wgr">
      {list.map((s, i) => {
        const body = (
          <>
            <Icon name="globe" size="sm" className="hx-rtw__ic" />
            <span className="hx-wgr__title">{s.title || hostOf(s.url)}</span>
            {s.url && <span className="hx-rtw__meta">{hostOf(s.url)}</span>}
          </>
        );
        return s.url ? (
          <a key={i} className="hx-wgr__row" href={s.url} target="_blank" rel="noreferrer" title={s.url}>
            {body}
          </a>
        ) : (
          <div key={i} className="hx-wgr__row">
            {body}
          </div>
        );
      })}
    </div>
  );
};

const Runs = ({ chat }: { chat: IRunChat }) => (
  <div className="hx-wgr">
    {chat.runs.slice(0, 6).map((run) => {
      const state = runState(run);
      const current = run.id === chat.run.id;
      const conv = run.conversationId;
      return (
        <button
          key={run.id}
          type="button"
          className="hx-wgr__row hx-rtw__run"
          data-current={current || undefined}
          aria-current={current || undefined}
          disabled={!conv || current}
          onClick={() => conv && goTo({ kind: ROUTE.chat, id: conv })}
        >
          <span className="hx-rtw__dot" data-state={state}>
            <Icon name={STATE_ICON[state]} size="sm" />
          </span>
          <span className="hx-rtw__runtext">
            <span className="hx-rtw__when">
              {when(run.createdAt)}
              {current && <em>{t('next.routinesApp.this')}</em>}
            </span>
            <span className="hx-rtw__sum">{runSummary(run) || t(`next.routinesApp.state.${state}`)}</span>
          </span>
        </button>
      );
    })}
  </div>
);

const Next = ({ chat }: { chat: IRunChat }) => {
  const { routine } = chat;
  if (!routine.enabled) return <Quiet text={t('next.routinesApp.off')} />;
  if (routine.next.length === 0) return <Quiet text={t('next.routinesApp.noNext')} />;
  return (
    <div className="hx-wgr">
      {routine.next.map((at) => (
        <div key={at} className="hx-wgr__row">
          <Icon name="clock" size="sm" className="hx-rtw__ic" />
          <span className="hx-wgr__title">{when(at)}</span>
        </div>
      ))}
    </div>
  );
};

const Memory = ({ chat }: { chat: IRunChat }) => {
  const [all, setAll] = useState(false);
  const text = chat.routine.state?.trim() ?? '';
  if (text === '') return <Quiet text={t('next.routinesApp.empty.memory')} />;
  const long = text.length > 280 || text.split('\n').length > 6;
  return (
    <div className="hx-rtw__mem">
      <p className="hx-rtw__memtext" data-clamped={(long && !all) || undefined}>
        {text}
      </p>
      {long && (
        <button type="button" className="hx-rtw__more" onClick={() => setAll(!all)}>
          {t(all ? 'next.routinesApp.less' : 'next.routinesApp.more')}
        </button>
      )}
    </div>
  );
};

const BODY: Record<TSideView, (p: { chat: IRunChat }) => ReactNode> = {
  results: Results,
  sources: Sources,
  runs: Runs,
  next: Next,
  memory: Memory,
};

/** The app's widgets of a run's chat, in its manifest's order. */
export const RoutineWidgetsView = ({ chat }: { chat: IRunChat }) => {
  const [folded, setFolded] = useState(readFolded);
  const onFold = (id: string, fold: boolean) => {
    const next = new Set(folded);
    if (fold) next.add(id);
    else next.delete(id);
    setFolded(next);
    saveFolded(next);
  };
  return (
    <div className="hx-wgs hx-rtws">
      {SIDE_VIEWS.map((v) => {
        const Body = BODY[v.id];
        return (
          <Card
            key={v.id}
            id={v.id}
            title={t(`next.routinesApp.w.${v.id}`)}
            icon={v.icon || 'clock'}
            folded={folded.has(v.id)}
            onFold={onFold}
          >
            <Body chat={chat} />
          </Card>
        );
      })}
    </div>
  );
};

/** The app's widgets in a run's chat; nothing in any other chat. */
const RoutineWidgets = ({ machine, chat }: { machine: MachineId; chat: ConvId }) => {
  const got = useRunChat(machine, chat);
  return got.k === 'ready' ? <RoutineWidgetsView chat={got.v} /> : null;
};

export default RoutineWidgets;
