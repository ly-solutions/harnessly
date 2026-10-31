// The stage of the live player: two frames, the scene on screen and the one that comes next, each at the format's own
// size and scaled to the slot, so a scene lays out as it will be rendered. A scene takes the frame the other one is not
// in, and at a boundary the frames swap: the next scene is already loaded and seeked to its start. A frame that is not
// in front stays in place, hidden. The slot's width is the stylesheet's; a ResizeObserver hands the scale to CSS as
// `--video-k`.
import { useLayoutEffect, useRef, type ReactNode, type RefObject } from 'react';
import { deckScale } from '@/next/features/designly/deck/layout';
import { VIDEO_SIZE, type TVideoFormat } from '@/next/features/designly/video/types';

export interface IVideoStageSlot {
  /** The scene's frame address; null while there is no scene for the frame. */
  src: string | null;
  title: string;
}

export type TVideoSlotIndex = 0 | 1;

export interface IVideoStageProps {
  format: TVideoFormat;
  slots: readonly [IVideoStageSlot, IVideoStageSlot];
  /** The frame that shows. */
  front: TVideoSlotIndex;
  frameRefs: readonly [RefObject<HTMLIFrameElement | null>, RefObject<HTMLIFrameElement | null>];
  /** The slide, for the pointer position. */
  slideRef: RefObject<HTMLDivElement | null>;
  /** Nothing is on screen yet. */
  loading: boolean;
  /** Laid over the scene: the subtitle, the pointer catcher, the failure. */
  children?: ReactNode;
}

const VideoStage = ({ format, slots, front, frameRefs, slideRef, loading, children }: IVideoStageProps) => {
  const slotRef = useRef<HTMLDivElement>(null);
  const { w, h } = VIDEO_SIZE[format];

  useLayoutEffect(() => {
    const slot = slotRef.current;
    if (!slot) return;
    slot.style.setProperty('--bw', String(w));
    slot.style.setProperty('--bh', String(h));
    const fit = () => slot.style.setProperty('--video-k', String(deckScale(slot.clientWidth, w)));
    fit();
    const watch = new ResizeObserver(fit);
    watch.observe(slot);
    return () => watch.disconnect();
  }, [w, h]);

  return (
    <div ref={slotRef} className="hx-video__slot" data-format={format}>
      <div ref={slideRef} className="hx-video__slide" data-loading={loading ? '1' : undefined}>
        {slots.map((slot, i) =>
          slot.src === null ? null : (
            <iframe
              key={i}
              ref={frameRefs[i]}
              className="hx-video__frame"
              data-on={front === i ? '1' : '0'}
              src={slot.src}
              title={slot.title}
              tabIndex={-1}
              aria-hidden={front === i ? undefined : true}
            />
          ),
        )}
        {loading && (
          <div className="hx-video__sk" aria-hidden="true">
            <span className="hx-sk" />
          </div>
        )}
        {children}
      </div>
    </div>
  );
};

export default VideoStage;
