// @vitest-environment jsdom
import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { MachineId } from '@/next/protocol/v2';

const runtimeJson = vi.fn();
vi.mock('@/next/core/net/runtime-json', async (real) => ({
  ...(await real<typeof import('@/next/core/net/runtime-json')>()),
  runtimeJson: (...a: unknown[]) => runtimeJson(...a),
}));

const { appService, serviceStateOf, useAppService, SERVICE_STATE, POLL_DOWN_MS } =
  await import('./service-health');

const machine = 'm1' as MachineId;

afterEach(() => {
  runtimeJson.mockReset();
  vi.useRealTimers();
});

describe('service health', () => {
  it('knows the services of the built-in apps only', () => {
    expect(appService('harnessly.docs')).toBe('documents');
    expect(appService('harnessly.boards')).toBe('boards');
    expect(appService('acme.notes')).toBeNull();
    expect(appService('toString')).toBeNull();
  });

  it('calls a service down only when the hub says so', () => {
    expect(serviceStateOf({ services: { documents: { up: false } } }, 'documents')).toBe(SERVICE_STATE.down);
    expect(serviceStateOf({ services: { documents: { up: true } } }, 'documents')).toBe(SERVICE_STATE.up);
    expect(serviceStateOf({ services: { documents: { up: false } } }, 'boards')).toBe(SERVICE_STATE.unknown);
    expect(serviceStateOf({ enabled: true }, 'documents')).toBe(SERVICE_STATE.unknown);
    expect(serviceStateOf(null, 'documents')).toBe(SERVICE_STATE.unknown);
  });

  it('asks again while the service is down and turns up by itself', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    runtimeJson.mockResolvedValue({ services: { boards: { up: false } } });
    const { result } = renderHook(() => useAppService(machine, 'harnessly.boards'));
    await waitFor(() => expect(result.current.state).toBe(SERVICE_STATE.down));
    expect(String(runtimeJson.mock.calls[0]?.[1])).toMatch(/\/docs\/_status$/);
    runtimeJson.mockResolvedValue({ services: { boards: { up: true } } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(POLL_DOWN_MS);
    });
    await waitFor(() => expect(result.current.state).toBe(SERVICE_STATE.up));
  });

  it('a page without the hub keeps the view as it was', async () => {
    runtimeJson.mockRejectedValue(new Error('404'));
    const { result } = renderHook(() => useAppService(machine, 'harnessly.docs'));
    await waitFor(() => expect(runtimeJson).toHaveBeenCalled());
    expect(result.current.state).toBe(SERVICE_STATE.unknown);
  });

  it('asks nothing for another app', () => {
    renderHook(() => useAppService(machine, 'acme.notes'));
    expect(runtimeJson).not.toHaveBeenCalled();
  });
});
