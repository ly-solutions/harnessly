// The search of everything (⌘K) as the shell hands it to the dialog: the chats and their messages, the
// settings (changed in the row), the designs, the team's documents, the apps and what the connected systems find, in
// one list; and, for a question in words, the answer of the model picked for «Ответы в поиске» with its sources.
// The sources are read when the dialog opens, so a page that never searches asks for nothing.
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useMessageSearch } from '@/next/bindings/use-message-search';
import { CONN_STATE, loadConnections, type IConn } from '@/next/features/connections/api';
import { fetchLibrary } from '@/next/features/designly/api';
import { messageGroups, recentChats, searchChats } from '@/next/features/search/search-chats';
import {
  ANSWER,
  type ISearchGroup,
  type ISearchScope,
  type TSearchAnswer,
} from '@/next/features/search/search-dialog';
import {
  SCOPE,
  connScope,
  designsGroup,
  docsGroup,
  foundGroup,
  idleGroups,
  inScope,
  settingsGroup,
} from '@/next/features/search/smart-rows';
import {
  ANSWER_NO_MODEL,
  FOUND_KIND,
  askSearch,
  loadCatalogDocs,
  loadSettingsCatalog,
  searchConnection,
  type IAnswerSource,
  type ICatalogDoc,
  type ICatalogSetting,
  type IFound,
  type TFoundKind,
} from '@/next/features/search/sources-api';
import { isQuestion, termsOf } from '@/next/features/search/terms';
import { putSetting } from '@/next/features/settings/changes-api';
import { isRefused } from '@/next/features/settings/platform-answer';
import type { ISidebarChat, ISidebarProject } from '@/next/features/sidebar/types';
import { t } from '@/next/i18n';
import type { DesignArtifact, MachineId, MessageHit } from '@/next/protocol/v2';
import { toast } from '@/next/ui/toast';

/** A connected system is asked this long after the last key, from this many letters. */
const FOUND_DEBOUNCE_MS = 400;
const FOUND_FROM = 3;
/** The model is asked this long after the question stopped changing. */
const ANSWER_DEBOUNCE_MS = 700;
/** How many of the found rows the model is shown. */
const ANSWER_SOURCES = 10;
/** A question's keywords the messages are looked for by, each on its own. */
const ASK_KEYWORDS = 3;

const CAPABILITY_KIND: Readonly<Record<string, TFoundKind>> = {
  code: FOUND_KIND.mr,
  issues: FOUND_KIND.issue,
  pages: FOUND_KIND.page,
};

interface ISources {
  machine: MachineId;
  catalog: readonly ICatalogSetting[];
  docs: readonly ICatalogDoc[];
  designs: readonly DesignArtifact[];
  conns: readonly IConn[];
}

/** A source that did not come has nothing to find: the search goes on with the others. */
const valueOr = <T>(res: { ok: true; value: T | { refused: string } } | object, none: T): T =>
  'ok' in res && !isRefused(res.value) ? (res.value as T) : none;

const readSources = async (machine: MachineId): Promise<ISources> => {
  const [catalog, docs, designs, conns] = await Promise.all([
    loadSettingsCatalog(machine),
    loadCatalogDocs(machine),
    fetchLibrary(machine).then(
      (library) => library.designs,
      () => [],
    ),
    loadConnections(machine),
  ]);
  return {
    machine,
    catalog: valueOr<readonly ICatalogSetting[]>(catalog, []),
    docs: valueOr<readonly ICatalogDoc[]>(docs, []),
    designs,
    conns: valueOr<readonly IConn[]>(conns, []).filter((conn) => conn.state === CONN_STATE.ok),
  };
};

interface IFoundState {
  key: string;
  byConn: Readonly<Record<string, readonly IFound[]>>;
}

interface IAnswerState {
  key: string;
  answer: TSearchAnswer;
}

export interface ISmartSearchInput {
  machine: MachineId | null;
  open: boolean;
  query: string;
  scope: string;
  /** null while the chat list is on its way. */
  chats: readonly ISidebarChat[] | null;
  projects: readonly ISidebarProject[];
  /** The apps' rows for the words (features/search/app-rows.ts). */
  appsGroup: ISearchGroup | null;
  now: number;
}

export interface ISmartSearch {
  scopes: readonly ISearchScope[];
  /** null while the answer is on its way. */
  results: readonly ISearchGroup[] | null;
  idle: readonly ISearchGroup[];
  indexing: boolean;
  /** The words lit in the rows. */
  marks: string;
  answer: TSearchAnswer | null;
  onAsk: () => void;
  onSetting: (key: string, value: string) => Promise<boolean>;
}

