// The main sign-in of Codex on a machine: the profile a new Codex chat starts on in place of the CLI's own sign-in
// (Settings → Codex → «Ещё входы»). The list of the sign-ins says it; whoever reads the list notes it here.
import { useSyncExternalStore } from 'react';
import type { MachineId } from '@/next/protocol/v2';

/** By machine: the profile, or null when the CLI's own sign-in is the main one. Absent: not read yet. */
const mains = new Map<MachineId, string | null>();
const listeners = new Set<() => void>();

export const noteCodexMain = (machine: MachineId, profile: string | null) => {
  if (mains.has(machine) && mains.get(machine) === profile) return;
  mains.set(machine, profile);
  for (const tell of listeners) tell();
};

const subscribe = (tell: () => void) => {
  listeners.add(tell);
  return () => {
    listeners.delete(tell);
  };
};

/** undefined while the machine's list has not been read. */
export const useCodexMain = (machine: MachineId | null): string | null | undefined =>
  useSyncExternalStore(subscribe, () => (machine === null ? undefined : mains.get(machine)));
