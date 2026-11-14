// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest';
import { storeSink } from '@/next/core/realtime/apply';
import {
  DEFAULT_LIMITS,
  hasFeature,
  keptFeatures,
  machinesStore,
  putMachines,
  type FleetMachine,
} from '@/next/stores/machines';

const fleet = (id: string, over: Partial<FleetMachine> = {}): FleetMachine => ({
  id,
  name: id,
  host: id,
  reach: 'online',
  lastSeenAt: null,
  runtime: null,
  agents: [],
  ...over,
});

const welcome = (id: string, features: FleetMachine['features'] = []) =>
  storeSink.welcome(id, {
    f: 'welcome',
    proto: 2,
    epoch: 'e1',
    serverTime: 1,
    machine: fleet(id),
    features: features ?? [],
    limits: DEFAULT_LIMITS,
  });

afterEach(() => {
  for (const id of [...machinesStore.keys()]) machinesStore.delete(id);
  localStorage.clear();
});

// the microphone came and went. It is offered only for a machine that said `voice` in its welcome, and the
// welcome comes over the socket seconds after the page (a busy machine, a restart for an update, a slow tunnel).
describe('a machine’s features before its welcome', () => {
  it('stand in from its last welcome in this browser', () => {
    welcome('m', ['voice', 'designly']);
    machinesStore.delete('m');

    putMachines([fleet('m')]);

    expect(hasFeature('m', 'voice')).toBe(true);
    expect(machinesStore.get('m')?.runtime).toBe('unknown');
  });

  it('give way to what the machine says now', () => {
    welcome('m', ['voice']);
    machinesStore.delete('m');
    putMachines([fleet('m')]);

    welcome('m', []);

    expect(hasFeature('m', 'voice')).toBe(false);
    expect(keptFeatures('m')).toEqual([]);
  });

  it('take the fleet list’s own word first, and nothing for a machine never seen', () => {
    welcome('a', ['voice']);
    machinesStore.delete('a');

    putMachines([fleet('a', { features: ['designly'] }), fleet('b')]);

    expect(machinesStore.get('a')?.features).toEqual(['designly']);
    expect(machinesStore.get('b')?.features).toEqual([]);
  });

  it('drop a feature this page does not know', () => {
    localStorage.setItem('hx-machine-features', JSON.stringify({ m: ['voice', 'from-the-future', 7] }));

    putMachines([fleet('m')]);

    expect(machinesStore.get('m')?.features).toEqual(['voice']);
  });
});
