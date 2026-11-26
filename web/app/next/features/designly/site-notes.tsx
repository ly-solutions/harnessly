// The person's edits on the page of a design: a note opens at the block they picked (the card .hx-note.hx-ann: the words
// for the agent and the hand's own changes of the block, note-tools.tsx), a pin
// with its number stays on the block, the notes ride over the island's field as a strip «N annotations» with a list, or,
// in «Правки», in the bar «Аннотирование · N» at the foot (.hx-notesbar) that sends them to the agent in one message.
// The pins, the card and the bar speak the shared language of annotations (ui/notes/notes.tsx).
import {
  useContext,
  useEffect,
  useEffectEvent,
  useLayoutEffect,
  useRef,
  useState,
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react';
import type { INoteChip } from '@/next/ui/notes/note-chips';
import { NoteBar, NoteDot } from '@/next/ui/notes/notes';
import Icon from '@/next/ui/icon';
import Sheet from '@/next/ui/sheet';
import { usePhone } from '@/next/ui/use-media';
import { changedKeys, hasEdit, toCss, type IStyleEdit } from '@/next/features/designly/element-style';
import { pickedLabel, type IPicked } from '@/next/features/designly/page-bridge';
import { BoardHandContext, HAND_KEY, type IHandWritten } from '@/next/features/designly/hand-edit';
import NoteTools, { SwatchDot } from '@/next/features/designly/note-tools';
import { useCardPlace, type IPlaceAt } from '@/next/features/designly/note-place';
import type { INoteArea } from '@/next/features/designly/note-area';
import { DRAFT_ANCHOR, useNoteAnchor } from '@/next/features/designly/note-anchor';
import { dragNote, keepNotePos, readNotePos } from '@/next/features/designly/note-drag';
import type { IPoint } from '@/next/features/designly/site-drag';
import { plural, t } from '@/next/i18n';
import type { MachineId } from '@/next/protocol/v2';
import { CardImageGen } from '@/next/features/designly/image-gen/canvas';
import { PRESENCE, useLeavingList, usePresence, usePresentFlag } from '@/next/ui/presence';
import '@/next/styles/notes.css';

export const NOTE_OUT_MS = 180;
const LIST_OUT_MS = 200;
const PIN_OUT_MS = 200;
const LASSO_OUT_MS = 160;
/** A press on these leaves the card as it is: the canvas's zoom, and what pops up out of the card or the page. */
const KEEPS_CARD = '.hx-vpzoom, [role="dialog"], [role="menu"], [role="listbox"], [data-sonner-toaster]';

/** Where a note opens: the block's corners in the section's pixels with the canvas at its fit, and the side it opens to
 *  (note-place.ts). */
export type INoteAt = IPlaceAt;

export interface INote {
  id: string;
  picked: IPicked;
  /** The page the block is on: its path on the library server. */
  page: string;
  text: string;
  /** Where its card opens again, from the pin or the list. */
  at: INoteAt;
  /** The look tried on the block; the page wears it while the note is there. */
  style: IStyleEdit;
  /** What the person changed by hand and the source already has, for the agent to keep. */
  saved?: readonly string[];
}

/** A note as a numbered chip of the field (ui/notes/note-chips.tsx): the block it is on; its card says the page, the
 *  block and the look tried on it, and holds the words to change. */
export const noteChip = (note: INote): INoteChip => {
  const diffs = [
    ...changedKeys(note.style).map((key) =>
      t(`next.notes.styleDiff.${key}`, { value: String(note.style[key]) }),
    ),
    ...(note.saved ?? []),
  ];
  return {
    id: note.id,
    name: pickedLabel(note.picked),
    comment: note.text,
    facts: [
      { label: t('next.composer.note.page'), value: note.page },
      { label: t('next.composer.note.element'), value: `<${note.picked.tag}>`, title: note.picked.selector },
      ...(diffs.length > 0 ? [{ label: t('next.notes.styleFact'), value: diffs.join(', ') }] : []),
    ],
  };
};

export interface INoteDraft {
  picked: IPicked;
  at: INoteAt;
  /** The note being changed; without it the card makes a new one. */
  id?: string;
  /** The words the card opens with. */
  text: string;
  /** The look on the block as the card has it now. */
  style: IStyleEdit;
  /** The card stands over an area or a drawing, not a block alone (note-area.ts). */
  area?: INoteArea;
}

/** The microphone of the card: dictation puts the words into the card's field. */
export interface INoteMic {
  recording: boolean;
  onToggle: () => void;
}

/**
 * The card of a guest of a link: it has only the words (the name was asked when commenting began), no block settings and no trash, and sends the comment to the
 * link's gateway: the card waits for the answer and stays, with its words, where it was refused.
 */
export interface INoteGuest {
  /** True when the comment was taken. */
  submit: (text: string) => Promise<boolean>;
}

/** How the card's words leave it: sent to the agent at once, or kept with the message (a press away). */
export const NOTE_OK = { send: 'send', keep: 'keep' } as const;
export type TNoteOk = (typeof NOTE_OK)[keyof typeof NOTE_OK];

export interface INotePopoverProps {
  n: number;
  draft: INoteDraft;
  text: string;
  onText: (text: string) => void;
  mic: INoteMic | null;
  /** The card's words go into the message to be sent with others («Правки», a note being changed), not at once. */
  batch: boolean;
  /** The card is done: the words for the agent, and the write of the hand's changes into the source where there is one. */
  onOk: (text: string, written: Promise<IHandWritten> | null, how: TNoteOk) => void;
  onCancel: () => void;
  /** The trash: a new card is dropped, a note is taken back. */
  onDelete: () => void;
  onStyle: (style: IStyleEdit) => void;
  /** The page no longer has the block: the look cannot be shown on it. */
  gone: boolean;
  /** Another page is on screen: the card steps away (it comes back with its page). */
  hidden?: boolean;
  guest?: INoteGuest;
  /** Where a picture can be drawn into the picked block: the machine, the design and the board file picked on. */
  image?: { machine: MachineId; artifact: string; file: string };
}

export const NotePopover = ({
  n,
  draft,
  text,
  onText,
  mic,
  batch,
  onOk,
  onCancel,
  onDelete,
  onStyle,
  gone,
  hidden = false,
  guest,
  image,
}: INotePopoverProps) => {
  const boardHand = useContext(BoardHandContext);
  // A guest only writes: the look of the block is the owner's to change.
  const look = guest ? null : draft.picked.look;
  const hand = look ? boardHand : null;
  const ref = useRef<HTMLDivElement>(null);
  const [closing, setClosing] = useState(false);
  const [sending, setSending] = useState(false);
  // The fill, the corners and the opacity are folded behind «⋯»: the card opens small, over less of the page.
  const [more, setMore] = useState(false);
  // A card opened by a double click on a text leaves the caret there.
  const [caretThere] = useState(() => hand?.caretAsked() === true);
  // Where the person moved the card (this session's); null while it stands at its block.
  const [moved, setMoved] = useState<IPoint | null>(readNotePos);
  /** The source takes the hand's changes: they are written when the card closes. */
  const writes = hand?.state != null && hand.state.tpl !== null;
  const direct = hasEdit(draft.style) || hand?.state?.changed === true;
  const words = text.trim();
  // An area or a drawing goes to the agent without words too.
  const area = draft.area;
  const wants = guest ? !words : !area && !words && !direct;
  /** Only the hand's own changes: the button saves them, no agent is asked. */
  const saves = writes && direct && !words;

  useCardPlace(ref, draft.at);

  useLayoutEffect(() => {
    if (!moved) return;
    ref.current?.style.setProperty('--mx', `${moved.x}px`);
    ref.current?.style.setProperty('--my', `${moved.y}px`);
  }, [moved]);

  // Its height goes to the CSS, which keeps a moved card inside the view as «⋯» unfolds.
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => el.style.setProperty('--nh', `${el.offsetHeight}px`);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  // The board holds the block while the card is open: its text is edited where it stands. Leaving without a word to the
  // board (the card taken away) gives the words back.
  const held = hand !== null && !hidden;
  const hold = useEffectEvent(() => hand?.hold(draft.picked));
  const drop = useEffectEvent(() => hand?.drop());
  useEffect(() => {
    if (!held) return;
    hold();
    return () => drop();
  }, [held, draft.picked]);

  const leave = (then: () => void) => {
    setClosing(true);
    window.setTimeout(then, NOTE_OUT_MS);
  };
  /** Hands the card's words and the hand's changes to its owner. */
  const finish = (how: TNoteOk) =>
    onOk(words, writes && direct ? (hand?.commit(toCss(draft.style, look)) ?? null) : null, how);
  const cancel = () => {
    hand?.drop();
    leave(onCancel);
  };
  // «Приложить» while the voice is still being written down: it finishes, and the card goes once the words are in.
  const [okAfterVoice, setOkAfterVoice] = useState(false);
  const ok = () => {
    // The card is on its way out: a second press would attach the note twice.
    if (closing || wants || sending) return;
    if (mic?.recording) {
      setOkAfterVoice(true);
      mic.onToggle();
      return;
    }
    if (!guest) {
      leave(() => finish(NOTE_OK.send));
      return;
    }
    setSending(true);
    void guest.submit(words).then((taken) => {
      setSending(false);
      if (taken) leave(onCancel);
    });
  };

  const voiceDone = okAfterVoice && !mic?.recording;
  const okAfterWords = useEffectEvent(() => {
    setOkAfterVoice(false);
    ok();
  });
  // The words land with the recording's end; the card goes on the next tick, with them in it.
  useEffect(() => {
    if (!voiceDone) return;
    const timer = window.setTimeout(okAfterWords, 0);
    return () => window.clearTimeout(timer);
  }, [voiceDone]);

  /**
   * A press away closes the card and keeps what was done: the hand's changes are written, typed words stay with the
   * message. At once, not after the exit: the press may open another card.
   */
  const away = useEffectEvent(() => {
    if (closing || sending || mic?.recording) return;
    if (guest) {
      if (!words) onCancel();
      return;
    }
    // A drawing is not lost to a stray press: the card of an area waits for its button.
    if (area) return;
    if (wants) {
      hand?.drop();
      onCancel();
      return;
    }
    finish(NOTE_OK.keep);
  });
  useEffect(() => {
    if (hidden) return;
    const onDown = (event: PointerEvent) => {
      const target = event.target;
      if (!(target instanceof Element) || ref.current?.contains(target) || target.closest(KEEPS_CARD)) return;
      away();
    };
    document.addEventListener('pointerdown', onDown, true);
    return () => document.removeEventListener('pointerdown', onDown, true);
  }, [hidden]);
  // The same from the board: a press away on it, Esc and Enter typed there.
  const signal = hand?.signal ?? null;
  const heard = useRef(signal?.n ?? 0);
  const answer = useEffectEvent(() => {
    if (!signal || signal.n <= heard.current) return;
    heard.current = signal.n;
    if (signal.kind === HAND_KEY.escape) cancel();
    else if (signal.kind === HAND_KEY.enter) ok();
    else away();
  });
  useEffect(() => {
    const timer = window.setTimeout(answer, 0);
    return () => window.clearTimeout(timer);
  }, [signal?.n]);

  // The words grow the field up to a few lines (the CSS caps it).
  const field = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => {
    const el = field.current;
    if (!el) return;
    el.style.height = '0px';
    el.style.height = `${el.scrollHeight}px`;
  }, [text]);

  // The card is moved by its own ground (not a field or a button); a double press puts it back at its block.
  const onGround = (event: ReactPointerEvent<HTMLElement>) => {
    if (event.target !== event.currentTarget) return;
    dragNote(event, (at) => {
      setMoved(at);
      keepNotePos(at);
    });
  };
  const onGroundDouble = (event: ReactMouseEvent<HTMLElement>) => {
    if (event.target !== event.currentTarget) return;
    setMoved(null);
    keepNotePos(null);
  };
  const okLabel = () => {
    if (guest) return t('next.guest.send');
    if (saves) return t('next.notes.save');
    return t(batch ? 'next.notes.toMessage' : 'next.notes.toAgent');
  };

  return (
    <div
      ref={ref}
      className="hx-note hx-ann"
      data-state={closing || hidden ? 'closing' : 'open'}
      data-more={look !== null && more}
      data-moved={moved !== null}
      role="dialog"
      aria-label={t('next.notes.dialog')}
      aria-hidden={hidden}
      inert={hidden}
      onPointerDown={onGround}
      onDoubleClick={onGroundDouble}
      onKeyDown={(event) => {
        if (event.key === 'Escape') {
          event.preventDefault();
          event.stopPropagation();
          cancel();
        }
      }}
    >
      <div className="hx-ann__body">
        <div className="hx-ann__row" onPointerDown={onGround} onDoubleClick={onGroundDouble}>
          {!look && (
            <NoteDot
              className="hx-ann__n"
              n={n}
              label={t('next.notes.pin', { n, label: pickedLabel(draft.picked) })}
              empty={!words}
            />
          )}
          <textarea
            ref={field}
            className="hx-ann__input"
            autoFocus={!caretThere}
            rows={1}
            disabled={sending}
            placeholder={guest ? t('next.guest.placeholder') : t('next.notes.placeholder')}
            aria-label={guest ? t('next.guest.placeholder') : t('next.notes.placeholder')}
            value={text}
            onChange={(event) => onText(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
                event.preventDefault();
                ok();
              }
            }}
          />
        </div>
        {look && (
          <NoteTools
            picked={draft.picked}
            look={look}
            edit={draft.style}
            onEdit={onStyle}
            hand={hand}
            open={more}
            onOpen={setMore}
            gone={gone}
          />
        )}
        {image && !guest && draft.picked.draw && (
          <CardImageGen
            machine={image.machine}
            artifact={image.artifact}
            file={image.file}
            draw={draft.picked.draw}
          />
        )}
        <div className="hx-ann__foot" onPointerDown={onGround} onDoubleClick={onGroundDouble}>
          {/* Leaves the card: the block and a note being changed go back to what they were, a new one is not kept. */}
          <button type="button" className="hx-ann__cancel" onClick={cancel}>
            {t('next.notes.revert')}
          </button>
          {!guest && (
            <button
              type="button"
              className="hx-ann__icon hx-ann__del hx-tipped"
              aria-label={t('next.notes.delete')}
              onClick={() => {
                hand?.drop();
                leave(onDelete);
              }}
            >
              <Icon name="trash-2" size="sm" />
              <span className="hx-tip" data-pos="below">
                {t('next.notes.delete')}
              </span>
            </button>
          )}
          <span className="hx-spacer" onPointerDown={onGround} onDoubleClick={onGroundDouble} />
          {area?.onComment && (
            <button
              type="button"
              className="hx-ann__icon hx-tipped"
              aria-label={t('next.notes.comment')}
              onClick={() => {
                hand?.drop();
                void area.onComment?.(words);
                leave(onCancel);
              }}
            >
              <Icon name="message-circle" size="sm" />
              <span className="hx-tip" data-pos="below" data-align="end">
                {t('next.notes.comment')}
              </span>
            </button>
          )}
          {mic && (
            <button
              type="button"
              className="hx-ann__icon hx-ann__mic hx-tipped"
              data-rec={mic.recording}
              aria-label={t('next.notes.dictate')}
              onClick={mic.onToggle}
            >
              <Icon name="mic" />
              <span className="hx-tip" data-pos="below" data-align="end">
                {t('next.notes.dictate')}
              </span>
            </button>
          )}
          <button
            type="button"
            className="hx-ann__ok hx-tipped"
            data-save={saves || undefined}
            aria-label={okLabel()}
            aria-disabled={wants || sending || undefined}
            aria-busy={sending || undefined}
            onClick={ok}
          >
            {saves ? okLabel() : <Icon name="arrow-up" />}
            <span className="hx-tip" data-pos="below" data-align="end">
              {okLabel()}
              <kbd>↵</kbd>
            </span>
          </button>
        </div>
      </div>
    </div>
  );
};

