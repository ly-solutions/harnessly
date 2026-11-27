// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const fetched: { url: string; body: string }[] = [];
vi.mock('@/lib/app-path', () => ({
  appFetch: vi.fn(async (url: string, init?: RequestInit) => {
    fetched.push({ url, body: typeof init?.body === 'string' ? init.body : '' });
    if (url === '/api/push/vapid-key') return { ok: true, json: async () => ({ publicKey: 'AQID' }) };
    return { ok: true, status: 200 };
  }),
  currentMachineId: () => 'm1',
  prefixAppPath: (p: string) => p,
}));
vi.mock('@/lib/push-device', () => ({ getDeviceId: () => 'd1', isMobileDevice: () => true }));

import { t } from '@/lib/i18n';
import { enablePush, preparePush, usePushStore } from '@/hooks/use-web-push';

// A phone whose push service answers subscribe() only after the tap's 15 seconds.
let answer: (sub: PushSubscription) => void = () => {};
let current: PushSubscription | null = null;
const sub = {
  endpoint: 'https://fcm.googleapis.com/x',
  toJSON: () => ({ endpoint: 'https://fcm.googleapis.com/x' }),
} as unknown as PushSubscription;

const setUA = (ua: string) =>
  Object.defineProperty(window.navigator, 'userAgent', { value: ua, configurable: true });

beforeEach(() => {
  vi.useFakeTimers();
  fetched.length = 0;
  current = null;
  const pushManager = {
    subscribe: () =>
      new Promise<PushSubscription>((resolve) => {
        answer = (s) => {
          current = s;
          resolve(s);
        };
      }),
    getSubscription: async () => current,
  };
  const reg = { active: {}, pushManager };
  Object.defineProperty(window.navigator, 'serviceWorker', {
    value: { getRegistration: async () => reg, register: async () => reg },
    configurable: true,
  });
  (window as unknown as { PushManager: unknown }).PushManager = function PushManager() {};
  (window as unknown as { Notification: unknown }).Notification = { permission: 'granted' };
});
afterEach(() => vi.useRealTimers());

describe('enablePush on a phone whose push service is slow', () => {
  it('names the likely cause on Android, points at the Telegram bot, and keeps a late subscription', async () => {
    setUA('Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 Chrome/131 Mobile Safari/537.36');
    await preparePush();
    const result = enablePush();
    await vi.advanceTimersByTimeAsync(15_001);
    const r = await result;
    expect(r.error).toBe(t('hooks.useWebPush.subscribeHangsAndroid'));
    expect(r.telegram).toBe(true);
    expect(r.state).toBe('off');
    expect(
      fetched.some((f) => f.url === '/api/push/diagnostic' && f.body.includes('"step":"subscribe"')),
    ).toBe(true);
    expect(fetched.some((f) => f.url === '/api/push/subscribe')).toBe(false);

    // The answer comes later: it is saved and push is on.
    answer(sub);
    await vi.waitFor(() => expect(usePushStore.getState().state).toBe('on'));
    expect(
      fetched.some((f) => f.url === '/api/push/subscribe' && f.body.includes('fcm.googleapis.com')),
    ).toBe(true);
    expect(fetched.some((f) => f.url === '/api/push/diagnostic' && f.body.includes('subscribe-late'))).toBe(
      true,
    );
  });

  it('keeps the plain words off Android and does not offer the bot', async () => {
    setUA('Mozilla/5.0 (Macintosh; Intel Mac OS X 15_0) AppleWebKit/605.1.15 Version/18.0 Safari/605.1.15');
    await preparePush();
    const result = enablePush();
    await vi.advanceTimersByTimeAsync(15_001);
    const r = await result;
    expect(r.error).toBe(t('hooks.useWebPush.subscribeHangs'));
    expect(r.telegram).toBeUndefined();
  });
});
