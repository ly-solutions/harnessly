// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { pinPage, pinsOf, resetPinsMemory, unpinPage, visitPage } from '@/next/stores/browser-pins';
import { defaultPins } from '@/next/features/pane/pins';

vi.mock('@/platform/config', async (importOriginal) => {
  const real = await importOriginal<typeof import('@/platform/config')>();
  return {
    ...real,
    appConfig: () => ({ ...real.appConfig(), defaultPins: [{ text: 'admin.example.com', name: 'Admin' }] }),
  };
});

beforeEach(() => {
  window.localStorage.clear();
  resetPinsMemory();
});

describe('browser pins store', () => {
  it('starts a person with the defaults', () => {
    expect(pinsOf('ann@example.org').pins).toEqual([{ text: 'admin.example.com', name: 'Admin' }]);
    expect(pinsOf('ann@example.org').visits).toEqual([]);
  });

  it('keeps a pin after a reload, for that person only', () => {
    pinPage('ann@example.org', { text: 'stand-1.preview.example.com', name: 'Stand' });
    resetPinsMemory();
    expect(pinsOf('ann@example.org').pins.map((p) => p.text)).toContain('stand-1.preview.example.com');
    expect(pinsOf('bob@example.org').pins).toEqual(defaultPins());
  });

  it('keeps a removed default removed', () => {
    unpinPage('ann@example.org', 'admin.example.com');
    resetPinsMemory();
    expect(pinsOf('ann@example.org').pins.map((p) => p.text)).not.toContain('admin.example.com');
  });

  it('remembers visits, the latest first', () => {
    visitPage('ann@example.org', 'a.org');
    visitPage('ann@example.org', 'b.org');
    resetPinsMemory();
    expect(pinsOf('ann@example.org').visits).toEqual(['b.org', 'a.org']);
  });
});
