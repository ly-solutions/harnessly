// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import ErrorLine, { errorHeadline, errorReason } from '@/next/features/chat/error-line';
import { parseErrorText } from '@/next/features/chat/error-text';
import Steps from '@/next/features/chat/steps';
import type { Entry } from '@/next/features/chat/types';
import { t } from '@/next/i18n';

afterEach(cleanup);

const GUARD =
  'Please run /login · API Error: 403 {"error":"anon request failed: failed to detect critical data: fail anon mask critical data: anon error: details - {\\"error\\":\\"Security Guardrail Triggered: Malicious MCP detected. Invalid tools: mcp__harnessly-app__page_call\\",\\"operation\\":\\"forbidden\\"}\\n"}';

describe('ErrorLine', () => {
  it('says the filter’s refusal in one line, with the code apart and the raw text folded', () => {
    const error = parseErrorText(GUARD);
    const { container } = render(<ErrorLine error={error} headline={errorHeadline(error)} />);
    const line = `${t('next.error.guard')}: ${t('next.error.tool', { tool: 'mcp__harnessly-app__page_call' })}`;
    expect(container.querySelector('.hx-err__text')?.textContent).toBe(line);
    expect(container.querySelector('.hx-err__code')?.textContent).toBe('403');
    expect(container.textContent).not.toContain('/login');
    // The raw text is mounted only once «Details» was opened, and stays for the closing animation.
    const more = screen.getByRole('button', { name: t('next.error.details') });
    expect(more.getAttribute('aria-expanded')).toBe('false');
    expect(container.querySelector('.hx-err__pre')).toBeNull();
    fireEvent.click(more);
    expect(more.getAttribute('aria-expanded')).toBe('true');
    expect(container.querySelector('.hx-err__pre')?.textContent).toBe(GUARD);
    expect(screen.getByRole('button', { name: t('next.message.copy') })).toBeTruthy();
    fireEvent.click(more);
    expect(more.getAttribute('aria-expanded')).toBe('false');
    expect(container.querySelector('.hx-err__pre')?.textContent).toBe(GUARD);
  });

  it('draws markup in an error as text', () => {
    const error = parseErrorText('boom <img src=x onerror=alert(1)> <b>bold</b> and more words to say');
    const { container } = render(<ErrorLine error={error} headline={null} />);
    expect(container.querySelector('img, b')).toBeNull();
    expect(container.textContent).toContain('<img src=x onerror=alert(1)>');
  });

  it('offers no «Details» when the line already is the whole text', () => {
    const error = parseErrorText('Prompt is too long');
    const { container } = render(<ErrorLine error={error} headline={errorHeadline(error)} />);
    expect(container.querySelector('.hx-err__text')?.textContent).toBe('Prompt is too long');
    expect(container.querySelector('.hx-err__code')).toBeNull();
    expect(screen.queryByRole('button')).toBeNull();
  });

  it('names several refused tools', () => {
    const error = parseErrorText('API Error: 403 {"error":"Guardrail: Invalid tools: mcp__a__x, mcp__b__y"}');
    expect(errorReason(error)).toBe(t('next.error.tools', { tools: 'mcp__a__x, mcp__b__y' }));
  });
});

describe('the places that show an error', () => {
  const base = { turnId: null, at: 1, rev: 1 };

  it('takes the agent’s error notice apart in the feed', () => {
    const notice = { ...base, id: 'n', t: 'notice', level: 'error', code: 'agent-notice', detail: GUARD };
    const { container } = render(<Steps steps={[notice as unknown as Entry]} live={false} />);
    const row = container.querySelector('.hx-notice');
    expect(row?.querySelector('.hx-err__text')?.textContent).toContain(t('next.error.guard'));
    expect(row?.querySelector('.hx-err__code')?.textContent).toBe('403');
    expect(row?.textContent).not.toContain('/login');
  });

  it('leaves a plain note as it was written', () => {
    const notice = {
      ...base,
      id: 'n',
      t: 'notice',
      level: 'info',
      code: 'agent-notice',
      detail: 'Compacted',
    };
    const { container } = render(<Steps steps={[notice as unknown as Entry]} live={false} />);
    expect(container.querySelector('.hx-notice')?.textContent).toBe('Compacted');
    expect(container.querySelector('.hx-err')).toBeNull();
  });

  it('shows the reason of a failed MCP tool in its card, and a failed command’s output as output', () => {
    const tool = (id: string, name: string, text: string) =>
      ({
        ...base,
        id,
        t: 'tool',
        toolUseId: id,
        tool: name,
        summary: '',
        ok: false,
        output: { text, truncated: null },
      }) as unknown as Entry;
    const answer = 'MCP error -32603: {"status":502,"error":{"message":"Bad gateway from the tracker"}}';
    const { container } = render(
      <Steps
        steps={[tool('a', 'mcp__tracker__get_issue', answer), tool('b', 'Bash', answer)]}
        live={false}
      />,
    );
    const [mcp, bash] = [...container.querySelectorAll('.hx-tool')];
    fireEvent.click(mcp.querySelector('.hx-tool__head') as HTMLElement);
    fireEvent.click(bash.querySelector('.hx-tool__head') as HTMLElement);
    expect(mcp.querySelector('.hx-err__text')?.textContent).toBe('Bad gateway from the tracker');
    expect(mcp.querySelector('.hx-err__code')?.textContent).toBe('502');
    expect(bash.querySelector('.hx-err')).toBeNull();
    expect(bash.querySelector('.hx-cmd__ln')?.textContent).toBe(answer);
  });
});