/** «Аннотирование · N» at the foot of the page in «Правки»: «Отправить» sends the notes to the agent in one message. */
export const NotesBar = ({
  notes,
  shown,
  onSend,
}: {
  notes: readonly INote[];
  shown: boolean;
  onSend: () => void;
}) => (
  <NoteBar
    className="hx-notesbar"
    visible={shown}
    count={notes.length}
    sendLabel={t('next.notes.send')}
    onSend={onSend}
  />
);

const EMPTY_NOTES: readonly INote[] = [];

/** A note's number: the notes that stay before it, and itself (one that is leaving keeps the place it had). */
const numberAt = (rows: readonly { leaving: boolean }[], index: number) =>
  rows.slice(0, index).filter((row) => !row.leaving).length + 1;

/** A pin on a block: the frame, the number at the corner and, on hover, the note's words. */
const NotePin = ({
  note,
  n,
  open,
  leaving,
  onOpen,
}: {
  note: INote;
  n: number;
  open: boolean;
  leaving: boolean;
  onOpen: () => void;
}) => {
  const ref = useRef<HTMLDivElement>(null);
  useNoteAnchor(ref, note.id, note.picked.rect);
  return (
    <div
      ref={ref}
      className="hx-npin"
      data-open={open}
      data-state={leaving ? PRESENCE.closing : PRESENCE.open}
    >
      <span className="hx-npin__frame" />
      <span className="hx-npin__mark">
        <NoteDot
          n={n}
          label={t('next.notes.pin', { n, label: pickedLabel(note.picked) })}
          empty={!note.text}
          active={open}
          inert={leaving}
          onOpen={onOpen}
        />
        {note.text && <span className="hx-npin__tip">{note.text}</span>}
      </span>
    </div>
  );
};

