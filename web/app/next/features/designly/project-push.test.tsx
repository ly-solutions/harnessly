// @vitest-environment jsdom
import { cleanup, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

const runtimeJson = vi.hoisted(() => vi.fn(() => Promise.resolve({ ok: true })));
vi.mock('@/next/core/net/runtime-json', () => ({
  runtimeJson,
  postJson: (body: unknown) => ({ method: 'POST', body: JSON.stringify(body) }),
}));

import type { IDesign, IDesignLibrary, IStoreLibrary } from '@/next/features/designly/api';
import {
  PROJECT_FROM,
  PROJECT_READ,
  readProject,
  useStorePush,
  type TProjectRead,
} from '@/next/features/designly/project-read';
import type { MachineId } from '@/next/protocol/v2';

const M = 'm-1' as MachineId;
const library = (...ids: string[]): IDesignLibrary => ({
  port: 4400,
  root: '/lib',
  designs: ids.map((id) => ({ id }) as unknown as IDesign),
});
const noCopy: IStoreLibrary = { designs: [], layout: { folders: [], placed: {}, systems: {} } };
const pushes = () =>
  runtimeJson.mock.calls.filter((call) => String((call as unknown[])[1]).endsWith('/design/store-push'));

describe('a project the store could not give (HRN-1308)', () => {
  afterEach(() => {
    cleanup();
    runtimeJson.mockClear();
  });

  it('no copy in the store, the machine on the link: opened from the machine and put into the hub', async () => {
    const shown: TProjectRead[] = [];
    await readProject(
      'p1',
      { machine: () => Promise.resolve(library('p1')), store: () => Promise.resolve(noCopy) },
      (state) => shown.push(state),
      { store: 0, machine: 1000 },
    );
    expect(shown).toHaveLength(1);
    expect(shown[0]).toMatchObject({ k: PROJECT_READ.open, from: PROJECT_FROM.machine });
    // The page: the project is open from the machine and the store's head says «none».
    const view = renderHook(({ lacks }: { lacks: boolean }) => useStorePush(M, 'p1', lacks), {
      initialProps: { lacks: true },
    });
    expect(pushes()).toHaveLength(1);
    expect((pushes()[0] as unknown[])[2]).toEqual({
      method: 'POST',
      body: JSON.stringify({ project: 'p1' }),
    });
    // Asked once: the head turning to «asking» and back to «none» does not ask the machine again.
    view.rerender({ lacks: false });
    view.rerender({ lacks: true });
    expect(pushes()).toHaveLength(1);
  });

  it('the store cannot be read, the machine on the link: opened from the machine and put into the hub', async () => {
    const shown: TProjectRead[] = [];
    await readProject(
      'p1',
      { machine: () => Promise.resolve(library('p1')), store: () => Promise.reject(new Error('store 500')) },
      (state) => shown.push(state),
      { store: 0, machine: 1000 },
    );
    expect(shown[0]).toMatchObject({ k: PROJECT_READ.open, from: PROJECT_FROM.machine });
    renderHook(() => useStorePush(M, 'p1', true));
    expect(pushes()).toHaveLength(1);
  });

  it('a project the store has is not pushed', () => {
    renderHook(() => useStorePush(M, 'p1', false));
    expect(pushes()).toHaveLength(0);
  });
});
