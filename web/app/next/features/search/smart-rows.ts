// The search's rows beyond the chats: settings (changed in the row itself), the sections of Settings, the team's
// documents, the designs of the library and what the connected systems found. Each builder answers a group, or null
// when it has nothing for the words.
import { matching, type ITerms } from '@/next/features/search/terms';
import {
  ROW_GO,
  ROW_WIDGET,
  type ISearchGroup,
  type ISearchRow,
  type ISearchWidget,
} from '@/next/features/search/search-dialog';
import {
  FOUND_KIND,
  SETTING_KIND,
  SETTING_ON,
  type ICatalogDoc,
  type ICatalogSetting,
  type IFound,
  type TFoundKind,
} from '@/next/features/search/sources-api';
import { t } from '@/next/i18n';
import type { DesignArtifact } from '@/next/protocol/v2';
import type { SettingsSection } from '@/next/routes/routes';

export const SCOPE = {
  all: 'all',
  chats: 'chats',
  docs: 'docs',
  designs: 'designs',
  apps: 'apps',
  settings: 'settings',
} as const;

/** The scope of a connection's results. */
export const connScope = (id: string): string => `conn:${id}`;

const ROWS_MAX = 5;

/** The section of Settings a setting of the catalog is changed in. */
export const sectionOfSetting = (key: string): SettingsSection => {
  if (key.startsWith('background.')) return 'smart';
  if (key === 'providers') return 'accounts';
  return 'general';
};

const widgetOf = (setting: ICatalogSetting): ISearchWidget | undefined => {
  if (setting.kind === SETTING_KIND.bool)
    return { kind: ROW_WIDGET.toggle, on: setting.value === SETTING_ON };
  if (setting.kind === SETTING_KIND.enum)
    return {
      kind: ROW_WIDGET.pick,
      value: setting.value,
      label: setting.valueLabel,
      options: setting.options,
    };
  return undefined;
};

const settingRow = (setting: ICatalogSetting): ISearchRow => ({
  id: `setting:${setting.key}`,
  kind: 'setting',
  icon: 'sliders-horizontal',
  title: setting.title,
  snippet: `${setting.section} · ${setting.description}`,
  // A number has no control in the row: its value is said, and the row opens it in Settings.
  meta: setting.kind === SETTING_KIND.number ? setting.valueLabel : '',
  widget: widgetOf(setting),
  go: { k: ROW_GO.setting, key: setting.key, section: sectionOfSetting(setting.key) },
  preview: { kind: t('next.search.kindSetting'), text: setting.description },
});

/** The sections of Settings the search leads to, with the words each is found by. */
const SECTIONS: readonly SettingsSection[] = [
  'general',
  'appearance',
  'keys',
  'notifications',
  'voice',
  'accounts',
  'smart',
  'claude',
  'codex',
  'skills',
  'safe',
  'browser',
  'machines',
  'apps',
  'connections',
];

const sectionRow = (section: SettingsSection): ISearchRow => ({
  id: `section:${section}`,
  kind: 'setting',
  icon: 'settings',
  title: t(`next.settings.section.${section}`),
  snippet: t('next.search.sectionOf'),
  meta: '',
  go: { k: ROW_GO.section, section },
  preview: { kind: t('next.search.kindSection'), text: t('next.search.sectionHint') },
});

export const settingsGroup = (
  catalog: readonly ICatalogSetting[] | null,
  terms: ITerms,
): ISearchGroup | null => {
  if (terms.list.length === 0) return null;
  const settings = matching(catalog ?? [], terms, (s) => [s.title, s.section, s.description]).map(settingRow);
  const sections = matching(SECTIONS, terms, (section) => [t(`next.settings.section.${section}`)]).map(
    sectionRow,
  );
  const rows = [...settings, ...sections].slice(0, ROWS_MAX + 1);
  return rows.length > 0 ? { id: SCOPE.settings, label: t('next.search.groupSettings'), rows } : null;
};

/** «12 нояб.» for a time the catalog gave; '' for none. */
const dayOf = (at: number | null): string =>
  at === null || Number.isNaN(at)
    ? ''
    : new Date(at).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });

const docRow = (doc: ICatalogDoc): ISearchRow => ({
  id: `doc:${doc.slug}`,
  kind: 'doc',
  icon: 'file-text',
  title: doc.title,
  meta: dayOf(doc.changedAt === '' ? null : Date.parse(doc.changedAt)),
  go: { k: ROW_GO.link, url: doc.url },
  preview: { kind: t('next.search.kindDoc'), text: t('next.search.docHint') },
});

const newestDocs = (docs: readonly ICatalogDoc[]): ICatalogDoc[] =>
  [...docs].sort((a, b) => b.changedAt.localeCompare(a.changedAt));

export const docsGroup = (docs: readonly ICatalogDoc[] | null, terms: ITerms): ISearchGroup | null => {
  if (terms.list.length === 0) return null;
  const rows = matching(newestDocs(docs ?? []), terms, (doc) => [doc.title, doc.slug])
    .slice(0, ROWS_MAX)
    .map(docRow);
  return rows.length > 0 ? { id: SCOPE.docs, label: t('next.search.groupDocs'), rows } : null;
};

