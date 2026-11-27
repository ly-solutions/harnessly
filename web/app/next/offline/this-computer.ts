// «Этот компьютер»: the person's laptop runs one of their machines. While the network is there the page
// keeps a pass from that machine's runtime (cmd/runtime/routes_local.go); without it, the page goes to the runtime on
// the loopback, http://127.0.0.1:<port><base>, where the same app runs on the same chats with a session the pass opens —
// the whole app, agents included (on a local model, Settings → Providers). When the hub answers again the loopback page
// offers the way back to the same place in the hub.
//
// The pass lives in this origin's storage, by person: it opens a session on that laptop's loopback only.

import { createStore, useStore } from '@/next/core/store';
import { getPage } from '@/next/core/net/http';
import { platformPath, runtimePath } from '@/next/core/net/machine';
import { offlineBase } from '@/next/offline/client';
import { owner } from '@/next/offline/storage';
import { netStore } from '@/next/offline/net';
import type { MachineId } from '@/next/protocol/v2';
import { askedThisComputer, onLoopback, routeHere } from '@/next/offline/loopback';
import { t } from '@/next/i18n';
import { toast } from '@/next/ui/toast';

export interface IThisComputer {
  person: string;
  machine: MachineId;
  port: number;
  /** The runtime's base path, with its slash. */
  base: string;
  pass: string;
  expiresAt: number;
}

const key = () => 'hx-this-computer:' + offlineBase();
/** A pass this close to its end is asked anew while online. */
const RENEW_MS = 7 * 24 * 3600_000;
const PING_MS = 4000;

const read = (): IThisComputer | null => {
  try {
    const raw = localStorage.getItem(key());
    if (!raw) return null;
    const v = JSON.parse(raw) as IThisComputer;
    if (typeof v.pass !== 'string' || typeof v.port !== 'number' || typeof v.base !== 'string') return null;
    return v;
  } catch {
    return null;
  }
};

export const thisComputerStore = createStore<IThisComputer | null>(null);

/** This person's «this computer», if this browser has one and its pass is still good. */
export function thisComputer(): IThisComputer | null {
  const v = thisComputerStore.get() ?? read();
  const person = owner(offlineBase());
  if (!v || !person || v.person !== person || v.expiresAt < Date.now()) return null;
  return v;
}

/** The pairing as the page holds it; an expired pass is dropped when it is loaded or renewed. */
export const useThisComputer = (): IThisComputer | null => useStore(thisComputerStore);

const save = (v: IThisComputer | null) => {
  try {
    if (v) localStorage.setItem(key(), JSON.stringify(v));
    else localStorage.removeItem(key());
  } catch {
    // Storage blocked: the pairing lasts for this page only.
  }
  thisComputerStore.set(v);
};

/** The page signs a person out: the pass goes with them. */
export function forgetThisComputer(): void {
  save(null);
}

/** Loads what this browser kept for the person (the boot calls it once the person is confirmed). */
export function loadThisComputer(): void {
  thisComputerStore.set(thisComputer());
}

const loopback = (port: number, base: string) => `http://127.0.0.1:${port}${base}`;

interface IPassAnswer {
  port: number;
  base: string;
  pass: string;
  expiresAt: number;
}
const parsePass = (v: unknown): IPassAnswer => {
  const o = v as Partial<IPassAnswer> | null;
  if (
    !o ||
    typeof o.port !== 'number' ||
    typeof o.base !== 'string' ||
    !o.base.startsWith('/') ||
    typeof o.pass !== 'string' ||
    typeof o.expiresAt !== 'number'
  )
    throw new Error('bad pass');
  return {
    port: o.port,
    base: o.base.endsWith('/') ? o.base : o.base + '/',
    pass: o.pass,
    expiresAt: o.expiresAt,
  };
};

const askPass = async (machine: MachineId): Promise<IPassAnswer | null> => {
  const res = await getPage(runtimePath(platformPath('/local-pass'), machine), parsePass, 15_000);
  return 'ok' in res ? res.value : null;
};

/** The runtime on this computer's loopback answers for the pass: it is the machine this browser runs on. */
const answersHere = async (a: IPassAnswer): Promise<boolean> => {
  // The runtime's auth routes sit at the root of its port, and under its base where a proxy keeps the base.
  for (const at of [
    `http://127.0.0.1:${a.port}/api/auth/local`,
    `${loopback(a.port, a.base)}api/auth/local`,
  ]) {
    const ctrl = new AbortController();
    const cut = setTimeout(() => ctrl.abort(), PING_MS);
    try {
      const res = await fetch(at, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ pass: a.pass }),
        mode: 'cors',
        credentials: 'omit',
        signal: ctrl.signal,
        // Chrome's Local Network Access: the page asks the person once for the loopback.
        ...({ targetAddressSpace: 'loopback' } as object),
      });
      if (res.ok) return true;
    } catch {
      // Not here, or the browser kept the page from the loopback.
    } finally {
      clearTimeout(cut);
    }
  }
  return false;
};

/**
 * Makes `machine` this computer: asks its runtime for a pass through the hub and checks that the runtime answers on
 * this computer's loopback. False when it is another computer's machine (or no local one).
 */
export async function pairThisComputer(machine: MachineId): Promise<boolean> {
  const person = owner(offlineBase());
  if (!person) return false;
  const a = await askPass(machine);
  if (!a || !(await answersHere(a))) return false;
  save({ person, machine, ...a });
  return true;
}

/** Of the person's machines, the one on this computer; null when none answers here. */
export async function findThisComputer(machines: readonly MachineId[]): Promise<MachineId | null> {
  for (const m of machines) if (await pairThisComputer(m)) return m;
  return null;
}

/** A pass near its end is renewed while the network is there; a machine that no longer gives one is let go. */
export async function renewThisComputer(): Promise<void> {
  const v = thisComputer();
  if (!v || v.expiresAt - Date.now() > RENEW_MS || !netStore.get().online) return;
  const a = await askPass(v.machine);
  if (a) save({ ...v, ...a });
}

/** Where this page continues on this computer: the same route, the pass and the way back in the fragment. */
export function thisComputerUrl(v: IThisComputer): string {
  const back = location.origin + offlineBase();
  return `${loopback(v.port, v.base)}${routeHere()}#hx-local=${encodeURIComponent(v.pass)}&back=${encodeURIComponent(back)}`;
}

/** Goes on to this computer: the same app on the loopback. */
export function openThisComputer(): boolean {
  const v = thisComputer();
  if (!v || onLoopback()) return false;
  location.assign(thisComputerUrl(v));
  return true;
}

/**
 * The install on this computer opened the app with `?this-computer=<machine>&port=` (deploy/install/install.sh): the
 * page pairs it by itself, says so, and takes the words out of the address. The machine may take a moment to link.
 */
let pairedFromAddress = false;

export async function pairFromAddress(): Promise<void> {
  const machine = askedThisComputer;
  if (!machine || pairedFromAddress || onLoopback()) return;
  pairedFromAddress = true;
  const params = new URLSearchParams(location.search);
  if (params.has('this-computer')) {
    params.delete('this-computer');
    params.delete('port');
    const rest = params.toString();
    history.replaceState(history.state, '', location.pathname + (rest ? `?${rest}` : '') + location.hash);
  }
  for (let i = 0; i < 20; i += 1) {
    if (await pairThisComputer(machine as MachineId)) {
      toast(t('next.thisComputer.paired'), {
        tone: 'success',
        description: t('next.thisComputer.pairedHint'),
      });
      return;
    }
    await new Promise((r) => setTimeout(r, 3000));
  }
  toast(t('next.thisComputer.notHere'), { tone: 'error' });
}