const mergeHits = (lists: readonly (readonly MessageHit[])[], keywords: readonly string[]): MessageHit[] => {
  const seen = new Map<string, MessageHit>();
  for (const hit of lists.flat()) seen.set(`${hit.convId}:${hit.entryId}`, hit);
  const score = (hit: MessageHit) => {
    const text = hit.snippet.toLowerCase();
    return keywords.filter((word) => text.includes(word)).length;
  };
  return [...seen.values()].sort((a, b) => score(b) - score(a) || b.at - a.at);
};

export const useSmartSearch = ({
  machine,
  open,
  query,
  scope,
  chats,
  projects,
  appsGroup,
  now,
}: ISmartSearchInput): ISmartSearch => {
  const [sources, setSources] = useState<ISources | null>(null);
  /** The words the person asked the model about by hand. */
  const [asked, setAsked] = useState<string | null>(null);
  const [found, setFound] = useState<IFoundState | null>(null);
  const [answered, setAnswered] = useState<IAnswerState | null>(null);

  useEffect(() => {
    if (!open || machine === null) return;
    let stale = false;
    void readSources(machine).then((read) => {
      if (!stale) setSources(read);
    });
    return () => {
      stale = true;
    };
  }, [open, machine]);

  const mine = sources !== null && sources.machine === machine ? sources : null;
  const words = query.trim();
  const ask = words !== '' && (isQuestion(words) || asked === words);
  const terms = useMemo(() => termsOf(words, ask), [words, ask]);
  const keywords = ask ? terms.list : [];

  // A plain query asks for the messages that hold every word; a question asks for each of its keywords.
  const plain = useMessageSearch(machine, words, open && !ask);
  const first = useMessageSearch(machine, keywords[0] ?? '', open && ask && keywords.length > 0);
  const second = useMessageSearch(machine, keywords[1] ?? '', open && ask && keywords.length > 1);
  const third = useMessageSearch(machine, keywords[2] ?? '', open && ask && keywords.length > 2);
  const byKeyword = [first, second, third].slice(0, Math.min(keywords.length, ASK_KEYWORDS));
  const messagesAsked = ask ? byKeyword.some((m) => m.asked) : plain.asked;
  const messagesPending = ask
    ? byKeyword.some((m) => m.asked && m.hits === null)
    : plain.asked && plain.hits === null;
  const firstHits = first.hits;
  const secondHits = second.hits;
  const thirdHits = third.hits;
  const hits = useMemo((): readonly MessageHit[] | null => {
    if (!ask) return plain.hits;
    const lists = [firstHits, secondHits, thirdHits].filter((list) => list !== null);
    return lists.length === 0 ? null : mergeHits(lists, terms.list);
  }, [ask, plain.hits, firstHits, secondHits, thirdHits, terms]);

  // The connected systems: asked for the words (a question's first keyword), each answer shown as it comes.
  const conns = mine?.conns;
  const foundText = ask ? (keywords[0] ?? '') : words;
  const foundKey = `${machine}|${foundText}`;
  useEffect(() => {
    if (!open || machine === null || !conns || [...foundText].length < FOUND_FROM) return;
    let stale = false;
    const timer = setTimeout(() => {
      setFound({ key: foundKey, byConn: {} });
      for (const conn of conns) {
        const kinds = conn.capabilities.flatMap((capability) => CAPABILITY_KIND[capability] ?? []);
        void Promise.all(kinds.map((kind) => searchConnection(machine, conn.id, kind, foundText))).then(
          (answers) => {
            if (stale) return;
            const items = answers.flatMap((res) => valueOr<readonly IFound[]>(res, []));
            setFound((current) =>
              current?.key === foundKey
                ? { key: foundKey, byConn: { ...current.byConn, [conn.id]: items } }
                : current,
            );
          },
        );
      }
    }, FOUND_DEBOUNCE_MS);
    return () => {
      stale = true;
      clearTimeout(timer);
    };
  }, [open, machine, conns, foundText, foundKey]);

  const groups = useMemo((): readonly ISearchGroup[] | null => {
    if (chats === null) return null;
    const foundNow = found?.key === foundKey ? found.byConn : {};
    const all = [
      settingsGroup(mine?.catalog ?? null, terms),
      ...searchChats(chats, projects, words, terms),
      ...(hits ? messageGroups(hits, chats, now) : []),
      designsGroup(mine?.designs ?? null, terms),
      docsGroup(mine?.docs ?? null, terms),
      appsGroup,
      ...(mine?.conns ?? []).map((conn) => foundGroup(conn, foundNow[conn.id] ?? [])),
    ].filter((group) => group !== null);
    return all;
  }, [chats, projects, words, terms, hits, now, mine, appsGroup, found, foundKey]);

  const results = useMemo(() => {
    if (groups === null) return null;
    const shown = inScope(groups, scope);
    // The messages are still being asked and nothing else answers yet: «searching», not «nothing».
    if (messagesAsked && messagesPending && shown.length === 0) return null;
    return shown;
  }, [groups, scope, messagesAsked, messagesPending]);

  // The answer: asked once the question stops changing and what was found is here.
  const answerKey = `${machine}|${words}`;
  const ready = ask && groups !== null && !messagesPending;
  useEffect(() => {
    if (!open || machine === null || !ready || groups === null) return;
    const picked = groups
      .flatMap((group) =>
        group.rows.map((row) => ({ row, label: row.kind === 'message' ? group.label : row.title })),
      )
      .slice(0, ANSWER_SOURCES);
    if (picked.length === 0) return;
    let stale = false;
    const timer = setTimeout(() => {
      const sent: IAnswerSource[] = picked.map(({ row, label }) => ({
        kind: row.preview?.kind ?? t('next.search.kindChat'),
        title: label,
        text: row.snippet ?? row.preview?.text ?? '',
      }));
      void askSearch(machine, words, sent).then((res) => {
        if (stale) return;
        if (!('ok' in res)) {
          setAnswered({
            key: answerKey,
            answer: { k: ANSWER.failed, message: t('next.search.answerFailed') },
          });
          return;
        }
        const value = res.value;
        if (isRefused(value)) {
          setAnswered({
            key: answerKey,
            answer:
              value.refused === ANSWER_NO_MODEL
                ? { k: ANSWER.noModel }
                : { k: ANSWER.failed, message: value.message || t('next.search.answerFailed') },
          });
          return;
        }
        setAnswered({
          key: answerKey,
          answer: {
            k: ANSWER.ready,
            text: value.text,
            sources: value.cited.flatMap((index) => picked[index] ?? []),
          },
        });
      });
    }, ANSWER_DEBOUNCE_MS);
    return () => {
      stale = true;
      clearTimeout(timer);
    };
    // The rows the model is shown are those found when the question settled: what a connected system adds later does
    // not ask again.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, machine, ready, answerKey]);

  const hasRows = groups !== null && groups.some((group) => group.rows.length > 0);
  const answer = useMemo((): TSearchAnswer | null => {
    if (!ask) return null;
    if (answered?.key === answerKey) return answered.answer;
    // Nothing found and nothing more on its way: there is nothing to answer from.
    if (ready && !hasRows) return null;
    return { k: ANSWER.wait };
  }, [ask, answered, answerKey, ready, hasRows]);

  const scopes = useMemo(
    (): readonly ISearchScope[] => [
      { id: SCOPE.all, label: t('next.search.scopeAll') },
      { id: SCOPE.chats, label: t('next.search.groupChats') },
      { id: SCOPE.designs, label: t('next.search.groupDesigns') },
      { id: SCOPE.docs, label: t('next.search.groupDocs') },
      { id: SCOPE.apps, label: t('next.search.groupApps') },
      { id: SCOPE.settings, label: t('next.search.groupSettings') },
      ...(mine?.conns ?? [])
        .filter((conn) => conn.capabilities.some((capability) => capability in CAPABILITY_KIND))
        .map((conn) => ({ id: connScope(conn.id), label: conn.name })),
    ],
    [mine],
  );

  const idle = useMemo(
    () =>
      idleGroups({
        chats: chats === null ? [] : recentChats(chats, projects),
        designs: mine?.designs ?? null,
        docs: mine?.docs ?? null,
      }),
    [chats, projects, mine],
  );

  const onAsk = useCallback(() => setAsked(words), [words]);

  const onSetting = useCallback(
    async (key: string, value: string): Promise<boolean> => {
      if (machine === null) return false;
      const res = await putSetting(machine, key, value);
      if (!('ok' in res) || isRefused(res.value)) {
        const message = 'ok' in res && isRefused(res.value) ? res.value.message : '';
        toast(t('next.search.settingFailed'), { tone: 'error', description: message || undefined });
        return false;
      }
      const change = res.value;
      setSources((current) =>
        current === null
          ? current
          : {
              ...current,
              catalog: current.catalog.map((setting) =>
                setting.key === key
                  ? { ...setting, value: change.new, valueLabel: change.newLabel }
                  : setting,
              ),
            },
      );
      return true;
    },
    [machine],
  );

  return {
    scopes,
    results,
    idle,
    indexing: ask ? byKeyword.some((m) => m.indexing) : plain.indexing,
    marks: ask ? terms.list.join(' ') : words,
    answer,
    onAsk,
    onSetting,
  };
};
