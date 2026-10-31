// Which preview slot a browser tab uses. A slot is one cookie, so two tabs on one slot take each other's page over.
// Every tab holds a slot of its own (ports 1–6, sites 7–8); when all are held, the one used least recently is taken
// from its tab, which asks again when it is looked at.

export type TSlotKind = 'port' | 'site';

export const SLOTS: Readonly<Record<TSlotKind, readonly number[]>> = {
  port: [1, 2, 3, 4, 5, 6],
  site: [7, 8],
};

interface IHold {
  tab: string;
  kind: TSlotKind;
  at: number;
}

export interface ISlotLeases {
  /** The tab's slot for this kind of page: the one it holds, a free one, or the least recently used one of another tab. */
  lease: (kind: TSlotKind, tab: string) => number;
  /** The slot the tab holds now; null when it holds none (never asked, or it was taken). */
  slotOf: (tab: string) => number | null;
  /** Lets go of the tab's slot (a closed tab). */
  release: (tab: string) => void;
  /** Called with the tab that lost its slot to another. Returns the unsubscribe. */
  onTaken: (listener: (tab: string) => void) => () => void;
}

export const createSlotLeases = (clock: () => number = Date.now): ISlotLeases => {
  const holds = new Map<number, IHold>();
  const listeners = new Set<(tab: string) => void>();
  // A counter besides the clock: two leases in one millisecond still have an order.
  let tick = 0;
  const stamp = () => clock() * 1000 + (tick = (tick + 1) % 1000);

  const heldBy = (tab: string): [number, IHold] | undefined =>
    [...holds.entries()].find(([, hold]) => hold.tab === tab);

  const lease: ISlotLeases['lease'] = (kind, tab) => {
    const mine = heldBy(tab);
    if (mine && mine[1].kind === kind) {
      mine[1].at = stamp();
      return mine[0];
    }
    // The address moved from a port to a site (or back): the old slot is of no use to this tab.
    if (mine) holds.delete(mine[0]);
    const pool = SLOTS[kind];
    const free = pool.find((slot) => !holds.has(slot));
    if (free !== undefined) {
      holds.set(free, { tab, kind, at: stamp() });
      return free;
    }
    const oldest = pool
      .map((slot) => [slot, holds.get(slot)] as const)
      .reduce((best, now) => (now[1] && best[1] && now[1].at < best[1].at ? now : best));
    const [slot, taken] = oldest;
    holds.set(slot, { tab, kind, at: stamp() });
    if (taken) for (const listener of [...listeners]) listener(taken.tab);
    return slot;
  };

  return {
    lease,
    slotOf: (tab) => heldBy(tab)?.[0] ?? null,
    release: (tab) => {
      const mine = heldBy(tab);
      if (mine) holds.delete(mine[0]);
    },
    onTaken: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
};

/** The leases of this window's tabs: every browser tab asks here. */
export const slotLeases = createSlotLeases();
