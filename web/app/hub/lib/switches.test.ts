import { describe, expect, it } from 'vitest';

import { deletePath, emptyForm, fromConfig, putJson, sorted, toPut } from './switches';

describe('rollout switches helpers', () => {
  it('writes an empty person and machine as * so the confirm equals the call', () => {
    const p = toPut({ ...emptyForm(), feature: ' own-work ', note: ' hi ' });
    expect(p).toEqual({ feature: 'own-work', person: '*', machine: '*', enabled: true, note: 'hi' });
    expect(JSON.parse(putJson(p))).toEqual(p);
  });

  it('builds the DELETE path with escaped query', () => {
    expect(deletePath({ feature: 'f', person: 'a@b.c', machine: '*' })).toBe(
      'DELETE /api/admin/switches?feature=f&person=a%40b.c&machine=*',
    );
  });

  it('knows config-seeded rows and sorts by feature, person, machine', () => {
    expect(fromConfig({ updatedBy: 'config:HUB_SWITCHES' })).toBe(true);
    expect(fromConfig({ updatedBy: 'a@b.c' })).toBe(false);
    const r = (feature: string, person: string) => ({
      feature,
      person,
      machine: '*',
      enabled: true,
      note: '',
      updatedBy: '',
      updatedAt: '',
    });
    expect(sorted([r('b', '*'), r('a', 'z'), r('a', 'y')]).map((x) => `${x.feature}${x.person}`)).toEqual([
      'ay',
      'az',
      'b*',
    ]);
  });
});
