// The routines app in a run's chat on made-up data, for the eye and for screenshots: ?dev=routinesapp.
// The start card at the head of the feed, the agent's turn, the result card at its end, and the app's widgets beside.
// ?state=ok|quiet|failed|running the run; ?off=1 the routine turned off; ?width=<px> the column's width (280 by
// default); ?theme=light|dark&lang=en|ru the look. Never part of a production path.
import '@/next/styles/order';

import { useEffect, useRef } from 'react';
import { PortalContext } from '@/next/ui/portal';
import { useRouter } from '@/platform/router';
import { pinLocale } from '@/next/i18n';
import { ResultCard, StartCard } from '@/next/features/app-views/routines-app/cards';
import { RoutineWidgetsView } from '@/next/features/app-views/routines-app/widgets';
import type { IRunChat } from '@/next/features/app-views/routines-app/model';
import type { IRoutineRun } from '@/next/features/app-views/routines';
import type { MachineId } from '@/next/protocol/v2';

const DAY = 86_400_000;
const at = (daysAgo: number, h: number, m = 0): string => {
  const d = new Date(Date.now() - daysAgo * DAY);
  d.setHours(h, m, 0, 0);
  return d.toISOString();
};

const runs = (state: string): IRoutineRun[] => {
  const now: IRoutineRun = {
    id: 'run-5',
    ruleId: 'r1',
    fingerprint: 'schedule:5',
    status: state === 'running' ? 'running' : state === 'failed' ? 'failed' : 'done',
    ...(state === 'running' ? {} : { outcome: state as 'ok' | 'quiet' | 'failed' }),
    conversationId: 'tab-run5',
    createdAt: at(0, 9),
    ...(state === 'running' ? {} : { finishedAt: at(0, 9, 7) }),
    result:
      state === 'ok'
        ? {
            summary:
              'За сутки 3 новые статьи по агентам с памятью и один релиз LangGraph 0.4: главное — долговременная память через граф состояний.',
            artifacts: [
              {
                kind: 'doc',
                title: 'Дайджест агентов — 26 ноября',
                url: 'https://docs.example.com/d/agents-26',
                meta: 'версия 12',
              },
              {
                kind: 'file',
                title: 'digest/2026-11-26.md',
                url: 'https://git.example.com/digest/2026-11-26.md',
              },
            ],
            sources: [
              { kind: 'link', title: 'arXiv cs.AI', url: 'https://arxiv.org/list/cs.AI/recent' },
              {
                kind: 'link',
                title: 'LangGraph releases',
                url: 'https://github.com/langchain-ai/langgraph/releases',
              },
              { kind: 'link', title: 'Hacker News', url: 'https://news.ycombinator.com' },
            ],
          }
        : state === 'quiet'
          ? {
              summary: 'Нового нет: новые статьи повторяют вчерашние.',
              sources: [{ kind: 'link', title: 'arXiv cs.AI', url: 'https://arxiv.org/list/cs.AI/recent' }],
            }
          : state === 'failed'
            ? { error: 'Проверка «fresh»: в итоге нет сегодняшней даты.' }
            : undefined,
  };
  return [
    now,
    {
      id: 'run-4',
      ruleId: 'r1',
      fingerprint: 'manual:4',
      status: 'done',
      outcome: 'ok',
      conversationId: 'tab-run4',
      createdAt: at(1, 15, 20),
      finishedAt: at(1, 15, 26),
      result: {
        summary: 'Вышел Claude Agent SDK 0.9: субагенты и память в одном API.',
        manual: true,
        artifacts: [
          {
            kind: 'doc',
            title: 'Дайджест агентов — 25 ноября',
            url: 'https://docs.example.com/d/agents-25',
            meta: 'версия 11',
          },
        ],
      },
    },
    {
      id: 'run-3',
      ruleId: 'r1',
      fingerprint: 'schedule:3',
      status: 'done',
      outcome: 'quiet',
      conversationId: 'tab-run3',
      createdAt: at(1, 9),
      finishedAt: at(1, 9, 4),
      result: { summary: 'Нового нет.' },
    },
    {
      id: 'run-2',
      ruleId: 'r1',
      fingerprint: 'schedule:2',
      status: 'failed',
      outcome: 'failed',
      conversationId: 'tab-run2',
      createdAt: at(2, 9),
      result: { error: 'Хаб не дождался отчёта за 2 часа.' },
    },
    {
      id: 'run-1',
      ruleId: 'r1',
      fingerprint: 'schedule:1',
      status: 'done',
      outcome: 'ok',
      conversationId: 'tab-run1',
      createdAt: at(3, 9),
      finishedAt: at(3, 9, 9),
      result: {
        summary: 'Первый дайджест: 7 статей, 2 релиза.',
        artifacts: [
          {
            kind: 'mr',
            title: 'MR !412 — подписка на arXiv',
            url: 'https://git.example.com/mr/412',
            meta: 'черновик',
          },
        ],
      },
    },
  ];
};

