/*
 * Harnessly for Obsidian: the notes you choose, for your harnessly agents.
 *
 * Nothing is shared until you say so. You choose in the Harnessly panel (folders and notes), with tags («never share
 * #private», «always share #agent») and with a note's own frontmatter «harnessly: true | false». The plugin sends
 * the chosen notes, signed with the connection's key, to your harnessly machine through your hub; a note you stop
 * sharing is removed there at once. See README.md.
 *
 * Plain JavaScript on purpose: no build step, the file you read is the file that runs.
 */
'use strict';

const obsidian = require('obsidian');
const {
  Plugin, PluginSettingTab, Setting, Notice, ItemView, Modal, TFile, TFolder,
  requestUrl, setIcon, addIcon, debounce, getAllTags, Platform,
} = obsidian;

const VIEW = 'harnessly-share';
const PROTO = 1;
const MAX_NOTE = 512 * 1024; // the machine takes no bigger note
const MAX_BODY = 900 * 1024; // the hub takes 1 MiB a request
const BATCH = 600 * 1024;
const GAP_MS = 1200; // the hub lets 60 requests a minute through per connection
const PART = 4000; // paths in one part of the full list
const ICON = '<g fill="none" stroke="currentColor" stroke-width="8" stroke-linecap="round" stroke-linejoin="round">'
  + '<path d="M26 14v72"/><path d="M26 52c0-13 10-22 22-22s22 9 22 22v34"/></g>'
  + '<circle cx="80" cy="20" r="8" fill="currentColor"/>';

const DEFAULTS = {
  link: null, // {u: address, s: key, n: connection name, p: plugin base}
  device: '',
  rules: {}, // path → 'share' | 'hide'; '' is the whole vault; the deepest rule wins
  never: ['private'],
  always: [],
  frontmatter: true,
  sent: {}, // path → hash of what the machine has
  lastSync: 0,
  lastError: '',
  pairedAt: 0,
  updateSeen: '',
};

/* ---------- words ---------- */

const WORDS = {
  en: {
    title: 'Harnessly',
    notPaired: 'Not connected',
    notPairedHint: 'In harnessly open Settings → Connections → Obsidian and open the link it shows (or paste its code here).',
    paste: 'Paste the pairing code',
    pair: 'Connect',
    connectedTo: (n, h) => `Connected to «${n}» · ${h}`,
    synced: (w) => `synced ${w}`,
    syncing: 'syncing…',
    never: 'never',
    shared: (a, b) => `${a} of ${b} notes shared`,
    syncNow: 'Sync now',
    settings: 'Settings',
    search: 'Search',
    all: 'All',
    onlyShared: 'Shared',
    neverTags: 'Never share notes tagged',
    alwaysTags: 'Always share notes tagged',
    addTag: 'add tag',
    tooBig: 'Too big to share (over 512 KB)',
    whyDefault: 'Private: nothing chose to share it',
    whyFmOn: 'Shared: frontmatter «harnessly: true»',
    whyFmOff: 'Private: frontmatter «harnessly: false»',
    whyNever: (t) => `Private: tagged #${t}`,
    whyAlways: (t) => `Shared: tagged #${t}`,
    whyNote: (on) => on ? 'Shared: you shared this note' : 'Private: you hid this note',
    whyFolder: (on, f) => on ? `Shared with the folder «${f}»` : `Private with the folder «${f}»`,
    vault: 'Whole vault',
    neverBlocks: (t) => `This note is tagged #${t}, which is never shared. Remove the tag or the rule first.`,
    shareThis: 'Share with harnessly',
    hideThis: 'Stop sharing with harnessly',
    pairTitle: 'Connect this vault to harnessly',
    pairBody: (v, n, h) => `The vault «${v}» connects to «${n}» on ${h}. Only the notes you choose are sent; you can change it any time in the Harnessly panel.`,
    startPick: 'Choose what to share',
    startPickHint: 'Nothing is shared yet: open the panel and switch folders or notes on.',
    startAll: 'Share the whole vault',
    startAllHint: (t) => `Everything except notes tagged ${t}.`,
    badCode: 'This is not a harnessly pairing code',
    replace: (n) => `This vault is connected to «${n}». Connect it to the new one instead?`,
    paired: 'Connected to harnessly',
    keyRefused: 'harnessly refused the key: connect again with a new link from Settings → Connections',
    offline: 'The harnessly machine is offline; the notes go when it is back',
    unlink: 'Disconnect',
    unlinkDesc: 'Removes the shared notes from harnessly and forgets the key.',
    unlinked: 'Disconnected: harnessly no longer has notes of this vault',
    device: 'Device name',
    deviceDesc: 'How harnessly names this device.',
    fmToggle: 'Frontmatter decides',
    fmDesc: '«harnessly: true» or «harnessly: false» in a note overrides the folders and tags.',
    tagsDesc: 'Comma separated, without #. A tag covers its nested tags.',
    openPanel: 'Open the Harnessly panel',
    update: (v) => `Harnessly ${v} is out`,
    updateBtn: 'Update',
    updated: 'Updated: reload Obsidian to use it',
    status: 'Status',
    version: 'Version',
    statusShared: (n) => `harnessly · ${n}`,
    removed: (n) => `${n} removed`,
    sent: (n) => `${n} sent`,
  },
  ru: {
    title: 'Harnessly',
    notPaired: 'Не подключено',
    notPairedHint: 'В harnessly откройте Настройки → Подключения → Obsidian и откройте ссылку оттуда (или вставьте сюда её код).',
    paste: 'Вставьте код привязки',
    pair: 'Подключить',
    connectedTo: (n, h) => `Подключено к «${n}» · ${h}`,
    synced: (w) => `синхронизировано ${w}`,
    syncing: 'синхронизирую…',
    never: 'ещё нет',
    shared: (a, b) => `${a} из ${b} заметок видит агент`,
    syncNow: 'Синхронизировать',
    settings: 'Настройки',
    search: 'Поиск',
    all: 'Все',
    onlyShared: 'Общие',
    neverTags: 'Никогда не делиться с тегами',
    alwaysTags: 'Всегда делиться с тегами',
    addTag: 'тег',
    tooBig: 'Слишком большие (больше 512 КБ)',
    whyDefault: 'Скрыта: ничто не открыло её',
    whyFmOn: 'Открыта: в свойствах «harnessly: true»',
    whyFmOff: 'Скрыта: в свойствах «harnessly: false»',
    whyNever: (t) => `Скрыта: тег #${t}`,
    whyAlways: (t) => `Открыта: тег #${t}`,
    whyNote: (on) => on ? 'Открыта: вы открыли эту заметку' : 'Скрыта: вы скрыли эту заметку',
    whyFolder: (on, f) => on ? `Открыта вместе с папкой «${f}»` : `Скрыта вместе с папкой «${f}»`,
    vault: 'Всё хранилище',
    neverBlocks: (t) => `У заметки тег #${t}, такие никогда не уходят. Сначала уберите тег или правило.`,
    shareThis: 'Открыть для harnessly',
    hideThis: 'Скрыть от harnessly',
    pairTitle: 'Подключить хранилище к harnessly',
    pairBody: (v, n, h) => `Хранилище «${v}» подключится к «${n}» на ${h}. Уходят только заметки, которые вы выберете; выбор можно поменять в любой момент на панели Harnessly.`,
    startPick: 'Выбрать, чем делиться',
    startPickHint: 'Пока ничего не открыто: на панели включите нужные папки и заметки.',
    startAll: 'Открыть всё хранилище',
    startAllHint: (t) => `Всё, кроме заметок с тегами ${t}.`,
    badCode: 'Это не код привязки harnessly',
    replace: (n) => `Хранилище подключено к «${n}». Подключить к новому вместо него?`,
    paired: 'Подключено к harnessly',
    keyRefused: 'harnessly не принял ключ: подключите заново по новой ссылке из Настройки → Подключения',
    offline: 'Машина harnessly не в сети; заметки уйдут, когда она вернётся',
    unlink: 'Отключить',
    unlinkDesc: 'Убирает заметки из harnessly и забывает ключ.',
    unlinked: 'Отключено: в harnessly не осталось заметок этого хранилища',
    device: 'Имя устройства',
    deviceDesc: 'Как harnessly называет это устройство.',
    fmToggle: 'Свойства заметки решают',
    fmDesc: '«harnessly: true» или «harnessly: false» в заметке важнее папок и тегов.',
    tagsDesc: 'Через запятую, без #. Тег включает вложенные.',
    openPanel: 'Открыть панель Harnessly',
    update: (v) => `Вышел Harnessly ${v}`,
    updateBtn: 'Обновить',
    updated: 'Обновлено: перезапустите Obsidian',
    status: 'Состояние',
    version: 'Версия',
    statusShared: (n) => `harnessly · ${n}`,
    removed: (n) => `убрано ${n}`,
    sent: (n) => `отправлено ${n}`,
  },
};

