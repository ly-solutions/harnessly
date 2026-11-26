// «Макет» in the chat's tools panel on made-up data: ?dev=layoutpane, &part=tweaks|comments|pages|versions,
// &w=<pane width>. The panel, its strip and its tabs are the real ones; nothing is asked of a machine. Never part of a
// production path.
import '@/next/styles/order';

import { useState } from 'react';
import { pinLocale } from '@/next/i18n';
import { useRouter } from '@/platform/router';
import AppFrame from '@/next/features/shell/app-frame';
import PaneDrawer from '@/next/features/shell/pane-drawer';
import ToolPane, { TOOLS_TAB } from '@/next/features/pane/tool-pane';
import LayoutBody from '@/next/features/designly/panel/design-side';
import { LAYOUT_TAB, type TLayoutPart } from '@/next/features/designly/panel/layout-tab';
import OverviewTab from '@/next/features/designly/panel/overview-tab';
import SettingsTab from '@/next/features/designly/panel/settings-tab';
import CommentsTab from '@/next/features/designly/panel/comments-tab';
import PagesTab from '@/next/features/designly/panel/pages-tab';
import VersionsTab from '@/next/features/designly/panel/versions-tab';
import { readSiteComment, threadsOf, type ISiteComment } from '@/next/features/designly/panel/api';
import type { ITweak } from '@/next/features/designly/page-bridge';
import type { IProjectHistory } from '@/next/features/designly/versions/versions-api';
import PagePicker, { PICKER_PLACE } from '@/next/features/designly/page-picker';
import DeviceMenu from '@/next/features/designly/device-menu';
import { deviceOf } from '@/next/features/designly/devices';

const now = Date.now();
const HISTORY: IProjectHistory = {
  createdAt: Date.parse('2026-10-20T20:25:49Z'),
  points: [
    { id: 'p4', at: now - 600_000, who: 'автоснимок', agent: false, subject: 'Синк', auto: true },
    {
      id: 'p3',
      at: now - 3_600_000,
      who: 'anna · чат «Шапка» (anna · dev-anna)',
      agent: true,
      subject: 'Агент: шапка и отступы',
    },
    {
      id: 'p2',
      at: now - 86_400_000,
      who: 'petr@example.com · hub',
      agent: false,
      subject: 'Правка вручную',
    },
    {
      id: 'p1',
      at: now - 9 * 86_400_000,
      who: 'anna · dev-anna',
      agent: false,
      subject: 'Проект создан',
    },
  ],
  named: [],
  unsaved: false,
};

const opts = (...v: string[]) => v.map((value) => ({ value, text: value }));
const tweak = (over: Partial<ITweak>): ITweak => ({
  name: 'x',
  label: 'X',
  section: '',
  editor: 'enum',
  value: null,
  def: null,
  options: [],
  min: null,
  max: null,
  step: null,
  ...over,
});
const CONTROLS: ITweak[] = [
  tweak({
    name: 'who',
    label: 'Кто смотрит',
    value: 'Руководитель отдела продаж',
    def: 'Сотрудник',
    options: opts('Сотрудник', 'Руководитель отдела продаж', 'Администратор'),
  }),
  tweak({
    name: 'step',
    label: 'Экран',
    editor: 'steps',
    value: 'Настройки',
    def: 'Список',
    options: opts('Список', 'Настройки', 'Готово'),
  }),
  tweak({
    name: 'period',
    label: 'Период',
    value: '7 дней',
    def: '7 дней',
    options: opts('День', '7 дней', 'Месяц'),
  }),
  tweak({ name: 'empty', label: 'Пустое состояние', editor: 'boolean', value: true, def: false }),
  tweak({
    name: 'accent',
    label: 'Акцент',
    value: '#8b7cff',
    def: '#8b7cff',
    options: opts('#8b7cff', '#2f9e6e', '#e2774a'),
  }),
];

const comment = (over: Partial<ISiteComment>): ISiteComment => ({
  ...(readSiteComment({ id: 'c' }) as ISiteComment),
  ...over,
});
const THREADS = threadsOf([
  comment({
    id: 'a',
    board: 'a.dc.html',
    at: new Date(now - 7_200_000).toISOString(),
    by: { name: 'Анна', email: '' },
    text: 'Кнопку бы чуть левее, чтобы не спорила с заголовком',
  }),
  comment({
    id: 'r',
    parent: 'a',
    at: new Date(now - 3_600_000).toISOString(),
    by: { name: 'Пётр', email: '' },
    text: 'Согласен',
  }),
  comment({
    id: 'b',
    board: 'b.dc.html',
    at: new Date(now - 86_400_000).toISOString(),
    by: { name: 'Ксения', email: '' },
    text: 'А что будет, если задач нет вообще?',
    resolved: true,
  }),
]);

