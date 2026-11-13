import { describe, expect, it } from 'vitest';
import { MARKUP, TASK_STATE, splitMarkup, withImagePaths } from '@/next/features/chat/markup';

describe('splitMarkup', () => {
  it('leaves a text without service blocks as it is', () => {
    expect(splitMarkup('Привет, <div> тут код </div>')).toEqual([
      { key: 't0', text: 'Привет, <div> тут код </div>', block: null },
    ]);
  });

  it('reads a background task report', () => {
    const parts = splitMarkup(
      'Готово?\n<task-notification>\n<task-id>b1</task-id>\n<status>completed</status>\n<summary>Тесты прошли</summary>\n<output-file>/tmp/o.txt</output-file>\n</task-notification>',
    );
    expect(parts[0]?.text).toBe('Готово?');
    expect(parts[1]?.block).toMatchObject({
      kind: MARKUP.task,
      state: TASK_STATE.done,
      summary: 'Тесты прошли',
      output: '/tmp/o.txt',
    });
  });

  it('joins a slash command with its arguments and drops the echo and the caveat', () => {
    const parts = splitMarkup(
      '<local-command-caveat>Caveat</local-command-caveat><command-name>/review</command-name>\n<command-message>review</command-message>\n<command-args>123</command-args>',
    );
    expect(parts).toHaveLength(1);
    expect(parts[0]?.block).toEqual({ kind: MARKUP.command, name: '/review', args: '123' });
  });

  it('reads a shell command, its output and its error', () => {
    const parts = splitMarkup(
      '<bash-input>ls</bash-input><bash-stdout>a\nb</bash-stdout><bash-stderr>no</bash-stderr>',
    );
    expect(parts.map((part) => part.block)).toEqual([
      { kind: MARKUP.shell, command: 'ls' },
      { kind: MARKUP.output, body: 'a\nb', error: false },
      { kind: MARKUP.output, body: 'no', error: true },
    ]);
  });

  it('reads a reminder and a teammate with its attributes', () => {
    const parts = splitMarkup(
      'текст\n<system-reminder>\nПомни\n</system-reminder>\n<teammate-message teammate_id="lead" summary="план">Делаем</teammate-message>',
    );
    expect(parts.map((part) => part.block?.kind ?? part.text)).toEqual([
      'текст',
      MARKUP.reminder,
      MARKUP.teammate,
    ]);
    expect(parts[2]?.block).toMatchObject({ from: 'lead', summary: 'план', body: 'Делаем' });
  });
});

describe('withImagePaths', () => {
  it('turns Claude Code’s picture names into their paths', () => {
    expect(withImagePaths('см. [Image: source: /a/b.png] тут')).toBe('см. /a/b.png тут');
  });
});
