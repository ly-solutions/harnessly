// Which slides of a deck are kept loaded out of sight, and in what order they load. A slot by path (a hub's
// machine) frames its boards on an opaque origin, which the browser gives no HTTP cache: a board loads its engine, kit,
// fonts and pictures anew in every frame, so a frame once loaded is kept rather than dropped and loaded again. After
// the slide on screen is drawn its neighbours load, then the rest outward from it, a few at a time, so that a turn to
// any slide shows a frame that is already drawn. No React here.

/** How many frames load out of sight at once: they share the link and the processor with the slide on screen. */
export const DECK_WARM_PARALLEL = 2;

/** The most frames kept loaded besides the slide on screen; the farthest from it go first. Every frame is a document
 *  of its own with the board's engine. */
export const DECK_KEEP_MAX = 16;

/** The same on a phone, whose memory is short. */
export const DECK_KEEP_PHONE = 4;

/**
 * The slides other than `index` in the order they are loaded: the next, the previous, then outward, the next side
 * first at each distance.
 */
export const warmOrder = (count: number, index: number): number[] => {
  const order: number[] = [];
  for (let d = 1; d < count; d++) {
    if (index + d < count) order.push(index + d);
    if (index - d >= 0) order.push(index - d);
  }
  return order;
};

/** A frame of the stage as the warming reads it. */
export interface IWarmFrame {
  src: string;
  loaded: boolean;
}

/**
 * The addresses to put in the page now, in `ahead`'s order: those without a frame, as many as leaves at most `parallel`
 * frames out of sight still loading. The slide on screen is never one of them.
 */
export const nextToWarm = (
  ahead: readonly string[],
  frames: readonly IWarmFrame[],
  onScreen: string | null,
  parallel = DECK_WARM_PARALLEL,
): string[] => {
  const loading = frames.filter((f) => f.src !== onScreen && !f.loaded).length;
  const room = parallel - loading;
  if (room <= 0) return [];
  return ahead.filter((a) => a !== onScreen && !frames.some((f) => f.src === a)).slice(0, room);
};
