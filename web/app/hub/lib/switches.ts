// The rollout switches as the administrator edits them (internal/hub/api/switches.go, docs/hub/rollout-switches.md): a row
// is feature × person × machine, `*` for anyone / any machine. The confirm shows the exact JSON the hub will get.

export type RolloutSwitch = {
  feature: string;
  /** An email, or `*`. */
  person: string;
  /** A machine id, or `*`. */
  machine: string;
  enabled: boolean;
  note: string;
  updatedBy: string;
  updatedAt: string;
};

/** The body of `PUT /api/admin/switches`. */
export type SwitchPut = Pick<RolloutSwitch, 'feature' | 'person' | 'machine' | 'enabled' | 'note'>;

/** The key of a row: what `DELETE /api/admin/switches` names. */
export type SwitchKey = Pick<RolloutSwitch, 'feature' | 'person' | 'machine'>;

/** `updatedBy` of a row the hub seeded from `HUB_SWITCHES` on its start. */
export const CONFIG_SEED = 'config:HUB_SWITCHES';

export const fromConfig = (s: Pick<RolloutSwitch, 'updatedBy'>): boolean => s.updatedBy === CONFIG_SEED;

export const same = (a: SwitchKey, b: SwitchKey): boolean =>
  a.feature === b.feature && a.person === b.person && a.machine === b.machine;

export interface SwitchForm {
  feature: string;
  person: string;
  machine: string;
  enabled: boolean;
  note: string;
}

export const emptyForm = (): SwitchForm => ({
  feature: '',
  person: '',
  machine: '',
  enabled: true,
  note: '',
});

export const toForm = (s: RolloutSwitch): SwitchForm => ({
  feature: s.feature,
  person: s.person,
  machine: s.machine,
  enabled: s.enabled,
  note: s.note,
});

/** What goes to the hub: an empty person or machine is written as `*`, so the confirm and the call say the same. */
export const toPut = (f: SwitchForm): SwitchPut => ({
  feature: f.feature.trim(),
  person: f.person.trim() || '*',
  machine: f.machine.trim() || '*',
  enabled: f.enabled,
  note: f.note.trim(),
});

export const putJson = (p: SwitchPut): string => JSON.stringify(p);

export const deletePath = (k: SwitchKey): string => {
  const q = new URLSearchParams({ feature: k.feature, person: k.person, machine: k.machine });
  return `DELETE /api/admin/switches?${q.toString()}`;
};

/** Rows by feature, then person, then machine — the order a reader scans them in. */
export const sorted = (rows: readonly RolloutSwitch[]): RolloutSwitch[] =>
  [...rows].sort(
    (a, b) =>
      a.feature.localeCompare(b.feature) ||
      a.person.localeCompare(b.person) ||
      a.machine.localeCompare(b.machine),
  );
