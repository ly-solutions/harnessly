// Where the note's card stands over the board: under its block, above it where there is no room under, always
// inside the board and never over the block while either side has room. It follows the block's frame on the page, the
// canvas's zoom (--zs/--zcx/--zcy of the section) and its own height; once the person has moved it, it is theirs
// (site.css, data-moved).
import { useLayoutEffect, type RefObject } from 'react';

export interface IPlaceAt {
  /** The block's bottom-left corner and its top, in the section's pixels with the canvas at its fit. */
  x: number;
  y: number;
  top: number;
  /** The card opens to the block's left. */
  right: boolean;
}

export interface IBounds {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

const GAP = 10;
const EDGE = 8;
/** A card is never squeezed under this: a block that fills the board leaves it no room, and it lies over it. */
export const MIN_ROOM = 150;

const clamp = (v: number, min: number, max: number) => Math.max(min, Math.min(Math.max(min, max), v));

export interface IPlaced {
  x: number;
  y: number;
  /** The most the card may be high there. */
  max: number;
  above: boolean;
}

/** The card's place for a block shown at `block` (the section's pixels), inside `board`. */
export const placeCard = (
  block: { x: number; top: number; bottom: number },
  card: { width: number; height: number },
  board: IBounds,
  right: boolean,
): IPlaced => {
  const below = board.bottom - block.bottom - GAP - EDGE;
  const over = block.top - board.top - GAP - EDGE;
  const above = card.height > below && over > below;
  const max = Math.max(MIN_ROOM, above ? over : below);
  const height = Math.min(card.height, max);
  const y = above ? block.top - GAP - height : block.bottom + GAP;
  return {
    x: clamp(right ? block.x - card.width : block.x, board.left + EDGE, board.right - card.width - EDGE),
    y: clamp(y, board.top + EDGE, board.bottom - height - EDGE),
    max,
    above,
  };
};

export const useCardPlace = (ref: RefObject<HTMLElement | null>, { x, y, top, right }: IPlaceAt): void => {
  useLayoutEffect(() => {
    const el = ref.current;
    const section = el?.closest<HTMLElement>('.hx-site');
    if (!el || !section) return;
    const area = section.querySelector<HTMLElement>('.hx-browser__page');
    const body = el.firstElementChild instanceof HTMLElement ? el.firstElementChild : el;
    const pins = section.querySelector<HTMLElement>('.hx-site__pins');
    const place = () => {
      const read = (name: string, none: number) => {
        const value = Number.parseFloat(section.style.getPropertyValue(name));
        return Number.isFinite(value) ? value : none;
      };
      const k = read('--zs', 1);
      const dx = read('--zcx', 0);
      const dy = read('--zcy', 0);
      const home = section.getBoundingClientRect();
      const box = (area ?? section).getBoundingClientRect();
      // Where the block shows now (its frame follows it as the page scrolls and the words grow), else where it was
      // picked.
      const frame = section.querySelector<HTMLElement>(
        '.hx-lasso[data-on="true"]:not([data-place="hidden"]), .hx-npin[data-open="true"] .hx-npin__frame',
      );
      const shown = frame?.getBoundingClientRect();
      const block =
        shown && shown.height > 0
          ? {
              x: (right ? shown.right : shown.left) - home.left,
              top: shown.top - home.top,
              bottom: shown.bottom - home.top,
            }
          : { x: k * x + dx, top: k * top + dy, bottom: k * y + dy };
      const at = placeCard(
        block,
        // The whole card, whatever it is squeezed to now: what its body holds and its border.
        { width: el.offsetWidth, height: body.scrollHeight + el.offsetHeight - el.clientHeight },
        {
          left: box.left - home.left,
          top: box.top - home.top,
          right: box.right - home.left,
          bottom: box.bottom - home.top,
        },
        right,
      );
      el.style.setProperty('--px', `${at.x}px`);
      el.style.setProperty('--py', `${at.y}px`);
      el.style.setProperty('--nmax', `${at.max}px`);
      el.dataset.above = String(at.above);
      // The body scrolls only when it must: a scroller would cut the tips that hang out of the card.
      el.dataset.scroll = String(body.scrollHeight + el.offsetHeight - el.clientHeight > at.max + 1);
    };
    place();
    const sizes = new ResizeObserver(place);
    for (const node of [el, body, section, area]) if (node) sizes.observe(node);
    // The canvas writes its zoom onto the section's style.
    const zoom = new MutationObserver(place);
    zoom.observe(section, { attributes: true, attributeFilter: ['style'] });
    // The frames of the blocks are written where the page says they are.
    if (pins)
      zoom.observe(pins, {
        subtree: true,
        childList: true,
        attributes: true,
        attributeFilter: ['style', 'data-on', 'data-place', 'data-open'],
      });
    return () => {
      sizes.disconnect();
      zoom.disconnect();
    };
  }, [ref, x, y, top, right]);
};
