// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import TriggerPanel from '@/next/features/connections/trigger-panel';
import { readCandidate, type ITriggerApi } from '@/next/features/connections/triggers-api';
import { pinLocale } from '@/next/i18n';

beforeAll(() => pinLocale('ru'));
afterEach(cleanup);

// What the runtime's classifier answers for Mattermost's tools (internal/mcpconn/triggers.go).
const posts = readCandidate({
  tool: 'get_channel_posts',
  description: 'Fetch posts from a channel (GET /channels/{id}/posts).',
  score: 6,
  fits: true,
  plus: ['readOnlyHint', 'takes since: only what is new can be asked'],
  minus: [],
  label: 'New messages in a channel',
  labelRu: 'Новые сообщения в канале',
  readOnly: true,
  args: [
    { name: 'channel_id', type: 'string', required: true, description: 'Channel id.' },
    { name: 'since', type: 'integer', cursor: true },
  ],
});
const post = readCandidate({ tool: 'post_message', fits: false, excluded: true, label: 'x', labelRu: 'x' });

const api = (over: Partial<ITriggerApi> = {}): ITriggerApi => ({
  load: vi.fn(async () => ({ ok: true as const, value: [posts, post] })),
  draft: vi.fn(async () => ({
    ok: true as const,
    value: {
      recipe: {
        trigger: 'message.new',
        tool: 'get_channel_posts',
        args: { channel_id: 'c1', since: '{{cursor:number}}' },
        items: 'posts',
        id: 'id',
        at: 'create_at',
        title: 'message',
        actor: 'user_id',
        cursor: 'create_at',
      },
      samples: [{ id: 'p2', at: '2026-11-21T09:10:00Z', actor: 'u2', title: 'second', url: '' }],
    },
  })),
  test: vi.fn(async () => ({
    ok: true as const,
    value: [{ id: 'p3', at: '', actor: '', title: 'third', url: '' }],
  })),
  save: vi.fn(async () => ({ ok: true as const, value: 'cx1' })),
  eventsUrl: 'https://hub.example.com/events?tab=triggers',
  ...over,
});

describe('TriggerPanel', () => {
  it('shows a chip for each tool that fits, with its name for a person', async () => {
    render(<TriggerPanel api={api()} />);
    expect(await screen.findByRole('button', { name: 'Новые сообщения в канале' })).toBeTruthy();
    expect(screen.getByText('Может быть триггером:')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'x' })).toBeNull();
  });

  it('draws nothing when no tool fits', async () => {
    const a = api({ load: vi.fn(async () => ({ ok: true as const, value: [post] })) });
    const { container } = render(<TriggerPanel api={a} />);
    await waitFor(() => expect(a.load).toHaveBeenCalled());
    expect(container.firstChild).toBeNull();
  });

  it('drafts from one call, checks and saves the recipe with the mode', async () => {
    const a = api();
    render(<TriggerPanel api={a} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Новые сообщения в канале' }));
    const draft = screen.getByRole('button', { name: /Черновик/ });
    expect((draft as HTMLButtonElement).disabled).toBe(true); // channel_id is required
    fireEvent.change(screen.getByRole('textbox', { name: /channel_id/ }), { target: { value: 'c1' } });
    fireEvent.click(draft);
    await screen.findByText('second');
    expect(a.draft).toHaveBeenCalledWith(
      expect.objectContaining({
        tool: 'get_channel_posts',
        cursorArg: 'since',
        cursorNumber: true,
        args: { channel_id: 'c1' },
      }),
    );
    expect((screen.getByRole('textbox', { name: 'items' }) as HTMLInputElement).value).toBe('posts');
    fireEvent.click(screen.getByRole('button', { name: /Проверить/ }));
    await screen.findByText('third');
    fireEvent.click(screen.getByRole('radio', { name: 'Уведомить' }));
    fireEvent.click(screen.getByRole('button', { name: /Сохранить/ }));
    await screen.findByRole('status');
    const [recipe, unmarked, name] = (a.save as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(recipe).toMatchObject({
      trigger: 'message.new',
      tool: 'get_channel_posts',
      items: 'posts',
      id: 'id',
      at: 'create_at',
      mode: 'notify',
      args: { channel_id: 'c1', since: '{{cursor:number}}' },
    });
    expect(unmarked).toBe(false);
    expect(name).toBe('Новые сообщения в канале');
    expect(screen.getByRole('link', { name: 'События → Триггеры' }).getAttribute('href')).toBe(
      'https://hub.example.com/events?tab=triggers',
    );
  });

  it('sends the v2 fields: watch, facts, next page and the script', async () => {
    const a = api();
    render(<TriggerPanel api={a} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Новые сообщения в канале' }));
    fireEvent.change(screen.getByRole('textbox', { name: /channel_id/ }), { target: { value: 'c1' } });
    fireEvent.click(screen.getByRole('button', { name: /Черновик/ }));
    await screen.findByText('second');
    fireEvent.change(screen.getByRole('textbox', { name: /Следить за полями/ }), {
      target: { value: 'status, type' },
    });
    fireEvent.change(screen.getByRole('textbox', { name: /Факты для правил/ }), {
      target: { value: 'status' },
    });
    fireEvent.change(screen.getByRole('textbox', { name: /её курсор в ответе/ }), {
      target: { value: 'next_post_id' },
    });
    fireEvent.change(screen.getByRole('textbox', { name: /в какой аргумент/ }), {
      target: { value: 'after' },
    });
    fireEvent.change(screen.getByRole('textbox', { name: /Скрипт/ }), {
      target: { value: '(a, m) => ({ items: a.posts, memo: m })' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Сохранить/ }));
    await screen.findByRole('status');
    const [recipe] = (a.save as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(recipe).toMatchObject({
      watch: ['status', 'type'],
      facts: ['status'],
      next: { path: 'next_post_id', arg: 'after' },
      script: '(a, m) => ({ items: a.posts, memo: m })',
    });
  });

  it('on an API connection asks a GET path and query, without the classifier', async () => {
    const a = api();
    render(<TriggerPanel api={a} isApi />);
    expect(a.load).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Сделать триггером' }));
    fireEvent.change(screen.getByRole('textbox', { name: /Путь GET/ }), { target: { value: '/v1/jobs' } });
    fireEvent.change(screen.getByRole('textbox', { name: /Параметры/ }), {
      target: { value: 'since={{cursor}}\nstatus=done' },
    });
    fireEvent.change(screen.getByRole('textbox', { name: 'trigger' }), { target: { value: 'job.new' } });
    fireEvent.change(screen.getByRole('textbox', { name: 'id' }), { target: { value: 'id' } });
    fireEvent.click(screen.getByRole('button', { name: /Сохранить/ }));
    await screen.findByRole('status');
    const [recipe] = (a.save as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(recipe).toMatchObject({
      trigger: 'job.new',
      path: '/v1/jobs',
      query: { since: '{{cursor}}', status: 'done' },
      id: 'id',
    });
    expect(recipe.tool).toBeUndefined();
  });
});
