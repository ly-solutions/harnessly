import { describe, expect, it } from 'vitest';
import { classifyInput, parseAddress, resolveInput, searchAddress } from '@/next/features/pane/address';

describe('parseAddress', () => {
  it('reads a bare port as this machine', () => {
    expect(parseAddress('4400')).toEqual({
      target: { kind: 'port', port: 4400 },
      path: '/',
      text: 'localhost:4400',
    });
    expect(parseAddress(':3000')?.target).toEqual({ kind: 'port', port: 3000 });
  });

  it('reads localhost with a path as a port', () => {
    expect(parseAddress('http://localhost:5173/admin?x=1')).toEqual({
      target: { kind: 'port', port: 5173 },
      path: '/admin?x=1',
      text: 'localhost:5173/admin?x=1',
    });
  });

  it('reads a host as a site over https unless http is typed', () => {
    expect(parseAddress('stand-2.preview.example.com/adminpanel')).toEqual({
      target: { kind: 'url', origin: 'https://stand-2.preview.example.com' },
      path: '/adminpanel',
      text: 'stand-2.preview.example.com/adminpanel',
    });
    expect(parseAddress('http://10.0.0.5:8080')?.target).toEqual({
      kind: 'url',
      origin: 'http://10.0.0.5:8080',
    });
  });

  it('is not an address for words and a port-less localhost', () => {
    expect(parseAddress('терминал')).toBeNull();
    expect(parseAddress('localhost')).toBeNull();
    expect(parseAddress('99999')).toBeNull();
    expect(parseAddress('')).toBeNull();
  });
});

describe('classifyInput', () => {
  it('tells a port, a site and words apart', () => {
    expect(classifyInput('4400').kind).toBe('port');
    expect(classifyInput('localhost:4400/x').kind).toBe('port');
    expect(classifyInput('example.org').kind).toBe('url');
    expect(classifyInput('https://example.org/a b').kind).toBe('search');
    expect(classifyInput('hello world').kind).toBe('search');
    expect(classifyInput('hello').kind).toBe('search');
    expect(classifyInput('   ').kind).toBe('empty');
  });

  it('opens a search in DuckDuckGo with the words encoded', () => {
    const input = classifyInput('hello world & more');
    expect(input).toEqual({
      kind: 'search',
      q: 'hello world & more',
      address: {
        target: { kind: 'url', origin: 'https://html.duckduckgo.com' },
        path: '/html/?q=hello%20world%20%26%20more',
        text: '? hello world & more',
      },
    });
  });

  it("reads the bar's own «? words» as the same search, and a bare ? as nothing", () => {
    expect(resolveInput('? hello world')).toEqual(searchAddress('hello world'));
    expect(resolveInput('?')).toBeNull();
    expect(resolveInput('')).toBeNull();
  });

  it('resolves a port to its address', () => {
    expect(resolveInput(':3000')?.target).toEqual({ kind: 'port', port: 3000 });
  });
});
