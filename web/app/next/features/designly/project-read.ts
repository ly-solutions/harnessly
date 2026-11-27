// One project opened from wherever it can be had. The machine is asked at once; the hub's store is asked
// when the machine is slow, has failed or does not have the project. A store that cannot give the project (no copy,
// a failed read) never ends the wait for the machine: the project then opens from the machine and the page asks the
// machine to put it into the store (pushToStore), so the next look reads the hub. The card «not here» is the answer
// only when neither side gave the project, and it names why.
import { useEffect, useRef } from 'react';
import { platformPath } from '@/next/core/net/machine';
import { postJson, runtimeJson } from '@/next/core/net/runtime-json';
import { WordedError } from '@/next/core/net/worded';
import { type IDesign, type IDesignLibrary, type IStoreLibrary } from '@/next/features/designly/api';
import type { MachineId } from '@/next/protocol/v2';

export const PROJECT_FROM = { machine: 'machine', store: 'store' } as const;

/** Why the project is on neither side: what the machine said and what the store said. */
export const AWAY = {
  /** The machine did not answer (off, hung, an error) and the store has no copy. */
  silentNoCopy: 'silentNoCopy',
  /** The machine did not answer and the store could not be read. */
  silentUnreadable: 'silentUnreadable',
  /** The machine answered without the project and the store has no copy. */
  absentNoCopy: 'absentNoCopy',
  /** The machine answered without the project and the store could not be read. */
  absentUnreadable: 'absentUnreadable',
  /** The machine refused the person outright (403): the project is on a machine they may not open (GitHub #82). */
  machineDenied: 'machineDenied',
} as const;
export type TAway = (typeof AWAY)[keyof typeof AWAY];

/** Reasons nothing on this page can change: the card stays as it is and the library is not read again. */
export const awayIsFinal = (why: TAway | null): boolean => why === AWAY.machineDenied;

/** Reasons the machine is the one awaited: it has been waited for already, the next look leaves it the same time. */
export const awayIsSilent = (why: TAway | null): boolean =>
  why === AWAY.silentNoCopy || why === AWAY.silentUnreadable;

export const PROJECT_READ = { open: 'open', away: 'away' } as const;

/** What the person's machine has said by the time the store's copy is shown (GitHub #193): nothing yet, that it has
 *  not got the project (a role just given, its sync on the way), or nothing at all within its wait (off, hung, none). */
export const MACHINE_SAID = { waiting: 'waiting', absent: 'absent', down: 'down' } as const;
export type TMachineSaid = (typeof MACHINE_SAID)[keyof typeof MACHINE_SAID];

export type TProjectRead =
  | { k: typeof PROJECT_READ.open; from: typeof PROJECT_FROM.machine; library: IDesignLibrary }
  | { k: typeof PROJECT_READ.open; from: typeof PROJECT_FROM.store; design: IDesign; machine: TMachineSaid }
  | {
      k: typeof PROJECT_READ.away;
      why: TAway;
      /** The machine's own error when it failed outright, for a runtime with no hub behind it. */
      machineError: string | null;
    };

/** How long the machine is waited for after the store turned out to be of no help, from the start of the read, ms. */
export const MACHINE_PATIENCE_MS = 20_000;

const SIDE = {
  pending: 'pending',
  has: 'has',
  absent: 'absent',
  failed: 'failed',
  denied: 'denied',
} as const;
type TSide = (typeof SIDE)[keyof typeof SIDE];

const WHY: Record<'silent' | 'absent', Record<'absent' | 'failed', TAway>> = {
  silent: { absent: AWAY.silentNoCopy, failed: AWAY.silentUnreadable },
  absent: { absent: AWAY.absentNoCopy, failed: AWAY.absentUnreadable },
};

export interface IProjectReadWaits {
  /** The machine's head start before the store is asked too. */
  store: number;
  /** The longest the machine is waited for once the store has nothing. */
  machine: number;
}

/**
 * Reads project `id`. `show` gets every state worth showing, in order: the store's copy while the machine is slow, the
 * machine's project when it comes (it replaces the copy), or «away» with its reason. The promise settles on the first.
 * Nothing is aborted by a wait running out: a machine that answers late still opens the project. The copy is shown
 * again when the machine's word on it changes (it has not got the project, or it is down), so the page can say why.
 */
