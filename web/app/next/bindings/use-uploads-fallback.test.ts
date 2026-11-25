// @vitest-environment jsdom
// the hub's store refused every picture (its S3 key was revoked) and the chat could take none. A machine that
// is on takes the file itself when the hub cannot.
import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

const keepInHub = vi.fn();
const uploadFile = vi.fn();
vi.mock('@/next/commands/uploads', async (orig) => ({
  ...(await orig<typeof import('@/next/commands/uploads')>()),
  keepInHub: (...a: unknown[]) => keepInHub(...a),
  uploadFile: (...a: unknown[]) => uploadFile(...a),
}));
vi.mock('@/platform/config', async (orig) => {
  const real = await orig<typeof import('@/platform/config')>();
  return { ...real, appConfig: () => ({ ...real.appConfig(), centralUploads: true }) };
});
vi.mock('@/next/stores/connection', async (orig) => {
  const real = await orig<typeof import('@/next/stores/connection')>();
  return { ...real, connectionStore: { ...real.connectionStore, get: () => ({ k: 'live' }) } };
});

import { useUploads } from '@/next/bindings/use-uploads';
import { UPLOAD_FAILURE } from '@/next/commands/uploads';
import type { MachineId } from '@/next/protocol/v2';

const machine = 'm-1' as MachineId;
const picture = () => new File([new Uint8Array([7, 7])], 'shot.png', { type: 'image/png' });
const ref = { id: 'up_X', name: 'shot.png', kind: 'image', mime: 'image/png' };

describe('a chat picture the hub refuses', () => {
  it('goes to the machine that is on', async () => {
    keepInHub.mockResolvedValue({ ok: false, code: UPLOAD_FAILURE.http });
    uploadFile.mockResolvedValue({ ok: true, ref });
    const { result } = renderHook(() => useUploads(machine, 'design:fallback-1', [], true));
    act(() => result.current.onFiles([picture()]));
    await waitFor(() => expect(uploadFile).toHaveBeenCalledTimes(1), { timeout: 5000 });
    expect(keepInHub).toHaveBeenCalledTimes(1);
  });

  it('is not sent twice when the person cancelled it', async () => {
    keepInHub.mockReset().mockResolvedValue({ ok: false, code: UPLOAD_FAILURE.aborted });
    uploadFile.mockReset();
    const { result } = renderHook(() => useUploads(machine, 'design:fallback-2', [], true));
    act(() => result.current.onFiles([picture()]));
    await waitFor(() => expect(keepInHub).toHaveBeenCalledTimes(1), { timeout: 5000 });
    await new Promise((r) => setTimeout(r, 50));
    expect(uploadFile).not.toHaveBeenCalled();
  });
});