function lang() {
  try {
    return (window.localStorage.getItem('language') || '').startsWith('ru') ? 'ru' : 'en';
  } catch (e) {
    return 'en';
  }
}
const W = WORDS[lang()];

function ago(ms) {
  if (!ms) return W.never;
  const s = Math.max(0, Math.round((Date.now() - ms) / 1000));
  const ru = lang() === 'ru';
  if (s < 45) return ru ? 'только что' : 'just now';
  const m = Math.round(s / 60);
  if (m < 60) return ru ? `${m} мин назад` : `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 24) return ru ? `${h} ч назад` : `${h} h ago`;
  return new Date(ms).toLocaleDateString();
}

/* ---------- bytes ---------- */

const enc = new TextEncoder();

function hex(buf) {
  return Array.from(new Uint8Array(buf), (b) => b.toString(16).padStart(2, '0')).join('');
}

async function sha256(text) {
  return hex(await crypto.subtle.digest('SHA-256', enc.encode(text)));
}

async function hmac(key, body) {
  const k = await crypto.subtle.importKey('raw', enc.encode(key), { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']);
  return hex(await crypto.subtle.sign('HMAC', k, enc.encode(body)));
}

function randomID() {
  const b = new Uint8Array(12);
  crypto.getRandomValues(b);
  return hex(b);
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/* ---------- pairing code ---------- */

// parseCode takes «hly1.…», the obsidian:// link or a pasted URL with ?pair=.
function parseCode(raw) {
  let s = String(raw || '').trim();
  const m = s.match(/[?&]pair=([^&\s]+)/);
  if (m) s = decodeURIComponent(m[1]);
  if (!s.startsWith('hly1.')) return null;
  try {
    const b64 = s.slice(5).replace(/-/g, '+').replace(/_/g, '/');
    const bin = atob(b64 + '==='.slice((b64.length + 3) % 4));
    const json = new TextDecoder().decode(Uint8Array.from(bin, (c) => c.charCodeAt(0)));
    const p = JSON.parse(json);
    const u = new URL(p.u);
    const local = u.hostname === 'localhost' || u.hostname === '127.0.0.1';
    if (!(u.protocol === 'https:' || (local && u.protocol === 'http:')) || typeof p.s !== 'string' || p.s.length < 16) return null;
    if (p.p) {
      const pu = new URL(p.p);
      if (pu.origin !== u.origin) delete p.p;
    }
    return { u: p.u, s: p.s, n: String(p.n || 'harnessly').slice(0, 80), p: p.p || '' };
  } catch (e) {
    return null;
  }
}

function hostOf(link) {
  try {
    return new URL(link.u).host;
  } catch (e) {
    return '';
  }
}

/* ---------- the plugin ---------- */

class HarnesslyPlugin extends Plugin {
  async onload() {
    this.settings = Object.assign({}, DEFAULTS, await this.loadData());
    this.settings.rules = Object.assign({}, this.settings.rules);
    this.settings.sent = Object.assign({}, this.settings.sent);
    if (!this.settings.device) this.settings.device = defaultDevice();
    this.queue = new Set();
    this.full = false;
    this.busy = false;
    this.again = false;
    this.failures = 0;
    this.skipped = new Map(); // path → bytes
    this.state = 'idle'; // idle | syncing | error | offline
    this.listeners = new Set();
    this.update = null;

    addIcon('harnessly', ICON);
    this.registerView(VIEW, (leaf) => new ShareView(leaf, this));
    this.addRibbonIcon('harnessly', W.openPanel, () => this.openPanel());
    this.status = this.addStatusBarItem();
    this.status.addClass('hly-status');
    this.status.onClickEvent(() => this.openPanel());
    this.addSettingTab(new HarnesslySettings(this.app, this));

    this.addCommand({ id: 'open-panel', name: W.openPanel, callback: () => this.openPanel() });
    this.addCommand({ id: 'sync-now', name: W.syncNow, callback: () => this.syncAll() });
    this.addCommand({
      id: 'share-note', name: W.shareThis,
      checkCallback: (checking) => this.noteCommand(checking, true),
    });
    this.addCommand({
      id: 'hide-note', name: W.hideThis,
      checkCallback: (checking) => this.noteCommand(checking, false),
    });
    this.addCommand({ id: 'pair', name: W.paste, callback: () => new PasteModal(this).open() });

    this.registerObsidianProtocolHandler('harnessly', (params) => {
      if (params.pair) this.startPairing(params.pair);
    });

    this.registerEvent(this.app.workspace.on('file-menu', (menu, file) => this.fileMenu(menu, [file])));
    this.registerEvent(this.app.workspace.on('files-menu', (menu, files) => this.fileMenu(menu, files)));

    const later = debounce(() => this.flush(), 2500, true);
    this.later = later;
    const touch = (path) => {
      if (!this.settings.link) return;
      this.queue.add(path);
      later();
      this.changed();
    };
    this.registerEvent(this.app.workspace.on('layout-change', () => this.decorate()));

    // Registered once the vault is loaded: «create» fires for every file while it loads.
    this.app.workspace.onLayoutReady(() => {
      this.registerEvent(this.app.metadataCache.on('changed', (file) => file instanceof TFile && file.extension === 'md' && touch(file.path)));
      this.registerEvent(this.app.vault.on('create', (file) => file instanceof TFile && file.extension === 'md' && touch(file.path)));
      this.registerEvent(this.app.vault.on('delete', (file) => {
        if (file instanceof TFile) touch(file.path);
        else if (file instanceof TFolder) for (const p of Object.keys(this.settings.sent)) if (p.startsWith(file.path + '/')) touch(p);
      }));
      this.registerEvent(this.app.vault.on('rename', (file, old) => {
        this.renameRules(old, file.path);
        if (file instanceof TFile) {
          touch(old);
          touch(file.path);
        } else {
          for (const p of Object.keys(this.settings.sent)) if (p.startsWith(old + '/')) touch(p);
          this.full = true;
          later();
        }
      }));
      this.changed();
      if (this.settings.link) this.syncAll();
      this.checkUpdate();
    });
    this.registerInterval(window.setInterval(() => this.paintStatus(), 60 * 1000));
    this.register(() => window.clearTimeout(this.retry));
  }

  onunload() {
    this.listeners.clear();
  }

  async save() {
    await this.saveData(this.settings);
  }

  /* --- what is shared --- */

  ruleFor(path) {
    const rules = this.settings.rules;
    let p = path;
    for (;;) {
      if (Object.prototype.hasOwnProperty.call(rules, p)) return { rule: rules[p], at: p };
      if (p === '') return null;
      const i = p.lastIndexOf('/');
      p = i < 0 ? '' : p.slice(0, i);
    }
  }

  tagsOf(file) {
    const cache = this.app.metadataCache.getFileCache(file);
    const out = [];
    for (const t of (cache && getAllTags(cache)) || []) {
      const v = t.replace(/^#/, '');
      if (v && !out.includes(v)) out.push(v);
    }
    return out;
  }

  // decide says whether a note is shared and why: frontmatter, «never» tags, the note's own switch, «always» tags,
  // its folder, otherwise private.
  decide(file) {
    const cache = this.app.metadataCache.getFileCache(file);
    const fm = cache && cache.frontmatter ? cache.frontmatter.harnessly : undefined;
    if (this.settings.frontmatter && fm !== undefined && fm !== null) {
      const on = fm === true || fm === 'true' || fm === 'share' || fm === 'yes';
      return { shared: on, why: on ? W.whyFmOn : W.whyFmOff, fm: true };
    }
    const tags = this.tagsOf(file).map((t) => t.toLowerCase());
    const match = (list) => list.find((r) => tags.some((t) => t === r.toLowerCase() || t.startsWith(r.toLowerCase() + '/')));
    const never = match(this.settings.never);
    if (never) return { shared: false, why: W.whyNever(never), never };
    const r = this.ruleFor(file.path);
    if (r && r.at === file.path) return { shared: r.rule === 'share', why: W.whyNote(r.rule === 'share') };
    const always = match(this.settings.always);
    if (always) return { shared: true, why: W.whyAlways(always) };
    if (r) return { shared: r.rule === 'share', why: W.whyFolder(r.rule === 'share', r.at || W.vault) };
    return { shared: false, why: W.whyDefault };
  }

  isShared(file) {
    return file instanceof TFile && file.extension === 'md' && this.decide(file).shared;
  }

  notes() {
    return this.app.vault.getMarkdownFiles();
  }

  sharedCount() {
    let n = 0;
    for (const f of this.notes()) if (this.isShared(f) && !this.skipped.has(f.path)) n++;
    return n;
  }

  // setRule switches a folder ('' for the vault) or a note: the rules under it go, a rule equal to the inherited one is
  // not kept.
  async setRule(path, on, isFolder) {
    const rules = this.settings.rules;
    if (isFolder) {
      for (const k of Object.keys(rules)) if (path === '' || k.startsWith(path + '/')) delete rules[k];
    }
    delete rules[path];
    const inherited = path === '' ? null : this.ruleFor(parentOf(path));
    const want = on ? 'share' : 'hide';
    if (!(inherited ? inherited.rule === want : want === 'hide')) rules[path] = want;
    await this.rulesChanged();
  }

  // setNote switches one note: by its frontmatter when it has «harnessly:», otherwise by a rule.
  async setNote(file, on) {
    const d = this.decide(file);
    if (d.fm) {
      await this.app.fileManager.processFrontMatter(file, (fm) => { fm.harnessly = on; });
      this.queue.add(file.path);
      this.later();
      this.changed();
      return;
    }
    if (on && d.never) {
      new Notice(W.neverBlocks(d.never));
      return;
    }
    await this.setRule(file.path, on, false);
  }

  renameRules(from, to) {
    const rules = this.settings.rules;
    let moved = false;
    for (const k of Object.keys(rules)) {
      if (k === from || k.startsWith(from + '/')) {
        rules[to + k.slice(from.length)] = rules[k];
        delete rules[k];
        moved = true;
      }
    }
    if (moved) this.save();
  }

  async rulesChanged() {
    await this.save();
    this.full = true;
    this.changed();
    if (this.settings.link) this.later();
  }

  noteCommand(checking, on) {
    const file = this.app.workspace.getActiveFile();
    if (!file || file.extension !== 'md') return false;
    if (this.isShared(file) === on) return false;
    if (!checking) this.setNote(file, on);
    return true;
  }

  fileMenu(menu, files) {
    const list = files.filter((f) => f instanceof TFolder || (f instanceof TFile && f.extension === 'md'));
    if (!list.length) return;
    const stats = this.stats();
    const allShared = list.every((f) => (f instanceof TFolder ? this.folderState(f.path, stats) === 'on' : stats.on.has(f.path)));
    menu.addItem((item) => item
      .setTitle(allShared ? W.hideThis : W.shareThis)
      .setIcon('harnessly')
      .onClick(async () => {
        for (const f of list) {
          if (f instanceof TFolder) await this.setRule(f.path === '/' ? '' : f.path, !allShared, true);
          else await this.setNote(f, !allShared);
        }
      }));
  }

  // stats counts the notes and the shared ones of every folder in one pass ('' is the vault).
  stats() {
    const m = new Map();
    const on = new Set();
    for (const f of this.notes()) {
      const yes = this.isShared(f);
      if (yes) on.add(f.path);
      for (let q = parentOf(f.path); ; q = parentOf(q)) {
        let c = m.get(q);
        if (!c) m.set(q, (c = { all: 0, on: 0 }));
        c.all++;
        if (yes) c.on++;
        if (q === '') break;
      }
    }
    return { folders: m, on };
  }

  folderState(path, stats) {
    const key = path === '/' ? '' : path;
    const c = (stats || this.stats()).folders.get(key);
    if (!c || !c.all) {
      const r = this.ruleFor(key);
      return r && r.rule === 'share' ? 'on' : 'off';
    }
    return c.on === c.all ? 'on' : c.on === 0 ? 'off' : 'mixed';
  }

  /* --- what the person sees --- */

  onChange(fn) {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  changed() {
    if (!this._changed) {
      this._changed = debounce(() => {
        this.paintStatus();
        this.decorate();
        for (const fn of this.listeners) fn();
      }, 300, true);
    }
    this._changed();
  }

  paintStatus() {
    if (!this.status) return;
    this.status.empty();
    const dot = this.status.createSpan({ cls: 'hly-dot is-' + this.stateName() });
    dot.setAttr('aria-hidden', 'true');
    this.status.createSpan({ text: this.settings.link ? W.statusShared(this.sharedCount()) : 'harnessly' });
    this.status.setAttr('aria-label', this.statusLine());
    for (const fn of this.listeners) fn('status');
  }

  stateName() {
    if (!this.settings.link) return 'off';
    if (this.busy) return 'busy';
    if (this.state === 'error' || this.state === 'offline') return this.state;
    return 'ok';
  }

  statusLine() {
    if (!this.settings.link) return W.notPaired;
    if (this.busy) return W.syncing;
    if (this.state === 'error' || this.state === 'offline') return this.settings.lastError;
    return W.synced(ago(this.settings.lastSync));
  }

  // decorate marks the shared notes and folders in the file explorer.
  decorate() {
    if (!this._decorate) {
      this._decorate = debounce(() => {
        const stats = this.settings.link ? this.stats() : null;
        for (const leaf of this.app.workspace.getLeavesOfType('file-explorer')) {
          const items = leaf.view && leaf.view.fileItems;
          if (!items) continue;
          for (const [path, item] of Object.entries(items)) {
            const el = item && (item.selfEl || item.titleEl);
            if (!el) continue;
            const file = item.file;
            el.removeClass('hly-on', 'hly-mixed');
            if (!this.settings.link) continue;
            if (file instanceof TFile) {
              if (stats.on.has(file.path)) el.addClass('hly-on');
            } else if (file instanceof TFolder && path !== '/') {
              const s = this.folderState(path, stats);
              if (s === 'on') el.addClass('hly-on');
              else if (s === 'mixed') el.addClass('hly-mixed');
            }
          }
        }
      }, 400, true);
    }
    this._decorate();
  }

  async openPanel() {
    let leaf = this.app.workspace.getLeavesOfType(VIEW)[0];
    if (!leaf) {
      leaf = this.app.workspace.getRightLeaf(false);
      if (!leaf) return;
      await leaf.setViewState({ type: VIEW, active: true });
    }
    this.app.workspace.revealLeaf(leaf);
  }

  /* --- pairing --- */

  startPairing(raw) {
    const link = parseCode(raw);
    if (!link) {
      new Notice(W.badCode);
      return;
    }
    new PairModal(this, link).open();
  }

  async pair(link, shareAll) {
    const old = this.settings.link;
    if (old && old.u !== link.u) this.settings.sent = {};
    this.settings.link = link;
    this.settings.pairedAt = Date.now();
    this.settings.lastError = '';
    this.state = 'idle';
    if (shareAll) {
      this.settings.rules = { '': 'share' };
    }
    await this.save();
    this.full = true;
    await this.flush();
    if (this.state !== 'error' && this.state !== 'offline') new Notice(W.paired);
    this.openPanel();
  }

  async unlink() {
    if (this.settings.link) {
      try {
        // An empty full list removes everything the machine has from this vault.
        await this.send({ manifest: { sync: randomID(), part: 0, parts: 1, paths: [] } });
      } catch (e) {
        // the key may be refused already: forget it all the same
      }
    }
    this.settings.link = null;
    this.settings.sent = {};
    this.settings.lastSync = 0;
    this.settings.lastError = '';
    this.state = 'idle';
    await this.save();
    this.changed();
    new Notice(W.unlinked);
  }

  /* --- sending --- */

  syncAll() {
    if (!this.settings.link) {
      new PasteModal(this).open();
      return;
    }
    this.full = true;
    this.flush();
  }

  async flush() {
    if (!this.settings.link) return;
    if (this.busy) {
      this.again = true;
      return;
    }
    this.busy = true;
    this.changed();
    const full = this.full;
    this.full = false;
    const paths = new Set(this.queue);
    this.queue.clear();
    if (full) {
      for (const f of this.notes()) paths.add(f.path);
      for (const p of Object.keys(this.settings.sent)) paths.add(p);
    }
    try {
      const sent = this.settings.sent;
      const upserts = [];
      const deletes = [];
      for (const p of paths) {
        const file = this.app.vault.getAbstractFileByPath(p);
        if (!(file instanceof TFile) || file.extension !== 'md' || !this.decide(file).shared) {
          this.skipped.delete(p);
          if (sent[p] !== undefined) deletes.push(p);
          continue;
        }
        const text = await this.app.vault.cachedRead(file);
        const bytes = enc.encode(text).length;
        if (bytes > MAX_NOTE) {
          this.skipped.set(p, bytes);
          if (sent[p] !== undefined) deletes.push(p);
          continue;
        }
        this.skipped.delete(p);
        // The links are in the hash: a note is sent again when a note it links to stops (or starts) being shared.
        const item = this.item(file, text);
        const hash = (await sha256(text + '\u0000' + item.title + '\u0000' + item.tags.join(' ') + '\u0000' + item.links.join('\n'))).slice(0, 24);
        if (sent[p] === hash) continue;
        upserts.push({ item, hash });
      }
      let first = true;
      let group = [];
      let size = 0;
      const go = async () => {
        if (!group.length && !(first && deletes.length)) return;
        const body = { upsert: group.map((g) => g.item) };
        if (first && deletes.length) body.delete = deletes;
        await this.send(body);
        for (const g of group) sent[g.item.path] = g.hash;
        if (first) for (const p of deletes) delete sent[p];
        first = false;
        group = [];
        size = 0;
        await this.save();
      };
      for (const u of upserts) {
        const s = enc.encode(JSON.stringify(u.item)).length;
        if (s > MAX_BODY) {
          this.skipped.set(u.item.path, s);
          continue;
        }
        if (size + s > BATCH && group.length) await go();
        group.push(u);
        size += s;
      }
      await go();
      if (full) {
        const list = Object.keys(sent).sort();
        const sync = randomID();
        const parts = Math.max(1, Math.ceil(list.length / PART));
        for (let i = 0; i < parts; i++) {
          await this.send({ manifest: { sync, part: i, parts, paths: list.slice(i * PART, (i + 1) * PART) } });
        }
      }
      this.settings.lastSync = Date.now();
      this.settings.lastError = '';
      this.state = 'idle';
      this.failures = 0;
      await this.save();
    } catch (e) {
      for (const p of paths) this.queue.add(p);
      if (full) this.full = true;
      this.fail(e);
    } finally {
      this.busy = false;
      this.changed();
      this.paintStatus();
    }
    if (this.again && this.settings.link && this.state === 'idle') {
      this.again = false;
      this.later();
    }
  }

  fail(e) {
    const status = e && e.status;
    this.state = status === 503 || status === 0 || status === undefined ? 'offline' : 'error';
    if (status === 401) {
      this.settings.lastError = W.keyRefused;
      this.state = 'error';
      this.save();
      return; // no retry until the person pairs again
    }
    this.settings.lastError = this.state === 'offline' ? W.offline : (e && e.message) || String(e);
    this.failures++;
    const wait = status === 429 ? 61000 : Math.min(10 * 60 * 1000, 15000 * 2 ** Math.min(this.failures - 1, 6));
    window.clearTimeout(this.retry);
    this.retry = window.setTimeout(() => {
      this.state = 'idle';
      this.flush();
    }, wait);
  }

  item(file, text) {
    const cache = this.app.metadataCache.getFileCache(file) || {};
    let title = cache.frontmatter && typeof cache.frontmatter.title === 'string' ? cache.frontmatter.title : '';
    if (!title && cache.headings) {
      const h1 = cache.headings.find((h) => h.level === 1);
      if (h1) title = h1.heading;
    }
    if (!title) title = file.basename;
    // Links only to notes that are shared too: the name of a private note never leaves the vault.
    const links = [];
    const resolved = this.app.metadataCache.resolvedLinks[file.path] || {};
    for (const target of Object.keys(resolved)) {
      const t = this.app.vault.getAbstractFileByPath(target);
      if (t instanceof TFile && t.extension === 'md' && t.path !== file.path && this.isShared(t)) links.push(t.path);
    }
    return {
      path: file.path, text, mtime: file.stat.mtime, ctime: file.stat.ctime,
      title: title.slice(0, 200), tags: this.tagsOf(file).slice(0, 50), links: links.slice(0, 500),
    };
  }

  async send(part) {
    const link = this.settings.link;
    if (!link) throw new Error('not connected');
    const now = Date.now();
    if (this.lastSend && now - this.lastSend < GAP_MS) await sleep(GAP_MS - (now - this.lastSend));
    const body = JSON.stringify(Object.assign({
      v: PROTO, id: randomID(), at: Date.now(), vault: this.app.vault.getName(),
      device: this.settings.device, plugin: this.manifest.version,
    }, part));
    const sig = await hmac(link.s, body);
    let res;
    try {
      res = await requestUrl({
        url: link.u, method: 'POST', body, throw: false, contentType: 'application/json',
        headers: { 'X-Signature-256': 'sha256=' + sig },
      });
    } catch (e) {
      const err = new Error(String((e && e.message) || e));
      err.status = 0;
      throw err;
    } finally {
      this.lastSend = Date.now();
    }
    if (res.status >= 200 && res.status < 300) return;
    const err = new Error(`harnessly answered ${res.status}`);
    err.status = res.status;
    throw err;
  }

  /* --- updates from the hub --- */

  async checkUpdate() {
    const base = this.settings.link && this.settings.link.p;
    if (!base) return;
    try {
      const res = await requestUrl({ url: base + '/manifest.json', throw: false });
      if (res.status !== 200) return;
      const v = res.json && res.json.version;
      if (v && newer(v, this.manifest.version)) {
        this.update = v;
        this.changed();
        if (this.settings.updateSeen !== v) {
          this.settings.updateSeen = v;
          await this.save();
          new Notice(W.update(v));
        }
      }
    } catch (e) {
      // no update today
    }
  }

  async installUpdate() {
    const base = this.settings.link && this.settings.link.p;
    if (!base || !this.update) return;
    const dir = this.manifest.dir;
    const files = {};
    for (const name of ['main.js', 'styles.css', 'manifest.json']) {
      const res = await requestUrl({ url: base + '/' + name, throw: false });
      if (res.status !== 200) throw new Error(`${name}: ${res.status}`);
      files[name] = res.text;
    }
    JSON.parse(files['manifest.json']);
    for (const [name, text] of Object.entries(files)) await this.app.vault.adapter.write(dir + '/' + name, text);
    this.update = null;
    new Notice(W.updated);
    this.changed();
  }
}

function parentOf(path) {
  const i = path.lastIndexOf('/');
  return i < 0 ? '' : path.slice(0, i);
}

function newer(a, b) {
  const pa = String(a).split('.').map(Number);
  const pb = String(b).split('.').map(Number);
  for (let i = 0; i < 3; i++) {
    if ((pa[i] || 0) !== (pb[i] || 0)) return (pa[i] || 0) > (pb[i] || 0);
  }
  return false;
}

function defaultDevice() {
  try {
    if (Platform.isMobile) return Platform.isIosApp ? 'iPhone' : Platform.isAndroidApp ? 'Android' : 'phone';
    const os = require('os');
    return os.hostname().replace(/\.local$/, '');
  } catch (e) {
    return 'Obsidian';
  }
}

/* ---------- the panel ---------- */

class ShareView extends ItemView {
  constructor(leaf, plugin) {
    super(leaf);
    this.plugin = plugin;
    this.open = new Set(['']);
    this.query = '';
    this.only = false;
  }

  getViewType() { return VIEW; }

  getDisplayText() { return W.title; }

  getIcon() { return 'harnessly'; }

  async onOpen() {
    this.off = this.plugin.onChange((what) => (what === 'status' ? this.renderHead() : this.render()));
    this.render();
  }

  async onClose() {
    if (this.off) this.off();
  }

  render() {
    const root = this.contentEl;
    root.empty();
    root.addClass('hly-view');
    this.head = root.createDiv({ cls: 'hly-head' });
    this.renderHead();
    if (!this.plugin.settings.link) {
      this.renderPair(root);
      return;
    }
    this.renderTools(root);
    this.treeEl = root.createDiv({ cls: 'hly-tree' });
    this.renderTree();
    this.renderTags(root);
    this.renderSkipped(root);
  }

  renderHead() {
    if (!this.head) return;
    const p = this.plugin;
    const s = p.settings;
    const head = this.head;
    head.empty();
    const top = head.createDiv({ cls: 'hly-top' });
    const mark = top.createDiv({ cls: 'hly-mark' });
    setIcon(mark, 'harnessly');
    const words = top.createDiv({ cls: 'hly-words' });
    words.createDiv({ cls: 'hly-name', text: W.title });
    const line = words.createDiv({ cls: 'hly-line' });
    line.createSpan({ cls: 'hly-dot is-' + p.stateName() });
    line.createSpan({ text: s.link ? W.connectedTo(s.link.n, hostOf(s.link)) : W.notPaired });
    if (!s.link) return;
    const total = p.notes().length;
    const shared = p.sharedCount();
    const meter = head.createDiv({ cls: 'hly-meter' });
    const bar = meter.createDiv({ cls: 'hly-bar' });
    bar.createDiv({ cls: 'hly-fill' }).style.width = total ? Math.max(shared ? 2 : 0, Math.round((shared / total) * 100)) + '%' : '0';
    const row = meter.createDiv({ cls: 'hly-meter-row' });
    row.createSpan({ cls: 'hly-count', text: W.shared(shared, total) });
    row.createSpan({ cls: 'hly-when', text: p.statusLine() });
    if (p.update) {
      const up = head.createDiv({ cls: 'hly-update' });
      up.createSpan({ text: W.update(p.update) });
      const b = up.createEl('button', { text: W.updateBtn });
      b.onclick = () => p.installUpdate().catch((e) => new Notice(String(e.message || e)));
    }
  }

  renderPair(root) {
    const box = root.createDiv({ cls: 'hly-pair' });
    box.createDiv({ cls: 'hly-hint', text: W.notPairedHint });
    const input = box.createEl('input', { type: 'text', placeholder: W.paste });
    const b = box.createEl('button', { cls: 'mod-cta', text: W.pair });
    b.onclick = () => this.plugin.startPairing(input.value);
  }

  renderTools(root) {
    const tools = root.createDiv({ cls: 'hly-tools' });
    const search = tools.createEl('input', { type: 'search', placeholder: W.search, value: this.query });
    search.oninput = () => {
      this.query = search.value.trim().toLowerCase();
      this.renderTree();
    };
    const seg = tools.createDiv({ cls: 'hly-seg' });
    for (const [only, label] of [[false, W.all], [true, W.onlyShared]]) {
      const b = seg.createEl('button', { text: label, cls: this.only === only ? 'is-active' : '' });
      b.onclick = () => {
        this.only = only;
        this.render();
      };
    }
    const sync = tools.createEl('button', { cls: 'hly-icon-btn', attr: { 'aria-label': W.syncNow } });
    setIcon(sync, 'refresh-cw');
    if (this.plugin.busy) sync.addClass('is-spinning');
    sync.onclick = () => this.plugin.syncAll();
  }

  renderTree() {
    const el = this.treeEl;
    if (!el) return;
    el.empty();
    const p = this.plugin;
    const files = p.notes();
    const decisions = new Map();
    for (const f of files) decisions.set(f.path, p.decide(f));
    if (this.query) {
      const hits = files.filter((f) => f.path.toLowerCase().includes(this.query))
        .filter((f) => !this.only || decisions.get(f.path).shared)
        .sort((a, b) => a.path.localeCompare(b.path)).slice(0, 300);
      for (const f of hits) this.fileRow(el, f, decisions.get(f.path), 0, true);
      return;
    }
    // folder → {folders, files, all, on}
    const tree = new Map();
    const node = (path) => {
      let n = tree.get(path);
      if (!n) {
        n = { path, folders: new Set(), files: [], all: 0, on: 0 };
        tree.set(path, n);
        if (path !== '') node(parentOf(path)).folders.add(path);
      }
      return n;
    };
    node('');
    for (const f of files) {
      const parent = parentOf(f.path);
      node(parent).files.push(f);
      const on = decisions.get(f.path).shared;
      for (let q = parent; ; q = parentOf(q)) {
        const n = node(q);
        n.all++;
        if (on) n.on++;
        if (q === '') break;
      }
    }
    for (const folder of this.app.vault.getAllLoadedFiles()) {
      if (folder instanceof TFolder && folder.path !== '/' && !folder.path.startsWith('.')) node(folder.path);
    }
    this.folderRow(el, tree, tree.get(''), 0, decisions);
  }

  stateOf(n) {
    if (!n.all) {
      const r = this.plugin.ruleFor(n.path);
      return r && r.rule === 'share' ? 'on' : 'off';
    }
    return n.on === n.all ? 'on' : n.on === 0 ? 'off' : 'mixed';
  }

  folderRow(el, tree, n, depth, decisions) {
    if (this.only && n.on === 0 && n.path !== '') return;
    const open = this.open.has(n.path);
    const row = el.createDiv({ cls: 'hly-row is-folder' + (n.path === '' ? ' is-root' : '') });
    row.style.setProperty('--depth', String(depth));
    const chev = row.createSpan({ cls: 'hly-chev' + (open ? ' is-open' : '') });
    setIcon(chev, 'chevron-right');
    const icon = row.createSpan({ cls: 'hly-ico' });
    setIcon(icon, n.path === '' ? 'vault' : open ? 'folder-open' : 'folder');
    row.createSpan({ cls: 'hly-label', text: n.path === '' ? this.app.vault.getName() : n.path.slice(n.path.lastIndexOf('/') + 1) });
    row.createSpan({ cls: 'hly-num', text: `${n.on}/${n.all}` });
    const state = this.stateOf(n);
    const r = this.plugin.ruleFor(n.path);
    row.setAttr('aria-label', r ? W.whyFolder(r.rule === 'share', r.at || W.vault) : W.whyDefault);
    this.toggle(row, state, () => this.plugin.setRule(n.path, state !== 'on', true));
    row.onclick = (e) => {
      if (e.target.closest('.hly-switch')) return;
      if (open) this.open.delete(n.path);
      else this.open.add(n.path);
      this.renderTree();
    };
    if (!open) return;
    const kids = [...n.folders].sort((a, b) => a.localeCompare(b));
    for (const k of kids) this.folderRow(el, tree, tree.get(k), depth + 1, decisions);
    const files = n.files.slice().sort((a, b) => a.basename.localeCompare(b.basename));
    for (const f of files) {
      const d = decisions.get(f.path);
      if (this.only && !d.shared) continue;
      this.fileRow(el, f, d, depth + 1, false);
    }
  }

  fileRow(el, f, d, depth, full) {
    const big = this.plugin.skipped.has(f.path);
    const row = el.createDiv({ cls: 'hly-row is-file' + (big ? ' is-big' : '') });
    row.style.setProperty('--depth', String(depth));
    row.createSpan({ cls: 'hly-chev is-none' });
    const icon = row.createSpan({ cls: 'hly-ico' });
    setIcon(icon, d.fm ? 'file-cog' : d.never ? 'file-lock' : 'file-text');
    row.createSpan({ cls: 'hly-label', text: full ? f.path.replace(/\.md$/, '') : f.basename });
    row.setAttr('aria-label', big ? W.tooBig : d.why);
    this.toggle(row, d.shared && !big ? 'on' : 'off', () => this.plugin.setNote(f, !d.shared));
    row.onclick = (e) => {
      if (e.target.closest('.hly-switch')) return;
      this.app.workspace.getLeaf(false).openFile(f);
    };
  }

  toggle(row, state, fn) {
    const sw = row.createEl('button', { cls: 'hly-switch is-' + state, attr: { role: 'switch', 'aria-checked': state === 'on' ? 'true' : state === 'mixed' ? 'mixed' : 'false' } });
    sw.createSpan({ cls: 'hly-knob' });
    sw.onclick = async (e) => {
      e.stopPropagation();
      sw.addClass('is-pending');
      await fn();
    };
  }

  renderTags(root) {
    const s = this.plugin.settings;
    const box = root.createDiv({ cls: 'hly-tags' });
    for (const [key, label, cls] of [['never', W.neverTags, 'is-never'], ['always', W.alwaysTags, 'is-always']]) {
      const group = box.createDiv({ cls: 'hly-tag-group ' + cls });
      group.createDiv({ cls: 'hly-tag-title', text: label });
      const chips = group.createDiv({ cls: 'hly-chips' });
      for (const t of s[key]) {
        const chip = chips.createSpan({ cls: 'hly-chip', text: '#' + t });
        const x = chip.createSpan({ cls: 'hly-x' });
        setIcon(x, 'x');
        x.onclick = async () => {
          s[key] = s[key].filter((v) => v !== t);
          await this.plugin.rulesChanged();
        };
      }
      const input = chips.createEl('input', { type: 'text', cls: 'hly-chip-input', placeholder: '+ ' + W.addTag });
      input.onkeydown = async (e) => {
        if (e.key !== 'Enter') return;
        const v = input.value.trim().replace(/^#/, '');
        if (!v || s[key].includes(v)) return;
        s[key] = s[key].concat(v);
        await this.plugin.rulesChanged();
      };
    }
  }

  renderSkipped(root) {
    const big = [...this.plugin.skipped.entries()];
    if (!big.length) return;
    const box = root.createDiv({ cls: 'hly-skipped' });
    box.createDiv({ cls: 'hly-tag-title', text: W.tooBig });
    for (const [path, bytes] of big.slice(0, 20)) {
      box.createDiv({ cls: 'hly-skip-row', text: `${path} · ${Math.round(bytes / 1024)} KB` });
    }
  }
}

/* ---------- modals ---------- */

class PairModal extends Modal {
  constructor(plugin, link) {
    super(plugin.app);
    this.plugin = plugin;
    this.link = link;
  }

  onOpen() {
    const { contentEl } = this;
    contentEl.addClass('hly-modal');
    const top = contentEl.createDiv({ cls: 'hly-top' });
    setIcon(top.createDiv({ cls: 'hly-mark is-big' }), 'harnessly');
    top.createEl('h2', { text: W.pairTitle });
    const old = this.plugin.settings.link;
    if (old && old.u !== this.link.u) contentEl.createDiv({ cls: 'hly-warn', text: W.replace(old.n) });
    contentEl.createEl('p', { text: W.pairBody(this.app.vault.getName(), this.link.n, hostOf(this.link)) });
    const choices = contentEl.createDiv({ cls: 'hly-choices' });
    const pick = (all) => {
      this.close();
      this.plugin.pair(this.link, all);
    };
    const c1 = choices.createEl('button', { cls: 'hly-choice mod-cta' });
    c1.createDiv({ cls: 'hly-choice-title', text: W.startPick });
    c1.createDiv({ cls: 'hly-choice-hint', text: W.startPickHint });
    c1.onclick = () => pick(false);
    const c2 = choices.createEl('button', { cls: 'hly-choice' });
    c2.createDiv({ cls: 'hly-choice-title', text: W.startAll });
    c2.createDiv({ cls: 'hly-choice-hint', text: W.startAllHint(this.plugin.settings.never.map((t) => '#' + t).join(', ') || '—') });
    c2.onclick = () => pick(true);
  }

  onClose() {
    this.contentEl.empty();
  }
}

class PasteModal extends Modal {
  constructor(plugin) {
    super(plugin.app);
    this.plugin = plugin;
  }

  onOpen() {
    const { contentEl } = this;
    contentEl.addClass('hly-modal');
    contentEl.createEl('h2', { text: W.pairTitle });
    contentEl.createEl('p', { cls: 'hly-hint', text: W.notPairedHint });
    const input = contentEl.createEl('input', { type: 'text', placeholder: W.paste, cls: 'hly-wide' });
    const b = contentEl.createEl('button', { cls: 'mod-cta', text: W.pair });
    b.onclick = () => {
      this.close();
      this.plugin.startPairing(input.value);
    };
    input.focus();
  }

  onClose() {
    this.contentEl.empty();
  }
}

/* ---------- settings ---------- */

class HarnesslySettings extends PluginSettingTab {
  constructor(app, plugin) {
    super(app, plugin);
    this.plugin = plugin;
  }

  display() {
    const p = this.plugin;
    const s = p.settings;
    const el = this.containerEl;
    el.empty();
    new Setting(el).setName(W.status).setDesc(s.link ? `${W.connectedTo(s.link.n, hostOf(s.link))} · ${p.statusLine()}` : W.notPaired)
      .addButton((b) => b.setButtonText(s.link ? W.syncNow : W.pair).setCta().onClick(() => p.syncAll()))
      .addButton((b) => b.setButtonText(W.openPanel).onClick(() => p.openPanel()));
    if (s.link) {
      new Setting(el).setName(W.unlink).setDesc(W.unlinkDesc)
        .addButton((b) => b.setButtonText(W.unlink).setWarning().onClick(async () => {
          await p.unlink();
          this.display();
        }));
    }
    new Setting(el).setName(W.device).setDesc(W.deviceDesc)
      .addText((t) => t.setValue(s.device).onChange(async (v) => {
        s.device = v.trim().slice(0, 120);
        await p.save();
      }));
    new Setting(el).setName(W.fmToggle).setDesc(W.fmDesc)
      .addToggle((t) => t.setValue(s.frontmatter).onChange(async (v) => {
        s.frontmatter = v;
        await p.rulesChanged();
      }));
    for (const [key, label] of [['never', W.neverTags], ['always', W.alwaysTags]]) {
      new Setting(el).setName(label).setDesc(W.tagsDesc)
        .addText((t) => t.setValue(s[key].join(', ')).onChange(async (v) => {
          s[key] = v.split(',').map((x) => x.trim().replace(/^#/, '')).filter(Boolean);
          await p.rulesChanged();
        }));
    }
    new Setting(el).setName(W.version).setDesc(p.manifest.version + (p.update ? ' → ' + p.update : ''))
      .addButton((b) => {
        b.setButtonText(W.updateBtn).setDisabled(!p.update).onClick(() => p.installUpdate().then(() => this.display(), (e) => new Notice(String(e.message || e))));
      });
  }
}

module.exports = HarnesslyPlugin;

// For the plugin's own tests (internal/connectors/obsidian): read a pairing code the way the plugin does.
HarnesslyPlugin.parseCode = parseCode;
