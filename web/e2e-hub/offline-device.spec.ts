import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { startStack, type Stack } from './stack';

// the device itself loses the network. The production page with its service worker on a real hub and
// machine: a chat is made online; then the browser goes offline and the page is opened anew — the same app opens (not
// a separate reader), with the sidebar, the chat read from the device and «Офлайн» beside the workspace's name; a
// message typed offline waits. The network comes back without a reload: the mark goes, the message goes and is
// answered.

const cell = process.env.E2E_CELL ?? 'dev';
const shotsDir = process.env.E2E_SHOTS_DIR ?? join('e2e-hub-results', 'shots', cell);
const shot = async (page: Page, what: string) => {
  mkdirSync(shotsDir, { recursive: true });
  await page.screenshot({ path: join(shotsDir, `offline-device-${what}.png`), fullPage: false });
};

test.describe.configure({ mode: 'serial' });
test.skip(cell !== 'dev', 'the offline device is checked in the dev cell only');

let stack: Stack;
let context: BrowserContext;
let page: Page;
let chatUrl = '';
let laptop = { machineId: '', runtimePort: 0 };

test.beforeAll(async () => {
  test.setTimeout(600_000);
  stack = await startStack('dev', { HUB_FEATURES_DEFAULT: 'ui.hx,runtime.fleet' });
});
test.afterAll(async () => {
  await stack?.stop();
});

const api = async <T>(method: 'get' | 'post', path: string, data?: unknown): Promise<T> => {
  const res = await page.evaluate(
    async ([m, u, body]) => {
      const r = await fetch(u as string, {
        method: (m as string).toUpperCase(),
        headers: body === null ? {} : { 'content-type': 'application/json' },
        body: body === null ? undefined : JSON.stringify(body),
        credentials: 'same-origin',
      });
      return { ok: r.ok, status: r.status, text: await r.text() };
    },
    [method, path, data === undefined ? null : data] as const,
  );
  expect(res.ok, `${method} ${path}: ${res.status} ${res.text}`).toBeTruthy();
  return JSON.parse(res.text) as T;
};

const offlineMark = () => page.locator('.hx-brand .hx-offmark');

