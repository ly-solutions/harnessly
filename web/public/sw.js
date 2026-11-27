// Web push for the agent app: shows a notification for a push, brings the clicked conversation to the front and
// keeps the app icon badge honest. It also keeps the app itself for a device without a network: the files
// of the build, and the last page the server gave, so that offline the same app opens and draws what it kept.
// The worker lives next to the app (its directory is its scope), so the same file serves any base path.

const BASE = new URL('./', self.location.href).pathname;
const ICON = `${BASE}logo.svg`;
const PUSH_NAV_CACHE = 'push-nav';
const PUSH_NAV_KEY = `${BASE}_push-pending`;
const isWebKit =
  /AppleWebKit/.test(self.navigator.userAgent) && !/Chrome|Chromium|Edg\//.test(self.navigator.userAgent);

// The build writes the list of its files here (vite.offline.ts); a development page has none and keeps nothing.
const offline = /* offline manifest */ null;
const cachePrefix = 'hx-app:' + encodeURIComponent(BASE) + ':';
const appCache = cachePrefix + offline?.version;
// The page as the server last gave it (its settings are in it): one per base, replaced by every page that loads.
const pageCache = cachePrefix + 'page';
const pageKey = new URL(BASE, self.location.origin).href;
const appFiles = new Set(offline?.files.map((file) => BASE + file));
const coreFiles = new Set(offline?.core.map((file) => BASE + file));
// How long a page waits on the network before the kept one opens: a network that hangs must not keep the app blank.
const PAGE_WAIT_MS = 6000;
// Caches of older workers: the reader of before and builds that are gone.
const ours = (name) => name === appCache || name === pageCache || !name.startsWith(cachePrefix);
const stale = (name) => name.startsWith('hx-reader:') || !ours(name);
/** A file's name without the build's hash: the same view under the next version has the same stem. */
const stemOf = (path) => path.replace(/-[A-Za-z0-9_-]{8}(\.[a-z0-9]+)$/i, '$1');

/** Runs `work` over the items, a few at a time. */
const inTurns = async (items, work, at = 6) => {
  const queue = [...items];
  await Promise.all(
    Array.from({ length: at }, async () => {
      for (let item = queue.shift(); item !== undefined; item = queue.shift()) await work(item);
    }),
  );
};

/**
 * The install: the core of this build, and the views the person used under the builds kept before (by the stem of
 * their names). A file an older cache has under the same name is taken from there, not from the network.
 */
const keepBuild = async () => {
  const cache = await caches.open(appCache);
  const older = [];
  for (const name of await caches.keys())
    if (
      name !== appCache &&
      name !== pageCache &&
      (name.startsWith(cachePrefix) || name.startsWith('hx-reader:'))
    )
      older.push(await caches.open(name));
  const usedStems = new Set();
  for (const old of older)
    for (const req of await old.keys()) usedStems.add(stemOf(new URL(req.url).pathname));
  const keep = async (path, must) => {
    if (await cache.match(path)) return;
    for (const old of older) {
      const hit = await old.match(path);
      if (hit) return cache.put(path, hit);
    }
    try {
      // A build's file never changes under its name: the browser's own cache of the page that just loaded serves it.
      const res = await fetch(new Request(path, { redirect: 'error' }));
      if (!res.ok) throw new Error(`${path}: ${res.status}`);
      await cache.put(path, res);
    } catch (e) {
      if (must) throw e;
    }
  };
  await inTurns(coreFiles, (path) => keep(path, true));
  const used = [...appFiles].filter((path) => !coreFiles.has(path) && usedStems.has(stemOf(path)));
  await inTurns(used, (path) => keep(path, false));
};

self.addEventListener('install', (event) =>
  event.waitUntil(
    (async () => {
      if (offline) await keepBuild();
      await self.skipWaiting();
    })(),
  ),
);
self.addEventListener('activate', (event) =>
  event.waitUntil(
    (async () => {
      for (const name of await caches.keys()) if (stale(name)) await caches.delete(name);
      await self.clients.claim();
    })(),
  ),
);

/** A page of the app (not its API, not a file): what the server answers with index.html. */
const isAppPage = (request, url) => {
  if (request.mode !== 'navigate' || request.destination !== 'document') return false;
  const route = url.pathname.slice(BASE.length);
  return !/^(?:api|_|apps)\b/.test(route) && !/\.[a-z0-9]+$/i.test(route) && route !== 'sw.js';
};

const offlinePage = () =>
  new Response(
    '<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">' +
      '<title>Offline</title><body style="font:16px system-ui;padding:32px;background:#111;color:#ddd">' +
      '<p>Нет сети, а сохранённой копии приложения ещё нет. Откройте его один раз в сети.</p>' +
      '<p>No network, and the app has not been saved on this device yet. Open it once online.</p>',
    { status: 503, headers: { 'Content-Type': 'text/html; charset=utf-8' } },
  );

