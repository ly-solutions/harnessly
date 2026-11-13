// The timeline: a ruler with the time, a scene track of tiles sized by duration (number, title, length), a moments
// track (derived moments dashed), the playhead with its handle, comment markers for moments and ranges. A click or a
// drag anywhere scrubs the playhead; a drag with Shift selects a range and offers «Комментировать», «В чат» and
// «Повторять». A hover shows the time under the pointer.
import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react';
import { formatVideoAnnotation } from '@/next/features/designly/video/chat-context';
import { clock, spanText } from '@/next/features/designly/video/comments-model';
import {
  DRAG_SLOP_PX,
  SEEK_STEP_MS,
  clampAnchor,
  dotBoxes,
  markerBox,
  msToFrac,
  normalizeRange,
  pxToMs,
  rulerTicks,
  sceneIndexAt,
  spanOfRange,
  tickLabel,
  tileBoxes,
} from '@/next/features/designly/video/timeline-model';
import { useVideoComments } from '@/next/features/designly/video/use-video-comments';
import { STRIP, stripLanes, stripSpan, type TStrip } from '@/next/features/designly/video/overlays-model';
import type { IVideoOverlayEditor } from '@/next/features/designly/video/use-video-overlays';
import { useVars } from '@/next/features/designly/deck/use-vars';
import type {
  IVideoOverlay,
  IVideoRange,
  IVideoScene,
  IVideoState,
  TVideoFormat,
} from '@/next/features/designly/video/types';
import { t } from '@/next/i18n';
import type { MachineId } from '@/next/protocol/v2';
import Icon from '@/next/ui/icon';
import { usePresence } from '@/next/ui/presence';

export interface IVideoTimelineProps {
  machine: MachineId;
  id: string;
  video: IVideoState;
  format: TVideoFormat;
  /** The playhead, ms of the whole video. */
  time: number;
  onSeek: (time: number) => void;
  range: IVideoRange | null;
  onRange: (range: IVideoRange | null) => void;
  loop: boolean;
  onLoop: (loop: boolean) => void;
  /** «Комментировать» on the range: the view opens the composer on it. */
  onComment: (range: IVideoRange) => void;
  /** «В чат»: the annotation text (`<annotation n kind="video">`) for the chat's context. */
  onSendToChat: (annotation: string) => void;
  /** The overlay layer's editing: its strips lie under the scene on the playhead. */
  overlays: IVideoOverlayEditor;
}

/** The actions leave in 160 ms. */
const ACTIONS_OUT_MS = 160;

const pct = (frac: number) => `${(frac * 100).toFixed(3)}%`;

/** Something placed on a track by fractions of its width (CSS variables, no inline styles). */
const Placed = ({
  cls,
  left,
  width,
  ...rest
}: {
  cls: string;
  left: number;
  width?: number;
  title?: string;
  'data-on'?: string;
  'data-derived'?: string;
  'data-done'?: string;
  'data-changed'?: string;
  'data-range'?: string;
}) => {
  const ref = useVars<HTMLDivElement>({ '--l': pct(left), '--w': pct(width ?? 0) });
  return <div ref={ref} className={cls} {...rest} />;
};

const PlacedButton = ({
  cls,
  left,
  width,
  label,
  onClick,
  ...rest
}: {
  cls: string;
  left: number;
  width: number;
  label: string;
  onClick: () => void;
  'data-done'?: string;
  'data-changed'?: string;
  'data-range'?: string;
  'data-on'?: string;
}) => {
  const ref = useVars<HTMLButtonElement>({ '--l': pct(left), '--w': pct(width) });
  return (
    <button
      ref={ref}
      className={cls}
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      {...rest}
    />
  );
};

interface IStripDown {
  id: string;
  mode: TStrip;
  px: number;
  pointer: number;
  from: { at: number; out: number | null };
  moved: boolean;
}

interface IStripsProps {
  editor: IVideoOverlayEditor;
  scene: IVideoScene;
  total: number;
  track: { current: HTMLDivElement | null };
  onSeek: (time: number) => void;
}

