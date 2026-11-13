// @vitest-environment jsdom
import { cleanup, fireEvent, render } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import NoteVoice from '@/next/features/chat/note-voice';
import type { Recording } from '@/next/features/composer/types';
import { NoteMark } from '@/next/ui/notes/notes';
import { PRESENCE, type IPresence } from '@/next/ui/presence';

// The wave measures its canvas; jsdom has no observer of sizes.
beforeEach(() => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      disconnect() {}
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const REST: IPresence<Recording> = { value: null, state: PRESENCE.open };
const heard = (busy: boolean): IPresence<Recording> => ({
  value: { startedAt: Date.now(), busy, level: null },
  state: PRESENCE.open,
});

const setup = (bar: IPresence<Recording>, canRecord = true) => {
  const handlers = {
    onMic: vi.fn(),
    onCancel: vi.fn(),
    onFinish: vi.fn(),
    onValue: vi.fn(),
    onCommit: vi.fn(),
  };
  render(
    <NoteMark
      markRef={null}
      n={1}
      label="1"
      comment=""
      editing
      leaving={false}
      rowLabel="row"
      placeholder="what"
      onOpen={() => {}}
      onCommit={handlers.onCommit}
      onRemove={() => {}}
      onCancel={() => {}}
      field={{
        value: 'said so far',
        onValue: handlers.onValue,
        recording: bar.value !== null,
        tools: (
          <NoteVoice
            bar={bar}
            onMic={canRecord ? handlers.onMic : undefined}
            onCancel={handlers.onCancel}
            onFinish={handlers.onFinish}
          />
        ),
      }}
    />,
  );
  return handlers;
};

const row = () => document.querySelector('.hx-note__row') as HTMLElement;
const recButtons = () => [...document.querySelectorAll('.hx-note__recbar .hx-note__btn')];

describe('the microphone of an annotation', () => {
  it('stands in the foot of the row and starts the dictation', () => {
    const { onMic } = setup(REST);
    const mic = document.querySelector('.hx-note__mic') as Element;
    expect(row().querySelector('.hx-note__foot')?.contains(mic)).toBe(true);
    expect(row().hasAttribute('data-rec')).toBe(false);
    fireEvent.click(mic);
    expect(onMic).toHaveBeenCalledTimes(1);
  });

  it('is not offered where nothing can record', () => {
    setup(REST, false);
    expect(document.querySelector('.hx-note__mic')).toBeNull();
  });

  it("writes into the row the host keeps: the field shows the host's words", () => {
    const { onValue } = setup(REST);
    const field = row().querySelector('textarea') as HTMLTextAreaElement;
    expect(field.value).toBe('said so far');
    fireEvent.change(field, { target: { value: 'said so far, and typed' } });
    expect(onValue).toHaveBeenCalledWith('said so far, and typed');
  });

  it('shows the recording in place of the microphone: the time, the wave, cancel and stop', () => {
    const { onCancel, onFinish } = setup(heard(false));
    expect(row().hasAttribute('data-rec')).toBe(true);
    expect(document.querySelector('.hx-note__mic')).toBeNull();
    expect(document.querySelector('.hx-note__recbar .hx-recbar__time')).not.toBeNull();
    expect(document.querySelector('.hx-note__recbar .hx-wave')).not.toBeNull();
    const [cancel, stop] = recButtons();
    fireEvent.click(stop);
    expect(onFinish).toHaveBeenCalledTimes(1);
    fireEvent.click(cancel);
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('does not stop twice while the words are being recognized', () => {
    const { onFinish } = setup(heard(true));
    const [, stop] = recButtons();
    expect(stop.getAttribute('aria-disabled')).toBe('true');
    fireEvent.click(stop);
    expect(onFinish).not.toHaveBeenCalled();
  });
});
