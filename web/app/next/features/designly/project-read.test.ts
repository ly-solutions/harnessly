import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { IDesign, IDesignLibrary, IStoreLibrary } from '@/next/features/designly/api';
import { WordedError } from '@/next/core/net/worded';
import {
  AWAY,
  awayIsFinal,
  MACHINE_SAID,
  PROJECT_FROM,
  PROJECT_READ,
  readProject,
  type TProjectRead,
  createProjectReads,
} from '@/next/features/designly/project-read';

vi.mock('@/next/core/net/runtime-json', () => ({ runtimeJson: vi.fn(), postJson: vi.fn() }));

const design = (id: string) => ({ id }) as unknown as IDesign;
const library = (...ids: string[]): IDesignLibrary => ({
  port: 4400,
  root: '/lib',
  designs: ids.map(design),
});
const copy = (...ids: string[]): IStoreLibrary => ({
  designs: ids.map(design),
  layout: { folders: [], placed: {}, systems: {} },
});
const WAITS = { store: 2500, machine: 20_000 };
const after = <T>(ms: number, value: T) => new Promise<T>((resolve) => setTimeout(() => resolve(value), ms));
const failAfter = (ms: number, message: string) =>
  new Promise<never>((_, reject) => setTimeout(() => reject(new Error(message)), ms));
const never = <T>() => new Promise<T>(() => {});

const run = (machine: () => Promise<IDesignLibrary>, store: () => Promise<IStoreLibrary>) => {
  const shown: TProjectRead[] = [];
  const storeRead = vi.fn(store);
  void readProject('p1', { machine, store: storeRead }, (state) => shown.push(state), WAITS);
  return { shown, storeRead };
};

describe('readProject of a link with no card key (HRN-1779)', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  const shared = (...keys: string[]): IStoreLibrary => ({
    designs: keys.map((cardKey) => ({ id: 'p1', cardKey, accessSource: 'person' }) as unknown as IDesign),
    layout: { folders: [], placed: {}, systems: {} },
  });
  const keyless = (machine: () => Promise<IDesignLibrary>, store: IStoreLibrary) => {
    const shown: TProjectRead[] = [];
    const found = vi.fn();
    void readProject(
      'p1',
      { machine, store: () => after(100, store) },
      (state) => shown.push(state),
      WAITS,
      undefined,
      found,
    );
    return { shown, found };
  };

  it('takes the one shared copy once the machine says it has not got the project', async () => {
    const { shown, found } = keyless(() => after(4000, library('other')), shared('k1'));
    await vi.advanceTimersByTimeAsync(3500);
    expect(found).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1000);
    expect(found).toHaveBeenCalledWith('k1');
    expect(shown).toEqual([]);
  });

  it('leaves the machine’s own project of the same id alone', async () => {
    const { shown, found } = keyless(() => after(4000, library('p1')), shared('k1'));
    await vi.advanceTimersByTimeAsync(5000);
    expect(found).not.toHaveBeenCalled();
    expect(shown[0]).toMatchObject({ from: PROJECT_FROM.machine });
  });

  it('does not guess between two shared copies', async () => {
    const { shown, found } = keyless(() => after(100, library('other')), shared('k1', 'k2'));
    await vi.advanceTimersByTimeAsync(5000);
    expect(found).not.toHaveBeenCalled();
    expect(shown[0]).toMatchObject({ k: PROJECT_READ.away });
  });
});

