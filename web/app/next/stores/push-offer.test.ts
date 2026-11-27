// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest';
import {
  PUSH_ENABLE_EVENT,
  SNOOZE_MS,
  offerSnoozed,
  requestPushEnable,
  setPushOfferState,
  snoozeOffer,
  type IPushEnableDetail,
} from '@/next/stores/push-offer';

describe('push offer', () => {
  afterEach(() => localStorage.clear());

  it('«Not now» hides the offer for a week', () => {
    const now = 1_000_000;
    expect(offerSnoozed(now)).toBe(false);
    snoozeOffer(now);
    expect(offerSnoozed(now + SNOOZE_MS - 1)).toBe(true);
    expect(offerSnoozed(now + SNOOZE_MS)).toBe(false);
  });

  it('answers the state at once without the bridge, and the bridge’s answer with it', async () => {
    setPushOfferState('off');
    await expect(requestPushEnable()).resolves.toEqual({ state: 'off' });
    const onEnable = (event: Event) => {
      const detail = (event as CustomEvent<IPushEnableDetail>).detail;
      detail.taken = true;
      queueMicrotask(() => detail.done({ state: 'on' }));
    };
    window.addEventListener(PUSH_ENABLE_EVENT, onEnable);
    await expect(requestPushEnable()).resolves.toEqual({ state: 'on' });
    window.removeEventListener(PUSH_ENABLE_EVENT, onEnable);
  });
});