/** The frame of the block a new note is being written for. */
const NoteLasso = ({ rect }: { rect: IPicked['rect'] | null }) => {
  const ref = useRef<HTMLDivElement>(null);
  const { value, state } = usePresence(rect, LASSO_OUT_MS);
  useNoteAnchor(ref, DRAFT_ANCHOR, value);
  if (!value) return null;
  return <div ref={ref} className="hx-lasso" data-on={state === PRESENCE.open} aria-hidden="true" />;
};

/** The notes on the page itself, in the page's viewport pixels: they scale with it. */
export const NotePins = ({
  notes,
  draft,
  onOpen,
}: {
  notes: readonly INote[];
  draft: INoteDraft | null;
  onOpen: (id: string) => void;
}) => {
  const pins = useLeavingList(notes, (note) => note.id, PIN_OUT_MS);
  return (
    <>
      {pins.map(({ item, leaving }, index) => (
        <NotePin
          key={item.id}
          note={item}
          n={numberAt(pins, index)}
          open={draft?.id === item.id}
          leaving={leaving}
          onOpen={() => onOpen(item.id)}
        />
      ))}
      <NoteLasso rect={draft && draft.id === undefined ? draft.picked.rect : null} />
    </>
  );
};

/** What was changed on the block, one chip each: «Фон → #FFFFFF». */
const StyleChips = ({ edit }: { edit: IStyleEdit }) => {
  const keys = changedKeys(edit);
  if (keys.length === 0) return null;
  return (
    <div className="hx-anns__diffs">
      {keys.map((key) => (
        <span key={key} className="hx-anns__diff">
          {(key === 'background' || key === 'color') && <SwatchDot color={edit[key]} />}
          {t(`next.notes.styleDiff.${key}`, { value: String(edit[key]) })}
        </span>
      ))}
    </div>
  );
};

