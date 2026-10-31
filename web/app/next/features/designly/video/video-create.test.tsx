// @vitest-environment jsdom
import { cleanup, fireEvent, render, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { MachineId } from '@/next/protocol/v2';

let text = '';
vi.mock('@/next/stores/drafts', () => ({
  useDraft: () => ({ text, attachments: [] }),
  setDraftText: (_k: string, v: string) => {
    text = v;
  },
  clearDraft: vi.fn(),
}));
vi.mock('@/next/bindings/use-uploads', () => ({
  useUploads: () => ({
    attachments: [],
    ready: [{ id: 'a1' }],
    uploading: false,
    onFiles: vi.fn(),
    onRemove: vi.fn(),
    onRetry: vi.fn(),
    reset: vi.fn(),
  }),
}));

vi.setConfig({ testTimeout: 30000 });
// The settings panel asks whether the window is a phone's (a sheet) or wide (a popover).
window.matchMedia ??= ((query: string) =>
  ({
    matches: false,
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }) as unknown as MediaQueryList) as typeof window.matchMedia;

const { default: VideoCreate } = await import('@/next/features/designly/video/video-create');

afterEach(() => {
  cleanup();
  text = '';
});

describe('VideoCreate', () => {
  it('sends a payload that matches DesignlyCreate.video', async () => {
    text = 'A promo of the new tariff';
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<VideoCreate machine={'m1' as MachineId} systems={[]} onBack={vi.fn()} onCreate={onCreate} />);
    // Everything but the brief is in the settings: the design system, the formats, the length and the subtitles.
    fireEvent.click(document.querySelector('.hx-deck-chip') as HTMLElement);
    const groups = document.querySelectorAll('.hx-deck-set [role="radiogroup"]');
    fireEvent.click(groups[1].querySelectorAll('button')[2]);
    fireEvent.click(groups[2].querySelectorAll('button')[2]);
    fireEvent.click(document.querySelector('.hx-deck-set [role="switch"]') as HTMLElement);
    fireEvent.click(document.querySelector('.hx-round[data-variant="accent"]') as HTMLElement);
    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(1));
    expect(onCreate.mock.calls[0][0]).toEqual({
      title: 'A promo of the new tariff',
      brief: 'A promo of the new tariff',
      video: { formats: ['16:9', '9:16'], seconds: 30, subs: false },
      designSystem: null,
      attachmentIds: ['a1'],
    });
  });

  it('sends nothing for an empty brief', () => {
    const onCreate = vi.fn();
    render(<VideoCreate machine={'m1' as MachineId} systems={[]} onBack={vi.fn()} onCreate={onCreate} />);
    fireEvent.click(document.querySelector('.hx-round[data-variant="accent"]') as HTMLElement);
    expect(onCreate).not.toHaveBeenCalled();
  });
});