/** Network first; the kept page when the network fails or hangs. A good page from the network is kept for next time. */
const appPage = async (request) => {
  const cache = await caches.open(pageCache);
  const network = fetch(request).then(async (res) => {
    const html = res.headers.get('content-type')?.includes('text/html');
    if (res.ok && html && !res.redirected) await cache.put(pageKey, res.clone());
    return res;
  });
  let timer;
  const late = new Promise((resolve) => {
    timer = setTimeout(resolve, PAGE_WAIT_MS);
  });
  try {
    const res = await Promise.race([network, late]);
    if (res) return res;
  } catch {
    // No network: the kept page below.
  } finally {
    clearTimeout(timer);
  }
  const kept = await cache.match(pageKey);
  if (kept) return kept;
  return network.catch(offlinePage);
};

/** A file of the build: it never changes under its name, so the kept one is the one. */
const buildFile = async (request) => {
  const cache = await caches.open(appCache);
  const kept = await cache.match(request, { ignoreSearch: true });
  if (kept) return kept;
  const res = await fetch(request);
  if (res.ok && res.type === 'basic') await cache.put(request, res.clone()).catch(() => undefined);
  return res;
};

self.addEventListener('fetch', (event) => {
  if (!offline || event.request.method !== 'GET') return;
  const url = new URL(event.request.url);
  if (url.origin !== self.location.origin || !url.pathname.startsWith(BASE)) return;
  // A check of the network (offline/net.ts) asks the network alone.
  if (url.searchParams.has('probe')) return;
  if (appFiles.has(url.pathname) || url.pathname.startsWith(BASE + 'assets/')) {
    event.respondWith(buildFile(event.request));
    return;
  }
  if (isAppPage(event.request, url)) event.respondWith(appPage(event.request));
});

// The page signs a person out: the kept page (it carries their machine's settings) goes with them.
self.addEventListener('message', (event) => {
  if (event.data?.type === 'hx-forget-page') event.waitUntil(caches.delete(pageCache));
});

const windows = () => self.clients.matchAll({ type: 'window', includeUncontrolled: true });
const isTop = (client) => client.frameType !== 'nested';
const ownsPage = (client) => {
  try {
    const url = new URL(client.url);
    return url.origin === self.location.origin && url.pathname.startsWith(BASE);
  } catch {
    return false;
  }
};

// A push payload is attacker-controlled the moment a subscription leaks: a click goes only to a page of this app.
const trustedUrl = (value) => {
  if (typeof value !== 'string' || !value) return null;
  try {
    const url = new URL(value, self.location.origin);
    if (url.origin !== self.location.origin || !url.pathname.startsWith(BASE)) return null;
    return `${url.pathname}${url.search}${url.hash}`;
  } catch {
    return null;
  }
};

// The documentation page is the app's too, but it opens no chat: a click for a chat goes to a window of the shell.
const inShell = (client) => {
  try {
    return !new URL(client.url).pathname.startsWith(`${BASE}docs/`);
  } catch {
    return false;
  }
};
const conversationUrl = (message) =>
  message.tabId ? `${BASE}?open=${encodeURIComponent(message.tabId)}` : BASE;

const countedKind = (kind) => kind && kind !== 'test' && kind !== 'release' && kind !== 'settled';

// An action waiting for the person's yes is always a system notification, with «Send / Don't send» where
// the browser shows buttons (iOS shows none: a tap opens the chat with the card). «settled» replaces it quietly with
// the outcome once it is answered anywhere.
const alwaysShown = (kind) => kind === 'test' || kind === 'approval' || kind === 'settled';

const updateBadge = async () => {
  if (!self.navigator?.setAppBadge) return;
  const shown = await self.registration.getNotifications();
  const count = shown.filter((n) => countedKind(n.data?.kind)).length;
  if (count) await self.navigator.setAppBadge(count);
  else await self.navigator.clearAppBadge?.();
};

self.addEventListener('push', (event) => {
  let message = {};
  try {
    message = event.data?.json() ?? {};
  } catch {
    message = { title: event.data?.text() ?? '' };
  }

  event.waitUntil(
    (async () => {
      // A person looking at the app gets an in-page notice. WebKit requires a visible notification for every push.
      if (!isWebKit && !alwaysShown(message.kind)) {
        const focused = (await windows()).find(
          (c) => isTop(c) && c.focused && c.visibilityState === 'visible',
        );
        if (focused) {
          focused.postMessage({ type: 'hl:push-message', message });
          return;
        }
      }
      const quiet = message.kind === 'settled';
      const actions = Array.isArray(message.actions)
        ? message.actions
            .filter((a) => a && typeof a.action === 'string' && typeof a.title === 'string')
            .slice(0, 2)
            .map((a) => ({ action: a.action, title: a.title }))
        : [];
      await self.registration.showNotification(message.title || '', {
        body: message.body || '',
        icon: ICON,
        badge: ICON,
        tag: message.tag || undefined,
        renotify: Boolean(message.tag) && message.kind !== 'test' && !quiet,
        silent: quiet,
        requireInteraction: message.kind === 'question' || message.kind === 'approval',
        timestamp: typeof message.at === 'number' ? message.at : Date.now(),
        ...(actions.length ? { actions } : {}),
        data: message,
      });
      await updateBadge();
    })(),
  );
});