/** One row of the list: the number on the block's mark, what it is and what was asked, the pencil and the trash. */
const AnnRow = ({
  note,
  n,
  index,
  leaving,
  onEdit,
  onRemove,
  readOnly,
}: {
  note: INote;
  n: number;
  index: number;
  leaving: boolean;
  onEdit: () => void;
  onRemove: () => void;
  readOnly: boolean;
}) => {
  const ref = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    ref.current?.style.setProperty('--i', String(index));
  }, [index]);
  return (
    <div ref={ref} className="hx-anns__item" data-leaving={leaving}>
      <span className="hx-anns__thumb" data-img="false">
        <Icon name="square-dashed-mouse-pointer" size="sm" />
        <span className="hx-anns__n">{n}</span>
      </span>
      <div className="hx-anns__body">
        <div className="hx-anns__head">
          <code className="hx-anns__tag">{note.picked.tag}</code>
          <span className="hx-anns__target">{pickedLabel(note.picked)}</span>
        </div>
        {note.text && <div className="hx-anns__text">{note.text}</div>}
        <StyleChips edit={note.style} />
      </div>
      {!readOnly && (
        <div className="hx-anns__acts">
          <button
            type="button"
            className="hx-anns__act hx-tipped"
            aria-label={t('next.notes.edit')}
            onClick={onEdit}
          >
            <Icon name="pencil" size="sm" />
            <span className="hx-tip" data-align="end">
              {t('next.notes.edit')}
            </span>
          </button>
          <button
            type="button"
            className="hx-anns__act hx-tipped"
            data-danger="true"
            aria-label={t('next.notes.remove')}
            onClick={onRemove}
          >
            <Icon name="trash-2" size="sm" />
            <span className="hx-tip" data-align="end">
              {t('next.notes.remove')}
            </span>
          </button>
        </div>
      )}
    </div>
  );
};

