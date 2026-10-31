import { describe, expect, it } from 'vitest';
import { suggest, type ISuggestSources } from '@/next/features/pane/suggest';
import type { IListeningPort } from '@/next/core/net/preview';

const port = (n: number, over: Partial<IListeningPort> = {}): IListeningPort => ({
  port: n,
  pid: null,
  owner: null,
  command: 'vite',
  cwd: null,
  group: 'chat',
  kind: 'site',
  responding: true,
  title: null,
  description: null,
  icon: null,
  ...over,
});

const sources = (over: Partial<ISuggestSources> = {}): ISuggestSources => ({
  ports: [port(4400, { title: 'Admin dev' }), port(4401, { responding: false }), port(5000, { kind: 'api' })],
  pins: [{ text: 'admin.example.com', key: 'k' }],
  visits: ['example.org/docs', 'localhost:4400'],
  translate: () => 'Test admin',
  ...over,
});

describe('suggest', () => {
  it('lists servers, then pins, then visits when nothing is typed, without silent or api ports', () => {
    const rows = suggest('', sources());
    expect(rows.map((r) => r.kind)).toEqual(['port', 'pin', 'visit']);
    expect(rows[0]).toMatchObject({ label: 'Admin dev', hint: 'localhost:4400' });
    expect(rows[1].label).toBe('Test admin');
  });

  it('shows a page once even when it is a server and a visit', () => {
    const rows = suggest('', sources());
    expect(rows.filter((r) => r.address.text === 'localhost:4400')).toHaveLength(1);
  });

  it('narrows by the text and ends with the search for it', () => {
    const rows = suggest('docs', sources());
    expect(rows.map((r) => r.id)).toEqual(['visit:example.org/docs', 'search']);
    expect(rows[1].address.text).toBe('? docs');
  });

  it('finds a pin by its shipped name', () => {
    expect(suggest('test adm', sources())[0]).toMatchObject({ kind: 'pin' });
  });

  it('does not offer a search for an address typed in full', () => {
    expect(suggest('example.org', sources({ ports: null, pins: null })).map((r) => r.kind)).toEqual([
      'visit',
    ]);
    expect(suggest('4400', sources()).some((r) => r.kind === 'search')).toBe(false);
  });

  it('works with the servers or pins not known yet', () => {
    expect(suggest('', sources({ ports: null, pins: null })).map((r) => r.kind)).toEqual(['visit', 'visit']);
  });

  it('is limited', () => {
    const visits = Array.from({ length: 20 }, (_, i) => `site${i}.org`);
    expect(suggest('', sources({ ports: null, pins: null, visits }), 5)).toHaveLength(5);
  });
});
