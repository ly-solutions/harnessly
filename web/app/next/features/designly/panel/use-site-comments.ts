// The people's threads on a design's boards, one store for the list in «Макет → Комментарии» and the pins on
// the page: read on open and every few seconds while the page is seen, changed in place by what each write answered.
// A comment that calls @harnessly is handed on to the agent of the chat the board is open beside.
import { createContext, use, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { MachineId } from '@/next/protocol/v2';
import {
  addSiteComment,
  fetchSiteComments,
  reactSiteComment,
  resolveSiteComment,
  tellAgent,
  threadsOf,
  type ICommentDraft,
  type ISiteComment,
  type IThread,
} from '@/next/features/designly/panel/api';
import { toast } from '@/next/features/designly/sdk/toast';
import { t } from '@/next/i18n';

const POLL_MS = 8000;

/** `@harnessly` in a comment calls the agent of the chat. */
export const AGENT_MENTION = /(^|\s)@harnessly\b/i;

export interface ISiteComments {
  threads: IThread[];
  /** The threads of one board, the open ones and the resolved ones. */
  boardThreads: (board: string) => IThread[];
  /** The thread shown in its card; null for none. */
  open: string | null;
  setOpen: (id: string | null) => void;
  add: (draft: Omit<ICommentDraft, 'parent' | 'agent'>) => Promise<ISiteComment | null>;
  reply: (thread: string, text: string, mentions: ICommentDraft['mentions']) => Promise<boolean>;
  resolve: (thread: string, resolved: boolean) => void;
  react: (comment: string, emoji: string) => void;
  /** Comment mode: a click on the page starts a comment. */
  placing: boolean;
  setPlacing: (on: boolean) => void;
}

export const SiteCommentsContext = createContext<ISiteComments | null>(null);
export const useSiteCommentsContext = () => use(SiteCommentsContext);

export const useSiteComments = (
  machine: MachineId,
  project: string,
  /** The chat the board is open beside: a comment that calls @harnessly goes to its agent. */
  chat: string | null,
): ISiteComments => {
  const [comments, setComments] = useState<ISiteComment[]>([]);
  const [open, setOpen] = useState<string | null>(null);
  const [placing, setPlacing] = useState(false);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const now = new AbortController();
    const read = () => fetchSiteComments(machine, project, now.signal).then(setComments, () => undefined);
    void read();
    const timer = setInterval(() => {
      if (!document.hidden) void read();
    }, POLL_MS);
    return () => {
      now.abort();
      clearInterval(timer);
    };
  }, [machine, project, tick]);
  // The thread the address names (`?comment=`, a mention in the bell) opens once the threads are read.
  const asked = useRef(new URLSearchParams(window.location.search).get('comment'));
  useEffect(() => {
    if (asked.current && comments.some((c) => c.id === asked.current)) {
      setOpen(asked.current);
      asked.current = null;
    }
  }, [comments]);

  const put = useCallback((c: ISiteComment) => {
    setComments((list) => {
      const at = list.findIndex((x) => x.id === c.id);
      if (at < 0) return [...list, c];
      const next = list.slice();
      next[at] = c;
      return next;
    });
  }, []);
  const failed = (e: unknown) =>
    toast(t('next.ds.comments.failed'), {
      tone: 'error',
      description: e instanceof Error ? e.message : undefined,
    });
  const handOn = useCallback(
    (c: ISiteComment) => {
      if (!c.agent || chat === null) return;
      tellAgent(machine, project, c.id, chat).then(
        () => toast(t('next.ds.comments.toAgent')),
        (e: unknown) => failed(e),
      );
    },
    [machine, project, chat],
  );

  const threads = useMemo(() => threadsOf(comments), [comments]);
  return {
    threads,
    boardThreads: (board) => threads.filter((th) => th.root.board === board),
    open,
    setOpen,
    placing,
    setPlacing,
    add: async (draft) => {
      try {
        const c = await addSiteComment(machine, project, {
          ...draft,
          parent: '',
          agent: AGENT_MENTION.test(draft.text),
        });
        put(c);
        handOn(c);
        return c;
      } catch (e) {
        failed(e);
        return null;
      }
    },
    reply: async (thread, text, mentions) => {
      try {
        const c = await addSiteComment(machine, project, {
          board: '',
          x: 0,
          y: 0,
          block: '',
          label: '',
          text,
          parent: thread,
          mentions,
          agent: AGENT_MENTION.test(text),
        });
        put(c);
        handOn(c);
        return true;
      } catch (e) {
        failed(e);
        return false;
      }
    },
    resolve: (thread, resolved) => {
      resolveSiteComment(machine, project, thread, resolved).then(put, (e: unknown) => {
        failed(e);
        setTick((n) => n + 1);
      });
    },
    react: (comment, emoji) => {
      reactSiteComment(machine, project, comment, emoji).then(put, failed);
    },
  };
};
