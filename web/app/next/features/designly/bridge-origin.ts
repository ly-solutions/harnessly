/**
 * Whether a message came from the board's frame. The owner's board has the slot's origin. A guest's board is served
 * under a CSP sandbox and has an opaque origin: its messages say `null`, and only the frame's own window with exactly
 * that origin is believed (`*` is how the host addresses it, never how a message names itself).
 */
export const GUEST_ORIGIN = '*';
const OPAQUE = 'null';

/**
 * A slot by path (`<appBase>/__pv/<slot>-<key>/…`, the preview where a deployment has one host) serves the
 * board under a CSP sandbox without allow-same-origin: like a guest's, it is opaque, its messages say `null`.
 */
const PATH_SLOT = /\/__pv\/[^/]+\//;

/**
 * A board read from the hub's store (storeBoardHref): `<…>/design/store/<project>/f/<board>` on this page's
 * origin, served under a CSP sandbox like a guest's, so it is opaque too.
 */
const STORE_BOARD = /\/design\/store\/[^/]+\/f\//;

/**
 * Where a message to the board in a frame showing `src` goes, and the origin its answers are checked against (through
 * fromFrame): the frame's own origin, or GUEST_ORIGIN for a board on an opaque origin (a slot by path). null for an
 * address that is none.
 */
export const frameOrigin = (src: string): string | null => {
  try {
    const url = new URL(src, window.location.href);
    if (PATH_SLOT.test(url.pathname)) return GUEST_ORIGIN;
    if (url.origin === window.location.origin && STORE_BOARD.test(url.pathname)) return GUEST_ORIGIN;
    return url.origin;
  } catch {
    return null;
  }
};

/** Where an answer to a message goes: back to its origin, or to the frame's window itself when that is opaque. */
export const replyOrigin = (origin: string): string => (origin === OPAQUE ? GUEST_ORIGIN : origin);

export const fromFrame = (
  event: { source: unknown; origin: string },
  frame: unknown,
  origin: string | null,
): boolean => {
  if (origin === null || frame === null || frame === undefined || event.source !== frame) return false;
  return event.origin === (origin === GUEST_ORIGIN ? OPAQUE : origin);
};
