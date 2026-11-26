// The people's comments on the page in the view («Комментарий для команды»): a round pin with the author's
// avatar at the corner of the block it is about; in the comment mode the page outlines the block under the pointer
// (its pick, `tk:pick`) and a click opens a light card for a new comment with @ for the people who have access and
// @harnessly (the comment then goes to the agent of the chat). A pin opens the thread: «Обсуждение», the block and the
// count of replies, «Решено», the link and closing; the replies under the first comment, reactions, the reply field
// with its button inside. The layer rides the page's viewport, so the pins scale with it; their places are in the
// page's CSS pixels and follow its scroll (`tk:scroll`).
import { createContext, use, useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react';
import Icon from '@/next/ui/icon';
import { t, plural } from '@/next/i18n';
import { readBridgeMessage, type IPicked } from '@/next/features/designly/page-bridge';
import { GUEST_ORIGIN } from '@/next/features/designly/bridge-origin';
import type { ICommentsLayerSlot } from '@/next/features/designly/site-view';
import { REACTIONS, type IPerson, type IThread } from '@/next/features/designly/panel/api';
import { useSiteCommentsContext } from '@/next/features/designly/panel/use-site-comments';
import Avatar from '@/next/features/designly/panel/avatar';
import { authorOf } from '@/next/features/designly/panel/comments-tab';
import { whenOf } from '@/next/features/designly/panel/when';
import { ShapesGroup } from '@/next/features/designly/draw/render';
import { copyText } from '@/next/features/designly/access/copy-link';
import { toast } from '@/next/features/designly/sdk/toast';
import '@/next/styles/site-comments.css';

/** Who may be called by @: the people with access, and the agent. */
export interface IMentionable {
  name: string;
  email: string;
}

/** What the layer knows of the design (use-design-side gives it): the board on screen, who may be called… */
export interface ICommentsLayerInfo {
  /** The board on screen. */
  board: string;
  people: readonly IMentionable[];
  /** The person, for the avatar of a new comment. */
  me: string;
  /** The address of the thread, for «Скопировать ссылку». */
  linkOf: (thread: string) => string;
}

export const CommentsLayerInfo = createContext<ICommentsLayerInfo | null>(null);

const CARD_W = 300;

/** The word after the last `@` being typed; null when the caret is not in one. */
export const mentionQuery = (text: string): string | null => {
  const m = /(^|\s)@([^\s@]*)$/.exec(text);
  return m ? m[2] : null;
};

/** The people (and the agent) a typed word finds, the agent first. */
export const mentionMatches = (people: readonly IMentionable[], query: string): IMentionable[] => {
  const q = query.toLowerCase();
  const agent = { name: 'harnessly', email: '' };
  const all = [agent, ...people];
  return all.filter((p) => p.name.toLowerCase().includes(q) || p.email.toLowerCase().includes(q)).slice(0, 6);
};

/** The people a text calls by @Name, by the list they were picked from. */
export const mentionedIn = (text: string, people: readonly IMentionable[]): IPerson[] =>
  people
    .filter((p) => p.email !== '' && text.includes(`@${p.name}`))
    .map((p) => ({ name: p.name, email: p.email }));

const blockLabel = (p: IPicked) => {
  const words = p.text.trim().replace(/\s+/g, ' ');
  if (p.component) return p.component;
  if (words) return words.length > 40 ? `${words.slice(0, 39)}…` : words;
  return p.tag;
};

interface IDraft {
  x: number;
  y: number;
  block: string;
  label: string;
}

/** A field with @ for people: the list over it while a word after @ is typed. */
const MentionField = ({
  value,
  onChange,
  onSubmit,
  onCancel,
  people,
  placeholder,
  multiline,
  autoFocus,
}: {
  value: string;
  onChange: (v: string) => void;
  onSubmit: () => void;
  onCancel?: () => void;
  people: readonly IMentionable[];
  placeholder: string;
  multiline: boolean;
  autoFocus?: boolean;
}) => {
  const q = mentionQuery(value);
  const found = q === null ? [] : mentionMatches(people, q);
  const [at, setAt] = useState(0);
  const pick = (p: IMentionable) => {
    onChange(value.replace(/@([^\s@]*)$/, `@${p.name} `));
    setAt(0);
  };
  const onKey = (e: KeyboardEvent<HTMLElement>) => {
    if (found.length > 0 && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      e.preventDefault();
      setAt((n) => (n + (e.key === 'ArrowDown' ? 1 : found.length - 1)) % found.length);
      return;
    }
    if (found.length > 0 && (e.key === 'Enter' || e.key === 'Tab')) {
      e.preventDefault();
      pick(found[Math.min(at, found.length - 1)]);
      return;
    }
    if (e.key === 'Escape') {
      e.stopPropagation();
      onCancel?.();
      return;
    }
    if (e.key === 'Enter' && (!multiline || e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      onSubmit();
    }
  };
  return (
    <>
      {found.length > 0 && (
        <div className="hx-cm-ment" role="listbox">
          <div className="hx-cm-ment__h">{t('next.ds.comments.call')}</div>
          {found.map((p, i) => (
            <button
              key={p.email || p.name}
              type="button"
              role="option"
              aria-selected={i === at}
              className="hx-cm-ment__i"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => pick(p)}
            >
              <Avatar name={p.name} className="hx-cm-av" />
              <span className="hx-cm-ment__n">{p.name}</span>
              <span className="hx-cm-ment__m">{p.email === '' ? t('next.ds.comments.agent') : p.email}</span>
            </button>
          ))}
        </div>
      )}
      {multiline ? (
        <textarea
          className="hx-cm-draft__field"
          rows={3}
          value={value}
          placeholder={placeholder}
          autoFocus={autoFocus}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={onKey}
        />
      ) : (
        <input
          className="hx-cm-reply__field"
          value={value}
          placeholder={placeholder}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={onKey}
        />
      )}
    </>
  );
};

/** A card placed by the corner it opens at, kept inside the layer. */
const useCardPlace = (x: number, y: number, layer: HTMLElement | null) => {
  const ref = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const w = layer?.clientWidth ?? 0;
    const h = layer?.clientHeight ?? 0;
    const left = Math.max(8, Math.min(x + 18, w - CARD_W - 8));
    const top = Math.max(8, Math.min(y, h - el.offsetHeight - 8));
    el.style.setProperty('--x', `${left}px`);
    el.style.setProperty('--y', `${top}px`);
  });
  return ref;
};