/** The strips of a scene's overlays under it: the edges drag the in and out, the middle moves the whole strip. */
const OverlayStrips = ({ editor, scene, total, track, onSeek }: IStripsProps) => {
  const down = useRef<IStripDown | null>(null);
  const list = editor.overlaysOf(scene);
  const lanes = stripLanes(list, scene.dur);
  const count = Math.max(1, ...[...lanes.values()].map((l) => l + 1));
  const rowRef = useVars<HTMLDivElement>({ '--lanes': String(count) });

  const span = (event: ReactPointerEvent<HTMLElement>, d: IStripDown) => {
    const rect = track.current?.getBoundingClientRect();
    if (!rect || rect.width <= 0) return null;
    return stripSpan(d.from, d.mode, ((event.clientX - d.px) / rect.width) * total, scene.dur);
  };
  const onDown = (event: ReactPointerEvent<HTMLElement>, o: IVideoOverlay) => {
    if (event.button !== 0) return;
    event.stopPropagation();
    const edge = (event.target as HTMLElement).dataset.edge;
    const mode: TStrip = edge === 'start' ? STRIP.start : edge === 'end' ? STRIP.end : STRIP.move;
    event.currentTarget.setPointerCapture(event.pointerId);
    down.current = {
      id: o.id,
      mode,
      px: event.clientX,
      pointer: event.pointerId,
      from: { at: o.at, out: o.out },
      moved: false,
    };
  };
  const onMove = (event: ReactPointerEvent<HTMLElement>, o: IVideoOverlay) => {
    const d = down.current;
    if (!d || d.pointer !== event.pointerId) return;
    if (!d.moved && Math.abs(event.clientX - d.px) < DRAG_SLOP_PX) return;
    d.moved = true;
    const next = span(event, d);
    if (next) editor.preview(scene.id, { ...o, ...next });
  };
  const onUp = (event: ReactPointerEvent<HTMLElement>, o: IVideoOverlay) => {
    const d = down.current;
    if (!d || d.pointer !== event.pointerId) return;
    down.current = null;
    event.stopPropagation();
    if (!d.moved) {
      editor.setOn(true);
      editor.select(o.id);
      onSeek(scene.start + o.at);
      return;
    }
    const next = span(event, d);
    if (next === null || event.type === 'pointercancel') editor.discard(o.id);
    else void editor.save(scene.id, o.id, { at: next.at, out: next.out });
  };

  return (
    <div ref={rowRef} className="hx-video-tl__row" data-row="overlays">
      {list.map((o) => (
        <OverlayStrip
          key={o.id}
          overlay={o}
          scene={scene}
          total={total}
          lane={lanes.get(o.id) ?? 0}
          selected={editor.selected === o.id}
          onDown={onDown}
          onMove={onMove}
          onUp={onUp}
        />
      ))}
    </div>
  );
};

interface IStripProps {
  overlay: IVideoOverlay;
  scene: IVideoScene;
  total: number;
  lane: number;
  selected: boolean;
  onDown: (event: ReactPointerEvent<HTMLElement>, o: IVideoOverlay) => void;
  onMove: (event: ReactPointerEvent<HTMLElement>, o: IVideoOverlay) => void;
  onUp: (event: ReactPointerEvent<HTMLElement>, o: IVideoOverlay) => void;
}

const OverlayStrip = ({ overlay: o, scene, total, lane, selected, onDown, onMove, onUp }: IStripProps) => {
  const left = msToFrac(scene.start + o.at, total);
  const width = msToFrac((o.out ?? scene.dur) - o.at, total);
  const ref = useVars<HTMLDivElement>({ '--l': pct(left), '--w': pct(width), '--lane': String(lane) });
  return (
    <div
      ref={ref}
      className="hx-video-tl__strip"
      data-on={selected ? '1' : '0'}
      title={o.text}
      onPointerDown={(event) => onDown(event, o)}
      onPointerMove={(event) => onMove(event, o)}
      onPointerUp={(event) => onUp(event, o)}
      onPointerCancel={(event) => onUp(event, o)}
    >
      <i data-edge="start" />
      <i data-edge="end" />
    </div>
  );
};

/** What a pointer down on the track started: scrubbing the playhead, or (with Shift) selecting a range. */
const DRAG = { scrub: 'scrub', range: 'range' } as const;
type TDrag = (typeof DRAG)[keyof typeof DRAG];

interface IDown {
  px: number;
  at: number;
  pointer: number;
  mode: TDrag;
}

/** The track's width in px, kept as it resizes (the ruler picks its label step from it). */
const useWidth = (ref: { current: HTMLElement | null }): number => {
  const [width, setWidth] = useState(0);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    setWidth(el.getBoundingClientRect().width);
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    observer.observe(el);
    return () => observer.disconnect();
  }, [ref]);
  return width;
};

/** A scene tile: its number and title, its length at the right when there is room. */
const SceneTile = ({
  scene,
  index,
  left,
  width,
  on,
}: {
  scene: IVideoScene;
  index: number;
  left: number;
  width: number;
  on: boolean;
}) => {
  const ref = useVars<HTMLDivElement>({ '--l': pct(left), '--w': pct(width) });
  return (
    <div
      ref={ref}
      className="hx-video-tl__tile"
      data-on={on ? '1' : '0'}
      title={`${index + 1}. ${scene.title} · ${clock(scene.dur)}`}
    >
      <span className="hx-video-tl__tile-n">{index + 1}</span>
      <span className="hx-video-tl__tile-t">{scene.title}</span>
      <span className="hx-video-tl__tile-d">{clock(scene.dur)}</span>
    </div>
  );
};

