// A small stand-in for Obsidian's API: enough of the vault, metadata cache and workspace to load main.js and run its
// sync outside Obsidian (internal/connectors/obsidian's end-to-end test, testdata/run.js) and to draw its panel in a
// browser (testdata/panel.html). The vault is a map path → {text, fm, tags, links, mtime}.
'use strict';

function makeObsidian(vaultName, files, opts) {
  opts = opts || {};
  const notices = [];
  class TAbstractFile {}
  class TFile extends TAbstractFile {
    constructor(path, f) {
      super();
      this.path = path;
      this.name = path.slice(path.lastIndexOf('/') + 1);
      this.basename = this.name.replace(/\.[^.]+$/, '');
      this.extension = this.name.slice(this.name.lastIndexOf('.') + 1);
      this.stat = { mtime: f.mtime || 1764158400000, ctime: f.ctime || 1764158400000, size: (f.text || '').length };
    }
  }
  class TFolder extends TAbstractFile {
    constructor(path) {
      super();
      this.path = path;
      this.name = path.slice(path.lastIndexOf('/') + 1);
    }
  }
  const fileOf = new Map();
  const folders = new Map();
  const sync = () => {
    for (const p of Object.keys(files)) {
      if (!fileOf.has(p)) fileOf.set(p, new TFile(p, files[p]));
      for (let i = p.lastIndexOf('/'); i > 0; i = p.lastIndexOf('/', i - 1)) {
        const d = p.slice(0, i);
        if (!folders.has(d)) folders.set(d, new TFolder(d));
      }
    }
    for (const p of [...fileOf.keys()]) if (!files[p]) fileOf.delete(p);
  };
  sync();
  const resolvedLinks = new Proxy({}, {
    get: (_, p) => {
      const f = files[p];
      const out = {};
      for (const l of (f && f.links) || []) out[l] = 1;
      return out;
    },
  });
  const app = {
    vault: {
      getName: () => vaultName,
      getMarkdownFiles: () => (sync(), [...fileOf.values()].filter((f) => f.extension === 'md')),
      getAbstractFileByPath: (p) => (sync(), fileOf.get(p) || folders.get(p) || null),
      getAllLoadedFiles: () => (sync(), [...folders.values(), ...fileOf.values()]),
      cachedRead: async (f) => files[f.path].text,
      on: () => ({}),
      adapter: { write: async () => {} },
    },
    metadataCache: {
      getFileCache: (f) => {
        const x = files[f.path];
        if (!x) return null;
        return { frontmatter: x.fm, tags: (x.tags || []).map((t) => ({ tag: '#' + t })), headings: x.h1 ? [{ level: 1, heading: x.h1 }] : [] };
      },
      on: () => ({}),
      resolvedLinks,
    },
    workspace: {
      onLayoutReady: (cb) => { app._ready = cb; },
      on: () => ({}),
      getLeavesOfType: () => [],
      getActiveFile: () => null,
      getRightLeaf: () => null,
      revealLeaf: () => {},
      getLeaf: () => ({ openFile: () => {} }),
    },
    fileManager: {
      processFrontMatter: async (f, fn) => {
        const x = files[f.path];
        x.fm = Object.assign({}, x.fm);
        fn(x.fm);
      },
    },
  };
  const el = opts.el || (() => {
    const self = new Proxy(function () {}, { get: (_, k) => (k === 'style' ? {} : self), apply: () => self });
    return self;
  });
  class Component {
    register() {}
    registerEvent() {}
    registerInterval() {}
  }
  class Plugin extends Component {
    constructor(a, manifest) {
      super();
      this.app = a;
      this.manifest = manifest;
      this._data = opts.data || null;
    }
    async loadData() { return this._data; }
    async saveData(d) { this._data = JSON.parse(JSON.stringify(d)); }
    addRibbonIcon() { return el(); }
    addStatusBarItem() { return el(); }
    addSettingTab() {}
    addCommand() {}
    registerView() {}
    registerObsidianProtocolHandler() {}
  }
  class ItemView extends Component {
    constructor(leaf) {
      super();
      this.leaf = leaf;
      this.app = app;
      this.contentEl = opts.contentEl ? opts.contentEl() : el();
    }
  }
  class Modal {
    constructor(a) {
      this.app = a;
      this.contentEl = opts.contentEl ? opts.contentEl() : el();
    }
    open() { this.onOpen(); }
    close() {}
  }
  class PluginSettingTab {}
  class Setting {}
  class Notice {
    constructor(m) { notices.push(m); }
  }
  const requestUrl = async (r) => {
    try {
      const res = await fetch(r.url, { method: r.method || 'GET', body: r.body, headers: Object.assign({ 'Content-Type': r.contentType || 'application/json' }, r.headers) });
      const text = await res.text();
      return { status: res.status, text, get json() { return JSON.parse(text); } };
    } catch (e) {
      throw e;
    }
  };
  const debounce = (fn, ms) => {
    let t;
    return () => {
      clearTimeout(t);
      t = setTimeout(fn, ms);
    };
  };
  const getAllTags = (c) => (c.tags || []).map((t) => t.tag);
  const obsidian = {
    Plugin, PluginSettingTab, Setting, Notice, ItemView, Modal, TFile, TFolder, requestUrl,
    setIcon: opts.setIcon || (() => {}), addIcon: () => {}, debounce, getAllTags,
    Platform: { isMobile: false },
  };
  return { obsidian, app, notices, files };
}

// load runs main.js with the stand-in as its «obsidian» module and answers the plugin's class.
function load(source, obsidian) {
  const module = { exports: {} };
  const req = (name) => {
    if (name === 'obsidian') return obsidian;
    if (name === 'os') return { hostname: () => 'test-box' };
    throw new Error('no module ' + name);
  };
  new Function('require', 'module', 'exports', source)(req, module, module.exports);
  return module.exports;
}

if (typeof module !== 'undefined') module.exports = { makeObsidian, load };
