// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { seedLimits, useAgentLimits } from '@/next/bindings/use-limits';
import { limitsOwnerOf } from '@/next/features/limits/model';

const answer = (usedPct: number) => ({
  machines: [
    {
      id: 'local',
      limits: { claude: { fiveHour: { usedPct, resetsAt: 1_900_000_000 }, sevenDay: null }, codex: null },
    },
  ],
});

const respond = (body: unknown, status = 200) =>
  Promise.resolve(new Response(JSON.stringify(body), { status }));

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn());
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const CLAUDE = { provider: 'claude', profile: null } as const;

describe('useAgentLimits', () => {
  // the hub lists what the machine last said of its limits; the chip draws it before the machine answers,
  // and keeps it while the machine does not answer at all.
  it('draws what the hub remembers at once, and the machine’s own answer over it', async () => {
    let answer10: (r: Response) => void = () => undefined;
    vi.mocked(fetch).mockImplementation(() => new Promise<Response>((resolve) => (answer10 = resolve)));
    seedLimits('m-hub', answer(70).machines[0].limits);
    const { result } = renderHook(() => useAgentLimits('m-hub', CLAUDE));
    expect(result.current?.state === 'ready' && result.current.limits?.fiveHour?.usedPct).toBe(70);
    answer10(new Response(JSON.stringify(answer(10)), { status: 200 }));
    await waitFor(() =>
      expect(result.current?.state === 'ready' && result.current.limits?.fiveHour?.usedPct).toBe(10),
    );
  });

  it('keeps what the hub remembers when the machine does not answer', async () => {
    vi.mocked(fetch).mockImplementation(() => respond({ error: 'down' }, 502));
    seedLimits('m-silent', answer(55).machines[0].limits);
    const { result } = renderHook(() => useAgentLimits('m-silent', CLAUDE));
    await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalled());
    await act(async () => undefined);
    expect(result.current?.state === 'ready' && result.current.limits?.fiveHour?.usedPct).toBe(55);
  });

  it("draws nothing before the machine answers, then the agent's windows, and asks the platform route of the machine", async () => {
    vi.mocked(fetch).mockImplementation(() => respond(answer(10)));
    const { result } = renderHook(() => useAgentLimits('m-ok', CLAUDE));
    expect(result.current).toBeNull();
    await waitFor(() => expect(result.current).not.toBeNull());
    expect(result.current).toMatchObject({ state: 'ready', provider: 'claude' });
    expect(result.current?.state === 'ready' && result.current.limits?.fiveHour?.usedPct).toBe(10);
    expect(String(vi.mocked(fetch).mock.calls[0][0])).toMatch(/\/machines\?machine=m-ok$/);
  });

  it('reports an agent without limits as ready with none, not as a failure', async () => {
    vi.mocked(fetch).mockImplementation(() => respond(answer(10)));
    const { result } = renderHook(() => useAgentLimits('m-codex', { provider: 'codex', profile: null }));
    await waitFor(() =>
      expect(result.current).toMatchObject({ state: 'ready', provider: 'codex', limits: null, spend: null }),
    );
    // The card still tells the person's other subscription on the machine.
    const view = result.current;
    expect(view?.state === 'ready' && view.others?.map((o) => o.key)).toEqual(['claude']);
  });

  it('says it failed for a machine that never answered, and keeps the last good answer through a stumble', async () => {
    vi.mocked(fetch).mockImplementation(() => respond({ error: 'down' }, 500));
    const bad = renderHook(() => useAgentLimits('m-bad', CLAUDE));
    await waitFor(() => expect(bad.result.current).toMatchObject({ state: 'failed', provider: 'claude' }));

    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.mocked(fetch).mockImplementation(() => respond(answer(20)));
    const good = renderHook(() => useAgentLimits('m-good', CLAUDE));
    await waitFor(() => expect(good.result.current?.state).toBe('ready'));
    vi.mocked(fetch).mockImplementation(() => respond({ error: 'down' }, 500));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(61_000);
    });
    expect(good.result.current?.state).toBe('ready');
    vi.useRealTimers();
  });
});

describe('limitsOwnerOf', () => {
  it("names the agent of a chat on the CLI's own account and the profile of a chat on a vendor's", () => {
    expect(limitsOwnerOf({ provider: 'claude' })).toEqual({ provider: 'claude', profile: null });
    expect(limitsOwnerOf({ provider: 'codex', profile: null })).toEqual({ provider: 'codex', profile: null });
    expect(limitsOwnerOf({ provider: 'claude', profile: 'kimi-code' })).toEqual({
      provider: 'claude',
      profile: 'kimi-code',
    });
    expect(limitsOwnerOf(null)).toBeNull();
  });

  it('sends a GPT chat to the limits of the ChatGPT subscription and gives the router none', () => {
    expect(limitsOwnerOf({ provider: 'claude', profile: 'gpt' })).toEqual({
      provider: 'codex',
      profile: null,
    });
    expect(limitsOwnerOf({ provider: 'claude', profile: 'auto' })).toBeNull();
  });
});

describe("useAgentLimits for a vendor's profile", () => {
  const glm = {
    profile: 'glm',
    account: '',
    fiveHour: { usedPct: 25, resetsAt: 1_900_000_000, forecast: { status: 'insufficient_data' } },
    sevenDay: null,
  };

  it('gives the limits of the profile, without a forecast, and no chip for a profile the machine has nothing of', () => {
    seedLimits('m-profiles', { claude: null, codex: null, profiles: { glm } });
    const mine = renderHook(() => useAgentLimits('m-profiles', { provider: 'claude', profile: 'glm' }));
    const view = mine.result.current;
    expect(view?.state === 'ready' && view.profile).toBe('glm');
    expect(view?.state === 'ready' && view.limits?.fiveHour).toEqual({
      usedPct: 25,
      resetsAt: 1_900_000_000,
      forecast: null,
    });
    const none = renderHook(() => useAgentLimits('m-profiles', { provider: 'codex', profile: 'xai' }));
    expect(none.result.current).toBeNull();
  });
});
