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
  onDefault: vi.fn(async () => null),
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
      <ExtProvidersSection
        state={{ k: 'ready', providers: [], presets: [], accounts: false, launch: false }}
        usage={{}}
        {...actions()}
      />,
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

describe('several accounts of one provider', () => {
  // The same answer from a runtime that keeps accounts: a second DeepSeek account with a name of its own.
  const several = (): IExtRegistry => {
    const first = { ...answer.providers[0], group: 'preset:deepseek', default: true };
    const second = { ...first, id: 'work', name: 'Work', default: false };
    const read = readExtRegistry({ ...answer, providers: [first, second], accounts: true });
    if (isRefused(read)) throw new Error('refused');
    return read;
  };

  it('reads an older runtime as one account of a provider', () => {
    const read = registry();
    expect(read.accounts).toBe(false);
    expect(read.providers[0]).toMatchObject({ group: '', isDefault: false });
  });

  it('marks the default account and makes another the default', async () => {
    const found = actions();
    render(
      <ExtProvidersSection
        state={{ k: 'ready', ...several() }}
        usage={{ work: ['Notification summaries'] }}
        {...found}
      />,
    );
    expect(screen.getByRole('button', { name: /DeepSeek.*default/ })).toBeTruthy();
    expect(screen.queryByRole('button', { name: /Work.*default/ })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /Work/ }));
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Make it the default' }));
    });
    expect(found.onDefault).toHaveBeenCalledWith('work');
    // Removing an account that runs a feature says what becomes of the feature.
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
    expect(
      screen.getByText(/It runs: notification summaries\. They move to the default account/),
    ).toBeTruthy();
  });

  it('adds one more account of a connected provider under a name of its own', async () => {
    const found = actions();
    render(<ExtProvidersSection state={{ k: 'ready', ...several() }} usage={{}} {...found} />);
    fireEvent.click(screen.getByRole('button', { name: /Work/ }));
    fireEvent.click(screen.getByRole('button', { name: 'One more account' }));
    expect(screen.getByText('One more DeepSeek account')).toBeTruthy();
    fireEvent.change(screen.getByPlaceholderText('For example, “Work”'), { target: { value: 'Home' } });
    // The card of the account it was opened from is still leaving: the form's field is the last one.
    const keys = screen.getAllByLabelText('API key');
    fireEvent.change(keys[keys.length - 1] as HTMLElement, { target: { value: 'sk-home' } });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Check and add' }));
    });
    expect(found.onAdd).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'Home', key: 'sk-home', baseUrl: 'https://api.deepseek.com/v1' }),
    );
  });
});

describe('the variables of a launch', () => {
  // An Anthropic-compatible provider of the person's own, with a token the safe keeps and a variable of theirs.
  const corp = (): IExtRegistry => {
    const provider = {
      ...answer.providers[0],
      id: 'corp',
      name: 'Corp',
      api: 'anthropic',
      preset: 'custom',
      group: 'host:llm.corp.example',
      default: true,
      env: [
        { name: 'ANTHROPIC_AUTH_TOKEN', secret: true, set: true },
        { name: 'CORP_TEAM', value: 'platform', secret: false, set: true },
      ],
    };
    const read = readExtRegistry({ ...answer, providers: [provider], accounts: true, launch: true });
    if (isRefused(read)) throw new Error('refused');
    return read;
  };

  it('never has the value of a secret, only that it is set', () => {
    const read = corp();
    expect(read.providers[0]?.env).toEqual([
      { name: 'ANTHROPIC_AUTH_TOKEN', value: '', secret: true, set: true },
      { name: 'CORP_TEAM', value: 'platform', secret: false, set: true },
    ]);
    render(<ExtProvidersSection state={{ k: 'ready', ...corp() }} usage={{}} {...actions()} />);
    fireEvent.click(screen.getByRole('button', { name: /Corp/ }));
    const token = screen.getByLabelText('ANTHROPIC_AUTH_TOKEN') as HTMLInputElement;
    expect(token.value).toBe('');
    expect(token.placeholder).toBe('set, kept in the safe');
  });

  it('sends the whole list: what was typed, the kept secret without a value, nothing for an empty field', async () => {
    const found = actions();
    render(<ExtProvidersSection state={{ k: 'ready', ...corp() }} usage={{}} {...found} />);
    fireEvent.click(screen.getByRole('button', { name: /Corp/ }));
    fireEvent.change(screen.getByLabelText('ANTHROPIC_BASE_URL'), {
      target: { value: 'https://proxy.corp.example/v2' },
    });
    fireEvent.change(screen.getByLabelText('ANTHROPIC_MODEL'), { target: { value: '  ' } });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    });
    expect(found.onSave).toHaveBeenCalledWith(
      'corp',
      expect.objectContaining({
        env: [
          { name: 'ANTHROPIC_AUTH_TOKEN', secret: true },
          { name: 'CORP_TEAM', secret: false, value: 'platform' },
          { name: 'ANTHROPIC_BASE_URL', secret: false, value: 'https://proxy.corp.example/v2' },
        ],
      }),
    );
  });

  it('is not offered for a provider Claude Code cannot run on', () => {
    const read = corp();
    const openai = { ...read, providers: read.providers.map((p) => ({ ...p, api: 'openai' as const })) };
    render(<ExtProvidersSection state={{ k: 'ready', ...openai }} usage={{}} {...actions()} />);
    fireEvent.click(screen.getByRole('button', { name: /Corp/ }));
    expect(screen.queryByLabelText('ANTHROPIC_BASE_URL')).toBeNull();
  });
});
