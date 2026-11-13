// What the search (⌘K) reads besides the chats: the catalog of the settings, the team's documents, what the connected
// systems find (merge requests, issues, pages), and the model's answer to a question in words
// (cmd/runtime/routes_search.go).
import { getPage, sendBody } from '@/next/core/net/http';
import { platformPath, runtimePath } from '@/next/core/net/machine';
import { answer, list, num, obj, opt, str } from '@/next/features/settings/platform-answer';
import type { MachineId } from '@/next/protocol/v2';

const TIMEOUT_MS = 10_000;
const CONNECTION_TIMEOUT_MS = 20_000;
const ANSWER_TIMEOUT_MS = 30_000;

const url = (machine: MachineId, path: `/${string}`) => runtimePath(platformPath(path), machine);

export const SETTING_KIND = { enum: 'enum', bool: 'bool', number: 'number' } as const;
export const SETTING_ON = 'on';
export const SETTING_OFF = 'off';

export interface ISettingOption {
  value: string;
  label: string;
}

/** A setting of the catalog (internal/settingsctl): what it is, what it takes and what it is now. */
export interface ICatalogSetting {
  key: string;
  section: string;
  title: string;
  description: string;
  kind: string;
  options: readonly ISettingOption[];
  value: string;
  valueLabel: string;
}

const readOption = (v: unknown): ISettingOption => {
  const o = obj(v, 'option');
  return { value: str(o.value, 'option.value'), label: str(o.label, 'option.label') };
};

const readSetting = (v: unknown): ICatalogSetting => {
  const o = obj(v, 'setting');
  return {
    key: str(o.key, 'key'),
    section: str(o.section, 'section'),
    title: str(o.title, 'title'),
    description: str(o.description, 'description'),
    kind: str(o.kind, 'kind'),
    options: list(o.options, 'options', readOption),
    value: str(o.value, 'value'),
    valueLabel: str(o.valueLabel, 'valueLabel'),
  };
};

export const loadSettingsCatalog = (machine: MachineId) =>
  getPage(
    url(machine, '/settings-catalog'),
    answer((o) => list(o.settings, 'settings', readSetting)),
    TIMEOUT_MS,
  );

/** A document of the team's catalog. */
export interface ICatalogDoc {
  slug: string;
  title: string;
  url: string;
  /** ISO time of the last change; '' when the catalog does not say. */
  changedAt: string;
}

const readDoc = (v: unknown): ICatalogDoc => {
  const o = obj(v, 'doc');
  const slug = str(o.slug, 'slug');
  return { slug, title: opt(o.title) || slug, url: str(o.url, 'url'), changedAt: opt(o.lastActivityAt) };
};

export const loadCatalogDocs = (machine: MachineId) =>
  getPage(
    url(machine, '/docs'),
    answer((o) => list(o.docs, 'docs', readDoc)),
    TIMEOUT_MS,
  );

export const FOUND_KIND = { mr: 'mr', issue: 'issue', page: 'page' } as const;
export type TFoundKind = (typeof FOUND_KIND)[keyof typeof FOUND_KIND];

/** One thing a connected system found. */
export interface IFound {
  kind: TFoundKind;
  title: string;
  url: string;
  /** Where it is: the project of a merge request, the key of an issue, the space of a page. */
  place: string;
  /** The state in the system's own word (merged, opened, Done); '' when it has none. */
  state: string;
}

const readMr = (v: unknown): IFound => {
  const o = obj(v, 'mr');
  num(o.iid, 'iid');
  return {
    kind: FOUND_KIND.mr,
    title: str(o.title, 'title'),
    url: str(o.url, 'url'),
    place: opt(o.project),
    state: opt(o.state),
  };
};

const readIssue = (v: unknown): IFound => {
  const o = obj(v, 'issue');
  const status = o.status === undefined || o.status === null ? null : obj(o.status, 'status');
  return {
    kind: FOUND_KIND.issue,
    title: str(o.title, 'title'),
    url: str(o.url, 'url'),
    place: opt(o.key),
    state: status === null ? '' : opt(status.name),
  };
};

const readPage = (v: unknown): IFound => {
  const o = obj(v, 'page');
  return {
    kind: FOUND_KIND.page,
    title: str(o.title, 'title'),
    url: str(o.url, 'url'),
    place: opt(o.space),
    state: '',
  };
};

const FOUND_PER_KIND = 5;

const READERS = {
  [FOUND_KIND.mr]: { path: 'mrs', read: answer((o) => list(o.mrs, 'mrs', readMr)) },
  [FOUND_KIND.issue]: { path: 'issues', read: answer((o) => list(o.issues, 'issues', readIssue)) },
  [FOUND_KIND.page]: { path: 'pages', read: answer((o) => list(o.pages, 'pages', readPage)) },
} as const;

/** What one connection finds of one kind for the words. */
export const searchConnection = (machine: MachineId, conn: string, kind: TFoundKind, query: string) =>
  getPage(
    url(
      machine,
      `/connections/${encodeURIComponent(conn)}/${READERS[kind].path}?q=${encodeURIComponent(query)}&limit=${FOUND_PER_KIND}`,
    ),
    READERS[kind].read,
    CONNECTION_TIMEOUT_MS,
  );

/** A found thing as the model is shown it. */
export interface IAnswerSource {
  kind: string;
  title: string;
  text: string;
}

export interface ISearchAnswer {
  text: string;
  /** The sources the answer rests on, by their place in what was sent. */
  cited: readonly number[];
}

/** The refusal of an answer when no model is picked for «Answers in the search». */
export const ANSWER_NO_MODEL = 'no-model';

export const askSearch = (machine: MachineId, query: string, sources: readonly IAnswerSource[]) =>
  sendBody(
    'POST',
    url(machine, '/search/answer'),
    { query, sources },
    answer((o): ISearchAnswer => ({
      text: str(o.text, 'text'),
      cited: list(o.cited, 'cited', (x) => num(x, 'cited')),
    })),
    ANSWER_TIMEOUT_MS,
  );