export const readProject = (
  id: string,
  read: { machine: () => Promise<IDesignLibrary>; store: () => Promise<IStoreLibrary> },
  show: (state: TProjectRead) => void,
  waits: IProjectReadWaits,
  /** The copy of a shared project another machine holds (a card key): the machine's own project of the same slug is not it. */
  card?: string,
  /** The address names no card and the project is only a copy shared from outside, one of that id: its key. */
  found?: (card: string) => void,
): Promise<void> =>
  new Promise((resolve) => {
    let machine: TSide = SIDE.pending;
    let store: TSide = SIDE.pending;
    let storeAsked = false;
    let machineError: string | null = null;
    let patient = true;
    // The key of the one copy of this id shared with the person, for an address that names none.
    let sharedKey: string | undefined;
    // The store's copy on screen and what the machine had said when it was shown.
    let copyShown: { design: IDesign; said: TMachineSaid } | null = null;
    const put = (state: TProjectRead) => {
      show(state);
      resolve();
    };
    const machineSaid = (): TMachineSaid => {
      if (machine === SIDE.absent) return MACHINE_SAID.absent;
      if (machine === SIDE.pending && patient) return MACHINE_SAID.waiting;
      return MACHINE_SAID.down;
    };
    const showCopy = (design: IDesign) => {
      const said = machineSaid();
      if (copyShown?.said === said) return;
      copyShown = { design, said };
      put({ k: PROJECT_READ.open, from: PROJECT_FROM.store, design, machine: said });
    };
    // The machine's word changed after the copy went on screen: the copy again, with it.
    const resay = () => {
      if (copyShown !== null && machine !== SIDE.has) showCopy(copyShown.design);
    };
    // Away is said once both sides have spoken, or the machine has been waited for long enough.
    const settle = () => {
      if (machine === SIDE.has || store === SIDE.has || store === SIDE.pending) return;
      if (machine === SIDE.pending && patient) return;
      if (machine === SIDE.denied) {
        put({ k: PROJECT_READ.away, why: AWAY.machineDenied, machineError });
        return;
      }
      // A link carries no card key: with no project of the person's own by this id, the one shared copy is what it
      // means. The page takes the key into its address and reads again; two such copies are not guessed between.
      if (found && sharedKey) {
        found(sharedKey);
        resolve();
        return;
      }
      const machineSaid = machine === SIDE.absent ? 'absent' : 'silent';
      const storeSaid = store === SIDE.absent ? 'absent' : 'failed';
      put({ k: PROJECT_READ.away, why: WHY[machineSaid][storeSaid], machineError });
    };
    const askStore = () => {
      if (storeAsked || machine === SIDE.has) return;
      storeAsked = true;
      read.store().then(
        (copy) => {
          // The grants index alone: the store did not give the library itself.
          if (copy.metadataOnly) {
            store = SIDE.failed;
            settle();
            return;
          }
          const design = copy.designs.find((d) => d.id === id && d.cardKey === card);
          if (!design && card === undefined) {
            const shared = copy.designs.filter((d) => d.id === id && d.cardKey && d.accessSource);
            sharedKey = shared.length === 1 ? shared[0].cardKey : undefined;
          }
          store = design ? SIDE.has : SIDE.absent;
          // The machine's project, once shown, is never replaced by the copy.
          if (design && machine !== SIDE.has) showCopy(design);
          settle();
        },
        () => {
          store = SIDE.failed;
          settle();
        },
      );
    };
    const storeTimer = setTimeout(askStore, waits.store);
    const patience = setTimeout(() => {
      patient = false;
      resay();
      settle();
    }, waits.machine);
    read.machine().then(
      (library) => {
        clearTimeout(patience);
        if (card === undefined && library.designs.some((d) => d.id === id)) {
          clearTimeout(storeTimer);
          machine = SIDE.has;
          put({ k: PROJECT_READ.open, from: PROJECT_FROM.machine, library });
          return;
        }
        machine = SIDE.absent;
        resay();
        askStore();
        settle();
      },
      (failure: unknown) => {
        clearTimeout(patience);
        machine = failure instanceof WordedError && failure.status === 403 ? SIDE.denied : SIDE.failed;
        machineError = failure instanceof Error ? failure.message : String(failure);
        resay();
        askStore();
        settle();
      },
    );
  });

/**
 * The reads of a project on one page. A quiet re-read (the pages' poll while an agent works, every few seconds) used to
 * abort the read before it: on a loaded machine the library takes longer than the poll, so the machine never answered,
 * the store always did, and a design that had once fallen back to the snapshot (a runtime restarting answers 503) stayed
 * on it, read-only, with «Редактирование» locked. Now a re-read of the same project leaves the one before to
 * finish, and none starts while the machine is still answering one; another project, machine or attempt aborts them.
 */
export const createProjectReads = () => {
  let key: string | null = null;
  // The machine's reads on their way, of this key only: a read of an earlier key may still settle after it is aborted.
  let generation = 0;
  let machineBusy = 0;
  const running = new Set<AbortController>();
  const stop = () => {
    for (const abort of running) abort.abort();
    running.clear();
    key = null;
    generation += 1;
    machineBusy = 0;
  };
  return {
    /** A read for `next`: its signal, or null when the machine is still answering a read of the same project. */
    start: (next: string): AbortSignal | null => {
      if (next === key && machineBusy > 0) return null;
      if (next !== key) stop();
      key = next;
      const abort = new AbortController();
      running.add(abort);
      return abort.signal;
    },
    /** The machine's read, counted while it is on its way. */
    machine: <T>(read: () => Promise<T>): Promise<T> => {
      const mine = generation;
      machineBusy += 1;
      return read().finally(() => {
        if (mine === generation) machineBusy -= 1;
      });
    },
    stop,
  };
};

/** Asks the machine to put the project into the hub's store now (design.StorePush); the sync does the sending. */
export const pushToStore = (m: MachineId, project: string): Promise<unknown> =>
  runtimeJson<unknown>(m, platformPath('/design/store-push'), postJson({ project }));

/**
 * A project open from its machine that the hub's store has nothing of (`lacks`) is put into the store at once, so the
 * next look reads the hub. Once per project; the sync would get there by itself, only later.
 */
export const useStorePush = (m: MachineId, project: string, lacks: boolean): void => {
  const pushed = useRef<string | null>(null);
  useEffect(() => {
    const key = `${m}|${project}`;
    if (!lacks || pushed.current === key) return;
    pushed.current = key;
    // A runtime with no store refuses (409): the design stays open from the machine, as it is.
    pushToStore(m, project).catch(() => undefined);
  }, [m, project, lacks]);
};