/** A labelled point on the track: the playhead's or the hover's time above the ruler. */
const Flag = ({ cls, left, text }: { cls: string; left: number; text: string }) => {
  const ref = useVars<HTMLDivElement>({ '--l': pct(left) });
  return (
    <div ref={ref} className={cls}>
      <span>{text}</span>
    </div>
  );
};

const Tick = ({ left, major, label }: { left: number; major: boolean; label: string }) => {
  const ref = useVars<HTMLDivElement>({ '--l': pct(left) });
  return (
    <div ref={ref} className="hx-video-tl__tick" data-major={major ? '1' : '0'}>
      {major && <span>{label}</span>}
    </div>
  );
};

const VideoTimeline = ({
  machine,
  id,
  video,
  format,
  time,
  onSeek,
  range,
  onRange,
  loop,
  onLoop,
  onComment,
  onSendToChat,
  overlays,
}: IVideoTimelineProps) => {
  const track = useRef<HTMLDivElement>(null);
  const down = useRef<IDown | null>(null);
  const frame = useRef<number | null>(null);
  const pendingSeek = useRef<number | null>(null);
  const [dragging, setDragging] = useState<TDrag | null>(null);
  const [hover, setHover] = useState<number | null>(null);
  const comments = useVideoComments(machine, id, video);
  const width = useWidth(track);
  const total = video.durMs;
  const { scenes } = video;

  const tiles = tileBoxes(scenes, total);
  const dots = dotBoxes(scenes, total);
  const ticks = rulerTicks(total, width);
  const current = sceneIndexAt(scenes, time);

  useEffect(
    () => () => {
      if (frame.current !== null) cancelAnimationFrame(frame.current);
    },
    [],
  );

  const msAt = (event: ReactPointerEvent<HTMLElement>): number => {
    const rect = track.current?.getBoundingClientRect();
    return rect ? pxToMs(event.clientX - rect.left, rect.width, total) : 0;
  };
  // Scrubbing seeks once a frame: the player switches scenes, a seek per pointer event would queue them up.
  const scrubTo = (ms: number) => {
    pendingSeek.current = ms;
    if (frame.current !== null) return;
    frame.current = requestAnimationFrame(() => {
      frame.current = null;
      if (pendingSeek.current !== null) onSeek(pendingSeek.current);
      pendingSeek.current = null;
    });
  };

  const onDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    // A comment marker is a button of its own: its click seeks to the comment.
    if ((event.target as HTMLElement).closest('button')) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    const at = msAt(event);
    const mode = event.shiftKey ? DRAG.range : DRAG.scrub;
    down.current = { px: event.clientX, at, pointer: event.pointerId, mode };
    if (mode === DRAG.scrub) {
      onRange(null);
      setDragging(DRAG.scrub);
      scrubTo(at);
    }
  };
  const onMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    const at = msAt(event);
    setHover(at);
    const d = down.current;
    if (!d || d.pointer !== event.pointerId) return;
    if (d.mode === DRAG.scrub) {
      scrubTo(at);
      return;
    }
    if (dragging !== DRAG.range && Math.abs(event.clientX - d.px) < DRAG_SLOP_PX) return;
    setDragging(DRAG.range);
    onRange(normalizeRange(d.at, at, total));
  };
  const onUp = (event: ReactPointerEvent<HTMLDivElement>) => {
    const d = down.current;
    if (!d || d.pointer !== event.pointerId) return;
    down.current = null;
    setDragging(null);
    if (d.mode === DRAG.scrub) {
      scrubTo(msAt(event));
      return;
    }
    // A Shift-click without a drag is a plain seek.
    if (dragging !== DRAG.range) {
      onRange(null);
      onSeek(msAt(event));
    }
  };
  const onKey = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'ArrowLeft') onSeek(Math.max(0, time - SEEK_STEP_MS));
    else if (event.key === 'ArrowRight') onSeek(Math.min(total, time + SEEK_STEP_MS));
    else if (event.key === 'Home') onSeek(0);
    else if (event.key === 'End') onSeek(total);
    else if (event.key === 'Escape' && range) onRange(null);
    else return;
    event.preventDefault();
  };

  const shown = usePresence(range, ACTIONS_OUT_MS);
  const actionsRef = useVars<HTMLDivElement>({
    '--l': pct(msToFrac(shown.value?.t0 ?? 0, total)),
    '--f': String(msToFrac(shown.value?.t0 ?? 0, total)),
  });

  const send = (r: IVideoRange) => {
    const span = spanOfRange(scenes, r);
    if (!span) return;
    onSendToChat(
      formatVideoAnnotation({
        n: 1,
        project: id,
        scene: span.scene,
        sceneNo: span.index + 1,
        format,
        t0: span.t0,
        t1: span.t1,
        block: '',
        html: '',
        frame: '',
        comment: '',
      }),
    );
    onRange(null);
  };

  const showHover = hover !== null && dragging === null && Math.abs(hover - time) > total / 200;

  return (
    <section className="hx-video-tl" aria-label={t('next.video.timeline.label')}>
      <div
        ref={track}
        className="hx-video-tl__track"
        role="slider"
        tabIndex={0}
        aria-label={t('next.video.timeline.ruler')}
        aria-valuemin={0}
        aria-valuemax={total}
        aria-valuenow={time}
        aria-valuetext={clock(time)}
        data-dragging={dragging ?? '0'}
        onPointerDown={onDown}
        onPointerMove={onMove}
        onPointerLeave={() => setHover(null)}
        onPointerUp={onUp}
        onPointerCancel={onUp}
        onKeyDown={onKey}
      >
        <div className="hx-video-tl__row" data-row="ruler" aria-hidden="true">
          {ticks.map((tick) => (
            <Tick key={tick.ms} left={tick.left} major={tick.major} label={tickLabel(tick.ms)} />
          ))}
        </div>
        <div className="hx-video-tl__row" data-row="scenes">
          {tiles.map((box) => (
            <SceneTile
              key={box.id}
              scene={scenes[box.index]}
              index={box.index}
              left={box.left}
              width={box.width}
              on={box.index === current}
            />
          ))}
        </div>
        {dots.length > 0 && (
          <div className="hx-video-tl__row" data-row="moments">
            {dots.map((dot) => (
              <Placed
                key={dot.key}
                cls="hx-video-tl__dot"
                left={dot.left}
                title={dot.block}
                data-derived={dot.derived ? '1' : '0'}
              />
            ))}
          </div>
        )}
        {scenes[current] !== undefined && scenes[current].overlays.length > 0 && (
          <OverlayStrips
            editor={overlays}
            scene={scenes[current]}
            total={total}
            track={track}
            onSeek={onSeek}
          />
        )}
        <div className="hx-video-tl__row" data-row="comments">
          {comments.items.map((item) => {
            if (!item.time) return null;
            const scene = scenes[item.scene];
            const start = scene.start + item.time.t0;
            const box = markerBox(scene, clampAnchor(scene, item.time.t0, item.time.t1), total);
            return (
              <PlacedButton
                key={item.key}
                cls="hx-video-tl__mark"
                left={box.left}
                width={box.width}
                label={String(item.n)}
                data-done={item.resolved ? '1' : '0'}
                data-changed={item.time.changed ? '1' : '0'}
                data-range={box.width > 0 ? '1' : '0'}
                onClick={() => onSeek(start)}
              />
            );
          })}
        </div>
        {range && (
          <Placed
            cls="hx-video-tl__range"
            left={msToFrac(range.t0, total)}
            width={msToFrac(range.t1 - range.t0, total)}
          />
        )}
        {showHover && <Flag cls="hx-video-tl__hover" left={msToFrac(hover, total)} text={clock(hover)} />}
        <Flag cls="hx-video-tl__head" left={msToFrac(time, total)} text={clock(time)} />
      </div>
      {!shown.value && <p className="hx-video-tl__hint">{t('next.video.timeline.hint')}</p>}

      {shown.value && (
        <div
          ref={actionsRef}
          className="hx-video-tl__actions"
          data-closing={shown.state === 'closing' ? '1' : '0'}
          role="toolbar"
          aria-label={spanText(shown.value.t0, shown.value.t1)}
        >
          <span className="hx-video-tl__span">{spanText(shown.value.t0, shown.value.t1)}</span>
          <button className="hx-btn" data-size="sm" type="button" onClick={() => onComment(shown.value!)}>
            {t('next.video.timeline.comment')}
          </button>
          <button className="hx-btn" data-size="sm" type="button" onClick={() => send(shown.value!)}>
            {t('next.video.timeline.sendToChat')}
          </button>
          <button
            className="hx-btn"
            data-size="sm"
            type="button"
            aria-pressed={loop}
            onClick={() => onLoop(!loop)}
          >
            {t('next.video.timeline.loop')}
          </button>
          <button
            className="hx-btn"
            data-variant="ghost"
            data-size="sm"
            data-shape="icon"
            type="button"
            aria-label={t('next.video.timeline.close')}
            onClick={() => onRange(null)}
          >
            <Icon name="x" size="sm" />
          </button>
        </div>
      )}
    </section>
  );
};

export default VideoTimeline;
