// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import TurnView, { splitAtMessages, type ITurnHandlers } from '@/next/features/chat/turn';
import type { Entry, TurnItem } from '@/next/features/chat/types';
import { t } from '@/next/i18n';

afterEach(cleanup);

const say = (id: string, md: string) => ({ id, t: 'say', md }) as unknown as Entry;
const user = (id: string, text: string) =>
  ({ id, t: 'user', text, attachments: [], clientId: null }) as unknown as Entry;

const handlers: ITurnHandlers = {
  onToggle: vi.fn(),
  onContinueTurn: vi.fn(),
  onRetryTurn: vi.fn(),
  onRetryMessage: vi.fn(),
  onDeleteMessage: vi.fn(),
};

const item = (steps: Entry[], status: 'live' | 'done'): TurnItem =>
  ({
    kind: 'turn',
    key: 'c:1#0',
    turn: {
      id: 'c:1#0',
      author: { provider: 'claude', role: null },
      startedAt: 1,
      endedAt: status === 'live' ? null : 2,
      durationMs: 1,
      toolCount: 0,
      stepCount: steps.length,
      status,
      failure: null,
      answerEntryId: null,
      files: [],
      outputs: [],
    },
    prompt: null,
    woke: null,
    peer: null,
    note: null,
    steps,
    answer: null,
    now: null,
    wait: null,
    endNote: null,
  }) as unknown as TurnItem;

describe('splitAtMessages', () => {
  it('cuts the work where the person steered a message in', () => {
    const { parts, messages } = splitAtMessages([say('a', 'до'), user('u', 'эй'), say('b', 'после')]);
    expect(messages.map((m) => m.id)).toEqual(['u']);
    expect(parts.map((p) => p.map((e) => e.id))).toEqual([['a'], ['b']]);
  });

  it('leaves a turn without such messages whole', () => {
    expect(splitAtMessages([say('a', 'x')]).parts).toHaveLength(1);
  });
});

describe('a message sent while the agent works', () => {
  const steps = [say('a', 'до'), user('u', 'А почему застряло'), say('b', 'после')];

  it.each(['live', 'done'] as const)('is a bubble of the feed, outside the work, in a %s turn', (status) => {
    const { container } = render(
      <TurnView item={item(steps, status)} open machineName="vm" handlers={handlers} />,
    );
    const bubble = screen.getByText('А почему застряло').closest('.hx-msg');
    expect(bubble?.getAttribute('data-kind')).toBe('user');
    expect(bubble?.closest('.hx-work')).toBeNull();
    const kinds = [...container.children].map((node) => node.getAttribute('data-kind'));
    expect(kinds).toEqual(['turn', 'user', 'turn']);
  });

  it('stays in view when the turn is folded', () => {
    render(<TurnView item={item(steps, 'done')} open={false} machineName="vm" handlers={handlers} />);
    expect(screen.getByText('А почему застряло')).toBeTruthy();
  });
});

describe('a turn that failed on a call to the model (HRN-1327)', () => {
  const failed = (detail: string): TurnItem => {
    const base = item([], 'done');
    return {
      ...base,
      turn: { ...base.turn, status: 'failed', failure: { code: 'api-error', detail, retryAt: null } },
    } as unknown as TurnItem;
  };
  const buttons = (container: HTMLElement) =>
    [...container.querySelectorAll('.hx-turnend__acts .hx-btn')].map((node) => node.textContent);

  it('says a filter’s 403 in human words, without «/login», and offers the retry first', () => {
    const detail =
      'Please run /login · API Error: 403 {"error":"anon error: details - {\\"error\\":\\"Security Guardrail Triggered: Malicious MCP detected. Invalid tools: mcp__harnessly-app__page_call\\",\\"operation\\":\\"forbidden\\"}\\n"}';
    const { container } = render(
      <TurnView item={failed(detail)} open machineName="vm" handlers={handlers} />,
    );
    const why = container.querySelector('.hx-turnend__why');
    expect(why?.querySelector('.hx-err__text')?.textContent).toBe(
      `${t('next.error.guard')}: ${t('next.error.tool', { tool: 'mcp__harnessly-app__page_call' })}`,
    );
    expect(why?.querySelector('.hx-err__code')?.textContent).toBe('403');
    expect(container.querySelector('.hx-turnend')?.textContent).not.toContain('/login');
    expect(buttons(container)).toEqual([t('next.turnEnd.retry'), t('next.turnEnd.continue')]);
  });

  it('keeps the general words and «Continue» first for a text of no known kind', () => {
    const { container } = render(
      <TurnView item={failed('Something odd happened')} open machineName="vm" handlers={handlers} />,
    );
    expect(container.querySelector('.hx-err__text')?.textContent).toBe(
      `${t('next.turnEnd.apiError')}: Something odd happened`,
    );
    expect(buttons(container)).toEqual([t('next.turnEnd.continue'), t('next.turnEnd.retry')]);
  });
});
