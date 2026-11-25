// The last Sinopia library each machine's home showed, from the machine or the hub's copy. It lives here, apart
// from the library's API, so the page's persistence can keep it across a reload (core/persist.ts, area "library") and
// the home draws its cards before the network answers, then reads them anew behind.

/** What is kept of a library view: the cards, where they came from, and the hub copy's shelves. */
export interface KeptLibraryView<D = unknown, L = unknown> {
  designs: D[];
  from: 'machine' | 'store';
  layout: L | null;
}

const views = new Map<string, KeptLibraryView>();
/** Views set by the page itself, not restored from what an earlier page kept. */
const live = new Set<string>();
type Listener = (machine: string, view: KeptLibraryView, restored: boolean) => void;
const listeners = new Set<Listener>();

export const libraryView = (machine: string): KeptLibraryView | null => views.get(machine) ?? null;

/** A view the page has just shown. */
export const putLibraryView = (machine: string, view: KeptLibraryView): void => {
  views.set(machine, view);
  live.add(machine);
  for (const l of listeners) l(machine, view, false);
};

/** A view an earlier page kept: taken only while the page has shown none for that machine. */
export const restoreLibraryView = (machine: string, view: KeptLibraryView): void => {
  if (live.has(machine) || !Array.isArray(view?.designs)) return;
  views.set(machine, view);
  for (const l of listeners) l(machine, view, true);
};

export const onLibraryView = (fn: Listener): (() => void) => {
  listeners.add(fn);
  return () => void listeners.delete(fn);
};