describe('readProject (HRN-1308)', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('the store has no copy and the machine is slow but on the link: the project opens from the machine', async () => {
    const { shown } = run(
      () => after(6000, library('p1')),
      () => after(100, copy('other')),
    );
    await vi.advanceTimersByTimeAsync(5000);
    // The store's «no copy» is not the end: the machine is still waited for, and no card is shown.
    expect(shown).toEqual([]);
    await vi.advanceTimersByTimeAsync(1500);
    expect(shown).toHaveLength(1);
    expect(shown[0]).toMatchObject({ k: PROJECT_READ.open, from: PROJECT_FROM.machine });
  });

  it('the store cannot be read and the machine is on the link: the project opens from the machine', async () => {
    const { shown } = run(
      () => after(4000, library('p1')),
      () => failAfter(50, 'store 500'),
    );
    await vi.advanceTimersByTimeAsync(3500);
    expect(shown).toEqual([]);
    await vi.advanceTimersByTimeAsync(1000);
    expect(shown).toHaveLength(1);
    expect(shown[0]).toMatchObject({ k: PROJECT_READ.open, from: PROJECT_FROM.machine });
  });

  it('a machine that answers at once is not followed by a read of the store', async () => {
    const { shown, storeRead } = run(
      () => after(100, library('p1')),
      () => after(10, copy('p1')),
    );
    await vi.advanceTimersByTimeAsync(5000);
    expect(storeRead).not.toHaveBeenCalled();
    expect(shown.map((s) => s.k)).toEqual([PROJECT_READ.open]);
  });

  it('an ACL card alone leaves the project to the live reader', async () => {
    const { shown } = run(
      () => after(100, library()),
      () => after(100, { ...copy('p1'), metadataOnly: true }),
    );
    await vi.advanceTimersByTimeAsync(300);
    expect(shown).toEqual([{ k: PROJECT_READ.away, why: AWAY.absentUnreadable, machineError: null }]);
  });

  it('a machine without the project sends the read to the store at once', async () => {
    const { shown } = run(
      () => after(100, library('other')),
      () => after(100, copy('p1')),
    );
    await vi.advanceTimersByTimeAsync(300);
    expect(shown).toHaveLength(1);
    expect(shown[0]).toMatchObject({ k: PROJECT_READ.open, from: PROJECT_FROM.store });
  });

  it('the copy is shown while the machine is slow, and the machine replaces it when it answers', async () => {
    const { shown } = run(
      () => after(9000, library('p1')),
      () => after(100, copy('p1')),
    );
    await vi.advanceTimersByTimeAsync(3000);
    expect(shown.map((s) => (s.k === PROJECT_READ.open ? s.from : s.k))).toEqual([PROJECT_FROM.store]);
    await vi.advanceTimersByTimeAsync(7000);
    expect(shown.map((s) => (s.k === PROJECT_READ.open ? s.from : s.k))).toEqual([
      PROJECT_FROM.store,
      PROJECT_FROM.machine,
    ]);
  });

  it('no copy and a machine that is off: away, «the machine does not answer, no copy»', async () => {
    const { shown } = run(
      () => failAfter(200, 'machine offline'),
      () => after(100, copy()),
    );
    await vi.advanceTimersByTimeAsync(500);
    expect(shown).toEqual([
      { k: PROJECT_READ.away, why: AWAY.silentNoCopy, machineError: 'machine offline' },
    ]);
  });

  it('no copy and a machine that hangs: away only after the machine was waited for, and it still opens later', async () => {
    let answer: (l: IDesignLibrary) => void = () => {};
    const { shown } = run(
      () => new Promise<IDesignLibrary>((resolve) => (answer = resolve)),
      () => after(100, copy()),
    );
    await vi.advanceTimersByTimeAsync(19_000);
    expect(shown).toEqual([]);
    await vi.advanceTimersByTimeAsync(1500);
    expect(shown).toEqual([{ k: PROJECT_READ.away, why: AWAY.silentNoCopy, machineError: null }]);
    answer(library('p1'));
    await vi.advanceTimersByTimeAsync(10);
    expect(shown[1]).toMatchObject({ k: PROJECT_READ.open, from: PROJECT_FROM.machine });
  });

  it('names each reason: what the machine said and what the store said', async () => {
    const cases = [
      { machine: () => failAfter(10, 'off'), store: () => failAfter(10, 'down'), why: AWAY.silentUnreadable },
      { machine: () => after(10, library()), store: () => after(10, copy()), why: AWAY.absentNoCopy },
      { machine: () => after(10, library()), store: () => failAfter(10, 'down'), why: AWAY.absentUnreadable },
      {
        machine: () => never<IDesignLibrary>(),
        store: () => failAfter(10, 'down'),
        why: AWAY.silentUnreadable,
      },
    ];
    for (const c of cases) {
      const { shown } = run(c.machine, c.store);
      await vi.advanceTimersByTimeAsync(21_000);
      expect(shown.at(-1)).toMatchObject({ k: PROJECT_READ.away, why: c.why });
    }
  });

  // GitHub #82: a machine closed to the person is not «on its way».
  it('names a machine that refuses the person', async () => {
    const storeFails = () =>
      new Promise<never>((_, reject) => setTimeout(() => reject(new Error('down')), 10));
    const denied = () =>
      new Promise<never>((_, reject) =>
        setTimeout(() => reject(new WordedError('Forbidden', 'http', null, 403)), 10),
      );
    const cases = [
      { machine: denied, store: () => after(10, copy()), why: AWAY.machineDenied },
      { machine: denied, store: storeFails, why: AWAY.machineDenied },
    ];
    for (const c of cases) {
      const { shown } = run(c.machine, c.store);
      await vi.advanceTimersByTimeAsync(WAITS.machine + 100);
      expect(shown.map((s) => (s.k === PROJECT_READ.away ? s.why : s.k))).toEqual([c.why]);
    }
    expect(awayIsFinal(AWAY.machineDenied)).toBe(true);
    expect(awayIsFinal(AWAY.absentNoCopy)).toBe(false);
  });

  it('a machine that refuses the person still opens the project from a store that has it', async () => {
    const denied = () => Promise.reject(new WordedError('Forbidden', 'http', null, 403));
    const { shown } = run(denied, () => after(10, copy('p1')));
    await vi.advanceTimersByTimeAsync(100);
    expect(shown).toEqual([
      { k: PROJECT_READ.open, from: PROJECT_FROM.store, design: design('p1'), machine: MACHINE_SAID.down },
    ]);
  });
});

