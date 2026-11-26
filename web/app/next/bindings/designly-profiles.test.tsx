// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { ComponentProps } from 'react';
import type DesignlyHome from '@/next/features/designly/designly-home';
import type { MachineId } from '@/next/protocol/v2';

const { create, goTo, agents } = vi.hoisted(() => ({
  goTo: vi.fn(),
  create: vi.fn(async () => ({ id: 'design-1', conv: 'tab-1', title: 'Test' })),
  agents: [
    {
      provider: 'claude',
      status: 'ready',
      models: [{ id: 'sonnet', label: 'Sonnet', efforts: [], defaultEffort: null }],
      profiles: ['glm', 'kimi', 'gpt-compatible'].map((id) => ({
        id,
        label: id,
        vendor: 'glm',
        configured: true,
        models: [{ id: 'vendor/model', label: id, efforts: ['high'], defaultEffort: 'high' }],
      })),
    },
  ],
}));
vi.mock('@/next/stores/machines', () => ({
  useMachine: () => ({ machine: { agents } }),
  limitsOf: () => ({ maxText: 100_000 }),
}));
vi.mock('@/next/bindings/use-limits', () => ({ useAgentLimits: () => null }));
vi.mock('@/next/bindings/settings-data', () => ({
  useAgentSettings: () => ({ state: { k: 'ready', value: { options: [] } } }),
}));
vi.mock('@/next/features/tour/use-tour-step', () => ({ useTourStep: () => null }));
vi.mock('@/next/features/tour/use-tour-fill', () => ({ useTourFill: () => undefined }));
vi.mock('@/next/stores/tour', () => ({ tourFact: () => undefined }));
vi.mock('@/next/stores/connection', () => ({
  useLinkRestored: () => 0,
  onReconnectNow: () => () => undefined,
}));
vi.mock('@/next/stores/conversations', () => ({ useConversationList: () => [] }));
vi.mock('@/next/bindings/use-voice', () => ({ useVoice: () => ({ recording: false }) }));
vi.mock('@/next/bindings/use-uploads', () => ({ useUploads: () => ({ reset: () => undefined }) }));
vi.mock('@/next/routes/routes', () => ({ ROUTE: { site: 'site', chat: 'chat' }, goTo }));
vi.mock('@/next/features/app-catalog/api', () => ({
  loadKinds: async () => [],
  loadKindDocs: async () => [],
}));
vi.mock('@/next/features/designly/kit/kit-api', () => ({
  fetchKitCards: async () => [],
  shelfCards: () => [],
  kitCover: () => '',
}));
vi.mock('@/next/features/designly/api', async (original) => ({
  ...(await original<typeof import('@/next/features/designly/api')>()),
  fetchLibrary: async () => ({ designs: [], port: 0, root: '/designs' }),
  fetchSystems: async () => [],
  fetchLayout: async () => ({ folders: [], placed: {}, systems: {} }),
  createDesign: create,
}));
// Exercise the real binding and selection hook; replace only the presentation of the create form.
vi.mock('@/next/features/designly/designly-home', () => ({
  DESIGN_KIND: { site: 'site', slides: 'slides', ds: 'ds', video: 'video', scene3d: 'scene3d' },
  default: ({ compose }: ComponentProps<typeof DesignlyHome>) => (
    <div>
      {compose?.picker?.agents.map((a) => (
        <button
          key={a.profile ?? a.provider}
          onClick={() =>
            compose.picker?.onChange({
              provider: a.provider,
              profile: a.profile,
              model: a.models[0].id,
              effort: 'high',
              ultracode: false,
            })
          }
        >
          {a.profile ?? a.provider}
        </button>
      ))}
      {['site', 'slides', 'video', 'scene3d', 'ds'].map((kind) => (
        <button
          key={kind}
          onClick={() =>
            void compose
              ?.onCreate({
                brief: 'Test',
                kind: kind as 'site',
                designSystem: null,
                deck: null,
                video: null,
                attachmentIds: [],
              })
              .catch(() => undefined)
          }
        >
          {`Create ${kind}`}
        </button>
      ))}
    </div>
  ),
}));
import DesignlyView from './designly';
beforeEach(() => {
  localStorage.clear();
  create.mockClear();
  goTo.mockClear();
});
afterEach(cleanup);
it.each(['glm', 'kimi', 'gpt-compatible'])(
  'offers and forwards the %s profile without changing the CLI provider',
  async (profile) => {
    render(<DesignlyView machine={'m-profile' as MachineId} />);
    fireEvent.click(screen.getByRole('button', { name: profile }));
    fireEvent.click(screen.getByRole('button', { name: 'Create site' }));
    await waitFor(() =>
      expect(create).toHaveBeenCalledWith(
        'm-profile',
        expect.objectContaining({ provider: 'claude', profile, model: 'vendor/model', effort: 'high' }),
      ),
    );
  },
);
it.each(['site', 'slides', 'video', 'ds'])(
  'opens a new %s with the chat that draws it at once',
  async (kind) => {
    render(<DesignlyView machine={'m-profile' as MachineId} />);
    fireEvent.click(screen.getByRole('button', { name: `Create ${kind}` }));
    await waitFor(() =>
      expect(goTo).toHaveBeenCalledWith({
        kind: 'site',
        id: 'design-1',
        chat: 'tab-1',
        machine: 'm-profile',
        edit: true,
      }),
    );
  },
);
it('opens the chat of a new 3D scene, which has no page until it is built', async () => {
  render(<DesignlyView machine={'m-profile' as MachineId} />);
  fireEvent.click(screen.getByRole('button', { name: 'Create scene3d' }));
  await waitFor(() => expect(goTo).toHaveBeenCalledWith({ kind: 'chat', id: 'tab-1' }));
});
it('stays on the start when the design was not made', async () => {
  create.mockRejectedValueOnce(new Error('no room'));
  render(<DesignlyView machine={'m-profile' as MachineId} />);
  fireEvent.click(screen.getByRole('button', { name: 'Create site' }));
  await waitFor(() => expect(create).toHaveBeenCalled());
  await new Promise((done) => setTimeout(done, 0));
  expect(goTo).not.toHaveBeenCalled();
});
