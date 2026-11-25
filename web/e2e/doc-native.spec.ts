// A provider's document or board is drawn by the pane itself when its viewer cannot be embedded (GitHub #140): the
// adapter's embed address is refused (the document) or declared unusable (the board), no remote browser or preview
// port is involved, and the pane still shows the object's real text and board geometry, with the board's second page
// on request, a clear message for an object the person may not read and for a representation the adapter lacks.
// Runs with DOC_NATIVE_E2E set (a runtime of its own and an adapter standing in for the documents service).
import { mkdirSync, writeFileSync } from 'node:fs';
import { createServer, type Server } from 'node:http';
import { join } from 'node:path';

import { expect, type Page, test } from '@playwright/test';

import { freePort, startRuntime, type Runtime } from './runtime';

const PASSWORD = 'correct horse';

test.skip(!process.env.DOC_NATIVE_E2E, 'needs DOC_NATIVE_E2E');
test.describe.configure({ mode: 'serial', timeout: 180_000 });

let rt: Runtime;
let adapter: Server;
let base = '';
/** What the adapter was asked for at /v1/native/board: the cursors, to show the second page was a click. */
const cursors: string[] = [];

// The adapter's objects by their own IDs; the runtime names them by aliases (a lowercase id is its own alias).
const objects: Record<string, { kind: string; title: string }> = {
  plan: { kind: 'artifact', title: 'Launch plan' },
  secret: { kind: 'artifact', title: 'Board minutes' },
  legacy: { kind: 'artifact', title: 'Legacy page' },
  roadmap: { kind: 'board', title: 'Roadmap board' },
};

// A 1x1 PNG.
const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==',
  'base64',
);

const send = (res: import('node:http').ServerResponse, status: number, body: unknown) => {
  res.writeHead(status, { 'content-type': 'application/json' });
  res.end(JSON.stringify(body));
};

test.beforeAll(async () => {
  test.setTimeout(240_000);
  const port = await freePort();
  adapter = createServer((req, res) => {
    let raw = '';
    req.on('data', (c) => (raw += c));
    req.on('end', () => {
      const in_ = raw ? JSON.parse(raw) : {};
      const doc = objects[in_.slug as string];
      switch (req.url) {
        case '/v1/describe':
          return send(res, 200, {
            protocol: 'harnessly.docs/v1',
            name: 'example-docs',
            version: '1',
            capabilities: ['read', 'native', 'native-assets'],
          });
        case '/v1/list':
          return send(res, 200, {
            docs: Object.entries(objects).map(([slug, o]) => ({
              slug,
              kind: o.kind,
              title: o.title,
              url: `https://docs.example.com/d/${slug}`,
              noEmbed: o.kind === 'board',
            })),
          });
        case '/v1/get':
          return doc
            ? send(res, 200, {
                slug: in_.slug,
                kind: doc.kind,
                title: doc.title,
                url: `https://docs.example.com/d/${in_.slug}`,
                noEmbed: doc.kind === 'board',
              })
            : send(res, 404, { error: 'not found' });
        case '/v1/versions':
          return send(res, 200, { versions: [] });
        case '/v1/embed': // the viewer cannot be framed: the adapter refuses an address
          return send(res, 403, { error: 'denied' });
        case '/v1/native':
          if (in_.slug === 'secret') return send(res, 403, { error: 'denied' });
          if (in_.slug === 'legacy')
            return send(res, 200, {
              representation: 'unsupported',
              reason: 'Only the service can show this page',
            });
          return send(res, 200, {
            representation: 'markdown',
            markdown:
              '# Launch plan\n\nShip the **native** reader on Friday.\n\n- write the adapter\n- check the board',
            version: '3',
          });
        case '/v1/native/asset':
          if (in_.asset !== 'logo') return send(res, 404, { error: 'not found' });
          return send(res, 200, { contentType: 'image/png', data: PNG.toString('base64') });
        case '/v1/native/board': {
          cursors.push(in_.cursor ?? '');
          if (!in_.cursor)
            return send(res, 200, {
              representation: 'board',
              assets: true,
              scene: {
                objects: [
                  { id: 'n1', type: 'sticky', x: 100, y: 80, w: 160, h: 80, text: 'Design review' },
                  { id: 'n2', type: 'shape', x: 400, y: 80, w: 160, h: 80, text: 'Ship it' },
                  { id: 'pic', type: 'image', x: 100, y: 200, w: 120, h: 60, asset: 'logo' },
                  { id: 'gone', type: 'image', x: 300, y: 200, w: 120, h: 60, asset: 'missing' },
                ],
                next: 'page2',
              },
            });
          return send(res, 200, {
            representation: 'board',
            scene: {
              objects: [
                { id: 'link', type: 'connector', x: 0, y: 0, from: 'n1', to: 'n2' },
                { id: 'n3', type: 'text', x: 260, y: 300, w: 200, h: 40, text: 'Retro notes' },
              ],
            },
          });
        }
        default:
          return send(res, 501, { error: 'not_supported' });
      }
    });
  });
  await new Promise<void>((ok) => adapter.listen(port, '127.0.0.1', ok));
  const dir = join(process.env.TMPDIR ?? '/tmp', `doc-native-${port}`);
  mkdirSync(dir, { recursive: true });
  const file = join(dir, 'adapters.json');
  writeFileSync(
    file,
    JSON.stringify([
      { name: 'example-docs', kind: 'docs', url: `http://127.0.0.1:${port}`, protocol: 'harnessly.docs/v1' },
    ]),
  );
  // No remote browser and no preview ports: nothing for a frame to load from.
  rt = await startRuntime({ env: { RUNTIME_LOCALE: 'en', RUNTIME_DOC_ADAPTERS_FILE: file } });
});
test.afterAll(async () => {
  await rt?.stop();
  adapter?.close();
});