self.addEventListener('message', (event) => {
  if (event.data?.type !== 'clear-notifications') return;
  event.waitUntil(
    (async () => {
      const shown = await self.registration.getNotifications();
      // An action still waiting for a yes stays: its buttons answer it, and the chat with its card may not be open.
      shown.filter((n) => n.data?.kind !== 'approval').forEach((n) => n.close());
      await updateBadge();
    })(),
  );
});

// A button of an action's notification answers it with the proposal's single-use token, through the machine the
// subscription came from; then a short notification says what happened (a failure leads to the chat on a tap).
const answerFromNotification = async (message, verb) => {
  const results = message.results || {};
  let title = results.failed || 'Failed — open the chat';
  let body = message.title || '';
  try {
    const query = message.machine ? `?machine=${encodeURIComponent(message.machine)}` : '';
    const res = await fetch(`${BASE}api/push/act${query}`, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ proposal: message.proposal, verb, token: message.token }),
    });
    const answer = await res.json().catch(() => ({}));
    if (res.ok && answer.state === 'done') title = results.approve || 'Sent';
    else if (res.ok && answer.state === 'declined') title = results.decline || 'Not sent';
    else if (answer.result) body = answer.result;
  } catch {
    // The network failed: the card in the chat still waits.
  }
  await self.registration.showNotification(title, {
    body,
    icon: ICON,
    badge: ICON,
    tag: message.tag || undefined,
    silent: true,
    timestamp: Date.now(),
    data: { kind: 'settled', tabId: message.tabId, workspaceId: message.workspaceId, tag: message.tag },
  });
  await updateBadge();
};

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const message = event.notification.data || {};

  if (
    message.kind === 'approval' &&
    (event.action === 'approve' || event.action === 'decline') &&
    message.token &&
    message.proposal
  ) {
    event.waitUntil(answerFromNotification(message, event.action));
    return;
  }

  // A release push reloads every open window of the app onto the new build.
  if (message.kind === 'release') {
    event.waitUntil(
      (async () => {
        await updateBadge();
        const tops = (await windows()).filter(
          (c) => isTop(c) && ownsPage(c) && typeof c.navigate === 'function',
        );
        if (!tops.length) return self.clients.openWindow(BASE);
        await Promise.all(tops.map((c) => c.navigate(c.url).catch(() => undefined)));
        return tops[0].focus();
      })(),
    );
    return;
  }

  const safeUrl = trustedUrl(message.url);
  event.waitUntil(
    (async () => {
      // iOS runs this after the page has already resumed: the page reads the target from this cache on focus.
      if (message.tabId && !safeUrl) {
        const cache = await caches.open(PUSH_NAV_CACHE);
        const nav = { tabId: message.tabId, workspaceId: message.workspaceId || null };
        await cache.put(PUSH_NAV_KEY, new Response(JSON.stringify(nav)));
      }
      await updateBadge();
      const tops = (await windows()).filter((c) => isTop(c) && ownsPage(c));

      if (safeUrl) {
        const existing = tops.find((c) => {
          const url = new URL(c.url);
          return `${url.pathname}${url.search}` === safeUrl.split('#')[0];
        });
        if (existing) return existing.focus();
        const client = tops.find((c) => typeof c.navigate === 'function');
        if (client) {
          await client.navigate(safeUrl);
          return client.focus();
        }
        return self.clients.openWindow(safeUrl);
      }

      const client = tops.find(inShell);
      if (!client && message.tabId) {
        // Only the documentation is open: take that window to the chat.
        const docs = tops.find((c) => typeof c.navigate === 'function');
        if (docs) {
          await docs.navigate(conversationUrl(message));
          return docs.focus();
        }
        return self.clients.openWindow(conversationUrl(message));
      }
      // A push for no chat just brings the app forward, documentation and all.
      if (!client && tops[0]) return tops[0].focus();
      if (client) {
        if (message.tabId) {
          client.postMessage({
            type: 'hl:open-conversation',
            tabId: message.tabId,
            workspaceId: message.workspaceId || null,
          });
        }
        return client.focus();
      }
      return self.clients.openWindow(conversationUrl(message));
    })(),
  );
});
