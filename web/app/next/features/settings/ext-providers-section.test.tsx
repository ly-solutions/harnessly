// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { readExtRegistry, type IExtRegistry } from '@/next/features/settings/ext-providers-api';
import ExtProvidersSection from '@/next/features/settings/ext-providers-section';
import { isRefused } from '@/next/features/settings/platform-answer';
import { pinLocale } from '@/next/i18n';

beforeAll(() => pinLocale('en'));
afterEach(cleanup);

// GET <platform>/ext-providers as the runtime answers it (cmd/runtime/routes_extproviders.go).
const answer = {
  providers: [
    {
      id: 'deepseek',
      name: 'DeepSeek',
      baseUrl: 'https://api.deepseek.com/v1',
      host: 'api.deepseek.com',
      api: 'openai',
      preset: 'deepseek',
      hasKey: true,
      models: [{ id: 'deepseek-chat' }, { id: 'deepseek-reasoner', label: 'Reasoner' }],
      modelsAt: 1,
      claudeCli: false,
      createdAt: 1,
      updatedAt: 1,
    },
  ],
  presets: [
    { id: 'deepseek', name: 'DeepSeek', baseUrl: 'https://api.deepseek.com/v1', api: 'openai' },
    {
      id: 'groq',
      name: 'Groq',
      baseUrl: 'https://api.groq.com/openai/v1',
      api: 'openai',
      keyUrl: 'https://k',
    },
    { id: 'custom', name: 'Свой адрес', baseUrl: '', api: 'openai' },
  ],
};

const registry = (): IExtRegistry => {
  const read = readExtRegistry(answer);
  if (isRefused(read)) throw new Error('refused');
  return read;
};

const actions = () => ({
  onAdd: vi.fn(async () => ({ id: 'groq' }) as { id: string } | { error: string }),
  onSave: vi.fn(async () => null),
  onRemove: vi.fn(async () => null),
  onCheck: vi.fn(async () => ({ ok: true, text: 'The connection works' })),
  onRefresh: vi.fn(async () => null),
});

describe('the answer of the registry', () => {
  it('reads the providers without a key in them and names a model by its id when it has no label', () => {
    const read = registry();
    expect(read.providers[0]?.models).toEqual([
      { id: 'deepseek-chat', label: 'deepseek-chat' },
      { id: 'deepseek-reasoner', label: 'Reasoner' },
    ]);
    expect(Object.keys(read.providers[0] ?? {})).not.toContain('key');
  });

  it('keeps the words of a refusal', () => {
    expect(readExtRegistry({ error: 'too-many', message: 'There are too many providers.' })).toEqual({
      refused: 'too-many',
      message: 'There are too many providers.',
    });
  });
});

describe('the list', () => {
  it('shows loading, nothing connected and a failure as three different things', () => {
    const { rerender, container } = render(
      <ExtProvidersSection state={{ k: 'loading' }} usage={null} {...actions()} />,
    );
    expect(container.querySelector('.hx-sk')).toBeTruthy();
    expect(screen.queryByText(/None yet/)).toBeNull();

    rerender(
      <ExtProvidersSection state={{ k: 'ready', providers: [], presets: [] }} usage={{}} {...actions()} />,
    );
    expect(screen.getByText(/None yet/)).toBeTruthy();

    rerender(
      <ExtProvidersSection
        state={{ k: 'failed', text: 'No connection', onRetry: () => {} }}
        usage={null}
        {...actions()}
      />,
    );
    expect(screen.getByRole('alert').textContent).toContain('No connection');
    expect(screen.queryByText(/None yet/)).toBeNull();
  });

  it('says how many models a provider has and what it does', () => {
    render(
      <ExtProvidersSection
        state={{ k: 'ready', ...registry() }}
        usage={{ deepseek: ['Notification summaries'] }}
        {...actions()}
      />,
    );
    expect(screen.getByText('2 models · notification summaries')).toBeTruthy();
  });
});

describe('the form of a new provider', () => {
  const open = (found = actions()) => {
    render(<ExtProvidersSection state={{ k: 'ready', ...registry() }} usage={{}} {...found} />);
    fireEvent.click(screen.getByRole('button', { name: 'Add a provider' }));
    return found;
  };

  it('does not offer a preset that is connected already, and asks a preset only for the key', () => {
    open();
    expect(screen.queryByRole('radio', { name: 'DeepSeek' })).toBeNull();
    expect(screen.getByRole('radio', { name: 'Groq' }).getAttribute('aria-checked')).toBe('true');
    expect(screen.queryByText('Address')).toBeNull();
    expect(screen.queryByText('Which API')).toBeNull();
    expect(screen.getByLabelText('API key')).toBeTruthy();
  });

  it('asks an address of the person’s own for the name, the address and the kind of API', () => {
    open();
    fireEvent.click(screen.getByRole('radio', { name: 'Own address' }));
    expect(screen.getByText('Address')).toBeTruthy();
    expect(screen.getByRole('radiogroup', { name: 'Which API' })).toBeTruthy();
  });

  it('sends the preset’s address with the key and leaves when the machine has added it', async () => {
    const found = open();
    fireEvent.change(screen.getByLabelText('API key'), { target: { value: ' gsk-1 ' } });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Check and add' }));
    });
    expect(found.onAdd).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Groq',
        baseUrl: 'https://api.groq.com/openai/v1',
        api: 'openai',
        key: 'gsk-1',
      }),
    );
    // The form leaves with its exit.
    expect(screen.getByText('New provider').closest('form')?.getAttribute('data-state')).toBe('closing');
  });

  it('stays open and says the reason when the check fails', async () => {
    const found = actions();
    found.onAdd.mockResolvedValue({ error: 'The key is not accepted.' });
    open(found);
    fireEvent.change(screen.getByLabelText('API key'), { target: { value: 'bad' } });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Check and add' }));
    });
    expect(screen.getByRole('alert').textContent).toContain('The key is not accepted.');
    expect(screen.getByText('New provider')).toBeTruthy();
  });
});

describe('an open provider', () => {
  it('lists its models, checks it, and removes it only on the second press', async () => {
    const found = actions();
    render(<ExtProvidersSection state={{ k: 'ready', ...registry() }} usage={{}} {...found} />);
    fireEvent.click(screen.getByRole('button', { name: /DeepSeek/ }));
    expect(screen.getByText('Reasoner')).toBeTruthy();
    // A provider of a preset keeps the preset's address and API.
    expect(screen.queryByText('Address')).toBeNull();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Check' }));
    });
    expect(found.onCheck).toHaveBeenCalledWith('deepseek');
    expect(screen.getByText('The connection works')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
    expect(found.onRemove).not.toHaveBeenCalled();
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Remove for sure?' }));
    });
    expect(found.onRemove).toHaveBeenCalledWith('deepseek');
  });
});