const Thread = ({
  thread,
  x,
  y,
  layer,
  people,
  linkOf,
}: {
  thread: IThread;
  x: number;
  y: number;
  layer: HTMLElement | null;
  people: readonly IMentionable[];
  linkOf: (thread: string) => string;
}) => {
  const store = useSiteCommentsContext();
  const ref = useCardPlace(x, y, layer);
  const [reply, setReply] = useState('');
  const [picking, setPicking] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  if (!store) return null;
  const { root, replies } = thread;
  const send = async () => {
    const text = reply.trim();
    if (text === '' || sending) return;
    setSending(true);
    if (await store.reply(root.id, text, mentionedIn(text, people))) setReply('');
    setSending(false);
  };
  return (
    <div ref={ref} className="hx-cm-card" role="dialog" aria-label={t('next.ds.comments.thread')}>
      <div className="hx-cm-card__h">
        <div className="hx-cm-card__tt">
          <span className="hx-cm-card__t">{t('next.ds.comments.thread')}</span>
          <span className="hx-cm-card__sub">
            {root.label !== '' && (
              <span className="hx-cm-blk">
                <Icon name="square-mouse-pointer" size="sm" />
                {root.label}
              </span>
            )}
            {replies.length > 0 && <span>{plural(replies.length, 'next.ds.comments.replies')}</span>}
          </span>
        </div>
        <button
          type="button"
          className="hx-cm-card__b"
          aria-pressed={root.resolved}
          title={root.resolved ? t('next.ds.comments.reopen') : t('next.ds.comments.resolve')}
          aria-label={root.resolved ? t('next.ds.comments.reopen') : t('next.ds.comments.resolve')}
          onClick={() => store.resolve(root.id, !root.resolved)}
        >
          <Icon name="circle-check" size="sm" />
        </button>
        <button
          type="button"
          className="hx-cm-card__b"
          title={t('next.ds.comments.link')}
          aria-label={t('next.ds.comments.link')}
          onClick={() =>
            void copyText(Promise.resolve(linkOf(root.id))).then(
              () => toast(t('next.ds.copied')),
              () => toast(t('next.site.copyFailed'), { tone: 'error' }),
            )
          }
        >
          <Icon name="link" size="sm" />
        </button>
        <button
          type="button"
          className="hx-cm-card__b"
          title={t('next.ds.close')}
          aria-label={t('next.ds.close')}
          onClick={() => store.setOpen(null)}
        >
          <Icon name="x" size="sm" />
        </button>
      </div>
      <div className="hx-cm-card__list">
        {[root, ...replies].map((m) => (
          <div key={m.id} className="hx-cm-msg" data-reply={m.parent !== '' || undefined}>
            <Avatar name={authorOf(m)} className="hx-cm-av" />
            <div className="hx-cm-msg__b">
              <div className="hx-cm-msg__h">
                <b>{authorOf(m)}</b>
                <span>{whenOf(m.at)}</span>
                <div className="hx-cm-msg__tools">
                  <button
                    type="button"
                    title={t('next.ds.comments.react')}
                    aria-label={t('next.ds.comments.react')}
                    onClick={() => setPicking(picking === m.id ? null : m.id)}
                  >
                    🙂
                  </button>
                </div>
              </div>
              <div className="hx-cm-msg__x">{m.text}</div>
              {m.reactions.length > 0 && (
                <div className="hx-cm-msg__r">
                  {m.reactions.map((r) => (
                    <button
                      key={r.e}
                      type="button"
                      className="hx-cm-react"
                      aria-pressed={r.mine}
                      title={r.names.join(', ')}
                      onClick={() => store.react(m.id, r.e)}
                    >
                      {r.e} {r.n}
                    </button>
                  ))}
                </div>
              )}
              {picking === m.id && (
                <div className="hx-cm-emoji">
                  {REACTIONS.map((e) => (
                    <button
                      key={e}
                      type="button"
                      onClick={() => {
                        setPicking(null);
                        store.react(m.id, e);
                      }}
                    >
                      {e}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
      <div className="hx-cm-card__in">
        <div className="hx-cm-reply">
          <MentionField
            value={reply}
            onChange={setReply}
            onSubmit={() => void send()}
            onCancel={() => store.setOpen(null)}
            people={people}
            placeholder={t('next.ds.comments.replyHint')}
            multiline={false}
          />
          <button
            type="button"
            className="hx-cm-reply__send"
            aria-label={t('next.ds.comments.send')}
            disabled={reply.trim() === '' || sending}
            onClick={() => void send()}
          >
            <Icon name="arrow-up" size="sm" />
          </button>
        </div>
      </div>
    </div>
  );
};

const Draft = ({
  draft,
  layer,
  me,
  people,
  onCancel,
  onSent,
  board,
}: {
  draft: IDraft & { px: number; py: number };
  layer: HTMLElement | null;
  me: string;
  people: readonly IMentionable[];
  onCancel: () => void;
  onSent: (id: string) => void;
  board: string;
}) => {
  const store = useSiteCommentsContext();
  const ref = useCardPlace(draft.px, draft.py, layer);
  const [text, setText] = useState('');
  const [sending, setSending] = useState(false);
  if (!store) return null;
  const send = async () => {
    const words = text.trim();
    if (words === '' || sending) return;
    setSending(true);
    const c = await store.add({
      board,
      x: draft.x,
      y: draft.y,
      block: draft.block,
      label: draft.label,
      text: words,
      mentions: mentionedIn(words, people),
    });
    setSending(false);
    if (c) onSent(c.id);
  };
  return (
    <div ref={ref} className="hx-cm-card hx-cm-draft" role="dialog" aria-label={t('next.ds.comments.new')}>
      <div className="hx-cm-card__h">
        <Avatar name={me} className="hx-cm-av" />
        <div className="hx-cm-card__tt">
          <span className="hx-cm-card__t">{t('next.ds.comments.new')}</span>
          {draft.label !== '' && (
            <span className="hx-cm-card__sub">
              <span className="hx-cm-blk">
                <Icon name="square-mouse-pointer" size="sm" />
                {draft.label}
              </span>
            </span>
          )}
        </div>
        <button
          type="button"
          className="hx-cm-card__b"
          title={t('next.ds.comments.cancel')}
          aria-label={t('next.ds.comments.cancel')}
          onClick={onCancel}
        >
          <Icon name="x" size="sm" />
        </button>
      </div>
      <div className="hx-cm-card__in hx-cm-card__in--top">
        <MentionField
          value={text}
          onChange={setText}
          onSubmit={() => void send()}
          onCancel={onCancel}
          people={people}
          placeholder={t('next.ds.comments.hint')}
          multiline
          autoFocus
        />
        <div className="hx-cm-draft__f">
          <button
            type="button"
            className="hx-cm-draft__send"
            disabled={text.trim() === '' || sending}
            onClick={() => void send()}
          >
            {t('next.ds.comments.send')}
          </button>
        </div>
      </div>
    </div>
  );
};

const Pin = ({
  thread,
  x,
  y,
  open,
  onOpen,
}: {
  thread: IThread;
  x: number;
  y: number;
  open: boolean;
  onOpen: () => void;
}) => {
  const ref = useRef<HTMLButtonElement>(null);
  useLayoutEffect(() => {
    ref.current?.style.setProperty('--x', `${x}px`);
    ref.current?.style.setProperty('--y', `${y}px`);
  }, [x, y]);
  return (
    <button
      ref={ref}
      type="button"
      className="hx-cm-pin"
      data-resolved={thread.root.resolved || undefined}
      data-open={open || undefined}
      aria-label={`${authorOf(thread.root)}: ${thread.root.text}`}
      title={thread.root.text}
      onClick={onOpen}
    >
      <Avatar name={authorOf(thread.root)} className="hx-cm-pin__av" />
    </button>
  );
};

/** The layer as the view mounts it over the page (site-view's commentsLayer): it reads the rest from the contexts. */
const CommentsLayer = ({ frame, origin }: ICommentsLayerSlot) => {
  const info = use(CommentsLayerInfo);
  if (!info || origin === null) return null;
  return <Layer {...info} frame={frame} origin={origin} />;
};

const Layer = ({
  board,
  people,
  me,
  linkOf,
  frame,
  origin,
}: ICommentsLayerInfo & { frame: ICommentsLayerSlot['frame']; origin: string }) => {
  const store = useSiteCommentsContext();
  const [layer, setLayer] = useState<HTMLDivElement | null>(null);
  const [scroll, setScroll] = useState(0);
  const [width, setWidth] = useState(0);
  const [draft, setDraft] = useState<IDraft | null>(null);
  const placing = store?.placing ?? false;

  // The page's scroll and picks: read from this frame only.
  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      const el = frame.current;
      if (!el || event.source !== el.contentWindow || (origin !== GUEST_ORIGIN && event.origin !== origin))
        return;
      const m = readBridgeMessage(event.data);
      if (m?.k === 'scroll') setScroll(m.y);
      else if (m?.k === 'picked' && store?.placing) {
        const { rect } = m.picked;
        const w = el.offsetWidth || 1;
        // The pin stands at the block's top right corner.
        const cornerX = Math.min(rect.x + rect.width, w - 12);
        setDraft({
          x: (cornerX / w) * 100,
          y: rect.y + scroll,
          block: m.picked.selector,
          label: blockLabel(m.picked),
        });
        store.setOpen(null);
      }
    };
    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
  }, [store, scroll, frame, origin]);
  // The comment mode keeps the page's pick on: the block under the pointer is outlined, a click names it.
  useEffect(() => {
    frame.current?.contentWindow?.postMessage({ type: 'tk:pick', on: placing && draft === null }, origin);
  }, [placing, draft, frame, origin]);
  useEffect(() => {
    if (!placing) return;
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.key === 'Escape') {
        setDraft(null);
        store?.setPlacing(false);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [placing, store]);
  useLayoutEffect(() => {
    const el = layer;
    if (!el) return;
    const ro = new ResizeObserver(() => setWidth(el.clientWidth));
    ro.observe(el);
    return () => ro.disconnect();
  }, [layer]);

  if (!store) return null;
  const threads = store.boardThreads(board).filter((th) => !th.root.resolved || th.root.id === store.open);
  const placeOf = (x: number, y: number) => ({ px: (x / 100) * width - 14, py: y - scroll - 14 });
  const opened = threads.find((th) => th.root.id === store.open) ?? null;
  return (
    <div ref={setLayer} className="hx-cm-layer" data-placing={placing || undefined}>
      {placing && draft === null && (
        <div className="hx-cm-tip" role="status">
          <Icon name="message-square-plus" size="sm" />
          {t('next.ds.comments.place')}
          <kbd>Esc</kbd>
        </div>
      )}
      {/* The drawing of the open thread stands where it was drawn on the page (HRN-1772). */}
      {opened?.root.drawing && (
        <svg className="hx-cm-drawing" aria-hidden="true">
          <ShapesGroup shapes={opened.root.drawing} sx={0} sy={scroll} />
        </svg>
      )}
      {threads.map((th) => {
        const at = placeOf(th.root.x, th.root.y);
        return (
          <Pin
            key={th.root.id}
            thread={th}
            x={at.px}
            y={at.py}
            open={th.root.id === store.open}
            onOpen={() => {
              setDraft(null);
              store.setOpen(store.open === th.root.id ? null : th.root.id);
            }}
          />
        );
      })}
      {opened && (
        <Thread
          key={opened.root.id}
          thread={opened}
          {...(({ px, py }) => ({ x: px, y: py }))(placeOf(opened.root.x, opened.root.y))}
          layer={layer}
          people={people}
          linkOf={linkOf}
        />
      )}
      {draft && (
        <Draft
          draft={{ ...draft, ...placeOf(draft.x, draft.y) }}
          layer={layer}
          me={me}
          people={people}
          board={board}
          onCancel={() => setDraft(null)}
          onSent={(id) => {
            setDraft(null);
            store.setPlacing(false);
            store.setOpen(id);
          }}
        />
      )}
    </div>
  );
};

export default CommentsLayer;
