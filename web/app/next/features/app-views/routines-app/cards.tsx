// The routines app's cards in a run's chat (views `start` and `result` of internal/apps/builtin/routines): at the head
// of the feed, the task, its schedule, how this run started and when, with «Все запуски» and «Выключить»; at its end,
// once the agent reported, how it went, its summary, what it made, where it read and when the next run is. The feed
// puts them there itself (chat-view: the lead and the end), from what hl.routines says of the chat: nobody posts them.
import { useState } from 'react';
import { pageClock } from '@/next/core/clock';
import {
  hostOf,
  isManual,
  isOver,
  runState,
  runSummary,
  scheduleText,
  whenText,
  type IRunChat,
  type TRunState,
} from '@/next/features/app-views/routines-app/model';
import { setRoutineEnabled, useRunChat } from '@/next/features/app-views/routines-app/store';
import type { IRoutineArtifact } from '@/next/features/app-views/routines';
import { locale, t } from '@/next/i18n';
import type { ConvId, MachineId } from '@/next/protocol/v2';
import { goTo, ROUTE } from '@/next/routes/routes';
import Icon from '@/next/ui/icon';
import { toast } from '@/next/ui/toast';
import '@/next/styles/routines-app.css';

export const STATE_ICON: Record<TRunState, string> = {
  ok: 'circle-check',
  quiet: 'circle-dashed',
  failed: 'circle-x',
  running: 'loader-circle',
  waiting: 'clock',
};

export const ART_ICON: Record<string, string> = {
  doc: 'file-text',
  file: 'file-text',
  chart: 'chart-column',
  mr: 'git-merge',
  board: 'layout-dashboard',
  link: 'link',
};

/** A moment in the person's words and language. */
export const when = (iso: string): string =>
  whenText(
    iso,
    new Date(pageClock().now()),
    {
      today: t('next.routinesApp.today'),
      tomorrow: t('next.routinesApp.tomorrow'),
      yesterday: t('next.routinesApp.yesterday'),
    },
    locale(),
  );

/** One thing a run made, opened in a new tab of the browser. */
export const ArtifactRow = ({ artifact, meta }: { artifact: IRoutineArtifact; meta?: string }) => (
  <a
    className="hx-file hx-rta-art"
    href={artifact.url || undefined}
    target="_blank"
    rel="noreferrer"
    data-open={artifact.url ? 'true' : undefined}
  >
    <span className="hx-file__ico" data-tone="accent">
      <Icon name={ART_ICON[artifact.kind] ?? 'file-text'} />
    </span>
    <span className="hx-file__text">
      <span className="hx-file__name">{artifact.title}</span>
      {(meta || artifact.meta) && (
        <span className="hx-file__meta">{[artifact.meta, meta].filter(Boolean).join(' · ')}</span>
      )}
    </span>
  </a>
);

export const StartCard = ({ machine, chat }: { machine: MachineId; chat: IRunChat }) => {
  const { routine, run } = chat;
  const [busy, setBusy] = useState(false);
  const toggle = async () => {
    setBusy(true);
    try {
      await setRoutineEnabled(machine, routine.id, !routine.enabled);
      toast(t(routine.enabled ? 'next.routinesApp.turnedOff' : 'next.routinesApp.turnedOn'));
    } catch (e) {
      toast(t('next.routinesApp.toggleFailed'), { description: e instanceof Error ? e.message : undefined });
    } finally {
      setBusy(false);
    }
  };
  const line = [
    scheduleText(routine),
    t(isManual(run) ? 'next.routinesApp.byHand' : 'next.routinesApp.bySchedule'),
    when(run.createdAt),
  ].filter(Boolean);
  return (
    <section className="hx-rta" data-view="start" aria-label={t('next.routinesApp.kind')}>
      <div className="hx-rta__top">
        <span className="hx-rta__ic">
          <Icon name="clock" size="sm" />
        </span>
        <span className="hx-rta__kind">{t('next.routinesApp.kind')}</span>
        {!routine.enabled && <span className="hx-rta__chip">{t('next.routinesApp.off')}</span>}
      </div>
      <h3 className="hx-rta__name">{routine.name}</h3>
      <p className="hx-rta__line">{line.join(' · ')}</p>
      <div className="hx-rta__acts">
        <button
          type="button"
          className="hx-btn"
          data-size="sm"
          onClick={() => goTo({ kind: ROUTE.routines, id: routine.id, run: run.id })}
        >
          <Icon name="history" size="sm" />
          {t('next.routinesApp.allRuns')}
        </button>
        <button
          type="button"
          className="hx-btn"
          data-size="sm"
          data-variant="ghost"
          data-busy={busy || undefined}
          disabled={busy}
          onClick={() => void toggle()}
        >
          <Icon name={routine.enabled ? 'pause' : 'play'} size="sm" />
          {t(routine.enabled ? 'next.routinesApp.turnOff' : 'next.routinesApp.turnOn')}
        </button>
      </div>
    </section>
  );
};

export const ResultCard = ({ chat }: { chat: IRunChat }) => {
  const { routine, run } = chat;
  const state = runState(run);
  const summary = runSummary(run);
  const artifacts = run.result?.artifacts ?? [];
  const sources = run.result?.sources ?? [];
  const next = routine.enabled ? routine.next[0] : undefined;
  return (
    <section
      className="hx-rta"
      data-view="result"
      data-state={state}
      aria-label={t(`next.routinesApp.state.${state}`)}
    >
      <div className="hx-rta__top">
        <span className="hx-rta__ic" data-state={state}>
          <Icon name={STATE_ICON[state]} size="sm" />
        </span>
        <span className="hx-rta__state">{t(`next.routinesApp.state.${state}`)}</span>
        <span className="hx-rta__at">{run.finishedAt ? when(run.finishedAt) : ''}</span>
      </div>
      {summary && <p className="hx-rta__sum">{summary}</p>}
      {artifacts.length > 0 && (
        <div className="hx-rta__arts">
          {artifacts.map((a, i) => (
            <ArtifactRow key={i} artifact={a} />
          ))}
        </div>
      )}
      {sources.length > 0 && (
        <div className="hx-rta__srcs" aria-label={t('next.routinesApp.sources')}>
          <span className="hx-rta__label">{t('next.routinesApp.sources')}</span>
          {sources.map((s, i) =>
            s.url ? (
              <a key={i} className="hx-rta__src" href={s.url} target="_blank" rel="noreferrer" title={s.url}>
                <Icon name="globe" size="sm" />
                <span>{s.title || hostOf(s.url)}</span>
              </a>
            ) : (
              <span key={i} className="hx-rta__src">
                <Icon name="globe" size="sm" />
                <span>{s.title}</span>
              </span>
            ),
          )}
        </div>
      )}
      <p className="hx-rta__next">
        <Icon name="clock" size="sm" />
        {next ? t('next.routinesApp.next', { when: when(next) }) : t('next.routinesApp.noNext')}
      </p>
    </section>
  );
};

/** The card at the head of a run's chat; nothing in any other chat. */
export const RoutineStartCard = ({ machine, chat }: { machine: MachineId; chat: ConvId }) => {
  const got = useRunChat(machine, chat);
  return got.k === 'ready' ? <StartCard machine={machine} chat={got.v} /> : null;
};

/** The card at the end of a run's chat once the run reported; nothing before, nothing in any other chat. */
export const RoutineResultCard = ({ machine, chat }: { machine: MachineId; chat: ConvId }) => {
  const got = useRunChat(machine, chat);
  return got.k === 'ready' && isOver(got.v.run) ? <ResultCard chat={got.v} /> : null;
};