const MANY = Array.from({ length: 144 }, (_, i) => ({
  path: i === 40 ? 'a.dc.html' : `p${i}.dc.html`,
  name:
    i === 40
      ? 'Инициатива А · Настройки инициативы'
      : `${['Главная', 'Чат и Sinopia — без вкладок', 'Регулярные задачи — как в Codex', 'Ссылка — телефон/компьютер/аналитика'][i % 4]} ${i + 1}`,
}));
const PAGES = [
  { path: 'a.dc.html', name: 'Инициатива А · Настройки инициативы' },
  { path: 'b.dc.html', name: 'Регулярные задачи — как в Codex' },
  { path: 'c.dc.html', name: 'Главная' },
];

const LayoutPaneGallery = () => {
  const { query } = useRouter();
  pinLocale(query.lang === 'en' ? 'en' : 'ru');
  const [part, setPart] = useState<TLayoutPart>((query.part as TLayoutPart) || 'info');
  const [tab, setTab] = useState<string>(LAYOUT_TAB);
  if (query.size === '1')
    return (
      <div style={{ padding: 24 }}>
        <div style={{ position: 'relative', width: 300, marginLeft: 280, height: 8 }}>
          <DeviceMenu
            state="open"
            device={deviceOf('fill')}
            size={{ w: 1280, h: 720 }}
            zoom={{
              pct: 100,
              zoomed: false,
              canIn: true,
              canOut: true,
              zoomIn: () => undefined,
              zoomOut: () => undefined,
              fit: () => undefined,
              std: true,
              zoomTo: () => undefined,
              locked: false,
              onLock: () => undefined,
            }}
            onPick={() => undefined}
            onSize={() => undefined}
          />
        </div>
      </div>
    );
  if (query.picker === '1')
    return (
      <div style={{ padding: 24, containerType: 'inline-size' }}>
        <div style={{ position: 'relative', width: Number(query.w) || 360, height: 8 }}>
          <PagePicker
            pages={MANY}
            path="a.dc.html"
            state="open"
            place={PICKER_PLACE.start}
            boards={{ changed: new Set(['p3.dc.html', 'p7.dc.html']), onNew: () => undefined }}
            onGo={() => undefined}
            onClose={() => undefined}
            onSave={() => undefined}
            saveLabel={() => 'Скачать'}
          />
        </div>
      </div>
    );
  const render = (at: TLayoutPart) => {
    if (at === 'info')
      return (
        <OverviewTab
          title="harnessly"
          pages={144}
          current="Инициатива А · Настройки инициативы"
          history={HISTORY}
          owner="Анна"
          kit={query.kit === '0' ? null : { name: 'Acme DS 2.4', href: '#kit' }}
          chats={[]}
          people={[
            { name: 'Анна', role: 'Владелец' },
            { name: 'Пётр', role: 'Может редактировать' },
          ]}
          access="Вся организация может редактировать"
          onShare={() => undefined}
          links={[{ key: 'own', name: 'Адрес макета', meta: 'по доступу', url: 'https://example.com/d/1' }]}
        />
      );
    if (at === 'tweaks')
      return <SettingsTab controls={CONTROLS} onChange={() => undefined} onReset={() => undefined} />;
    if (at === 'comments')
      return (
        <CommentsTab
          threads={THREADS}
          boardName={(p) => p}
          current="a.dc.html"
          onOpen={() => undefined}
          onStart={() => undefined}
        />
      );
    if (at === 'pages')
      return (
        <PagesTab
          pages={query.many === '1' ? MANY : PAGES}
          hrefOf={(p) => `#${p}`}
          actions={
            query.view === '1'
              ? undefined
              : {
                  create: async () => undefined,
                  duplicate: async () => undefined,
                  rename: async () => undefined,
                  remove: async () => undefined,
                  failed: () => undefined,
                }
          }
          current="a.dc.html"
          changed={new Set(['b.dc.html', 'p3.dc.html', 'p7.dc.html'])}
          onPick={() => undefined}
        />
      );
    return (
      <VersionsTab
        machine={'gallery' as never}
        id="p"
        origin={null}
        page={null}
        mayEdit
        owner="Анна"
        history={HISTORY}
        error={null}
        reload={() => undefined}
        onRestored={() => undefined}
      />
    );
  };
  return (
    <AppFrame
      rail={false}
      drawerOpen={false}
      onDrawerChange={() => undefined}
      sidebar={<nav className="hx-sidebar" aria-label="chats" />}
      aside={
        <PaneDrawer
          open
          onClose={() => undefined}
          wide={false}
          width={Number(query.w) || 440}
          onWidth={() => undefined}
        >
          <ToolPane
            tabs={[{ id: LAYOUT_TAB, icon: 'presentation', name: 'Макет', pinned: true }]}
            tab={tab}
            maxed={false}
            onPick={setTab}
            onCloseTab={() => undefined}
            onToggleMax={() => undefined}
            onClose={() => undefined}
          >
            {tab === LAYOUT_TAB && (
              <div className="hx-ds-pane">
                <LayoutBody part={part} onPart={setPart} badges={{ comments: 1 }} render={render} />
              </div>
            )}
            {tab === TOOLS_TAB && <p>Инструменты</p>}
          </ToolPane>
        </PaneDrawer>
      }
    >
      <header className="hx-header" />
    </AppFrame>
  );
};

export default LayoutPaneGallery;
