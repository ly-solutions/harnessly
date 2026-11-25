import { describe, expect, it } from 'vitest';

import type { CapabilityItem } from '../../../hub-api/orgs';
import { stateOf, whereOf } from './capabilities-text';

const item = (over: Partial<CapabilityItem>): CapabilityItem => ({
  id: 'x',
  group: 'g',
  name: 'n',
  description: 'd',
  source: 'org',
  enabled: false,
  defaultOn: false,
  canToggle: false,
  ...over,
});

describe('the words of the «Возможности» screen', () => {
  it("says the state, with the hub's short detail when it has one", () => {
    expect(stateOf(item({ enabled: true }))).toBe('Включено');
    expect(stateOf(item({ enabled: false }))).toBe('Выключено');
    expect(stateOf(item({ enabled: true, detail: 'Только предупреждает' }))).toBe('Только предупреждает');
  });

  it("shows what the hub's settings decide as read-only and names the setting", () => {
    const w = whereOf(item({ source: 'env', setBy: 'HUB_DOCS' }));
    expect(w).toContain('HUB_DOCS');
    expect(w).toContain('здесь не меняется');
    expect(w).toContain('По умолчанию выключено');
  });

  it('shows a flag the instance forces as read-only too', () => {
    expect(whereOf(item({ source: 'org', forced: true, setBy: 'HUB_FEATURES_FORCE' }))).toContain(
      'HUB_FEATURES_FORCE',
    );
  });

  it('says who toggles a flag or a switch', () => {
    expect(whereOf(item({ source: 'org', canToggle: true }))).toContain('прямо здесь');
    expect(whereOf(item({ source: 'org' }))).toContain('администратор установки');
    expect(whereOf(item({ source: 'rollout', canToggle: true, defaultOn: true }))).toContain(
      'По умолчанию включено',
    );
    expect(whereOf(item({ source: 'rollout' }))).toContain('администратор установки');
  });
});