test('1. a person signs in, connects a machine and makes a chat', async ({ browser }) => {
  context = await browser.newContext();
  page = await context.newPage();
  await page.goto(`${stack.hub}/signin`);
  await page.locator('input[type=email]').fill('owner@example.com');
  await page.getByRole('button', { name: 'Получить код' }).click();
  await page.locator('.hxa__otp').waitFor();
  await page.waitForTimeout(500);
  await page.locator('input').first().fill(stack.emailCode('owner@example.com'));
  await page.waitForURL((u) => !/\/signin/.test(u.pathname), { timeout: 20_000 });
  const me = await api<{ spaces: { id: string }[] }>('get', '/api/me');
  const spaceId =
    me.spaces[0]?.id ??
    (await api<{ space: { id: string } }>('post', '/api/spaces', { name: 'Main' })).space.id;
  const reg = await api<{ code: string }>('post', `/api/spaces/${spaceId}/machines/codes`, {
    profile: 'full',
    confirmFull: true,
  });
  laptop = await stack.registerMachine({ code: reg.code, name: 'laptop-box' });
  const { machineId } = laptop;
  await expect
    .poll(
      async () =>
        (await api<{ machine: { status: string } }>('get', `/api/machines/${machineId}`)).machine.status,
      {
        timeout: 60_000,
      },
    )
    .toBe('online');

  const fresh = page.getByRole('button', { name: /new chat|новый чат/i }).first();
  await expect(async () => {
    await page.goto(`${stack.hub}/agents/`);
    await expect(fresh).toBeVisible({ timeout: 10_000 });
  }).toPass({ timeout: 120_000 });
  await fresh.click();
  const box = page.getByRole('textbox').last();
  await box.fill('hello before the flight');
  await box.press('Enter');
  await expect(page.getByText('Fake agent reply: hello before the flight').first()).toBeVisible({
    timeout: 90_000,
  });
  await page.waitForURL(/\/agents\/c\//, { timeout: 30_000 });
  chatUrl = page.url();
});

test('2. the service worker keeps the app, the page keeps the chat', async () => {
  // The worker is the production one: it is registered by the page and takes it over.
  await page.evaluate(async () => {
    await navigator.serviceWorker.ready;
  });
  // Opened once more under the worker, so the page itself is kept; the chat is written to IndexedDB a moment later.
  await page.goto(chatUrl);
  await expect(page.getByText('Fake agent reply: hello before the flight').first()).toBeVisible({
    timeout: 30_000,
  });
  await expect
    .poll(() => page.evaluate(() => navigator.serviceWorker.controller !== null), { timeout: 20_000 })
    .toBe(true);
  await expect
    .poll(
      () =>
        page.evaluate(async () => {
          const names = await caches.keys();
          const pageCache = names.find((n) => n.startsWith('hx-app:') && n.endsWith(':page'));
          const appCache = names.find((n) => n.startsWith('hx-app:') && !n.endsWith(':page'));
          if (!pageCache || !appCache) return 0;
          return (
            (await (await caches.open(pageCache)).keys()).length *
            (await (await caches.open(appCache)).keys()).length
          );
        }),
      { timeout: 30_000 },
    )
    .toBeGreaterThan(0);
  // The feed is written to the device on its own beat (FEEDS_DELAY_MS), into the person's IndexedDB.
  await expect
    .poll(
      () =>
        page.evaluate(async () => {
          const person = localStorage.getItem('hx-next-person');
          if (!person) return -1;
          const db = await new Promise<IDBDatabase>((resolve, reject) => {
            const r = indexedDB.open(`hx-next:${person}`);
            r.onsuccess = () => resolve(r.result);
            r.onerror = () => reject(r.error);
          });
          try {
            if (!db.objectStoreNames.contains('feeds')) return -2;
            const req = db.transaction('feeds').objectStore('feeds').count();
            return await new Promise<number>((resolve) => {
              req.onsuccess = () => resolve(req.result);
            });
          } finally {
            db.close();
          }
        }),
      { timeout: 30_000 },
    )
    .toBeGreaterThan(0);
});

test('3. offline, the same app opens with the chat and says it is offline', async () => {
  await context.setOffline(true);
  await page.goto(chatUrl);
  // The app, not a reader: the sidebar and its brand, the chat from the device.
  await expect(page.locator('.hx-brand')).toBeVisible({ timeout: 30_000 });
  await expect(offlineMark()).toBeVisible({ timeout: 30_000 });
  await expect(offlineMark()).toHaveAttribute('aria-label', /сервер|server/);
  await expect(page.getByText('Fake agent reply: hello before the flight').first()).toBeVisible({
    timeout: 30_000,
  });
  await expect(page.getByText(/Офлайн-библиотека|Offline library/)).toHaveCount(0);
  // The chat is in the sidebar, drawn from the hub's list the device kept.
  await expect(page.locator('.hx-item__title', { hasText: 'hello before the flight' }).first()).toBeVisible();
  await expect(page.locator('.hx-net')).toContainText(/Офлайн|Offline/, { timeout: 15_000 });
  await shot(page, 'offline');

  // A message typed offline waits in the outbox.
  const box = page.getByRole('textbox').last();
  await box.fill('written on the plane');
  await box.press('Enter');
  await expect(page.getByText('written on the plane').first()).toBeVisible({ timeout: 15_000 });
  // The page is marked: a reload would lose it, and the way back must not reload.
  await page.evaluate(() => {
    (window as unknown as { __hxStay: number }).__hxStay = 1;
  });
});

test('4. the network comes back without a reload, and the message goes', async () => {
  await context.setOffline(false);
  await expect(offlineMark()).toHaveCount(0, { timeout: 30_000 });
  await expect(page.getByText('Fake agent reply: written on the plane').first()).toBeVisible({
    timeout: 90_000,
  });
  expect(await page.evaluate(() => (window as unknown as { __hxStay?: number }).__hxStay)).toBe(1);
  await shot(page, 'back');
});

// the machine runs on this very computer (its runtime listens on the loopback the browser sees). The install
// opens the app with ?this-computer=…; then, without the hub, the app goes on there: the whole app, agents included.
test('5. the install opens the app, and this computer is paired by itself', async () => {
  await page.goto(`${stack.hub}/agents/?this-computer=${laptop.machineId}&port=${laptop.runtimePort}`);
  await expect(page.getByText(/This computer is connected|Этот компьютер подключён/).first()).toBeVisible({
    timeout: 60_000,
  });
  await expect(page).not.toHaveURL(/this-computer=/);
});

test('6. without the hub, the app opens on this computer and its agent answers', async () => {
  const hubOrigin = new URL(stack.hub).origin;
  // The hub is out of reach; the loopback of this computer is not.
  await context.route(
    (url) => url.origin === hubOrigin,
    (route) => route.abort('internetdisconnected'),
  );
  await page.goto(chatUrl);
  await page.waitForURL((u) => u.hostname === '127.0.0.1' && u.port === String(laptop.runtimePort), {
    timeout: 60_000,
  });
  await expect(page).not.toHaveURL(/hx-local/, { timeout: 20_000 });
  await expect(page.getByText('Fake agent reply: hello before the flight').first()).toBeVisible({
    timeout: 60_000,
  });
  await expect(page.locator('.hx-brand .hx-offmark[data-tone="chip"]')).toBeVisible();
  await shot(page, 'this-computer');
  const box = page.getByRole('textbox').last();
  await box.fill('working on the laptop');
  await box.press('Enter');
  await expect(page.getByText('Fake agent reply: working on the laptop').first()).toBeVisible({
    timeout: 90_000,
  });
});

test('7. the hub is back: the way back leads to the same chat there', async () => {
  await context.unrouteAll({ behavior: 'ignoreErrors' });
  const back = page.locator('.hx-local-back');
  await expect(back).toBeVisible({ timeout: 60_000 });
  await back.click();
  await page.waitForURL((u) => u.origin === new URL(stack.hub).origin, { timeout: 30_000 });
  expect(new URL(page.url()).pathname).toBe(new URL(chatUrl).pathname);
  await expect(page.getByText('Fake agent reply: working on the laptop').first()).toBeVisible({
    timeout: 60_000,
  });
});
