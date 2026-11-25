// the home's library is the machine's when it answers in time, else the hub's copy until it does.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  LIBRARY_FROM,
  readLibrary,
  type IDesign,
  type IDesignLibrary,
  type ILibraryView,
  type IStoreLibrary,
} from '@/next/features/designly/api';
import { parsePage, type MachineId } from '@/next/protocol/v2';

const design = (id: string): IDesign =>
  ({
    id,
    title: id,
    boards: [],
    convs: [],
    kind: 'site',
    pinned: false,
    cover: null,
    projectPath: '',
    updatedAt: 1,
  }) as IDesign;
const machineLib: IDesignLibrary = { port: 1, root: '/lib', designs: [design('live')] };
const storeLib: IStoreLibrary = {
  designs: [design('copy')],
  layout: { folders: [], placed: {}, systems: {} },
};

let n = 0;
const m = () => `m${++n}` as MachineId;

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe('readLibrary', () => {
  it('decodes grant metadata from the wire without requiring it from older runtimes', () => {
    const entitled = {
      ...design('allowed'),
      role: 'view',
      accessSource: 'person',
      accessUntil: '2030-01-01T13:14:00Z',
    };
    expect(
      parsePage.designlyLibrary({ root: '/lib', port: 0, artifacts: [entitled, design('legacy')] }).artifacts,
    ).toEqual([entitled, design('legacy')]);
  });

  it('shows a fast machine immediately and checks the Hub for shared projects', async () => {
    const store = vi.fn(async () => storeLib);
    const shown: ILibraryView[] = [];
    await readLibrary(m(), { machine: async () => machineLib, store }, (v) => shown.push(v));
    expect(shown[0].from).toBe(LIBRARY_FROM.machine);
    expect(store).toHaveBeenCalledOnce();
  });

  it('retains a shared project from the Hub after a slow machine arrives and uses its authoritative role', async () => {
    const shared = { ...design('team-design'), accessSource: 'team', role: 'view' } as const;
    const link = {
      ...design('linked'),
      accessSource: 'link',
      role: 'comment',
      accessUntil: '2030-11-05T22:00:00Z',
    } as const;
    let answer!: (l: IDesignLibrary) => void;
    const shown: ILibraryView[] = [];
    const done = readLibrary(
      m(),
      {
        machine: () =>
          new Promise((resolve) => {
            answer = resolve;
          }),
        store: async () => ({ ...storeLib, designs: [shared, link] }),
      },
      (v) => shown.push(v),
    );
    await vi.advanceTimersByTimeAsync(1500);
    await done;
    answer({
      ...machineLib,
      designs: [...machineLib.designs, { ...shared, role: 'edit' }, { ...shared, id: 'revoked' }],
    });
    await vi.advanceTimersByTimeAsync(0);
    expect(shown.at(-1)?.designs).toEqual([...machineLib.designs, shared, link]);
  });

  it('shows the hub copy at once when the machine answers 504', async () => {
    const shown: ILibraryView[] = [];
    await readLibrary(
      m(),
      { machine: () => Promise.reject(new Error('504')), store: async () => storeLib },
      (v) => shown.push(v),
    );
    expect(shown).toHaveLength(1);
    expect(shown[0].from).toBe(LIBRARY_FROM.store);
    expect(shown[0].designs[0].id).toBe('copy');
  });

  it('shows the hub copy after the wait, then the machine when it comes', async () => {
    let answer: (l: IDesignLibrary) => void = () => undefined;
    const shown: ILibraryView[] = [];
    const done = readLibrary(
      m(),
      { machine: () => new Promise<IDesignLibrary>((ok) => (answer = ok)), store: async () => storeLib },
      (v) => shown.push(v),
    );
    await vi.advanceTimersByTimeAsync(1500);
    await done;
    expect(shown.map((v) => v.from)).toEqual([LIBRARY_FROM.store]);
    answer(machineLib);
    await vi.advanceTimersByTimeAsync(0);
    expect(shown.map((v) => v.from)).toEqual([LIBRARY_FROM.store, LIBRARY_FROM.machine]);
  });

  it('fails only when the hub fails too', async () => {
    const shown: ILibraryView[] = [];
    await expect(
      readLibrary(
        m(),
        { machine: () => Promise.reject(new Error('504')), store: () => Promise.reject(new Error('404')) },
        (v) => shown.push(v),
      ),
    ).rejects.toThrow('504');
    expect(shown).toHaveLength(0);
  });

  // GitHub #179: a slug held twice is two cards, each named by its holder; neither takes the other's actions.
  it('keeps a shared copy of a local slug as a card of its own, keyed by its holder', async () => {
    const mine = { ...design('site'), role: 'owner', accessSource: 'owner' } as const;
    const theirs = { ...design('site'), cardKey: 'aaaa', accessSource: 'person', role: 'view' } as const;
    const other = { ...design('site'), cardKey: 'bbbb', accessSource: 'team', role: 'view' } as const;
    const shown: ILibraryView[] = [];
    await readLibrary(
      m(),
      {
        machine: async () => ({
          ...machineLib,
          designs: [{ ...mine, boards: [{ path: 'a.html' }] } as IDesign],
        }),
        store: async () => ({ ...storeLib, designs: [theirs as IDesign, other as IDesign] }),
      },
      (v) => shown.push(v),
    );
    await vi.advanceTimersByTimeAsync(0);
    const cards = shown[shown.length - 1].designs;
    expect(cards.map((d) => [d.id, d.cardKey ?? null])).toEqual([
      ['site', null],
      ['site', 'aaaa'],
      ['site', 'bbbb'],
    ]);
    // The local card keeps its own grant and boards: the shared copies did not touch it.
    expect(cards[0].accessSource).toBe('owner');
    expect(cards[0].boards).toHaveLength(1);
    expect(cards[1].role).toBe('view');
  });

  it('drops a shared copy the hub no longer lists (revoked or expired) and never trusts one without the hub', async () => {
    const theirs = { ...design('site'), cardKey: 'aaaa', accessSource: 'person', role: 'view' } as IDesign;
    const kept: ILibraryView[] = [];
    await readLibrary(
      m(),
      { machine: async () => machineLib, store: async () => ({ ...storeLib, designs: [theirs] }) },
      (v) => kept.push(v),
    );
    await vi.advanceTimersByTimeAsync(0);
    expect(kept[kept.length - 1].designs.some((d) => d.cardKey === 'aaaa')).toBe(true);
    const gone: ILibraryView[] = [];
    await readLibrary(
      m(),
      { machine: async () => machineLib, store: async () => ({ ...storeLib, designs: [] }) },
      (v) => gone.push(v),
    );
    await vi.advanceTimersByTimeAsync(0);
    expect(gone[gone.length - 1].designs.some((d) => d.cardKey)).toBe(false);
    const failed: ILibraryView[] = [];
    await readLibrary(
      m(),
      {
        machine: async () => ({ ...machineLib, designs: [theirs] }),
        store: async () => {
          throw new Error('502');
        },
      },
      (v) => failed.push(v),
    );
    await vi.advanceTimersByTimeAsync(0);
    expect(failed[failed.length - 1].designs.some((d) => d.cardKey)).toBe(false);
  });
});
