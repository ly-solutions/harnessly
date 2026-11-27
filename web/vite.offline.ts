import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { Plugin, Rolldown } from 'vite';

type OutputBundle = Rolldown.OutputBundle;

// The app for a device without a network: the service worker keeps the files of this build, so that
// offline the same app opens. It keeps the core at its install — the page's entry and the interface's shell with
// everything they import — and, of the rest, the views the person opened under the previous version (by the name of
// their files): an update brings what was used, not the whole 30 MB of every view and language. A view opened later
// is kept when it loads. The list goes into sw.js at the build; data is kept by the page itself, not here.

/** Files of the build the worker may keep: code, styles, fonts and the app's own pictures. */
const KEPT = /\.(?:js|mjs|css|woff2?|svg|png|webp|wasm)$/;
/** A single file beyond this is left to the network (a speech model, a big library of a rare view). */
const MAX_FILE = 3 * 1024 * 1024;
/** The interface's lazy root (main.tsx loads it): it and what it imports are the core. */
const SHELL = /\/app\/next\/entry\.tsx$/;

type Chunk = Extract<OutputBundle[string], { type: 'chunk' }>;

/** The file and everything it imports statically, with their styles and fonts. */
const closure = (bundle: OutputBundle, roots: readonly string[]): Set<string> => {
  const seen = new Set<string>();
  const visit = (name: string) => {
    if (seen.has(name)) return;
    seen.add(name);
    const file = bundle[name];
    if (file?.type !== 'chunk') return;
    const meta = (
      file as Chunk & { viteMetadata?: { importedCss: Set<string>; importedAssets: Set<string> } }
    ).viteMetadata;
    for (const child of [...file.imports, ...(meta?.importedCss ?? []), ...(meta?.importedAssets ?? [])])
      visit(child);
  };
  for (const root of roots) visit(root);
  return seen;
};

export const offlineApp = (): Plugin => ({
  name: 'harnessly-offline-app',
  apply: 'build',
  writeBundle(options, bundle) {
    const dir = options.dir!;
    const sizeOf = (name: string) => readFileSync(resolve(dir, name)).length;
    const keepable = (name: string) => KEPT.test(name) && !name.endsWith('.map') && name !== 'sw.js';
    const chunks = Object.values(bundle).filter((f): f is Chunk => f.type === 'chunk');
    const roots = chunks
      .filter(
        (c) => (c.isEntry && c.facadeModuleId?.endsWith('/index.html')) || SHELL.test(c.facadeModuleId ?? ''),
      )
      .map((c) => c.fileName);
    if (roots.length < 2) this.error('The offline app found no entry or no shell');
    const core = [...closure(bundle as OutputBundle, roots)].filter(keepable).sort();
    const files = Object.values(bundle)
      .map((f) => f.fileName)
      .filter((name) => keepable(name) && sizeOf(name) <= MAX_FILE)
      .sort();
    const hash = createHash('sha256');
    for (const file of files) hash.update(file);
    const worker = resolve(dir, 'sw.js');
    const source = readFileSync(worker, 'utf8');
    if (!source.includes('/* offline manifest */ null'))
      this.error('sw.js has no place for the offline manifest');
    writeFileSync(
      worker,
      source.replace(
        '/* offline manifest */ null',
        JSON.stringify({ version: hash.digest('hex').slice(0, 16), core, files }),
      ),
    );
  },
});
