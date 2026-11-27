// The plugin against a harnessly connector (internal/connectors/obsidian TestPluginEndToEnd): node run.js <pairing code>.
// It pairs, shares one folder, then the whole vault, then hides one note, and prints the notices.
'use strict';
const fs = require('fs');
const path = require('path');
const { makeObsidian, load } = require('./obsidian-mock.js');

globalThis.window = globalThis;
const store = {};
globalThis.localStorage = { getItem: (k) => store[k] || null };

const files = {
  'Projects/alpha.md': { text: '# Alpha\nsee [[beta]] and [[diary]]\n- [ ] ship it 📅 2026-12-01\n', tags: ['project/alpha'], links: ['Projects/beta.md', 'Personal/diary.md'], h1: 'Alpha' },
  'Projects/beta.md': { text: 'beta notes', tags: [] },
  'Projects/secret.md': { text: 'not for agents', fm: { harnessly: false } },
  'Personal/diary.md': { text: 'dear diary', tags: ['private'] },
  'Inbox.md': { text: 'inbox', tags: ['inbox'] },
};

(async () => {
  const { obsidian, app, notices } = makeObsidian('Work', files);
  const Plugin = load(fs.readFileSync(path.join(__dirname, '..', 'main.js'), 'utf8'), obsidian);
  const link = Plugin.parseCode(process.argv[2]);
  if (!link) throw new Error('the plugin did not take the code');
  const p = new Plugin(app, { version: '1.0.0', dir: '.obsidian/plugins/harnessly' });
  await p.onload();
  const send = p.send.bind(p);
  p.send = (x) => { p.lastSend = 0; return send(x); };
  p.later = () => {};
  p.settings.device = 'MacBook';
  const step = async (name) => {
    await p.flush();
    if (p.state !== 'idle') throw new Error(name + ': ' + p.settings.lastError);
    console.log(name, JSON.stringify(Object.keys(p.settings.sent).sort()));
  };
  await p.pair(link, false);
  console.log('paired', JSON.stringify(Object.keys(p.settings.sent)));
  await p.setRule('Projects', true, true);
  await step('projects');
  await p.setRule('', true, true);
  await step('vault');
  await p.setNote(app.vault.getAbstractFileByPath('Projects/beta.md'), false);
  await step('hide-beta');
  await p.setNote(app.vault.getAbstractFileByPath('Personal/diary.md'), true);
  files['Inbox.md'].text = 'inbox, changed';
  p.queue.add('Inbox.md');
  await step('edit');
  console.log('notices', JSON.stringify(notices));
  process.exit(0);
})().catch((e) => {
  console.error(e && e.stack || e);
  process.exit(1);
});
