// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import {
  readExtRegistry,
  readLocalFound,
  type ILocalFound,
} from '@/next/features/settings/ext-providers-api';
import LocalModelCard from '@/next/features/settings/local-model-card';
import { isRefused } from '@/next/features/settings/platform-answer';
import { pinLocale } from '@/next/i18n';

beforeAll(() => pinLocale('en'));
afterEach(cleanup);

// GET <platform>/ext-providers/local as the runtime answers it (cmd/runtime/routes_extproviders.go).
const found = (): ILocalFound => {
  const read = readLocalFound({
    servers: [
      {
        kind: 'ollama',
        name: 'Ollama',
        baseUrl: 'http://127.0.0.1:11434',
        api: 'anthropic',
        context: 4096,
        models: [{ id: 'qwen3-coder:30b' }, { id: 'a' }, { id: 'b' }, { id: 'c' }, { id: 'd' }],
      },
      {
        kind: 'vllm',
        name: 'vLLM',
        baseUrl: 'http://127.0.0.1:8000/v1',
        api: 'openai',
        models: [{ id: 'm' }],
      },
    ],
  });
  if (isRefused(read)) throw new Error('refused');
  return read;
};

const connectedOllama = () => {
  const read = readExtRegistry({
    providers: [
      {
        id: 'local-ollama',
        name: 'Local model · Ollama',
        baseUrl: 'http://127.0.0.1:11434',
        host: '127.0.0.1:11434',
        api: 'anthropic',
        preset: 'local',
        hasKey: false,
        models: [{ id: 'qwen3-coder:30b' }],
      },
    ],
    presets: [],
  });
  if (isRefused(read)) throw new Error('refused');
  return read.providers;
};

describe('the answer of the discovery', () => {
  it('reads the servers with their models and the typed address, and keeps the words of a refusal', () => {
    expect(found().servers.map((s) => [s.kind, s.api, s.models.length])).toEqual([
      ['ollama', 'anthropic', 5],
      ['vllm', 'openai', 1],
    ]);
    expect(found().custom).toBeNull();
    expect(
      readLocalFound({ servers: null, custom: { address: 'http://10.0.0.5:8000', found: false } }),
    ).toEqual({
      servers: [],
      custom: { address: 'http://10.0.0.5:8000', found: false },
    });
    expect(readLocalFound({ error: 'not-local', message: 'Not a local address.' })).toEqual({
      refused: 'not-local',
      message: 'Not a local address.',
    });
  });
});

describe('the card of a local model', () => {
  it('lists the servers found with their models, and says what an agent needs', () => {
    render(
      <LocalModelCard
        state={{ k: 'ready', found: found() }}
        connected={[]}
        onScan={vi.fn()}
        onConnect={vi.fn(async () => null)}
      />,
    );
    expect(screen.getByText('Ollama')).toBeTruthy();
    expect(screen.getByText('127.0.0.1:11434')).toBeTruthy();
    // Four models are named, the rest are counted.
    expect(screen.getByText(/qwen3-coder:30b, a, b, c and 1 more/)).toBeTruthy();
    expect(screen.getAllByRole('button', { name: 'Connect' })).toHaveLength(2);
    expect(screen.getByText(/30B or more/)).toBeTruthy();
    // A server that runs its model with a context too small for an agent says so; one that says nothing does not.
    expect(screen.getAllByRole('note')).toHaveLength(1);
    expect(screen.getByRole('note').textContent).toMatch(/context of 4096 tokens/);
  });

  it('connects the server whose button was pressed, and shows one already connected as such', async () => {
    const onConnect = vi.fn(async (_server: unknown) => null);
    render(
      <LocalModelCard
        state={{ k: 'ready', found: found() }}
        connected={connectedOllama()}
        onScan={vi.fn()}
        onConnect={onConnect}
      />,
    );
    expect(screen.getByText('Connected')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Connect' }));
    await waitFor(() => expect(onConnect).toHaveBeenCalledTimes(1));
    expect(onConnect.mock.calls[0]?.[0]).toMatchObject({ kind: 'vllm', api: 'openai' });
  });

  it('says so when nothing answers, and looks again with the address that was typed', () => {
    const onScan = vi.fn();
    render(
      <LocalModelCard
        state={{
          k: 'ready',
          found: { servers: [], custom: { address: 'http://10.0.0.5:8000', found: false } },
        }}
        connected={[]}
        onScan={onScan}
        onConnect={vi.fn(async () => null)}
      />,
    );
    expect(screen.getByRole('status').textContent).toMatch(/No server found/);
    expect(screen.getByRole('alert').textContent).toMatch(/Nothing answers at http:\/\/10\.0\.0\.5:8000/);
    fireEvent.change(screen.getByLabelText('Address of a server'), {
      target: { value: ' http://10.0.0.5:8000 ' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(onScan).toHaveBeenCalledWith('http://10.0.0.5:8000');
  });

  it('puts the refusal of a connect under the server', async () => {
    render(
      <LocalModelCard
        state={{ k: 'ready', found: found() }}
        connected={[]}
        onScan={vi.fn()}
        onConnect={vi.fn(async () => 'Not a local address.')}
      />,
    );
    fireEvent.click(screen.getAllByRole('button', { name: 'Connect' })[0] as HTMLElement);
    expect((await screen.findByRole('alert')).textContent).toContain('Not a local address.');
  });
});
