// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useStandSignIn } from '@/next/features/pane/stand-sign-in';

const ORIGIN = 'https://ai-u-7.preview.example.com';
const URL_IN = 'https://stand-1.example.com/oauth2/start?rd=x';

afterEach(() => vi.restoreAllMocks());

const setup = (signInUrl: string | null = URL_IN) => {
  const postMessage = vi.fn();
  const win = { postMessage } as unknown as Window;
  const iframe = { contentWindow: win } as HTMLIFrameElement;
  const onReload = vi.fn();
  const hook = renderHook(() =>
    useStandSignIn({ frame: { current: iframe }, origin: ORIGIN, signInUrl, onReload }),
  );
  const say = (data: unknown, origin = ORIGIN, source: unknown = win) =>
    act(() => {
      window.dispatchEvent(
        new MessageEvent('message', { data, origin, source: source as MessageEventSource }),
      );
    });
  return { hook, postMessage, say, onReload };
};

describe('stand sign-in', () => {
  it('pings the page after a load, and the wall answering shows «Войти»', () => {
    const { hook, postMessage, say } = setup();
    act(() => hook.result.current.onFrameLoad());
    expect(postMessage).toHaveBeenCalledWith({ type: 'tk:ping' }, ORIGIN);
    say({ type: 'tk:sign-in' });
    expect(hook.result.current.state).toBe('asked');
  });

  it('ignores the same message from another window or origin', () => {
    const { hook, say } = setup();
    say({ type: 'tk:sign-in' }, 'https://evil.example.com');
    say({ type: 'tk:sign-in' }, ORIGIN, {});
    expect(hook.result.current.state).toBe('none');
  });

  it('does not offer a sign-in for a page that is not a stand', () => {
    const { hook, say } = setup(null);
    say({ type: 'tk:sign-in' });
    expect(hook.result.current.state).toBe('none');
  });

  it('opens the stand sign-in in a tab, waits, and reloads when the person comes back', () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null);
    window.matchMedia = ((q: string) => ({
      matches: false,
      media: q,
    })) as unknown as typeof window.matchMedia;
    const { hook, say, onReload } = setup();
    say({ type: 'tk:sign-in' });
    act(() => hook.result.current.start());
    expect(open).toHaveBeenCalledWith(URL_IN, '_blank', 'noopener');
    expect(hook.result.current.state).toBe('waiting');
    act(() => {
      window.dispatchEvent(new Event('focus'));
    });
    expect(onReload).toHaveBeenCalledTimes(1);
    expect(hook.result.current.state).toBe('none');
    act(() => {
      window.dispatchEvent(new Event('focus'));
    });
    expect(onReload).toHaveBeenCalledTimes(1);
  });

  it('clears the offer when a new document loads', () => {
    const { hook, say } = setup();
    say({ type: 'tk:sign-in' });
    act(() => hook.result.current.onFrameLoad());
    expect(hook.result.current.state).toBe('none');
  });
});
