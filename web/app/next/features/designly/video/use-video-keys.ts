// The video player's keys. Space or K plays and pauses; ←/→ jump 5 s (Shift: 10 s), J/L jump 10 s, «,» and «.» step a
// frame; PageUp/PageDown or ↑/↓ change the scene; Home/End go to the ends; 0–9 go to a tenth of the video; I and O mark
// the range; C comments; F goes full screen. Russian layout letters do the same (л, о, д, ш, щ, с, а, and б, ю on the
// comma and period keys). Keys typed into a field or into editable text are the field's.
import { useEffect, useEffectEvent } from 'react';
import { isTypingTarget } from '@/next/features/designly/deck/use-deck-keys';

export const VIDEO_KEY = {
  toggle: 'toggle',
  back5: 'back5',
  forward5: 'forward5',
  back10: 'back10',
  forward10: 'forward10',
  frameBack: 'frameBack',
  frameForward: 'frameForward',
  prevScene: 'prevScene',
  nextScene: 'nextScene',
  start: 'start',
  end: 'end',
  tenth: 'tenth',
  markIn: 'markIn',
  markOut: 'markOut',
  comment: 'comment',
  full: 'full',
} as const;
type TPlainKey = Exclude<(typeof VIDEO_KEY)[keyof typeof VIDEO_KEY], 'tenth'>;

export type TVideoKeyAction = { key: TPlainKey } | { key: typeof VIDEO_KEY.tenth; n: number };

/** Layout-independent letters: the Latin key and the Russian one on the same place. */
const LETTERS: Record<string, TPlainKey> = {
  k: VIDEO_KEY.toggle,
  л: VIDEO_KEY.toggle,
  j: VIDEO_KEY.back10,
  о: VIDEO_KEY.back10,
  l: VIDEO_KEY.forward10,
  д: VIDEO_KEY.forward10,
  i: VIDEO_KEY.markIn,
  ш: VIDEO_KEY.markIn,
  o: VIDEO_KEY.markOut,
  щ: VIDEO_KEY.markOut,
  c: VIDEO_KEY.comment,
  с: VIDEO_KEY.comment,
  f: VIDEO_KEY.full,
  а: VIDEO_KEY.full,
  ',': VIDEO_KEY.frameBack,
  б: VIDEO_KEY.frameBack,
  '.': VIDEO_KEY.frameForward,
  ю: VIDEO_KEY.frameForward,
};

const NAMED: Record<string, TPlainKey> = {
  ' ': VIDEO_KEY.toggle,
  PageUp: VIDEO_KEY.prevScene,
  ArrowUp: VIDEO_KEY.prevScene,
  PageDown: VIDEO_KEY.nextScene,
  ArrowDown: VIDEO_KEY.nextScene,
  Home: VIDEO_KEY.start,
  End: VIDEO_KEY.end,
};

/** What a key does; null when it does nothing. */
export const videoKeyAction = (key: string, shift = false): TVideoKeyAction | null => {
  if (key === 'ArrowLeft') return { key: shift ? VIDEO_KEY.back10 : VIDEO_KEY.back5 };
  if (key === 'ArrowRight') return { key: shift ? VIDEO_KEY.forward10 : VIDEO_KEY.forward5 };
  if (/^[0-9]$/.test(key)) return { key: VIDEO_KEY.tenth, n: Number(key) };
  const named = Object.hasOwn(NAMED, key) ? NAMED[key] : undefined;
  if (named !== undefined) return { key: named };
  const letter = key.toLowerCase();
  const plain = Object.hasOwn(LETTERS, letter) ? LETTERS[letter] : undefined;
  return plain === undefined ? null : { key: plain };
};

/**
 * Listens to the window's keys while `on` and calls `onKey` with the action; the keys it acts on do nothing else (no
 * scroll on an arrow or Space). A key with a modifier (but Shift) is the browser's. Space on a focused button is the
 * button's.
 */
export const useVideoKeys = (on: boolean, onKey: (action: TVideoKeyAction) => void) => {
  const act = useEffectEvent(onKey);
  useEffect(() => {
    if (!on) return;
    const down = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return;
      if (isTypingTarget(event.target)) return;
      if (event.key === ' ' && event.target instanceof HTMLButtonElement) return;
      const action = videoKeyAction(event.key, event.shiftKey);
      if (action === null) return;
      event.preventDefault();
      act(action);
    };
    window.addEventListener('keydown', down);
    return () => window.removeEventListener('keydown', down);
  }, [on]);
};
