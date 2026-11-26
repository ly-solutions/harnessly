import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath, pathToFileURL } from 'node:url';

test('the packaged embed entry only connects a nonce-bearing frame to its configured origin', async t => {
  const out = await mkdtemp(join(tmpdir(), 'app-sdk-test-'));
  t.after(() => rm(out, { recursive: true, force: true }));
  execFileSync(fileURLToPath(new URL('./app-sdk-package.sh', import.meta.url)), [out, '0.0.0']);
  const { connectEmbedded } = await import(pathToFileURL(join(out, 'embed.js')));
  const hostOrigin = 'https://host.example';
  const calls = [];
  const parent = { postMessage: (message, origin) => calls.push({ message, origin }) };
  const view = new EventTarget();
  view.parent = view;
  globalThis.window = view;
  globalThis.location = new URL('https://view.example/path#hl-nonce=test');
  globalThis.document = { querySelector: () => null };
  t.after(() => {
    delete globalThis.window;
    delete globalThis.location;
    delete globalThis.document;
  });
  assert.equal(await connectEmbedded({ hostOrigin }), null);
  view.parent = parent;
  location.hash = '';
  assert.equal(await connectEmbedded({ hostOrigin }), null);
  await assert.rejects(connectEmbedded({ hostOrigin: hostOrigin + '/other' }), TypeError);
  assert.deepEqual(calls, []);

  const { installBridge } = await import(pathToFileURL(join(out, 'bridge-shim.js')));
  location.hash = '#hl-nonce=test';
  installBridge(hostOrigin);
  const [{ message, origin }] = calls;
  assert.equal(origin, hostOrigin);
  assert.equal(message.params.nonce, 'test');
  let started = 0;
  const reply = origin => {
    const event = Object.assign(new Event('message'), {
      source: parent, origin, data: { jsonrpc: '2.0', id: message.id, result: {} },
      ports: [{ start() { started++; } }],
    });
    view.dispatchEvent(event);
  };
  reply('https://other.example');
  assert.equal(started, 0);
  reply(hostOrigin);
  assert.equal(started, 1);
});
