// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';
import { frameOrigin, fromFrame, replyOrigin } from '@/next/features/designly/bridge-origin';
import { readBridgeMessage } from '@/next/features/designly/page-bridge';

const win = {};

describe('whose message is believed', () => {
  it('is the guest frame saying null', () => {
    expect(fromFrame({ source: win, origin: 'null' }, win, '*')).toBe(true);
  });
  it('is not another window', () => {
    expect(fromFrame({ source: {}, origin: 'null' }, win, '*')).toBe(false);
    expect(fromFrame({ source: win, origin: 'null' }, null, '*')).toBe(false);
  });
  it('is not a guest message with an origin of its own, nor one naming itself *', () => {
    expect(fromFrame({ source: win, origin: 'https://evil.example' }, win, '*')).toBe(false);
    expect(fromFrame({ source: win, origin: '*' }, win, '*')).toBe(false);
  });
  it('is the slot origin for the owner', () => {
    expect(fromFrame({ source: win, origin: 'https://a.b' }, win, 'https://a.b')).toBe(true);
    expect(fromFrame({ source: win, origin: 'null' }, win, 'https://a.b')).toBe(false);
  });
});

describe('where a message to a frame goes', () => {
  it('is the frame origin for a slot of its own host', () => {
    expect(frameOrigin('https://1-k.preview.example/x/s1.dc.html')).toBe('https://1-k.preview.example');
  });
  it('is the window itself for a slot by path, which is sandboxed to an opaque origin', () => {
    expect(frameOrigin('https://ai.example/m/m1/__pv/1-key/x/s1.dc.html')).toBe('*');
    expect(frameOrigin('/m/m1/__pv/1-key/s1.dc.html')).toBe('*');
  });
  it('is not a path that only mentions the slot prefix', () => {
    expect(frameOrigin('https://a.b/__pv')).toBe('https://a.b');
  });
  it('answers an opaque sender at its window', () => {
    expect(replyOrigin('null')).toBe('*');
    expect(replyOrigin('https://a.b')).toBe('https://a.b');
  });
});

describe('what a board may say', () => {
  it('drops crooked controls and caps their number', () => {
    const many = Array.from({ length: 100 }, (_, i) => ({ name: `n${i}`, value: i }));
    const read = readBridgeMessage({ type: 'tk:tweaks', controls: [null, 5, { name: 3 }, ...many] });
    expect(read?.k === 'tweaks' && read.controls.length).toBeLessThanOrEqual(40);
    expect(readBridgeMessage({ type: 'tk:tweaks', controls: 'x' })).toBeNull();
  });
  it('refuses a pick with a wild box or no selector', () => {
    const base = { type: 'tk:picked', selector: 'p', tag: 'p', rect: { x: 0, y: 0, width: 1, height: 1 } };
    expect(readBridgeMessage({ ...base, rect: { x: 0, y: 0, width: -1, height: 1 } })).toBeNull();
    expect(readBridgeMessage({ ...base, selector: '' })).toBeNull();
    const far = readBridgeMessage({ ...base, rect: { x: 1e12, y: 0, width: 1, height: 1 } });
    expect(far?.k === 'picked' && far.picked.rect.x).toBe(1e6);
  });
});

describe('a board read from the store (HRN-1262)', () => {
  it('is addressed as * and believed saying null', () => {
    const src = '/agents/api/m1/design/store/demo/f/Scene%201.dc.html';
    expect(frameOrigin(src)).toBe('*');
    expect(fromFrame({ source: win, origin: 'null' }, win, frameOrigin(src))).toBe(true);
  });
  it('is not a page of another origin naming the store path', () => {
    expect(frameOrigin('https://evil.example/api/m1/design/store/x/f/a.html')).toBe('https://evil.example');
  });
});