// GitHub #193: an editor sees the hub's copy until the project is on their machine. The copy says what the machine
// said, so the page tells «on its way» from «your machine is off» and never sends an editor to ask for access.
describe('readProject: the machine’s word with the store’s copy (GitHub #193)', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());
  const saids = (shown: TProjectRead[]) =>
    shown.map((s) => (s.k === PROJECT_READ.open && s.from === PROJECT_FROM.store ? s.machine : s.k));

  it('a machine without the project yet: the copy says it is on its way', async () => {
    const { shown } = run(
      () => after(100, library('other')),
      () => after(100, copy('p1')),
    );
    await vi.advanceTimersByTimeAsync(300);
    expect(saids(shown)).toEqual([MACHINE_SAID.absent]);
  });

  it('a machine that is off: the copy says so', async () => {
    const { shown } = run(
      () => failAfter(100, 'machine offline'),
      () => after(100, copy('p1')),
    );
    await vi.advanceTimersByTimeAsync(300);
    expect(saids(shown)).toEqual([MACHINE_SAID.down]);
  });

  it('a slow machine: the copy first waits for it, then says what it answered', async () => {
    const { shown } = run(
      () => after(5000, library('other')),
      () => after(100, copy('p1')),
    );
    await vi.advanceTimersByTimeAsync(3000);
    expect(saids(shown)).toEqual([MACHINE_SAID.waiting]);
    await vi.advanceTimersByTimeAsync(3000);
    expect(saids(shown)).toEqual([MACHINE_SAID.waiting, MACHINE_SAID.absent]);
  });

  it('a machine that hangs: the copy says it is down once the machine was waited for', async () => {
    const { shown } = run(
      () => never<IDesignLibrary>(),
      () => after(100, copy('p1')),
    );
    await vi.advanceTimersByTimeAsync(WAITS.machine - 100);
    expect(saids(shown)).toEqual([MACHINE_SAID.waiting]);
    await vi.advanceTimersByTimeAsync(200);
    expect(saids(shown)).toEqual([MACHINE_SAID.waiting, MACHINE_SAID.down]);
  });

  it('a slow machine that brings the project: the copy is replaced, not said again', async () => {
    const { shown } = run(
      () => after(5000, library('p1')),
      () => after(100, copy('p1')),
    );
    await vi.advanceTimersByTimeAsync(WAITS.machine + 100);
    expect(shown.map((s) => (s.k === PROJECT_READ.open ? s.from : s.k))).toEqual([
      PROJECT_FROM.store,
      PROJECT_FROM.machine,
    ]);
  });
});

describe('readProject with a card (GitHub #179)', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());
  it("does not open the machine's own project of the same slug for a shared copy", async () => {
    const shown: unknown[] = [];
    void readProject(
      'p1',
      {
        machine: async () => library('p1'),
        store: async () => ({ ...copy(), designs: [{ id: 'p1', cardKey: 'aaaa' } as unknown as IDesign] }),
      },
      (state) => shown.push(state),
      WAITS,
      'aaaa',
    );
    await vi.advanceTimersByTimeAsync(0);
    expect(shown).toEqual([expect.objectContaining({ k: 'open', from: 'store' })]);
  });

  it('is away when the hub lists no copy under that card', async () => {
    const shown: unknown[] = [];
    void readProject(
      'p1',
      { machine: async () => library('p1'), store: async () => ({ ...copy(), designs: [design('p1')] }) },
      (state) => shown.push(state),
      WAITS,
      'aaaa',
    );
    await vi.advanceTimersByTimeAsync(0);
    expect(shown).toEqual([expect.objectContaining({ k: 'away' })]);
  });
});

describe('createProjectReads (HRN-1405)', () => {
  const later = () => {
    let done!: (v: string) => void;
    const p = new Promise<string>((resolve) => {
      done = resolve;
    });
    return { p, done };
  };

  it('a poll while the machine still answers leaves that read be and starts none', async () => {
    const reads = createProjectReads();
    const first = reads.start('m|harnessly|0|0');
    expect(first).not.toBeNull();
    const slow = later();
    const answer = reads.machine(() => slow.p);
    expect(reads.start('m|harnessly|0|0')).toBeNull();
    expect(first?.aborted).toBe(false);
    slow.done('library');
    await expect(answer).resolves.toBe('library');
    const next = reads.start('m|harnessly|0|0');
    expect(next).not.toBeNull();
    expect(first?.aborted).toBe(false);
  });

  it('a poll after the machine answered reads again without aborting the read before', () => {
    const reads = createProjectReads();
    const first = reads.start('k');
    const second = reads.start('k');
    expect(second).not.toBeNull();
    expect(first?.aborted).toBe(false);
  });

  it('another project or attempt aborts the reads on their way', () => {
    const reads = createProjectReads();
    const first = reads.start('m|a|0|0');
    void reads.machine(() => new Promise(() => undefined));
    const other = reads.start('m|b|0|0');
    expect(first?.aborted).toBe(true);
    expect(other).not.toBeNull();
  });

  it('stop aborts everything, and the same project starts again after it (a remount)', () => {
    const reads = createProjectReads();
    const first = reads.start('k');
    void reads.machine(() => new Promise(() => undefined));
    reads.stop();
    expect(first?.aborted).toBe(true);
    expect(reads.start('k')).not.toBeNull();
  });
});