const signIn = async (page: Page) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.addLocatorHandler(page.getByRole('button', { name: /^(Got it|Понятно)$/ }), (b) => b.click());
  await page.goto(`${rt.url}/`);
  const setup = page.getByTestId('setup-form');
  await setup.locator('input[name=password]').fill(PASSWORD);
  await setup.locator('input[name=confirm]').fill(PASSWORD);
  await setup.locator('button[type=submit]').click();
  await expect(page.locator('.hx-hello')).toBeVisible();
  base = new URL(page.url()).pathname.replace(/\/$/, '');
};

test('provider documents and boards are drawn natively when the embed is unavailable', async ({ page }) => {
  await signIn(page);
  const open = (id: string) => page.goto(`${rt.url}${base}/docs/${id}`);

  // A document: its real text, no frame at all.
  await open('plan');
  const doc = page.getByTestId('native-document');
  await expect(doc.getByRole('heading', { name: 'Launch plan' })).toBeVisible({ timeout: 30_000 });
  await expect(doc).toContainText('Ship the native reader on Friday.');
  await expect(doc.getByRole('listitem')).toHaveText(['write the adapter', 'check the board']);
  await expect(page.locator('iframe')).toHaveCount(0);

  // A board: the first page's geometry, then the second one on a click, with the connector between the notes.
  await open('roadmap');
  const board = page.getByTestId('native-board');
  await expect(board.locator('[data-object-id="n1"]')).toBeVisible({ timeout: 30_000 });
  await expect(board.locator('[data-object-id="n1"]')).toHaveAttribute('data-x', '100');
  await expect(board.locator('[data-object-id="n2"]')).toHaveAttribute('data-x', '400');
  await expect(board).toContainText('Design review');
  // An image object is read through the authorized asset route; one that cannot be read is a placeholder.
  const image = board.getByTestId('native-image');
  await expect(image).toHaveCount(1);
  await expect(image).toHaveAttribute('href', /\/docs\/roadmap\/native\/asset\?id=logo$/);
  await expect(board.getByTestId('native-image-missing')).toHaveCount(1, { timeout: 30_000 });
  await expect(board.locator('[data-object-id="pic"]')).toHaveAttribute('data-object-type', 'image');
  await expect(board.locator('[data-object-id="n3"]')).toHaveCount(0);
  expect(cursors).toEqual(['']);
  await board.getByRole('button', { name: 'Load more objects' }).click();
  await expect(board).toContainText('Retro notes');
  await expect(board.locator('[data-object-id="n3"]')).toHaveAttribute('data-x', '260');
  await expect(board.locator('[data-object-type="connector"][data-from="n1"][data-to="n2"]')).toHaveCount(1);
  await expect(board.getByRole('button', { name: 'Load more objects' })).toHaveCount(0);
  expect(cursors).toEqual(['', 'page2']);
  await expect(page.locator('iframe')).toHaveCount(0);

  // A refusal is about the object.
  await open('secret');
  const denied = page.getByTestId('native-error');
  await expect(denied).toBeVisible({ timeout: 30_000 });
  await expect(denied).toContainText('You do not have access to this document or board');
  await expect(denied).toContainText('Board minutes');

  // A representation the adapter does not offer is said so.
  await open('legacy');
  await expect(page.getByTestId('native-unsupported')).toContainText('Only the service can show this page', {
    timeout: 30_000,
  });
});
