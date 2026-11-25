// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import NativeView from '@/components/agents/docs/native-view';

const json = (status: number, body: unknown) =>
  Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } }),
  );

beforeEach(() => vi.stubGlobal('fetch', vi.fn()));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const mockFetch = (...answers: Promise<Response>[]) => {
  const f = vi.fn();
  for (const a of answers) f.mockReturnValueOnce(a);
  vi.stubGlobal('fetch', f);
  return f;
};

describe('NativeView', () => {
  it('draws Markdown as text and never as markup', async () => {
    mockFetch(json(200, { representation: 'markdown', markdown: '# Plan\n\nBody <script>x</script>' }));
    render(<NativeView slug="plan" title="Plan" version={null} isBoard={false} />);
    expect(await screen.findByRole('heading', { name: 'Plan' })).toBeTruthy();
    expect(document.querySelector('script')).toBeNull();
  });

  it('draws structured blocks', async () => {
    mockFetch(
      json(200, {
        representation: 'structured',
        blocks: [
          { type: 'heading', level: 2, text: 'Goals' },
          { type: 'list', items: ['one', 'two'] },
        ],
      }),
    );
    render(<NativeView slug="plan" title="Plan" version={null} isBoard={false} />);
    expect(await screen.findByRole('heading', { name: 'Goals' })).toBeTruthy();
    expect(screen.getByText('two')).toBeTruthy();
  });

  it('draws board objects at their places and loads the next page', async () => {
    const f = mockFetch(
      json(200, {
        representation: 'board',
        scene: {
          objects: [{ id: 'a', type: 'sticky', x: 10, y: 20, w: 100, h: 50, text: 'Hello' }],
          next: 'p2',
        },
      }),
      json(200, {
        representation: 'board',
        scene: {
          objects: [
            {
              id: 'c',
              type: 'connector',
              x: 0,
              y: 0,
              from: 'a',
              to: 'a',
              points: [
                { x: 1, y: 2 },
                { x: 3, y: 4 },
              ],
            },
          ],
        },
      }),
    );
    const { container } = render(<NativeView slug="b" title="B" version={null} isBoard />);
    expect(await screen.findByText('Hello')).toBeTruthy();
    expect(container.querySelector('[data-object-id="a"]')?.getAttribute('data-x')).toBe('10');
    fireEvent.click(screen.getByRole('button', { name: /more/i }));
    await waitFor(() =>
      expect(container.querySelector('[data-object-id="c"][data-object-type="connector"]')).toBeTruthy(),
    );
    expect(String(f.mock.calls[1][0])).toContain('cursor=p2');
    expect(screen.queryByRole('button', { name: /more/i })).toBeNull();
  });

  it('says an access denial about the object', async () => {
    mockFetch(json(403, { code: 'denied', error: 'You do not have access to this document or board' }));
    render(<NativeView slug="x" title="Roadmap" version={null} isBoard={false} />);
    const alert = await screen.findByTestId('native-error');
    expect(alert.getAttribute('data-denied')).toBe('true');
    expect(alert.textContent).toContain('Roadmap');
  });

  it('says explicitly when the representation is unsupported', async () => {
    mockFetch(json(200, { representation: 'unsupported', reason: 'whiteboard only' }));
    render(<NativeView slug="x" title="X" version={null} isBoard />);
    expect(await screen.findByTestId('native-unsupported')).toBeTruthy();
    expect(screen.getByText('whiteboard only')).toBeTruthy();
  });

  it('draws an image object through the asset route and a placeholder when its read fails', async () => {
    mockFetch(
      json(200, {
        representation: 'board',
        assets: true,
        scene: { objects: [{ id: 'pic', type: 'image', x: 5, y: 6, w: 80, h: 60, asset: 'img 1' }] },
      }),
    );
    render(<NativeView slug="roadmap" title="Roadmap" version={null} isBoard />);
    const img = await screen.findByTestId('native-image');
    expect(img.getAttribute('href')).toContain('/docs/roadmap/native/asset?id=img%201');
    fireEvent.error(img);
    expect(screen.getByTestId('native-image-missing')).toBeTruthy();
    expect(screen.queryByTestId('native-image')).toBeNull();
    expect(screen.getByTestId('native-board')).toBeTruthy();
  });
});