/** Over the island's field: «N annotations» with the cross that takes them all back; the button opens the list, where
 *  a note is changed or taken back. The strip leaves with the last note, the list with its own animation. */
export const NoteStrip = ({
  notes,
  onEdit,
  onRemove,
  onClear,
  readOnly = false,
}: {
  notes: readonly INote[];
  onEdit: (id: string) => void;
  onRemove: (id: string) => void;
  onClear: () => void;
  /** A guest's list of the comments: only read; on the phone it is a bottom sheet. */
  readOnly?: boolean;
}) => {
  const phone = usePhone();
  const sheeted = readOnly && phone;
  const [open, setOpen] = useState(false);
  const kept = usePresence(notes.length > 0 ? notes : null, LIST_OUT_MS);
  const list = usePresentFlag(open && notes.length > 0, LIST_OUT_MS);
  const rows = useLeavingList(kept.value ?? EMPTY_NOTES, (note) => note.id, LIST_OUT_MS);

  // A press outside the strip and Esc close the list (a sheet closes itself).
  useEffect(() => {
    if (!open || sheeted) return;
    const onDown = (event: PointerEvent) => {
      if (!(event.target as Element).closest('.hx-anns')) setOpen(false);
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    window.addEventListener('pointerdown', onDown);
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('pointerdown', onDown);
      window.removeEventListener('keydown', onKey);
    };
  }, [open, sheeted]);

  if (kept.value === null) return null;
  const listBox = (
    <div
      className="hx-anns__list"
      data-state={sheeted ? PRESENCE.open : list.state}
      role="dialog"
      aria-label={t('next.notes.annotations')}
    >
      {rows.map(({ item, leaving }, index) => (
        <AnnRow
          key={item.id}
          note={item}
          n={numberAt(rows, index)}
          index={index}
          leaving={leaving}
          readOnly={readOnly}
          onEdit={() => {
            setOpen(false);
            onEdit(item.id);
          }}
          onRemove={() => {
            if (notes.length <= 1) setOpen(false);
            onRemove(item.id);
          }}
        />
      ))}
    </div>
  );
  return (
    <div className="hx-island__chips">
      <div className="hx-anns" data-gone={kept.state === PRESENCE.closing}>
        {list.mounted && !sheeted && listBox}
        {sheeted && (
          <Sheet open={open} onClose={() => setOpen(false)} label={t('next.notes.annotations')}>
            {listBox}
          </Sheet>
        )}
        <span className="hx-anns__chip" data-open={open}>
          <button
            type="button"
            className="hx-anns__main"
            aria-expanded={open}
            onClick={() => setOpen((on) => !on)}
          >
            <Icon name="message-square-text" size="sm" />
            <span>{plural(kept.value.length, 'next.notes.annotationsCount')}</span>
            <Icon name="chevron-up" size="sm" className="hx-anns__chev" />
          </button>
          {!readOnly && (
            <button
              type="button"
              className="hx-anns__x"
              aria-label={t('next.notes.clear')}
              onClick={() => {
                setOpen(false);
                onClear();
              }}
            >
              <Icon name="x" size="sm" />
            </button>
          )}
        </span>
      </div>
    </div>
  );
};