const designRow = (design: DesignArtifact, board: string | null): ISearchRow => ({
  id: board === null ? `design:${design.id}` : `design:${design.id}:${board}`,
  kind: 'board',
  icon: 'pen-tool',
  title: board ?? design.title,
  snippet: board === null ? undefined : design.title,
  meta: dayOf(design.updatedAt),
  go: { k: ROW_GO.design, id: design.id },
  preview: {
    kind: t('next.search.kindDesign'),
    text: board === null ? t('next.search.designHint') : t('next.search.boardHint', { design: design.title }),
  },
});

const newestDesigns = (designs: readonly DesignArtifact[]): DesignArtifact[] =>
  [...designs].sort((a, b) => b.updatedAt - a.updatedAt);

/** The designs whose name has the words, then the boards that have them (a board says which design it is of). */
export const designsGroup = (
  designs: readonly DesignArtifact[] | null,
  terms: ITerms,
): ISearchGroup | null => {
  if (terms.list.length === 0) return null;
  const sorted = newestDesigns(designs ?? []);
  const byName = matching(sorted, terms, (design) => [design.title]).map((design) => designRow(design, null));
  const boards = sorted.flatMap((design) => design.boards.map((board) => ({ design, title: board.title })));
  const byBoard = matching(boards, terms, (board) => [board.title]).map((board) =>
    designRow(board.design, board.title),
  );
  const rows = [...byName, ...byBoard].slice(0, ROWS_MAX);
  return rows.length > 0 ? { id: SCOPE.designs, label: t('next.search.groupDesigns'), rows } : null;
};

const FOUND_ICON: Record<TFoundKind, string> = {
  [FOUND_KIND.mr]: 'git-pull-request',
  [FOUND_KIND.issue]: 'circle-dot',
  [FOUND_KIND.page]: 'book-open',
};

const foundRow = (conn: string, found: IFound): ISearchRow => ({
  id: `found:${conn}:${found.url}`,
  kind: 'found',
  icon: FOUND_ICON[found.kind],
  title: found.title,
  snippet: [found.place, t(`next.search.found.${found.kind}`), found.state].filter(Boolean).join(' · '),
  meta: '',
  go:
    found.kind === FOUND_KIND.mr ? { k: ROW_GO.review, url: found.url } : { k: ROW_GO.link, url: found.url },
  preview: { kind: t(`next.search.found.${found.kind}`), text: t('next.search.foundHint') },
});

/** What one connected system found, under its name. */
export const foundGroup = (
  conn: { id: string; name: string },
  found: readonly IFound[],
): ISearchGroup | null =>
  found.length > 0
    ? { id: connScope(conn.id), label: conn.name, rows: found.map((item) => foundRow(conn.id, item)) }
    : null;

export interface IIdleInput {
  chats: readonly ISearchRow[];
  designs: readonly DesignArtifact[] | null;
  docs: readonly ICatalogDoc[] | null;
}

const IDLE_CHATS = 4;
const IDLE_OTHERS = 2;

/** Before anything is typed: what to go on with, and what the search can start. */
export const idleGroups = ({ chats, designs, docs }: IIdleInput): readonly ISearchGroup[] => {
  const goOn = [
    ...chats.slice(0, IDLE_CHATS),
    ...newestDesigns(designs ?? [])
      .slice(0, IDLE_OTHERS)
      .map((design) => designRow(design, null)),
    ...newestDocs(docs ?? [])
      .slice(0, IDLE_OTHERS)
      .map(docRow),
  ];
  const actions: ISearchRow[] = [
    {
      id: 'act:new-chat',
      kind: 'action',
      icon: 'plus',
      title: t('next.search.actNewChat'),
      snippet: t('next.search.actNewChatHint'),
      meta: '',
      go: { k: ROW_GO.newChat },
      preview: { kind: t('next.search.kindAction'), text: t('next.search.actNewChatText') },
    },
    {
      id: 'act:designs',
      kind: 'action',
      icon: 'pen-tool',
      title: t('next.search.actDesigns'),
      meta: '',
      go: { k: ROW_GO.designs },
      preview: { kind: t('next.search.kindAction'), text: t('next.search.actDesignsText') },
    },
    {
      id: 'act:settings',
      kind: 'action',
      icon: 'settings',
      title: t('next.search.actSettings'),
      meta: '',
      go: { k: ROW_GO.section, section: null },
      preview: { kind: t('next.search.kindAction'), text: t('next.search.actSettingsText') },
    },
  ];
  return [
    ...(goOn.length > 0 ? [{ id: 'go-on', label: t('next.search.groupGoOn'), rows: goOn }] : []),
    { id: 'actions', label: t('next.search.groupActions'), rows: actions },
  ];
};

/** The groups a scope keeps: all of them for «Всё», else the one of that scope (the chats' scope keeps the messages). */
export const inScope = (groups: readonly ISearchGroup[], scope: string): readonly ISearchGroup[] => {
  if (scope === SCOPE.all) return groups;
  if (scope === SCOPE.chats)
    return groups.filter((group) => group.id === SCOPE.chats || group.id.startsWith('messages:'));
  return groups.filter((group) => group.id === scope);
};
