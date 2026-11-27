// The developer mode: a switch of the machine's settings file (developerMode) and the connectors' trace the
// runtime keeps while it is on (GET <platform>/connectors/trace, its SSE stream). Only on a computer: the phone never
// shows the switch nor the panel.
import { useEffect, useSyncExternalStore } from 'react';
import { getPage, sendJson } from '@/next/core/net/http';
import { platformPath, runtimePath } from '@/next/core/net/machine';
import type { MachineId } from '@/next/protocol/v2';

const TIMEOUT_MS = 15_000;

export const TRACE_LEVEL = { info: 'info', warn: 'warn', error: 'error' } as const;
export type TTraceLevel = (typeof TRACE_LEVEL)[keyof typeof TRACE_LEVEL];

export interface ITraceEntry {
  seq: number;
  at: string;
  conn: string;
  part: string;
  level: TTraceLevel;
  message: string;
  /** The raw detail (arguments, the service's answer, the envelope), shown folded. */
  data: unknown;
}

export interface ITrace {
  on: boolean;
  entries: readonly ITraceEntry[];
  conns: readonly string[];
}

const isObj = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null;

export const readTraceEntry = (v: unknown): ITraceEntry | null => {
  if (!isObj(v) || typeof v.seq !== 'number' || typeof v.message !== 'string') return null;
  const level = v.level === 'warn' || v.level === 'error' ? v.level : 'info';
  return {
    seq: v.seq,
    at: typeof v.at === 'string' ? v.at : '',
    conn: typeof v.conn === 'string' ? v.conn : '',
    part: typeof v.part === 'string' ? v.part : '',
    level,
    message: v.message,
    data: v.data ?? null,
  };
};

const readTrace = (v: unknown): ITrace => {
  const o = isObj(v) ? v : {};
  const entries = Array.isArray(o.entries) ? o.entries.map(readTraceEntry).filter((e) => e !== null) : [];
  const conns = Array.isArray(o.conns) ? o.conns.filter((c): c is string => typeof c === 'string') : [];
  return { on: o.on === true, entries, conns };
};

const tracePath = (machine: MachineId, rest = '', conn = '') =>
  runtimePath(
    platformPath(`/connectors/trace${rest}${conn ? `?conn=${encodeURIComponent(conn)}` : ''}`),
    machine,
  );

export const loadTrace = (machine: MachineId, conn = '') =>
  getPage(tracePath(machine, '', conn), readTrace, TIMEOUT_MS);

/** Follows the new lines of the trace; the returned function closes the stream. */
export const followTrace = (machine: MachineId, conn: string, fn: (e: ITraceEntry) => void): (() => void) => {
  if (typeof EventSource === 'undefined') return () => {};
  const source = new EventSource(tracePath(machine, '/stream', conn), { withCredentials: true });
  source.onmessage = (event: MessageEvent<string>) => {
    try {
      const e = readTraceEntry(JSON.parse(event.data));
      if (e) fn(e);
    } catch {
      // a broken line is skipped
    }
  };
  return () => source.close();
};

// --- the switch, one value per machine shared by the settings page and the panels ---------------------------------

const modes = new Map<MachineId, boolean>();
const listeners = new Set<() => void>();
const loading = new Set<MachineId>();

const notify = () => {
  for (const l of [...listeners]) l();
};

const readMode = (v: unknown): boolean => isObj(v) && v.developerMode === true;

const configPath = (machine: MachineId) => runtimePath('/api/config', machine);

const fetchMode = async (machine: MachineId) => {
  if (loading.has(machine)) return;
  loading.add(machine);
  const res = await getPage(configPath(machine), readMode, TIMEOUT_MS);
  loading.delete(machine);
  if ('ok' in res) {
    modes.set(machine, res.value);
    notify();
  }
};

const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => listeners.delete(l);
};

/** The machine's developer mode: null until read. */
export const useDevMode = (machine: MachineId): boolean | null => {
  const on = useSyncExternalStore(
    subscribe,
    () => modes.get(machine) ?? null,
    () => null,
  );
  useEffect(() => {
    if (!modes.has(machine)) void fetchMode(machine);
  }, [machine]);
  return on;
};

/** Turns the mode on or off in the machine's settings; false when the machine refused it. */
export const setDevMode = async (machine: MachineId, on: boolean): Promise<boolean> => {
  const res = await sendJson(configPath(machine), { developerMode: on }, () => null, TIMEOUT_MS);
  if (!('ok' in res)) return false;
  modes.set(machine, on);
  notify();
  return true;
};

/** For tests: forget what was read. */
export const resetDevModes = () => {
  modes.clear();
  loading.clear();
  notify();
};