const chatOf = (state: string, off: boolean): IRunChat => {
  const list = runs(state);
  return {
    routine: {
      id: 'r1',
      name: 'Дайджест: агенты и память',
      enabled: !off,
      model: 'sonnet',
      trigger: { schedule: { cron: '0 9 * * 1-5', tz: 'Europe/Moscow' } },
      routine: { words: 'по будням в 9:00' },
      next: off ? [] : [at(-1, 9), at(-2, 9), at(-5, 9)],
      state:
        'Уже сообщил: LangGraph 0.4, Agent SDK 0.9, статьи 2411.0123 и 2411.0456.\nДокумент дайджеста: https://docs.example.com/d/agents (версия 12).\nЧеловек просил не повторять обзоры HN без новых цифр.',
      stats: { runs: 5, ok: 3, quiet: 1, failed: 1 },
    },
    run: list[0],
    runs: list,
  };
};

const MACHINE = 'local' as MachineId;

const RoutinesAppGallery = () => {
  const { query } = useRouter();
  const theme = query.theme === 'dark' ? 'dark' : 'light';
  pinLocale(query.lang === 'en' ? 'en' : 'ru');
  const state = typeof query.state === 'string' ? query.state : 'ok';
  const off = query.off === '1';
  const width = Number(query.width) || 280;
  const portalRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const root = document.documentElement;
    root.dataset.theme = theme;
    root.classList.toggle('dark', theme === 'dark');
    root.style.colorScheme = theme;
  }, [theme]);
  const chat = chatOf(state, off);
  return (
    <PortalContext value={portalRef}>
      <div
        style={{
          minHeight: '100vh',
          background: 'var(--hx-bg-app)',
          color: 'var(--hx-fg-1)',
          display: 'flex',
          flexWrap: 'wrap',
          gap: 24,
          padding: 16,
        }}
      >
        <div
          style={{
            flex: '1 1 280px',
            minWidth: 0,
            maxWidth: 720,
            display: 'flex',
            flexDirection: 'column',
            gap: 12,
          }}
        >
          <div className="hx-feed__lead">
            <StartCard machine={MACHINE} chat={chat} />
          </div>
          <p
            style={{
              margin: 0,
              padding: '10px 14px',
              borderRadius: 18,
              background: 'var(--hx-bg-bubble)',
              maxWidth: 560,
            }}
          >
            Это регулярная задача «Дайджест: агенты и память», запуск по расписанию…
          </p>
          <p style={{ margin: 0, maxWidth: 560, lineHeight: 1.6 }}>
            Собрал свежие статьи и релизы, сверил с памятью, записал дайджест новой версией документа и
            отчитался.
          </p>
          {chat.run.status !== 'running' && <ResultCard chat={chat} />}
        </div>
        <div style={{ width, flex: 'none' }}>
          <div className="hx-wgs-wrap" data-shown="true">
            <RoutineWidgetsView chat={chat} />
          </div>
        </div>
      </div>
      <div ref={portalRef} />
    </PortalContext>
  );
};

export default RoutinesAppGallery;
